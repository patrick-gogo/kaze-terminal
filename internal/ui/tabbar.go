package ui

import (
	"image"
	"image/color"
	"math"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

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
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

const chromeFace = "JetBrains Mono"

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
		data []byte
		name string
	}{{fonts.MonoRegular, chromeFace}, {fonts.CJK, "Noto Sans JP"}} {
		f, err := opentype.Parse(src.data)
		if err != nil {
			panic("ui: bundled font does not parse: " + err.Error()) // embedded at build time
		}
		faces = append(faces, font.FontFace{Font: font.Font{Typeface: font.Typeface(src.name)}, Face: f})
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

func place(ops *op.Ops, call op.CallOp, at image.Point) {
	t := op.Offset(at).Push(ops)
	call.Add(ops)
	t.Pop()
}

// layoutCaptions draws the sidebar toggle, the settings gear and the minimize,
// maximize/restore and close buttons.
func (c *chrome) layoutCaptions(gtx layout.Context, b barLayout, st captionState, maximized bool) {
	ops := gtx.Ops
	rects := [5]image.Rectangle{b.Buttons[0], b.Buttons[1], b.Buttons[2], b.Gear, b.Toggle}
	for i, r := range rects {
		if r.Empty() {
			continue
		}
		btn := captionButton(i + 1)
		fg := c.th.CapGlyph
		if bg, ok := c.captionBG(btn, st); ok {
			paint.FillShape(ops, bg, clip.Rect(r).Op())
			fg = c.th.ActiveFG
		}
		var glyph clip.Op
		if btn == capToggle {
			glyph = toggleGlyph(gtx, r)
		} else {
			glyph = captionGlyph(gtx, btn, r, maximized)
		}
		cl := clip.Rect(r).Push(ops)
		paint.FillShape(ops, fg, glyph)
		cl.Pop()
	}
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
	case btn == capGear:
		// A ring with eight teeth.
		cx, cy, ring := x0+s/2, y0+s/2, s*0.27
		p.MoveTo(f32.Pt(cx+ring, cy))
		p.Arc(f32.Pt(-ring, 0), f32.Pt(-ring, 0), 2*math.Pi)
		for i := range 8 {
			a := float64(i) * math.Pi / 4
			dx, dy := float32(math.Cos(a)), float32(math.Sin(a))
			p.MoveTo(f32.Pt(cx+dx*s*0.38, cy+dy*s*0.38))
			p.LineTo(f32.Pt(cx+dx*(s/2), cy+dy*(s/2)))
		}
	}
	return clip.Stroke{Path: p.End(), Width: sw}.Op()
}

// layoutStatus draws the status bar: folder and branch on the left, the rest on the right.
// It returns the server dots' and the update notice's rects in bar-local pixels,
// each empty when not drawn.
func (c *chrome) layoutStatus(gtx layout.Context, p statusParts, height int) (dots, upd image.Rectangle) {
	ops := gtx.Ops
	width := gtx.Constraints.Max.X
	pad := gtx.Dp(14)
	paint.FillShape(ops, c.th.Chrome, clip.Rect{Max: image.Pt(width, height)}.Op())
	paint.FillShape(ops, c.th.Border, clip.Rect{Max: image.Pt(width, 1)}.Op())
	call, size := c.label(gtx, p.Right, 11, c.th.StatusFG)
	place(ops, call, image.Pt(width-size.X-pad, (height-size.Y)/2))
	right := width - size.X - pad // where the right-hand text starts
	if ux := c.layoutUsage(gtx, p.Usage, right-gtx.Dp(16), height); ux >= 0 {
		right = ux
	}
	dots, dx := c.layoutServers(gtx, p.Servers, right-gtx.Dp(16), height)
	if dx >= 0 {
		right = dx
	}
	if p.Update != "" {
		call, size := c.label(gtx, p.Update, 11, c.th.Accent)
		if x := right - gtx.Dp(16) - size.X; x >= pad {
			place(ops, call, image.Pt(x, (height-size.Y)/2))
			upd = image.Rect(x-gtx.Dp(6), 0, right-gtx.Dp(10), height)
			right = x
		}
	}

	avail := right - 2*pad // room left of the right-hand text, with a gap
	x := pad
	var bcall op.CallOp
	var bsize image.Point
	gap := gtx.Dp(12)
	if p.Branch != "" {
		bcall, bsize = c.label(gtx, c.branchLabel(p.Branch), 11, c.th.Branch)
		avail -= bsize.X + gap
	}
	if p.Folder != "" && avail > 0 {
		folder := p.Folder
		fcall, fsize := c.label(gtx, folder, 11, c.th.Folder)
		// Shorten from the left until it fits; bounded so a bad measure cannot spin.
		for i := 0; i < 8 && fsize.X > avail && folder != "…"; i++ {
			n := utf8.RuneCountInString(folder) * avail / fsize.X
			if n >= utf8.RuneCountInString(folder) {
				n = utf8.RuneCountInString(folder) - 1
			}
			folder = truncateLeft(folder, n)
			fcall, fsize = c.label(gtx, folder, 11, c.th.Folder)
		}
		place(ops, fcall, image.Pt(x, (height-fsize.Y)/2))
		x += fsize.X + gap
	}
	if p.Branch != "" && x+bsize.X <= right-pad {
		place(ops, bcall, image.Pt(x, (height-bsize.Y)/2))
	}
	return dots, upd
}

// layoutUsage draws the usage segments ending at end and returns where they
// start, or -1 when there are none or they do not fit.
func (c *chrome) layoutUsage(gtx layout.Context, segs []usage.Segment, end, height int) int {
	if len(segs) == 0 {
		return -1
	}
	type piece struct {
		call op.CallOp
		size image.Point
	}
	var ps []piece
	w := 0
	for i, s := range segs {
		if i > 0 {
			call, size := c.label(gtx, "  ·  ", 11, c.th.StatusFG)
			ps = append(ps, piece{call, size})
			w += size.X
		}
		call, size := c.label(gtx, s.Text, 11, c.usageColor(s.Level))
		ps = append(ps, piece{call, size})
		w += size.X
	}
	x := end - w
	if x < gtx.Dp(14) {
		return -1
	}
	start := x
	for _, p := range ps {
		place(gtx.Ops, p.call, image.Pt(x, (height-p.size.Y)/2))
		x += p.size.X
	}
	return start
}

// layoutServers draws one dot per run member and the word "servers", ending at
// end. It returns the drawn rect and where it starts, or -1 when there are none
// or they do not fit.
func (c *chrome) layoutServers(gtx layout.Context, ds []serverDot, end, height int) (image.Rectangle, int) {
	if len(ds) == 0 {
		return image.Rectangle{}, -1
	}
	call, size := c.label(gtx, "servers", 11, c.th.StatusFG)
	d, gap := gtx.Dp(7), gtx.Dp(4)
	x := end - (len(ds)*(d+gap) + size.X)
	if x < gtx.Dp(14) {
		return image.Rectangle{}, -1
	}
	start := x
	y := (height - d) / 2
	for _, s := range ds {
		col := c.th.Muted
		switch {
		case s.Down:
			col = c.th.UsageHigh
		case s.Up:
			col = c.th.Done // the same green as the section's "all up"
		}
		paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rect(x, y, x+d, y+d), d/2).Op(gtx.Ops))
		x += d + gap
	}
	place(gtx.Ops, call, image.Pt(x, (height-size.Y)/2))
	return image.Rect(start, 0, end, height), start
}
