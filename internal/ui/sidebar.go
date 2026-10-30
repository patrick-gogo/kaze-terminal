package ui

import (
	"image"
	"image/color"
	"math"
	"path/filepath"
	"strconv"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// sidebarW is the sidebar's width in dp at 100% scale.
const sidebarW = 260

// sideGroupH and sideRowH are a group header's and a session row's height in dp.
const sideGroupH, sideRowH = 28, 60

// maxSideText bounds how much of one title is shaped; the rest never fits 260dp anyway.
const maxSideText = 160

// sideState is what the window knows that the model does not.
type sideState struct {
	shown      tabs.TabID      // highlighted row or member
	settings   bool            // the pinned "Settings" row is open
	settingsOn bool            // and shown
	hover      tabs.TabID      // row showing its ×
	scroll     int             // px the session list is scrolled
	filter     string          // as typed, for the placeholder/clear state
	ed         *widget.Editor  // the filter's editor; nil draws the text without a caret
	renaming   tabs.TabID      // the row whose title is ren, 0 for none
	ren        *widget.Editor  // the rename editor
	buttons    bool            // macOS: toggle and gear sit in the sidebar's top row
	runFolded  map[uint64]bool // workspace runs whose header is folded
}

type sideRowHit struct {
	id          tabs.TabID
	rect, close image.Rectangle
	title       image.Rectangle // a session row's title line, where a double click renames
	mark        mark            // the alert dot a member line drew
}

type sideKeyHit struct {
	key  string
	rect image.Rectangle
}

type sideRunHit struct {
	run  uint64
	rect image.Rectangle
	mark mark // the alert dot a folded header drew
}

// sideHits is where the last sidebar frame drew things, for clicks and the wheel.
type sideHits struct {
	rows                    []sideRowHit
	groups                  []sideKeyHit
	members                 []sideRowHit
	runs                    []sideRunHit
	settings, settingsClose image.Rectangle
	plus, filter            image.Rectangle
	toggle, gear            image.Rectangle
	list                    image.Rectangle // the scrollable area, for wheel events
	rename                  image.Rectangle // the rename editor, empty while none is drawn
	scrollMax               int
	noMatches               bool // the filter hides every session
}

// fitSide draws s at size within room px, cut with a trailing "…" when it is longer.
func (c *chrome) fitSide(gtx layout.Context, s string, size unit.Sp, room int, col color.NRGBA) (op.CallOp, image.Point) {
	s = cutRunes(c.fit(s), maxSideText)
	call, sz := c.label(gtx, s, size, col)
	for i := 0; i < 6 && sz.X > room && s != ""; i++ {
		n := utf8.RuneCountInString(s)
		keep := min(n*max(room, 0)/max(sz.X, 1), n-1)
		if keep < 2 {
			s = ""
		} else {
			s = cutRunes(s, keep)
		}
		call, sz = c.label(gtx, s, size, col)
	}
	if sz.X > room {
		call, sz = c.label(gtx, "", size, col)
		sz.X = 0
	}
	return call, sz
}

// sideDot draws a state mark: an amber dot with a soft halo for needs you, green for
// done, an accent ring for running, muted otherwise.
func (c *chrome) sideDot(gtx layout.Context, d image.Rectangle, mk mark, running bool) {
	ops := gtx.Ops
	if running && mk == markNone {
		paint.FillShape(ops, c.th.Accent, runningRing(gtx, d))
		return
	}
	col, ok := c.dotColor(false, mk)
	if !ok {
		col = c.th.Muted
	}
	if mk == markNeeds {
		h := gtx.Dp(3)
		halo := col
		halo.A = 0x40
		paint.FillShape(ops, halo, clip.Ellipse(image.Rect(d.Min.X-h, d.Min.Y-h, d.Max.X+h, d.Max.Y+h)).Op(ops))
	}
	paint.FillShape(ops, col, clip.Ellipse(d).Op(ops))
}

// runningRing is an open ring around d, its gap at the top. It is drawn still, so a
// session stuck in running never costs a frame.
func runningRing(gtx layout.Context, d image.Rectangle) clip.Op {
	sw := float32(max(gtx.Dp(2), 1))
	r := float32(d.Dx())/2 + sw/2
	cx, cy := float32(d.Min.X)+float32(d.Dx())/2, float32(d.Min.Y)+float32(d.Dy())/2
	ctr := f32.Pt(cx, cy)
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(cx+r*0.7071, cy-r*0.7071)) // top right, then round through the bottom
	p.ArcTo(ctr, ctr, 3*math.Pi/2)
	return clip.Stroke{Path: p.End(), Width: sw}.Op()
}

// toggleGlyph is the sidebar icon: a window outline with a divider.
func toggleGlyph(gtx layout.Context, r image.Rectangle) clip.Op {
	sw := float32(max(gtx.Dp(1), 1))
	n := gtx.Dp(14)
	x0 := float32(r.Min.X + (r.Dx()-n)/2)
	y0 := float32(r.Min.Y + (r.Dy()-n)/2)
	s, h := float32(n), sw/2
	var p clip.Path
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(x0+h, y0+h))
	p.LineTo(f32.Pt(x0+s-h, y0+h))
	p.LineTo(f32.Pt(x0+s-h, y0+s-h))
	p.LineTo(f32.Pt(x0+h, y0+s-h))
	p.Close()
	p.MoveTo(f32.Pt(x0+s*0.38, y0+h))
	p.LineTo(f32.Pt(x0+s*0.38, y0+s-h))
	return clip.Stroke{Path: p.End(), Width: sw}.Op()
}

// layoutSidebar paints the sidebar into size and returns where its parts are. The
// session list scrolls inside hits.list; the workspaces section is pinned to the bottom.
func (c *chrome) layoutSidebar(gtx layout.Context, m sideModel, st sideState, size image.Point) sideHits {
	ops, th := gtx.Ops, c.th
	var h sideHits
	paint.FillShape(ops, th.Chrome, clip.Rect{Max: size}.Op())
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(size.X-1, 0, size.X, size.Y)).Op())
	root := clip.Rect{Max: size}.Push(ops)
	defer root.Pop()

	pad, gap := gtx.Dp(8), gtx.Dp(8)
	hitPad := gtx.Dp(4)
	y := 0

	if st.buttons {
		bar := gtx.Dp(36)
		btn := gtx.Dp(28)
		h.toggle = image.Rect(pad, (bar-btn)/2, pad+btn, (bar-btn)/2+btn)
		h.gear = image.Rect(size.X-pad-btn, (bar-btn)/2, size.X-pad, (bar-btn)/2+btn)
		fillGlyph(ops, th.CapGlyph, h.toggle, toggleGlyph(gtx, h.toggle))
		fillGlyph(ops, th.CapGlyph, h.gear, captionGlyph(gtx, capGear, h.gear, false))
		y = bar
	}

	// Filter field and the + button.
	fieldH := gtx.Dp(30)
	plusW := gtx.Dp(30)
	h.plus = image.Rect(size.X-pad-plusW, y+pad, size.X-pad, y+pad+fieldH)
	h.filter = image.Rect(pad, y+pad, h.plus.Min.X-gap/2, y+pad+fieldH)
	frr := clip.UniformRRect(h.filter, gtx.Dp(7))
	paint.FillShape(ops, th.Border, clip.Stroke{Path: frr.Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
	ftxt, fcol := "Filter sessions…", th.Muted
	if st.filter != "" {
		ftxt, fcol = st.filter, th.ActiveFG
	}
	if st.ed == nil {
		fCall, fSize := c.fitSide(gtx, ftxt, 12, h.filter.Dx()-2*gap, fcol)
		place(ops, fCall, image.Pt(h.filter.Min.X+gap, h.filter.Min.Y+(fieldH-fSize.Y)/2))
	} else {
		c.lineEditor(gtx, st.ed, h.filter, ftxt, fcol, gap)
	}
	prr := clip.UniformRRect(h.plus, gtx.Dp(7))
	paint.FillShape(ops, th.Border, clip.Stroke{Path: prr.Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
	pCall, pSize := c.label(gtx, "+", 14, th.Muted)
	place(ops, pCall, image.Pt(h.plus.Min.X+(plusW-pSize.X)/2, h.plus.Min.Y+(fieldH-pSize.Y)/2))
	y += pad + fieldH + pad

	// The pinned Settings row.
	closeCall, closeSize := c.label(gtx, "×", 12, th.Muted)
	if st.settings {
		rowH := gtx.Dp(28)
		r := image.Rect(pad, y, size.X-pad, y+rowH)
		fg := th.Muted
		if st.settingsOn {
			fg = th.ActiveFG
			paint.FillShape(ops, th.Active, clip.UniformRRect(r, gtx.Dp(7)).Op(ops))
		}
		call, sz := c.label(gtx, "Settings", 12, fg)
		place(ops, call, image.Pt(r.Min.X+gap, r.Min.Y+(rowH-sz.Y)/2))
		cx := r.Max.X - gap - closeSize.X
		place(ops, closeCall, image.Pt(cx, r.Min.Y+(rowH-closeSize.Y)/2))
		h.settings = r
		h.settingsClose = image.Rect(cx-hitPad, r.Min.Y, r.Max.X, r.Max.Y)
		y += rowH + gtx.Dp(4)
	}

	// The workspaces section takes at most half the sidebar, from the bottom.
	secTop := size.Y
	if len(m.Runs) > 0 {
		secH := gtx.Dp(26)
		for _, run := range m.Runs {
			secH += gtx.Dp(26)
			if !st.runFolded[run.Run] {
				secH += len(run.Members) * gtx.Dp(24)
			}
		}
		secH = min(secH+gtx.Dp(6), max(size.Y/2, 0))
		secTop = size.Y - secH
	}
	h.list = image.Rect(0, min(y, secTop), size.X-1, secTop)

	c.sideList(gtx, &h, m, st, closeCall, closeSize)
	if len(m.Runs) > 0 {
		c.sideRuns(gtx, &h, m, st, secTop, size, closeCall, closeSize)
	}
	return h
}

// lineEditor draws a sidebar text field's editor in r, gap in from its left, and ph
// while it is empty.
func (c *chrome) lineEditor(gtx layout.Context, ed *widget.Editor, r image.Rectangle, ph string, phCol color.NRGBA, gap int) {
	ops, th := gtx.Ops, c.th
	_, line := c.label(gtx, "Ag", 12, th.ActiveFG)
	at := image.Pt(r.Min.X+gap, r.Min.Y+(r.Dy()-line.Y)/2)
	if ed.Len() == 0 && ph != "" {
		call, _ := c.fitSide(gtx, ph, 12, r.Dx()-2*gap, phCol)
		place(ops, call, at)
	}
	rec := op.Record(ops)
	paint.ColorOp{Color: th.ActiveFG}.Add(ops)
	fg := rec.Stop()
	rec = op.Record(ops)
	paint.ColorOp{Color: tint(th.Accent, 0x55)}.Add(ops)
	sel := rec.Stop()
	eg := gtx
	eg.Constraints = layout.Exact(image.Pt(max(r.Dx()-2*gap, 1), line.Y))
	t := op.Offset(at).Push(ops)
	ed.Layout(eg, c.shaper, font.Font{Typeface: chromeFace}, 12, fg, sel)
	t.Pop()
}

func fillGlyph(ops *op.Ops, col color.NRGBA, r image.Rectangle, glyph clip.Op) {
	cl := clip.Rect(r).Push(ops)
	paint.FillShape(ops, col, glyph)
	cl.Pop()
}

// sideList draws the scrolled groups and loose rows, clipped to h.list, and records hits
// for the visible parts only.
func (c *chrome) sideList(gtx layout.Context, h *sideHits, m sideModel, st sideState, closeCall op.CallOp, closeSize image.Point) {
	ops, th := gtx.Ops, c.th
	pad, gap, hitPad := gtx.Dp(8), gtx.Dp(8), gtx.Dp(4)
	groupH, rowH := gtx.Dp(sideGroupH), gtx.Dp(sideRowH)
	dot := gtx.Dp(7)

	total := 0
	for _, g := range m.Groups {
		total += groupH
		if !g.Folded {
			total += len(g.Rows) * rowH
		}
	}
	total += len(m.Loose) * rowH
	h.scrollMax = max(0, total-h.list.Dy())
	if total == 0 && st.filter != "" && !h.list.Empty() {
		h.noMatches = true
		call, sz := c.label(gtx, "No matches", 12, th.Muted)
		place(ops, call, image.Pt(h.list.Min.X+pad+gap, h.list.Min.Y+gtx.Dp(12)-sz.Y/2))
	}
	scroll := min(max(st.scroll, 0), h.scrollMax)

	if h.list.Empty() {
		return
	}
	cl := clip.Rect(h.list).Push(ops)
	defer cl.Pop()
	y := h.list.Min.Y - scroll

	row := func(r sideRow) {
		rect := image.Rect(pad, y, h.list.Max.X-pad, y+rowH)
		y += rowH
		if !rect.Overlaps(h.list) {
			return
		}
		on := r.ID == st.shown && !st.settingsOn
		fg := th.Folder
		if on {
			paint.FillShape(ops, th.Active, clip.UniformRRect(rect, gtx.Dp(7)).Op(ops))
			paint.FillShape(ops, th.Accent, clip.UniformRRect(image.Rect(rect.Min.X, rect.Min.Y+gtx.Dp(8), rect.Min.X+gtx.Dp(2), rect.Max.Y-gtx.Dp(8)), gtx.Dp(1)).Op(ops))
			fg = th.ActiveFG
		}
		tx := rect.Min.X + gap + dot + gap
		c.sideDot(gtx, image.Rect(rect.Min.X+gap, rect.Min.Y+gtx.Dp(14)-dot/2, rect.Min.X+gap+dot, rect.Min.Y+gtx.Dp(14)-dot/2+dot), r.Mark, r.Running)
		room := rect.Max.X - gap - closeSize.X - gap - tx
		titleBand := image.Rect(tx, rect.Min.Y+gtx.Dp(4), tx+max(room, 0), rect.Min.Y+gtx.Dp(24))
		word := r.stateWord()
		wcol := th.Accent
		switch r.Mark {
		case markNeeds:
			wcol = th.Needs
		case markDone:
			wcol = th.Done
		}
		var wCall op.CallOp
		var wSize image.Point
		wordW := 0
		if word != "" {
			wCall, wSize = c.label(gtx, word, 11, wcol)
			wordW = wSize.X
		}
		for i, ln := range []struct {
			s    string
			size unit.Sp
			col  color.NRGBA
			mid  unit.Dp
		}{
			{r.Title, 12, fg, 14},
			{r.Folder, 11, th.Muted, 31},
			{r.Branch, 11, th.Branch, 46},
		} {
			if ln.s == "" {
				continue
			}
			if i == 0 {
				c.rowTitle(gtx, r, st, h, titleBand, fg)
				continue
			}
			rm := room
			if i > 0 {
				rm = rect.Max.X - gap - tx // only the title shares its line with the ×
			}
			if i == 2 && word != "" {
				rm -= wordW + gap
			}
			call, sz := c.fitSide(gtx, ln.s, ln.size, rm, ln.col)
			place(ops, call, image.Pt(tx, rect.Min.Y+gtx.Dp(ln.mid)-sz.Y/2))
		}
		if word != "" {
			place(ops, wCall, image.Pt(rect.Max.X-gap-wordW, rect.Min.Y+gtx.Dp(46)-wSize.Y/2))
		}
		hit := sideRowHit{id: r.ID, rect: rect.Intersect(h.list), title: titleBand.Intersect(h.list)}
		if st.hover == r.ID && r.ID != 0 {
			cx := rect.Max.X - gap - closeSize.X
			cmid := rect.Min.Y + gtx.Dp(14)
			place(ops, closeCall, image.Pt(cx, cmid-closeSize.Y/2))
			hit.close = image.Rect(cx-hitPad, cmid-closeSize.Y/2-hitPad, rect.Max.X, cmid+closeSize.Y/2+hitPad).Intersect(h.list)
		}
		h.rows = append(h.rows, hit)
	}

	for _, g := range m.Groups {
		hd := image.Rect(pad, y, h.list.Max.X-pad, y+groupH)
		y += groupH
		if hd.Overlaps(h.list) {
			c.sideGroupHeader(gtx, g, st, hd)
			h.groups = append(h.groups, sideKeyHit{key: g.Key, rect: hd.Intersect(h.list)})
		}
		if g.Folded {
			continue
		}
		for _, r := range g.Rows {
			row(r)
		}
	}
	for _, r := range m.Loose {
		row(r)
	}
}

// rowTitle draws a row's first line in band: the title, then " · /command" in the
// theme's magenta; while the row is renamed, the rename editor instead.
func (c *chrome) rowTitle(gtx layout.Context, r sideRow, st sideState, h *sideHits, band image.Rectangle, fg color.NRGBA) {
	ops, th := gtx.Ops, c.th
	if r.ID == st.renaming && st.ren != nil {
		field := image.Rect(band.Min.X-gtx.Dp(4), band.Min.Y, band.Max.X, band.Max.Y)
		rr := clip.UniformRRect(field, gtx.Dp(4))
		paint.FillShape(ops, th.Chrome, rr.Op(ops))
		paint.FillShape(ops, th.Accent, clip.Stroke{Path: rr.Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
		c.lineEditor(gtx, st.ren, field, "", th.Muted, gtx.Dp(4))
		h.rename = field.Intersect(h.list)
		return
	}
	mid := band.Min.Y + band.Dy()/2
	room := band.Dx()
	cmd, mag := " · "+r.Command, c.memberColor("magenta")
	var cCall op.CallOp
	var cSize image.Point
	if r.Command != "" {
		cCall, cSize = c.fitSide(gtx, cmd, 12, room, mag)
	}
	// The command keeps at most half the line while the title needs the rest.
	tCall, tSize := c.fitSide(gtx, r.Title, 12, room-min(cSize.X, room/2), fg)
	place(ops, tCall, image.Pt(band.Min.X, mid-tSize.Y/2))
	if r.Command != "" {
		if cSize.X > room-tSize.X {
			cCall, cSize = c.fitSide(gtx, cmd, 12, room-tSize.X, mag)
		}
		place(ops, cCall, image.Pt(band.Min.X+tSize.X, mid-cSize.Y/2))
	}
}

// sideGroupHeader draws one repo header: the dot stack, the name and the count, or the
// amber "N waiting"; a folded group also names the shown session inside it.
func (c *chrome) sideGroupHeader(gtx layout.Context, g sideGroup, st sideState, hd image.Rectangle) {
	ops, th := gtx.Ops, c.th
	gap := gtx.Dp(8)
	dot, step := gtx.Dp(7), gtx.Dp(5)
	mid := hd.Min.Y + hd.Dy()/2

	cx := hd.Min.X + gap
	chev := "▾"
	if g.Folded {
		chev = "›"
	}
	cCall, cSize := c.label(gtx, chev, 10, th.Muted)
	place(ops, cCall, image.Pt(cx, mid-cSize.Y/2))
	cx += cSize.X + gap

	dots := min(len(g.Rows), maxStackDots)
	for i := 0; i < dots; i++ {
		c.sideDot(gtx, image.Rect(cx+i*step, mid-dot/2, cx+i*step+dot, mid-dot/2+dot), g.Rows[i].Mark, g.Rows[i].Running)
	}
	if dots > 0 {
		cx += dot + (dots-1)*step + gap
	}

	right, rcol := strconv.Itoa(g.Count), th.Muted
	if g.Waiting > 0 {
		right, rcol = strconv.Itoa(g.Waiting)+" waiting", th.Needs
	}
	rCall, rSize := c.label(gtx, right, 11, rcol)
	rx := hd.Max.X - gap - rSize.X
	place(ops, rCall, image.Pt(rx, mid-rSize.Y/2))

	name := g.Name
	if g.Folded {
		for _, r := range g.Rows {
			if r.ID == st.shown && !st.settingsOn {
				name += " › " + cutRunes(r.Title, maxMemberLabel)
				break
			}
		}
	}
	nCall, nSize := c.fitSide(gtx, name, 12, rx-gap-cx, th.ActiveFG)
	place(ops, nCall, image.Pt(cx, mid-nSize.Y/2))
}

// sideRuns draws the workspaces section from secTop to the bottom: a header, then per
// run its name with all up / N down and one line per member, with its alert dot and,
// while hovered, its ×.
func (c *chrome) sideRuns(gtx layout.Context, h *sideHits, m sideModel, st sideState, secTop int, size image.Point, closeCall op.CallOp, closeSize image.Point) {
	ops, th := gtx.Ops, c.th
	pad, gap, hitPad := gtx.Dp(8), gtx.Dp(8), gtx.Dp(4)
	hdH, memH := gtx.Dp(26), gtx.Dp(24)
	right := size.X - 1
	area := image.Rect(0, secTop, right, size.Y)
	paint.FillShape(ops, th.Chrome, clip.Rect(area).Op())
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(0, secTop, right, secTop+1)).Op())
	cl := clip.Rect(area).Push(ops)
	defer cl.Pop()

	y := secTop
	call, sz := c.label(gtx, "WORKSPACES", 10, th.Muted)
	place(ops, call, image.Pt(pad+gap, y+(hdH-sz.Y)/2))
	y += hdH
	whole := func(r image.Rectangle) bool { return r.Max.Y <= size.Y }

	remaining := 0 // members still to place, so a capped section can say how many are left
	for _, run := range m.Runs {
		if !st.runFolded[run.Run] {
			remaining += len(run.Members)
		}
	}
	hidden, moreY := 0, 0
	for _, run := range m.Runs {
		folded := st.runFolded[run.Run]
		hd := image.Rect(pad, y, right-pad, y+hdH)
		y += hdH
		if whole(hd) {
			state, scol := "all up", th.Done
			if run.Down > 0 {
				state, scol = strconv.Itoa(run.Down)+" down", th.UsageHigh
			}
			sCall, sSize := c.label(gtx, state, 11, scol)
			sx := hd.Max.X - gap - sSize.X
			place(ops, sCall, image.Pt(sx, hd.Min.Y+(hdH-sSize.Y)/2))
			hit := sideRunHit{run: run.Run, rect: hd}
			if folded && run.Mark != markNone {
				// Folded, the members' lines are gone, so the header carries their strongest mark.
				sx = c.alertDot(gtx, sx, hd.Min.Y+hdH/2, run.Mark)
				hit.mark = run.Mark
			}
			chev := "▾"
			if folded {
				chev = "›"
			}
			cCall, cSize := c.label(gtx, chev, 10, th.Muted)
			place(ops, cCall, image.Pt(hd.Min.X+gap, hd.Min.Y+(hdH-cSize.Y)/2))
			nx := hd.Min.X + gap + cSize.X + gap
			nCall, nSize := c.fitSide(gtx, run.Name, 12, sx-gap-nx, th.ActiveFG)
			place(ops, nCall, image.Pt(nx, hd.Min.Y+(hdH-nSize.Y)/2))
			h.runs = append(h.runs, hit)
		}
		if folded {
			continue
		}
		for _, mem := range run.Members {
			r := image.Rect(pad, y, right-pad, y+memH)
			y += memH
			remaining--
			// Keep the last slot for "+N more" while members remain after this one.
			limit := size.Y
			if remaining > 0 {
				limit -= memH
			}
			if r.Max.Y > limit {
				if hidden == 0 {
					moreY = r.Min.Y
				}
				hidden++
				continue
			}
			on := mem.ID != 0 && mem.ID == st.shown && !st.settingsOn
			if on {
				paint.FillShape(ops, th.Active, clip.UniformRRect(r, gtx.Dp(6)).Op(ops))
			}
			bar := image.Rect(r.Min.X+gap, r.Min.Y+gtx.Dp(5), r.Min.X+gap+gtx.Dp(3), r.Max.Y-gtx.Dp(5))
			paint.FillShape(ops, c.memberColor(mem.Color), clip.UniformRRect(bar, gtx.Dp(1)).Op(ops))
			scol := th.Muted
			if mem.Down {
				scol = th.UsageHigh
			} else if mem.Mark == markNeeds {
				scol = th.Needs
			}
			hit := sideRowHit{id: mem.ID, rect: r}
			end, mid := r.Max.X-gap, r.Min.Y+memH/2
			if st.hover == mem.ID && mem.ID != 0 {
				cx := end - closeSize.X
				place(ops, closeCall, image.Pt(cx, mid-closeSize.Y/2))
				hit.close = image.Rect(cx-hitPad, r.Min.Y, r.Max.X, r.Max.Y)
				end = cx - gap
			}
			sCall, sSize := c.label(gtx, mem.Status, 11, scol)
			sx := end - sSize.X
			place(ops, sCall, image.Pt(sx, mid-sSize.Y/2))
			if mem.Mark != markNone {
				sx = c.alertDot(gtx, sx, mid, mem.Mark)
				hit.mark = mem.Mark
			}
			tx := bar.Max.X + gap
			fg := th.Folder
			if on {
				fg = th.ActiveFG
			}
			tCall, tSize := c.fitSide(gtx, mem.Title, 12, sx-gap-tx, fg)
			place(ops, tCall, image.Pt(tx, mid-tSize.Y/2))
			if mem.ID != 0 {
				h.members = append(h.members, hit)
			}
		}
	}
	if hidden > 0 {
		mCall, mSize := c.label(gtx, "+"+strconv.Itoa(hidden)+" more", 11, th.Muted)
		place(ops, mCall, image.Pt(pad+gap, moreY+(memH-mSize.Y)/2))
	}
}

const (
	// maxMemberLabel bounds the "› title" a folded group header adds for the shown session.
	maxMemberLabel = 16
	// maxStackDots bounds a group header's dot stack, so a big group cannot widen it.
	maxStackDots = 8
)

// cutRunes shortens s to n runes with a trailing "…"; short text is not copied.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// shortFolder is the last two parts of a member's folder, enough to tell members apart.
func shortFolder(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	parent := filepath.Dir(dir)
	if parent == dir || filepath.Base(parent) == parent || parent == filepath.VolumeName(parent)+string(filepath.Separator) {
		return filepath.Base(dir)
	}
	return filepath.Join(filepath.Base(parent), filepath.Base(dir))
}

// memberColor maps a recipe color through the theme's ANSI colors.
func (c *chrome) memberColor(name string) color.NRGBA {
	switch name {
	case "red":
		return c.ansi[1]
	case "green":
		return c.ansi[2]
	case "yellow":
		return c.ansi[3]
	case "blue":
		return c.ansi[4]
	case "magenta":
		return c.ansi[5]
	case "cyan":
		return c.ansi[6]
	}
	return c.th.Muted
}

// badgeColor is an alert mark's color: amber while it needs you, green when done.
func (c *chrome) badgeColor(m mark) color.NRGBA {
	switch m {
	case markNeeds:
		return c.th.Needs
	case markDone:
		return c.th.Done
	}
	return c.th.Muted
}

// alertDot draws a mark's dot centred on mid, ending a gap left of right, and
// returns its left edge.
func (c *chrome) alertDot(gtx layout.Context, right, mid int, mk mark) int {
	d, gap := gtx.Dp(6), gtx.Dp(6)
	x := right - gap - d
	paint.FillShape(gtx.Ops, c.badgeColor(mk), clip.Ellipse(image.Rect(x, mid-d/2, x+d, mid-d/2+d)).Op(gtx.Ops))
	return x
}
