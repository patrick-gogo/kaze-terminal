package ui

import (
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/unit"
	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"image"
)

func agentInner(gtx layout.Context, body image.Rectangle) image.Rectangle {
	return image.Rect(body.Min.X+gtx.Dp(10), body.Min.Y+gtx.Dp(6), body.Max.X-gtx.Dp(10), body.Max.Y-gtx.Dp(6))
}
func (win *window) agentPaneLink(gtx layout.Context, p *pane, at image.Point) (string, bool) {
	inner := agentInner(gtx, p.hits.body)
	if !at.In(inner) {
		return "", false
	}
	i := (at.Y - inner.Min.Y + p.scroll) / max(1, gtx.Dp(18))
	if i < 0 || i >= len(p.wrapped) {
		return "", false
	}
	line := p.wrapped[i]
	x := inner.Min.X
	size := 11
	face := font.Font{Typeface: sansFace}
	switch line.kind {
	case linePrompt:
		x += gtx.Dp(10)
		size = 12
	case lineWaiting:
		x += gtx.Dp(8)
		face.Weight = font.Medium
	case lineMessage, lineReport:
		size = 12
	case linePromptHead, lineReportHead:
		face.Weight = font.SemiBold
	}
	for _, l := range render.LinksIn(line.text) {
		left := x + win.ui.textWidth(gtx, line.text[:l.Start], unit.Sp(size), face)
		right := x + win.ui.textWidth(gtx, line.text[:l.End], unit.Sp(size), face)
		if at.X >= left && at.X < right {
			return l.Text, true
		}
	}
	return "", false
}
func (win *window) agentPanePointer(gtx layout.Context, p *pane, e pointer.Event) {
	switch e.Kind {
	case pointer.Press:
		if e.Buttons.Contain(pointer.ButtonPrimary) && e.Modifiers.Contain(key.ModShortcut) {
			if ref, ok := win.agentPaneLink(gtx, p, e.Position.Round()); ok {
				if render.IsWebLink(ref) {
					win.open(ref)
				} else {
					owner := win.mgr.Lookup(win.fl.owner(p.id))
					win.openDocRef(owner, true, ref)
				}
				win.w.Invalidate()
				return
			}
		}
		if o := win.fl.owner(p.id); o != 0 {
			win.fl.raise(o, p.id)
		}
	case pointer.Scroll:
		p.agentScroll(int(e.Scroll.Y), max(len(p.wrapped)*gtx.Dp(18)-p.hits.body.Dy()+gtx.Dp(12), 0))
	}
	win.w.Invalidate()
}
