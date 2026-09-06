package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureTelemetryPreservesUnrelatedConfig(t *testing.T) {
	home := t.TempDir()
	original := "model = \"gpt-5\"\n\n[otel]\nlog_user_prompt = true\nexporter = \"none\"\n\n[mcp_servers.demo]\ncommand = \"demo\"\n"
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := ConfigureTelemetry(home, "https://meter.example", "secret")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(home, "config.toml"))
	text := string(data)
	for _, wanted := range []string{"model = \"gpt-5\"", "[mcp_servers.demo]", "log_user_prompt = false", "http://127.0.0.1:9464/v1/logs"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("missing %q in %s", wanted, text)
		}
	}
	if strings.Contains(text, "log_user_prompt = true") {
		t.Fatal("unsafe prompt logging retained")
	}
}

func TestTelemetryFingerprintIgnoresUnrelatedSettings(t *testing.T) {
	home := t.TempDir()
	first, err := ConfigureTelemetry(home, "https://meter.example", "secret")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "config.toml")
	file, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = file.WriteString("\n[mcp_servers.unrelated]\ncommand = \"demo\"\n")
	_ = file.Close()
	second, valid, err := TelemetryStatus(home, "https://meter.example", "secret")
	if err != nil || !valid {
		t.Fatalf("valid=%v err=%v", valid, err)
	}
	if first != second {
		t.Fatalf("unrelated setting changed fingerprint: %s != %s", first, second)
	}
}
