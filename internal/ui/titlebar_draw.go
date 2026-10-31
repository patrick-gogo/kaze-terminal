package ui

import (
	"image"
	"image/color"
	"runtime"
	"strconv"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// crumb is what the title bar says about the view in front (spec 5.1): the
// shown session by the sidebar's naming rule, or the page that covers it.
func (win *window) crumb() crumbParts {
	switch {
	case win.set.shown:
		return crumbParts{Title: "Settings"}
	case win.board.open:
		return crumbParts{Title: "Agents"}
	}
	t := win.mgr.Active()
	if t == nil {
		return crumbParts{}
	}
	title, repo := win.sessionName(t.ID)
	if repo == title {
		repo = ""
	}
	a := win.agentOf(t.ID)
	c := crumbParts{Repo: repo, Title: title, Cmd: crumbCommand(a.command)}
	switch {
	case win.alerts.markOf(t.ID) == markNeeds:
		c.State = "needs"
	case a.running:
		c.State = "running"
	case win.alerts.markOf(t.ID) == markDone:
		c.State = "done"
	}
	return c
}

// titleDimsFor is the bar's sizes at this scale.
func titleDimsFor(gtx layout.Context) titleDims {
	return titleDims{
		BarH: gtx.Dp(42), Btn: gtx.Dp(30), BtnH: gtx.Dp(28), CapW: gtx.Dp(46), Mark: gtx.Dp(26),
		Pad: gtx.Dp(14), LeadPad: gtx.Dp(8), Gap: gtx.Dp(8),
		SearchMin: gtx.Dp(190), SearchMax: gtx.Dp(400), SearchH: gtx.Dp(28), KeyPad: gtx.Dp(6),
	}
}

// drawBar lays out and draws the shared title bar (spec 5) and records its
// layout for input.
func (win *window) drawBar(gtx layout.Context, barH int) {
	ops, c, th := gtx.Ops, win.ui, win.ui.th
	size := image.Pt(gtx.Constraints.Max.X, barH)
	d := titleDimsFor(gtx)
	d.BarH = barH
	lights := 0
	if !win.custom {
		lights = int(win.lights.Inset()*float64(gtx.Metric.PxPerDp) + 0.5)
	}
	side := win.side.w
	key := formatShortcut(runtime.GOOS, win.binds["palette"])
	measure := func(s string, mono bool) int {
		face := font.Font{Typeface: sansFace, Weight: font.SemiBold}
		size := unit.Sp(12.5)
		if mono {
			face, size = font.Font{Typeface: chromeFace}, 11.5
		}
		return c.textWidth(gtx, s, size, face)
	}
	// Use the same font/locale invalidation as the status cache.
	if win.titleCache.locale != gtx.Locale {
		win.titleCache.valid = false
		win.titleCache.locale = gtx.Locale
	}
	b := win.titleCache.layout(titleIn{Size: size, Mac: !win.custom, LightsW: lights, SideW: side, Crumb: win.crumb(),
		Key: key, Dims: d, Measure: measure}, gtx.Metric, c.shaper)
	win.titleCache.font = c.shaper
	win.bar = b
	win.lights.setDrag(b.Drag, gtx.Metric.PxPerDp)

	// The bar, with the lead strip in the sidebar's colour over the sidebar.
	bg := mix(th.Chrome, win.pal.BG, 0.45)
	paint.FillShape(ops, bg, clip.Rect{Max: size}.Op())
	if side > 0 {
		paint.FillShape(ops, th.Chrome, clip.Rect(b.Lead).Op())
		paint.FillShape(ops, th.Border, clip.Rect(image.Rect(b.Lead.Max.X-1, 0, b.Lead.Max.X, size.Y)).Op())
	}
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(0, size.Y-1, size.X, size.Y)).Op())

	if m := b.Mark; !m.Empty() {
		px := gtx.Dp(18)
		if mop, img := markImage(px); img != nil {
			cl := clip.Rect(m).Push(ops)
			t := op.Offset(image.Pt(m.Min.X+(m.Dx()-px)/2, m.Min.Y+(m.Dy()-px)/2)).Push(ops)
			mop.Add(ops)
			paint.PaintOp{}.Add(ops)
			t.Pop()
			cl.Pop()
		}
	}
	win.drawCrumb(gtx, b)
	c.drawSearchField(gtx, b, key, win.caption.hover == capSearch)

	icon := func(btn captionButton, r image.Rectangle, name string) {
		if r.Empty() {
			return
		}
		fg := th.CapGlyph
		if bgc, ok := c.captionBG(btn, win.caption); ok {
			paint.FillShape(ops, bgc, clip.UniformRRect(r, gtx.Dp(7)).Op(ops))
			fg = th.ActiveFG
		}
		c.toolIcon(gtx, name, r, fg)
	}
	icon(capToggle, b.Toggle, "panel")
	icon(capPlus, b.Plus, "plus")
	icon(capScratch, b.Scratch, "terminal")
	icon(capAgents, b.Agents, "agents")
	icon(capGear, b.Gear, "cog")
	win.drawAgentsBadge(gtx, b.Agents)
	for i, r := range b.Buttons {
		if r.Empty() {
			continue
		}
		btn := captionButton(i + 1)
		fg := th.CapGlyph
		if bgc, ok := c.captionBG(btn, win.caption); ok {
			paint.FillShape(ops, bgc, clip.Rect(r).Op())
			fg = th.ActiveFG
		}
		cl := clip.Rect(r).Push(ops)
		paint.FillShape(ops, fg, captionGlyph(gtx, btn, r, win.maximized))
		cl.Pop()
	}
}

// drawAgentsBadge is the Agents button's count: running and waiting
// subagents, amber when anything waits on the owner.
func (win *window) drawAgentsBadge(gtx layout.Context, r image.Rectangle) {
	n := win.subs.Active(0)
	if n == 0 || r.Empty() {
		return
	}
	ops, c, th := gtx.Ops, win.ui, win.ui.th
	_, waiting, _ := win.subs.Board()
	bcol, fg := th.Accent, th.ActiveFG
	if len(waiting) > 0 || len(win.subs.SessionWaits()) > 0 {
		bcol = th.Needs
	}
	call, sz := c.sans(gtx, strconv.Itoa(min(n, 99)), 9.5, fg, font.Bold)
	h := gtx.Dp(15)
	w := max(h, sz.X+gtx.Dp(8))
	box := image.Rect(r.Max.X-w+gtx.Dp(2), r.Min.Y-gtx.Dp(1), r.Max.X+gtx.Dp(2), r.Min.Y-gtx.Dp(1)+h)
	ring := box.Inset(-gtx.Dp(2))
	paint.FillShape(ops, mix(th.Chrome, win.pal.BG, 0.45), clip.UniformRRect(ring, ring.Dy()/2).Op(ops))
	paint.FillShape(ops, bcol, clip.UniformRRect(box, box.Dy()/2).Op(ops))
	place(ops, call, image.Pt(box.Min.X+(box.Dx()-sz.X)/2, box.Min.Y+(box.Dy()-sz.Y)/2))
}

// drawCrumb draws the session crumb: state mark, repo / title, command, state pill.
func (win *window) drawCrumb(gtx layout.Context, b titleLayout) {
	r := b.Crumb
	if r.Empty() {
		return
	}
	ops, c, th := gtx.Ops, win.ui, win.ui.th
	cp := b.CrumbShow
	gap := gtx.Dp(8)
	mid := r.Min.Y + r.Dy()/2
	cl := clip.Rect(r).Push(ops)
	defer cl.Pop()
	x := r.Min.X
	if cp.State != "" {
		d := gtx.Dp(9)
		dot := image.Rect(x, mid-d/2, x+d, mid-d/2+d)
		mk := markNone
		switch cp.State {
		case "needs":
			mk = markNeeds
		case "done":
			mk = markDone
		}
		c.stateDot(gtx, dot, mk, cp.State == "running", false)
		x += gtx.Dp(10) + gap
	}
	text := func(s string, col color.NRGBA, w font.Weight) {
		call, sz := c.sans(gtx, s, 12.5, col, w)
		place(ops, call, image.Pt(x, mid-sz.Y/2))
		x += sz.X
	}
	if cp.Repo != "" {
		text(cp.Repo, th.Muted, font.Medium)
		x += gap
		text("/", th.Border, font.Medium)
		x += gap
	}
	pill := statePill(cp.State)
	pillW := 0
	if pill != "" {
		_, sz := c.sans(gtx, pill, 10.5, th.Accent, font.SemiBold)
		pillW = sz.X + 2*gtx.Dp(6) + gap
	}
	cmdW := 0
	if cp.Cmd != "" {
		_, sz := c.label(gtx, cp.Cmd, 11.5, th.Muted)
		cmdW = sz.X + gap
	}
	if cp.Title != "" {
		room := max(r.Max.X-x-cmdW-pillW, 0)
		call, sz := c.fitSideW(gtx, cp.Title, 12.5, room, th.ActiveFG, font.SemiBold)
		place(ops, call, image.Pt(x, mid-sz.Y/2))
		x += sz.X
	}
	if cp.Cmd != "" {
		x += gap
		call, sz := c.label(gtx, cp.Cmd, 11.5, c.memberColor("magenta"))
		place(ops, call, image.Pt(x, mid-sz.Y/2))
		x += sz.X
	}
	if pill != "" {
		x += gap
		col := th.Accent
		switch cp.State {
		case "needs":
			col = th.Needs
		case "done":
			col = th.Done
		}
		call, sz := c.sans(gtx, pill, 10.5, col, font.SemiBold)
		ph := sz.Y + gtx.Dp(4)
		box := image.Rect(x, mid-ph/2, x+sz.X+2*gtx.Dp(6), mid-ph/2+ph)
		paint.FillShape(ops, tint(col, 0x26), clip.UniformRRect(box, ph/2).Op(ops))
		place(ops, call, image.Pt(box.Min.X+gtx.Dp(6), mid-sz.Y/2))
	}
}

// drawSearchField is the search field that opens the palette: a card with a
// line, the search icon, the placeholder and the key cap.
func (c *chrome) drawSearchField(gtx layout.Context, b titleLayout, key string, hover bool) {
	r := b.Search
	if r.Empty() {
		return
	}
	ops, th := gtx.Ops, c.th
	line := th.Border
	if hover {
		line = th.Muted
	}
	rad := gtx.Dp(8)
	paint.FillShape(ops, mix(th.Chrome, th.Active, 0.5), clip.UniformRRect(r, rad).Op(ops))
	rr := clip.UniformRRect(r, rad)
	paint.FillShape(ops, line, clip.Stroke{Path: rr.Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
	is := gtx.Dp(30)
	c.toolIcon(gtx, "search", image.Rect(r.Min.X, r.Min.Y, r.Min.X+is, r.Max.Y).Inset(gtx.Dp(6)), th.Muted)
	right := r.Max.X - gtx.Dp(4)
	if !b.SearchIconOnly || b.SearchKey {
		call, sz := c.label(gtx, key, 10.5, th.Muted)
		pad := gtx.Dp(6)
		box := image.Rect(right-sz.X-2*pad, r.Min.Y+gtx.Dp(4), right, r.Max.Y-gtx.Dp(4))
		paint.FillShape(ops, th.Active, clip.UniformRRect(box, gtx.Dp(5)).Op(ops))
		br := clip.UniformRRect(box, gtx.Dp(5))
		paint.FillShape(ops, th.Border, clip.Stroke{Path: br.Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
		place(ops, call, image.Pt(box.Min.X+pad, box.Min.Y+(box.Dy()-sz.Y)/2))
		right = box.Min.X - gtx.Dp(8)
	}
	if !b.SearchIconOnly {
		call, sz := c.fitSideW(gtx, "Search sessions, skills, commands", 12, max(right-r.Min.X-is, 0), th.Muted, font.Normal)
		place(ops, call, image.Pt(r.Min.X+is, r.Min.Y+(r.Dy()-sz.Y)/2))
	}
}

// fitSideW is fitSide in the UI font at a weight: s cut with an ellipsis to room.
func (c *chrome) fitSideW(gtx layout.Context, s string, size unit.Sp, room int, col color.NRGBA, w font.Weight) (op.CallOp, image.Point) {
	call, sz := c.sans(gtx, s, size, col, w)
	if sz.X <= room {
		return call, sz
	}
	r := []rune(s)
	lo, hi := 0, len(r)-1 // the longest prefix that fits with an ellipsis
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if _, m := c.sans(gtx, string(r[:mid])+"…", size, col, w); m.X <= room {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	if lo > 0 {
		return c.sans(gtx, string(r[:lo])+"…", size, col, w)
	}
	return op.CallOp{}, image.Point{}
}

// crumbCommand is a command short enough for the title bar: a plugin's
// namespaced command shows only its own name, and anything longer than 24
// runes is cut with an ellipsis. The sidebar and palette keep the full name.
func crumbCommand(cmd string) string {
	if i := strings.LastIndexByte(cmd, ':'); i >= 0 && strings.HasPrefix(cmd, "/") {
		cmd = "/" + cmd[i+1:]
	}
	if r := []rune(cmd); len(r) > 24 {
		return string(r[:23]) + "…"
	}
	return cmd
}
