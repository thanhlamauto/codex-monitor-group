package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/config"
	queuepkg "github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/queue"
)

type flakyTransport struct {
	fail     bool
	received int
}

func (f *flakyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if f.fail {
		return nil, io.ErrUnexpectedEOF
	}
	f.received++
	return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(strings.NewReader(`{"accepted":true}`)), Header: make(http.Header), Request: request}, nil
}

func TestOfflineQueueReplaysWhenConnectionReturns(t *testing.T) {
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	binary := filepath.Join(dir, "agent")
	_ = os.WriteFile(binary, []byte("binary"), 0700)
	c := &config.Config{ServerURL: "https://meter.invalid", DeviceID: "00000000-0000-0000-0000-000000000001", PrivateKey: base64.StdEncoding.EncodeToString(private), StateDir: dir, AgentPath: binary, CCUsagePath: binary, CodexHome: dir, NextSequence: 1, RetentionDays: 30}
	c.Defaults()
	if err := config.Save(configPath, c); err != nil {
		t.Fatal(err)
	}
	transport := &flakyTransport{fail: true}
	a := &Client{ConfigPath: configPath, Config: c, HTTP: &http.Client{Transport: transport, Timeout: time.Second}, Queue: queuepkg.New(filepath.Join(dir, "queue.json"), 30), started: time.Now()}
	if err := a.enqueue("/api/v1/heartbeat", map[string]any{"test": true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err == nil {
		t.Fatal("expected offline error")
	}
	if a.Queue.Len() != 1 {
		t.Fatal("event was lost offline")
	}
	transport.fail = false
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	if a.Queue.Len() != 0 || transport.received != 1 {
		t.Fatalf("queue=%d received=%d", a.Queue.Len(), transport.received)
	}
}

func TestUnknownDeviceReEnrollsAndDropsOnlyStaleQueue(t *testing.T) {
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	const oldDeviceID = "00000000-0000-0000-0000-000000000001"
	const newDeviceID = "00000000-0000-0000-0000-000000000002"
	registerCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/register":
			registerCalls++
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode registration: %v", err)
			}
			if body["name"] != "Student" || body["device_label"] != "Laptop" || body["public_key"] != base64.StdEncoding.EncodeToString(public) {
				t.Errorf("unexpected registration: %#v", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"device_id":"`+newDeviceID+`","student_name":"Student","device_label":"Laptop","otlp_token":"new-token","heartbeat_seconds":60,"usage_seconds":300,"integrity_seconds":300,"classroom_timezone":"Asia/Ho_Chi_Minh"}`)
		case "/api/v1/heartbeat":
			var envelope struct {
				DeviceID string `json:"device_id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&envelope); err != nil {
				t.Errorf("decode heartbeat: %v", err)
			}
			if envelope.DeviceID == oldDeviceID {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"detail":"unknown device"}`)
				return
			}
			if envelope.DeviceID != newDeviceID {
				t.Errorf("unexpected device id: %s", envelope.DeviceID)
			}
			_, _ = io.WriteString(w, `{"accepted":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	binary := filepath.Join(dir, "agent")
	if err := os.WriteFile(binary, []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	c := &config.Config{
		ServerURL: server.URL, DeviceID: oldDeviceID, StudentName: "Student", DeviceLabel: "Laptop",
		PrivateKey: base64.StdEncoding.EncodeToString(private), PublicKey: base64.StdEncoding.EncodeToString(public),
		OTLPToken: "old-token", StateDir: dir, AgentPath: binary, CCUsagePath: binary, CodexHome: dir,
		NextSequence: 1, RetentionDays: 30,
	}
	c.Defaults()
	if err := config.Save(configPath, c); err != nil {
		t.Fatal(err)
	}
	a := &Client{ConfigPath: configPath, Config: c, HTTP: server.Client(), Queue: queuepkg.New(filepath.Join(dir, "queue.json"), 30), started: time.Now()}
	if err := a.enqueue("/api/v1/heartbeat", map[string]any{"test": true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	if registerCalls != 1 || a.Config.DeviceID != newDeviceID || a.Config.OTLPToken != "new-token" || a.Config.NextSequence != 1 || a.Queue.Len() != 0 {
		t.Fatalf("calls=%d device=%s token=%s sequence=%d queue=%d", registerCalls, a.Config.DeviceID, a.Config.OTLPToken, a.Config.NextSequence, a.Queue.Len())
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DeviceID != newDeviceID || loaded.OTLPToken != "new-token" {
		t.Fatalf("re-enrollment was not persisted: %#v", loaded)
	}
	_, telemetryOK, err := TelemetryStatus(dir, server.URL, "new-token")
	if err != nil || !telemetryOK {
		t.Fatalf("telemetry was not updated: ok=%v err=%v", telemetryOK, err)
	}
	if err := a.enqueue("/api/v1/heartbeat", map[string]any{"test": true}); err != nil {
		t.Fatal(err)
	}
	if err := a.Flush(); err != nil {
		t.Fatal(err)
	}
	if registerCalls != 1 || a.Queue.Len() != 0 {
		t.Fatalf("new registration did not recover: calls=%d queue=%d", registerCalls, a.Queue.Len())
	}
}
