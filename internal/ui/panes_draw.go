package ui

import (
	"image"
	"image/color"
	"runtime"
	"slices"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// paneLook is what a pane's header shows.
type paneLook struct {
	title, status string
	statusDown    bool   // status drawn in UsageHigh (red)
	color         string // member color name, "" for the scratch shell
	mark          mark   // alert mark of the pane's tab
	front         bool
	expand        bool   // members only
	folder        string // scratch only: the session's folder, ~-shortened
	key           string // scratch only: the scratch_shell key text
	bg            color.NRGBA
}

// paneView is a visible pane with the terminal view it draws this frame.
type paneView struct {
	p *pane
	v termView
}

func (c *chrome) paneBorder(front bool) color.NRGBA {
	if front {
		return c.th.Accent
	}
	return c.th.Border
}

// layoutPane draws a pane's frame and header in r and fills its body with the
// terminal background; the caller draws the terminal into hits.body.
func (c *chrome) layoutPane(gtx layout.Context, r image.Rectangle, lk paneLook) paneHits {
	ops, th := gtx.Ops, c.th
	h := paneHits{outer: r}
	radius, line := gtx.Dp(paneRadiusDp), max(gtx.Dp(1), 1)
	headH := gtx.Dp(paneHeaderDp)
	paint.FillShape(ops, c.paneBorder(lk.front), clip.UniformRRect(r, radius).Op(ops))
	inner := r.Inset(line)
	cl := clip.UniformRRect(inner, max(radius-line, 0)).Push(ops)
	defer cl.Pop()
	h.header = image.Rect(inner.Min.X, inner.Min.Y, inner.Max.X, min(inner.Min.Y+headH, inner.Max.Y))
	h.body = image.Rect(inner.Min.X, h.header.Max.Y, inner.Max.X, inner.Max.Y)
	paint.FillShape(ops, th.Chrome, clip.Rect(h.header).Op())
	paint.FillShape(ops, lk.bg, clip.Rect(h.body).Op())
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(h.header.Min.X, h.header.Max.Y-1, h.header.Max.X, h.header.Max.Y)).Op())

	mid := h.header.Min.Y + h.header.Dy()/2
	pad := gtx.Dp(10)
	right := h.header.Max.X
	// × then Expand, from the right.
	btnW := gtx.Dp(28)
	h.close = image.Rect(right-btnW, h.header.Min.Y, right, h.header.Max.Y)
	call, sz := c.label(gtx, "×", 14, th.Muted)
	place(ops, call, image.Pt(h.close.Min.X+(btnW-sz.X)/2, mid-sz.Y/2))
	right = h.close.Min.X
	if lk.expand {
		call, sz = c.label(gtx, "Expand", 11, th.Muted)
		w := sz.X + 2*gtx.Dp(8)
		h.expand = image.Rect(right-w, h.header.Min.Y, right, h.header.Max.Y)
		place(ops, call, image.Pt(h.expand.Min.X+gtx.Dp(8), mid-sz.Y/2))
		right = h.expand.Min.X
	}
	if lk.key != "" {
		call, sz = c.fitSide(gtx, lk.key, 11, max((right-h.header.Min.X)/3, 0), th.Muted)
		right -= sz.X + gtx.Dp(8)
		place(ops, call, image.Pt(right, mid-sz.Y/2))
	}
	if lk.mark != markNone {
		right = c.alertDot(gtx, right, mid, lk.mark)
	}

	x := h.header.Min.X + pad
	if lk.color != "" {
		bar := image.Rect(x, mid-gtx.Dp(7), x+gtx.Dp(3), mid+gtx.Dp(7))
		paint.FillShape(ops, c.memberColor(lk.color), clip.UniformRRect(bar, gtx.Dp(1)).Op(ops))
		x = bar.Max.X + gtx.Dp(8)
	}
	room := max(right-gtx.Dp(8)-x, 0)
	call, sz = c.fitSide(gtx, lk.title, 12, room, th.ActiveFG)
	place(ops, call, image.Pt(x, mid-sz.Y/2))
	x += sz.X + gtx.Dp(8)
	room = max(right-gtx.Dp(8)-x, 0)
	if lk.status != "" {
		col := th.Muted
		if lk.statusDown {
			col = th.UsageHigh
		}
		call, sz = c.fitSide(gtx, lk.status, 11, room, col)
		place(ops, call, image.Pt(x, mid-sz.Y/2))
		x += sz.X + gtx.Dp(8)
		room = max(right-gtx.Dp(8)-x, 0)
	}
	if lk.folder != "" {
		call, sz = c.fitSide(gtx, lk.folder, 11, room, th.Muted)
		place(ops, call, image.Pt(x, mid-sz.Y/2))
	}
	return h
}

// paneOwner is the session panes float over: the shown tab when it is a
// session, else 0 (a member or a scratch shell in front, or Settings).
func (win *window) paneOwner() tabs.TabID {
	t := win.shownTab()
	if t == nil || t.Parent != 0 {
		return 0
	}
	if _, member := win.isMember(t); member {
		return 0
	}
	return t.ID
}

// paneStep keeps the pane model in line with the tabs before drawing: the
// shown tab is never also a pane, gone tabs leave their sets, and an orphaned
// scratch shell closes.
func (win *window) paneStep(active *tabs.Tab) {
	if active != nil {
		win.mgr.Dock(active.ID) // a no-op unless it floated
		win.fl.remove(active.ID)
	}
	orphans, released := win.fl.prune(win.mgr.Has)
	for _, id := range orphans {
		win.mgr.Close(id)
	}
	for _, t := range win.mgr.Tabs() {
		if t.Parent != 0 && !win.mgr.Has(t.Parent) {
			win.mgr.Close(t.ID) // its session is gone
		}
	}
	if active != nil && active.Parent != 0 {
		// A scratch shell is never the main view: show its session instead.
		if win.mgr.Has(active.Parent) {
			win.mgr.SetActive(active.Parent)
		}
	}
	for _, id := range released {
		win.mgr.Dock(id)
	}
}

// paneViews fits the shown session's visible panes into termRect, back to
// front, and gives each pane's tab the grid it draws.
func (win *window) paneViews(gtx layout.Context, termRect image.Rectangle, cw, ch int) []paneView {
	owner := win.paneOwner()
	s := win.fl.sets[owner]
	if owner == 0 || s == nil {
		return nil
	}
	scale := gtx.Metric.PxPerDp
	if scale <= 0 {
		scale = 1
	}
	toPx := func(v int) int { return int(float32(v)*scale + 0.5) }
	area := image.Rect(0, 0, int(float32(termRect.Dx())/scale), int(float32(termRect.Dy())/scale))
	line, headH := max(gtx.Dp(1), 1), gtx.Dp(paneHeaderDp)
	padX, padY := gtx.Dp(8), gtx.Dp(4)
	var out []paneView
	for _, p := range s.visible() {
		t := win.mgr.Lookup(p.id)
		if t == nil {
			continue
		}
		f := fitPane(p.r, area, paneMinDp)
		p.fit = f
		outer := image.Rect(toPx(f.Min.X), toPx(f.Min.Y), toPx(f.Max.X), toPx(f.Max.Y)).Add(termRect.Min).Intersect(termRect)
		body := image.Rect(outer.Min.X+line, outer.Min.Y+line+headH, outer.Max.X-line, outer.Max.Y-line)
		grid := image.Rect(body.Min.X+padX, body.Min.Y+padY, body.Max.X-padX, body.Max.Y-padY)
		cols, rows := gridSize(grid.Dx(), grid.Dy(), cw, ch)
		win.mgr.ResizeTab(p.id, cols, rows)
		p.hits = paneHits{outer: outer}
		out = append(out, paneView{p: p, v: termView{tab: t, tag: &p.body, grid: grid, cols: cols, rows: rows}})
	}
	return out
}

// paneLook is what pane p's header says.
func (win *window) paneLook(p *pane, t *tabs.Tab, front bool) paneLook {
	lk := paneLook{front: front, bg: win.pal.BG, mark: win.alerts.markOf(p.id)}
	if p.kind == paneScratch {
		lk.title = "scratch shell"
		if s := win.mgr.Lookup(t.Parent); s != nil {
			lk.folder = cleanText(tildeHome(win.scratchDir(s), win.home))
		}
		if sc, ok := win.binds["scratch_shell"]; ok {
			lk.key = formatShortcut(runtime.GOOS, sc)
		}
		return lk
	}
	lk.title, lk.color, lk.expand = t.Title, t.Color, true
	for _, r := range win.curRuns() {
		for _, m := range r.Members {
			if m.ID == p.id {
				lk.title, lk.status, lk.statusDown = m.Title, m.Status, m.Down
			}
		}
	}
	return lk
}

// scratchDir is where a session's scratch shell starts: the session's last
// reported folder, else the folder it started in.
func (win *window) scratchDir(session *tabs.Tab) string {
	if cwd := win.locOf(session).cwd; cwd != "" {
		return cwd
	}
	return session.Dir
}

// drawPanes draws the views back to front, each with its own renderer, and
// takes their input areas so nothing under a pane is clicked.
func (win *window) drawPanes(gtx layout.Context, views []paneView) {
	ops := gtx.Ops
	for i, pv := range views {
		p, v := pv.p, pv.v
		lk := win.paneLook(p, v.tab, i == len(views)-1)
		p.hits = win.ui.layoutPane(gtx, p.hits.outer, lk)
		rend := win.paneRend(p.id)
		rend.SetFocused(win.viewFocused(p.id))
		sel := win.checkSel(v.tab)
		g := clip.Rect(v.grid.Intersect(p.hits.body)).Push(ops)
		off := op.Offset(v.grid.Min).Push(ops)
		tg := gtx
		tg.Constraints = layout.Exact(image.Pt(max(v.grid.Dx(), 0), max(v.grid.Dy(), 0)))
		rend.Layout(tg, v.tab.Session, &sel)
		off.Pop()
		g.Pop()
		area := clip.Rect(p.hits.body).Push(ops)
		event.Op(ops, &p.body)
		area.Pop()
		area = clip.Rect(p.hits.header).Push(ops)
		pointer.CursorGrab.Add(ops)
		event.Op(ops, &p.frame)
		area.Pop()
		for _, b := range []image.Rectangle{p.hits.close, p.hits.expand} {
			if !b.Empty() {
				area = clip.Rect(b).Push(ops)
				pointer.CursorPointer.Add(ops)
				event.Op(ops, &p.frame)
				area.Pop()
			}
		}
		paneEdges(gtx, p)
	}
}

// viewFocused reports whether the view of pane id (0: the session) draws the
// focused cursor: only the one with the keyboard, while the window has focus.
func (win *window) viewFocused(id tabs.TabID) bool {
	return win.focused && win.fl.keys == id
}

// paneRend is pane id's own renderer: one renderer per drawn tab, because a
// renderer keeps that tab's strips and takes its screen's dirty rows.
func (win *window) paneRend(id tabs.TabID) *render.Renderer {
	if win.fl.rends == nil {
		win.fl.rends = map[tabs.TabID]*render.Renderer{}
	}
	r := win.fl.rends[id]
	if r == nil {
		r = render.NewRenderer(win.rendPx, win.pal, rendOptions(win.opt.Config))
		win.fl.rends[id] = r
	}
	return r
}

// syncVisible marks the drawn panes' tabs visible, so their output asks for a
// frame, and unmarks the rest; a tab no longer drawn loses its renderer.
func (win *window) syncVisible(views []paneView) {
	now := make([]tabs.TabID, 0, len(views))
	for _, pv := range views {
		now = append(now, pv.p.id)
	}
	for _, id := range win.fl.shown {
		if !slices.Contains(now, id) {
			win.mgr.SetVisible(id, false)
			delete(win.fl.rends, id)
		}
	}
	for _, id := range now {
		if !slices.Contains(win.fl.shown, id) {
			win.mgr.SetVisible(id, true)
			// One more frame, so a pane drawn for the first time has its input
			// filters up before the owner's next click arrives.
			win.w.Invalidate()
			if id == win.fl.keys {
				win.fl.refocus = true // Gio ignores focus on a tag with no filters yet
			}
		}
	}
	for id := range win.fl.rends {
		if !slices.Contains(now, id) {
			delete(win.fl.rends, id)
		}
	}
	win.fl.shown, win.fl.views = now, views
	for _, s := range win.fl.sets {
		for _, p := range s.panes {
			if !slices.Contains(now, p.id) {
				p.hits = paneHits{}
			}
		}
	}
}

// scratchGlyph is a small terminal prompt centred in r: a chevron and an underscore.
func scratchGlyph(gtx layout.Context, r image.Rectangle) clip.Op {
	s := float32(min(r.Dx(), r.Dy(), gtx.Dp(28)))
	cx, cy := float32(r.Min.X)+float32(r.Dx())/2, float32(r.Min.Y)+float32(r.Dy())/2
	u, w := s*0.085, max(float32(gtx.Dp(1))*1.4, 1)
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx-3*u, cy-2*u))
	p.LineTo(f32.Pt(cx-u, cy))
	p.LineTo(f32.Pt(cx-3*u, cy+2*u))
	p.MoveTo(f32.Pt(cx, cy+2.2*u))
	p.LineTo(f32.Pt(cx+3*u, cy+2.2*u))
	return clip.Stroke{Path: p.End(), Width: w}.Op()
}
