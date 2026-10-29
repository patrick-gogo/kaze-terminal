package ui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

// ctrlKind is one of the settings page's control types.
type ctrlKind int

const (
	ctlButton ctrlKind = iota
	ctlToggle
	ctlSegmented
	ctlStepper
	ctlSwatch
)

// ctrl is a control a section asks for. activate gets its id and an arg that
// depends on the kind: a button or swatch passes arg, a toggle its new value
// (1 on, 0 off), a segmented control the chosen option, a stepper the new value.
type ctrl struct {
	kind            ctrlKind
	id              string
	text            string // button label
	strong          bool   // button filled with the accent
	on              bool   // toggle
	options         []string
	selected        int // segmented
	min, max, value int // stepper
	theme           theme.Theme
	current         bool // swatch: the theme in use, ringed
	arg             int  // button, swatch
}

// control is one clickable area laid out this frame, in window pixels.
type control struct {
	kind ctrlKind
	rect image.Rectangle
	id   string
	arg  int
	dead bool // a stepper end already at its limit: drawn dim, does nothing
}

// shift moves the hit areas [from, to) from a macro's origin to where it was placed.
func (s *settingsState) shift(from, to int, at image.Point) {
	for i := from; i < to; i++ {
		s.controls[i].rect = s.controls[i].rect.Add(at)
	}
}

// ctrl records k drawn at the origin, w wide for a swatch, and returns it with its
// size and the index of its first hit area.
func (p *page) ctrl(k ctrl, w int) (op.CallOp, image.Point, int) {
	from := len(p.st.controls)
	m := op.Record(p.gtx.Ops)
	var size image.Point
	switch k.kind {
	case ctlButton:
		size = p.button(k)
	case ctlToggle:
		size = p.toggle(k)
	case ctlSegmented:
		size = p.segmented(k)
	case ctlStepper:
		size = p.stepper(k)
	case ctlSwatch:
		size = p.swatch(k, w)
	}
	return m.Stop(), size, from
}

// hit records a clickable area; the focused one gets a 1px accent ring.
func (p *page) hit(kind ctrlKind, r image.Rectangle, id string, arg int, dead bool) {
	if len(p.st.controls) == p.st.focus {
		g := p.gtx.Dp(2)
		stroke(p.gtx.Ops, r.Inset(-g), p.gtx.Dp(9), p.col.accent)
	}
	p.st.controls = append(p.st.controls, control{kind: kind, rect: r, id: id, arg: arg, dead: dead})
}

// stroke draws a 1px rounded outline.
func stroke(ops *op.Ops, r image.Rectangle, radius int, col color.NRGBA) {
	paint.FillShape(ops, col, clip.Stroke{Path: clip.UniformRRect(r, radius).Path(ops), Width: 1}.Op())
}

func (p *page) button(k ctrl) image.Point {
	gtx, ops := p.gtx, p.gtx.Ops
	fg := p.col.fg
	if k.strong {
		fg = p.col.bg
	}
	call, size := p.text(k.text, 12, fg)
	r := image.Rect(0, 0, size.X+2*gtx.Dp(12), gtx.Dp(24))
	if k.strong {
		paint.FillShape(ops, p.col.accent, clip.UniformRRect(r, gtx.Dp(7)).Op(ops))
	} else {
		stroke(ops, r, gtx.Dp(7), p.col.line)
	}
	place(ops, call, image.Pt((r.Dx()-size.X)/2, (r.Dy()-size.Y)/2))
	p.hit(ctlButton, r, k.id, k.arg, false)
	return r.Size()
}

func (p *page) toggle(k ctrl) image.Point {
	gtx, ops := p.gtx, p.gtx.Ops
	w, h, d, in := gtx.Dp(34), gtx.Dp(19), gtx.Dp(13), gtx.Dp(3)
	r := image.Rect(0, 0, w, h)
	track, knob, x, arg := p.col.line, p.col.muted, in, 1
	if k.on {
		track, knob, x, arg = p.col.accent, p.col.bg, w-in-d, 0
	}
	paint.FillShape(ops, track, clip.UniformRRect(r, h/2).Op(ops))
	y := (h - d) / 2
	paint.FillShape(ops, knob, clip.Ellipse(image.Rect(x, y, x+d, y+d)).Op(ops))
	p.hit(ctlToggle, r, k.id, arg, false)
	return r.Size()
}

func (p *page) segmented(k ctrl) image.Point {
	gtx, ops := p.gtx, p.gtx.Ops
	padX, h := gtx.Dp(11), gtx.Dp(24)
	type opt struct {
		call op.CallOp
		size image.Point
		r    image.Rectangle
	}
	opts := make([]opt, 0, len(k.options))
	x := 0
	for _, o := range k.options {
		call, size := p.text(o, 12, p.col.fg)
		r := image.Rect(x, 0, x+size.X+2*padX, h)
		opts = append(opts, opt{call, size, r})
		x = r.Max.X
	}
	outer := image.Rect(0, 0, x, h)
	rr := clip.UniformRRect(outer, gtx.Dp(7))
	if k.selected >= 0 && k.selected < len(opts) {
		cl := rr.Push(ops)
		paint.FillShape(ops, tint(p.col.accent, 0x33), clip.Rect(opts[k.selected].r).Op())
		cl.Pop()
	}
	stroke(ops, outer, gtx.Dp(7), p.col.line)
	for i, o := range opts {
		place(ops, o.call, image.Pt(o.r.Min.X+padX, (h-o.size.Y)/2))
		p.hit(ctlSegmented, o.r, k.id, i, false)
	}
	return outer.Size()
}

func (p *page) stepper(k ctrl) image.Point {
	gtx, ops := p.gtx, p.gtx.Ops
	cell, h := gtx.Dp(24), gtx.Dp(24)
	vc, vs := p.text(strconv.Itoa(k.value), 12, p.col.fg)
	valW := vs.X + gtx.Dp(16)
	outer := image.Rect(0, 0, 2*cell+valW, h)
	stroke(ops, outer, gtx.Dp(7), p.col.line)
	place(ops, vc, image.Pt(cell+(valW-vs.X)/2, (h-vs.Y)/2))
	ends := []struct {
		glyph string
		r     image.Rectangle
		to    int
		dead  bool
	}{
		{"−", image.Rect(0, 0, cell, h), k.value - 1, k.value <= k.min},
		{"+", image.Rect(cell+valW, 0, outer.Max.X, h), k.value + 1, k.value >= k.max},
	}
	for _, e := range ends {
		fg := p.col.fg
		if e.dead {
			fg = p.col.dim
		}
		call, size := p.text(e.glyph, 12, fg)
		place(ops, call, e.r.Min.Add(image.Pt((cell-size.X)/2, (h-size.Y)/2)))
		p.hit(ctlStepper, e.r, k.id, e.to, e.dead)
	}
	return outer.Size()
}

// swatch draws a theme in its own colors: its name, five sample colors and its accent.
func (p *page) swatch(k ctrl, w int) image.Point {
	gtx, ops, t := p.gtx, p.gtx.Ops, k.theme
	pad, chip, gap := gtx.Dp(8), gtx.Dp(12), gtx.Dp(3)
	call, size := p.text(t.Name, 11, t.Term.FG)
	r := image.Rect(0, 0, w, pad+size.Y+gtx.Dp(6)+chip+pad)
	paint.FillShape(ops, t.Term.BG, clip.UniformRRect(r, gtx.Dp(8)).Op(ops))
	if k.current {
		paint.FillShape(ops, p.col.accent, clip.Stroke{Path: clip.UniformRRect(r, gtx.Dp(8)).Path(ops), Width: float32(gtx.Dp(2))}.Op())
	} else {
		stroke(ops, r, gtx.Dp(8), t.UI.Border)
	}
	place(ops, call, image.Pt(pad, pad))
	colors := []color.NRGBA{t.Term.ANSI[1], t.Term.ANSI[2], t.Term.ANSI[3], t.Term.ANSI[4], t.Term.ANSI[5], t.UI.Accent}
	x, y := pad, pad+size.Y+gtx.Dp(6)
	for _, c := range colors {
		if x+chip > w-pad {
			break
		}
		paint.FillShape(ops, c, clip.UniformRRect(image.Rect(x, y, x+chip, y+chip), gtx.Dp(3)).Op(ops))
		x += chip + gap
	}
	p.hit(ctlSwatch, r, k.id, k.arg, false)
	return r.Size()
}
