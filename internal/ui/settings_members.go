package ui

import (
	"gioui.org/font"
	"gioui.org/op"
	"image"
	"strings"

	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// memberRow draws a workspace card: its name and state, a table of its tabs
// (dot, title, folder, commands, wait tag) with the columns lined up, and its
// buttons under the table.
func (p *page) memberRow(r image.Rectangle, y int, row settingRow) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col

	// The header: an initial badge, the name and tab count, the state pill and the buttons.
	headH := gtx.Dp(70)
	tile := gtx.Dp(38)
	ty := y + (headH-tile)/2
	tb := image.Rect(r.Min.X, ty, r.Min.X+tile, ty+tile)
	initial := "?"
	for _, c := range row.label {
		initial = strings.ToUpper(string(c))
		break
	}
	paint.FillShape(ops, tint(col.accent, 0x3a), clip.UniformRRect(tb, gtx.Dp(10)).Op(ops))
	stroke(ops, tb, gtx.Dp(10), tint(col.accent, 0x70))
	ic, is := p.textW(initial, 15, col.title, font.Bold)
	place(ops, ic, image.Pt(tb.Min.X+(tile-is.X)/2, tb.Min.Y+(tile-is.Y)/2))
	nameX := tb.Max.X + gtx.Dp(14)

	type laid struct {
		call     op.CallOp
		size     image.Point
		from, to int
	}
	var cs []laid
	w, h := 0, 0
	if row.pill != "" {
		call, size := p.pill(row.pill, row.tone)
		cs = append(cs, laid{call, size, len(p.st.controls), len(p.st.controls)})
		w, h = size.X, size.Y
	}
	for _, k := range row.ctrls {
		call, size, from := p.ctrl(k, 0)
		cs = append(cs, laid{call, size, from, len(p.st.controls)})
		if w > 0 {
			w += gtx.Dp(8)
		}
		w += size.X
		h = max(h, size.Y)
	}
	x := r.Max.X - w
	for _, c := range cs {
		at := image.Pt(x, y+(headH-c.size.Y)/2)
		p.st.shift(c.from, c.to, at)
		place(ops, c.call, at)
		x += c.size.X + gtx.Dp(8)
	}
	nameW := max(r.Max.X-w-gtx.Dp(16)-nameX, 1)
	nc, ns := p.textW(p.fitSans(row.label, 15, nameW, font.SemiBold), 15, col.title, font.SemiBold)
	sub := strings.Join(row.lines, " · ")
	sc, ss := p.text(sub, 12, col.muted)
	place(ops, nc, image.Pt(nameX, y+(headH-ns.Y-ss.Y)/2))
	place(ops, sc, image.Pt(nameX, y+(headH-ns.Y-ss.Y)/2+ns.Y))
	y += headH
	if len(row.members) == 0 {
		return y
	}

	// The members, under a hairline across the whole card.
	pad := gtx.Dp(18)
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X-pad, y, r.Max.X+pad, y+1)).Op())
	const sz unit.Sp = 12
	bar, gap := gtx.Dp(4), gtx.Dp(16)
	tagW := 0
	titleW, folderW := 0, 0
	for _, m := range row.members {
		_, s := p.c.sans(gtx, m.title, 13, col.fg, font.SemiBold)
		titleW = max(titleW, s.X)
		_, s = p.c.label(gtx, m.folder, 11.5, col.dim)
		folderW = max(folderW, s.X)
		_, s = p.c.sans(gtx, m.tag, 11.5, col.muted, font.Normal)
		tagW = max(tagW, s.X)
	}
	titleX := r.Min.X + bar + gtx.Dp(14)
	// Reserve space for commands before assigning title and folder columns. A
	// valid long title must not push the entire command chain outside the pane.
	available := max(r.Max.X-titleX-tagW-3*gap, 1)
	titleW = min(titleW, max(available/4, 1))
	folderW = min(folderW, max(available/3, 1))
	folderX := titleX + titleW + gap
	cmdX := folderX + folderW + gap
	chipPad := gtx.Dp(8)
	cmdW := max(r.Max.X-tagW-gap-cmdX-2*chipPad, 1)
	y += gtx.Dp(6)
	for k, m := range row.members {
		if k > 0 {
			paint.FillShape(ops, tint(col.line, 0xb0), clip.Rect(image.Rect(r.Min.X-pad, y, r.Max.X+pad, y+1)).Op())
		}
		lines := p.chain(m.cmds, sz, cmdW)
		_, lh := p.c.label(gtx, "M", sz, col.fg)
		chipH := max(len(lines), 1)*lh.Y + gtx.Dp(6)
		rowH := max(chipH+gtx.Dp(16), gtx.Dp(44))
		mid := y + rowH/2
		paint.FillShape(ops, p.c.memberColor(m.color), clip.UniformRRect(image.Rect(r.Min.X, mid-gtx.Dp(13), r.Min.X+bar, mid+gtx.Dp(13)), bar/2).Op(ops))
		tc, ts := p.textW(p.fitSans(m.title, 13, titleW, font.SemiBold), 13, col.title, font.SemiBold)
		place(ops, tc, image.Pt(titleX, mid-ts.Y/2))
		fc, fs := p.mono(p.fitStart(m.folder, 11.5, folderW), 11.5, col.muted)
		place(ops, fc, image.Pt(folderX, mid-fs.Y/2))
		p.st.memberFolderX = append(p.st.memberFolderX, folderX)
		if len(lines) > 0 {
			chipW := 0
			for _, l := range lines {
				_, s := p.c.label(gtx, l, sz, col.fg)
				chipW = max(chipW, s.X)
			}
			chip := image.Rect(cmdX, mid-chipH/2, cmdX+chipW+2*chipPad, mid-chipH/2+chipH)
			paint.FillShape(ops, col.nav, clip.UniformRRect(chip, gtx.Dp(6)).Op(ops))
			stroke(ops, chip, gtx.Dp(6), col.line2)
			ly := chip.Min.Y + gtx.Dp(3)
			for _, l := range lines {
				call, size := p.mono(l, sz, mix(col.accent, col.title, 0.55))
				place(ops, call, image.Pt(cmdX+chipPad, ly))
				ly += size.Y
			}
		}
		if m.tag != "" {
			gc, gs := p.text(m.tag, 11.5, col.muted)
			place(ops, gc, image.Pt(r.Max.X-gs.X, mid-gs.Y/2))
		}
		y += rowH
	}
	return y + gtx.Dp(6)
}

// chain lays commands out as "a → b → c" lines no wider than w, breaking only
// between commands. A single command wider than w is cut at its end with "…".
func (p *page) chain(cmds []string, sz unit.Sp, w int) []string {
	var lines []string
	cur := ""
	_, arrow := p.c.label(p.gtx, " →", sz, p.col.fg)
	for i, c := range cmds {
		c = cleanText(c)
		room := w // a line the chain continues from keeps room for its " →"
		if i < len(cmds)-1 {
			room -= arrow.X
		}
		try := c
		if cur != "" {
			try = cur + " → " + c
		}
		if _, s := p.c.label(p.gtx, try, sz, p.col.fg); s.X > room && cur != "" {
			lines = append(lines, cur+" →")
			try = c
		}
		cur = p.fit(try, sz, room)
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// fitStart shortens s at its start with "…", keeping the end of a path, which tells folders apart.
func (p *page) fitStart(s string, sz unit.Sp, w int) string {
	if _, size := p.c.label(p.gtx, s, sz, p.col.fg); size.X <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[1:]
		if _, size := p.c.label(p.gtx, "…"+string(r), sz, p.col.fg); size.X <= w {
			break
		}
	}
	return "…" + string(r)
}

// fit shortens s at its end with "…" until it is no wider than w.
// fitSans is fit for text drawn in the UI font at weight.
func (p *page) fitSans(s string, sz unit.Sp, w int, weight font.Weight) string {
	if _, size := p.c.sans(p.gtx, s, sz, p.col.fg, weight); size.X <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if _, size := p.c.sans(p.gtx, string(r)+"…", sz, p.col.fg, weight); size.X <= w {
			break
		}
	}
	return string(r) + "…"
}

func (p *page) fit(s string, sz unit.Sp, w int) string {
	if _, size := p.c.label(p.gtx, s, sz, p.col.fg); size.X <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 1 {
		r = r[:len(r)-1]
		if _, size := p.c.label(p.gtx, string(r)+"…", sz, p.col.fg); size.X <= w {
			break
		}
	}
	return string(r) + "…"
}
