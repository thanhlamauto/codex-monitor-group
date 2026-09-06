package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestCollectUsageNormalizesCCUsage(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ccusage")
	if runtime.GOOS == "windows" {
		script += ".exe"
	}
	data := `{"daily":[{"date":"2026-09-06","inputTokens":100,"cacheReadTokens":50,"outputTokens":20,"reasoningOutputTokens":5,"totalTokens":170}]}`
	source := "package main\nimport \"fmt\"\nfunc main(){fmt.Print(`" + data + "`)}\n"
	sourcePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourcePath, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-o", script, sourcePath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fake ccusage: %v: %s", err, output)
	}
	days, err := CollectUsage(script, filepath.Join(dir, "codex"), "UTC", time.Date(2026, 9, 6, 1, 0, 0, 0, time.UTC), 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].CachedInputTokens != 50 || days[0].TotalTokens != 170 {
		t.Fatalf("unexpected: %#v", days)
	}
}
