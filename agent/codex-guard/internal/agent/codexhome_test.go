package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thanhlamauto/codex-monitor-group/agent/codex-guard/internal/config"
)

func writeSessionLog(t *testing.T, home, name string, modified time.Time) string {
	t.Helper()
	path := filepath.Join(home, "sessions", "2026", "09", name+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestResolveCodexHomeReplacesEmptyConfiguredHome(t *testing.T) {
	configured := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(configured, 0700); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(t.TempDir(), ".codex")
	writeSessionLog(t, actual, "rollout", time.Now())

	got := resolveCodexHome(configured, []codexHomeCandidate{{path: actual, source: "windows-profile"}})
	if got.Path != actual || got.Source != "windows-profile" || got.LogCount != 1 {
		t.Fatalf("unexpected resolution: %#v", got)
	}
}

func TestResolveCodexHomeKeepsConfiguredHomeWithLogs(t *testing.T) {
	configured := filepath.Join(t.TempDir(), ".codex")
	writeSessionLog(t, configured, "configured", time.Now().Add(-time.Hour))
	other := filepath.Join(t.TempDir(), ".codex")
	writeSessionLog(t, other, "other", time.Now())

	got := resolveCodexHome(configured, []codexHomeCandidate{{path: other, source: "windows-profile"}})
	if got.Path != configured || got.Source != "configured" {
		t.Fatalf("configured home should stay pinned: %#v", got)
	}
}

func TestSessionLogSummaryFindsNestedAndArchivedLogs(t *testing.T) {
	home := t.TempDir()
	writeSessionLog(t, home, "active", time.Now())
	archive := filepath.Join(home, "archived_sessions", "old.jsonl")
	if err := os.MkdirAll(filepath.Dir(archive), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archive, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	count, _, err := sessionLogSummary(home)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 logs, got %d", count)
	}
}

func TestRefreshCodexHomePersistsDiscoveredPathAndTelemetry(t *testing.T) {
	configured := filepath.Join(t.TempDir(), ".codex")
	if err := os.MkdirAll(configured, 0700); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(t.TempDir(), ".codex")
	writeSessionLog(t, actual, "rollout", time.Now().Add(24*time.Hour))
	t.Setenv("CODEX_HOME", actual)
	configPath := filepath.Join(t.TempDir(), "config.json")
	c := &config.Config{
		ServerURL:   "https://meter.example",
		DeviceID:    "device",
		PrivateKey:  "private",
		OTLPToken:   "otel-token",
		CodexHome:   configured,
		StateDir:    t.TempDir(),
		CCUsagePath: "ccusage",
		AgentPath:   "codex-guard",
	}
	c.Defaults()
	if err := config.Save(configPath, c); err != nil {
		t.Fatal(err)
	}
	a := &Client{ConfigPath: configPath, Config: c}
	if err := a.refreshCodexHome(); err != nil {
		t.Fatal(err)
	}
	if a.Config.CodexHome != actual || !a.homeAuto {
		t.Fatalf("home was not recovered: %#v", a.Config)
	}
	loaded, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CodexHome != actual {
		t.Fatalf("discovered home was not persisted: %s", loaded.CodexHome)
	}
	if _, valid, err := TelemetryStatus(actual, loaded.ServerURL, loaded.OTLPToken); err != nil || !valid {
		t.Fatalf("telemetry was not configured in discovered home: valid=%v err=%v", valid, err)
	}
}
