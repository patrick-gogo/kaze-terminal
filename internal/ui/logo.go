package ui

import (
	"bytes"
	"image"
	"image/png"
	"sync"

	"gioui.org/op/paint"
	xdraw "golang.org/x/image/draw"

	"github.com/patrick-gogo/kaze-terminal/assets/branding"
)

type markEntry struct {
	op  paint.ImageOp
	img *image.NRGBA
}

// marks holds the logo scaled per pixel size, so a frame never decodes or
// re-uploads it.
var marks struct {
	sync.Mutex
	src  image.Image
	bySz map[int]markEntry
}

// markImage is the kaze mark scaled to px by px; nil when the embedded PNG is unusable.
func markImage(px int) (paint.ImageOp, *image.NRGBA) {
	marks.Lock()
	defer marks.Unlock()
	if e, ok := marks.bySz[px]; ok {
		return e.op, e.img
	}
	if marks.src == nil {
		src, err := png.Decode(bytes.NewReader(branding.Mark))
		if err != nil {
			return paint.ImageOp{}, nil
		}
		marks.src = src
	}
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	xdraw.CatmullRom.Scale(img, img.Bounds(), marks.src, marks.src.Bounds(), xdraw.Over, nil)
	e := markEntry{op: paint.NewImageOp(img), img: img}
	if marks.bySz == nil {
		marks.bySz = map[int]markEntry{}
	}
	marks.bySz[px] = e
	return e.op, e.img
}
