package render

import (
	"image"
	"testing"
)

func TestMissingRunesShareOneNotdef(t *testing.T) {
	c := NewRenderer(17, DefaultPalette(), Options{}).glyphs
	for i := 0; i < 20000; i++ {
		c.get(0xF0000+rune(i), i%2 == 0, false) // supplementary private use: in neither font
	}
	if n := c.len(); n > glyphCap {
		t.Fatalf("cache holds %d masks, cap is %d", n, glyphCap)
	}
	notdef := 0
	for _, g := range c.m {
		if g.face == faceMissing {
			notdef++
		}
	}
	if notdef != 1 {
		t.Fatalf("%d .notdef masks, want 1", notdef)
	}
	if a, b := c.get(0xF0001, false, false), c.get(0xF0002, true, false); a != b {
		t.Fatal("two missing runes got different masks")
	}
}

func TestGlyphCacheIsBounded(t *testing.T) {
	c := NewRenderer(18, DefaultPalette(), Options{}).glyphs
	for r := rune(0x4E00); r < 0x4E00+6000; r++ { // mostly present in the CJK subset
		c.get(r, false, true)
		if c.len() > glyphCap {
			t.Fatalf("cache grew to %d past cap %d", c.len(), glyphCap)
		}
	}
}

func TestCJKBoldSharesMask(t *testing.T) {
	c := NewRenderer(14, DefaultPalette(), Options{}).glyphs
	if c.get('日', true, true) != c.get('日', false, true) {
		t.Fatal("bold CJK made its own mask, but there is no CJK bold face")
	}
}

func TestRenderersShareFontsAndGlyphs(t *testing.T) {
	a, b := NewRenderer(19, DefaultPalette(), Options{}), NewRenderer(19, DefaultPalette(), Options{})
	if a.glyphs != b.glyphs || a.fs != b.fs {
		t.Fatal("two renderers of the same size have their own glyph cache or fonts")
	}
	if NewRenderer(20, DefaultPalette(), Options{}).glyphs == a.glyphs {
		t.Fatal("different sizes must not share a cache")
	}
}

func inkAt(m *image.Alpha, x, y int) bool { return m.AlphaAt(x, y).A != 0 }

func TestBoxLinesReachCellEdges(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	w, h := r.CellSize()
	v := r.glyphs.get('│', false, false).mask
	if !inkAt(v, w/2, 0) && !inkAt(v, w/2-1, 0) || !inkAt(v, w/2, h-1) && !inkAt(v, w/2-1, h-1) {
		t.Fatal("│ does not reach the top and bottom rows")
	}
	hz := r.glyphs.get('─', false, false).mask
	row := -1
	for y := 0; y < h; y++ {
		if inkAt(hz, 0, y) && inkAt(hz, w-1, y) {
			row = y
		}
	}
	if row < 0 {
		t.Fatal("─ does not reach the left and right columns")
	}
	for _, ch := range []rune{'═', '━', '┼', '╋', '╬', '┄'} {
		m := r.glyphs.get(ch, false, false).mask
		left, right := false, false
		for y := 0; y < h; y++ {
			left = left || inkAt(m, 0, y)
			right = right || inkAt(m, w-1, y)
		}
		if !left || !right {
			t.Errorf("%c does not reach both sides", ch)
		}
	}
}

func TestRoundedCornerConnectsRightAndDown(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	w, h := r.CellSize()
	m := r.glyphs.get('╭', false, false).mask
	right, down, top, left := false, false, false, false
	for y := 0; y < h; y++ {
		right = right || inkAt(m, w-1, y)
		left = left || inkAt(m, 0, y)
	}
	for x := 0; x < w; x++ {
		down = down || inkAt(m, x, h-1)
		top = top || inkAt(m, x, 0)
	}
	if !right || !down || top || left {
		t.Fatalf("╭ edges: right=%v down=%v top=%v left=%v", right, down, top, left)
	}
	// The horizontal arm must line up with ─ so they join.
	hz := r.glyphs.get('─', false, false).mask
	for y := 0; y < h; y++ {
		if inkAt(hz, w-1, y) != inkAt(m, w-1, y) {
			t.Fatalf("╭ right arm at row %d does not line up with ─", y)
		}
	}
}

func TestBlockElements(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	w, h := r.CellSize()
	full := r.glyphs.get('█', false, false).mask
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if full.AlphaAt(x, y).A != 0xff {
				t.Fatalf("█ not solid at %d,%d", x, y)
			}
		}
	}
	lower := r.glyphs.get('▄', false, false).mask
	if inkAt(lower, 0, 0) || !inkAt(lower, 0, h-1) || !inkAt(lower, w-1, h-1) {
		t.Fatal("▄ should fill only the lower half")
	}
	if a := r.glyphs.get('▒', false, false).mask.AlphaAt(0, 0).A; a == 0 || a == 0xff {
		t.Fatalf("▒ coverage = %d, want partial", a)
	}
}

func TestBoxLinesAreSolidPixelStrokes(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	w, h := r.CellSize()
	for _, ch := range []rune{'│', '─', '┼', '═', '║', '┏', '╋', '▌'} {
		m := r.glyphs.get(ch, false, false).mask
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if a := m.AlphaAt(x, y).A; a != 0 && a != 0xff {
					t.Fatalf("%c has a partial pixel (%d) at %d,%d: strokes must be whole pixels", ch, a, x, y)
				}
			}
		}
	}
	light, heavy := r.glyphs.get('│', false, false).mask, r.glyphs.get('┃', false, false).mask
	count := func(m *image.Alpha) (n int) {
		for x := 0; x < w; x++ {
			if inkAt(m, x, h/2) {
				n++
			}
		}
		return n
	}
	if count(heavy) <= count(light) {
		t.Fatalf("heavy line (%dpx) is not thicker than light (%dpx)", count(heavy), count(light))
	}
}
