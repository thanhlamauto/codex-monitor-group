package agent

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type checkpoint struct {
	OpaqueFileID string `json:"opaque_file_id"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	PrefixSHA256 string `json:"prefix_sha256"`
	FullSHA256   string `json:"full_sha256,omitempty"`
	Archived     bool   `json:"archived"`
	MtimeUnix    int64  `json:"mtime_unix"`
}
type integrityState struct {
	Files map[string]checkpoint `json:"files"`
}
type Finding struct {
	EventType    string `json:"event_type"`
	Severity     string `json:"severity"`
	OpaqueFileID string `json:"opaque_file_id,omitempty"`
	OldSize      *int64 `json:"old_size,omitempty"`
	NewSize      *int64 `json:"new_size,omitempty"`
}
type FileMetadata struct {
	OpaqueFileID string `json:"opaque_file_id"`
	Size         int64  `json:"size"`
	PrefixSize   int64  `json:"prefix_size"`
	PrefixSHA256 string `json:"prefix_sha256"`
	FullSHA256   string `json:"full_sha256,omitempty"`
	Archived     bool   `json:"archived"`
	MtimeUnix    int64  `json:"mtime_unix"`
}
type IntegrityPayload struct {
	FilesChecked int            `json:"files_checked"`
	Status       string         `json:"status"`
	Findings     []Finding      `json:"findings"`
	FileMetadata []FileMetadata `json:"file_metadata"`
}

func hashPrefix(path string, size int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.CopyN(h, f, size)
	if err != nil && err != io.EOF {
		return "", err
	}
	if n != size {
		return "", fmt.Errorf("short read: expected %d got %d", size, n)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func opaqueID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }

func loadIntegrityState(path string) (integrityState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return integrityState{Files: map[string]checkpoint{}}, nil
	}
	if err != nil {
		return integrityState{}, err
	}
	var state integrityState
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	if state.Files == nil {
		state.Files = map[string]checkpoint{}
	}
	return state, nil
}
func saveIntegrityState(path string, state integrityState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func discoverLogs(codexHome string) (map[string]string, map[string]bool, error) {
	paths := map[string]string{}
	archived := map[string]bool{}
	for _, root := range []struct {
		name     string
		archived bool
	}{{"sessions", false}, {"archived_sessions", true}} {
		base := filepath.Join(codexHome, root.name)
		info, err := os.Stat(base)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, nil, err
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".jsonl") {
				return nil
			}
			relative, err := filepath.Rel(codexHome, path)
			if err != nil {
				return err
			}
			paths[relative] = path
			archived[relative] = root.archived
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return paths, archived, nil
}

func ScanIntegrity(codexHome, statePath string) (IntegrityPayload, error) {
	state, err := loadIntegrityState(statePath)
	if err != nil {
		return IntegrityPayload{}, err
	}
	paths, archivedPaths, err := discoverLogs(codexHome)
	if err != nil {
		return IntegrityPayload{}, err
	}
	findings := []Finding{}
	next := map[string]checkpoint{}
	used := map[string]bool{}
	keys := make([]string, 0, len(paths))
	for key := range paths {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	// First reconcile direct paths and verify append-only or archived immutability.
	for _, relative := range keys {
		path := paths[relative]
		info, err := os.Stat(path)
		if err != nil {
			return IntegrityPayload{}, err
		}
		size := info.Size()
		archived := archivedPaths[relative]
		previous, exists := state.Files[relative]
		if exists {
			used[relative] = true
			currentHash, hashErr := hashPrefix(path, size)
			if hashErr != nil {
				return IntegrityPayload{}, hashErr
			}
			cp := checkpoint{OpaqueFileID: previous.OpaqueFileID, RelativePath: relative, Size: size, PrefixSHA256: currentHash, Archived: archived, MtimeUnix: info.ModTime().Unix()}
			if archived {
				cp.FullSHA256 = currentHash
			}
			if previous.Archived {
				if size != previous.Size || currentHash != previous.FullSHA256 {
					old, current := previous.Size, size
					findings = append(findings, Finding{EventType: "ARCHIVED_LOG_MODIFIED", Severity: "CRITICAL", OpaqueFileID: previous.OpaqueFileID, OldSize: &old, NewSize: &current})
				}
			} else if size < previous.Size {
				old, current := previous.Size, size
				findings = append(findings, Finding{EventType: "LOG_TRUNCATED", Severity: "CRITICAL", OpaqueFileID: previous.OpaqueFileID, OldSize: &old, NewSize: &current})
			} else {
				prefix, prefixErr := hashPrefix(path, previous.Size)
				if prefixErr != nil {
					return IntegrityPayload{}, prefixErr
				}
				if prefix != previous.PrefixSHA256 {
					old, current := previous.Size, size
					findings = append(findings, Finding{EventType: "LOG_PREFIX_MODIFIED", Severity: "CRITICAL", OpaqueFileID: previous.OpaqueFileID, OldSize: &old, NewSize: &current})
				}
			}
			next[relative] = cp
			continue
		}
		// A newly seen archived file may be a valid move of a previously active path.
		var movedFrom string
		var moved checkpoint
		if archived {
			for oldRelative, old := range state.Files {
				if used[oldRelative] || old.Archived || size < old.Size {
					continue
				}
				prefix, _ := hashPrefix(path, old.Size)
				if prefix == old.PrefixSHA256 {
					movedFrom, moved = oldRelative, old
					break
				}
			}
		}
		full, hashErr := hashPrefix(path, size)
		if hashErr != nil {
			return IntegrityPayload{}, hashErr
		}
		id := opaqueID()
		if movedFrom != "" {
			used[movedFrom] = true
			id = moved.OpaqueFileID
		}
		cp := checkpoint{OpaqueFileID: id, RelativePath: relative, Size: size, PrefixSHA256: full, Archived: archived, MtimeUnix: info.ModTime().Unix()}
		if archived {
			cp.FullSHA256 = full
		}
		next[relative] = cp
	}
	for relative, previous := range state.Files {
		if used[relative] {
			continue
		}
		if _, still := paths[relative]; still {
			continue
		}
		old := previous.Size
		findings = append(findings, Finding{EventType: "LOG_DELETED", Severity: "CRITICAL", OpaqueFileID: previous.OpaqueFileID, OldSize: &old})
	}
	if err := saveIntegrityState(statePath, integrityState{Files: next}); err != nil {
		return IntegrityPayload{}, err
	}
	metadata := make([]FileMetadata, 0, len(next))
	for _, cp := range next {
		metadata = append(metadata, FileMetadata{OpaqueFileID: cp.OpaqueFileID, Size: cp.Size, PrefixSize: cp.Size, PrefixSHA256: cp.PrefixSHA256, FullSHA256: cp.FullSHA256, Archived: cp.Archived, MtimeUnix: cp.MtimeUnix})
	}
	sort.Slice(metadata, func(i, j int) bool { return metadata[i].OpaqueFileID < metadata[j].OpaqueFileID })
	status := "OK"
	if len(findings) > 0 {
		status = "TAMPER"
	}
	return IntegrityPayload{FilesChecked: len(next), Status: status, Findings: findings, FileMetadata: metadata}, nil
}

func (a *Client) Integrity() error {
	payload, err := ScanIntegrity(a.Config.CodexHome, filepath.Join(a.Config.StateDir, "integrity.json"))
	if err != nil {
		return err
	}
	if err := a.enqueue("/api/v1/integrity", payload); err != nil {
		return err
	}
	return a.Flush()
}
