package sessionstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
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
	s.Note(1, "claude", id, "", "", 2, false)
	s.Rename(1, "review")
	s.Flush()
	got, _ := Load(p)
	if got.Entries[0].Session != id || got.Entries[0].Title != "review" {
		t.Fatalf("%+v", got)
	}
	s.Note(1, "codex", "", "", "", 2, true)
	s.Flush()
	got, _ = Load(p)
	if got.Entries[0].Session != "" {
		t.Fatal("agent switch retained old identity")
	}
	s.Note(1, "codex", id, "", "", 2, true)
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
	s.Note(1, "claude", "01234567-89ab-cdef-0123-456789abcdef", "", "", 4, true)
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

// Review fixes for PR 76.

func waitFile(t *testing.T, p string, ok func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got, err := Load(p); err == nil && ok(got) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, err := Load(p)
	t.Fatalf("file never matched: %+v %v", got, err)
	return got
}

func TestAgentFolderWinsOverShellStartFolder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := "01234567-89ab-cdef-0123-456789abcdef"
	s.SetTabs([]Live{{ID: 1, Cwd: "/home"}, {ID: 2, Cwd: "/home"}}, 1)
	s.Note(1, "claude", id, "", "/home/proj", 0, true)
	s.Note(2, "codex", id, "", "", 0, true)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(p)
	if got.Entries[0].Cwd != "/home/proj" {
		t.Fatalf("agent folder lost: %+v", got.Entries[0])
	}
	if got.Entries[1].Cwd != "/home" {
		t.Fatalf("empty hook folder replaced shell folder: %+v", got.Entries[1])
	}
	// Once the agent is gone the shell's folder is saved again.
	s.Cancel(1)
	s.Flush()
	got, _ = Load(p)
	if got.Entries[0].Cwd != "/home" {
		t.Fatalf("stale agent folder kept: %+v", got.Entries[0])
	}
}

func TestFailedSaveIsRetriedOnClose(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	original := replace
	replace = func(string, string) error { return errors.New("probe replacement failure") }
	s.SetTabs([]Live{{ID: 1, Cwd: "/latest"}}, 1)
	if err := s.Flush(); err == nil {
		replace = original
		s.Close()
		t.Fatal("expected flush failure")
	}
	replace = original
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil || got.Entries[0].Cwd != "/latest" {
		t.Fatalf("final save was not retried: %+v %v", got, err)
	}
}

func TestFailedBackgroundSaveIsRetried(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	var mu sync.Mutex
	fails := 1
	original := replace
	replace = func(a, b string) error {
		mu.Lock()
		defer mu.Unlock()
		if fails > 0 {
			fails--
			return errors.New("probe replacement failure")
		}
		return original(a, b)
	}
	defer func() { replace = original }()
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetTabs([]Live{{ID: 1, Cwd: "/latest"}}, 1)
	waitFile(t, p, func(got Snapshot) bool { return len(got.Entries) == 1 && got.Entries[0].Cwd == "/latest" })
}

func TestSaveRetriesABrieflyBlockedReplace(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	calls := 0
	original := replace
	replace = func(a, b string) error {
		calls++
		if calls < 3 {
			return errors.New("probe sharing violation")
		}
		return original(a, b)
	}
	defer func() { replace = original }()
	if err := Save(p, Snapshot{Version: 1, Entries: []Entry{{Cwd: "/x"}}}); err != nil {
		t.Fatalf("transient replace failure not retried: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls %d", calls)
	}
}

func TestOneBadFieldDoesNotBlockOtherSessions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := "01234567-89ab-cdef-0123-456789abcdef"
	s.SetTabs([]Live{{ID: 1, Cwd: "/ok"}, {ID: 2, Cwd: "/bad\u0085dir"}, {ID: 3, Cwd: "/agent"}}, 3)
	s.Rename(1, strings.Repeat("é", 3000)+"\u0085tail")
	s.Note(3, "claude", id, "/t\u0085.jsonl", "", 0, true)
	if err := s.Flush(); err != nil {
		t.Fatalf("one bad field blocked saving: %v", err)
	}
	got, err := Load(p)
	if err != nil || len(got.Entries) != 3 {
		t.Fatalf("%+v %v", got, err)
	}
	title := got.Entries[0].Title
	if len(title) > 4096 || !utf8.ValidString(title) || strings.ContainsRune(title, '\u0085') || title == "" {
		t.Fatalf("title not cleaned: %d bytes", len(title))
	}
	if got.Entries[1].Cwd != "" {
		t.Fatalf("invalid folder kept: %q", got.Entries[1].Cwd)
	}
	if got.Entries[2].Session != "" || got.Entries[2].Agent != "" || got.Entries[2].Transcript != "" || got.Active != 2 {
		t.Fatalf("invalid conversation file kept: %+v", got)
	}
}

func TestOversizedSessionSetStillSaves(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	big := strings.Repeat("w", 4000)
	var live []Live
	for i := 1; i <= MaxEntries; i++ {
		live = append(live, Live{ID: i, Cwd: "/" + big, FixedTitle: big, Workspace: big, WorkspaceName: big})
	}
	s.SetTabs(live, 1)
	if err := s.Flush(); err != nil {
		t.Fatalf("oversized set blocked saving: %v", err)
	}
	got, err := Load(p)
	if err != nil || len(got.Entries) == 0 || got.Active != 0 {
		t.Fatalf("%d %v", len(got.Entries), err)
	}
}

func TestEventsWhileFrozenApplyWhenThawed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	id := "01234567-89ab-cdef-0123-456789abcdef"
	s.SetTabs([]Live{{ID: 1, Cwd: "/a"}}, 1)
	s.Freeze(true)
	s.SetTabs([]Live{{ID: 1, Cwd: "/a"}, {ID: 2, Cwd: "/b"}}, 2)
	s.Note(2, "claude", id, "", "/b", 0, true)
	s.Rename(2, "late")
	s.Freeze(false)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(p)
	if len(got.Entries) != 2 || got.Entries[1].Session != id || got.Entries[1].Title != "late" {
		t.Fatalf("events during freeze were lost: %+v", got)
	}
}

func TestEventsWhileFrozenAreDroppedOnClose(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s.SetTabs([]Live{{ID: 1, Cwd: "/a"}}, 1)
	s.Flush()
	s.Freeze(true)
	s.SetTabs(nil, 0) // tabs closing at exit
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	got, _ := Load(p)
	if len(got.Entries) != 1 {
		t.Fatalf("shutdown closes were saved: %+v", got)
	}
}

func TestPendingAfterCloseChangesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s.Pending(1, Entry{Agent: "claude", Session: "01234567-89ab-cdef-0123-456789abcdef"}, 0)
	if len(s.agents) != 0 || s.dirty {
		t.Fatal("pending recorded after close")
	}
}

func TestSaveSyncsTheFolderAfterReplacing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sessions.json")
	var synced string
	original := syncDir
	syncDir = func(d string) error { synced = d; return nil }
	defer func() { syncDir = original }()
	if err := Save(p, Snapshot{Version: 1}); err != nil {
		t.Fatal(err)
	}
	if synced != dir {
		t.Fatalf("synced %q", synced)
	}
}

func TestCloseKeepingOwnershipSavesButHoldsTheLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sessions.json")
	s, err := Open(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.lock.Close() })
	s.SetTabs([]Live{{ID: 1, Cwd: "/kept"}}, 1)
	if err := s.CloseKeepingOwnership(); err != nil {
		t.Fatal(err)
	}
	if got, err := Load(p); err != nil || got.Entries[0].Cwd != "/kept" {
		t.Fatalf("%+v %v", got, err)
	}
	if other, err := Open(p); !errors.Is(err, ErrOwned) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("ownership released early: %v", err)
	}
}
