package ui

import (
	"errors"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/sessionstate"
)

func TestStartDirFreshAndNewPlainTabs(t *testing.T) {
	f := newRunFixture(t)
	f.win.opt.Dir = f.win.home
	f.win.opt.Config = config.Default()
	f.win.opt.Config.StartDir = f.dirs[1]
	if err := f.win.openStartup(nil); err != nil {
		t.Fatal(err)
	}
	f.win.newTab()
	for i, tb := range f.win.mgr.Tabs() {
		if f.st.dirs[i] != f.dirs[1] || tb.Dir != f.dirs[1] {
			t.Fatalf("tab %d started in %q, metadata %q", i, f.st.dirs[i], tb.Dir)
		}
	}
	// Changing the default must not rewrite existing tabs before their first OSC 7.
	f.win.opt.Config.StartDir = f.dirs[0]
	f.win.newTab()
	st, _ := f.win.mgr.State()
	if st[0].Dir != f.dirs[1] || st[2].Dir != f.dirs[0] {
		t.Fatalf("%+v", st)
	}
}
func TestStartDirKeepsWorkspaceAndRestoredFolders(t *testing.T) {
	for _, kind := range []string{"workspace", "explicit workspace", "restored"} {
		t.Run(kind, func(t *testing.T) {
			f := newRunFixture(t)
			f.win.opt.Dir = f.win.home
			f.win.opt.Config = config.Default()
			start := t.TempDir()
			f.win.opt.Config.StartDir = start
			if kind == "restored" {
				f.win.restorePath = filepath.Join(t.TempDir(), "sessions.json")
				sessionstate.Save(f.win.restorePath, sessionstate.Snapshot{Version: 1, Entries: []sessionstate.Entry{{Cwd: f.dirs[0]}}})
				f.win.initRestore()
				t.Cleanup(f.win.closeRestore)
			} else if kind == "explicit workspace" {
				f.win.opt.Workspace = "samurai"
			} else {
				f.win.opt.Config.StartupWorkspace = "ws_samurai"
			}
			if err := f.win.openStartup(nil); err != nil {
				t.Fatal(err)
			}
			for _, dir := range f.st.dirs {
				if dir == start {
					t.Fatal("default replaced an explicit folder")
				}
			}
			if f.st.dirs[0] != f.dirs[0] {
				t.Fatalf("got %q", f.st.dirs[0])
			}
			f.win.newTab()
			if f.st.dirs[len(f.st.dirs)-1] != start {
				t.Fatal("new plain tab did not use start folder")
			}
		})
	}
}
func TestStartDirMissingFolderFallsBackToHome(t *testing.T) {
	f := newRunFixture(t)
	f.win.opt.Dir = f.win.home
	dir := filepath.Join(t.TempDir(), "gone")
	os.Mkdir(dir, 0700)
	f.win.opt.Config.StartDir = dir
	os.Remove(dir)
	f.win.newTab()
	if f.st.dirs[0] != f.win.home {
		t.Fatalf("started in %q", f.st.dirs[0])
	}
	if !strings.Contains(f.win.notice, "start_dir") {
		t.Fatal("no fallback warning")
	}
}
func TestStartDirPickerSavesAndClears(t *testing.T) {
	fp := usePicker(t)
	f := newWSPage(t)
	t.Setenv("HOME", f.win.home)
	t.Setenv("USERPROFILE", f.win.home)
	f.press("ws.start.pick", 0)
	select {
	case start := <-fp.starts:
		if start != f.win.home {
			t.Fatalf("started picker in %q", start)
		}
	case <-time.After(time.Second):
		t.Fatal("picker did not start")
	}
	fp.answer <- f.dirs[0]
	f.settlePick()
	cfg, w, err := config.Load(f.win.cfgPath)
	if err != nil || len(w) != 0 || cfg.StartDir == "" {
		t.Fatalf("%+v %v %v", cfg, w, err)
	}
	got, err := config.ResolveStartDir(cfg.StartDir, f.win.home)
	if err != nil || got != f.dirs[0] {
		t.Fatalf("%q %v", got, err)
	}
	f.press("ws.start.home", 0)
	cfg, _, _ = config.Load(f.win.cfgPath)
	if cfg.StartDir != "" {
		t.Fatal("Use home did not clear config")
	}
}
func TestStartDirPickerCancellationAndErrorPreserveSetting(t *testing.T) {
	for _, fail := range []string{"cancel", "error", "bad path", "empty result"} {
		t.Run(fail, func(t *testing.T) {
			f := newWSPage(t)
			f.win.opt.Config.StartDir = f.dirs[0]
			config.Set(f.win.cfgPath, "start_dir", f.dirs[0])
			before, _ := os.ReadFile(f.win.cfgPath)
			old := pickFolder
			pickFolder = func(string) (string, bool, error) {
				if fail == "error" {
					return "", false, errors.New("probe")
				}
				if fail == "empty result" {
					return "", true, nil
				}
				if fail == "bad path" {
					return filepath.Join(t.TempDir(), "missing"), true, nil
				}
				return "", false, nil
			}
			t.Cleanup(func() { pickFolder = old })
			f.draw()
			f.press("ws.start.pick", 0)
			f.settlePick()
			after, _ := os.ReadFile(f.win.cfgPath)
			if string(after) != string(before) || f.win.opt.Config.StartDir != f.dirs[0] {
				t.Fatal("unsuccessful picker changed config")
			}
		})
	}
}
func TestStartDirPickerSharedBusyStateAndEmptyWorkspaceList(t *testing.T) {
	fp := usePicker(t)
	f := newWSPage(t)
	f.win.wsp.doc.Workspaces = nil
	f.draw()
	f.press("ws.start.pick", 0)
	<-fp.starts
	f.draw()
	if c, ok := f.ctl("ws.start.pick", 0); !ok || !c.dead {
		t.Fatal("picker not disabled while busy")
	}
	wsAct(f.win, "ws.start.pick", 0)
	select {
	case <-fp.starts:
		t.Fatal("second picker opened")
	default:
	}
	fp.answer <- ""
	f.settlePick()
}

func TestStartDirNarrowLayoutContainsPathAndActions(t *testing.T) {
	f := newWSPage(t)
	f.win.wsp.doc.Workspaces = nil
	f.win.opt.Config.StartDir = "~/Documents/Client Projects/日本語/project-a/repositories/development"
	f.win.drawSettings(pageContext(480, 850), image.Rect(0, 0, 480, 850))
	var card image.Rectangle
	for _, c := range f.win.set.cards {
		if c.Min.Y > 200 {
			card = c
			break
		}
	}
	for _, id := range []string{"ws.start.pick", "ws.start.home"} {
		c, ok := f.ctl(id, 0)
		if !ok || !c.rect.In(card) {
			t.Fatalf("%s %v outside %v", id, c.rect, card)
		}
	}
	for _, s := range f.win.set.texts {
		if strings.HasPrefix(s, "~/Documents") && !strings.Contains(s, "…") {
			t.Fatal("long folder hint is not fitted to its narrow row")
		}
	}
}

func TestStartDirRealShellCwd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell probe; Windows has directory-routing tests")
	}
	home := t.TempDir()
	dir := filepath.Join(home, "Documents 日本語")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	win := &window{home: home}
	win.opt.Dir = home
	win.opt.Shell = []string{"/bin/sh", "-c", "pwd"}
	win.opt.Config.StartDir = "~/Documents 日本語"
	win.mgr = tabs.NewManager(win.start, 160, 24, nil)
	t.Cleanup(win.mgr.CloseAll)
	tb, err := win.mgr.NewOpts(tabs.StartOpts{Dir: win.plainStartDir()})
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := filepath.EvalSymlinks(dir)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.TrimSpace(visibleRow(tb.Session, 0)) == expected {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("shell cwd %q, want %q", visibleRow(tb.Session, 0), expected)
}
