package render

import (
	"image/color"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

func TestBlueBoldBackgroundTextIsReadable(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	// pwsh folder names: ESC[44;1m, default foreground on blue.
	fg, bg := r.colorsOf(screen.Cell{Rune: 'd', FG: screen.DefaultColor, BG: screen.Indexed(4), Attrs: screen.AttrBold})
	if bg != hex(0x8ab4ff) {
		t.Fatalf("bg = %v, the palette must not change", bg)
	}
	if c := contrast(fg, bg); c < 4.5 {
		t.Fatalf("fg %v on %v has contrast %.2f, want >= 4.5", fg, bg, c)
	}
}

func TestReadableColorsAreUnchanged(t *testing.T) {
	p := DefaultPalette()
	r := NewRenderer(14, p, Options{})
	if fg, bg := r.colorsOf(screen.Cell{Rune: 'a', FG: screen.DefaultColor, BG: screen.DefaultColor}); fg != p.FG || bg != p.BG {
		t.Fatalf("default text = %v on %v", fg, bg)
	}
	if fg, _ := r.colorsOf(screen.Cell{Rune: 'a', FG: screen.Indexed(1), BG: screen.DefaultColor}); fg != p.ANSI[1] {
		t.Fatalf("red on default = %v, want %v", fg, p.ANSI[1])
	}
	// Same fg and bg is deliberate (hidden text), so it stays hidden.
	same := screen.Cell{Rune: 'a', FG: screen.RGB(40, 40, 40), BG: screen.RGB(40, 40, 40)}
	if fg, bg := r.colorsOf(same); fg != bg {
		t.Fatalf("fg == bg cell was changed to %v on %v", fg, bg)
	}
}

func TestUnreadableDimTextIsFixed(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	cell := screen.Cell{Rune: 'a', FG: screen.RGB(80, 80, 90), BG: screen.DefaultColor, Attrs: screen.AttrDim}
	raw, _ := cellColors(r.pal, cell)
	fg, bg := r.colorsOf(cell)
	if contrast(raw, bg) >= 4.5 {
		t.Fatalf("test setup: raw dim contrast %.2f is already readable", contrast(raw, bg))
	}
	if c := contrast(fg, bg); c < 4.5 {
		t.Fatalf("dim fg %v on %v has contrast %.2f, want >= 4.5", fg, bg, c)
	}
}

func TestSelectedTextIsReadable(t *testing.T) {
	r := NewRenderer(14, DefaultPalette(), Options{})
	fg := r.selectedFG(screen.Cell{Rune: 'a', FG: screen.RGB(60, 60, 100), BG: screen.DefaultColor})
	if c := contrast(fg, r.pal.Selection); c < 4.5 {
		t.Fatalf("selected fg %v on selection has contrast %.2f, want >= 4.5", fg, c)
	}
	if fg := r.selectedFG(screen.Cell{Rune: 'a', FG: screen.DefaultColor, BG: screen.DefaultColor}); fg != r.pal.FG {
		t.Fatalf("default text on the selection changed to %v", fg)
	}
}

func TestContrastRatio(t *testing.T) {
	black, white := color.NRGBA{A: 0xff}, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	if c := contrast(black, white); c < 20.9 || c > 21.1 {
		t.Fatalf("black/white contrast = %.2f, want 21", c)
	}
	if c := contrast(white, white); c != 1 {
		t.Fatalf("white/white contrast = %.2f, want 1", c)
	}
}
