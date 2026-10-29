package ui

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// notesWorkspace is a second, one-tab workspace in the fixture's first folder.
func notesWorkspace(f *runFixture) workspaces.Workspace {
	return workspaces.Workspace{ID: "ws_notes", Name: "notes", Tabs: []workspaces.Tab{
		{ID: "t_n", Title: "notes", Folder: f.dirs[0], Color: "green", Wait: workspaces.WaitNone},
	}}
}

// firstRows is the first tab's top rows, where startup warnings are written.
func firstRows(f *runFixture) string {
	t := f.win.mgr.Tabs()[0]
	var rows []string
	for r := range 4 {
		rows = append(rows, visibleRow(t.Session, r))
	}
	return strings.Join(rows, "\n")
}

func TestStartupPrecedence(t *testing.T) {
	cases := []struct {
		name      string
		flag, id  string
		setup     func(f *runFixture)
		workspace string // the run's workspace; "" means the usual single tab
		warn      string // in the first tab
	}{
		{name: "nothing chosen"},
		{name: "startup id", id: "ws_samurai", workspace: "ws_samurai"},
		{name: "flag by name, any case", flag: "SAMURAI", workspace: "ws_samurai"},
		{name: "flag wins over the id", flag: "notes", id: "ws_samurai", workspace: "ws_notes"},
		{name: "unknown name never falls back to the id", flag: "nope", id: "ws_samurai", warn: `no workspace is named "nope"`},
		{name: "missing id", id: "ws_gone", warn: "startup workspace no longer exists"},
		{name: "invalid file", id: "ws_samurai", setup: func(f *runFixture) {
			if err := os.WriteFile(workspaces.Path(f.win.cfgPath), []byte("schema_version = 9\n"), 0o644); err != nil {
				f.t.Fatal(err)
			}
		}, warn: "schema_version must be 1"},
		{name: "preflight fails", id: "ws_samurai", setup: func(f *runFixture) {
			f.win.opt.Shell = []string{`C:\Windows\System32\cmd.exe`}
		}, warn: `workspace "samurai" not started`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRunFixture(t)
			doc, _, _ := workspaces.Load(workspaces.Path(f.win.cfgPath))
			f.save(doc.Workspaces[0], notesWorkspace(f))
			f.win.opt.Config = config.Default()
			f.win.opt.Workspace, f.win.opt.Config.StartupWorkspace = c.flag, c.id
			if c.setup != nil {
				c.setup(f)
			}
			if err := f.win.openStartup(nil); err != nil {
				t.Fatal(err)
			}
			list := f.win.mgr.Tabs()
			if c.workspace == "" {
				if len(list) != 1 || list[0].Group != nil || len(f.win.runs) != 0 {
					t.Fatalf("%d tabs, %d runs; want the usual single tab", len(list), len(f.win.runs))
				}
			} else {
				for _, tb := range list {
					if tb.Group == nil || tb.Group.Workspace != c.workspace {
						t.Fatalf("tab %q is not in %s; a valid workspace replaces the usual tab", tb.Title, c.workspace)
					}
				}
			}
			warns := strings.Join(f.win.opt.Warnings, "\n")
			if c.warn == "" && warns != "" || !strings.Contains(warns, c.warn) {
				t.Fatalf("warnings %q, want %q", warns, c.warn)
			}
			if c.warn != "" && !strings.Contains(firstRows(f), "kaze: ") {
				t.Fatalf("the first tab does not show the warning: %q", firstRows(f))
			}
			for i := range f.st.ptys {
				if got := f.st.pty(i).sent(); got != "" {
					t.Fatalf("shell %d was sent %q at startup", i, got)
				}
			}
		})
	}
}

// Every workspace shell failing still leaves a usable window with one tab.
func TestStartupFallsBackWhenNoWorkspaceTabStarts(t *testing.T) {
	f := newRunFixture(t)
	f.win.opt.Config.StartupWorkspace = "ws_samurai"
	calls := 0
	f.st.probe = func() {
		calls++
		f.st.mu.Lock()
		f.st.fail = nil
		if calls <= len(samuraiTabs) {
			f.st.fail = errors.New("out of ptys")
		}
		f.st.mu.Unlock()
	}
	if err := f.win.openStartup(nil); err != nil {
		t.Fatal(err)
	}
	list := f.win.mgr.Tabs()
	if len(list) != 1 || list[0].Group != nil {
		t.Fatalf("%d tabs after every workspace shell failed, want the usual one", len(list))
	}
	if warns := strings.Join(f.win.opt.Warnings, "\n"); !strings.Contains(warns, "samurai") || !strings.Contains(warns, "out of ptys") {
		t.Fatalf("warnings %q", warns)
	}
	f.win.wsTick(f.now)
	if len(f.win.runs) != 0 || f.win.shouldClose() {
		t.Fatalf("%d runs left, close %v", len(f.win.runs), f.win.shouldClose())
	}
}

// Config warnings still reach the owner when a workspace replaces the usual tab.
func TestStartupWorkspaceShowsTheConfigWarnings(t *testing.T) {
	f := newRunFixture(t)
	f.win.opt.Config.StartupWorkspace = "ws_samurai"
	f.win.opt.Warnings = []string{"font_size too big"}
	if err := f.win.openStartup(nil); err != nil {
		t.Fatal(err)
	}
	if len(f.win.mgr.Tabs()) != 4 {
		t.Fatalf("%d tabs", len(f.win.mgr.Tabs()))
	}
	if !strings.Contains(firstRows(f), "kaze: font_size too big") {
		t.Fatalf("first tab: %q", firstRows(f))
	}
}

// Review focus 2: Run after a startup run focuses it; nothing starts twice.
func TestRunAfterStartupStartsNothingTwice(t *testing.T) {
	f := newRunFixture(t)
	f.win.opt.Config.StartupWorkspace = "ws_samurai"
	if err := f.win.openStartup(nil); err != nil {
		t.Fatal(err)
	}
	f.win.runWorkspace("ws_samurai")
	if n := len(f.win.mgr.Tabs()); n != 4 || len(f.win.runs) != 1 {
		t.Fatalf("%d tabs and %d runs", n, len(f.win.runs))
	}
}
