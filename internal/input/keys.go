// Package input turns key presses, pastes and mouse events into the bytes a
// shell expects (xterm encodings).
package input

import (
	"runtime"
	"strconv"
	"strings"

	"gioui.org/io/key"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

// Shortcut is a key with an exact set of modifiers.
type Shortcut struct {
	Name key.Name
	Mods key.Modifiers
}

// reservedKeys are the keys kaze keeps for itself (copy, paste, new/close/next
// tab). Plain Ctrl+C is never here. The window replaces these with the user's
// bindings at startup through SetReserved.
var reservedKeys = map[string][]Shortcut{
	"windows": {
		{"C", key.ModCtrl | key.ModShift}, {"V", key.ModCtrl | key.ModShift},
		{"T", key.ModCtrl | key.ModShift}, {"W", key.ModCtrl | key.ModShift},
		{key.NameTab, key.ModCtrl}, {key.NameTab, key.ModCtrl | key.ModShift},
	},
	"darwin": {
		{"C", key.ModCommand}, {"V", key.ModCommand},
		{"T", key.ModCommand}, {"W", key.ModCommand},
	},
}

// SetReserved replaces goos's reserved keys. Call it once at startup, before
// any key is encoded; the table is not locked.
func SetReserved(goos string, keys []Shortcut) {
	reservedKeys[goos] = append([]Shortcut(nil), keys...)
}

// IsReserved reports whether kaze handles the key itself on the given GOOS.
func IsReserved(name key.Name, mods key.Modifiers, goos string) bool {
	for _, s := range reservedKeys[goos] {
		if s.Name == name && s.Mods == mods {
			return true
		}
	}
	return false
}

var finalKeys = map[key.Name]byte{
	key.NameUpArrow: 'A', key.NameDownArrow: 'B', key.NameRightArrow: 'C', key.NameLeftArrow: 'D',
	key.NameHome: 'H', key.NameEnd: 'F',
}

var tildeKeys = map[key.Name]int{
	key.NameDeleteForward: 3, key.NamePageUp: 5, key.NamePageDown: 6,
	key.NameF5: 15, key.NameF6: 17, key.NameF7: 18, key.NameF8: 19,
	key.NameF9: 20, key.NameF10: 21, key.NameF11: 23, key.NameF12: 24,
}

var ss3Keys = map[key.Name]byte{
	key.NameF1: 'P', key.NameF2: 'Q', key.NameF3: 'R', key.NameF4: 'S',
}

// xtermMod is the CSI modifier parameter: 1 + shift(1) + alt(2) + ctrl(4).
func xtermMod(mods key.Modifiers) int {
	n := 1
	if mods&key.ModShift != 0 {
		n += 1
	}
	if mods&key.ModAlt != 0 {
		n += 2
	}
	if mods&key.ModCtrl != 0 {
		n += 4
	}
	return n
}

// EncodeKey returns the bytes for a key press, or nil when kaze handles the
// key itself or it has no encoding.
func EncodeKey(name key.Name, mods key.Modifiers, m term.Modes) []byte {
	return encodeKey(runtime.GOOS, name, mods, m)
}

// modifierKeys are pressed on their own, never typed: Gio names Command "⌘",
// one rune, which would otherwise pass as a character.
var modifierKeys = map[key.Name]bool{
	key.NameCommand: true, key.NameCtrl: true, key.NameShift: true, key.NameAlt: true, key.NameSuper: true,
}

func encodeKey(goos string, name key.Name, mods key.Modifiers, m term.Modes) []byte {
	if IsReserved(name, mods, goos) || modifierKeys[name] {
		return nil
	}
	mod := xtermMod(mods)

	if f, ok := finalKeys[name]; ok {
		if mod > 1 {
			return []byte("\x1b[1;" + strconv.Itoa(mod) + string(f))
		}
		if m.AppCursorKeys {
			return []byte{0x1b, 'O', f}
		}
		return []byte{0x1b, '[', f}
	}
	if f, ok := ss3Keys[name]; ok {
		if mod > 1 {
			return []byte("\x1b[1;" + strconv.Itoa(mod) + string(f))
		}
		return []byte{0x1b, 'O', f}
	}
	if n, ok := tildeKeys[name]; ok {
		if mod > 1 {
			return []byte("\x1b[" + strconv.Itoa(n) + ";" + strconv.Itoa(mod) + "~")
		}
		return []byte("\x1b[" + strconv.Itoa(n) + "~")
	}

	var b []byte
	switch name {
	case key.NameReturn, key.NameEnter:
		b = []byte{'\r'}
	case key.NameTab:
		if mods&key.ModShift != 0 {
			if mods&key.ModAlt != 0 {
				return []byte("\x1b\x1b[Z")
			}
			return []byte("\x1b[Z")
		}
		b = []byte{'\t'}
	case key.NameDeleteBackward:
		// Ctrl+Backspace is 0x08 so PSReadLine and Windows Terminal word-delete works.
		if mods&key.ModCtrl != 0 {
			b = []byte{0x08}
		} else {
			b = []byte{0x7f}
		}
	case key.NameEscape:
		b = []byte{0x1b}
	case key.NameSpace:
		if mods&key.ModCtrl != 0 {
			b = []byte{0}
		} else {
			b = []byte{' '}
		}
	default:
		r := []rune(string(name))
		if len(r) != 1 {
			return nil
		}
		c := r[0]
		// Unreserved Cmd/Super combos belong to the OS, not the shell.
		if mods&(key.ModCommand|key.ModSuper) != 0 {
			return nil
		}
		switch {
		case mods&key.ModCtrl != 0 && c == '/':
			b = []byte{0x1f}
		case mods&key.ModCtrl != 0 && (c == '@' || c == '2'):
			b = []byte{0}
		case mods&key.ModCtrl != 0 && c == '?':
			b = []byte{0x7f}
		case mods&key.ModCtrl != 0 && c >= 'A' && c <= 'Z':
			b = []byte{byte(c - 'A' + 1)}
		case mods&key.ModCtrl != 0 && c >= '[' && c <= '_':
			b = []byte{byte(c - '@')}
		case mods&key.ModCtrl != 0:
			return nil
		default:
			s := string(c)
			if c >= 'A' && c <= 'Z' && mods&key.ModShift == 0 {
				s = strings.ToLower(s)
			}
			b = []byte(s)
		}
	}
	if mods&key.ModAlt != 0 {
		return append([]byte{0x1b}, b...)
	}
	return b
}

// EncodeText is typed text, sent as UTF-8.
func EncodeText(s string) []byte { return []byte(s) }

// EncodeFocus reports focus changes when the app enabled mode 1004.
func EncodeFocus(focused bool, m term.Modes) []byte {
	if !m.FocusEvents {
		return nil
	}
	if focused {
		return []byte("\x1b[I")
	}
	return []byte("\x1b[O")
}
