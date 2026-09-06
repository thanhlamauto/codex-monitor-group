package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func CodexVersion(path string) string {
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return "not-detected"
	}
	return strings.TrimSpace(string(out))
}
func CCUsageVersion(path string) string {
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		return "not-detected"
	}
	fields := strings.Fields(string(out))
	if len(fields) >= 2 {
		return fields[len(fields)-1]
	}
	return strings.TrimSpace(string(out))
}

func telemetryBlock(server, token string) string {
	endpoint := "http://127.0.0.1:9464/v1/logs"
	return fmt.Sprintf("[otel]\nenvironment = \"classroom\"\nlog_user_prompt = false\nexporter = { otlp-http = { endpoint = %q, protocol = \"json\", headers = { Authorization = %q } } }\n", endpoint, "Bearer "+token)
}

func ConfigureTelemetry(codexHome, server, token string) (string, error) {
	if err := os.MkdirAll(codexHome, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(codexHome, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if len(data) > 0 {
		_ = os.WriteFile(path+".codex-guard.bak", data, 0600)
	}
	lines := strings.Split(string(data), "\n")
	section := regexp.MustCompile(`^\s*\[([^]]+)\]\s*(?:#.*)?$`)
	kept := make([]string, 0, len(lines))
	inOtel := false
	for _, line := range lines {
		if match := section.FindStringSubmatch(line); match != nil {
			inOtel = match[1] == "otel" || strings.HasPrefix(match[1], "otel.")
		}
		if !inOtel {
			kept = append(kept, line)
		}
	}
	content := strings.TrimSpace(strings.Join(kept, "\n"))
	if content != "" {
		content += "\n\n"
	}
	content += telemetryBlock(server, token)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return TelemetryFingerprint(codexHome, server, token)
}

func TelemetryFingerprint(codexHome, server, token string) (string, error) {
	hash, _, err := TelemetryStatus(codexHome, server, token)
	return hash, err
}

func TelemetryStatus(codexHome, server, token string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return "missing", false, err
	}
	type exporter struct {
		Endpoint string            `toml:"endpoint"`
		Protocol string            `toml:"protocol"`
		Headers  map[string]string `toml:"headers"`
	}
	type document struct {
		OTel struct {
			LogUserPrompt *bool               `toml:"log_user_prompt"`
			Exporter      map[string]exporter `toml:"exporter"`
		} `toml:"otel"`
	}
	var parsed document
	if err := toml.Unmarshal(data, &parsed); err != nil {
		return "invalid", false, err
	}
	httpExporter := parsed.OTel.Exporter["otlp-http"]
	auth := httpExporter.Headers["Authorization"]
	promptDisabled := parsed.OTel.LogUserPrompt != nil && !*parsed.OTel.LogUserPrompt
	expectedEndpoint := "http://127.0.0.1:9464/v1/logs"
	valid := promptDisabled && httpExporter.Endpoint == expectedEndpoint && httpExporter.Protocol == "json" && auth == "Bearer "+token
	canonical := fmt.Sprintf("endpoint=%s\nprotocol=%s\nlog_user_prompt=%t\nauth_sha256=%x", httpExporter.Endpoint, httpExporter.Protocol, !promptDisabled, sha256.Sum256([]byte(auth)))
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), valid, nil
}
