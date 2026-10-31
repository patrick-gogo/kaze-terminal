package ui

import (
	"fmt"
	"image/color"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"

	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

// action is something kaze does itself instead of sending a key to the shell.
type action int

const (
	actNone action = iota
	actNewTab
	actCloseTab
	actNextTab
	actPrevTab
	actCopy
	actPaste
	actOpenSettings
	actToggleSidebar
	actNextWaiting
	actPalette
	actScratch
	actAgentsBoard
)

// actionByName maps config [keys] names to actions; it must match config.KnownActions.
var actionByName = map[string]action{
	"new_tab": actNewTab, "close_tab": actCloseTab, "next_tab": actNextTab,
	"prev_tab": actPrevTab, "copy": actCopy, "paste": actPaste, "open_settings": actOpenSettings,
	"toggle_sidebar": actToggleSidebar, "next_waiting": actNextWaiting, "palette": actPalette,
	"scratch_shell": actScratch, "agents_board": actAgentsBoard,
}

// maxGrid matches term's per-dimension clamp, so a huge window never asks for more.
const maxGrid = 4096

// gridSize is how many whole cells fit in a w x h pixel area, at least 1x1.
func gridSize(w, h, cellW, cellH int) (cols, rows int) {
	if cellW <= 0 || cellH <= 0 {
		return 1, 1
	}
	cols = min(max(w/cellW, 1), maxGrid)
	rows = min(max(h/cellH, 1), maxGrid)
	return cols, rows
}

// wheel adds up scroll deltas into whole notches.
type wheel struct{ acc float32 }

// notches takes Gio's scroll Y (> 0 means down) in units of one notch and
// returns whole notches with > 0 meaning up, the sign input.EncodeWheel wants.
func (w *wheel) notches(dy, unit float32) int {
	if unit <= 0 {
		return 0
	}
	w.acc += dy
	n := int(w.acc / unit)
	w.acc -= float32(n) * unit
	return -n
}

// defaultBindings are the platform's default shortcuts per action name.
func defaultBindings(goos string) map[string]input.Shortcut {
	cs := key.ModCtrl | key.ModShift
	if goos == "darwin" {
		return map[string]input.Shortcut{
			"new_tab": {Name: "T", Mods: key.ModCommand}, "close_tab": {Name: "W", Mods: key.ModCommand},
			"next_tab": {Name: key.NameTab, Mods: key.ModCtrl}, "prev_tab": {Name: key.NameTab, Mods: cs},
			"copy": {Name: "C", Mods: key.ModCommand}, "paste": {Name: "V", Mods: key.ModCommand},
			"open_settings":  {Name: ",", Mods: key.ModCommand},
			"toggle_sidebar": {Name: "B", Mods: key.ModCommand},
			"next_waiting":   {Name: "J", Mods: key.ModCommand},
			"palette":        {Name: "K", Mods: key.ModCommand},
			"scratch_shell":  {Name: "`", Mods: key.ModCtrl},
			"agents_board":   {Name: "A", Mods: key.ModCommand | key.ModShift},
		}
	}
	return map[string]input.Shortcut{
		"new_tab": {Name: "T", Mods: cs}, "close_tab": {Name: "W", Mods: cs},
		"next_tab": {Name: key.NameTab, Mods: key.ModCtrl}, "prev_tab": {Name: key.NameTab, Mods: cs},
		"copy": {Name: "C", Mods: cs}, "paste": {Name: "V", Mods: cs},
		"open_settings":  {Name: ",", Mods: key.ModCtrl},
		"toggle_sidebar": {Name: "B", Mods: cs},
		"next_waiting":   {Name: "J", Mods: cs},
		"palette":        {Name: "K", Mods: cs},
		"scratch_shell":  {Name: "`", Mods: cs},
		"agents_board":   {Name: "A", Mods: cs},
	}
}

var modNames = map[string]key.Modifiers{
	"ctrl": key.ModCtrl, "control": key.ModCtrl, "shift": key.ModShift,
	"alt": key.ModAlt, "option": key.ModAlt, "opt": key.ModAlt,
	"cmd": key.ModCommand, "command": key.ModCommand,
	"super": key.ModSuper, "win": key.ModSuper, "meta": key.ModSuper,
}

var keyNames = map[string]key.Name{
	"tab": key.NameTab, "space": key.NameSpace, "enter": key.NameReturn, "return": key.NameReturn,
	"esc": key.NameEscape, "escape": key.NameEscape, "backspace": key.NameDeleteBackward,
	"delete": key.NameDeleteForward, "del": key.NameDeleteForward,
	"pageup": key.NamePageUp, "pagedown": key.NamePageDown, "home": key.NameHome, "end": key.NameEnd,
	"up": key.NameUpArrow, "down": key.NameDownArrow, "left": key.NameLeftArrow, "right": key.NameRightArrow,
	"f1": key.NameF1, "f2": key.NameF2, "f3": key.NameF3, "f4": key.NameF4, "f5": key.NameF5, "f6": key.NameF6,
	"f7": key.NameF7, "f8": key.NameF8, "f9": key.NameF9, "f10": key.NameF10, "f11": key.NameF11, "f12": key.NameF12,
}

// parseShortcut reads config text like "ctrl+shift+t" or "cmd+t" into Gio's key name and modifiers.
func parseShortcut(s string) (input.Shortcut, bool) {
	parts := strings.Split(strings.ToLower(s), "+")
	var sc input.Shortcut
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if i < len(parts)-1 {
			m, ok := modNames[p]
			if !ok {
				return input.Shortcut{}, false
			}
			sc.Mods |= m
			continue
		}
		if n, ok := keyNames[p]; ok {
			sc.Name = n
		} else if utf8.RuneCountInString(p) == 1 {
			sc.Name = key.Name(strings.ToUpper(p)) // Gio names letter keys in upper case
		} else {
			return input.Shortcut{}, false
		}
	}
	return sc, true
}

type keymap map[input.Shortcut]action

// buildKeymap applies config [keys] over the defaults. A bad or clashing
// shortcut keeps the default and comes back as a warning.
func buildKeymap(goos string, overrides map[string]string) (keymap, []string) {
	b, warns := resolveBindings(goos, overrides)
	km := keymap{}
	for name, sc := range b {
		km[sc] = actionByName[name]
	}
	return km, warns
}

// resolveBindings is buildKeymap's name-to-shortcut step, kept so the palette
// and Settings can show each action's key, overrides included.
func resolveBindings(goos string, overrides map[string]string) (map[string]input.Shortcut, []string) {
	b := defaultBindings(goos)
	var warns []string
	names := make([]string, 0, len(overrides))
	for n := range overrides {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		text := overrides[name]
		if _, ok := b[name]; !ok {
			continue // config already warned about unknown actions
		}
		sc, ok := parseShortcut(text)
		if !ok {
			warns = append(warns, fmt.Sprintf("keys.%s: cannot read shortcut %q; using the default", name, text))
			continue
		}
		clash := ""
		for other, osc := range b {
			if other != name && osc == sc {
				clash = other
			}
		}
		if clash != "" {
			warns = append(warns, fmt.Sprintf("keys.%s: %q is already %s; using the default", name, text, clash))
			continue
		}
		b[name] = sc
	}
	return b, warns
}

func (k keymap) shortcuts() []input.Shortcut {
	out := make([]input.Shortcut, 0, len(k))
	for sc := range k {
		out = append(out, sc)
	}
	return out
}

// keyOut is what one key press does: a kaze action, bytes for the shell, or nothing.
type keyOut struct {
	act  action
	data []byte
	alt  bool // encoded with Alt, so its duplicate text event must be dropped
}

// Gio names these keys with single symbols, so rune count alone can't tell them from characters.
var symbolKeys = map[key.Name]bool{
	key.NameLeftArrow: true, key.NameRightArrow: true, key.NameUpArrow: true, key.NameDownArrow: true,
	key.NameReturn: true, key.NameEnter: true, key.NameEscape: true, key.NameHome: true, key.NameEnd: true,
	key.NameDeleteBackward: true, key.NameDeleteForward: true, key.NamePageUp: true, key.NamePageDown: true,
	key.NameCommand: true,
}

func printableName(n key.Name) bool {
	return n == key.NameSpace || (utf8.RuneCountInString(string(n)) == 1 && !symbolKeys[n])
}

// routeKey decides what a key press does. Printable keys without Ctrl, Alt or
// Cmd are left to the text event, which carries the layout's real character.
func routeKey(goos string, km keymap, name key.Name, mods key.Modifiers, m term.Modes) keyOut {
	if a, ok := km[input.Shortcut{Name: name, Mods: mods}]; ok {
		return keyOut{act: a}
	}
	if printableName(name) {
		if mods&^key.ModShift == 0 {
			return keyOut{}
		}
		// AltGr arrives as Ctrl+Alt on Windows; its character comes as text.
		if goos == "windows" && mods&key.ModCtrl != 0 && mods&key.ModAlt != 0 {
			return keyOut{}
		}
		// Ctrl+? is Ctrl+Shift+/ on a US layout.
		if name == "/" && mods&key.ModCtrl != 0 && mods&key.ModShift != 0 {
			name, mods = "?", mods&^key.ModShift
		}
	}
	data := input.EncodeKey(name, mods, m)
	return keyOut{data: data, alt: data != nil && mods&key.ModAlt != 0}
}

// textGate drops the text event that follows an Alt-encoded key press
// (macOS sends "å" after Option+A).
type textGate struct{ suppress bool }

func (g *textGate) onKey(press bool, o keyOut) { g.suppress = press && o.alt }
func (g *textGate) allow() bool                { return !g.suppress }

func compositionActive(c key.CompositionEvent) bool { return c.Start >= 0 && c.End != c.Start }

// committedText picks the text to send from one frame's edit events. IME preedit
// arrives as edits followed by an active composition range; those stay unsent
// until a commit, which is an edit followed by the composition ending.
func committedText(evs []event.Event, composing bool) (string, bool) {
	var b strings.Builder
	for i, e := range evs {
		switch e := e.(type) {
		case key.CompositionEvent:
			composing = compositionActive(e)
		case key.EditEvent:
			if editCommits(evs, i, composing) {
				b.WriteString(e.Text)
			}
		}
	}
	return b.String(), composing
}

// editCommits reports whether the edit at evs[i] is committed text: the next
// composition event in the frame (or, with none, the current state) is inactive.
func editCommits(evs []event.Event, i int, composing bool) bool {
	for _, next := range evs[i+1:] {
		if c, ok := next.(key.CompositionEvent); ok {
			return !compositionActive(c)
		}
	}
	return !composing
}

func parseHex(s string) (color.NRGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, true
}

// applyColors lays config [colors] over the palette; bad values keep the default.
func applyColors(p render.Palette, colors map[string]string) render.Palette {
	for k, v := range colors {
		c, ok := parseHex(v)
		if !ok {
			continue
		}
		switch k {
		case "fg":
			p.FG = c
		case "bg":
			p.BG = c
		case "cursor":
			p.Cursor = c
		case "selection":
			p.Selection = c
		default:
			n, err := strconv.Atoi(strings.TrimPrefix(k, "color"))
			if strings.HasPrefix(k, "color") && err == nil && n >= 0 && n < 16 {
				p.ANSI[n] = c
			}
		}
	}
	return p
}

const maxLabel = 28

// looksLikePath catches shells that set their title to their own exe path (pwsh does).
func looksLikePath(s string) bool {
	if strings.HasSuffix(strings.ToLower(s), ".exe") {
		return true
	}
	return len(s) > 2 && s[1] == ':' && (s[2] == '\\' || s[2] == '/')
}

// tabLabel is the text on a tab: the app's title (or def), shortened, plus the exit marker.
func tabLabel(t *tabs.Tab, def string) string {
	return tabLabelWith(t, def, func(s string) string { return s })
}

// tabLabelWith cleans the title with clean before shortening, so dropped runes never eat the budget.
func tabLabelWith(t *tabs.Tab, def string, clean func(string) string) string {
	title := strings.TrimSpace(clean(t.Title))
	if title == "" {
		title = def
	}
	if looksLikePath(title) {
		if i := strings.LastIndexAny(title, `\/`); i >= 0 {
			title = title[i+1:]
		}
	}
	if r := []rune(title); len(r) > maxLabel {
		title = string(r[:maxLabel-1]) + "…"
	}
	if t.Exited {
		title += " · exited"
	}
	return title
}

// maxWarnings keeps a broken config from filling the first screen.
const maxWarnings = 20

// injectWarnings prints config warnings as dim lines in a new tab. ConPTY
// clears the screen when the shell starts, so the lines are pushed into
// history and the view starts scrolled back to show them; typing snaps back.
func injectWarnings(s *term.Session, warns []string) {
	if len(warns) == 0 {
		return
	}
	if len(warns) > maxWarnings {
		warns = append(warns[:maxWarnings-1:maxWarnings-1], fmt.Sprintf("... and %d more", len(warns)-maxWarnings+1))
	}
	s.Lock()
	defer s.Unlock()
	a, scr := s.Applier(), s.Screen()
	before := scr.ScrollbackLen()
	for _, w := range warns {
		a.CSI([]int{2}, nil, 0, 'm')
		for _, r := range "kaze: " + w {
			a.Print(r)
		}
		a.CSI([]int{0}, nil, 0, 'm')
		a.Execute(0x0d) // CR
		a.Execute(0x0a) // LF
	}
	_, row := scr.Cursor()
	scr.ScrollUp(row)
	scr.SetCursor(0, 0)
	scr.SetViewOffset(scr.ScrollbackLen() - before)
}
