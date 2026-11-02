package ui

import (
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"image"
)

type docHits struct {
	view, outline                image.Rectangle
	heads                        []docHeadHit
	links                        []docLinkHit
	scrollMax, outScroll, outMax int
}
type docHeadHit struct {
	rect image.Rectangle
	i    int
}
type docLinkHit struct {
	rect image.Rectangle
	link int
}

func (win *window) docCurrent(gtx layout.Context, d *docView) int {
	current := -1
	for i, y := range d.lay.headY {
		if y > d.scroll+gtx.Dp(24) {
			break
		}
		current = i
	}
	return current
}
func (win *window) docScrollTo(d *docView, y int) {
	next := min(max(y, 0), win.docs.hits.scrollMax)
	if next != d.scroll {
		d.scroll = next
		win.w.Invalidate()
	}
}
func (win *window) docKey(gtx layout.Context, e key.Event) {
	if e.State != key.Press {
		return
	}
	if a, ok := win.km[input.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
		win.runShortcut(gtx, a)
		return
	}
	d := win.shownDoc()
	if d == nil {
		return
	}
	y := d.scroll
	switch e.Name {
	case key.NameDownArrow:
		y += gtx.Dp(40)
	case key.NameUpArrow:
		y -= gtx.Dp(40)
	case key.NamePageDown:
		y += max(1, win.docs.hits.view.Dy()-gtx.Dp(40))
	case key.NamePageUp:
		y -= max(1, win.docs.hits.view.Dy()-gtx.Dp(40))
	case key.NameHome:
		y = 0
	case key.NameEnd:
		y = win.docs.hits.scrollMax
	}
	win.docScrollTo(d, y)
}
func (win *window) docPointer(gtx layout.Context, e pointer.Event) {
	d := win.shownDoc()
	if d == nil {
		return
	}
	h := &win.docs.hits
	p := e.Position.Round()
	switch e.Kind {
	case pointer.Scroll:
		if p.In(h.outline) {
			sc := min(max(h.outScroll+int(e.Scroll.Y), 0), h.outMax)
			if sc != h.outScroll {
				h.outScroll = sc
				win.w.Invalidate()
			}
		} else {
			win.docScrollTo(d, d.scroll+int(e.Scroll.Y))
		}
	case pointer.Press:
		if !e.Buttons.Contain(pointer.ButtonPrimary) {
			return
		}
		for _, lh := range h.links {
			if p.In(lh.rect) && lh.link >= 0 && lh.link < len(d.lay.links) {
				win.followDocLink(d, d.lay.links[lh.link])
				return
			}
		}
		for _, hh := range h.heads {
			if p.In(hh.rect) {
				win.docScrollTo(d, d.lay.headY[hh.i]-gtx.Dp(12))
				return
			}
		}
	}
}
func (win *window) drawDocOutline(gtx layout.Context, d *docView, r image.Rectangle) {
	h := &win.docs.hits
	h.heads = h.heads[:0]
	h.outline = image.Rectangle{}
	if r.Dx() < gtx.Dp(600) {
		return
	}
	w := min(gtx.Dp(220), r.Dx()/3)
	out := image.Rect(r.Min.X, r.Min.Y, r.Min.X+w, r.Max.Y)
	h.outline = out
	paint.FillShape(gtx.Ops, win.ui.th.Chrome, clip.Rect(out).Op())
	paint.FillShape(gtx.Ops, win.ui.th.Border, clip.Rect(image.Rect(out.Max.X-1, out.Min.Y, out.Max.X, out.Max.Y)).Op())
	call, _ := win.ui.sans(gtx, "Outline", 18, win.ui.th.ActiveFG, font.SemiBold)
	place(gtx.Ops, call, out.Min.Add(image.Pt(gtx.Dp(20), gtx.Dp(24))))
	win.docs.texts = append(win.docs.texts, "Outline")
	list := out
	list.Min.Y += gtx.Dp(64)
	h.outMax = max(0, len(d.doc.Outline)*gtx.Dp(30)-list.Dy())
	h.outScroll = min(h.outScroll, h.outMax)
	cl := clip.Rect(list).Push(gtx.Ops)
	defer cl.Pop()
	level := 6
	for _, head := range d.doc.Outline {
		level = min(level, head.Level)
	}
	current := win.docCurrent(gtx, d)
	for i, head := range d.doc.Outline {
		y := list.Min.Y + i*gtx.Dp(30) - h.outScroll
		band := image.Rect(out.Min.X+gtx.Dp(8), y, out.Max.X-gtx.Dp(8), y+gtx.Dp(30))
		if !band.Overlaps(list) {
			continue
		}
		col := win.ui.th.Muted
		if i == current {
			col = win.ui.th.ActiveFG
			paint.FillShape(gtx.Ops, tint(win.ui.th.Accent, 0x2e), clip.UniformRRect(band, gtx.Dp(5)).Op(gtx.Ops))
		}
		x := band.Min.X + gtx.Dp(unit.Dp(12+12*min(3, max(0, head.Level-level))))
		call, sz := win.ui.fitSide(gtx, head.Text, 12, max(1, band.Max.X-x-gtx.Dp(6)), col)
		place(gtx.Ops, call, image.Pt(x, y+(band.Dy()-sz.Y)/2))
		hit := band.Intersect(list)
		h.heads = append(h.heads, docHeadHit{hit, i})
		a := clip.Rect(hit).Push(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		a.Pop()
	}
}
