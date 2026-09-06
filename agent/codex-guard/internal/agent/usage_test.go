package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectUsageNormalizesCCUsage(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ccusage")
	data := `{"daily":[{"date":"2026-09-06","inputTokens":100,"cacheReadTokens":50,"outputTokens":20,"reasoningOutputTokens":5,"totalTokens":170}]}`
	content := "#!/bin/sh\nprintf '%s' '" + data + "'\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	days, err := CollectUsage(script, filepath.Join(dir, "codex"), "UTC", time.Date(2026, 9, 6, 1, 0, 0, 0, time.UTC), 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 1 || days[0].CachedInputTokens != 50 || days[0].TotalTokens != 170 {
		t.Fatalf("unexpected: %#v", days)
	}
}
