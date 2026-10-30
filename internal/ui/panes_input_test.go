package ui

import (
	"image"
	"io"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func at(p image.Point) f32.Point { return f32.Pt(float32(p.X), float32(p.Y)) }

// clickAt presses and releases the primary button at window point p.
func clickAt(t *testing.T, win *window, r *input.Router, p image.Point) {
	t.Helper()
	r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at(p)},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at(p)},
	)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

// dragFrom drags with the primary button from p by d in steps, one frame each.
func dragFrom(t *testing.T, win *window, r *input.Router, p, d image.Point) {
	t.Helper()
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at(p)})
	sideFrame(t, win, r)
	for i := 1; i <= 4; i++ {
		q := p.Add(d.Mul(i).Div(4))
		r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at(q)})
		sideFrame(t, win, r)
	}
	r.Queue(pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at(p.Add(d))})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

func typeKeys(t *testing.T, win *window, r *input.Router, s string) {
	t.Helper()
	r.Queue(key.EditEvent{Text: s})
	sideFrame(t, win, r)
}

// floatOne floats m1 over a and gives it the keyboard with a click on its body.
func floatOne(t *testing.T) (*window, *recStarter, *input.Router, *tabs.Tab, *tabs.Tab, *tabs.Tab, *pane) {
	t.Helper()
	win, st, r, a, b, m1, _ := paneWindow(t)
	p, _ := win.fl.open(a.ID, m1.ID, paneMember)
	floatFrames(t, win, r)
	clickAt(t, win, r, centre(p.hits.body))
	return win, st, r, a, b, m1, p
}

func emitTo(t *testing.T, st *recStarter, i int, tab *tabs.Tab, s string, done func() bool) {
	t.Helper()
	go ptyOf(st, i).w.Write([]byte(s))
	deadline := time.Now().Add(3 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("output %q never applied", s)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestClickingAPaneGivesItTheKeyboard(t *testing.T) {
	win, st, r, _, _, m1, p := floatOne(t)
	if win.fl.keys != m1.ID || win.focusTarget() != &p.body || !r.Source().Focused(&p.body) {
		t.Fatalf("keys %d, focus target %v", win.fl.keys, win.focusTarget())
	}
	typeKeys(t, win, r, "a")
	waitSent(t, ptyOf(st, 2), "a")
	if got := ptyOf(st, 0).sent(); got != "" {
		t.Fatalf("the session got %q", got)
	}
}

func TestClickingTheSessionTakesTheKeyboardBack(t *testing.T) {
	win, st, r, _, _, _, p := floatOne(t)
	outside := image.Pt(win.gridRect.Max.X-20, win.gridRect.Max.Y-20)
	if outside.In(p.hits.outer) {
		t.Fatal("test point is inside the pane")
	}
	clickAt(t, win, r, outside)
	if win.fl.keys != 0 || !r.Source().Focused(win.termTag) {
		t.Fatalf("keys %d after clicking the session", win.fl.keys)
	}
	typeKeys(t, win, r, "b")
	waitSent(t, ptyOf(st, 0), "b")
}

func TestEscGoesToThePaneProgram(t *testing.T) {
	win, st, r, _, _, m1, p := floatOne(t)
	r.Queue(key.Event{Name: key.NameEscape, State: key.Press})
	sideFrame(t, win, r)
	waitSent(t, ptyOf(st, 2), "\x1b")
	if win.fl.keys != m1.ID || p.hidden || p.hits.outer.Empty() {
		t.Fatal("Esc closed or left the pane")
	}
}

func TestPasteGoesToThePane(t *testing.T) {
	win, st, r, _, _, _, _ := floatOne(t)
	sc := shortcutFor(t, win, actPaste)
	r.Queue(sc)
	sideFrame(t, win, r)
	if !r.ClipboardRequested() {
		t.Fatal("no clipboard request")
	}
	r.Queue(transfer.DataEvent{Type: "application/text", Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("x")) }})
	sideFrame(t, win, r)
	waitSent(t, ptyOf(st, 2), "x")
	if got := ptyOf(st, 0).sent(); got != "" {
		t.Fatalf("the session got %q", got)
	}
}

func TestShortcutsWorkFromAPane(t *testing.T) {
	win, _, r, a, _, _, _ := floatOne(t)
	r.Queue(shortcutFor(t, win, actNewTab))
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if act := win.mgr.Active(); act == nil || act.ID == a.ID || win.fl.keys != 0 {
		t.Fatalf("active %v, keys %d", act, win.fl.keys)
	}
}

func TestCloseTabClosesTheSessionNotThePane(t *testing.T) {
	win, _, r, a, _, m1, _ := floatOne(t)
	r.Queue(shortcutFor(t, win, actCloseTab))
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if win.mgr.Has(a.ID) || !win.mgr.Has(m1.ID) {
		t.Fatalf("session open %v, member open %v", win.mgr.Has(a.ID), win.mgr.Has(m1.ID))
	}
}

func TestDraggingTheHeaderMovesThePane(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	p, _ := win.fl.open(a.ID, m1.ID, paneMember)
	floatFrames(t, win, r)
	from := p.r
	grip := image.Pt(p.hits.header.Min.X+60, p.hits.header.Min.Y+p.hits.header.Dy()/2)
	dragFrom(t, win, r, grip, image.Pt(50, 20))
	if p.r != from.Add(image.Pt(50, 20)) {
		t.Fatalf("r %v, want %v", p.r, from.Add(image.Pt(50, 20)))
	}
	if p.grab.on {
		t.Fatal("the grab outlived the release")
	}
}

func TestDraggingTheCornerResizesThePTY(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	p, _ := win.fl.open(a.ID, m1.ID, paneMember)
	floatFrames(t, win, r)
	before, sess := ptyOf(st, 2).lastSize(), ptyOf(st, 0).lastSize()
	from := p.r
	dragFrom(t, win, r, p.hits.outer.Max.Sub(image.Pt(3, 3)), image.Pt(100, 60))
	if p.r.Dx() != from.Dx()+100 || p.r.Dy() != from.Dy()+60 || p.r.Min != from.Min {
		t.Fatalf("r %v from %v", p.r, from)
	}
	if got := ptyOf(st, 2).lastSize(); got[0] <= before[0] || got[1] <= before[1] {
		t.Fatalf("pane pty %v, was %v", got, before)
	}
	if ptyOf(st, 0).lastSize() != sess {
		t.Fatal("the session resized")
	}
}

func TestResizeStopsAtTheMinimum(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	p, _ := win.fl.open(a.ID, m1.ID, paneMember)
	floatFrames(t, win, r)
	left := image.Pt(p.hits.outer.Min.X+2, p.hits.outer.Min.Y+p.hits.outer.Dy()/2)
	dragFrom(t, win, r, left, image.Pt(1000, 0))
	if p.r.Dx() != paneMinDp.X {
		t.Fatalf("width %d, want %d", p.r.Dx(), paneMinDp.X)
	}
}

func twoPanes(t *testing.T) (*window, *recStarter, *input.Router, *pane, *pane) {
	t.Helper()
	win, st, r, a, _, m1, m2 := paneWindow(t)
	back, _ := win.fl.open(a.ID, m1.ID, paneMember)
	front, _ := win.fl.open(a.ID, m2.ID, paneMember)
	floatFrames(t, win, r)
	return win, st, r, back, front
}

func TestClickRaisesTheBackPane(t *testing.T) {
	win, _, r, back, front := twoPanes(t)
	keep := front.r
	only := image.Pt(back.hits.body.Min.X+10, back.hits.body.Min.Y+10)
	if only.In(front.hits.outer) {
		t.Fatal("test point is under the front pane")
	}
	clickAt(t, win, r, only)
	if f := win.fl.set(win.mgr.Active().ID).front(); f != back || front.r != keep {
		t.Fatalf("front %+v", f)
	}
}

func TestOverlapHitsOnlyTheFrontPane(t *testing.T) {
	win, st, r, back, front := twoPanes(t)
	for i, id := range []tabs.TabID{back.id, front.id} {
		tab := win.mgr.Lookup(id)
		emitTo(t, st, 2+i, tab, "\x1b[?1000h\x1b[?1006h", func() bool { return mouseOn(modes(tab.Session)) })
	}
	sideFrame(t, win, r)
	overlap := front.hits.body.Intersect(back.hits.body)
	if overlap.Empty() {
		t.Fatal("the panes do not overlap")
	}
	clickAt(t, win, r, centre(overlap))
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(ptyOf(st, 3).sent(), "\x1b[<0;") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(ptyOf(st, 3).sent(), "\x1b[<0;") {
		t.Fatalf("the front pane got %q", ptyOf(st, 3).sent())
	}
	time.Sleep(30 * time.Millisecond)
	if got := ptyOf(st, 2).sent(); got != "" {
		t.Fatalf("the back pane got %q", got)
	}
}

func TestDragUnderAnotherPaneKeepsTheGesture(t *testing.T) {
	win, st, r, back, front := twoPanes(t)
	keep := front.r
	start := back.r
	grip := image.Pt(back.hits.header.Min.X+12, back.hits.header.Min.Y+back.hits.header.Dy()/2)
	if grip.In(front.hits.outer) {
		t.Fatal("grip is under the front pane")
	}
	dragFrom(t, win, r, grip, image.Pt(200, 120))
	if back.r != start.Add(image.Pt(200, 120)) || front.r != keep {
		t.Fatalf("back %v (from %v), front %v (was %v)", back.r, start, front.r, keep)
	}
	time.Sleep(30 * time.Millisecond)
	if got := ptyOf(st, 3).sent(); got != "" {
		t.Fatalf("the other pane got %q", got)
	}
}

func TestHoveringEdgesAsksForNoFrame(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	p, _ := win.fl.open(a.ID, m1.ID, paneMember)
	floatFrames(t, win, r)
	before := win.fl.redraws
	o := p.hits.outer
	for i := 0; i < 10; i++ {
		pts := []image.Point{{o.Min.X + 2, o.Min.Y + 40 + i}, {o.Min.X + 60 + i, o.Min.Y + 10}, {o.Max.X - 2, o.Min.Y + 50}}
		r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: at(pts[i%3])})
	}
	sideFrame(t, win, r)
	if win.fl.redraws != before {
		t.Fatalf("%d redraws from hovering", win.fl.redraws-before)
	}
	if at, ok := r.WakeupTime(); ok {
		t.Fatalf("hovering asked for a frame at %v", at)
	}
}

func TestFocusReportsFollowTheKeyboard(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	emitTo(t, st, 0, a, "\x1b[?1004h", func() bool { return modes(a.Session).FocusEvents })
	emitTo(t, st, 2, m1, "\x1b[?1004h", func() bool { return modes(m1.Session).FocusEvents })
	p, _ := win.fl.open(a.ID, m1.ID, paneMember)
	floatFrames(t, win, r)
	clickAt(t, win, r, centre(p.hits.body))
	waitSent(t, ptyOf(st, 0), "\x1b[O")
	waitSent(t, ptyOf(st, 2), "\x1b[I")
}

func TestSwitchingSessionsReturnsTheKeyboard(t *testing.T) {
	win, _, r, _, _, _, _ := floatOne(t)
	r.Queue(shortcutFor(t, win, actNextTab))
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if win.fl.keys != 0 || win.focusTarget() != win.termTag {
		t.Fatalf("keys %d, focus target %v", win.fl.keys, win.focusTarget())
	}
}
