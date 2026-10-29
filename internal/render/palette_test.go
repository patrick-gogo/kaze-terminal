package render

import "testing"

func TestReadablePullsTowardThemeInk(t *testing.T) {
	fg, bg, ink := hex(0xd8d2c4), hex(0xf6f2ea), hex(0x2b2a33)
	got := readable(fg, bg, ink)
	if c := contrast(got, bg); c < 4.5 {
		t.Fatalf("%v on %v has contrast %.2f, want >= 4.5", got, bg, c)
	}
	if luminance(got) >= luminance(fg) {
		t.Fatalf("%v is not darker than %v", got, fg)
	}
}

func TestRendererInkIsTheDarkerDefault(t *testing.T) {
	if r := NewRenderer(14, DefaultPalette(), Options{}); r.ink != hex(0x0f1117) {
		t.Fatalf("Midnight ink = %v, want the background #0f1117", r.ink)
	}
	light := DefaultPalette()
	light.FG, light.BG = hex(0x2b2a33), hex(0xf6f2ea)
	if r := NewRenderer(14, light, Options{}); r.ink != light.FG {
		t.Fatalf("light ink = %v, want the foreground %v", r.ink, light.FG)
	}
}
