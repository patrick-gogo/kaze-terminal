package ui

import (
	"gioui.org/font"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"image"
	"strings"
	"testing"
)

func TestStatusGeometryCacheInvalidates(t *testing.T) {
	var cache statusLayoutCache
	m := busyModel()
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	shape := new(text.Shaper)
	calls := 0
	measure := func(s string, mono bool) int { calls++; return seven(s, mono) }
	get := func(w int) { cache.layout(m, w, statusTestDims, metric, shape, measure) }
	get(600)
	calls = 0
	get(600)
	if calls != 0 {
		t.Fatal("unchanged status reshaped")
	}
	changes := []func(){func() { m.Branch += "x" }, func() { m.Usage[0].Pct++ }, func() { m.Servers[0].Down = true }, func() { m.Notice = "failed" }, func() { metric.PxPerSp = 2 }, func() { shape = new(text.Shaper) }}
	for i, change := range changes {
		calls = 0
		change()
		get(600)
		if calls == 0 {
			t.Fatalf("change %d reused stale layout", i)
		}
	}
	calls = 0
	get(700)
	if calls == 0 {
		t.Fatal("width change reused stale layout")
	}
}

func TestTitleGeometryCacheInvalidates(t *testing.T) {
	var cache titleLayoutCache
	in := titleIn{Size: image.Pt(1200, 42), SideW: 248, Crumb: crumbParts{Title: "demo"}, Dims: titleDimsFor(pageContext(1200, 760))}
	calls := 0
	in.Measure = func(s string, mono bool) int { calls++; return seven(s, mono) }
	metric := unit.Metric{PxPerDp: 1, PxPerSp: 1}
	shape := new(text.Shaper)
	cache.layout(in, metric, shape)
	calls = 0
	cache.layout(in, metric, shape)
	if calls != 0 {
		t.Fatal("unchanged title reshaped")
	}
	for i, change := range []func(){func() { in.Key = "Cmd+K" }, func() { in.Crumb.Title = "changed" }, func() { in.SideW = 0 }, func() { in.LightsW = 70 }, func() { metric.PxPerSp = 2 }, func() { shape = new(text.Shaper) }} {
		change()
		calls = 0
		cache.layout(in, metric, shape)
		if calls == 0 {
			t.Fatalf("change %d reused stale title", i)
		}
	}
}

func TestChromeWidthMatchesDrawnLabels(t *testing.T) {
	c := newChrome()
	gtx := pageContext(1600, 760)
	for _, scale := range []float32{1, 1.5, 2} {
		gtx.Metric = unit.Metric{PxPerDp: scale, PxPerSp: scale}
		for _, s := range []string{"", "hello", "ends with spaces  ", "日本語", "café", "abc אבג"} {
			for _, mono := range []bool{false, true} {
				face := font.Font{Typeface: sansFace, Weight: font.SemiBold}
				if mono {
					face = font.Font{Typeface: chromeFace}
				}
				got := c.textWidth(gtx, s, 11, face)
				var size image.Point
				if mono {
					_, size = c.label(gtx, s, 11, c.th.Muted)
				} else {
					_, size = c.sans(gtx, s, 11, c.th.Muted, font.SemiBold)
				}
				if got != size.X {
					t.Errorf("%q mono=%v scale=%v width=%d drawn=%d", s, mono, scale, got, size.X)
				}
			}
		}
	}
}

func TestTruncationUsesBoundedMeasurements(t *testing.T) {
	s := strings.Repeat("日", 256)
	calls := 0
	measure := func(s string, mono bool) int { calls++; return seven(s, mono) }
	for _, cut := range []func(string, int, func(string, bool) int) string{cutLeft, cutRight} {
		calls = 0
		got := cut(s, 70, measure)
		if got == "" || seven(got, true) > 70 {
			t.Fatalf("bad truncated text: %q", got)
		}
		if calls > 12 {
			t.Fatalf("%d measurements for a 256-rune truncation", calls)
		}
	}
}

func BenchmarkStatusGeometryCached(b *testing.B) {
	c := newChrome()
	gtx := pageContext(600, 760)
	var ops op.Ops
	gtx.Ops = &ops
	m := busyModel()
	m.Folder = "~/" + strings.Repeat("a/", 120) + "todo-api"
	m.Branch = "feature/" + strings.Repeat("x", 120)
	var cache statusLayoutCache
	d := statusDimsFor(gtx)
	cache.layout(m, 600, d, gtx.Metric, c.shaper, c.statusMeasure(gtx))
	cache.font = c.shaper
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ops.Reset()
		cache.layout(m, 600, d, gtx.Metric, c.shaper, c.statusMeasure(gtx))
	}
}

// Optimized truncation must retain the previous measured fit, including
// fallback fonts and combining characters, rather than treating runes as cells.
func TestMeasuredTruncationMatchesLinear(t *testing.T) {
	c := newChrome()
	gtx := pageContext(800, 600)
	measure := c.statusMeasure(gtx)
	left := func(s string, w int) string {
		r := []rune(s)
		for i := range r {
			v := s
			if i > 0 {
				v = "…" + string(r[i:])
			}
			if measure(v, true) <= w {
				return v
			}
		}
		return ""
	}
	right := func(s string, w int) string {
		if measure(s, true) <= w {
			return s
		}
		r := []rune(s)
		for n := len(r) - 1; n > 0; n-- {
			v := string(r[:n]) + "…"
			if measure(v, true) <= w {
				return v
			}
		}
		return ""
	}
	for _, s := range []string{"feature/auth-token-refresh", strings.Repeat("日", 20), "e\u0301/combining/path", "abc אבג"} {
		for w := 0; w <= 240; w += 12 {
			if got, want := cutLeft(s, w, measure), left(s, w); got != want {
				t.Errorf("left %q at %d: %q != %q", s, w, got, want)
			}
			if got, want := cutRight(s, w, measure), right(s, w); got != want {
				t.Errorf("right %q at %d: %q != %q", s, w, got, want)
			}
		}
	}
}
