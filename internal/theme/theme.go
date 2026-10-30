// Package theme holds the built-in color themes: the terminal palette plus the window chrome colors.
package theme

import (
	"image/color"

	"github.com/patrick-gogo/kaze-terminal/internal/render"
)

// Chrome is every color the window chrome (title bar, sidebar, status bar, toasts) draws with.
type Chrome struct {
	Chrome, Active, ActiveFG, Muted, Border, Accent, StatusFG, Folder, Branch color.NRGBA
	CapGlyph, CapHover, CapPress, CloseHover, ClosePress                      color.NRGBA
	Needs, Done, ToastBG, ToastLine, ToastFG, ToastGo, ToastTitle, UsageHigh  color.NRGBA
}

// Theme is one selectable look.
type Theme struct {
	Key, Name string
	Term      render.Palette
	UI        Chrome
}

func hex(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

func ansi(v ...uint32) (a [16]color.NRGBA) {
	for i, c := range v {
		a[i] = hex(c)
	}
	return a
}

// All returns the built-in themes in picker order.
func All() []Theme {
	return []Theme{midnight(), sumi(), tide(), washi(), sakura()}
}

// ByKey finds a theme by its config key.
func ByKey(k string) (Theme, bool) {
	for _, t := range All() {
		if t.Key == k {
			return t, true
		}
	}
	return Theme{}, false
}

// Default is Midnight, the original look.
func Default() Theme { return midnight() }

// Close-button reds are shared: a red hover reads the same on every theme.
var (
	closeHover = hex(0xe5484d)
	closePress = hex(0xc93c40)
)

func midnight() Theme {
	return Theme{
		Key: "midnight", Name: "Midnight", Term: render.DefaultPalette(),
		UI: Chrome{
			Chrome: hex(0x0b0d12), Active: hex(0x0f1117), ActiveFG: hex(0xffffff), Muted: hex(0x8a90a2),
			Border: hex(0x1d2230), Accent: hex(0x8b7bff), StatusFG: hex(0x7d8394), Folder: hex(0xc9cdd8),
			Branch: hex(0x62d39a), CapGlyph: hex(0xa7adbd), CapHover: hex(0x1a1f2c), CapPress: hex(0x151925),
			CloseHover: closeHover, ClosePress: closePress, Needs: hex(0xf2b84b), Done: hex(0x62d39a),
			ToastBG: hex(0x1a1f2c), ToastLine: hex(0x2a3142), ToastFG: hex(0xe7e9f0), ToastGo: hex(0xb9a6ff),
			ToastTitle: hex(0xffffff), UsageHigh: hex(0xff6b6b),
		},
	}
}

func sumi() Theme {
	return Theme{
		Key: "sumi", Name: "Sumi",
		Term: render.Palette{
			FG: hex(0xe4e1da), BG: hex(0x111111), Cursor: hex(0xe2553d), Selection: hex(0x3a2420),
			ANSI: ansi(0x1c1c1c, 0xe2553d, 0x9fbf8f, 0xd9b56a, 0x7f9fbf, 0xc08fa8, 0x8fbfb5, 0xe4e1da,
				0x55524c, 0xff7a5c, 0xb8d6a8, 0xf0cf86, 0xa3bfdc, 0xd9acc2, 0xa9d9cf, 0xf3efe6),
		},
		UI: Chrome{
			Chrome: hex(0x0c0c0c), Active: hex(0x111111), ActiveFG: hex(0xffffff), Muted: hex(0x77736b),
			Border: hex(0x222222), Accent: hex(0xe2553d), StatusFG: hex(0x77736b), Folder: hex(0xe4e1da),
			Branch: hex(0x9fbf8f), CapGlyph: hex(0x77736b), CapHover: hex(0x2e2e2e), CapPress: hex(0x191919),
			CloseHover: closeHover, ClosePress: closePress, Needs: hex(0xd9b56a), Done: hex(0x9fbf8f),
			ToastBG: hex(0x181818), ToastLine: hex(0x222222), ToastFG: hex(0xe4e1da), ToastGo: hex(0xe2553d),
			ToastTitle: hex(0xffffff), UsageHigh: hex(0xff7a5c),
		},
	}
}

func tide() Theme {
	return Theme{
		Key: "tide", Name: "Tide",
		Term: render.Palette{
			FG: hex(0xcfe3ef), BG: hex(0x0b1626), Cursor: hex(0x40e0d0), Selection: hex(0x123a48),
			ANSI: ansi(0x132235, 0xff7b8a, 0x5fe0a8, 0xffd27a, 0x7cc8ff, 0xc3a6ff, 0x5fe0d0, 0xcfe3ef,
				0x3f5b6b, 0xff9daa, 0x8ff0c2, 0xffe29f, 0xa6dcff, 0xd8c4ff, 0x8ff0e4, 0xe9fbff),
		},
		UI: Chrome{
			Chrome: hex(0x08111e), Active: hex(0x0b1626), ActiveFG: hex(0xffffff), Muted: hex(0x7fa3b8),
			Border: hex(0x16283d), Accent: hex(0x40e0d0), StatusFG: hex(0x6e8ea2), Folder: hex(0xcfe3ef),
			Branch: hex(0x5fe0a8), CapGlyph: hex(0x7fa3b8), CapHover: hex(0x1e3550), CapPress: hex(0x101e2f),
			CloseHover: closeHover, ClosePress: closePress, Needs: hex(0xffd27a), Done: hex(0x5fe0a8),
			ToastBG: hex(0x0c1e30), ToastLine: hex(0x16283d), ToastFG: hex(0xcfe3ef), ToastGo: hex(0x40e0d0),
			ToastTitle: hex(0xffffff), UsageHigh: hex(0xff9daa),
		},
	}
}

// Washi is the light theme, so its caption hover/press go darker, not lighter.
func washi() Theme {
	return Theme{
		Key: "washi", Name: "Washi",
		Term: render.Palette{
			FG: hex(0x2b2a33), BG: hex(0xf6f2ea), Cursor: hex(0x3f51b5), Selection: hex(0xd6dcf5),
			ANSI: ansi(0x2b2a33, 0xc0392b, 0x2e7d4f, 0x9a6a00, 0x2f5fb3, 0x8e44ad, 0x1f7f8a, 0xbdb6a8,
				0x6b665c, 0xd9534a, 0x3c9a63, 0xb8860b, 0x4a7bd0, 0xa35fc2, 0x2a9aa6, 0x1a1a1f),
		},
		UI: Chrome{
			Chrome: hex(0xece6da), Active: hex(0xf6f2ea), ActiveFG: hex(0x2b2a33), Muted: hex(0x7a7468),
			Border: hex(0xddd5c6), Accent: hex(0x3f51b5), StatusFG: hex(0x8a8478), Folder: hex(0x2b2a33),
			Branch: hex(0x2e7d4f), CapGlyph: hex(0x7a7468), CapHover: hex(0xcfc7b7), CapPress: hex(0xc4bcab),
			CloseHover: closeHover, ClosePress: closePress, Needs: hex(0x9a6a00), Done: hex(0x2e7d4f),
			ToastBG: hex(0xfffdf8), ToastLine: hex(0xddd5c6), ToastFG: hex(0x2b2a33), ToastGo: hex(0x3f51b5),
			ToastTitle: hex(0x2b2a33), UsageHigh: hex(0xc0392b),
		},
	}
}

func sakura() Theme {
	return Theme{
		Key: "sakura", Name: "Sakura",
		Term: render.Palette{
			FG: hex(0xeedde6), BG: hex(0x1a1218), Cursor: hex(0xff8fb8), Selection: hex(0x4a2a3c),
			ANSI: ansi(0x2b1f28, 0xff6f8e, 0x9fd8a8, 0xf3c98b, 0x9fb6ff, 0xff8fb8, 0x8fd8d0, 0xeedde6,
				0x6a5562, 0xff93ab, 0xbdeac3, 0xffdcaa, 0xbccbff, 0xffb3cf, 0xb0e8e1, 0xffffff),
		},
		UI: Chrome{
			Chrome: hex(0x140e13), Active: hex(0x1a1218), ActiveFG: hex(0xffffff), Muted: hex(0xa8909e),
			Border: hex(0x2b1f28), Accent: hex(0xff8fb8), StatusFG: hex(0x9c8492), Folder: hex(0xeedde6),
			Branch: hex(0x9fd8a8), CapGlyph: hex(0xa8909e), CapHover: hex(0x372a34), CapPress: hex(0x211720),
			CloseHover: closeHover, ClosePress: closePress, Needs: hex(0xf3c98b), Done: hex(0x9fd8a8),
			ToastBG: hex(0x24181f), ToastLine: hex(0x2b1f28), ToastFG: hex(0xeedde6), ToastGo: hex(0xff8fb8),
			ToastTitle: hex(0xffffff), UsageHigh: hex(0xff93ab),
		},
	}
}
