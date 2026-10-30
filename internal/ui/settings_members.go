package ui

import (
	"image"

	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// memberRow draws a workspace card: its name and state, a table of its tabs
// (dot, title, folder, commands, wait tag) with the columns lined up, and its
// buttons under the table.
func (p *page) memberRow(r image.Rectangle, y int, row settingRow) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, y, r.Max.X, y+1)).Op())
	ty := y + gtx.Dp(11)
	call, size := p.text(row.label, 13, col.fg)
	place(ops, call, image.Pt(r.Min.X, ty))
	ty += size.Y
	for _, l := range row.lines {
		call, size = p.text(l, 12, col.dim)
		place(ops, call, image.Pt(r.Min.X, ty))
		ty += size.Y
	}
	ty += gtx.Dp(6)

	const sz unit.Sp = 12
	dot, gap := gtx.Dp(8), gtx.Dp(16)
	titleW, folderW := 0, 0
	for _, m := range row.members {
		_, s := p.c.label(gtx, m.title, sz, col.fg)
		titleW = max(titleW, s.X)
		_, s = p.c.label(gtx, m.folder, sz, col.dim)
		folderW = max(folderW, s.X)
	}
	titleX := r.Min.X + dot + gtx.Dp(10)
	folderMax := r.Dx() * 3 / 10 // the commands matter more than the folder
	folderW = min(folderW, folderMax)
	folderX := titleX + titleW + gap
	cmdX := folderX + folderW + gap
	cmdW := max(r.Max.X-cmdX, gtx.Dp(80))
	for _, m := range row.members {
		_, lh := p.c.label(gtx, "M", sz, col.fg)
		mid := ty + lh.Y/2
		paint.FillShape(ops, p.c.memberColor(m.color), clip.Ellipse(image.Rect(r.Min.X, mid-dot/2, r.Min.X+dot, mid+dot/2)).Op(ops))
		call, _ = p.text(m.title, sz, col.fg)
		place(ops, call, image.Pt(titleX, ty))
		call, _ = p.text(p.fitStart(m.folder, sz, folderW), sz, col.dim)
		place(ops, call, image.Pt(folderX, ty))
		p.st.memberFolderX = append(p.st.memberFolderX, folderX)
		lines := p.chain(m.cmds, sz, cmdW)
		lineY := ty
		for i, l := range lines {
			call, size = p.text(l, sz, col.fg)
			place(ops, call, image.Pt(cmdX, lineY))
			if i < len(lines)-1 {
				lineY += size.Y
			}
		}
		if m.tag != "" {
			end := cmdX
			if len(lines) > 0 {
				_, s := p.c.label(gtx, lines[len(lines)-1], sz, col.fg)
				end += s.X + gtx.Dp(10)
			}
			_, ts := p.c.label(gtx, m.tag, 11, col.dim)
			if len(lines) > 0 && end+ts.X > r.Max.X {
				lineY += lh.Y
				end = cmdX
			}
			call, _ = p.text(m.tag, 11, col.dim)
			place(ops, call, image.Pt(end, lineY+(lh.Y-ts.Y)/2))
		}
		ty = lineY + lh.Y + gtx.Dp(4)
	}
	ty += gtx.Dp(8)
	x, h := r.Min.X, 0
	for _, k := range row.ctrls {
		call, size, from := p.ctrl(k, 0)
		at := image.Pt(x, ty)
		p.st.shift(from, len(p.st.controls), at)
		place(ops, call, at)
		x += size.X + gtx.Dp(8)
		h = max(h, size.Y)
	}
	return ty + h + gtx.Dp(11)
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
