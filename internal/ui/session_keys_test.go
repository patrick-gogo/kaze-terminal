package ui

import (
	"testing"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func TestMacSessionKeysThroughRouter(t *testing.T) {
	for _, front := range []string{"terminal", "board", "settings", "scratch"} {
		t.Run(front, func(t *testing.T) {
			win, _ := sideWindow(t, true)
			win.km, _ = buildKeymap("darwin", nil)
			win.binds, _ = resolveBindings("darwin", nil)
			a, b, c := openIn(t, win, ""), openIn(t, win, ""), openIn(t, win, "")
			openMember(t, win, "server")
			win.mgr.SetActive(a.ID)
			win.wantFocus = true
			r := new(input.Router)
			sideFrame(t, win, r)
			sideFrame(t, win, r)
			switch front {
			case "board":
				win.openBoard(0)
			case "settings":
				win.openSettings()
			case "scratch":
				win.toggleScratch()
			}
			sideFrame(t, win, r)
			sideFrame(t, win, r)
			first := b.ID
			if front == "settings" {
				first = a.ID
			}
			pressKey(t, win, r, key.NameTab, key.ModCtrl)
			if win.mgr.Active().ID != first || win.set.shown || win.board.open {
				t.Fatalf("Ctrl+Tab from %s: active %d, want %d", front, win.mgr.Active().ID, first)
			}
			// Start at a again and test both directions through the real router,
			// including wraparound. Workspace members and scratch are skipped.
			win.closeSettings()
			win.selectSession(a.ID)
			sideFrame(t, win, r)
			for _, want := range []tabs.TabID{b.ID, c.ID, a.ID} {
				pressKey(t, win, r, key.NameTab, key.ModCtrl)
				if win.mgr.Active().ID != want {
					t.Fatalf("next: got %d want %d", win.mgr.Active().ID, want)
				}
			}
			for _, want := range []tabs.TabID{c.ID, b.ID, a.ID} {
				pressKey(t, win, r, key.NameTab, key.ModCtrl|key.ModShift)
				if win.mgr.Active().ID != want {
					t.Fatalf("previous: got %d want %d", win.mgr.Active().ID, want)
				}
			}
		})
	}
}

func TestMacSessionKeysRespectOverrides(t *testing.T) {
	win, _ := sideWindow(t, true)
	var warns []string
	win.km, warns = buildKeymap("darwin", map[string]string{"next_tab": "ctrl+n", "prev_tab": "ctrl+tab"})
	if len(warns) != 0 {
		t.Fatal(warns)
	}
	a := openIn(t, win, "")
	openIn(t, win, "")
	c := openIn(t, win, "")
	win.mgr.SetActive(a.ID)
	win.wantFocus = true
	r := new(input.Router)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	pressKey(t, win, r, key.NameTab, key.ModCtrl)
	if win.mgr.Active().ID != c.ID {
		t.Fatal("overridden Ctrl+Tab did not select previous session")
	}
	pressKey(t, win, r, "N", key.ModCtrl)
	if win.mgr.Active().ID != a.ID {
		t.Fatal("custom next-session shortcut did not work")
	}
}
