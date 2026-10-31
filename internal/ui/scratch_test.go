package ui

import (
	"slices"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// scratchOf is session's scratch tab, or nil.
func scratchOf(win *window, session tabs.TabID) *tabs.Tab {
	for _, t := range win.mgr.Tabs() {
		if t.Parent == session {
			return t
		}
	}
	return nil
}

func toggle(t *testing.T, win *window, r *input.Router) {
	t.Helper()
	r.Queue(shortcutFor(t, win, actScratch))
	floatFrames(t, win, r)
}

func TestScratchToggleStartsInTheSessionsFolder(t *testing.T) {
	win, st, r, a, _, _, _ := paneWindow(t)
	toggle(t, win, r)
	s := scratchOf(win, a.ID)
	if s == nil {
		t.Fatal("no scratch shell")
	}
	st.mu.Lock()
	dir := st.where[len(st.where)-1]
	st.mu.Unlock()
	if dir != osPath("/code/alpha/a") {
		t.Fatalf("started in %q", dir)
	}
	p := paneOf(t, win, s.ID)
	if p.kind != paneScratch || p.hits.outer.Empty() || win.fl.keys != s.ID || win.mgr.Active().ID != a.ID {
		t.Fatalf("pane %+v keys %d active %d", p, win.fl.keys, win.mgr.Active().ID)
	}
}

func TestScratchFallsBackToTheStartFolder(t *testing.T) {
	win, st, r, _, _, _, _ := paneWindow(t)
	x, err := win.mgr.NewOpts(tabs.StartOpts{Dir: osPath("/x")})
	if err != nil {
		t.Fatal(err)
	}
	win.selectTab(x.ID)
	sideFrame(t, win, r)
	toggle(t, win, r)
	st.mu.Lock()
	dir := st.where[len(st.where)-1]
	st.mu.Unlock()
	if scratchOf(win, x.ID) == nil || dir != osPath("/x") {
		t.Fatalf("started in %q", dir)
	}
}

func TestScratchToggleHidesAndKeepsItRunning(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	toggle(t, win, r)
	s := scratchOf(win, a.ID)
	p := paneOf(t, win, s.ID)
	rect := p.r
	toggle(t, win, r)
	if !p.hidden || !win.mgr.Has(s.ID) || win.fl.keys != 0 || !p.hits.outer.Empty() {
		t.Fatalf("hidden %v open %v keys %d", p.hidden, win.mgr.Has(s.ID), win.fl.keys)
	}
	toggle(t, win, r)
	if scratchOf(win, a.ID).ID != s.ID || p.hidden || p.r != rect || win.fl.keys != s.ID {
		t.Fatalf("third toggle: %+v", p)
	}
}

func TestScratchIsNotASession(t *testing.T) {
	win, _, r, a, b, _, _ := paneWindow(t)
	toggle(t, win, r)
	s := scratchOf(win, a.ID)
	if slices.Contains(win.sessionIDs(), s.ID) || hasRow(win, s.ID) {
		t.Fatal("the scratch shell is listed as a session")
	}
	for range 4 {
		r.Queue(shortcutFor(t, win, actNextTab))
		sideFrame(t, win, r)
		if id := win.mgr.Active().ID; id != a.ID && id != b.ID {
			t.Fatalf("next session showed %d", id)
		}
	}
}

func TestOneScratchPerSession(t *testing.T) {
	win, _, r, a, b, _, _ := paneWindow(t)
	toggle(t, win, r)
	win.selectTab(b.ID)
	sideFrame(t, win, r)
	toggle(t, win, r)
	sa, sb := scratchOf(win, a.ID), scratchOf(win, b.ID)
	if sa == nil || sb == nil || sa.ID == sb.ID {
		t.Fatalf("scratch %v %v", sa, sb)
	}
	if len(win.fl.views) != 1 || win.fl.views[0].p.id != sb.ID {
		t.Fatal("b shows a's scratch shell")
	}
}

func TestExitedScratchIsReplaced(t *testing.T) {
	win, st, r, a, _, _, _ := paneWindow(t)
	toggle(t, win, r)
	old := scratchOf(win, a.ID)
	ptyOf(st, 4).Close()
	deadline := time.Now().Add(3 * time.Second)
	for !old.Exited && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		win.mgr.Tabs()
	}
	toggle(t, win, r) // an exited shell is replaced, not hidden
	if win.mgr.Has(old.ID) {
		t.Fatal("the exited scratch shell is still open")
	}
	if s := scratchOf(win, a.ID); s == nil || s.ID == old.ID || paneOf(t, win, s.ID).hidden {
		t.Fatalf("no new scratch shell: %v", s)
	}
}

func TestClosingTheSessionClosesItsScratch(t *testing.T) {
	for _, hide := range []bool{false, true} {
		win, _, r, a, b, m1, m2 := paneWindow(t)
		toggle(t, win, r)
		if hide {
			toggle(t, win, r)
		}
		s := scratchOf(win, a.ID)
		win.closeTab(a.ID)
		sideFrame(t, win, r)
		if win.mgr.Has(s.ID) || win.mgr.Active().Parent != 0 {
			t.Fatalf("hide=%v: scratch open %v, active %+v", hide, win.mgr.Has(s.ID), win.mgr.Active())
		}
		for _, id := range []tabs.TabID{b.ID, m1.ID, m2.ID} {
			win.closeTab(id)
		}
		sideFrame(t, win, r)
		if !win.mgr.Empty() {
			t.Fatalf("hide=%v: %d tabs left", hide, len(win.mgr.Tabs()))
		}
	}
}

func TestOrphanScratchCloses(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	toggle(t, win, r)
	s := scratchOf(win, a.ID)
	win.mgr.Close(a.ID) // not through closeTab
	sideFrame(t, win, r)
	if win.mgr.Has(s.ID) {
		t.Fatal("a scratch shell outlived its session")
	}
}

func TestScratchToggleDoesNothingOverAMember(t *testing.T) {
	win, st, r, _, _, m1, _ := paneWindow(t)
	win.selectTab(m1.ID)
	sideFrame(t, win, r)
	n := len(st.ptys)
	toggle(t, win, r)
	if len(st.ptys) != n {
		t.Fatal("a scratch shell started over a member")
	}
}

func TestScratchHookCountsTowardItsSession(t *testing.T) {
	win, _, r, a, b, _, _ := paneWindow(t)
	toggle(t, win, r)
	s := scratchOf(win, a.ID)
	win.liveMgr.Store(win.mgr)
	win.selectTab(b.ID)
	sideFrame(t, win, r)
	win.onNotify(notify.Msg{Tab: int(s.ID), Kind: notify.KindDone})
	sideFrame(t, win, r)
	if win.alerts.markOf(a.ID) != markDone || win.alerts.markOf(s.ID) != markNone {
		t.Fatalf("marks: session %v scratch %v", win.alerts.markOf(a.ID), win.alerts.markOf(s.ID))
	}
	win.onNotify(notify.Msg{Tab: int(a.ID), Kind: notify.KindNeeds})
	sideFrame(t, win, r)
	if win.alerts.markOf(a.ID) != markNeeds {
		t.Fatalf("latest message did not win: %v", win.alerts.markOf(a.ID))
	}
}

func TestScratchUsageIsNotMoved(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	toggle(t, win, r)
	s := scratchOf(win, a.ID)
	got := hookTab(win.mgr, s.ID, notify.KindUsage)
	if got != s.ID || hookTab(win.mgr, s.ID, notify.KindDone) != a.ID || hookTab(win.mgr, a.ID, notify.KindDone) != a.ID {
		t.Fatal("hookTab")
	}
}

func TestScratchButtonInTheTitleBar(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	b := win.bar
	if b.Scratch.Empty() || b.Scratch.Max.X > b.Gear.Min.X {
		t.Fatalf("scratch %v gear %v", b.Scratch, b.Gear)
	}
	for _, d := range b.Drag {
		if b.Scratch.Overlaps(d) {
			t.Fatalf("scratch %v overlaps drag %v", b.Scratch, d)
		}
	}
	c := centre(b.Scratch)
	win.captionEvent(pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: at(c)})
	win.captionEvent(pointer.Event{Kind: pointer.Release, Position: at(c)})
	floatFrames(t, win, r)
	if scratchOf(win, a.ID) == nil {
		t.Fatal("the title bar button did not toggle a scratch shell")
	}
}

// macOS draws the same title bar, so its scratch button works the same way.
func TestScratchButtonOnMac(t *testing.T) {
	win, _ := sideWindow(t, false)
	a := openIn(t, win, "")
	win.wantFocus = true
	var r input.Router
	sideFrame(t, win, &r)
	click(win, win.bar.Scratch)
	floatFrames(t, win, &r)
	if scratchOf(win, a.ID) == nil {
		t.Fatal("the title bar button did not start a scratch shell on macOS")
	}
}
