package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// dblClick presses twice on rect, 100ms apart.
func dblClick(t *testing.T, win *window, r *input.Router, rect image.Rectangle) {
	t.Helper()
	if rect.Empty() {
		t.Fatal("nothing to double click")
	}
	at := sidePt(win, centre(rect))
	for _, ms := range []time.Duration{0, 100} {
		r.Queue(
			pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at, Time: ms * time.Millisecond},
			pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at, Time: ms*time.Millisecond + 10},
		)
	}
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

func rowTitle(t *testing.T, win *window, id tabs.TabID) string {
	t.Helper()
	m := buildSide(win.sideIn())
	for _, g := range m.Groups {
		for _, row := range g.Rows {
			if row.ID == id {
				return row.Title
			}
		}
	}
	for _, row := range m.Loose {
		if row.ID == id {
			return row.Title
		}
	}
	t.Fatalf("no row for tab %d", id)
	return ""
}

// startRename double-clicks tab id's title and checks the editor took the keyboard.
func startRename(t *testing.T, win *window, r *input.Router, id tabs.TabID) {
	t.Helper()
	dblClick(t, win, r, rowHit(t, win, id).title)
	if win.side.renaming != id || win.focusTarget() != &win.side.ren || !r.Source().Focused(&win.side.ren) {
		t.Fatalf("double click: renaming %d, want %d (target ren %v, focused %v, rename rect %v)", win.side.renaming, id, win.focusTarget() == &win.side.ren, r.Source().Focused(&win.side.ren), win.side.hits.rename)
	}
}

// retype replaces the rename editor's text with s.
func retype(t *testing.T, win *window, r *input.Router, s string) {
	t.Helper()
	r.Queue(key.EditEvent{Range: key.Range{Start: 0, End: win.side.ren.Len()}, Text: s})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

func TestDoubleClickRenames(t *testing.T) {
	win, st, r, tb := filterWindow(t)
	startRename(t, win, r, tb[1].ID)
	if got := win.side.ren.Text(); got != "other" {
		t.Fatalf("editor prefilled %q, want the title other", got)
	}
	if s, e := win.side.ren.Selection(); s == e || min(s, e) != 0 || max(s, e) != win.side.ren.Len() {
		t.Fatalf("selection %d..%d, want all of %d", s, e, win.side.ren.Len())
	}
	retype(t, win, r, "api work")
	pressKey(t, win, r, key.NameReturn, 0)
	if got := rowTitle(t, win, tb[1].ID); got != "api work" || win.names[tb[1].ID] != "api work" {
		t.Fatalf("title %q, names %v", got, win.names)
	}
	if win.side.renaming != 0 || !r.Source().Focused(win.termTag) {
		t.Fatalf("after Enter: renaming %d", win.side.renaming)
	}
	typedReaches(t, win, r, st.ptys[1], "x")
}

func TestRenameBlankRestoresTheRule(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	win.names = map[tabs.TabID]string{tb[2].ID: "api work"}
	sideFrame(t, win, r)
	startRename(t, win, r, tb[2].ID)
	retype(t, win, r, "   ")
	pressKey(t, win, r, key.NameReturn, 0)
	if got := rowTitle(t, win, tb[2].ID); got != "misc" {
		t.Fatalf("title %q after a blank name, want the folder misc", got)
	}
	if _, ok := win.names[tb[2].ID]; ok {
		t.Fatalf("blank name kept: %v", win.names)
	}
}

func TestRenameEscCancels(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	startRename(t, win, r, tb[0].ID)
	retype(t, win, r, "zzz")
	pressKey(t, win, r, key.NameEscape, 0)
	if got := rowTitle(t, win, tb[0].ID); got != "ticket33" || len(win.names) != 0 {
		t.Fatalf("title %q names %v after Esc", got, win.names)
	}
	if win.side.renaming != 0 || !r.Source().Focused(win.termTag) {
		t.Fatalf("after Esc: renaming %d", win.side.renaming)
	}
}

func TestRenameClickElsewhereSaves(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	startRename(t, win, r, tb[0].ID)
	retype(t, win, r, "api work")
	sideClick(win, r, rowHit(t, win, tb[2].ID).rect, pointer.ButtonPrimary)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if got := rowTitle(t, win, tb[0].ID); got != "api work" || win.side.renaming != 0 {
		t.Fatalf("title %q renaming %d after a click elsewhere", got, win.side.renaming)
	}
	if win.mgr.Active().ID != tb[2].ID || !r.Source().Focused(win.termTag) {
		t.Fatal("the click did not show its row with the keyboard")
	}
}

func TestMemberLineDoesNotRename(t *testing.T) {
	win, _ := sideWindow(t, true)
	openIn(t, win, `C:\code\alpha\x`)
	m := openMember(t, win, "api")
	r := new(input.Router)
	sideFrame(t, win, r)
	var line image.Rectangle
	for _, h := range win.side.hits.members {
		if h.id == m.ID {
			line = h.rect
		}
	}
	dblClick(t, win, r, line)
	if win.side.renaming != 0 {
		t.Fatalf("a member line started a rename of %d", win.side.renaming)
	}
}

func TestRenameSurvivesClaudeLeaving(t *testing.T) {
	win, _, r, tb := filterWindow(t)
	a, b := tb[0], tb[1]
	deliverNow(win, promptMsg(int(a.ID), "/ship-ticket", "V3-3348"), t0)
	deliverNow(win, promptMsg(int(b.ID), "/ship-ticket", "V3-1"), t0)
	sideFrame(t, win, r)
	startRename(t, win, r, b.ID)
	retype(t, win, r, "api work")
	pressKey(t, win, r, key.NameReturn, 0)
	report(t, a, osPath("/code/alpha/ticket33")) // both shells are back at a prompt: Claude left
	report(t, b, osPath("/code/alpha/other"))
	sideFrame(t, win, r)
	m := buildSide(win.sideIn())
	rows := m.Groups[0].Rows
	if rows[0].Title != "V3-3348" || rows[0].Running || rows[0].Command != "" {
		t.Fatalf("after Claude left, row %+v, want its title V3-3348 kept", rows[0])
	}
	if rows[1].Title != "api work" {
		t.Fatalf("after Claude left, named row %+v", rows[1])
	}
}

func TestShortcutWhileRenamingEndsIt(t *testing.T) {
	win, st, r, tb := filterWindow(t)
	startRename(t, win, r, tb[0].ID)
	retype(t, win, r, "api work")
	before := len(win.mgr.Tabs())
	sc := shortcutFor(t, win, actNewTab)
	pressKey(t, win, r, sc.Name, sc.Modifiers)
	if got := len(win.mgr.Tabs()); got != before+1 {
		t.Fatalf("%d sessions after the new-session shortcut, want %d", got, before+1)
	}
	if win.side.renaming != 0 || win.focusTarget() != win.termTag || !r.Source().Focused(win.termTag) {
		t.Fatalf("after the shortcut: renaming %d", win.side.renaming)
	}
	if win.names[tb[0].ID] != "api work" {
		t.Fatalf("names %v, want the typed name saved", win.names)
	}
	typedReaches(t, win, r, st.ptys[3], "x")
}

// A renamed row scrolled out of the list ends the rename, so keys reach the terminal.
func TestRenameEndsWhenItsRowScrollsAway(t *testing.T) {
	win, st := sideWindow(t, true)
	var first *tabs.Tab
	for i := range 20 {
		tab := openIn(t, win, "")
		if i == 0 {
			first = tab
		}
	}
	win.mgr.SetActive(first.ID)
	r := new(input.Router)
	sideFrame(t, win, r)
	startRename(t, win, r, first.ID)
	retype(t, win, r, "api work")
	at := sidePt(win, centre(win.side.hits.list))
	r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: at, Scroll: f32.Pt(0, 1<<16)})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if win.side.renaming != 0 || win.names[first.ID] != "api work" {
		t.Fatalf("after scrolling the row away: renaming %d, names %v", win.side.renaming, win.names)
	}
	typedReaches(t, win, r, st.ptys[0], "x")
}

// A renamed row the filter stops matching ends the rename, so keys reach the terminal.
func TestRenameEndsWhenItsRowIsFilteredOut(t *testing.T) {
	win, st, r, tb := filterWindow(t)
	deliverNow(win, promptMsg(int(tb[0].ID), "/ship-ticket", ""), t0)
	focusFilter(t, win, r)
	typeFilter(t, win, r, "ship") // matches only the command
	if !hasRow(win, tb[0].ID) || hasRow(win, tb[1].ID) {
		t.Fatalf("setup: rows %+v", win.side.hits.rows)
	}
	startRename(t, win, r, tb[0].ID)
	retype(t, win, r, "api work")
	report(t, tb[0], `C:\code\alpha\ticket33`) // Claude left: the command and the match go
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if win.side.renaming != 0 || hasRow(win, tb[0].ID) {
		t.Fatalf("after the row was filtered out: renaming %d", win.side.renaming)
	}
	typedReaches(t, win, r, st.ptys[0], "x")
}
