package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLatestQuotaExtractsOnlyRateLimitMetadata(t *testing.T) {
	home := t.TempDir()
	directory := filepath.Join(home, "sessions", "2026", "09", "06")
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	log := `{"timestamp":"2026-09-06T01:00:00Z","type":"event_msg","payload":{"type":"message","prompt":"must never be returned"}}` + "\n" +
		`{"timestamp":"2026-09-06T02:00:00Z","type":"event_msg","payload":{"type":"token_count","info":{"rate_limits":{"primary":{"used_percent":23.5,"window_minutes":300,"resets_at":1788667200},"secondary":{"used_percent":61,"window_minutes":10080,"resets_at":1789272000},"credits":{"balance":"private"},"plan_type":"pro"}}}}` + "\n"
	if err := os.WriteFile(filepath.Join(directory, "rollout.jsonl"), []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	quota, err := LatestQuota(home)
	if err != nil {
		t.Fatal(err)
	}
	if quota == nil || quota.PlanType != "pro" || quota.Primary == nil || quota.Secondary == nil {
		t.Fatalf("unexpected quota: %#v", quota)
	}
	if quota.Primary.UsedPercent != 23.5 || quota.Primary.WindowMinutes != 300 {
		t.Fatalf("unexpected primary window: %#v", quota.Primary)
	}
	data, _ := json.Marshal(quota)
	if strings.Contains(string(data), "prompt") || strings.Contains(string(data), "balance") {
		t.Fatalf("private fields leaked: %s", data)
	}
}

func TestLatestQuotaReturnsNilWhenCodexHasNotReportedLimits(t *testing.T) {
	quota, err := LatestQuota(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if quota != nil {
		t.Fatalf("expected no quota, got %#v", quota)
	}
}
