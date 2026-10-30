package ui

import (
	"image"
	"strings"

	"gioui.org/op/clip"
	"gioui.org/op/paint"
)

// wsDraw draws the Workspaces section: the list, or the open draft as two panes,
// the tab list on the left and the selected tab's details on the right. A page
// too narrow for both stacks the list above the details.
func wsDraw(p *page, r image.Rectangle, win *window) int {
	sec := workspacesSection()
	y := p.heading(r, sec.title, sec.hint)
	rows := wsRows(win) // also applies a folder picker's answer
	d := win.wsp.draft
	if d == nil {
		win.wsp.flagged = win.wsp.flagged[:0]
		return p.rows(r, y, rows)
	}
	gtx := p.gtx
	ed := win.editorParts(d)
	y = p.rows(r, y, ed.head)

	listW, gap, bar := gtx.Dp(200), gtx.Dp(20), gtx.Dp(3)
	two := r.Dx() >= gtx.Dp(560)
	list := image.Rect(r.Min.X, y, r.Max.X, r.Max.Y)
	if two {
		list.Max.X = r.Min.X + listW
	}
	listEnd := p.wsTabList(list, d, ed.errs, win)
	top := y
	detail := image.Rect(list.Max.X+gap, y, r.Max.X, r.Max.Y)
	if !two {
		top = listEnd + gtx.Dp(12)
		detail = image.Rect(r.Min.X+gtx.Dp(14), top, r.Max.X, r.Max.Y)
	}
	detail.Min.Y = top
	end := p.rows(detail, top, ed.tab)
	_, t := d.selected()
	stripe := image.Rect(detail.Min.X-gtx.Dp(12), top, detail.Min.X-gtx.Dp(12)+bar, end)
	paint.FillShape(gtx.Ops, p.c.memberColor(t.color), clip.UniformRRect(stripe, bar/2).Op(gtx.Ops))
	return p.rows(r, max(end, listEnd)+gtx.Dp(8), ed.foot)
}

// wsTabList draws one item per draft tab and + Add tab under them, and returns
// where it ends. A click on an item selects that tab (ws.sel, arg = its key).
func (p *page) wsTabList(r image.Rectangle, d *wsDraft, errs wsErrors, win *window) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1)).Op())
	y := r.Min.Y + gtx.Dp(8)
	itemH, pad, dot := gtx.Dp(44), gtx.Dp(10), gtx.Dp(9)
	win.wsp.flagged = win.wsp.flagged[:0]
	for _, t := range d.tabs {
		item := image.Rect(r.Min.X, y, r.Max.X, y+itemH)
		sel := t.key == d.sel
		if sel {
			paint.FillShape(ops, tint(col.accent, 0x26), clip.UniformRRect(item, gtx.Dp(7)).Op(ops))
		}
		mid := item.Min.Y + itemH/2
		dx := item.Min.X + pad
		paint.FillShape(ops, p.c.memberColor(t.color), clip.Ellipse(image.Rect(dx, mid-gtx.Dp(8)-dot/2, dx+dot, mid-gtx.Dp(8)+dot/2)).Op(ops))
		tx := dx + dot + gtx.Dp(8)
		right := item.Max.X - pad
		bad := len(errs.tab[t.key]) > 0 || errs.missing[t.key]
		if bad {
			win.wsp.flagged = append(win.wsp.flagged, t.key)
			right -= dot + gtx.Dp(6)
			paint.FillShape(ops, col.err, clip.Ellipse(image.Rect(right+gtx.Dp(6), mid-dot/2, right+gtx.Dp(6)+dot, mid+dot/2)).Op(ops))
		}
		name, fg := strings.TrimSpace(t.title), col.fg
		if name == "" {
			name, fg = "Untitled tab", col.dim
		}
		if sel {
			fg = col.title
		}
		cl := clip.Rect(image.Rect(tx, item.Min.Y, max(right, tx), item.Max.Y)).Push(ops)
		call, size := p.text(cleanText(name), 13, fg)
		place(ops, call, image.Pt(tx, mid-gtx.Dp(8)-size.Y/2))
		call, size = p.text(commandCount(t), 11, col.dim)
		place(ops, call, image.Pt(tx, mid+gtx.Dp(9)-size.Y/2))
		cl.Pop()
		p.hit(ctlButton, item, "ws.sel", t.key, false)
		y = item.Max.Y + gtx.Dp(2)
	}
	call, size, from := p.ctrl(addTabCtrl(d), 0)
	at := image.Pt(r.Min.X, y+gtx.Dp(8))
	p.st.shift(from, len(p.st.controls), at)
	place(ops, call, at)
	return at.Y + size.Y + gtx.Dp(8)
}

// commandCount is a tab's commands in words, counting only the ones typed in.
func commandCount(t *wsDraftTab) string {
	n := 0
	for _, c := range t.cmds {
		if strings.TrimSpace(c.text) != "" {
			n++
		}
	}
	if n == 0 {
		return "No commands"
	}
	return plural(n, "command")
}
