package tabs_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func TestNewOptsPassesEachTabsOwnDir(t *testing.T) {
	m, st := newManager(t)
	if _, err := m.NewOpts(tabs.StartOpts{Dir: `C:\src\api`}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.NewOpts(tabs.StartOpts{Dir: `C:\src\web`}); err != nil {
		t.Fatal(err)
	}
	mustNew(t, m)
	st.mu.Lock()
	defer st.mu.Unlock()
	want := []string{`C:\src\api`, `C:\src\web`, ""} // "" lets the window use its default
	if len(st.dirs) != len(want) {
		t.Fatalf("dirs %q, want %q", st.dirs, want)
	}
	for i := range want {
		if st.dirs[i] != want[i] {
			t.Fatalf("dirs %q, want %q", st.dirs, want)
		}
	}
}

func TestNewOptsKeepsGroupAndColor(t *testing.T) {
	m, _ := newManager(t)
	g := &tabs.Group{Workspace: "dev", Name: "api", Run: 3, Member: 1}
	tab, err := m.NewOpts(tabs.StartOpts{Title: "api", Color: "#ff8800", Group: g})
	if err != nil {
		t.Fatal(err)
	}
	if tab.Group != g || tab.Color != "#ff8800" || tab.FixedTitle != "api" {
		t.Fatalf("tab group %v color %q fixed %q", tab.Group, tab.Color, tab.FixedTitle)
	}
	if a := m.Active(); a == nil || a.ID != tab.ID {
		t.Fatal("NewOpts tab is not active")
	}
}

func TestFixedTitleSurvivesOSCTitle(t *testing.T) {
	m, st := newManager(t)
	fixed, err := m.NewOpts(tabs.StartOpts{Title: "api"})
	if err != nil {
		t.Fatal(err)
	}
	plain := mustNew(t, m)
	st.mu.Lock()
	fakes := append([]*fakePTY(nil), st.fakes...)
	st.mu.Unlock()
	fakes[0].outW.Write([]byte("\x1b]0;pwsh\x07"))
	fakes[1].outW.Write([]byte("\x1b]0;pwsh\x07"))
	oscTitle := func(tab *tabs.Tab) string {
		tab.Session.Lock()
		defer tab.Session.Unlock()
		return tab.Session.Applier().Title()
	}
	waitFor(t, "OSC titles applied", func() bool { return oscTitle(fixed) == "pwsh" && oscTitle(plain) == "pwsh" })

	for _, tab := range m.Tabs() {
		want := map[tabs.TabID]string{fixed.ID: "api", plain.ID: "pwsh"}[tab.ID]
		if tab.Title != want {
			t.Fatalf("tab %d title %q, want %q", tab.ID, tab.Title, want)
		}
	}
	m.SetActive(fixed.ID)
	if got := m.Active().Title; got != "api" {
		t.Fatalf("active title %q, want the fixed one", got)
	}
}

// A watched background tab asks for a frame on output, so a workspace tab's
// first prompt is noticed without waiting for some other wake.
func TestWatchedBackgroundTabRequestsFrames(t *testing.T) {
	st := &starter{}
	var frames atomic.Int64
	m := tabs.NewManager(st.start, 80, 24, func() { frames.Add(1) })
	t.Cleanup(st.closeAll)
	bg := mustNew(t, m)
	mustNew(t, m) // active
	m.Watch(bg.ID, true)
	before := frames.Load()
	st.fakes[0].outW.Write([]byte("prompt> "))
	waitFor(t, "a frame for watched output", func() bool { return frames.Load() > before })

	m.Watch(bg.ID, false)
	st.fakes[0].outW.Write([]byte("x"))
	waitFor(t, "output applied", func() bool {
		bg.Session.Lock()
		defer bg.Session.Unlock()
		return bg.Session.Screen().Cell(8, 0).Rune == 'x'
	})
	time.Sleep(20 * time.Millisecond) // let any stray notifications land
	settled := frames.Load()
	st.fakes[0].outW.Write([]byte("y"))
	time.Sleep(20 * time.Millisecond)
	if frames.Load() != settled {
		t.Fatal("an unwatched background tab still asks for frames")
	}
	m.Watch(9999, true) // unknown tabs are ignored
}

// A launched workspace member is no longer watched for every output burst, but
// a later prompt still changes its dropdown status from Running to Finished.
func TestWorkspaceBackgroundPromptRequestsFrameWithoutWatchingOutput(t *testing.T) {
	st := &starter{}
	changed := make(chan struct{}, 16)
	m := tabs.NewManager(st.start, 80, 24, func() { changed <- struct{}{} })
	t.Cleanup(st.closeAll)
	bg, err := m.NewOpts(tabs.StartOpts{Title: "worker", Group: &tabs.Group{Workspace: "w", Run: 1}})
	if err != nil {
		t.Fatal(err)
	}
	mustNew(t, m)
	st.fakes[0].outW.Write([]byte("started"))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("initial title notification missing")
	}
	st.fakes[0].outW.Write([]byte("\x1b]133;A\x07"))
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("background workspace prompt did not request a frame")
	}
	bg.Session.Lock()
	count := bg.Session.Applier().PromptSeq()
	bg.Session.Unlock()
	if count != 1 {
		t.Fatalf("prompt count %d", count)
	}
	st.fakes[0].outW.Write([]byte("ordinary output"))
	select {
	case <-changed:
		t.Fatal("ordinary output requested a frame without watching")
	case <-time.After(50 * time.Millisecond):
	}
}
