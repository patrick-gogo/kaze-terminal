package theme

import (
	"image/color"
	"math"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/render"
)

func hexc(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

func lum(c color.NRGBA) float64 {
	f := func(u uint8) float64 {
		v := float64(u) / 255
		if v <= 0.04045 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	return 0.2126*f(c.R) + 0.7152*f(c.G) + 0.0722*f(c.B)
}

func ratio(a, b color.NRGBA) float64 {
	la, lb := lum(a), lum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestMidnightMatchesTodaysColors(t *testing.T) {
	m := Default()
	if m.Term != render.DefaultPalette() {
		t.Fatal("midnight Term differs from render.DefaultPalette()")
	}
	if m.UI.Accent != hexc(0x8b7bff) || m.UI.Chrome != hexc(0x0b0d12) ||
		m.UI.Needs != hexc(0xf2b84b) || m.UI.UsageHigh != hexc(0xff6b6b) {
		t.Fatalf("midnight chrome drifted: %+v", m.UI)
	}
}

func TestAllThemes(t *testing.T) {
	want := []string{"midnight", "sumi", "tide", "washi", "sakura"}
	all := All()
	if len(all) != len(want) {
		t.Fatalf("got %d themes, want %d", len(all), len(want))
	}
	for i, th := range all {
		if th.Key != want[i] {
			t.Errorf("theme %d key = %q, want %q", i, th.Key, want[i])
		}
		if r := ratio(th.Term.FG, th.Term.BG); r < 7 {
			t.Errorf("%s FG/BG contrast %.2f < 7", th.Key, r)
		}
	}
	if s, ok := ByKey("sumi"); !ok || s.Tabs != Underline {
		t.Error("sumi should use Underline tabs")
	}
	if s, ok := ByKey("tide"); !ok || s.Tabs != Pill {
		t.Error("tide should use Pill tabs")
	}
	if _, ok := ByKey("nope"); ok {
		t.Error("ByKey(nope) should be false")
	}
	if w, _ := ByKey("washi"); lum(w.Term.BG) <= 0.8 {
		t.Error("washi BG should be light")
	}
}
