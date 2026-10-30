package ui

import (
	"image"
	"image/color"
	"strconv"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// sidebarW is the sidebar's width in dp at 100% scale.
const sidebarW = 260

// maxSideText bounds how much of one title is shaped; the rest never fits 260dp anyway.
const maxSideText = 160

// sideState is what the window knows that the model does not.
type sideState struct {
	shown      tabs.TabID // highlighted row or member
	settings   bool       // the pinned "Settings" row is open
	settingsOn bool       // and shown
	hover      tabs.TabID // row showing its ×
	scroll     int        // px the session list is scrolled
	filter     string     // as typed, for the placeholder/clear state
	buttons    bool       // macOS: toggle and gear sit in the sidebar's top row
}

type sideRowHit struct {
	id          tabs.TabID
	rect, close image.Rectangle
}

type sideKeyHit struct {
	key  string
	rect image.Rectangle
}

type sideRunHit struct {
	run  uint64
	rect image.Rectangle
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
	scrollMax               int

	// The colors the selected row and the sidebar were painted with; the theme test reads them.
	selFill, bg color.NRGBA
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
// done, muted otherwise.
func (c *chrome) sideDot(gtx layout.Context, d image.Rectangle, mk mark) {
	ops := gtx.Ops
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
	h.bg, h.selFill = th.Chrome, th.Active
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
	fCall, fSize := c.fitSide(gtx, ftxt, 12, h.filter.Dx()-2*gap, fcol)
	place(ops, fCall, image.Pt(h.filter.Min.X+gap, h.filter.Min.Y+(fieldH-fSize.Y)/2))
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
			secH += gtx.Dp(26) + len(run.Members)*gtx.Dp(24)
		}
		secH = min(secH+gtx.Dp(6), max(size.Y/2, 0))
		secTop = size.Y - secH
	}
	h.list = image.Rect(0, min(y, secTop), size.X-1, secTop)

	c.sideList(gtx, &h, m, st, closeCall, closeSize)
	if len(m.Runs) > 0 {
		c.sideRuns(gtx, &h, m, st, secTop, size)
	}
	return h
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
	groupH, rowH := gtx.Dp(28), gtx.Dp(60)
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
		c.sideDot(gtx, image.Rect(rect.Min.X+gap, rect.Min.Y+gtx.Dp(14)-dot/2, rect.Min.X+gap+dot, rect.Min.Y+gtx.Dp(14)-dot/2+dot), r.Mark)
		room := rect.Max.X - gap - closeSize.X - gap - tx
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
			rm := room
			if i > 0 {
				rm = rect.Max.X - gap - tx // only the title shares its line with the ×
			}
			call, sz := c.fitSide(gtx, ln.s, ln.size, rm, ln.col)
			place(ops, call, image.Pt(tx, rect.Min.Y+gtx.Dp(ln.mid)-sz.Y/2))
		}
		hit := sideRowHit{id: r.ID, rect: rect.Intersect(h.list)}
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
		c.sideDot(gtx, image.Rect(cx+i*step, mid-dot/2, cx+i*step+dot, mid-dot/2+dot), g.Rows[i].Mark)
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
// run its name with all up / N down and one line per member.
func (c *chrome) sideRuns(gtx layout.Context, h *sideHits, m sideModel, st sideState, secTop int, size image.Point) {
	ops, th := gtx.Ops, c.th
	pad, gap := gtx.Dp(8), gtx.Dp(8)
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

	for _, run := range m.Runs {
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
			nCall, nSize := c.fitSide(gtx, run.Name, 12, sx-gap-(hd.Min.X+gap), th.ActiveFG)
			place(ops, nCall, image.Pt(hd.Min.X+gap, hd.Min.Y+(hdH-nSize.Y)/2))
			h.runs = append(h.runs, sideRunHit{run: run.Run, rect: hd})
		}
		for _, mem := range run.Members {
			r := image.Rect(pad, y, right-pad, y+memH)
			y += memH
			if !whole(r) {
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
			sCall, sSize := c.label(gtx, mem.Status, 11, scol)
			sx := r.Max.X - gap - sSize.X
			place(ops, sCall, image.Pt(sx, r.Min.Y+(memH-sSize.Y)/2))
			tx := bar.Max.X + gap
			fg := th.Folder
			if on {
				fg = th.ActiveFG
			}
			tCall, tSize := c.fitSide(gtx, mem.Title, 12, sx-gap-tx, fg)
			place(ops, tCall, image.Pt(tx, r.Min.Y+(memH-tSize.Y)/2))
			h.members = append(h.members, sideRowHit{id: mem.ID, rect: r})
		}
	}
}
