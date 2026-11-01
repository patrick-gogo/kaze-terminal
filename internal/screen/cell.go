package screen

// Color packs a palette index (0-255), a truecolor value or the default color.
type Color uint32

const (
	// DefaultColor means "use the theme's foreground/background".
	DefaultColor Color = 1 << 31
	rgbFlag      Color = 1 << 30
)

// Indexed returns palette color n.
func Indexed(n uint8) Color { return Color(n) }

// RGB returns a truecolor value.
func RGB(r, g, b uint8) Color {
	return rgbFlag | Color(r)<<16 | Color(g)<<8 | Color(b)
}

// Attr is a bit set of text attributes.
type Attr uint8

const (
	AttrBold Attr = 1 << iota
	AttrItalic
	AttrUnderline
	AttrInverse
	AttrDim
	AttrStrike
	// AttrWrapped is set only on a row's last cell, when printing ran on into
	// the next row. It is not drawn; it tells a wrap from a full row and a newline.
	AttrWrapped
)

// Cell is one grid position. Width is 1, 2 (lead half of a wide char) or 0 (continuation half).
type Cell struct {
	Rune   rune
	FG, BG Color
	Width  uint8 // after the 4-byte fields, so a Cell packs into 16 bytes, not 20
	Attrs  Attr
}

// blankCell is what an erased position looks like.
var blankCell = Cell{Rune: ' ', Width: 1, FG: DefaultColor, BG: DefaultColor}

// IsDefault reports whether c means "use the theme's color".
func (c Color) IsDefault() bool { return c&DefaultColor != 0 }

// IsRGB reports whether c is a truecolor value.
func (c Color) IsRGB() bool { return c&DefaultColor == 0 && c&rgbFlag != 0 }

// RGB returns a truecolor value's components; only meaningful when IsRGB.
func (c Color) RGB() (r, g, b uint8) { return uint8(c >> 16), uint8(c >> 8), uint8(c) }

// Index returns a palette color's index; only meaningful when neither IsDefault nor IsRGB.
func (c Color) Index() uint8 { return uint8(c) }
