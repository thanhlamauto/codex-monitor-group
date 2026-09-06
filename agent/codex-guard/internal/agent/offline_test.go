package agent

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/example/codex-classroom-monitor/agent/codex-guard/internal/config"
	queuepkg "github.com/example/codex-classroom-monitor/agent/codex-guard/internal/queue"
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
