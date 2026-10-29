package render

import (
	"image"
	"image/color"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

// fakePTY is a shell that only produces what the test writes to it.
type fakePTY struct {
	outR *io.PipeReader
	outW *io.PipeWriter
	once sync.Once
	done chan struct{}
}

func newFakePTY() *fakePTY {
	r, w := io.Pipe()
	return &fakePTY{outR: r, outW: w, done: make(chan struct{})}
}

func (f *fakePTY) Read(b []byte) (int, error)  { return f.outR.Read(b) }
func (f *fakePTY) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakePTY) Resize(int, int) error       { return nil }
func (f *fakePTY) Wait() (int, error)          { <-f.done; return 0, nil }
func (f *fakePTY) Close() error {
	f.once.Do(func() { f.outW.Close(); close(f.done) })
	return nil
}

type harness struct {
	t       *testing.T
	pty     *fakePTY
	s       *term.Session
	changed chan struct{}
	r       *Renderer
}

func newHarness(t *testing.T, cols, rows int) *harness {
	t.Helper()
	h := &harness{t: t, pty: newFakePTY(), changed: make(chan struct{}, 64)}
	h.s = term.NewSession(h.pty, cols, rows, func() { h.changed <- struct{}{} })
	t.Cleanup(h.s.Close)
	h.r = NewRenderer(14, DefaultPalette(), Options{})
	return h
}

// out feeds shell output and waits until the session has applied it.
func (h *harness) out(s string) {
	h.t.Helper()
	if _, err := h.pty.outW.Write([]byte(s)); err != nil {
		h.t.Fatal(err)
	}
	select {
	case <-h.changed:
	case <-time.After(2 * time.Second):
		h.t.Fatal("output was not applied")
	}
}

// frame lays out one frame and returns how many rows it rasterized.
func (h *harness) frame() int {
	before := h.r.Stats().RowsRedrawn
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(2000, 2000)), Now: time.Now()}
	h.r.Layout(gtx, h.s, nil)
	return h.r.Stats().RowsRedrawn - before
}

func TestOnlyDirtyRowsAreRedrawn(t *testing.T) {
	h := newHarness(t, 80, 24)
	if n := h.frame(); n != 24 {
		t.Fatalf("first frame redrew %d rows, want 24", n)
	}
	h.out("a")
	if n := h.frame(); n != 1 {
		t.Fatalf("after one char, redrew %d rows, want 1", n)
	}
}

func TestNoChangeMeansNoRowsRedrawn(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.frame()
	if n := h.frame(); n != 0 {
		t.Fatalf("idle frame redrew %d rows, want 0", n)
	}
}

func TestOnlyDirtyStripsGetNewImage(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.frame()
	before := h.r.Stats().StripsUploaded
	h.out("a")
	h.frame()
	if d := h.r.Stats().StripsUploaded - before; d != 1 {
		t.Fatalf("one-cell change made %d new strip images, want 1", d)
	}
}

func TestGlyphRasterizedOnce(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.r = NewRenderer(14.25, DefaultPalette(), Options{}) // a size no other test uses, so its shared cache starts empty
	h.frame()
	for i := 0; i < 10; i++ {
		h.out(strings.Repeat("a", 100))
		h.frame()
	}
	if n := h.r.Stats().GlyphsCached; n != 1 {
		t.Fatalf("GlyphsCached = %d after 1000 'a', want 1", n)
	}
}

func TestSynchronizedOutputHoldsFrame(t *testing.T) {
	h := newHarness(t, 80, 24)
	var offset time.Duration
	h.r.now = func() time.Time { return time.Now().Add(offset) }
	h.frame()

	h.out("\x1b[?2026hx")
	if n := h.frame(); n != 0 {
		t.Fatalf("frame during sync redrew %d rows, want 0", n)
	}
	offset = 200 * time.Millisecond // past the 150ms hold, mode still set
	if n := h.frame(); n != 1 {
		t.Fatalf("frame after sync timeout redrew %d rows, want 1", n)
	}

	offset = 0
	h.out("\x1b[?2026l\x1b[?2026h\r\ny")
	if n := h.frame(); n != 0 {
		t.Fatalf("frame in a new sync redrew %d rows, want 0", n)
	}
	h.out("\x1b[?2026l")
	if n := h.frame(); n != 1 {
		t.Fatalf("frame after sync reset redrew %d rows, want 1", n)
	}
}

func TestJapaneseUsesFallbackFont(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	g := r.glyphs.get('日', false, true)
	if g.face != faceCJK {
		t.Fatalf("'日' came from face %d, want the CJK face", g.face)
	}
	if g.mask == nil || g.mask.Rect.Dx() != 2*r.cellW {
		t.Fatalf("'日' mask should span two cells")
	}
	if !hasInk(g.mask) {
		t.Fatal("'日' rasterized to an empty glyph")
	}
	if a := r.glyphs.get('a', false, false); a.face != faceMono {
		t.Fatalf("'a' came from face %d, want the mono face", a.face)
	}
}

func hasInk(m *image.Alpha) bool {
	for _, p := range m.Pix {
		if p != 0 {
			return true
		}
	}
	return false
}

func TestCellSizeComesFromMonoFont(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	w, h := r.CellSize()
	if w < 6 || w > 12 || h < w || h > 3*w {
		t.Fatalf("CellSize = %dx%d, implausible for 14px", w, h)
	}
}

// rasterized cell pixels: these check colors land where they should.

func rowsImage(r *Renderer, row int) *image.RGBA {
	s := r.strips[row/r.rowsPerStrip]
	return s.img
}

func pixelAt(r *Renderer, col, row, dx, dy int) [4]uint8 {
	img := rowsImage(r, row)
	y := (row%r.rowsPerStrip)*r.cellH + dy
	x := col*r.cellW + dx
	i := img.PixOffset(x, y)
	return [4]uint8{img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3]}
}

func toArr(c color.NRGBA) [4]uint8 { return [4]uint8{c.R, c.G, c.B, c.A} }

func TestBackgroundAndInverse(t *testing.T) {
	h := newHarness(t, 10, 2)
	h.out("\x1b[7m \x1b[0m\x1b[41m \x1b[0m")
	h.frame()
	p := DefaultPalette()
	want := func(c [4]uint8, col [4]uint8, what string) {
		t.Helper()
		if c != col {
			t.Fatalf("%s: got %v want %v", what, c, col)
		}
	}
	want(pixelAt(h.r, 0, 0, 0, 0), toArr(p.FG), "inverse blank shows FG as background")
	want(pixelAt(h.r, 1, 0, 0, 0), toArr(p.ANSI[1]), "red background")
	want(pixelAt(h.r, 5, 1, 0, 0), toArr(p.BG), "default background")
}

func TestBoldBrightensBasicColors(t *testing.T) {
	p := DefaultPalette()
	c := resolveFG(p, screen.Cell{FG: screen.Indexed(1), BG: screen.DefaultColor, Attrs: screen.AttrBold})
	if c != p.ANSI[9] {
		t.Fatalf("bold red = %v, want bright red %v", c, p.ANSI[9])
	}
	c = resolveFG(p, screen.Cell{FG: screen.Indexed(200), BG: screen.DefaultColor, Attrs: screen.AttrBold})
	if c != indexed(p, 200) {
		t.Fatal("bold must not change colors outside 0-7")
	}
}

func TestDimIsHalfwayToBackground(t *testing.T) {
	p := DefaultPalette()
	fg, bg := cellColors(p, screen.Cell{FG: screen.RGB(200, 100, 0), BG: screen.RGB(0, 0, 0), Attrs: screen.AttrDim})
	if fg.R != 100 || fg.G != 50 || fg.B != 0 || bg.R != 0 {
		t.Fatalf("dim fg = %v", fg)
	}
}

func TestIndexedCube(t *testing.T) {
	p := DefaultPalette()
	if c := indexed(p, 16); c.R != 0 || c.G != 0 || c.B != 0 {
		t.Fatalf("16 = %v", c)
	}
	if c := indexed(p, 231); c.R != 255 || c.G != 255 || c.B != 255 {
		t.Fatalf("231 = %v", c)
	}
	if c := indexed(p, 196); c.R != 255 || c.G != 0 || c.B != 0 {
		t.Fatalf("196 = %v", c)
	}
	if c := indexed(p, 232); c.R != 8 {
		t.Fatalf("232 = %v", c)
	}
	if c := indexed(p, 255); c.R != 238 {
		t.Fatalf("255 = %v", c)
	}
}

func TestCursorMoveDoesNotReuploadStrips(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.frame()
	before := h.r.Stats().StripsUploaded
	h.out("\x1b[10;10H") // cursor only, no cell changes
	h.frame()
	h.r.SetFocused(false)
	h.frame()
	if d := h.r.Stats().StripsUploaded - before; d != 0 {
		t.Fatalf("cursor-only changes made %d new strip images, want 0", d)
	}
}

func TestResizeRedrawsEverything(t *testing.T) {
	h := newHarness(t, 80, 24)
	h.frame()
	h.s.Resize(100, 30)
	if n := h.frame(); n != 30 {
		t.Fatalf("after resize redrew %d rows, want 30", n)
	}
	if len(h.r.strips) != stripCount {
		t.Fatalf("strips = %d, want %d", len(h.r.strips), stripCount)
	}
}

// frameSel is frame with a selection passed to Layout.
func (h *harness) frameSel(sel *Selection) int {
	before := h.r.Stats().RowsRedrawn
	gtx := layout.Context{Ops: new(op.Ops), Constraints: layout.Exact(image.Pt(2000, 2000)), Now: time.Now()}
	h.r.Layout(gtx, h.s, sel)
	return h.r.Stats().RowsRedrawn - before
}

func TestSelectedCellsUseSelectionColor(t *testing.T) {
	h := newHarness(t, 20, 6)
	h.frame()
	sel := &Selection{Start: Point{5, 1}, End: Point{8, 1}, Active: true}
	h.frameSel(sel)
	w, ch := h.r.CellSize()
	at := func(col, row int) color.RGBA { return h.r.pixelAt(col*w, row*ch) }
	want := h.r.pal.Selection
	if got := at(6, 1); got != (color.RGBA{want.R, want.G, want.B, 0xff}) {
		t.Fatalf("selected cell is %v, want selection color", got)
	}
	if got := at(10, 1); got == (color.RGBA{want.R, want.G, want.B, 0xff}) {
		t.Fatal("cell outside the selection was painted with the selection color")
	}
}

func TestSelectionChangeRedrawsOnlyAffectedRows(t *testing.T) {
	h := newHarness(t, 20, 6)
	h.frame()
	a := &Selection{Start: Point{0, 1}, End: Point{3, 2}, Active: true}
	if n := h.frameSel(a); n != 2 {
		t.Fatalf("selecting 2 rows redrew %d", n)
	}
	if n := h.frameSel(a); n != 0 {
		t.Fatalf("unchanged selection redrew %d rows", n)
	}
	b := &Selection{Start: Point{0, 1}, End: Point{3, 1}, Active: true} // row 2 leaves, row 1 loses its tail
	if n := h.frameSel(b); n != 2 {
		t.Fatalf("shrinking redrew %d rows, want 2", n)
	}
	if n := h.frameSel(nil); n != 1 {
		t.Fatalf("clearing redrew %d rows, want 1", n)
	}
}

func TestSelectionFollowsViewOffset(t *testing.T) {
	h := newHarness(t, 4, 2)
	h.out("aaaa\r\nbbbb\r\ncccc") // "aaaa" scrolls into history; rows: aaaa / bbbb / cccc
	h.frame()
	sel := &Selection{Start: Point{0, 0}, End: Point{3, 0}, Active: true} // the history row
	h.frameSel(sel)                                                       // offset 0: not visible
	h.s.Lock()
	h.s.Screen().SetViewOffset(1)
	h.s.Unlock()
	h.frameSel(sel)
	w, ch := h.r.CellSize()
	want := h.r.pal.Selection
	if got := h.r.pixelAt(w*3, 0); got != (color.RGBA{want.R, want.G, want.B, 0xff}) {
		t.Fatalf("history row not highlighted when scrolled into view: %v (cellH %d)", got, ch)
	}
}

// pixelAt reads a drawn pixel by window coordinates, across strips.
func (r *Renderer) pixelAt(x, y int) color.RGBA {
	st := r.strips[y/r.cellH/r.rowsPerStrip]
	return st.img.RGBAAt(x, y-st.row0*r.cellH)
}

func TestLineScaleChangesCellHeight(t *testing.T) {
	// 14.5 is used by no other test, so these font sets are built here.
	nw, nh := NewRenderer(14.5, DefaultPalette(), Options{LineScale: 1}).CellSize()
	tw, th := NewRenderer(14.5, DefaultPalette(), Options{LineScale: 0.92}).CellSize()
	rw, rh := NewRenderer(14.5, DefaultPalette(), Options{LineScale: 1.2}).CellSize()
	if !(th < nh && nh < rh) {
		t.Fatalf("cell heights tight %d normal %d roomy %d, want increasing", th, nh, rh)
	}
	if tw != nw || rw != nw {
		t.Fatalf("cell widths tight %d normal %d roomy %d, want equal", tw, nw, rw)
	}
	if zw, zh := NewRenderer(14.5, DefaultPalette(), Options{}).CellSize(); zw != nw || zh != nh {
		t.Fatalf("zero Options cell %dx%d, want normal %dx%d", zw, zh, nw, nh)
	}
}

func TestBoxDrawingFillsScaledCell(t *testing.T) {
	r := NewRenderer(14.5, DefaultPalette(), Options{LineScale: 1.2})
	g := r.glyphs.get('│', false, false)
	m := g.mask
	if m.Rect.Dy() != r.cellH {
		t.Fatalf("box mask %d tall, want the scaled cell %d", m.Rect.Dy(), r.cellH)
	}
	inked := func(y int) bool {
		for x := 0; x < m.Rect.Dx(); x++ {
			if m.AlphaAt(x, y).A != 0 {
				return true
			}
		}
		return false
	}
	if !inked(0) || !inked(r.cellH-1) {
		t.Fatal("│ at roomy does not reach the top and bottom rows, so lines would not join")
	}
}

// cursorHarness draws one frame of a focused 10x3 grid with the cursor at 3,1.
func cursorHarness(t *testing.T, o Options) *harness {
	t.Helper()
	h := newHarness(t, 10, 3)
	h.r = NewRenderer(14, DefaultPalette(), o)
	h.out("\x1b[2;4H")
	h.frame()
	return h
}

func TestBarAndUnderlineCursor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style CursorStyle
		want  func(cw, ch int) image.Rectangle
	}{
		{"bar", CursorBar, func(cw, ch int) image.Rectangle { return image.Rect(3*cw, ch, 3*cw+2, 2*ch) }},
		{"underline", CursorUnderline, func(cw, ch int) image.Rectangle { return image.Rect(3*cw, 2*ch-2, 4*cw, 2*ch) }},
	} {
		h := cursorHarness(t, Options{Cursor: tc.style})
		cw, ch := h.r.CellSize()
		if h.r.curPaint.image || h.r.curImg != nil {
			t.Fatalf("%s: the cursor used an image", tc.name)
		}
		got := h.r.edges[:h.r.curPaint.rects]
		if len(got) != 1 || got[0] != tc.want(cw, ch) {
			t.Fatalf("%s: painted %v, want [%v]", tc.name, got, tc.want(cw, ch))
		}
	}
	h := cursorHarness(t, Options{})
	if !h.r.curPaint.image || h.r.curPaint.rects != 0 {
		t.Fatalf("block cursor painted %+v, want its image", h.r.curPaint)
	}
}

func TestCursorHiddenPhase(t *testing.T) {
	for _, style := range []CursorStyle{CursorBlock, CursorBar, CursorUnderline} {
		h := cursorHarness(t, Options{Cursor: style})
		before := h.r.Stats().StripsUploaded
		h.r.SetCursorShown(false)
		h.frame()
		if p := h.r.curPaint; p.image || p.rects != 0 {
			t.Fatalf("style %d: hidden phase painted %+v", style, p)
		}
		h.r.SetCursorShown(true)
		h.frame()
		if p := h.r.curPaint; !p.image && p.rects == 0 {
			t.Fatalf("style %d: shown phase painted nothing", style)
		}
		if d := h.r.Stats().StripsUploaded - before; d != 0 {
			t.Fatalf("style %d: blinking made %d strip uploads, want 0", style, d)
		}
	}
}
