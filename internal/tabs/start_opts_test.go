package tabs_test

import (
	"testing"

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
