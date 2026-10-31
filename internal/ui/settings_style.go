package ui

import (
	"image"
	"image/color"
	"math"
	"strings"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// The settings page's look (docs/design/settings-redesign-mockup.html): a menu
// with icons, rows in cards under small section labels, status pills.

// mix is a blended t of the way from a to b, for card and raised fills that
// follow any theme, light or dark.
func mix(a, b color.NRGBA, t float32) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return color.NRGBA{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: 0xff}
}

// textW is text at a weight.
func (p *page) textW(s string, size unit.Sp, col color.NRGBA, w font.Weight) (op.CallOp, image.Point) {
	p.st.texts = append(p.st.texts, s)
	return p.c.sans(p.gtx, s, size, col, w)
}

// mono is text in the terminal's font, for keys, paths, commands and versions.
func (p *page) mono(s string, size unit.Sp, col color.NRGBA) (op.CallOp, image.Point) {
	p.st.texts = append(p.st.texts, s)
	return p.c.label(p.gtx, s, size, col)
}

// sectionLabel draws a small uppercase label over a card and returns where the card starts.
func (p *page) sectionLabel(r image.Rectangle, y int, s string) int {
	call, size := p.textW(strings.ToUpper(s), 10.5, p.col.dim, font.SemiBold)
	place(p.gtx.Ops, call, image.Pt(r.Min.X+p.gtx.Dp(2), y))
	return y + size.Y + p.gtx.Dp(10)
}

// card draws rows inside one card from y and returns where the next thing starts.
func (p *page) card(r image.Rectangle, y int, rows []settingRow) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	pad := gtx.Dp(18)
	inner := image.Rect(r.Min.X+pad, y, r.Max.X-pad, y)
	rec := op.Record(ops)
	yy := y
	for k, row := range rows {
		if k > 0 && !row.preview {
			paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, yy, r.Max.X, yy+1)).Op())
		}
		switch {
		case row.preview:
			yy = p.preview(inner, yy)
		case row.block:
			yy = p.blockRow(inner, yy+gtx.Dp(14), row) + gtx.Dp(6)
		case len(row.members) > 0:
			yy = p.memberRow(inner, yy, row)
		default:
			yy = p.row(inner, yy, row)
		}
	}
	call := rec.Stop()
	box := image.Rect(r.Min.X, y, r.Max.X, yy)
	radius := gtx.Dp(12)
	paint.FillShape(ops, col.card, clip.UniformRRect(box, radius).Op(ops))
	call.Add(ops)
	stroke(ops, box, radius, col.line)
	p.st.cards = append(p.st.cards, box)
	return yy
}

// pillColor is the color of a status pill's kind.
func (p *page) pillColor(kind string) color.NRGBA {
	switch kind {
	case "ok":
		return p.col.done
	case "warn":
		return p.col.needs
	case "run":
		return p.col.accent
	case "err":
		return p.col.err
	}
	return p.col.muted
}

// pill draws a status pill at the origin: a dot and its text on a faint fill of its color.
func (p *page) pill(text, kind string) (op.CallOp, image.Point) {
	gtx := p.gtx
	col := p.pillColor(kind)
	rec := op.Record(gtx.Ops)
	tc, ts := p.textW(text, 11.5, col, font.SemiBold)
	d, padX, gap := gtx.Dp(6), gtx.Dp(10), gtx.Dp(6)
	h := max(ts.Y+gtx.Dp(6), gtx.Dp(22))
	box := image.Rect(0, 0, padX+d+gap+ts.X+padX, h)
	paint.FillShape(gtx.Ops, tint(col, 0x1f), clip.UniformRRect(box, h/2).Op(gtx.Ops))
	paint.FillShape(gtx.Ops, col, clip.Ellipse(image.Rect(padX, (h-d)/2, padX+d, (h-d)/2+d)).Op(gtx.Ops))
	place(gtx.Ops, tc, image.Pt(padX+d+gap, (h-ts.Y)/2))
	return rec.Stop(), box.Size()
}

// iconTile draws a row's icon in a small rounded tile at the origin.
func (p *page) iconTile(name string) (op.CallOp, image.Point) {
	gtx := p.gtx
	rec := op.Record(gtx.Ops)
	s := gtx.Dp(34)
	box := image.Rect(0, 0, s, s)
	paint.FillShape(gtx.Ops, p.col.raised, clip.UniformRRect(box, gtx.Dp(9)).Op(gtx.Ops))
	stroke(gtx.Ops, box, gtx.Dp(9), p.col.line2)
	i := gtx.Dp(16)
	p.c.icon(gtx, name, image.Rect((s-i)/2, (s-i)/2, (s-i)/2+i, (s-i)/2+i), p.col.fg)
	return rec.Stop(), box.Size()
}

// iconNames are the icons chrome.icon draws.
var iconNames = map[string]bool{
	"appearance": true, "workspaces": true, "claude": true, "shortcuts": true, "updates": true,
	"about": true, "check": true, "alert": true, "prompt": true, "chart": true, "download": true,
	"panel": true, "search": true, "terminal": true, "cog": true, "agents": true, "plus": true, "folder": true, "branch": true, "chip": true, "tabs": true, "up": true,
}

// toolIcon draws one of the sidebar and title bar buttons' icons (panel,
// search, terminal, cog) at one optical size, centered in r, so the row reads
// as a set: same grid, same stroke.
func (c *chrome) toolIcon(gtx layout.Context, name string, r image.Rectangle, col color.NRGBA) {
	s := min(gtx.Dp(18), r.Dx(), r.Dy())
	x, y := r.Min.X+(r.Dx()-s)/2, r.Min.Y+(r.Dy()-s)/2
	c.icon(gtx, name, image.Rect(x, y, x+s, y+s), col)
}

// icon draws a 24-unit line icon scaled into r; an unknown name draws nothing.
func (c *chrome) icon(gtx layout.Context, name string, r image.Rectangle, col color.NRGBA) {
	if !iconNames[name] {
		return // before the path starts: an open path would break every op after it
	}
	ops := gtx.Ops
	k := float32(r.Dx()) / 24
	pt := func(x, y float32) f32.Point { return f32.Pt(float32(r.Min.X)+x*k, float32(r.Min.Y)+y*k) }
	width := max(float32(gtx.Dp(1))*1.6, 1)
	var p clip.Path
	p.Begin(ops)
	line := func(pts ...float32) {
		p.MoveTo(pt(pts[0], pts[1]))
		for i := 2; i+1 < len(pts); i += 2 {
			p.LineTo(pt(pts[i], pts[i+1]))
		}
	}
	rect := func(x, y, w, h float32) { line(x, y, x+w, y, x+w, y+h, x, y+h, x, y) }
	// rrect is a rectangle with rounded corners of radius rad.
	rrect := func(x, y, w, h, rad float32) {
		p.MoveTo(pt(x+rad, y))
		p.LineTo(pt(x+w-rad, y))
		p.QuadTo(pt(x+w, y), pt(x+w, y+rad))
		p.LineTo(pt(x+w, y+h-rad))
		p.QuadTo(pt(x+w, y+h), pt(x+w-rad, y+h))
		p.LineTo(pt(x+rad, y+h))
		p.QuadTo(pt(x, y+h), pt(x, y+h-rad))
		p.LineTo(pt(x, y+rad))
		p.QuadTo(pt(x, y), pt(x+rad, y))
	}
	circle := func(cx, cy, rad float32, sweep float64, from float64) {
		ctr := pt(cx, cy)
		p.MoveTo(f32.Pt(ctr.X+rad*k*float32(math.Cos(from)), ctr.Y+rad*k*float32(math.Sin(from))))
		p.ArcTo(ctr, ctr, float32(sweep))
	}
	var fill func()
	switch name {
	case "appearance":
		circle(12, 12, 9, 2*math.Pi, 0)
		fill = func() {
			var h clip.Path
			h.Begin(ops)
			ctr := pt(12, 12)
			h.MoveTo(pt(12, 3))
			h.ArcTo(ctr, ctr, -math.Pi)
			h.Close()
			paint.FillShape(ops, tint(col, 0x66), clip.Outline{Path: h.End()}.Op())
		}
	case "workspaces":
		rect(3, 4, 18, 6)
		rect(3, 14, 18, 6)
	case "claude":
		line(12, 3, 14.2, 9.8, 21, 12, 14.2, 14.2, 12, 21, 9.8, 14.2, 3, 12, 9.8, 9.8, 12, 3)
	case "shortcuts":
		rect(2.5, 6, 19, 12)
		line(6.5, 10, 7.5, 10)
		line(10.5, 10, 11.5, 10)
		line(14.5, 10, 15.5, 10)
		line(7, 14, 17, 14)
	case "updates":
		circle(12, 12, 9, 1.72*math.Pi, -0.22*math.Pi)
		line(21, 4, 21, 9, 16, 9)
	case "about":
		circle(12, 12, 9, 2*math.Pi, 0)
		line(12, 11, 12, 16)
		line(12, 7.6, 12, 8.4)
	case "check":
		line(5, 12, 10, 17, 19, 7)
	case "alert":
		circle(12, 12, 9, 2*math.Pi, 0)
		line(12, 8, 12, 12.5)
		line(12, 15.6, 12, 16.4)
	case "prompt":
		line(4, 6, 20, 6)
		line(4, 12, 14, 12)
		line(4, 18, 11, 18)
	case "chart":
		line(3, 17, 21, 17)
		line(6, 13, 9, 10, 12, 13, 17, 7)
	case "download":
		line(12, 4, 12, 16)
		line(6, 10, 12, 16, 18, 10)
		line(5, 20, 19, 20)
	case "panel":
		rrect(3, 4, 18, 16, 3)
		line(9.5, 4, 9.5, 20)
	case "search":
		circle(10.5, 10.5, 7.2, 2*math.Pi, 0)
		line(15.6, 15.6, 20.6, 20.6)
	case "folder":
		rrect(3, 6, 18, 13, 2)
		line(3, 9, 10, 9, 11.5, 6)
	case "branch":
		circle(7, 5.5, 2.2, 2*math.Pi, 0)
		circle(7, 18.5, 2.2, 2*math.Pi, 0)
		circle(17, 8.5, 2.2, 2*math.Pi, 0)
		line(7, 7.7, 7, 16.3)
		line(17, 10.7, 17, 12, 7, 15.5)
	case "chip":
		rrect(6, 6, 12, 12, 2)
		line(9, 3, 9, 6)
		line(15, 3, 15, 6)
		line(9, 18, 9, 21)
		line(15, 18, 15, 21)
		line(3, 9, 6, 9)
		line(3, 15, 6, 15)
		line(18, 9, 21, 9)
		line(18, 15, 21, 15)
	case "tabs":
		rrect(3, 7, 15, 13, 2)
		line(7, 4, 19, 4, 21, 6, 21, 15)
	case "up":
		line(12, 19, 12, 5)
		line(6.5, 10.5, 12, 5, 17.5, 10.5)
	case "plus":
		line(12, 5, 12, 19)
		line(5, 12, 19, 12)
	case "agents":
		circle(12, 6, 3, 2*math.Pi, 0)
		circle(6, 18, 3, 2*math.Pi, 0)
		circle(18, 18, 3, 2*math.Pi, 0)
		line(10.5, 8.6, 7.5, 15.4)
		line(13.5, 8.6, 16.5, 15.4)
	case "terminal":
		rrect(3, 4, 18, 16, 3)
		line(7.5, 9, 10.5, 12, 7.5, 15)
		line(12.5, 15, 16.5, 15)
	case "cog":
		// Eight teeth around a ring, and the hub.
		const teeth = 8
		ctr := pt(12, 12)
		at := func(a, rad float64) f32.Point {
			return f32.Pt(ctr.X+float32(rad*math.Cos(a))*k, ctr.Y+float32(rad*math.Sin(a))*k)
		}
		step := 2 * math.Pi / teeth
		for i := 0; i < teeth; i++ {
			a := float64(i)*step - math.Pi/2
			pts := []f32.Point{at(a-step*0.30, 7.2), at(a-step*0.17, 9.6), at(a+step*0.17, 9.6), at(a+step*0.30, 7.2)}
			if i == 0 {
				p.MoveTo(pts[0])
			} else {
				p.LineTo(pts[0])
			}
			for _, q := range pts[1:] {
				p.LineTo(q)
			}
		}
		p.Close()
		circle(12, 12, 3, 2*math.Pi, 0)
	}
	paint.FillShape(ops, col, clip.Stroke{Path: p.End(), Width: width}.Op())
	if fill != nil {
		fill()
	}
}

// preview draws a small terminal in the current theme, at the font size and
// with the cursor shape, under the theme cards.
func (p *page) preview(r image.Rectangle, y int) int {
	gtx, ops := p.gtx, p.gtx.Ops
	c := p.win.opt.Config
	t := startTheme(c)
	size := unit.Sp(min(max(c.FontSize, 8), 22) * 0.9) // capped, so a big font keeps the page usable
	box := image.Rect(r.Min.X, y, r.Max.X, y)
	bar := gtx.Dp(26)
	green, yellow := t.Term.ANSI[2], t.Term.ANSI[3]
	prompt := []any{t.UI.Accent, "todo-api ", green, "main ", t.Term.FG, "> "}
	lines := [][]any{
		append(append([]any{}, prompt...), t.Term.FG, "go test ./..."),
		{t.Term.FG, "ok   example.com/todo-api/api    0.21s"},
		{t.Term.FG, "ok   example.com/todo-api/store  0.08s"},
		{yellow, "! ", t.Term.FG, "1 todo left in store.go"},
		prompt,
	}
	rec := op.Record(ops)
	ly := y + bar + gtx.Dp(12)
	var lastX, lineH int
	for i, l := range lines {
		x := r.Min.X + gtx.Dp(14)
		h := 0
		for k := 0; k+1 < len(l); k += 2 {
			call, sz := p.mono(l[k+1].(string), size, l[k].(color.NRGBA))
			place(ops, call, image.Pt(x, ly))
			x += sz.X
			h = max(h, sz.Y)
		}
		if i == len(lines)-1 {
			lastX, lineH = x, h
		}
		ly += h + gtx.Dp(2)
	}
	// The cursor after the last prompt, in its shape.
	cw := max(lineH*6/10, 2)
	cy := ly - gtx.Dp(2) - lineH
	cur := image.Rect(lastX, cy, lastX+cw, cy+lineH)
	switch c.CursorStyle {
	case "bar":
		cur.Max.X = lastX + max(gtx.Dp(2), 1)
	case "underline":
		cur.Min.Y = cur.Max.Y - max(gtx.Dp(2), 1)
	}
	paint.FillShape(ops, t.Term.Cursor, clip.Rect(cur).Op())
	call := rec.Stop()
	box.Max.Y = ly + gtx.Dp(10)
	radius := gtx.Dp(10)
	paint.FillShape(ops, t.Term.BG, clip.UniformRRect(box, radius).Op(ops))
	stroke(ops, box, radius, p.col.line2)
	paint.FillShape(ops, p.col.line2, clip.Rect(image.Rect(box.Min.X, box.Min.Y+bar, box.Max.X, box.Min.Y+bar+1)).Op())
	for i := 0; i < 3; i++ {
		d := gtx.Dp(8)
		x := box.Min.X + gtx.Dp(12) + i*gtx.Dp(14)
		paint.FillShape(ops, tint(t.Term.FG, 0x40), clip.Ellipse(image.Rect(x, box.Min.Y+(bar-d)/2, x+d, box.Min.Y+(bar-d)/2+d)).Op(ops))
	}
	call.Add(ops)
	return box.Max.Y + gtx.Dp(18)
}
