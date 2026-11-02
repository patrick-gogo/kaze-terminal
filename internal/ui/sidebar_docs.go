package ui

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"image"
)

const sideDocH = 34

type sideDoc struct {
	ID           docID
	Name, Folder string
}
type sideDocHit struct {
	id          docID
	rect, close image.Rectangle
}

func (win *window) sideDocs(id tabs.TabID) []sideDoc {
	var out []sideDoc
	for _, d := range win.docs.list {
		if d.session == id {
			out = append(out, sideDoc{d.id, d.name, d.folder})
		}
	}
	return out
}
func (c *chrome) docLines(gtx layout.Context, h *sideHits, r sideRow, st sideState, x0, x1, y int, closeCall op.CallOp, closeSize image.Point) {
	for _, d := range r.Docs {
		band := image.Rect(x0, y, x1, y+gtx.Dp(sideDocH))
		y = band.Max.Y
		if !band.Overlaps(h.list) {
			continue
		}
		fg, ic := c.th.Folder, c.th.Muted
		if st.doc == d.ID {
			paint.FillShape(gtx.Ops, c.th.Active, clip.UniformRRect(band, gtx.Dp(5)).Op(gtx.Ops))
			fg, ic = c.th.ActiveFG, c.th.Accent
		}
		c.icon(gtx, "doc", image.Rect(x0+gtx.Dp(2), band.Min.Y+gtx.Dp(5), x0+gtx.Dp(14), band.Min.Y+gtx.Dp(17)), ic)
		tx := x0 + gtx.Dp(22)
		cl := image.Rect(x1-closeSize.X-gtx.Dp(2), band.Min.Y+gtx.Dp(7), x1-gtx.Dp(2), band.Min.Y+gtx.Dp(7)+closeSize.Y)
		call, sz := c.fitSide(gtx, d.Name, 12, max(0, cl.Min.X-tx-gtx.Dp(4)), fg)
		place(gtx.Ops, call, image.Pt(tx, band.Min.Y+gtx.Dp(2)))
		_ = sz
		call, _ = c.fitSide(gtx, d.Folder, 10, max(0, x1-tx), c.th.Muted)
		place(gtx.Ops, call, image.Pt(tx, band.Min.Y+gtx.Dp(19)))
		if st.hoverDoc == d.ID {
			place(gtx.Ops, closeCall, cl.Min)
		}
		h.docs = append(h.docs, sideDocHit{d.ID, band.Intersect(h.list), cl.Intersect(h.list)})
	}
}
