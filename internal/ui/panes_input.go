package ui

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// paneEdges takes a pane's edge bands and corners for the frame tag, after its
// header and body so they win, each with its resize cursor. The cursors are
// static ops, so hovering never asks for a frame.
func paneEdges(gtx layout.Context, p *pane) {
	o := p.hits.outer
	b, c := gtx.Dp(paneEdgeDp), gtx.Dp(paneCornerDp)
	zones := []struct {
		r   image.Rectangle
		cur pointer.Cursor
	}{
		{image.Rect(o.Min.X+c, o.Min.Y, o.Max.X-c, o.Min.Y+b), pointer.CursorNorthResize},
		{image.Rect(o.Min.X+c, o.Max.Y-b, o.Max.X-c, o.Max.Y), pointer.CursorSouthResize},
		{image.Rect(o.Min.X, o.Min.Y+c, o.Min.X+b, o.Max.Y-c), pointer.CursorWestResize},
		{image.Rect(o.Max.X-b, o.Min.Y+c, o.Max.X, o.Max.Y-c), pointer.CursorEastResize},
		{image.Rect(o.Min.X, o.Min.Y, o.Min.X+c, o.Min.Y+b), pointer.CursorNorthWestResize},
		{image.Rect(o.Min.X, o.Min.Y, o.Min.X+b, o.Min.Y+c), pointer.CursorNorthWestResize},
		{image.Rect(o.Max.X-c, o.Min.Y, o.Max.X, o.Min.Y+b), pointer.CursorNorthEastResize},
		{image.Rect(o.Max.X-b, o.Min.Y, o.Max.X, o.Min.Y+c), pointer.CursorNorthEastResize},
		{image.Rect(o.Min.X, o.Max.Y-b, o.Min.X+c, o.Max.Y), pointer.CursorSouthWestResize},
		{image.Rect(o.Min.X, o.Max.Y-c, o.Min.X+b, o.Max.Y), pointer.CursorSouthWestResize},
		{image.Rect(o.Max.X-c, o.Max.Y-b, o.Max.X, o.Max.Y), pointer.CursorSouthEastResize},
		{image.Rect(o.Max.X-b, o.Max.Y-c, o.Max.X, o.Max.Y), pointer.CursorSouthEastResize},
	}
	for _, z := range zones {
		if z.r.Empty() {
			continue
		}
		area := clip.Rect(z.r).Push(gtx.Ops)
		z.cur.Add(gtx.Ops)
		event.Op(gtx.Ops, &p.frame)
		area.Pop()
	}
}

// paneEvents runs the shown session's panes' input from the last frame's
// views: the frame tag (header and edges) moves and resizes, the body tag is
// the pane's terminal.
func (win *window) paneEvents(gtx layout.Context, cw, ch int) {
	views := win.fl.views
	for _, pv := range views {
		p := pv.p
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: &p.frame, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel})
			if !ok {
				break
			}
			if e, ok := ev.(pointer.Event); ok && !win.bench.IgnoreInput() {
				win.paneFrame(gtx, p, e)
			}
		}
	}
	for _, pv := range views {
		if win.mgr.Lookup(pv.v.tab.ID) == nil || win.fl.owner(pv.p.id) == 0 {
			continue // closed or removed by an earlier event this frame
		}
		win.termEvents(gtx, pv.v, cw, ch)
	}
}

// paneFrame handles a pointer event on a pane's header or edges.
func (win *window) paneFrame(gtx layout.Context, p *pane, e pointer.Event) {
	pt := e.Position.Round()
	switch e.Kind {
	case pointer.Press:
		if !e.Buttons.Contain(pointer.ButtonPrimary) {
			return
		}
		win.focusPane(p.id)
		switch {
		case pt.In(p.hits.close):
			win.closePane(p.id)
			return
		case pt.In(p.hits.expand):
			win.expandMember(p.id)
			return
		}
		e, ok := edgeAt(p.hits.outer, pt, gtx.Dp(paneEdgeDp), gtx.Dp(paneCornerDp))
		if !ok {
			return
		}
		p.grab = paneGrab{on: true, e: e, from: pt, start: p.fit}
	case pointer.Drag:
		if !p.grab.on {
			return
		}
		scale := gtx.Metric.PxPerDp
		if scale <= 0 {
			scale = 1
		}
		d := pt.Sub(p.grab.from)
		d = image.Pt(int(float32(d.X)/scale), int(float32(d.Y)/scale))
		p.r = dragRect(p.grab.start, p.grab.e, d, paneMinDp)
		win.fl.redraws++
		win.w.Invalidate()
	case pointer.Release, pointer.Cancel:
		p.grab = paneGrab{}
	}
}

// keyTab is the tab that has the keyboard: the pane with it, else the shown tab.
func (win *window) keyTab() *tabs.Tab {
	if v, ok := win.paneKeyView(); ok {
		return v.tab
	}
	return win.shownTab()
}

// paneKeyView is the drawn pane with the keyboard, if any.
func (win *window) paneKeyView() (termView, bool) {
	if win.fl.keys == 0 || win.set.shown {
		return termView{}, false
	}
	for _, pv := range win.fl.views {
		if pv.p.id == win.fl.keys {
			return pv.v, true
		}
	}
	return termView{}, false
}

// moveKeys gives the keyboard to pane id (0: the session) and moves the
// focus-in and focus-out reports with it.
func (win *window) moveKeys(id tabs.TabID) {
	if id == win.fl.keys {
		win.wantFocus = true
		return
	}
	from := win.keyTab()
	win.fl.keys = id
	to := win.keyTab()
	if win.focused && from != to {
		if from != nil {
			sendFocus(from, false)
		}
		if to != nil {
			sendFocus(to, true)
		}
	}
	win.wantFocus = true
	win.w.Invalidate()
}

// focusPane raises pane id and gives it the keyboard.
func (win *window) focusPane(id tabs.TabID) {
	if o := win.fl.owner(id); o != 0 {
		win.fl.raise(o, id)
	}
	win.dropSideEdit()
	win.moveKeys(id)
}

// focusSession gives the keyboard back to the shown session.
func (win *window) focusSession() { win.moveKeys(0) }

// closePane is a pane's ×: a member leaves the set and goes back to the shared
// size, still running; the scratch shell is only hidden.
func (win *window) closePane(id tabs.TabID) {
	o := win.fl.owner(id)
	if o == 0 {
		return
	}
	if win.fl.keys == id {
		win.focusSession()
	}
	p := win.fl.set(o).find(id)
	if p.kind == paneScratch {
		p.hidden = true
	} else {
		win.fl.remove(id)
		win.mgr.Dock(id)
	}
	win.w.Invalidate()
}

// expandMember makes a floating member the main view; picking a session goes back.
func (win *window) expandMember(id tabs.TabID) {
	win.fl.remove(id)
	win.mgr.Dock(id)
	win.selectTab(id)
	win.side.reveal = true
}
