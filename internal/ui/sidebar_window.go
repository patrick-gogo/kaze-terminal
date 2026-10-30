package ui

import (
	"image"
	"slices"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const (
	// minSideWindow is the narrowest window, in dp, that still shows the sidebar.
	minSideWindow = 560
	// railW is the macOS rail's width in dp: it keeps the toggle and gear while the sidebar is hidden.
	railW = 36
)

// sideUI is the window's sidebar: the state the drawing needs, the folds kept
// while kaze runs and where the last frame put things. UI goroutine only.
type sideUI struct {
	st      sideState
	folded  map[string]bool // repo groups the owner folded, by key
	hidden  bool            // off in the settings, or the window is too narrow
	hits    sideHits        // sidebar-local, like the pointer events on tag
	at      image.Point     // the sidebar's top-left in the window
	tag     int             // pointer events on the sidebar or the rail
	reveal  bool            // scroll the shown row into view next frame
	redraws int             // frames the sidebar's pointer input asked for; tests count them
}

// isMember reports whether t belongs to an open workspace run; members are not sessions.
func (win *window) isMember(t *tabs.Tab) (uint64, bool) {
	if t.Group != nil && win.runs[t.Group.Run] != nil {
		return t.Group.Run, true
	}
	return 0, false
}

// sideIn is the sidebar model's input, unfiltered and without the workspaces section.
func (win *window) sideIn() sideInput {
	in := sideInput{
		Tabs:     win.mgr.Tabs(),
		Loc:      win.locOf,
		Mark:     win.alerts.markOf,
		Member:   win.isMember,
		Folded:   win.side.folded,
		Home:     win.home,
		DefTitle: win.defTitle,
	}
	if win.ui != nil {
		in.Clean = win.ui.fit
	}
	return in
}

// sessionIDs is every session in sidebar order, folded groups included, members left out.
func (win *window) sessionIDs() []tabs.TabID {
	return sessionOrder(buildSide(win.sideIn()))
}

// selectSession shows a session picked by keyboard and scrolls its row into view.
func (win *window) selectSession(id tabs.TabID) {
	win.selectTab(id)
	win.side.reveal = true
	win.w.Invalidate()
}

// closeTab closes a tab. When it is the shown one, the next session in sidebar
// order (else the one before) is shown first, so closing never lands on a member
// while a session is left; with none left, the first open member is shown.
func (win *window) closeTab(id tabs.TabID) {
	if a := win.mgr.Active(); a != nil && a.ID == id {
		if next := win.afterClose(id); next != 0 {
			win.mgr.SetActive(next)
			win.side.reveal = true
		}
	}
	win.mgr.Close(id)
	win.w.Invalidate()
}

func (win *window) afterClose(id tabs.TabID) tabs.TabID {
	order := win.sessionIDs()
	switch i := slices.Index(order, id); {
	case i >= 0 && i+1 < len(order):
		return order[i+1]
	case i > 0:
		return order[i-1]
	case i < 0 && len(order) > 0:
		return order[0]
	}
	for _, t := range win.mgr.Tabs() {
		if _, ok := win.isMember(t); ok && t.ID != id {
			return t.ID
		}
	}
	return 0
}

// toggleSidebar shows or hides the sidebar and saves the choice.
func (win *window) toggleSidebar() {
	on := !win.opt.Config.Sidebar
	win.opt.Config.Sidebar = on
	win.side.st.hover = 0
	win.saveSetting("sidebar", on)
}

// sideWidth is how wide the sidebar (or the macOS rail) is in a window this wide,
// and sets hidden. A narrow window hides it without touching the setting.
func (win *window) sideWidth(gtx layout.Context, width int) int {
	on := win.opt.Config.Sidebar && width >= gtx.Dp(minSideWindow)
	win.side.hidden = !on
	w := 0
	switch {
	case on:
		w = gtx.Dp(sidebarW)
	case !win.custom:
		w = gtx.Dp(railW)
	}
	return min(w, width)
}

// drawSide draws the sidebar, or the macOS rail while it is hidden, into r and
// takes its pointer input there.
func (win *window) drawSide(gtx layout.Context, r image.Rectangle) {
	s := &win.side
	s.at = r.Min
	if r.Empty() {
		s.hits, s.st.hover, s.reveal = sideHits{}, 0, false
		return
	}
	ops := gtx.Ops
	off := op.Offset(r.Min).Push(ops)
	defer off.Pop()
	sg := gtx
	sg.Constraints = layout.Exact(r.Size())
	if s.hidden {
		s.st.hover, s.reveal = 0, false
		s.hits = win.ui.layoutRail(sg, r.Size())
	} else {
		in := win.sideIn()
		in.Filter = s.st.filter
		in.Runs = win.sideRuns()
		m := buildSide(in)
		s.st.shown = 0
		if a := win.mgr.Active(); a != nil {
			s.st.shown = a.ID
		}
		s.st.settings, s.st.settingsOn = win.set.open, win.set.shown
		s.st.buttons = !win.custom
		rec := op.Record(ops)
		h := win.ui.layoutSidebar(sg, m, s.st, r.Size())
		call := rec.Stop()
		if s.reveal {
			s.reveal = false
			if sc, ok := revealScroll(sg, m, s.st, h); ok && sc != s.st.scroll {
				// Laid out again at the new scroll; only this recording is drawn.
				s.st.scroll = sc
				rec = op.Record(ops)
				h = win.ui.layoutSidebar(sg, m, s.st, r.Size())
				call = rec.Stop()
			}
		}
		call.Add(ops)
		s.hits = h
		s.st.scroll = min(max(s.st.scroll, 0), h.scrollMax)
	}
	area := clip.Rect{Max: r.Size()}.Push(ops)
	event.Op(ops, &s.tag)
	area.Pop()
}

// revealScroll is the scroll that shows the shown row whole, if it is in an open group.
func revealScroll(gtx layout.Context, m sideModel, st sideState, h sideHits) (int, bool) {
	gH, rH := gtx.Dp(sideGroupH), gtx.Dp(sideRowH)
	y, top, found := 0, 0, false
	rows := func(rs []sideRow) {
		for _, r := range rs {
			if r.ID == st.shown && !found {
				top, found = y, true
			}
			y += rH
		}
	}
	for _, g := range m.Groups {
		y += gH
		if !g.Folded {
			rows(g.Rows)
		}
	}
	rows(m.Loose)
	if !found {
		return 0, false
	}
	sc := min(max(st.scroll, 0), h.scrollMax)
	sc = max(min(sc, top), top+rH-h.list.Dy())
	return min(max(sc, 0), h.scrollMax), true
}

// layoutRail draws the macOS rail: the sidebar toggle and the gear, where the
// sidebar's top row has them.
func (c *chrome) layoutRail(gtx layout.Context, size image.Point) sideHits {
	ops, th := gtx.Ops, c.th
	paint.FillShape(ops, th.Chrome, clip.Rect{Max: size}.Op())
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(size.X-1, 0, size.X, size.Y)).Op())
	bar, btn := gtx.Dp(36), gtx.Dp(28)
	x, y := (size.X-btn)/2, (bar-btn)/2
	var h sideHits
	h.toggle = image.Rect(x, y, x+btn, y+btn)
	h.gear = image.Rect(x, bar+y, x+btn, bar+y+btn)
	fillGlyph(ops, th.CapGlyph, h.toggle, toggleGlyph(gtx, h.toggle))
	fillGlyph(ops, th.CapGlyph, h.gear, captionGlyph(gtx, capGear, h.gear, false))
	return h
}

func (win *window) sideEvents(gtx layout.Context) {
	kinds := pointer.Press | pointer.Scroll | pointer.Move | pointer.Enter | pointer.Leave
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &win.side.tag, Kinds: kinds, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok && !win.bench.IgnoreInput() {
			win.sidePointer(e)
		}
	}
}

// sideRedraw asks for one frame for a change the sidebar's input made.
func (win *window) sideRedraw() {
	win.side.redraws++
	win.w.Invalidate()
}

// sidePointer handles one pointer event on the sidebar. Hover redraws only when
// the hovered row changes, so moving the mouse over the sidebar stays cheap.
func (win *window) sidePointer(e pointer.Event) {
	s := &win.side
	p := e.Position.Round()
	switch e.Kind {
	case pointer.Move, pointer.Enter, pointer.Leave:
		var hover tabs.TabID
		if e.Kind != pointer.Leave {
			for _, r := range s.hits.rows {
				if p.In(r.rect) {
					hover = r.id
					break
				}
			}
		}
		if hover != s.st.hover {
			s.st.hover = hover
			win.sideRedraw()
		}
	case pointer.Scroll:
		if !p.In(s.hits.list) {
			return
		}
		if sc := min(max(s.st.scroll+int(e.Scroll.Y), 0), s.hits.scrollMax); sc != s.st.scroll {
			s.st.scroll = sc
			win.sideRedraw()
		}
	case pointer.Press:
		win.sidePress(p, e.Buttons)
	}
}

// sidePress handles a click: a row shows its session, its × or a middle click
// closes it; headers fold; + opens a session; a member becomes the main view.
func (win *window) sidePress(p image.Point, b pointer.Buttons) {
	primary, middle := b.Contain(pointer.ButtonPrimary), b.Contain(pointer.ButtonTertiary)
	if !primary && !middle {
		return
	}
	h := &win.side.hits
	win.wantFocus = true
	defer win.sideRedraw()
	switch {
	case p.In(h.toggle):
		if primary {
			win.toggleSidebar()
		}
		return
	case p.In(h.gear):
		if primary {
			win.openSettings()
		}
		return
	case p.In(h.settings):
		switch {
		case middle, primary && p.In(h.settingsClose):
			win.closeSettings()
		case primary:
			win.openSettings()
		}
		return
	case p.In(h.plus):
		if primary {
			win.newTab()
		}
		return
	case p.In(h.filter):
		return // typing in the filter comes with its own focus handling
	}
	for _, r := range h.rows {
		if p.In(r.rect) {
			switch {
			case middle, p.In(r.close):
				win.closeTab(r.id)
			default:
				win.selectTab(r.id)
			}
			return
		}
	}
	if !primary {
		return
	}
	for _, g := range h.groups {
		if p.In(g.rect) {
			if win.side.folded == nil {
				win.side.folded = map[string]bool{}
			}
			flip(win.side.folded, g.key)
			return
		}
	}
	for _, r := range h.runs {
		if p.In(r.rect) {
			if win.side.st.runFolded == nil {
				win.side.st.runFolded = map[uint64]bool{}
			}
			flip(win.side.st.runFolded, r.run)
			return
		}
	}
	for _, m := range h.members {
		if p.In(m.rect) {
			win.selectTab(m.id)
			return
		}
	}
}

// flip folds k, or unfolds it by forgetting it.
func flip[K comparable](m map[K]bool, k K) {
	if m[k] {
		delete(m, k)
	} else {
		m[k] = true
	}
}
