package ui

import (
	"strings"

	"gioui.org/io/key"

	"github.com/patrick-gogo/kaze-terminal/internal/input"
)

// kazeCommand is one of kaze's own actions, listed in the palette's kaze
// section and in Settings > Commands from this one table.
type kazeCommand struct {
	Config string // config name under [keys]
	Label  string // shown in the palette and Settings
	Act    action
}

var kazeCommands = []kazeCommand{
	{"new_tab", "New session", actNewTab},
	{"close_tab", "Close session", actCloseTab},
	{"next_tab", "Next session", actNextTab},
	{"prev_tab", "Previous session", actPrevTab},
	{"copy", "Copy", actCopy},
	{"paste", "Paste", actPaste},
	{"open_settings", "Open settings", actOpenSettings},
	{"toggle_sidebar", "Toggle sidebar", actToggleSidebar},
	{"next_waiting", "Go to the next session that needs you", actNextWaiting},
	{"palette", "Open the palette", actPalette},
}

var keyLabels = map[key.Name]string{
	key.NameTab: "Tab", key.NameReturn: "Enter", key.NameEnter: "Enter", key.NameEscape: "Esc",
	key.NameUpArrow: "Up", key.NameDownArrow: "Down", key.NameLeftArrow: "Left", key.NameRightArrow: "Right",
	key.NamePageUp: "PageUp", key.NamePageDown: "PageDown", key.NameHome: "Home", key.NameEnd: "End",
	key.NameDeleteBackward: "Backspace", key.NameDeleteForward: "Delete", key.NameSpace: "Space",
}

// formatShortcut writes a shortcut the way the platform names its keys:
// "Ctrl+Shift+K" on Windows, "Cmd+K" on macOS.
func formatShortcut(goos string, sc input.Shortcut) string {
	type mod struct {
		m    key.Modifiers
		name string
	}
	mods := []mod{{key.ModCtrl, "Ctrl"}, {key.ModAlt, "Alt"}, {key.ModShift, "Shift"}, {key.ModSuper, "Win"}, {key.ModCommand, "Cmd"}}
	if goos == "darwin" {
		mods = []mod{{key.ModCtrl, "Ctrl"}, {key.ModAlt, "Option"}, {key.ModShift, "Shift"}, {key.ModSuper, "Super"}, {key.ModCommand, "Cmd"}}
	}
	var parts []string
	for _, m := range mods {
		if sc.Mods.Contain(m.m) {
			parts = append(parts, m.name)
		}
	}
	name := string(sc.Name)
	if l, ok := keyLabels[sc.Name]; ok {
		name = l
	}
	return strings.Join(append(parts, name), "+")
}

// togglePalette opens or closes the palette; filled in by the palette window code.
func (win *window) togglePalette() {}
