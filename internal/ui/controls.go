package ui

import (
	"image"
	"image/color"
	"strconv"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	kinput "github.com/patrick-gogo/kaze-terminal/internal/input"
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
	ctlText
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

	// Text field: val is its value, placed in the editor when the field first shows
	// and again whenever the row passes a different one; set gets every edit.
	val, placeholder string
	err              string // a red line under the field; "" for none
	width            int    // in dp; 0 is fieldWidth
	set              func(string)
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
	case ctlText:
		size = p.textField(k)
	}
	return m.Stop(), size, from
}

// hit records a clickable area; the focused one gets a 1px accent ring.
func (p *page) hit(kind ctrlKind, r image.Rectangle, id string, arg int, dead bool) {
	// A field with the keyboard shows it with its own accent border instead.
	if len(p.st.controls) == p.st.focus && (kind != ctlText || id != p.st.editing) {
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

// fieldWidth is a text field's width in dp when its row does not set one.
const fieldWidth = 240

// textField is one text control's editor. It lives in settingsState.fields by id,
// so its text, caret and scroll survive frames and theme switches.
type textField struct {
	ed      widget.Editor
	given   string // the val the row passed last
	set     func(string)
	had     bool // had the keyboard last frame, to notice a click giving it
	seen    bool // laid out this frame; fields not seen are dropped
	filters []event.Filter
}

// textField draws a field: a filled box with a single-line editor that scrolls
// sideways, and its error line under it.
func (p *page) textField(k ctrl) image.Point {
	gtx, ops, col, s := p.gtx, p.gtx.Ops, p.col, p.st
	f := s.fields[k.id]
	if f == nil {
		if s.fields == nil {
			s.fields = map[string]*textField{}
		}
		f = &textField{given: k.val}
		f.ed.SingleLine, f.ed.Submit = true, true
		f.ed.SetText(k.val)
		p.c.fit(k.val)
		s.fields[k.id] = f
	} else if k.val != f.given {
		if k.val != f.ed.Text() {
			f.ed.SetText(k.val)
			p.c.fit(k.val)
		}
		f.given = k.val
	}
	f.set, f.seen = k.set, true
	// Clicking the editor gives it the keyboard directly; follow it with the page focus.
	if on := gtx.Focused(&f.ed); on != f.had {
		f.had = on
		if on && s.editing != k.id {
			s.editing, s.focus, s.navFocus = k.id, len(s.controls), false
		}
	}

	w := k.width
	if w <= 0 {
		w = fieldWidth
	}
	pad, h, radius := gtx.Dp(8), gtx.Dp(26), gtx.Dp(6)
	box := image.Rect(0, 0, gtx.Dp(unit.Dp(w)), h)
	paint.FillShape(ops, col.nav, clip.UniformRRect(box, radius).Op(ops))
	border := col.line
	if s.editing == k.id {
		border = col.accent
	}
	stroke(ops, box, radius, border)

	if p.fieldH == 0 {
		_, line := p.c.label(gtx, "Ag", 12, col.fg)
		p.fieldH = line.Y
	}
	at := image.Pt(pad, (h-p.fieldH)/2)
	if f.ed.Len() == 0 && k.placeholder != "" {
		call, _ := p.text(k.placeholder, 12, col.dim)
		place(ops, call, at)
	}
	eg := gtx
	eg.Constraints = layout.Exact(image.Pt(max(box.Dx()-2*pad, 1), p.fieldH))
	fg := op.Record(ops)
	paint.ColorOp{Color: col.fg}.Add(ops)
	fgc := fg.Stop()
	sel := op.Record(ops)
	paint.ColorOp{Color: tint(col.accent, 0x55)}.Add(ops)
	selc := sel.Stop()
	// Layout drains Update itself and would drop a change still queued; take it first.
	p.win.drainField(gtx, k.id, f)
	t := op.Offset(at).Push(ops)
	f.ed.Layout(eg, p.c.shaper, font.Font{Typeface: chromeFace}, 12, fgc, selc)
	t.Pop()
	p.hit(ctlText, box, k.id, 0, false)

	size := box.Size()
	if k.err != "" {
		call, es := p.text(k.err, 11, col.err)
		place(ops, call, image.Pt(0, h+gtx.Dp(3)))
		size = image.Pt(max(size.X, es.X), h+gtx.Dp(3)+es.Y)
	}
	return size
}

// fieldText is the text in field id, or "" when no such field is shown.
func (win *window) fieldText(id string) string {
	if f := win.set.fields[id]; f != nil {
		return f.ed.Text()
	}
	return ""
}

// editField gives field id the keyboard, from the end of the frame.
func (win *window) editField(id string) {
	win.set.editing = id
	win.wantFocus = true
	win.w.Invalidate()
}

// fieldEvents runs the text fields' own keys before their editors see them: Esc
// and Tab leave a field, so does Enter (its submit), and kaze's Ctrl / Cmd shortcuts still
// work. Everything else goes to the editor, never to the page or the terminal.
func (win *window) fieldEvents(gtx layout.Context) {
	for id, f := range win.set.fields {
		if f.filters == nil {
			f.filters = []event.Filter{
				key.Filter{Focus: &f.ed, Name: key.NameEscape, Optional: allMods},
				key.Filter{Focus: &f.ed, Name: key.NameTab, Optional: allMods},
			}
			for sc := range win.km {
				if sc.Mods&(key.ModCtrl|key.ModCommand) != 0 {
					f.filters = append(f.filters, key.Filter{Focus: &f.ed, Name: sc.Name, Required: sc.Mods})
				}
			}
		}
		for {
			ev, ok := gtx.Event(f.filters...)
			if !ok {
				break
			}
			if e, ok := ev.(key.Event); ok && e.State == key.Press && win.fieldKey(gtx, f, e) {
				return // a shortcut may have closed the page; the layout drains what is left
			}
		}
		win.drainField(gtx, id, f)
	}
}

// drainField runs field id's editor events: an edit goes to set, a submit
// (Enter) gives the keyboard back to the page.
func (win *window) drainField(gtx layout.Context, id string, f *textField) {
	for {
		ev, ok := f.ed.Update(gtx)
		if !ok {
			return
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			text := f.ed.Text()
			win.ui.fit(text) // loads a fallback font for any rune the bundled ones lack
			if f.set != nil {
				f.set(text)
			}
		case widget.SubmitEvent:
			if win.set.editing == id {
				win.editField("")
			}
		}
	}
}

// fieldKey handles one key fieldEvents caught; it reports whether a kaze
// shortcut ran.
func (win *window) fieldKey(gtx layout.Context, f *textField, e key.Event) bool {
	if a, ok := win.km[kinput.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
		switch a {
		case actPaste:
			gtx.Execute(clipboard.ReadCmd{Tag: &f.ed})
		case actCopy:
			win.writeClipboard(gtx, f.ed.SelectedText())
		default:
			win.runShortcut(gtx, a)
			return true
		}
		return false
	}
	switch {
	case e.Name == key.NameEscape:
		win.editField("")
	case e.Name == key.NameTab && e.Modifiers == 0:
		win.moveFocus(1)
	case e.Name == key.NameTab && e.Modifiers == key.ModShift:
		win.moveFocus(-1)
	}
	return false
}
