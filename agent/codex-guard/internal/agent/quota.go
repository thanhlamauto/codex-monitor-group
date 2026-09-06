package agent

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const quotaTailBytes int64 = 4 << 20
const quotaRefreshInterval = 5 * time.Minute
const quotaFileLimit = 8

type RateLimitWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int64   `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

type QuotaSnapshot struct {
	PlanType   string           `json:"plan_type,omitempty"`
	Primary    *RateLimitWindow `json:"primary,omitempty"`
	Secondary  *RateLimitWindow `json:"secondary,omitempty"`
	ObservedAt string           `json:"observed_at"`
}

type quotaLogEntry struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type string `json:"type"`
		Info struct {
			RateLimits *struct {
				PlanType  string           `json:"plan_type"`
				Primary   *RateLimitWindow `json:"primary"`
				Secondary *RateLimitWindow `json:"secondary"`
			} `json:"rate_limits"`
		} `json:"info"`
	} `json:"payload"`
}

type quotaFile struct {
	path    string
	modTime time.Time
}

func quotaFiles(codexHome string) ([]quotaFile, error) {
	files := []quotaFile{}
	for _, directory := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(codexHome, directory)
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			files = append(files, quotaFile{path: path, modTime: info.ModTime()})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })
	if len(files) > quotaFileLimit {
		files = files[:quotaFileLimit]
	}
	return files, nil
}

func quotaFromFile(file quotaFile) (*QuotaSnapshot, time.Time, error) {
	handle, err := os.Open(file.path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return nil, time.Time{}, err
	}
	start := info.Size() - quotaTailBytes
	if start < 0 {
		start = 0
	}
	if _, err := handle.Seek(start, io.SeekStart); err != nil {
		return nil, time.Time{}, err
	}
	scanner := bufio.NewScanner(handle)
	scanner.Buffer(make([]byte, 64*1024), int(quotaTailBytes)+1)
	if start > 0 {
		scanner.Scan() // Discard the partial first line from the tail window.
	}
	var latest *QuotaSnapshot
	latestAt := time.Time{}
	for scanner.Scan() {
		var entry quotaLogEntry
		if json.Unmarshal(scanner.Bytes(), &entry) != nil || entry.Type != "event_msg" || entry.Payload.Type != "token_count" || entry.Payload.Info.RateLimits == nil {
			continue
		}
		rate := entry.Payload.Info.RateLimits
		if rate.Primary != nil && (rate.Primary.UsedPercent < 0 || rate.Primary.UsedPercent > 100 || rate.Primary.WindowMinutes < 1) {
			rate.Primary = nil
		}
		if rate.Secondary != nil && (rate.Secondary.UsedPercent < 0 || rate.Secondary.UsedPercent > 100 || rate.Secondary.WindowMinutes < 1) {
			rate.Secondary = nil
		}
		if rate.Primary == nil && rate.Secondary == nil {
			continue
		}
		observedAt, parseErr := time.Parse(time.RFC3339Nano, entry.Timestamp)
		if parseErr != nil {
			observedAt = file.modTime.UTC()
		}
		if latest == nil || observedAt.After(latestAt) {
			latestAt = observedAt
			latest = &QuotaSnapshot{PlanType: rate.PlanType, Primary: rate.Primary, Secondary: rate.Secondary, ObservedAt: observedAt.UTC().Format(time.RFC3339)}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, time.Time{}, err
	}
	return latest, latestAt, nil
}

// LatestQuota reads only the tail of Codex JSONL files and returns numeric rate-limit
// metadata. Prompt text, model output, and conversation content are never returned.
func LatestQuota(codexHome string) (*QuotaSnapshot, error) {
	files, err := quotaFiles(codexHome)
	if err != nil {
		return nil, err
	}
	var latest *QuotaSnapshot
	latestAt := time.Time{}
	for _, file := range files {
		snapshot, observedAt, err := quotaFromFile(file)
		if err != nil {
			return nil, err
		}
		if snapshot != nil && (latest == nil || observedAt.After(latestAt)) {
			latest, latestAt = snapshot, observedAt
		}
	}
	return latest, nil
}

func (a *Client) cachedQuota() *QuotaSnapshot {
	a.quotaMu.Lock()
	defer a.quotaMu.Unlock()
	if !a.quotaRead.IsZero() && time.Since(a.quotaRead) < quotaRefreshInterval {
		return a.quota
	}
	a.quotaRead = time.Now()
	quota, err := LatestQuota(a.Config.CodexHome)
	if err == nil && quota != nil {
		a.quota = quota
	}
	return a.quota
}
