package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/config"
)

func TestOTelCollectorFiltersPrivateFields(t *testing.T) {
	state := t.TempDir()
	a := &Client{Config: &config.Config{StateDir: state, Timezone: "UTC", RetentionDays: 30}}
	payload := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"timeUnixNano":"1000000000","attributes":[{"key":"event.name","value":{"stringValue":"codex.sse_event"}},{"key":"event.kind","value":{"stringValue":"response.completed"}},{"key":"input_token_count","value":{"stringValue":"100"}},{"key":"cached_token_count","value":{"intValue":"40"}},{"key":"output_token_count","value":{"stringValue":"20"}},{"key":"reasoning_token_count","value":{"intValue":"5"}},{"key":"tool_token_count","value":{"stringValue":"120"}},{"key":"prompt","value":{"stringValue":"private prompt"}},{"key":"output","value":{"stringValue":"private source code"}}]}]}]}]}`
	created, err := a.ingestOTLP([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatal(created)
	}
	data, _ := os.ReadFile(filepath.Join(state, "otel-usage.json"))
	if strings.Contains(string(data), "private") || strings.Contains(string(data), "prompt") {
		t.Fatalf("private content persisted: %s", data)
	}
	store := loadOTelStore(filepath.Join(state, "otel-usage.json"))
	for _, day := range store.Days {
		if day.TotalTokens != 120 || day.CachedInputTokens != 40 {
			t.Fatal(day)
		}
	}
}
