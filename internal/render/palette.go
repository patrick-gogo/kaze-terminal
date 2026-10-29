package render

import (
	"image/color"
	"math"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

// Palette is the theme: default colors plus the 16 ANSI colors.
type Palette struct {
	FG, BG, Cursor, Selection color.NRGBA
	ANSI                      [16]color.NRGBA
}

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

// DefaultPalette is the "Midnight" look.
func DefaultPalette() Palette {
	return Palette{
		FG:        hex(0xd7dae3),
		BG:        hex(0x0f1117),
		Cursor:    hex(0xb9a6ff),
		Selection: hex(0x2e2a52),
		ANSI: [16]color.NRGBA{
			hex(0x1d2230), hex(0xf07178), hex(0x62d39a), hex(0xf2b84b),
			hex(0x8ab4ff), hex(0xb9a6ff), hex(0x6fd3e0), hex(0xd7dae3),
			hex(0x5d6273), hex(0xff8b92), hex(0x8be3b4), hex(0xffd27a),
			hex(0xa9c7ff), hex(0xcdbfff), hex(0x95e6f0), hex(0xffffff),
		},
	}
}

// indexed maps 0-255: the theme's 16, then the xterm 6x6x6 cube and grayscale ramp.
func indexed(p Palette, n uint8) color.NRGBA {
	switch {
	case n < 16:
		return p.ANSI[n]
	case n < 232:
		n -= 16
		level := func(v uint8) uint8 {
			if v == 0 {
				return 0
			}
			return 55 + v*40
		}
		return color.NRGBA{R: level(n / 36), G: level(n / 6 % 6), B: level(n % 6), A: 0xff}
	default:
		g := 8 + (n-232)*10
		return color.NRGBA{R: g, G: g, B: g, A: 0xff}
	}
}

func resolve(p Palette, c screen.Color, def color.NRGBA) color.NRGBA {
	switch {
	case c.IsDefault():
		return def
	case c.IsRGB():
		r, g, b := c.RGB()
		return color.NRGBA{R: r, G: g, B: b, A: 0xff}
	default:
		return indexed(p, c.Index())
	}
}

// resolveFG applies bold-is-bright to the 8 basic colors, as xterm does.
func resolveFG(p Palette, c screen.Cell) color.NRGBA {
	fg := c.FG
	if c.Attrs&screen.AttrBold != 0 && !fg.IsDefault() && !fg.IsRGB() && fg.Index() < 8 {
		fg = screen.Indexed(fg.Index() + 8)
	}
	return resolve(p, fg, p.FG)
}

// cellColors returns the colors a cell is drawn with after inverse and dim.
func cellColors(p Palette, c screen.Cell) (fg, bg color.NRGBA) {
	fg, bg = resolveFG(p, c), resolve(p, c.BG, p.BG)
	if c.Attrs&screen.AttrInverse != 0 {
		fg, bg = bg, fg
	}
	if c.Attrs&screen.AttrDim != 0 {
		fg = color.NRGBA{R: mid(fg.R, bg.R), G: mid(fg.G, bg.G), B: mid(fg.B, bg.B), A: 0xff}
	}
	return fg, bg
}

func mid(a, b uint8) uint8 { return uint8((int(a) + int(b)) / 2) }

// minContrast is the WCAG AA ratio for normal text.
const minContrast = 4.5

// fixCap bounds the per-renderer cache of fixed (fg, bg) pairs; a full cache is cleared.
const fixCap = 512

// linear maps an sRGB channel to linear light, so luminance needs no pow per cell.
var linear = func() (t [256]float64) {
	for i := range t {
		c := float64(i) / 255
		if c <= 0.04045 {
			t[i] = c / 12.92
		} else {
			t[i] = math.Pow((c+0.055)/1.055, 2.4)
		}
	}
	return t
}()

func luminance(c color.NRGBA) float64 {
	return 0.2126*linear[c.R] + 0.7152*linear[c.G] + 0.0722*linear[c.B]
}

// inkOf is the darker of a palette's FG and BG, so a light theme pulls dim text to its own ink.
func inkOf(p Palette) color.NRGBA {
	if luminance(p.FG) < luminance(p.BG) {
		return p.FG
	}
	return p.BG
}

// contrast is the WCAG 2 contrast ratio of two opaque colors, 1 to 21.
func contrast(a, b color.NRGBA) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// blend moves a toward b by t/256.
func blend(a, b color.NRGBA, t int) color.NRGBA {
	m := func(x, y uint8) uint8 { return uint8(int(x) + (int(y)-int(x))*t/256) }
	return color.NRGBA{R: m(a.R, b.R), G: m(a.G, b.G), B: m(a.B, b.B), A: 0xff}
}

// readable moves fg toward the theme's ink or white, whichever contrasts more
// with bg, only as far as needed to reach minContrast, so dim and colored text keep
// their tint. fg == bg is left alone: that is hidden text on purpose.
func readable(fg, bg, ink color.NRGBA) color.NRGBA {
	if fg == bg || contrast(fg, bg) >= minContrast {
		return fg
	}
	to := hex(0xffffff)
	if contrast(ink, bg) > contrast(to, bg) {
		to = ink
	}
	lo, hi := 0, 256 // blend(fg, to, 256) is to itself, the best available
	for lo+1 < hi {
		mid := (lo + hi) / 2
		if contrast(blend(fg, to, mid), bg) >= minContrast {
			hi = mid
		} else {
			lo = mid
		}
	}
	return blend(fg, to, hi)
}

// readableCached is readable memoized per (fg, bg), so the float math runs once per pair.
func (r *Renderer) readableCached(fg, bg color.NRGBA) color.NRGBA {
	if fg == r.pal.FG && bg == r.pal.BG {
		return r.defFG
	}
	k := uint64(fg.R)<<40 | uint64(fg.G)<<32 | uint64(fg.B)<<24 | uint64(bg.R)<<16 | uint64(bg.G)<<8 | uint64(bg.B)
	if v, ok := r.fixed[k]; ok {
		return v
	}
	if r.fixed == nil || len(r.fixed) >= fixCap {
		r.fixed = make(map[uint64]color.NRGBA, 64)
	}
	v := readable(fg, bg, r.ink)
	r.fixed[k] = v
	return v
}

// colorsOf is cellColors with the text kept readable.
func (r *Renderer) colorsOf(c screen.Cell) (fg, bg color.NRGBA) {
	fg, bg = cellColors(r.pal, c)
	return r.readableCached(fg, bg), bg
}

// selectedFG is a selected cell's text color, kept readable over the selection background.
func (r *Renderer) selectedFG(c screen.Cell) color.NRGBA {
	fg, _ := cellColors(r.pal, c)
	return r.readableCached(fg, r.pal.Selection)
}
