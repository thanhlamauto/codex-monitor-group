package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type UsageDay struct {
	Date                  string `json:"date"`
	InputTokens           uint64 `json:"inputTokens"`
	CacheReadTokens       uint64 `json:"cacheReadTokens"`
	OutputTokens          uint64 `json:"outputTokens"`
	ReasoningOutputTokens uint64 `json:"reasoningOutputTokens"`
	TotalTokens           uint64 `json:"totalTokens"`
}

type ccusageOutput struct {
	Daily []UsageDay `json:"daily"`
}

type normalizedUsage struct {
	Date                  string `json:"date"`
	InputTokens           uint64 `json:"input_tokens"`
	CachedInputTokens     uint64 `json:"cached_input_tokens"`
	OutputTokens          uint64 `json:"output_tokens"`
	ReasoningOutputTokens uint64 `json:"reasoning_output_tokens"`
	TotalTokens           uint64 `json:"total_tokens"`
}

func CollectUsage(ccusagePath, codexHome, timezone string, now time.Time, retentionDays int) ([]normalizedUsage, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("timezone %q: %w", timezone, err)
	}
	until := now.In(location).Format("2006-01-02")
	since := now.In(location).AddDate(0, 0, -retentionDays).Format("2006-01-02")
	cmd := exec.Command(ccusagePath, "codex", "daily", "--json", "--no-cost", "--offline", "--timezone", timezone, "--since", since, "--until", until)
	cmd.Env = append(os.Environ(), "CODEX_HOME="+codexHome, "NO_COLOR=1")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ccusage: %w", err)
	}
	var parsed ccusageOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("parse ccusage JSON: %w", err)
	}
	days := make([]normalizedUsage, 0, len(parsed.Daily))
	for _, day := range parsed.Daily {
		days = append(days, normalizedUsage{Date: day.Date, InputTokens: day.InputTokens, CachedInputTokens: day.CacheReadTokens, OutputTokens: day.OutputTokens, ReasoningOutputTokens: day.ReasoningOutputTokens, TotalTokens: day.TotalTokens})
	}
	return days, nil
}

func (a *Client) Usage() error {
	if err := a.refreshCodexHome(); err != nil {
		return err
	}
	days, err := CollectUsage(a.Config.CCUsagePath, a.Config.CodexHome, a.Config.Timezone, time.Now(), a.Config.RetentionDays)
	if err != nil {
		return err
	}
	if err := a.enqueue("/api/v1/usage", map[string]any{"source": "local", "days": days}); err != nil {
		return err
	}
	return a.Flush()
}
