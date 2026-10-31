package ui

import (
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// The palette's size in dp at 100% scale (mockup .pal).
const (
	palW        = 560
	palTop      = 60
	palListMaxH = 380
	palRowH     = 28
	palHeadH    = 24
	palFieldH   = 44
	palFootH    = 30
)

const palPlaceholder = "Type to search waiting items, sessions and skills…"

const palFooter = "↑↓ move    Enter run in this session    Shift+Enter new session    Esc close"

var palHeaders = [...]string{palWaiting: "WAITING ON YOU", palSessions: "SESSIONS", palSkills: "SKILLS", palKaze: "KAZE"}

// palState is what the drawing needs from the window.
type palState struct {
	items       []palItem
	sel, scroll int
	hover       int
	reveal      bool // scroll the selected row into view
	ed          *widget.Editor
}

// palHits is where the last frame drew the palette, in window pixels.
type palHits struct {
	box, field, list image.Rectangle
	rows             []palRowHit // visible rows only
	headers          []string
	scroll           int // the scroll the list was drawn at
	scrollMax        int
}

type palRowHit struct {
	index int
	rect  image.Rectangle
}

// rowAt is the item index of the visible row under p.
func (h palHits) rowAt(p image.Point) (int, bool) {
	for _, r := range h.rows {
		if p.In(r.rect) {
			return r.index, true
		}
	}
	return 0, false
}

type palEntry struct {
	y, h   int
	header string
	index  int // item index; -1 for a header
}

// layoutPalette draws the palette over main, the main view, which it dims:
// the query field, the sectioned list and the key help. Only rows inside the
// list's visible part are laid out.
func (c *chrome) layoutPalette(gtx layout.Context, st palState, main image.Rectangle) palHits {
	ops, th := gtx.Ops, c.th
	var h palHits
	if main.Dx() <= 2 || main.Dy() <= 2 {
		return h
	}
	paint.FillShape(ops, tint(th.Chrome, 0x88), clip.Rect(main).Op())

	rowH, headH := gtx.Dp(palRowH), gtx.Dp(palHeadH)
	var entries []palEntry
	y := 0
	last := palSection(-1)
	for i, it := range st.items {
		if it.Section != last {
			last = it.Section
			entries = append(entries, palEntry{y: y, h: headH, header: palHeaders[it.Section], index: -1})
			y += headH
		}
		entries = append(entries, palEntry{y: y, h: rowH, index: i})
		y += rowH
	}
	if len(st.items) == 0 {
		entries = append(entries, palEntry{y: 0, h: headH, header: "NO MATCHES", index: -1})
		y = headH
	}
	total := y + gtx.Dp(4)

	w := min(gtx.Dp(palW), main.Dx()*92/100)
	x := main.Min.X + (main.Dx()-w)/2
	top := main.Min.Y + gtx.Dp(palTop)
	fieldH, footH := gtx.Dp(palFieldH), gtx.Dp(palFootH)
	listH := min(total, gtx.Dp(palListMaxH))
	if room := main.Max.Y - top - fieldH - 1 - footH - gtx.Dp(8); listH > room {
		// A short window: move the box up first, then shorten the list.
		top = max(main.Min.Y+gtx.Dp(4), top-(listH-room))
		listH = max(min(listH, main.Max.Y-top-fieldH-1-footH-gtx.Dp(4)), 0)
	}
	box := image.Rect(x, top, x+w, top+fieldH+1+listH+footH).Intersect(main)
	h.box = box
	radius, line := gtx.Dp(10), max(gtx.Dp(1), 1)
	paint.FillShape(ops, th.ToastLine, clip.UniformRRect(box, radius).Op(ops))
	paint.FillShape(ops, th.ToastBG, clip.UniformRRect(box.Inset(line), max(radius-line, 0)).Op(ops))
	pad := gtx.Dp(14)
	cl := clip.Rect(box).Push(ops)
	defer cl.Pop()

	// The query field.
	h.field = image.Rect(box.Min.X, box.Min.Y, box.Max.X, min(box.Min.Y+fieldH, box.Max.Y))
	const fieldSp unit.Sp = 13.5
	_, lh := c.label(gtx, "Ag", fieldSp, th.ToastFG)
	at := image.Pt(h.field.Min.X+pad, h.field.Min.Y+(h.field.Dy()-lh.Y)/2)
	if st.ed != nil {
		if st.ed.Len() == 0 {
			call, _ := c.fitSide(gtx, palPlaceholder, fieldSp, h.field.Dx()-2*pad, th.Muted)
			place(ops, call, at)
		}
		rec := op.Record(ops)
		paint.ColorOp{Color: th.ToastFG}.Add(ops)
		fg := rec.Stop()
		rec = op.Record(ops)
		paint.ColorOp{Color: tint(th.Accent, 0x55)}.Add(ops)
		sel := rec.Stop()
		eg := gtx
		eg.Constraints = layout.Exact(image.Pt(max(h.field.Dx()-2*pad, 1), max(lh.Y, 1)))
		t := op.Offset(at).Push(ops)
		st.ed.Layout(eg, c.shaper, font.Font{Typeface: chromeFace}, fieldSp, fg, sel)
		t.Pop()
	}
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(box.Min.X, h.field.Max.Y, box.Max.X, h.field.Max.Y+1)).Op())

	// The list.
	h.list = image.Rect(box.Min.X, h.field.Max.Y+1, box.Max.X, min(h.field.Max.Y+1+listH, box.Max.Y))
	h.scrollMax = max(total-h.list.Dy(), 0)
	sc := min(max(st.scroll, 0), h.scrollMax)
	if st.reveal {
		for _, e := range entries {
			if e.index == st.sel {
				top := e.y
				if e.index == firstInSection(st.items, e.index) {
					top -= headH // keep the section's header in view with its first row
				}
				sc = max(min(sc, top), e.y+e.h-h.list.Dy())
				sc = min(max(sc, 0), h.scrollMax)
			}
		}
	}
	h.scroll = sc
	lc := clip.Rect(h.list).Push(ops)
	for _, e := range entries {
		if e.header != "" {
			h.headers = append(h.headers, e.header)
		}
		ey := h.list.Min.Y + e.y - sc
		if ey+e.h <= h.list.Min.Y || ey >= h.list.Max.Y {
			continue
		}
		if e.index < 0 {
			call, sz := c.fitSide(gtx, e.header, 10.5, box.Dx()-2*pad, th.Muted)
			place(ops, call, image.Pt(box.Min.X+pad, ey+e.h-sz.Y-gtx.Dp(4)))
			continue
		}
		rr := image.Rect(box.Min.X+gtx.Dp(6), ey, box.Max.X-gtx.Dp(6), ey+e.h)
		c.palRow(gtx, st.items[e.index], rr, e.index == st.sel)
		h.rows = append(h.rows, palRowHit{index: e.index, rect: rr.Intersect(h.list)})
	}
	lc.Pop()

	// The key help.
	foot := image.Rect(box.Min.X, h.list.Max.Y, box.Max.X, box.Max.Y)
	if foot.Dy() > 0 {
		paint.FillShape(ops, th.Border, clip.Rect(image.Rect(foot.Min.X, foot.Min.Y, foot.Max.X, foot.Min.Y+1)).Op())
		call, sz := c.fitSide(gtx, palFooter, 11, foot.Dx()-2*pad, th.Muted)
		place(ops, call, image.Pt(foot.Min.X+pad, foot.Min.Y+(foot.Dy()-sz.Y)/2))
	}
	return h
}

func firstInSection(items []palItem, i int) int {
	for i > 0 && items[i-1].Section == items[i].Section {
		i--
	}
	return i
}

// palRow draws one item in r: its mark, name, " · /command", and the detail
// right-aligned in at most 60% of the row.
func (c *chrome) palRow(gtx layout.Context, it palItem, r image.Rectangle, selected bool) {
	ops, th := gtx.Ops, c.th
	if selected {
		paint.FillShape(ops, th.Active, clip.UniformRRect(r, gtx.Dp(6)).Op(ops))
	}
	const sz unit.Sp = 12.5
	pad := gtx.Dp(8)
	x := r.Min.X + pad
	mid := r.Min.Y + r.Dy()/2
	if it.Section == palWaiting || it.Section == palSessions {
		dot := gtx.Dp(8)
		d := image.Rect(x, mid-dot/2, x+dot, mid+dot/2)
		switch {
		case it.Member:
			paint.FillShape(ops, c.memberColor(it.Color), clip.Ellipse(d).Op(ops))
		case it.Section == palWaiting:
			paint.FillShape(ops, th.Needs, clip.Ellipse(d).Op(ops))
		default:
			c.sideDot(gtx, d, it.Mark, it.Running)
		}
		x += dot + gtx.Dp(10)
	}
	right := r.Max.X - pad
	detailW := 0
	if it.Detail != "" {
		call, dsz := c.fitSide(gtx, it.Detail, 11.5, (r.Dx()*60)/100, th.Muted)
		place(ops, call, image.Pt(right-dsz.X, mid-dsz.Y/2))
		detailW = dsz.X + gtx.Dp(16)
	}
	room := max(right-detailW-x, 0)
	fg := th.ToastFG
	if it.Section == palSkills {
		fg = c.memberColor("magenta")
	}
	if selected {
		fg = th.ActiveFG
		if it.Section == palSkills {
			fg = c.memberColor("magenta")
		}
	}
	call, nsz := c.fitSide(gtx, it.Name, sz, room, fg)
	place(ops, call, image.Pt(x, mid-nsz.Y/2))
	if command := agentCommand(it.Agent, it.Command); command != "" && nsz.X > 0 {
		call, csz := c.fitSide(gtx, " · "+command, sz, room-nsz.X, th.Muted)
		place(ops, call, image.Pt(x+nsz.X, mid-csz.Y/2))
	}
}

// layoutSearch draws the title bar's search box in the column r: a rounded
// box h high with the magnifier, its placeholder and the palette's key.
func (c *chrome) layoutSearch(gtx layout.Context, r image.Rectangle, h int, keyText string, hover bool) {
	ops, th := gtx.Ops, c.th
	box := image.Rect(r.Min.X, r.Min.Y+(r.Dy()-h)/2, r.Max.X, r.Min.Y+(r.Dy()-h)/2+h)
	border := th.Border
	if hover {
		border = th.Muted
	}
	rr := clip.UniformRRect(box, gtx.Dp(6))
	paint.FillShape(ops, border, clip.Stroke{Path: rr.Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
	gap := gtx.Dp(8)
	g := image.Rect(box.Min.X+gap/2, box.Min.Y, box.Min.X+gap/2+box.Dy(), box.Max.Y)
	c.toolIcon(gtx, "search", g.Inset(gtx.Dp(5)), th.Muted)
	right := box.Max.X - gap
	if keyText != "" {
		call, sz := c.fitSide(gtx, keyText, 11, box.Dx()/3, th.Muted)
		right -= sz.X
		place(ops, call, image.Pt(right, box.Min.Y+(box.Dy()-sz.Y)/2))
		right -= gap
	}
	call, sz := c.fitSide(gtx, "Search sessions, skills…", 12, max(right-g.Max.X, 0), th.Muted)
	place(ops, call, image.Pt(g.Max.X, box.Min.Y+(box.Dy()-sz.Y)/2))
}
