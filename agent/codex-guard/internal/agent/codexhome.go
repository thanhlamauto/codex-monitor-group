package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// CodexHomeResolution describes the privacy-preserving result of local Codex
// home discovery. The path is never sent to the server; heartbeats continue to
// report only its opaque hash.
type CodexHomeResolution struct {
	Path      string
	Source    string
	LogCount  int
	LatestLog time.Time
}

type codexHomeCandidate struct {
	path   string
	source string
}

func splitCodexHomes(value string) []string {
	var paths []string
	for _, commaPart := range strings.Split(value, ",") {
		for _, path := range filepath.SplitList(commaPart) {
			if path = strings.TrimSpace(path); path != "" {
				paths = append(paths, path)
			}
		}
	}
	return paths
}

func platformCodexHomeCandidates() []codexHomeCandidate {
	var candidates []codexHomeCandidate
	if value := os.Getenv("CODEX_HOME"); value != "" {
		for _, path := range splitCodexHomes(value) {
			candidates = append(candidates, codexHomeCandidate{path: path, source: "CODEX_HOME"})
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, codexHomeCandidate{path: filepath.Join(home, ".codex"), source: "current-user"})
	}
	if runtime.GOOS == "windows" {
		if profile := os.Getenv("USERPROFILE"); profile != "" {
			candidates = append(candidates, codexHomeCandidate{path: filepath.Join(profile, ".codex"), source: "USERPROFILE"})
		}
		drive := os.Getenv("SystemDrive")
		if drive == "" {
			drive = `C:`
		}
		matches, _ := filepath.Glob(filepath.Join(drive+string(os.PathSeparator), "Users", "*", ".codex"))
		for _, path := range matches {
			candidates = append(candidates, codexHomeCandidate{path: path, source: "windows-profile"})
		}
	} else {
		for _, pattern := range []string{"/home/*/.codex", "/Users/*/.codex"} {
			matches, _ := filepath.Glob(pattern)
			for _, path := range matches {
				candidates = append(candidates, codexHomeCandidate{path: path, source: "local-profile"})
			}
		}
	}
	return candidates
}

func sessionLogSummary(codexHome string) (int, time.Time, error) {
	count := 0
	latest := time.Time{}
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
			count++
			if info.ModTime().After(latest) {
				latest = info.ModTime()
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return 0, time.Time{}, err
		}
	}
	return count, latest, nil
}

func inspectCodexHome(candidate codexHomeCandidate) (CodexHomeResolution, bool) {
	path, err := filepath.Abs(candidate.path)
	if err != nil {
		return CodexHomeResolution{}, false
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return CodexHomeResolution{}, false
	}
	logs, latest, err := sessionLogSummary(path)
	if err != nil {
		return CodexHomeResolution{}, false
	}
	_, configErr := os.Stat(filepath.Join(path, "config.toml"))
	if logs == 0 && configErr != nil {
		return CodexHomeResolution{}, false
	}
	return CodexHomeResolution{Path: path, Source: candidate.source, LogCount: logs, LatestLog: latest}, true
}

func pathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func resolveCodexHome(configured string, candidates []codexHomeCandidate) CodexHomeResolution {
	all := append([]codexHomeCandidate{{path: configured, source: "configured"}}, candidates...)
	seen := map[string]bool{}
	valid := make([]CodexHomeResolution, 0, len(all))
	for _, candidate := range all {
		if strings.TrimSpace(candidate.path) == "" {
			continue
		}
		key := pathKey(candidate.path)
		if seen[key] {
			continue
		}
		seen[key] = true
		if inspected, ok := inspectCodexHome(candidate); ok {
			valid = append(valid, inspected)
		}
	}
	if len(valid) == 0 {
		path, _ := filepath.Abs(configured)
		return CodexHomeResolution{Path: filepath.Clean(path), Source: "configured"}
	}

	// A configured home that already contains logs stays pinned. This prevents a
	// shared computer from silently switching between two real Codex users. We
	// only self-heal when the configured home has no session data.
	for _, candidate := range valid {
		if candidate.Source == "configured" && candidate.LogCount > 0 {
			return candidate
		}
	}
	sort.SliceStable(valid, func(i, j int) bool {
		if (valid[i].LogCount > 0) != (valid[j].LogCount > 0) {
			return valid[i].LogCount > 0
		}
		if !valid[i].LatestLog.Equal(valid[j].LatestLog) {
			return valid[i].LatestLog.After(valid[j].LatestLog)
		}
		return valid[i].Source == "configured"
	})
	return valid[0]
}

// ResolveCodexHome checks the configured location, CODEX_HOME, the current
// profile and local OS profiles. A wrong/empty configured location is replaced
// by the candidate that actually contains the newest Codex session logs.
func ResolveCodexHome(configured string) CodexHomeResolution {
	return resolveCodexHome(configured, platformCodexHomeCandidates())
}
