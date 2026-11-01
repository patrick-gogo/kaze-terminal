package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"gioui.org/op/paint"
	xdraw "golang.org/x/image/draw"

	"github.com/patrick-gogo/kaze-terminal/assets/branding"
)

type logoKey struct {
	name string
	px   int
	col  color.NRGBA
}

// agentLogos holds each agent's logo scaled and tinted per size and colour, so
// a frame never decodes or re-uploads one.
var agentLogos struct {
	sync.Mutex
	src map[string]image.Image
	by  map[logoKey]markEntry
}

// logoSource is the embedded white logo for an agent; nil for an unknown one.
func logoSource(name string) []byte {
	switch name {
	case "claude":
		return branding.ClaudeLogo
	case "codex":
		return branding.OpenAILogo
	}
	return nil
}

// agentLogo is the agent's logo, px by px, in col; nil for an unknown agent or
// an unusable embedded PNG.
func agentLogo(name string, px int, col color.NRGBA) (paint.ImageOp, *image.NRGBA) {
	data := logoSource(name)
	if data == nil || px <= 0 {
		return paint.ImageOp{}, nil
	}
	agentLogos.Lock()
	defer agentLogos.Unlock()
	key := logoKey{name, px, col}
	if e, ok := agentLogos.by[key]; ok {
		return e.op, e.img
	}
	src := agentLogos.src[name]
	if src == nil {
		var err error
		if src, err = png.Decode(bytes.NewReader(data)); err != nil {
			return paint.ImageOp{}, nil
		}
		if agentLogos.src == nil {
			agentLogos.src = map[string]image.Image{}
		}
		agentLogos.src[name] = src
	}
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	xdraw.CatmullRom.Scale(img, img.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	for i := 0; i < len(img.Pix); i += 4 { // keep the shape, take the colour
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = col.R, col.G, col.B
		img.Pix[i+3] = uint8(uint16(img.Pix[i+3]) * uint16(col.A) / 0xff)
	}
	e := markEntry{op: paint.NewImageOp(img), img: img}
	if agentLogos.by == nil {
		agentLogos.by = map[logoKey]markEntry{}
	}
	agentLogos.by[key] = e
	return e.op, e.img
}

// claudeCoral is the Claude logo's own colour.
var claudeCoral = color.NRGBA{R: 0xd9, G: 0x77, B: 0x57, A: 0xff}

// agentLogoColor is the colour an agent's logo is drawn in: Claude's coral, and
// the row's text colour for the black-and-white OpenAI logo.
func agentLogoColor(name string, fg color.NRGBA) color.NRGBA {
	if name == "claude" {
		return claudeCoral
	}
	return fg
}
