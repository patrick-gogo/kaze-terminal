package ui

import (
	"testing"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// filterWindow has three sessions in two repos, the first one shown, and the
// filter field focused by a click.
func filterWindow(t *testing.T) (*window, *recStarter, *input.Router, [3]*tabs.Tab) {
	t.Helper()
	win, st := sideWindow(t, true)
	a := openIn(t, win, osPath("/code/alpha/ticket33"))
	b := openIn(t, win, osPath("/code/alpha/other"))
	c := openIn(t, win, osPath("/code/beta/misc"))
	win.mgr.SetActive(a.ID)
	r := new(input.Router)
	sideFrame(t, win, r)
	return win, st, r, [3]*tabs.Tab{a, b, c}
}

func focusFilter(t *testing.T, win *window, r *input.Router) {
	t.Helper()
	sideClick(win, r, win.side.hits.filter, pointer.ButtonPrimary)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if !win.side.filtering || !r.Source().Focused(&win.side.ed) {
		t.Fatalf("clicking the field did not give it the keyboard (filtering %v)", win.side.filtering)
	}
}

func typeFilter(t *testing.T, win *window, r *input.Router, s string) {
	t.Helper()
	n := win.side.ed.Len()
	r.Queue(key.EditEvent{Range: key.Range{Start: n, End: n}, Text: s})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

func pressKey(t *testing.T, win *window, r *input.Router, name key.Name, mods key.Modifiers) {
	t.Helper()
	r.Queue(key.Event{Name: name, Modifiers: mods, State: key.Press})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

func TestFilterNarrowsRows(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "33")
	if !hasRow(win, tb[0].ID) || hasRow(win, tb[1].ID) || hasRow(win, tb[2].ID) {
		t.Fatalf("rows after typing 33: %+v", win.side.hits.rows)
	}
}

func TestFilterEscClearsAndReturnsKeys(t *testing.T) {
	win, st, r, tb := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "33")
	pressKey(t, win, r, key.NameEscape, 0)
	if win.side.ed.Text() != "" || win.side.filtering {
		t.Fatalf("after Esc: text %q, filtering %v", win.side.ed.Text(), win.side.filtering)
	}
	if !hasRow(win, tb[1].ID) {
		t.Fatal("rows did not come back after Esc")
	}
	if !r.Source().Focused(win.termTag) {
		t.Fatal("the terminal does not have the keyboard after Esc")
	}
	r.Queue(key.EditEvent{Text: "x"})
	sideFrame(t, win, r)
	if got := st.ptys[0].sent(); got != "x" {
		t.Fatalf("session got %q, want %q", got, "x")
	}
}

func TestFilterEnterShowsFirstMatch(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "misc")
	pressKey(t, win, r, key.NameReturn, 0)
	if got := win.mgr.Active().ID; got != tb[2].ID {
		t.Fatalf("shown %d after Enter, want %d", got, tb[2].ID)
	}
	if win.side.filtering || !r.Source().Focused(win.termTag) {
		t.Fatal("the session does not have the keyboard after Enter")
	}
	if win.side.ed.Text() != "misc" {
		t.Fatalf("Enter changed the text to %q", win.side.ed.Text())
	}
}

func TestFilterKeepsTheShownSession(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "misc") // hides the shown session's row
	if hasRow(win, tb[0].ID) {
		t.Fatal("the shown session's row is not hidden")
	}
	if got := win.mgr.Active().ID; got != tb[0].ID {
		t.Fatalf("filtering moved the shown session to %d", got)
	}
	pressKey(t, win, r, key.NameEscape, 0)
	if !hasRow(win, tb[0].ID) || win.side.st.shown != tb[0].ID {
		t.Fatalf("shown row is not back and highlighted: shown %d", win.side.st.shown)
	}
}

func TestFilterNoMatches(t *testing.T) {
	win, _, r, _ := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "zzz")
	if !win.side.hits.noMatches || len(win.side.hits.rows) != 0 {
		t.Fatalf("noMatches %v, rows %+v", win.side.hits.noMatches, win.side.hits.rows)
	}
}

func TestShortcutsWorkWhileFiltering(t *testing.T) {
	win, _, r, _ := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "33")
	var sc key.Event
	for k, a := range win.km {
		if a == actNewTab {
			sc = key.Event{Name: k.Name, Modifiers: k.Mods, State: key.Press}
		}
	}
	before := len(win.mgr.Tabs())
	r.Queue(sc)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if got := len(win.mgr.Tabs()); got != before+1 {
		t.Fatalf("%d sessions after the new-tab shortcut, want %d", got, before+1)
	}
	if win.side.ed.Text() != "33" {
		t.Fatalf("text %q after the shortcut, want 33", win.side.ed.Text())
	}
}

func TestClickingTheTerminalLeavesTheFilterText(t *testing.T) {
	win, _, r, _ := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "33")
	at := f32.Pt(float32(win.gridMin.X+20), float32(win.gridMin.Y+20))
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if win.side.filtering || win.side.ed.Text() != "33" || !r.Source().Focused(win.termTag) {
		t.Fatalf("filtering %v, text %q", win.side.filtering, win.side.ed.Text())
	}
}

// shortcutFor is the key event bound to a, from the window's keymap.
func shortcutFor(t *testing.T, win *window, a action) key.Event {
	t.Helper()
	for k, got := range win.km {
		if got == a {
			return key.Event{Name: k.Name, Modifiers: k.Mods, State: key.Press}
		}
	}
	t.Fatalf("no shortcut for action %d", a)
	return key.Event{}
}

// typedReaches types s and waits, bounded, for p to get exactly s.
func typedReaches(t *testing.T, win *window, r *input.Router, p *recPTY, s string) {
	t.Helper()
	r.Queue(key.EditEvent{Text: s})
	sideFrame(t, win, r)
	waitSent(t, p, s)
}

func TestNewSessionWhileFilteringTakesTheKeyboard(t *testing.T) {
	win, st, r, _ := filterWindow(t)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "33")
	pressKey(t, win, r, shortcutFor(t, win, actNewTab).Name, shortcutFor(t, win, actNewTab).Modifiers)
	if win.side.filtering || win.focusTarget() != win.termTag || !r.Source().Focused(win.termTag) {
		t.Fatalf("after the new-session shortcut: filtering %v, terminal focused %v", win.side.filtering, r.Source().Focused(win.termTag))
	}
	typedReaches(t, win, r, st.ptys[3], "x")
}

func TestCtrlTabWhileFilteringTakesTheKeyboard(t *testing.T) {
	win, st, r, _ := filterWindow(t)
	focusFilter(t, win, r)
	pressKey(t, win, r, key.NameTab, key.ModCtrl)
	if win.side.filtering || !r.Source().Focused(win.termTag) {
		t.Fatalf("after Ctrl+Tab: filtering %v", win.side.filtering)
	}
	typedReaches(t, win, r, st.ptys[1], "x")
}

func TestToggleWhileFilteringGivesTheTerminalTheKeyboard(t *testing.T) {
	win, st, r, tb := filterWindow(t)
	focusFilter(t, win, r)
	pressKey(t, win, r, "B", key.ModCtrl|key.ModShift)
	if !win.side.hidden || win.side.filtering || !r.Source().Focused(win.termTag) {
		t.Fatalf("after Ctrl+Shift+B: hidden %v, filtering %v", win.side.hidden, win.side.filtering)
	}
	if win.mgr.Active().ID != tb[0].ID {
		t.Fatal("hiding the sidebar changed the shown session")
	}
	typedReaches(t, win, r, st.ptys[0], "y")
}

func TestNarrowingWhileFilteringGivesTheTerminalTheKeyboard(t *testing.T) {
	win, st, r, _ := filterWindow(t)
	focusFilter(t, win, r)
	sizedFrame(t, win, r, 500, 600)
	sizedFrame(t, win, r, 500, 600)
	if !win.side.hidden || win.side.filtering || !r.Source().Focused(win.termTag) {
		t.Fatalf("narrowed: hidden %v, filtering %v", win.side.hidden, win.side.filtering)
	}
	r.Queue(key.EditEvent{Text: "z"})
	sizedFrame(t, win, r, 500, 600)
	waitSent(t, st.ptys[0], "z")
}

func TestOpeningSettingsWhileFilteringFocusesThePage(t *testing.T) {
	win, _, r, _ := filterWindow(t)
	focusFilter(t, win, r)
	sc := shortcutFor(t, win, actOpenSettings)
	pressKey(t, win, r, sc.Name, sc.Modifiers)
	if !win.set.shown || win.side.filtering || win.focusTarget() != &win.set.tag || !r.Source().Focused(&win.set.tag) {
		t.Fatalf("after opening settings: shown %v, filtering %v", win.set.shown, win.side.filtering)
	}
}

func TestRowClickWhileFilteringGivesTheTerminalTheKeyboard(t *testing.T) {
	win, st, r, tb := filterWindow(t)
	focusFilter(t, win, r)
	sideClick(win, r, rowHit(t, win, tb[2].ID).rect, pointer.ButtonPrimary)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if win.mgr.Active().ID != tb[2].ID || win.focusTarget() != win.termTag || !r.Source().Focused(win.termTag) {
		t.Fatalf("row click: active %d, filtering %v", win.mgr.Active().ID, win.side.filtering)
	}
	typedReaches(t, win, r, st.ptys[2], "x")
}

func TestEnterOnAnEmptyFilterKeepsTheShownSession(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	win.mgr.SetActive(tb[1].ID)
	sideFrame(t, win, r)
	focusFilter(t, win, r)
	pressKey(t, win, r, key.NameReturn, 0)
	if got := win.mgr.Active().ID; got != tb[1].ID {
		t.Fatalf("Enter on an empty filter showed %d, want the shown session %d", got, tb[1].ID)
	}
	if win.side.filtering || !r.Source().Focused(win.termTag) {
		t.Fatal("Enter on an empty filter kept the keyboard in the field")
	}
}
