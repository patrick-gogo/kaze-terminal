package sessionstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSnapshotRoundTripAndLimits(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s := Snapshot{Version: 1, Active: 0, Entries: []Entry{{Cwd: t.TempDir(), Title: "日本語", Agent: "codex", Session: "01234567-89ab-cdef-0123-456789abcdef"}}}
	if err := Save(p, s); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got.Entries[0] != s.Entries[0] {
		t.Fatalf("%+v %v", got, err)
	}
	s.Entries[0].Title = strings.Repeat("x", 5000)
	if Save(p, s) == nil {
		t.Fatal("accepted oversized string")
	}
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), strings.Repeat("x", 5000)) {
		t.Fatal("failed write replaced snapshot")
	}
	os.WriteFile(p, []byte(`{"version":99}`), 0600)
	if _, err := Load(p); err == nil {
		t.Fatal("accepted future version")
	}
	os.WriteFile(p, []byte(strings.Repeat(" ", MaxBytes+1)), 0600)
	if _, err := Load(p); err == nil {
		t.Fatal("accepted oversized file")
	}
}
func TestOwnershipAndCoalescedClose(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(p); err == nil {
		other.Close()
		t.Fatal("two owners")
	}
	s.SetTabs([]Live{{ID: 1, Cwd: "/first"}}, 1)
	s.SetTabs([]Live{{ID: 1, Cwd: "/last"}}, 1)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got.Entries[0].Cwd != "/last" {
		t.Fatalf("%+v %v", got, err)
	}
	s, err = Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetTabs(nil, 0)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	got, _ = Load(p)
	if len(got.Entries) != 0 {
		t.Fatal("closed tab resurrected")
	}
}
func TestOriginIdentityLifecycle(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := "01234567-89ab-cdef-0123-456789abcdef"
	s.SetTabs([]Live{{ID: 1, Cwd: "/repo", Seq: 2}}, 1)
	s.Note(1, "claude", id, "", 2, false)
	s.Rename(1, "review")
	s.Flush()
	got, _ := Load(p)
	if got.Entries[0].Session != id || got.Entries[0].Title != "review" {
		t.Fatalf("%+v", got)
	}
	s.Note(1, "codex", "", "", 2, true)
	s.Flush()
	got, _ = Load(p)
	if got.Entries[0].Session != "" {
		t.Fatal("agent switch retained old identity")
	}
	s.Note(1, "codex", id, "", 2, true)
	s.SetTabs([]Live{{ID: 1, Cwd: "/other", Seq: 3}}, 1)
	s.Flush()
	got, _ = Load(p)
	if got.Entries[0].Agent != "" {
		t.Fatal("shell return retained agent")
	}
	s.SetEnabled(false)
	s.Flush()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("disabled state retained")
	}
	s.SetEnabled(true)
	s.Flush()
	if _, err := Load(p); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
}

func TestAtomicReplaceFailurePreservesLastSnapshot(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	old := Snapshot{Version: 1, Entries: []Entry{{Cwd: "/old"}}}
	if err := Save(p, old); err != nil {
		t.Fatal(err)
	}
	original := replace
	replace = func(string, string) error { return errors.New("probe replacement failure") }
	defer func() { replace = original }()
	if err := Save(p, Snapshot{Version: 1, Entries: []Entry{{Cwd: "/new"}}}); err == nil {
		t.Fatal("expected replace failure")
	}
	got, err := Load(p)
	if err != nil || got.Entries[0].Cwd != "/old" {
		t.Fatalf("%+v %v", got, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(p), ".sessions-*"))
	if len(leftovers) != 0 {
		t.Fatal(leftovers)
	}
}
func TestCrashRecoverySavesWithoutFlushAndRejectsStaleIdentity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetTabs([]Live{{ID: 1, Cwd: "/new", Seq: 5}}, 1)
	s.Note(1, "claude", "01234567-89ab-cdef-0123-456789abcdef", "", 4, true)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := Load(p); err == nil {
			if got.Entries[0].Agent != "" {
				t.Fatal("stale event resurrected agent")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no coalesced save without flush")
}

func TestPendingBootstrapCannotOverwriteSavedSessions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	entry := Entry{Cwd: "/original", Agent: "claude", Session: "01234567-89ab-cdef-0123-456789abcdef"}
	Save(p, Snapshot{Version: 1, Entries: []Entry{entry}})
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.Freeze(true)
	s.Pending(1, entry, 0)
	time.Sleep(300 * time.Millisecond)
	got, err := Load(p)
	if err != nil || len(got.Entries) != 1 {
		t.Fatalf("bootstrap overwrote previous snapshot: %+v %v", got, err)
	}
}
