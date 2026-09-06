package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func setupLog(t *testing.T) (string, string, string) {
	t.Helper()
	home := t.TempDir()
	session := filepath.Join(home, "sessions", "2026", "test.jsonl")
	if err := os.MkdirAll(filepath.Dir(session), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(session, []byte("one\ntwo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "integrity.json")
	first, err := ScanIntegrity(home, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Findings) != 0 {
		t.Fatal(first.Findings)
	}
	return home, state, session
}
func finding(payload IntegrityPayload, kind string) bool {
	for _, f := range payload.Findings {
		if f.EventType == kind {
			return true
		}
	}
	return false
}
func TestAppendValid(t *testing.T) {
	home, state, file := setupLog(t)
	f, _ := os.OpenFile(file, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("three\n")
	_ = f.Close()
	p, err := ScanIntegrity(home, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Findings) != 0 {
		t.Fatal(p.Findings)
	}
}

func TestIntegrityCheckpointChainAdvances(t *testing.T) {
	home, state, _ := setupLog(t)
	first, err := ScanIntegrity(home, state)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ScanIntegrity(home, state)
	if err != nil {
		t.Fatal(err)
	}
	if first.StateID != second.StateID || second.ScanSequence != first.ScanSequence+1 {
		t.Fatalf("chain did not advance: %#v %#v", first, second)
	}
	if second.PreviousSnapshotHash != first.SnapshotHash || len(second.SnapshotHash) != 64 {
		t.Fatal("snapshot hash chain is broken")
	}
}

func TestIntegrityStateDeletionCreatesNewIdentity(t *testing.T) {
	home, state, _ := setupLog(t)
	before, err := ScanIntegrity(home, state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(state); err != nil {
		t.Fatal(err)
	}
	after, err := ScanIntegrity(home, state)
	if err != nil {
		t.Fatal(err)
	}
	if before.StateID == after.StateID || after.ScanSequence != 1 {
		t.Fatal("state reset was not made remotely detectable")
	}
}
func TestPrefixModified(t *testing.T) {
	home, state, file := setupLog(t)
	if err := os.WriteFile(file, []byte("XXX\ntwo\nmore\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, _ := ScanIntegrity(home, state)
	if !finding(p, "LOG_PREFIX_MODIFIED") {
		t.Fatal(p.Findings)
	}
}
func TestTruncated(t *testing.T) {
	home, state, file := setupLog(t)
	if err := os.WriteFile(file, []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	p, _ := ScanIntegrity(home, state)
	if !finding(p, "LOG_TRUNCATED") {
		t.Fatal(p.Findings)
	}
}
func TestDeleted(t *testing.T) {
	home, state, file := setupLog(t)
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	p, _ := ScanIntegrity(home, state)
	if !finding(p, "LOG_DELETED") {
		t.Fatal(p.Findings)
	}
}
func TestArchiveMoveValid(t *testing.T) {
	home, state, file := setupLog(t)
	archive := filepath.Join(home, "archived_sessions", "test.jsonl")
	if err := os.MkdirAll(filepath.Dir(archive), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file, archive); err != nil {
		t.Fatal(err)
	}
	p, err := ScanIntegrity(home, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Findings) != 0 {
		t.Fatal(p.Findings)
	}
	if !p.FileMetadata[0].Archived {
		t.Fatal("not archived")
	}
}
func TestArchivedModified(t *testing.T) {
	home, state, file := setupLog(t)
	archive := filepath.Join(home, "archived_sessions", "test.jsonl")
	_ = os.MkdirAll(filepath.Dir(archive), 0700)
	_ = os.Rename(file, archive)
	_, _ = ScanIntegrity(home, state)
	f, _ := os.OpenFile(archive, os.O_APPEND|os.O_WRONLY, 0600)
	_, _ = f.WriteString("changed\n")
	_ = f.Close()
	p, _ := ScanIntegrity(home, state)
	if !finding(p, "ARCHIVED_LOG_MODIFIED") {
		t.Fatal(p.Findings)
	}
}
