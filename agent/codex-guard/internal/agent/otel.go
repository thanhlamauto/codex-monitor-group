package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type otelStore struct {
	Days map[string]normalizedUsage `json:"days"`
	Seen map[string]int64           `json:"seen"`
}

func otelValue(value map[string]any) any {
	for _, key := range []string{"stringValue", "intValue", "doubleValue", "boolValue"} {
		if found, ok := value[key]; ok {
			return found
		}
	}
	return nil
}

func otelAttributes(items []any) map[string]any {
	out := map[string]any{}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		key, _ := item["key"].(string)
		value, _ := item["value"].(map[string]any)
		if key != "" {
			out[key] = otelValue(value)
		}
	}
	return out
}

func asUint(value any) uint64 {
	switch v := value.(type) {
	case string:
		n, _ := strconv.ParseUint(v, 10, 64)
		return n
	case float64:
		if v > 0 {
			return uint64(v)
		}
	}
	return 0
}

func field(attrs map[string]any, keys ...string) uint64 {
	for _, key := range keys {
		if value, ok := attrs[key]; ok {
			return asUint(value)
		}
	}
	return 0
}

func loadOTelStore(path string) otelStore {
	store := otelStore{Days: map[string]normalizedUsage{}, Seen: map[string]int64{}}
	data, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(data, &store)
	}
	if store.Days == nil {
		store.Days = map[string]normalizedUsage{}
	}
	if store.Seen == nil {
		store.Seen = map[string]int64{}
	}
	return store
}

func saveOTelStore(path string, store otelStore) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *Client) ingestOTLP(data []byte) (int, error) {
	var body map[string]any
	if err := json.Unmarshal(data, &body); err != nil {
		return 0, err
	}
	location, err := time.LoadLocation(a.Config.Timezone)
	if err != nil {
		return 0, err
	}
	path := filepath.Join(a.Config.StateDir, "otel-usage.json")
	a.otelMu.Lock()
	defer a.otelMu.Unlock()
	store := loadOTelStore(path)
	created := 0
	cutoff := time.Now().AddDate(0, 0, -a.Config.RetentionDays).Unix()
	for id, seen := range store.Seen {
		if seen < cutoff {
			delete(store.Seen, id)
		}
	}
	resources, _ := body["resourceLogs"].([]any)
	for _, rr := range resources {
		resource, _ := rr.(map[string]any)
		scopes, _ := resource["scopeLogs"].([]any)
		for _, ss := range scopes {
			scope, _ := ss.(map[string]any)
			records, _ := scope["logRecords"].([]any)
			for _, raw := range records {
				record, _ := raw.(map[string]any)
				items, _ := record["attributes"].([]any)
				attrs := otelAttributes(items)
				if attrs["event.name"] != "codex.sse_event" || attrs["event.kind"] != "response.completed" {
					continue
				}
				input := field(attrs, "input_token_count")
				cached := field(attrs, "cached_token_count")
				output := field(attrs, "output_token_count")
				reasoning := field(attrs, "reasoning_token_count")
				total := field(attrs, "tool_token_count")
				if total == 0 {
					total = input + output
				}
				if total == 0 {
					continue
				}
				nanos, _ := record["timeUnixNano"].(string)
				identity := fmt.Sprintf("%s|%v|%v|%d|%d|%d|%d", nanos, record["traceId"], record["spanId"], input, cached, output, reasoning)
				sum := sha256.Sum256([]byte(identity))
				id := hex.EncodeToString(sum[:])
				if _, ok := store.Seen[id]; ok {
					continue
				}
				occurred := time.Now()
				if n, parseErr := strconv.ParseInt(nanos, 10, 64); parseErr == nil && n > 0 {
					occurred = time.Unix(0, n)
				}
				day := occurred.In(location).Format("2006-01-02")
				value := store.Days[day]
				value.Date = day
				value.InputTokens += input
				value.CachedInputTokens += cached
				value.OutputTokens += output
				value.ReasoningOutputTokens += reasoning
				value.TotalTokens += total
				store.Days[day] = value
				store.Seen[id] = time.Now().Unix()
				created++
			}
		}
	}
	return created, saveOTelStore(path, store)
}

func (a *Client) StartOTelServer(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.Config.OTelListen)
	if err != nil {
		return fmt.Errorf("start local OTel collector: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/logs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+a.Config.OTLPToken {
			http.Error(w, "unauthorized", 401)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1_048_577))
		if err != nil || len(body) > 1_048_576 {
			http.Error(w, "invalid request", 413)
			return
		}
		if _, err := a.ingestOTLP(body); err != nil {
			http.Error(w, "invalid OTLP JSON", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"partialSuccess":{}}`))
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	go func() { _ = server.Serve(listener) }()
	return nil
}

func (a *Client) OTelUsage() error {
	a.otelMu.Lock()
	store := loadOTelStore(filepath.Join(a.Config.StateDir, "otel-usage.json"))
	a.otelMu.Unlock()
	if len(store.Days) == 0 {
		return nil
	}
	days := make([]normalizedUsage, 0, len(store.Days))
	for _, day := range store.Days {
		days = append(days, day)
	}
	if err := a.enqueue("/api/v1/usage", map[string]any{"source": "otel", "days": days}); err != nil {
		return err
	}
	return a.Flush()
}
