package ui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/sessionstate"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/update"
)

func restoreFixture(t *testing.T) (*runFixture, string) {
	f := newRunFixture(t)
	f.win.opt.Config = config.Default()
	f.win.restorePath = filepath.Join(t.TempDir(), "sessions.json")
	return f, f.win.restorePath
}

func TestResumeUsesShellPathWithMinimalAppPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // neither agent exists in the GUI app's PATH
	id := "01234567-89ab-cdef-0123-456789abcdef"
	for _, shell := range []string{"/bin/zsh", "pwsh.exe"} {
		for _, agent := range []string{"claude", "codex"} {
			win := &window{}
			win.opt.Shell = []string{shell}
			line, err := win.resumeLine(sessionstate.Entry{Agent: agent, Session: id})
			if err != nil {
				t.Fatalf("%s via %s: %v", agent, shell, err)
			}
			want := agent + " "
			if agent == "claude" {
				want += "--resume "
			} else {
				want += "resume "
			}
			want += "'" + id + "'"
			if line != want {
				t.Fatalf("got %q, want %q", line, want)
			}
		}
	}
}
func TestRestoreExactSessionsAndNoRecipeReplay(t *testing.T) {
	f, p := restoreFixture(t)
	ids := []string{"01234567-89ab-cdef-0123-456789abcdef", "11234567-89ab-cdef-0123-456789abcdef", "21234567-89ab-cdef-0123-456789abcdef", "31234567-89ab-cdef-0123-456789abcdef"}
	s := sessionstate.Snapshot{Version: 1, Active: 1, Entries: []sessionstate.Entry{
		{Cwd: f.dirs[0], Title: "Claude review", Agent: "claude", Session: ids[0]},
		{Cwd: f.dirs[0], Title: "Codex build", Agent: "codex", Session: ids[1], Workspace: "ws_samurai", WorkspaceName: "samurai", Run: 3, Member: 0},
		{Cwd: f.dirs[0], Agent: "claude", Session: ids[2]},
		{Cwd: f.dirs[0], Agent: "codex", Session: ids[3]},
	}}
	if err := sessionstate.Save(p, s); err != nil {
		t.Fatal(err)
	}
	f.win.opt.Config.StartupWorkspace = "ws_samurai"
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	if err := f.win.openStartup(nil); err != nil {
		t.Fatal(err)
	}
	list := f.win.mgr.Tabs()
	if len(list) != 4 || f.win.mgr.Active().ID != list[1].ID || f.win.names[list[0].ID] != "Claude review" {
		t.Fatalf("%+v", list)
	}
	if len(f.win.launches) != 4 {
		t.Fatalf("launches %d", len(f.win.launches))
	}
	for i, tb := range list {
		l := f.win.launches[tb.ID]
		if !strings.Contains(l.run.Line, ids[i]) {
			t.Fatal(l.run.Line)
		}
		if strings.Contains(l.run.Line, "--last") {
			t.Fatal("picked latest")
		}
	}
	if !strings.Contains(f.win.launches[list[0].ID].run.Line, "--resume") || !strings.Contains(f.win.launches[list[1].ID].run.Line, "resume") {
		t.Fatal("wrong commands")
	}
	f.win.runWorkspace("ws_samurai")
	if len(f.win.mgr.Tabs()) != 4 {
		t.Fatal("workspace duplicated")
	}
	if err := f.win.openStartup(nil); err != nil {
		t.Fatal(err)
	}
	if len(f.win.mgr.Tabs()) != 4 {
		t.Fatal("restored twice")
	}
}
func TestRestoreMissingFolderAndExplicitWorkspace(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(map[bool]string{true: "explicit", false: "fallback"}[explicit], func(t *testing.T) {
			f, p := restoreFixture(t)
			sessionstate.Save(p, sessionstate.Snapshot{Version: 1, Entries: []sessionstate.Entry{{Cwd: filepath.Join(t.TempDir(), "gone"), Agent: "codex", Session: "01234567-89ab-cdef-0123-456789abcdef"}}})
			if explicit {
				f.win.opt.Workspace = "samurai"
			}
			f.win.initRestore()
			t.Cleanup(f.win.closeRestore)
			if err := f.win.openStartup(nil); err != nil {
				t.Fatal(err)
			}
			if explicit {
				if len(f.win.mgr.Tabs()) != 4 {
					t.Fatal("explicit workspace ignored")
				}
			} else {
				if len(f.win.mgr.Tabs()) != 1 || len(f.win.launches) != 0 {
					t.Fatal("missing directory launched agent")
				}
				if len(f.win.opt.Warnings) == 0 {
					t.Fatal("missing warning")
				}
			}
		})
	}
}
func TestScratchCannotReplaceSavedMainIdentity(t *testing.T) {
	f, _ := restoreFixture(t)
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	main := f.win.mgr.Active()
	scratch, err := f.win.mgr.NewOpts(tabs.StartOpts{Parent: main.ID})
	if err != nil {
		t.Fatal(err)
	}
	f.win.liveMgr.Store(f.win.mgr)
	id := "01234567-89ab-cdef-0123-456789abcdef"
	other := "11234567-89ab-cdef-0123-456789abcdef"
	f.win.onNotify(notify.Msg{Tab: int(main.ID), Kind: notify.KindSession, Session: id, Agent: "claude"})
	f.win.onNotify(notify.Msg{Tab: int(scratch.ID), Kind: notify.KindSession, Session: other, Agent: "codex"})
	f.win.restore.Flush()
	got, err := sessionstate.Load(f.win.restorePath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 || got.Entries[0].Session != id {
		t.Fatalf("%+v", got)
	}
}
func TestRestoreToggleAvailableWithoutWorkspaces(t *testing.T) {
	f, p := restoreFixture(t)
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	f.win.wsp.doc.Workspaces = nil
	found := false
	for _, r := range f.win.listRows() {
		for _, c := range r.ctrls {
			if c.id == "ws.restore" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("toggle absent in empty workspace list")
	}
	wsAct(f.win, "ws.restore", 0)
	f.win.restore.Flush()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("opt-out retained state")
	}
}

func TestUpdateFlushesBeforeRelaunchAndRetainsOwnershipOnFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			rel := newFakeReleases(t, "v99.0", "new", "")
			win, _, _, quit, _ := updWindow(t, rel)
			win.restorePath = filepath.Join(t.TempDir(), "sessions.json")
			win.opt.Config.RestoreSessions = true
			win.initRestore()
			win.trackRestore()
			t.Cleanup(win.closeRestore)
			id := win.mgr.Active().ID
			win.restore.Rename(int(id), "saved before relaunch")
			win.upd.relaunch = func(_ update.Target) error {
				got, err := sessionstate.Load(win.restorePath)
				if err != nil {
					return err
				}
				found := false
				for _, e := range got.Entries {
					if e.Title == "saved before relaunch" {
						found = true
					}
				}
				if !found {
					return errors.New("snapshot not flushed")
				}
				if fail {
					return errors.New("probe relaunch failure")
				}
				return nil
			}
			win.checkUpdates()
			waitUpd(t, win, func() bool { return win.upd.newer })
			win.installUpdate()
			waitUpd(t, win, func() bool { return *quit > 0 || win.upd.err != "" })
			if fail {
				if *quit != 0 || win.restartBusy {
					t.Fatal("failed restart did not resume window")
				}
				if other, err := sessionstate.Open(win.restorePath); err == nil {
					other.Close()
					t.Fatal("failed restart lost ownership")
				}
			} else if *quit != 1 {
				t.Fatal("no quit")
			}
		})
	}
}

func TestRestoreRejectsInvalidSnapshotAndShowsFallbackWarnings(t *testing.T) {
	f, p := restoreFixture(t)
	os.WriteFile(p, []byte(`{"version":99,"active":0,"entries":[{"cwd":"`+f.dirs[0]+`","title":"must not restore"}]}`), 0600)
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	if len(f.win.mgr.Tabs()) != 1 || len(f.win.names) != 0 {
		t.Fatal("invalid snapshot was restored")
	}
	if !strings.Contains(firstRows(f), "Could not read saved sessions") {
		t.Fatal("startup warning not visible")
	}
}
func TestRestoredWorkspaceResumeUsesReadinessBanner(t *testing.T) {
	f, p := restoreFixture(t)
	sessionstate.Save(p, sessionstate.Snapshot{Version: 1, Entries: []sessionstate.Entry{{Cwd: f.dirs[0], Agent: "codex", Session: "01234567-89ab-cdef-0123-456789abcdef", Workspace: "ws_samurai", WorkspaceName: "samurai", Run: 1}}})
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	tb := f.win.mgr.Active()
	if f.win.bannerMember(tb) != f.win.launchOf(tb) {
		t.Fatal("workspace member masks resume readiness banner")
	}
	f.win.takeOver(tb)
	f.win.restore.Flush()
	got, _ := sessionstate.Load(p)
	if len(f.win.launches) != 0 || got.Entries[0].Session != "" {
		t.Fatal("typing did not cancel persisted pending resume")
	}
}

func TestRestoreWaitsForPromptSubmitsOnlyOnceAndTracksBackgroundReturn(t *testing.T) {
	f, p := restoreFixture(t)
	id := "01234567-89ab-cdef-0123-456789abcdef"
	sessionstate.Save(p, sessionstate.Snapshot{Version: 1, Entries: []sessionstate.Entry{{Cwd: f.dirs[0], Agent: "codex", Session: id}}})
	f.win.initRestore()
	t.Cleanup(f.win.closeRestore)
	f.win.openStartup(nil)
	f.win.launchTick(f.now)
	if f.st.pty(0).sent() != "" {
		t.Fatal("submitted before prompt")
	}
	f.prompt(0)
	f.win.launchTick(f.now)
	deadline := time.Now().Add(2 * time.Second)
	for f.st.pty(0).sent() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	first := f.st.pty(0).sent()
	if !strings.Contains(first, "resume") || !strings.HasSuffix(first, "\r") {
		t.Fatal(first)
	}
	f.win.launchTick(f.now)
	if f.st.pty(0).sent() != first {
		t.Fatal("submitted twice")
	}
	// A subsequent prompt clears the agent in persistence even without a frame.
	f.win.mgr.New()
	f.promptAgain(0)
	f.win.captureRestore()
	f.win.restore.Flush()
	got, _ := sessionstate.Load(p)
	if got.Entries[0].Agent != "" {
		t.Fatal("background shell return kept agent identity")
	}
}
