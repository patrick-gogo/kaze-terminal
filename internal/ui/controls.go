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
	ctlColor // one recipe color, a filled square
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
	arg             int  // button, swatch, color
	dead            bool // button: drawn dim and does nothing (Save while the draft is invalid)
	ghost           bool // button: an outline only, for quiet actions
	cursorChips     bool // segmented: options drawn as cursor-shape chips instead of words

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
	dead bool // a stepper end at its limit or a disabled button: drawn dim, does nothing
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
	case ctlColor:
		size = p.colorChip(k)
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
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	fg := col.title
	switch {
	case k.dead:
		fg = col.dim
	case k.strong:
		fg = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	}
	call, size := p.textW(k.text, 12.5, fg, font.SemiBold)
	r := image.Rect(0, 0, size.X+2*gtx.Dp(14), gtx.Dp(30))
	radius := gtx.Dp(8)
	switch {
	case k.strong && k.dead:
		paint.FillShape(ops, tint(col.accent, 0x40), clip.UniformRRect(r, radius).Op(ops))
	case k.strong:
		paint.FillShape(ops, col.accent, clip.UniformRRect(r, radius).Op(ops))
	case k.ghost:
		stroke(ops, r, radius, col.line2)
	default:
		paint.FillShape(ops, col.raised, clip.UniformRRect(r, radius).Op(ops))
		stroke(ops, r, radius, col.line2)
	}
	place(ops, call, image.Pt((r.Dx()-size.X)/2, (r.Dy()-size.Y)/2))
	p.hit(ctlButton, r, k.id, k.arg, k.dead)
	return r.Size()
}

// colorChip is one recipe color in the current theme; the chosen one is ringed.
func (p *page) colorChip(k ctrl) image.Point {
	gtx, ops := p.gtx, p.gtx.Ops
	s, g := gtx.Dp(18), gtx.Dp(3)
	r := image.Rect(g, g, g+s, g+s)
	paint.FillShape(ops, p.c.memberColor(k.text), clip.UniformRRect(r, gtx.Dp(4)).Op(ops))
	outer := r.Inset(-g)
	if k.on {
		paint.FillShape(ops, p.col.title, clip.Stroke{Path: clip.UniformRRect(outer, gtx.Dp(6)).Path(ops), Width: float32(gtx.Dp(2))}.Op())
	}
	p.hit(ctlColor, outer, k.id, k.arg, false)
	return outer.Max
}

func (p *page) toggle(k ctrl) image.Point {
	gtx, ops := p.gtx, p.gtx.Ops
	w, h, d, in := gtx.Dp(38), gtx.Dp(22), gtx.Dp(16), gtx.Dp(3)
	r := image.Rect(0, 0, w, h)
	white := color.NRGBA{R: 0xf2, G: 0xf3, B: 0xf7, A: 0xff}
	track, knob, x, arg := p.col.line2, white, in, 1
	if k.on {
		track, x, arg = p.col.accent, w-in-d, 0
	}
	paint.FillShape(ops, track, clip.UniformRRect(r, h/2).Op(ops))
	y := (h - d) / 2
	paint.FillShape(ops, knob, clip.Ellipse(image.Rect(x, y, x+d, y+d)).Op(ops))
	p.hit(ctlToggle, r, k.id, arg, false)
	return r.Size()
}

func (p *page) segmented(k ctrl) image.Point {
	if k.cursorChips {
		return p.cursorChips(k)
	}
	gtx, ops := p.gtx, p.gtx.Ops
	padX, in, h := gtx.Dp(12), gtx.Dp(3), gtx.Dp(32)
	type opt struct {
		call op.CallOp
		size image.Point
		r    image.Rectangle
	}
	opts := make([]opt, 0, len(k.options))
	x := in
	for i, o := range k.options {
		fg, w := p.col.muted, font.Medium
		if i == k.selected {
			fg = p.col.title
		}
		call, size := p.textW(o, 12, fg, w)
		r := image.Rect(x, in, x+size.X+2*padX, h-in)
		opts = append(opts, opt{call, size, r})
		x = r.Max.X + gtx.Dp(2)
	}
	outer := image.Rect(0, 0, x-gtx.Dp(2)+in, h)
	paint.FillShape(ops, p.col.nav, clip.UniformRRect(outer, gtx.Dp(9)).Op(ops))
	stroke(ops, outer, gtx.Dp(9), p.col.line2)
	if k.selected >= 0 && k.selected < len(opts) {
		paint.FillShape(ops, mix(p.col.raised, p.col.title, 0.08), clip.UniformRRect(opts[k.selected].r, gtx.Dp(6)).Op(ops))
	}
	for i, o := range opts {
		place(ops, o.call, image.Pt(o.r.Min.X+padX, o.r.Min.Y+(o.r.Dy()-o.size.Y)/2))
		p.hit(ctlSegmented, o.r, k.id, i, false)
	}
	return outer.Size()
}

func (p *page) stepper(k ctrl) image.Point {
	gtx, ops := p.gtx, p.gtx.Ops
	cell, h := gtx.Dp(32), gtx.Dp(32)
	vc, vs := p.mono(strconv.Itoa(k.value), 12.5, p.col.title)
	valW := vs.X + gtx.Dp(22)
	outer := image.Rect(0, 0, 2*cell+valW, h)
	paint.FillShape(ops, p.col.nav, clip.UniformRRect(outer, gtx.Dp(9)).Op(ops))
	stroke(ops, outer, gtx.Dp(9), p.col.line2)
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
		call, size := p.textW(e.glyph, 15, fg, font.Medium)
		place(ops, call, e.r.Min.Add(image.Pt((cell-size.X)/2, (h-size.Y)/2)))
		p.hit(ctlStepper, e.r, k.id, e.to, e.dead)
	}
	return outer.Size()
}

// swatch draws a theme as a card: a small terminal in its own colors, then
// its name and a radio dot; the theme in use gets the accent ring.
func (p *page) swatch(k ctrl, w int) image.Point {
	gtx, ops, t, col := p.gtx, p.gtx.Ops, k.theme, p.col
	pad := gtx.Dp(8)
	shotH := gtx.Dp(62)
	shot := image.Rect(pad, pad, w-pad, pad+shotH)
	paint.FillShape(ops, t.Term.BG, clip.UniformRRect(shot, gtx.Dp(6)).Op(ops))
	lines := [][]any{
		{t.UI.Accent, "~/todo-api ", t.Term.ANSI[2], "main"},
		{t.Term.FG, "> make"},
		{t.Term.FG, "ok api 0.21s"},
		{t.Term.ANSI[3], "warn ", t.Term.FG, "1 left"},
	}
	ly := shot.Min.Y + gtx.Dp(6)
	for _, l := range lines {
		x, h := shot.Min.X+gtx.Dp(7), 0
		for i := 0; i+1 < len(l); i += 2 {
			call, sz := p.c.label(gtx, l[i+1].(string), 8, l[i].(color.NRGBA))
			if x+sz.X <= shot.Max.X-gtx.Dp(4) {
				place(ops, call, image.Pt(x, ly))
			}
			x += sz.X
			h = max(h, sz.Y)
		}
		ly += h
	}
	nc, ns := p.textW(t.Name, 12.5, col.title, font.SemiBold)
	nameY := shot.Max.Y + gtx.Dp(8)
	place(ops, nc, image.Pt(pad+gtx.Dp(2), nameY))
	d := gtx.Dp(14)
	dot := image.Rect(w-pad-d, nameY+(ns.Y-d)/2, w-pad, nameY+(ns.Y-d)/2+d)
	r := image.Rect(0, 0, w, nameY+ns.Y+pad+gtx.Dp(2))
	if k.current {
		paint.FillShape(ops, col.accent, clip.Ellipse(dot).Op(ops))
		in := dot.Inset(gtx.Dp(4))
		paint.FillShape(ops, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, clip.Ellipse(in).Op(ops))
		paint.FillShape(ops, tint(col.accent, 0x30), clip.Stroke{Path: clip.UniformRRect(r.Inset(-gtx.Dp(2)), gtx.Dp(12)).Path(ops), Width: float32(gtx.Dp(3))}.Op())
		stroke(ops, r, gtx.Dp(10), col.accent)
	} else {
		paint.FillShape(ops, col.line2, clip.Stroke{Path: clip.Ellipse(dot).Path(ops), Width: float32(max(gtx.Dp(1), 1)) * 1.5}.Op())
		stroke(ops, r, gtx.Dp(10), col.line2)
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
		y := h + gtx.Dp(3)
		for _, line := range p.wrap(k.err, 11, box.Dx()) {
			call, es := p.text(line, 11, col.err)
			place(ops, call, image.Pt(0, y))
			y += es.Y
		}
		size.Y = y
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
	if id != "" {
		win.dropSideEdit()
	}
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

// cursorChips draws a segmented choice of cursor shapes as chips, each an "a"
// with the shape after it; the chosen one gets the accent ring.
func (p *page) cursorChips(k ctrl) image.Point {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	w, h, gap := gtx.Dp(46), gtx.Dp(34), gtx.Dp(8)
	x := 0
	for i, o := range k.options {
		r := image.Rect(x, 0, x+w, h)
		radius := gtx.Dp(8)
		paint.FillShape(ops, col.nav, clip.UniformRRect(r, radius).Op(ops))
		if i == k.selected {
			paint.FillShape(ops, tint(col.accent, 0x30), clip.Stroke{Path: clip.UniformRRect(r.Inset(-gtx.Dp(2)), radius+gtx.Dp(2)).Path(ops), Width: float32(gtx.Dp(3))}.Op())
			stroke(ops, r, radius, col.accent)
		} else {
			stroke(ops, r, radius, col.line2)
		}
		ac, as := p.mono("a", 12, col.title)
		p.st.texts = append(p.st.texts, o)
		cw, ch := gtx.Dp(7), gtx.Dp(14)
		total := as.X + gtx.Dp(1) + cw
		ax := r.Min.X + (w-total)/2
		place(ops, ac, image.Pt(ax, (h-as.Y)/2))
		cx, cy := ax+as.X+gtx.Dp(1), (h-ch)/2
		cur := image.Rect(cx, cy, cx+cw, cy+ch)
		switch i {
		case 1:
			cur.Max.X = cx + max(gtx.Dp(2), 1)
		case 2:
			cur.Min.Y = cur.Max.Y - max(gtx.Dp(2), 1)
		}
		paint.FillShape(ops, col.accent, clip.Rect(cur).Op())
		p.hit(ctlSegmented, r, k.id, i, false)
		x += w + gap
	}
	return image.Pt(x-gap, h)
}
