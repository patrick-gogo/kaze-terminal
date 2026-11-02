package ui

import (
	"fmt"
	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"image"
	"sort"
)

func (win *window) drawDoc(gtx layout.Context, d *docView, r image.Rectangle) {
	ops := gtx.Ops
	paint.FillShape(ops, win.pal.BG, clip.Rect(r).Op())
	area := clip.Rect(r).Push(ops)
	defer area.Pop()
	event.Op(ops, &win.docs.tag)
	win.docs.texts = win.docs.texts[:0]
	pad := min(gtx.Dp(36), max(0, (r.Dx()-1)/4))
	width := max(1, min(r.Dx()-2*pad, gtx.Dp(720)))
	x := r.Min.X + (r.Dx()-width)/2
	y := r.Min.Y + gtx.Dp(24)
	label := func(s string, size unit.Sp) {
		call, sz := win.ui.faceText(gtx, s, size, font.Font{Typeface: chromeFace}, win.docColor(roleMuted))
		place(ops, call, image.Pt(x, y))
		win.docs.texts = append(win.docs.texts, s)
		y += sz.Y + gtx.Dp(8)
	}
	label(tildeHome(d.path, win.home), 11)
	label("Loaded "+d.loadedAt.Format("3:04 PM"), 11)
	if d.file.Cut {
		label(fmt.Sprintf("Showing the first 2 MB of %.1f MB.", float64(d.file.Total)/1e6), 11)
	}
	if d.removed {
		label("File removed. Showing the last loaded version.", 11)
	}
	y += gtx.Dp(8)
	l := win.layoutDoc(gtx, d, width)
	d.scroll = min(max(d.scroll, 0), max(0, l.height-(r.Max.Y-y)))
	view := image.Rect(x, y, x+width, r.Max.Y)
	cl := clip.Rect(view).Push(ops)
	defer cl.Pop()
	offset := image.Pt(x, y-d.scroll)
	// Boxes may overlap multiple lines, so retain any whose bottom is still visible.
	for _, b := range l.boxes {
		rr := b.r.Add(offset)
		if !rr.Overlaps(view) {
			continue
		}
		col := win.ui.th.Border
		switch b.kind {
		case boxCode:
			paint.FillShape(ops, mix(win.pal.BG, win.pal.FG, .07), clip.UniformRRect(rr, gtx.Dp(6)).Op(ops))
			continue
		case boxTableHead:
			col = tint(win.ui.th.Accent, 0x14)
		case boxTaskOpen, boxTaskDone:
			col = win.ui.th.Muted
			if b.kind == boxTaskDone {
				paint.FillShape(ops, win.ui.th.Accent, clip.Rect(rr).Op())
				win.ui.icon(gtx, "check", rr, win.pal.BG)
			} else {
				paint.FillShape(ops, col, clip.Stroke{Path: clip.Rect(rr).Path(), Width: 1}.Op())
			}
			continue
		}
		paint.FillShape(ops, col, clip.Rect(rr).Op())
	}
	start := sort.Search(len(l.lines), func(i int) bool { return l.lines[i].y+l.lines[i].h > d.scroll })
	for _, line := range l.lines[start:] {
		if line.y+offset.Y >= view.Max.Y {
			break
		}
		for _, run := range line.runs {
			at := offset.Add(image.Pt(run.x, line.y))
			if run.code {
				paint.FillShape(ops, mix(win.pal.BG, win.pal.FG, .07), clip.Rect(image.Rect(at.X, at.Y, at.X+run.w, at.Y+line.h)).Op())
			}
			call, _ := win.ui.faceText(gtx, run.text, run.size, run.face, win.docColor(run.role))
			place(ops, call, at)
			win.docs.texts = append(win.docs.texts, run.text)
			if run.under {
				paint.FillShape(ops, win.docColor(run.role), clip.Rect(image.Rect(at.X, at.Y+gtx.Sp(run.size)+2, at.X+run.w, at.Y+gtx.Sp(run.size)+3)).Op())
			}
			if run.strike {
				paint.FillShape(ops, win.docColor(run.role), clip.Rect(image.Rect(at.X, at.Y+line.h/2, at.X+run.w, at.Y+line.h/2+1)).Op())
			}
		}
	}
}
