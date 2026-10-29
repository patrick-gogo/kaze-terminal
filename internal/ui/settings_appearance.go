package ui

import (
	"math"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

// The page's font size range; the file still accepts 4 to 72.
const pageFontMin, pageFontMax = 8, 32

var (
	spacings     = []string{"tight", "normal", "roomy"}
	cursorStyles = []string{"block", "bar", "underline"}
)

func appearanceSection() settingsSection {
	return settingsSection{
		title: "Appearance", hint: "Changes show at once in every tab.",
		rows: appearanceRows, act: appearanceAct,
	}
}

func appearanceRows(win *window) []settingRow {
	c := win.opt.Config
	cur := startTheme(c).Key
	var swatches []ctrl
	for i, t := range theme.All() {
		swatches = append(swatches, ctrl{kind: ctlSwatch, id: "theme", theme: t, current: t.Key == cur, arg: i})
	}
	return []settingRow{
		{label: "Theme", block: true, ctrls: swatches},
		{label: "Font size", hint: "JetBrains Mono, bundled", ctrls: []ctrl{{
			kind: ctlStepper, id: "font", min: pageFontMin, max: pageFontMax,
			value: int(math.Round(float64(c.FontSize))),
		}}},
		{label: "Line spacing", ctrls: []ctrl{{
			kind: ctlSegmented, id: "spacing", options: []string{"Tight", "Normal", "Roomy"},
			selected: choiceIndex(spacings, c.LineSpacing, 1),
		}}},
		{label: "Cursor", ctrls: []ctrl{{
			kind: ctlSegmented, id: "cursor", options: []string{"Block", "Bar", "Underline"},
			selected: choiceIndex(cursorStyles, c.CursorStyle, 0),
		}}},
		{label: "Blinking cursor", hint: "Off keeps idle CPU at zero", ctrls: []ctrl{{kind: ctlToggle, id: "blink", on: c.CursorBlink}}},
	}
}

// choiceIndex is v's place in choices, or def for an unset value.
func choiceIndex(choices []string, v string, def int) int {
	for i, c := range choices {
		if c == v {
			return i
		}
	}
	return def
}

func appearanceAct(win *window, id string, arg int) {
	switch id {
	case "theme":
		if all := theme.All(); arg >= 0 && arg < len(all) {
			win.setTheme(all[arg].Key)
		}
	case "font":
		win.setFontSize(arg)
	case "spacing":
		if arg >= 0 && arg < len(spacings) {
			win.setLineSpacing(spacings[arg])
		}
	case "cursor":
		if arg >= 0 && arg < len(cursorStyles) {
			win.setCursorStyle(cursorStyles[arg])
		}
	case "blink":
		win.setBlink(arg == 1)
	}
}

func (win *window) setTheme(key string) {
	t, ok := theme.ByKey(key)
	if !ok {
		return
	}
	win.opt.Config.Theme = key
	win.applyTheme(t)
	win.saveSetting("theme", key)
}

// setFontSize clamps n into the page's range, so a size from the file outside it steps back in.
func (win *window) setFontSize(n int) {
	n = min(max(n, pageFontMin), pageFontMax)
	if win.opt.Config.FontSize == float32(n) {
		return
	}
	win.opt.Config.FontSize = float32(n)
	win.rend = nil // the next frame rebuilds it and resizes every tab to the new grid
	win.saveSetting("font_size", n)
}

func (win *window) setLineSpacing(v string) {
	win.opt.Config.LineSpacing = v
	win.rend = nil // same font size, so frame would keep the old renderer
	win.saveSetting("line_spacing", v)
}

func (win *window) setCursorStyle(v string) {
	win.opt.Config.CursorStyle = v
	win.rend = nil
	win.saveSetting("cursor_style", v)
}

func (win *window) setBlink(on bool) {
	win.opt.Config.CursorBlink = on // frame reads it through blinks
	win.saveSetting("cursor_blink", on)
}

// saveSetting writes one key; a failure shows in the status bar and the live change stays.
func (win *window) saveSetting(key string, value any) {
	if win.cfgPath != "" {
		if err := config.Set(win.cfgPath, key, value); err != nil {
			win.notice = "settings not saved: " + err.Error()
			win.noticeUntil = time.Now().Add(noticeFor)
		}
	}
	win.w.Invalidate()
}
