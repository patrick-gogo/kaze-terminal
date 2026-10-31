package ui

import (
	"image"
	"image/color"
	"os"
	"strings"
	"unicode"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/patrick-gogo/kaze-terminal/assets/fonts"
	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

const chromeFace = "JetBrains Mono"

// sansFace is the settings page's UI font.
const sansFace = "Manrope"

// chrome draws the title bar, sidebar and status bar with Gio's shaper over the bundled fonts.
type chrome struct {
	shaper    *text.Shaper
	toastHits []toastHit
	th        theme.Chrome
	ansi      [16]color.NRGBA // the terminal's ANSI colors, for workspace members

	faces   []font.FontFace
	covered map[rune]bool   // rune -> some face has it
	loaded  map[string]bool // system font files already added
}

func newChrome() *chrome {
	var faces []font.FontFace
	for _, src := range []struct {
		data   []byte
		name   string
		weight font.Weight
	}{
		{fonts.MonoRegular, chromeFace, font.Normal}, {fonts.CJK, "Noto Sans JP", font.Normal},
		{fonts.SansRegular, sansFace, font.Normal}, {fonts.SansMedium, sansFace, font.Medium},
		{fonts.SansSemiBold, sansFace, font.SemiBold},
	} {
		f, err := opentype.Parse(src.data)
		if err != nil {
			panic("ui: bundled font does not parse: " + err.Error()) // embedded at build time
		}
		faces = append(faces, font.FontFace{Font: font.Font{Typeface: font.Typeface(src.name), Weight: src.weight}, Face: f})
	}
	return &chrome{
		shaper:  text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces)),
		faces:   faces,
		covered: map[rune]bool{},
		loaded:  map[string]bool{},
		th:      theme.Default().UI,
		ansi:    theme.Default().Term.ANSI,
	}
}

// setTheme switches the colors the next frame draws with.
func (c *chrome) setTheme(t theme.Theme) {
	c.th, c.ansi = t.UI, t.Term.ANSI
}

// fit makes s drawable: for each rune the bundled fonts lack, it adds the system font
// the terminal grid would use (once, only now), and drops runes no font has so a title never shows a box.
func (c *chrome) fit(s string) string {
	added := false
	var out *strings.Builder // nil until a rune is changed or dropped
	for i, r := range s {
		keep, w := true, r
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			w = ' '
		case !c.usable(r, &added):
			keep = false
		}
		if out == nil && (!keep || w != r) {
			out = &strings.Builder{}
			out.WriteString(s[:i])
		}
		if out != nil && keep {
			out.WriteRune(w)
		}
	}
	if added {
		c.shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(c.faces))
	}
	if out == nil {
		return s
	}
	return out.String()
}

// maxCovered bounds the rune cache; a hostile title stream cannot grow it forever.
const maxCovered = 4096

// maxFontFile keeps big collections and emoji fonts out of memory; their runes are trimmed.
var maxFontFile int64 = 8 << 20

// usable reports whether r can be drawn, loading its system font on first need.
func (c *chrome) usable(r rune, added *bool) bool {
	if ok, seen := c.covered[r]; seen {
		return ok
	}
	ok := false
	// Cf covers bidi overrides and zero-width marks; Zl/Zp would break the one-line label.
	if !unicode.IsControl(r) && !unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
		ok = render.BundledCovers(r)
		if !ok {
			if p := render.SystemFontFor(r); p != "" {
				var fresh bool
				ok, fresh = c.addFont(p)
				*added = *added || fresh
			}
		}
	}
	if len(c.covered) >= maxCovered {
		clear(c.covered)
	}
	c.covered[r] = ok
	return ok
}

// addFont parses a system font file into the collection. fresh is true only when this call
// added faces; a bad or oversized file adds nothing and is not retried.
func (c *chrome) addFont(path string) (ok, fresh bool) {
	if done, seen := c.loaded[path]; seen {
		return done, false
	}
	defer func() {
		if recover() != nil {
			ok, fresh = false, false
		}
		c.loaded[path] = ok
	}()
	if st, err := os.Stat(path); err != nil || st.Size() > maxFontFile {
		return false, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}
	ff, err := opentype.ParseCollection(data)
	if err != nil || len(ff) == 0 {
		return false, false
	}
	c.faces = append(c.faces, ff...)
	return true, true
}

// label records one line of text so its size is known before it is placed.
func (c *chrome) label(gtx layout.Context, s string, size unit.Sp, col color.NRGBA) (op.CallOp, image.Point) {
	s = c.fit(s)
	m := op.Record(gtx.Ops)
	mat := op.Record(gtx.Ops)
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	matc := mat.Stop()
	lg := gtx
	lg.Constraints = layout.Constraints{Max: image.Pt(1<<16, 1<<16)}
	d := widget.Label{MaxLines: 1}.Layout(lg, c.shaper, font.Font{Typeface: chromeFace}, size, s, matc)
	return m.Stop(), d.Size
}

// sans is label in the settings page's UI font at weight.
func (c *chrome) sans(gtx layout.Context, s string, size unit.Sp, col color.NRGBA, weight font.Weight) (op.CallOp, image.Point) {
	s = c.fit(s)
	m := op.Record(gtx.Ops)
	mat := op.Record(gtx.Ops)
	paint.ColorOp{Color: col}.Add(gtx.Ops)
	matc := mat.Stop()
	lg := gtx
	lg.Constraints = layout.Constraints{Max: image.Pt(1<<16, 1<<16)}
	d := widget.Label{MaxLines: 1}.Layout(lg, c.shaper, font.Font{Typeface: sansFace, Weight: weight}, size, s, matc)
	return m.Stop(), d.Size
}

func place(ops *op.Ops, call op.CallOp, at image.Point) {
	t := op.Offset(at).Push(ops)
	call.Add(ops)
	t.Pop()
}

// captionBG is a button's background: pressed while held over it, hover when only hovered.
func (c *chrome) captionBG(btn captionButton, st captionState) (color.NRGBA, bool) {
	hover, press := c.th.CapHover, c.th.CapPress
	if btn == capClose {
		hover, press = c.th.CloseHover, c.th.ClosePress
	}
	switch {
	case st.pressed == btn && st.hover == btn:
		return press, true
	case st.pressed == capNone && st.hover == btn:
		return hover, true
	}
	return color.NRGBA{}, false
}

// captionGlyph is the 10dp line drawing for a button, centred in r. Lines sit half a
// stroke inside their box so a 1px stroke lands on whole pixels.
func captionGlyph(gtx layout.Context, btn captionButton, r image.Rectangle, maximized bool) clip.Op {
	sw := float32(max(gtx.Dp(1), 1))
	n := gtx.Dp(10)
	x0 := float32(r.Min.X + (r.Dx()-n)/2)
	y0 := float32(r.Min.Y + (r.Dy()-n)/2)
	s, h, d := float32(n), sw/2, float32(gtx.Dp(2))
	var p clip.Path
	p.Begin(gtx.Ops)
	box := func(x1, y1, x2, y2 float32) {
		p.MoveTo(f32.Pt(x1+h, y1+h))
		p.LineTo(f32.Pt(x2-h, y1+h))
		p.LineTo(f32.Pt(x2-h, y2-h))
		p.LineTo(f32.Pt(x1+h, y2-h))
		p.Close()
	}
	switch {
	case btn == capMinimize:
		y := y0 + float32(n/2) + h
		p.MoveTo(f32.Pt(x0, y))
		p.LineTo(f32.Pt(x0+s, y))
	case btn == capMaximize && !maximized:
		box(x0, y0, x0+s, y0+s)
	case btn == capMaximize:
		box(x0, y0+d, x0+s-d, y0+s) // front window
		// Back window: only its top and right edges show.
		p.MoveTo(f32.Pt(x0+d+h, y0+d))
		p.LineTo(f32.Pt(x0+d+h, y0+h))
		p.LineTo(f32.Pt(x0+s-h, y0+h))
		p.LineTo(f32.Pt(x0+s-h, y0+s-d-h))
		p.LineTo(f32.Pt(x0+s-d, y0+s-d-h))
	case btn == capClose:
		p.MoveTo(f32.Pt(x0, y0))
		p.LineTo(f32.Pt(x0+s, y0+s))
		p.MoveTo(f32.Pt(x0+s, y0))
		p.LineTo(f32.Pt(x0, y0+s))
	}
	return clip.Stroke{Path: p.End(), Width: sw}.Op()
}
