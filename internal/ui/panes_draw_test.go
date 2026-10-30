package ui

import (
	"image"
	"testing"

	"gioui.org/io/input"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

// paneWindow has session a shown (pty 0), session b (pty 1) and two workspace
// members m1, m2 (ptys 2, 3); nothing floats yet.
func paneWindow(t *testing.T) (win *window, st *recStarter, r *input.Router, a, b, m1, m2 *tabs.Tab) {
	t.Helper()
	win, st = sideWindow(t, true)
	a = openIn(t, win, osPath("/code/alpha/a"))
	b = openIn(t, win, osPath("/code/beta/b"))
	m1 = openMember(t, win, "api")
	m2 = openMember(t, win, "web")
	win.mgr.SetActive(a.ID)
	win.wantFocus = true
	r = new(input.Router)
	sideFrame(t, win, r)
	return
}

// floatFrames draws twice after panes change: the first frame draws a new
// pane, the next one registers its input filters, as the window's own wake does.
func floatFrames(t *testing.T, win *window, r *input.Router) {
	t.Helper()
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

func ptyOf(st *recStarter, i int) *recPTY {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.ptys[i]
}

func paneOf(t *testing.T, win *window, id tabs.TabID) *pane {
	t.Helper()
	for _, s := range win.fl.sets {
		if p := s.find(id); p != nil {
			return p
		}
	}
	t.Fatalf("no pane for tab %d", id)
	return nil
}

// paneGrid is the PTY size a pane of r dp gets at scale 1.
func paneGrid(win *window, r image.Rectangle) [2]int {
	cw, ch := win.rend.CellSize()
	c, rows := gridSize(r.Dx()-2-16, r.Dy()-2-paneHeaderDp-8, cw, ch)
	return [2]int{c, rows}
}

func TestEachVisiblePaneHasItsOwnRenderer(t *testing.T) {
	win, _, r, a, _, m1, m2 := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	win.fl.open(a.ID, m2.ID, paneMember)
	sideFrame(t, win, r)
	r1, r2 := win.fl.rends[m1.ID], win.fl.rends[m2.ID]
	if len(win.fl.rends) != 2 || r1 == nil || r2 == nil || r1 == r2 || r1 == win.rend || r2 == win.rend {
		t.Fatalf("renderers %v (main %p)", win.fl.rends, win.rend)
	}
}

func TestHiddenPanesDropTheirRenderers(t *testing.T) {
	win, _, r, a, b, m1, m2 := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	win.fl.open(a.ID, m2.ID, paneMember)
	sideFrame(t, win, r)
	before := paneOf(t, win, m1.ID).hits.outer
	win.selectTab(b.ID)
	sideFrame(t, win, r)
	if len(win.fl.rends) != 0 || len(win.fl.shown) != 0 {
		t.Fatalf("over b: renderers %d, shown %v", len(win.fl.rends), win.fl.shown)
	}
	win.selectTab(a.ID)
	sideFrame(t, win, r)
	if len(win.fl.rends) != 2 || paneOf(t, win, m1.ID).hits.outer != before {
		t.Fatalf("back on a: renderers %d, outer %v want %v", len(win.fl.rends), paneOf(t, win, m1.ID).hits.outer, before)
	}
}

func TestPanePTYGetsThePaneGrid(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	sideFrame(t, win, r)
	want := paneGrid(win, image.Rectangle{Max: memberSizeDp})
	if got := ptyOf(st, 2).lastSize(); got != want {
		t.Fatalf("pane pty %v, want %v", got, want)
	}
	cols, rows := win.mgr.Size()
	if got := ptyOf(st, 0).lastSize(); got != [2]int{cols, rows} {
		t.Fatalf("session pty %v, want the main size %v", got, [2]int{cols, rows})
	}
}

func TestWindowResizeLeavesFittingPanes(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	sideFrame(t, win, r)
	pane, sess := ptyOf(st, 2).lastSize(), ptyOf(st, 0).lastSize()
	sizedFrame(t, win, r, 1200, 700)
	if ptyOf(st, 0).lastSize() == sess {
		t.Fatal("the session did not resize with the window")
	}
	if got := ptyOf(st, 2).lastSize(); got != pane {
		t.Fatalf("pane pty %v, want %v", got, pane)
	}
}

func TestShrinkingTheWindowFitsPanes(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	p, _ := win.fl.open(a.ID, m1.ID, paneMember)
	p.r = image.Rect(500, 300, 1060, 560)
	stored := p.r
	sideFrame(t, win, r)
	big := ptyOf(st, 2).lastSize()
	sizedFrame(t, win, r, 700, 400)
	term := image.Rect(win.sideWidth(pageContext(700, 400), 700), 38, 700, 400-24)
	if !p.hits.outer.In(term) {
		t.Fatalf("pane %v outside the terminal area %v", p.hits.outer, term)
	}
	if got := ptyOf(st, 2).lastSize(); got[0] >= big[0] {
		t.Fatalf("pane pty %v did not shrink from %v", got, big)
	}
	sideFrame(t, win, r)
	if p.r != stored || ptyOf(st, 2).lastSize() != big {
		t.Fatalf("after growing back: r %v pty %v, want %v %v", p.r, ptyOf(st, 2).lastSize(), stored, big)
	}
}

func TestHidingTheSidebarMovesPanesWithTheTerminal(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	sideFrame(t, win, r)
	x := paneOf(t, win, m1.ID).hits.outer.Min.X
	win.toggleSidebar()
	sideFrame(t, win, r)
	if got := paneOf(t, win, m1.ID).hits.outer.Min.X; got != x-sidebarW {
		t.Fatalf("pane at x %d, want %d", got, x-sidebarW)
	}
}

func TestExpandedMemberLeavesEverySet(t *testing.T) {
	win, st, r, a, _, m1, _ := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	sideFrame(t, win, r)
	win.selectTab(m1.ID)
	sideFrame(t, win, r)
	cols, rows := win.mgr.Size()
	if win.fl.owner(m1.ID) != 0 || ptyOf(st, 2).lastSize() != [2]int{cols, rows} || len(win.fl.rends) != 0 {
		t.Fatalf("owner %d, pty %v, renderers %d", win.fl.owner(m1.ID), ptyOf(st, 2).lastSize(), len(win.fl.rends))
	}
}

func TestNoPanesOverSettings(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	sideFrame(t, win, r)
	win.openSettings()
	sideFrame(t, win, r)
	if len(win.fl.rends) != 0 || len(win.fl.shown) != 0 || !paneOf(t, win, m1.ID).hits.outer.Empty() {
		t.Fatalf("panes over settings: renderers %d shown %v", len(win.fl.rends), win.fl.shown)
	}
}

func TestFrontPaneDrawsTheAccentBorder(t *testing.T) {
	c := newChrome()
	if c.paneBorder(true) != c.th.Accent || c.paneBorder(false) != c.th.Border {
		t.Fatal("border colors")
	}
	win, _, r, a, _, m1, m2 := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	win.fl.open(a.ID, m2.ID, paneMember)
	sideFrame(t, win, r)
	if f := win.fl.set(a.ID).front(); f == nil || f.id != m2.ID {
		t.Fatalf("front %+v", f)
	}
}

func TestPanesDrawInEveryTheme(t *testing.T) {
	win, _, r, a, _, m1, _ := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	for _, th := range theme.All() {
		win.applyTheme(th)
		sideFrame(t, win, r)
		if th.UI.Chrome == win.pal.BG {
			t.Errorf("%s: the header would not stand out from the body", th.Key)
		}
		if paneOf(t, win, m1.ID).hits.header.Empty() {
			t.Errorf("%s: no header drawn", th.Key)
		}
	}
}

func TestIdlePanesAskForNoFrame(t *testing.T) {
	win, _, r, a, _, m1, m2 := paneWindow(t)
	win.fl.open(a.ID, m1.ID, paneMember)
	win.fl.open(a.ID, m2.ID, paneMember)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if at, ok := r.WakeupTime(); ok {
		t.Fatalf("idle panes asked for a frame at %v", at)
	}
}
