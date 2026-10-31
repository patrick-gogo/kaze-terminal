package ui

import (
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

func clickMember(t *testing.T, win *window, r *input.Router, id tabs.TabID) {
	t.Helper()
	sideClick(win, r, memberHit(t, win, id).rect, pointer.ButtonPrimary)
	floatFrames(t, win, r)
}

func TestMemberClickFloatsOverTheSession(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	clickMember(t, win, r, m1.ID)
	p := paneOf(t, win, m1.ID)
	if win.mgr.Active().ID != a.ID || win.fl.owner(m1.ID) != a.ID || p.hits.outer.Empty() || win.fl.keys != m1.ID {
		t.Fatalf("active %d owner %d keys %d outer %v", win.mgr.Active().ID, win.fl.owner(m1.ID), win.fl.keys, p.hits.outer)
	}
}

func TestMemberClickWhenItIsTheMainViewDoesNothing(t *testing.T) {
	win, _, r, _, _, m1, _ := paneWindow(t)
	win.selectTab(m1.ID)
	sideFrame(t, win, r)
	clickMember(t, win, r, m1.ID)
	if win.mgr.Active().ID != m1.ID || win.fl.owner(m1.ID) != 0 {
		t.Fatalf("active %d owner %d", win.mgr.Active().ID, win.fl.owner(m1.ID))
	}
}

func TestMemberClickWhileAMemberIsShownShowsIt(t *testing.T) {
	win, _, r, _, _, m1, m2 := paneWindow(t)
	win.selectTab(m1.ID)
	sideFrame(t, win, r)
	clickMember(t, win, r, m2.ID)
	if win.mgr.Active().ID != m2.ID || win.fl.owner(m2.ID) != 0 {
		t.Fatalf("active %d owner %d", win.mgr.Active().ID, win.fl.owner(m2.ID))
	}
}

func TestExpandMakesTheMemberTheMainView(t *testing.T) {
	win, st, r, _, _, m1, _ := paneWindow(t)
	clickMember(t, win, r, m1.ID)
	clickAt(t, win, r, centre(paneOf(t, win, m1.ID).hits.expand))
	cols, rows := win.mgr.Size()
	if win.mgr.Active().ID != m1.ID || win.fl.owner(m1.ID) != 0 || ptyOf(st, 2).lastSize() != [2]int{cols, rows} || win.side.st.shown != m1.ID {
		t.Fatalf("active %d owner %d pty %v shown %d", win.mgr.Active().ID, win.fl.owner(m1.ID), ptyOf(st, 2).lastSize(), win.side.st.shown)
	}
}

func TestPickingASessionAfterExpandGoesBack(t *testing.T) {
	win, _, r, a, _, m1, m2 := paneWindow(t)
	clickMember(t, win, r, m2.ID)
	clickMember(t, win, r, m1.ID)
	clickAt(t, win, r, centre(paneOf(t, win, m1.ID).hits.expand))
	sideClick(win, r, rowHit(t, win, a.ID).rect, pointer.ButtonPrimary)
	floatFrames(t, win, r)
	if win.mgr.Active().ID != a.ID || paneOf(t, win, m2.ID).hits.outer.Empty() {
		t.Fatalf("active %d, m2 shown %v", win.mgr.Active().ID, paneOf(t, win, m2.ID).hits.outer)
	}
}

func TestCloseHidesAMemberWithoutStoppingIt(t *testing.T) {
	win, st, r, _, _, m1, _ := paneWindow(t)
	clickMember(t, win, r, m1.ID)
	clickAt(t, win, r, centre(paneOf(t, win, m1.ID).hits.close))
	if win.fl.owner(m1.ID) != 0 || !win.mgr.Has(m1.ID) || win.memberOf(m1) == nil {
		t.Fatalf("owner %d, open %v", win.fl.owner(m1.ID), win.mgr.Has(m1.ID))
	}
	time.Sleep(20 * time.Millisecond)
	if got := ptyOf(st, 2).sent(); got != "" {
		t.Fatalf("closing the pane wrote %q", got)
	}
}

func TestOpeningAMemberInAnotherSessionMovesIt(t *testing.T) {
	win, _, r, a, b, m1, _ := paneWindow(t)
	clickMember(t, win, r, m1.ID)
	win.selectTab(b.ID)
	sideFrame(t, win, r)
	clickMember(t, win, r, m1.ID)
	if win.fl.owner(m1.ID) != b.ID {
		t.Fatalf("owner %d", win.fl.owner(m1.ID))
	}
	win.selectTab(a.ID)
	floatFrames(t, win, r)
	if len(win.fl.views) != 0 {
		t.Fatalf("a still shows %d panes", len(win.fl.views))
	}
}

func TestFifthMemberReplacesTheOldest(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	ids := []tabs.TabID{m1.ID}
	for _, name := range []string{"x", "y", "z", "w"} {
		ids = append(ids, openMember(t, win, name).ID)
	}
	win.selectTab(a.ID)
	sideFrame(t, win, r)
	for _, id := range ids {
		clickMember(t, win, r, id)
	}
	cols, rows := win.mgr.Size()
	if n := len(win.fl.set(a.ID).panes); n != maxPanes || win.fl.owner(m1.ID) != 0 || ptyOf(st, 2).lastSize() != [2]int{cols, rows} {
		t.Fatalf("%d panes, m1 owner %d pty %v", n, win.fl.owner(m1.ID), ptyOf(st, 2).lastSize())
	}
}

func TestServerDotsFloatTheDownMember(t *testing.T) {
	win, _, r, a, _, _, m2 := paneWindow(t)
	win.runs[1].members[1].run.State = workspaces.Failed
	sideFrame(t, win, r)
	if win.statusRect(grpServers).Empty() {
		t.Fatal("no server dots")
	}
	clickAt(t, win, r, centre(win.statusRect(grpServers)))
	if win.fl.owner(m2.ID) != a.ID || win.mgr.Active().ID != a.ID {
		t.Fatalf("owner %d active %d", win.fl.owner(m2.ID), win.mgr.Active().ID)
	}
}

func TestMemberExitRemovesItsPane(t *testing.T) {
	win, _, r, _, _, m1, _ := paneWindow(t)
	clickMember(t, win, r, m1.ID)
	win.mgr.Close(m1.ID)
	floatFrames(t, win, r)
	if win.fl.owner(m1.ID) != 0 || len(win.fl.rends) != 0 || win.fl.keys != 0 || len(win.fl.views) != 0 {
		t.Fatalf("owner %d rends %d keys %d", win.fl.owner(m1.ID), len(win.fl.rends), win.fl.keys)
	}
	if !win.focused || win.focusTarget() != win.termTag {
		t.Fatal("the keyboard did not come back to the session")
	}
}

func TestFloatingMemberIsHighlighted(t *testing.T) {
	win, _, r, _, _, m1, m2 := paneWindow(t)
	clickMember(t, win, r, m1.ID)
	if win.side.st.floating != m1.ID {
		t.Fatalf("floating %d", win.side.st.floating)
	}
	clickMember(t, win, r, m2.ID)
	if win.side.st.floating != m2.ID {
		t.Fatalf("floating %d after m2", win.side.st.floating)
	}
}

func TestPaletteFloatsAMember(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	win.palAct(pageContext(10, 10), palItem{Section: palSessions, Member: true, Tab: m1.ID}, false)
	floatFrames(t, win, r)
	if win.fl.owner(m1.ID) != a.ID || win.mgr.Active().ID != a.ID {
		t.Fatalf("owner %d active %d", win.fl.owner(m1.ID), win.mgr.Active().ID)
	}
}

func TestFloatingAMemberMovesTheFocusReports(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	emitTo(t, st, 0, a, "\x1b[?1004h", func() bool { return modes(a.Session).FocusEvents })
	emitTo(t, st, 2, m1, "\x1b[?1004h", func() bool { return modes(m1.Session).FocusEvents })
	clickMember(t, win, r, m1.ID)
	waitSent(t, ptyOf(st, 0), "\x1b[O")
	waitSent(t, ptyOf(st, 2), "\x1b[I")
}

// A member floated from the sidebar takes the keyboard: typing goes to it at once.
func TestTypingAfterFloatingAMemberReachesIt(t *testing.T) {
	win, st, r, _, _, m1, _ := paneWindow(t)
	clickMember(t, win, r, m1.ID)
	typeKeys(t, win, r, "q")
	waitSent(t, ptyOf(st, 2), "q")
	if got := ptyOf(st, 0).sent(); got != "" {
		t.Fatalf("the session got %q", got)
	}
}
