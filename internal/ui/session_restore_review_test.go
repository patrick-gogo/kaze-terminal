package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/sessionstate"
	"github.com/patrick-gogo/kaze-terminal/internal/update"
)

const reviewID = "01234567-89ab-cdef-0123-456789abcdef"

func loadSaved(t *testing.T, f *runFixture) sessionstate.Snapshot {
	t.Helper()
	if err := f.win.restore.Flush(); err != nil {
		t.Fatal(err)
	}
	got, err := sessionstate.Load(f.win.restorePath)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// A shell without OSC 7 reports no folder; the hook's folder is where the agent runs.
func TestRestoreSavesTheAgentsFolderNotTheTabsStartFolder(t *testing.T) {
	f, _ := restoreFixture(t)
	f.win.opt.Dir = f.dirs[0]
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	f.win.liveMgr.Store(f.win.mgr)
	main := f.win.mgr.Active()
	f.win.onNotify(notify.Msg{Tab: int(main.ID), Kind: notify.KindSession, Session: reviewID, Agent: "claude", Cwd: f.dirs[1]})
	got := loadSaved(t, f)
	if got.Entries[0].Cwd != f.dirs[1] || got.Entries[0].Session != reviewID {
		t.Fatalf("%+v", got.Entries[0])
	}
}

func TestAgentQuittingInFrontClearsItsSavedConversation(t *testing.T) {
	f, _ := restoreFixture(t)
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	f.win.liveMgr.Store(f.win.mgr)
	p := newProbe()
	p.install(f.win)
	tb := f.win.mgr.Active()
	f.win.onNotify(notify.Msg{Tab: int(tb.ID), Kind: notify.KindSession, Session: reviewID, Agent: "claude"})
	now := time.Unix(100, 0)
	p.fg[tb.ID], p.names[7], p.out[tb.ID] = 7, "claude", 1
	f.win.detectAgents(now)
	if got := loadSaved(t, f); got.Entries[0].Session != reviewID {
		t.Fatal("running agent was cleared")
	}
	p.fg[tb.ID], p.out[tb.ID] = 3, 2 // the shell is back in front
	f.win.detectAgents(now.Add(2 * time.Second))
	if got := loadSaved(t, f); got.Entries[0].Session != "" || got.Entries[0].Agent != "" {
		t.Fatalf("finished conversation still offered: %+v", got.Entries[0])
	}
}

func TestRestoreWarningsStayInTheirOwnTab(t *testing.T) {
	f, p := restoreFixture(t)
	gone := filepath.Join(t.TempDir(), "gone")
	sessionstate.Save(p, sessionstate.Snapshot{Version: 1, Entries: []sessionstate.Entry{{Cwd: gone}, {Cwd: f.dirs[0]}, {Cwd: f.dirs[1]}}})
	f.win.opt.Dir = f.dirs[2]
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	list := f.win.mgr.Tabs()
	if len(list) != 3 {
		t.Fatalf("%d tabs", len(list))
	}
	if !strings.Contains(firstRows(f), "folder unavailable") {
		t.Fatal("warning missing from its tab")
	}
	for _, tb := range list[1:] {
		for r := range 4 {
			if strings.Contains(visibleRow(tb.Session, r), "folder unavailable") {
				t.Fatalf("tab %d repeats another tab's warning", tb.ID)
			}
		}
	}
}

func TestShutdownWithHungShellsKeepsOwnershipUntilExit(t *testing.T) {
	f, p := restoreFixture(t)
	f.win.initRestore()
	f.win.openStartup(nil)
	f.win.finishRestore(false)
	if other, err := sessionstate.Open(p); !errors.Is(err, sessionstate.ErrOwned) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("lock released while shells may still run: %v", err)
	}
	if _, err := sessionstate.Load(p); err != nil {
		t.Fatalf("not saved: %v", err)
	}
}

func holdLock(t *testing.T, p string) *sessionstate.Store {
	t.Helper()
	s, err := sessionstate.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestUpdateHandoffWaitsForTheOldOwner(t *testing.T) {
	f, p := restoreFixture(t)
	old := holdLock(t, p)
	go func() { time.Sleep(200 * time.Millisecond); old.Close() }()
	f.win.restoreWait = true
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	if f.win.restore == nil || f.win.restoreFatal != nil {
		t.Fatalf("handoff failed: %v", f.win.restoreFatal)
	}
}

func TestUpdateHandoffTimeoutIsAnActionableError(t *testing.T) {
	f, p := restoreFixture(t)
	old := holdLock(t, p)
	t.Cleanup(func() { old.Close() })
	f.win.restoreWait = true
	f.win.restoreWaitFor = 150 * time.Millisecond
	start := time.Now()
	f.win.initRestore()
	if time.Since(start) > 2*time.Second {
		t.Fatal("wait not bounded")
	}
	if f.win.restoreFatal == nil || !strings.Contains(f.win.restoreFatal.Error(), "close the other Kaze window") {
		t.Fatalf("not actionable: %v", f.win.restoreFatal)
	}
}

func TestPlainLaunchDoesNotWaitForAnotherOwner(t *testing.T) {
	f, p := restoreFixture(t)
	old := holdLock(t, p)
	t.Cleanup(func() { old.Close() })
	start := time.Now()
	f.win.initRestore()
	if time.Since(start) > time.Second || f.win.restoreFatal != nil || f.win.restore != nil {
		t.Fatal("plain launch waited or failed")
	}
	if len(f.win.opt.Warnings) == 0 {
		t.Fatal("no warning")
	}
}

func TestUpdateHandoffOtherErrorsOpenWithAWarning(t *testing.T) {
	f, _ := restoreFixture(t)
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0600)
	f.win.restorePath = filepath.Join(blocker, "sessions.json") // its folder is a file
	f.win.restoreWait = true
	f.win.initRestore()
	if f.win.restoreFatal != nil {
		t.Fatalf("non-ownership error stopped startup: %v", f.win.restoreFatal)
	}
	if len(f.win.opt.Warnings) == 0 {
		t.Fatal("no warning")
	}
}

func TestUnreadableSavedSessionsAreBackedUp(t *testing.T) {
	f, p := restoreFixture(t)
	bad := []byte(`{"version":99}`)
	os.WriteFile(p, bad, 0600)
	f.win.initRestore()
	f.win.openStartup(nil)
	f.win.closeRestore()
	b, err := os.ReadFile(p + ".bak")
	if err != nil || string(b) != string(bad) {
		t.Fatalf("unreadable file not kept: %q %v", b, err)
	}
	if !strings.Contains(strings.Join(f.win.opt.Warnings, "\n"), ".bak") {
		t.Fatal("warning does not say where the copy is")
	}
}

func TestFailedRestartKeepsEventsFromTheHandoff(t *testing.T) {
	rel := newFakeReleases(t, "v99.0", "new", "")
	win, _, _, _, _ := updWindow(t, rel)
	win.restorePath = filepath.Join(t.TempDir(), "sessions.json")
	win.opt.Config.RestoreSessions = true
	win.initRestore()
	win.trackRestore()
	t.Cleanup(win.closeRestore)
	win.liveMgr.Store(win.mgr)
	id := win.mgr.Active().ID
	win.upd.relaunch = func(update.Target) error {
		win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindSession, Session: reviewID, Agent: "claude"})
		return errors.New("probe relaunch failure")
	}
	win.checkUpdates()
	waitUpd(t, win, func() bool { return win.upd.newer })
	win.installUpdate()
	waitUpd(t, win, func() bool { return win.upd.err != "" })
	win.restore.Flush()
	got, _ := sessionstate.Load(win.restorePath)
	if len(got.Entries) == 0 || got.Entries[0].Session != reviewID {
		t.Fatalf("conversation from the handoff lost: %+v", got)
	}
}
