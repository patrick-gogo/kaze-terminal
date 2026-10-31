package input_test

import (
	"testing"

	"gioui.org/io/key"
	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

func chk(t *testing.T, got []byte, want string) {
	t.Helper()
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestArrowsNormalAndAppMode(t *testing.T) {
	chk(t, input.EncodeKey(key.NameUpArrow, 0, term.Modes{}), "\x1b[A")
	chk(t, input.EncodeKey(key.NameUpArrow, 0, term.Modes{AppCursorKeys: true}), "\x1bOA")
}

func TestCtrlLetters(t *testing.T) {
	chk(t, input.EncodeKey("C", key.ModCtrl, term.Modes{}), "\x03")
	chk(t, input.EncodeKey("A", key.ModCtrl, term.Modes{}), "\x01")
	chk(t, input.EncodeKey("Z", key.ModCtrl, term.Modes{}), "\x1a")
}

func TestAltPrefixesEscape(t *testing.T) {
	chk(t, input.EncodeKey("B", key.ModAlt, term.Modes{}), "\x1bb")
	chk(t, input.EncodeKey("B", key.ModAlt|key.ModShift, term.Modes{}), "\x1bB")
}

func TestEnterTabBackspaceEscape(t *testing.T) {
	m := term.Modes{}
	chk(t, input.EncodeKey(key.NameReturn, 0, m), "\r")
	chk(t, input.EncodeKey(key.NameEnter, 0, m), "\r")
	chk(t, input.EncodeKey(key.NameTab, 0, m), "\t")
	chk(t, input.EncodeKey(key.NameTab, key.ModShift, m), "\x1b[Z")
	chk(t, input.EncodeKey(key.NameDeleteBackward, 0, m), "\x7f")
	chk(t, input.EncodeKey(key.NameEscape, 0, m), "\x1b")
	chk(t, input.EncodeKey(key.NameSpace, 0, m), " ")
}

func TestFunctionAndNavKeys(t *testing.T) {
	m := term.Modes{}
	chk(t, input.EncodeKey(key.NameF1, 0, m), "\x1bOP")
	chk(t, input.EncodeKey(key.NameF5, 0, m), "\x1b[15~")
	chk(t, input.EncodeKey(key.NameF12, 0, m), "\x1b[24~")
	chk(t, input.EncodeKey(key.NameHome, 0, m), "\x1b[H")
	chk(t, input.EncodeKey(key.NameHome, 0, term.Modes{AppCursorKeys: true}), "\x1bOH")
	chk(t, input.EncodeKey(key.NameEnd, 0, m), "\x1b[F")
	chk(t, input.EncodeKey(key.NamePageUp, 0, m), "\x1b[5~")
	chk(t, input.EncodeKey(key.NamePageDown, 0, m), "\x1b[6~")
	chk(t, input.EncodeKey(key.NameDeleteForward, 0, m), "\x1b[3~")
	chk(t, input.EncodeKey(key.NameLeftArrow, key.ModCtrl, m), "\x1b[1;5D")
	chk(t, input.EncodeKey(key.NameLeftArrow, key.ModCtrl, term.Modes{AppCursorKeys: true}), "\x1b[1;5D")
	chk(t, input.EncodeKey(key.NameF1, key.ModShift, m), "\x1b[1;2P")
	chk(t, input.EncodeKey(key.NameDeleteForward, key.ModAlt, m), "\x1b[3;3~")
}

func TestKazeShortcutsReturnNil(t *testing.T) {
	win := func(n key.Name, mods key.Modifiers) bool { return input.IsReserved(n, mods, "windows") }
	mac := func(n key.Name, mods key.Modifiers) bool { return input.IsReserved(n, mods, "darwin") }
	cs := key.ModCtrl | key.ModShift
	for _, n := range []key.Name{"C", "V", "T", "W"} {
		if !win(n, cs) || !mac(n, key.ModCommand) {
			t.Fatalf("%s not reserved", n)
		}
	}
	if !win(key.NameTab, key.ModCtrl) || !win(key.NameTab, cs) {
		t.Fatal("tab switching not reserved")
	}
	if win("C", key.ModCtrl) || mac("C", key.ModCtrl) {
		t.Fatal("plain Ctrl+C must reach the shell")
	}
}

// A lone modifier press is not a character: Gio names Command "⌘", one rune,
// and macOS can report it with no modifier bit set.
func TestModifierKeysAloneSendNothing(t *testing.T) {
	for _, n := range []key.Name{key.NameCommand, key.NameCtrl, key.NameShift, key.NameAlt, key.NameSuper} {
		for _, mods := range []key.Modifiers{0, key.ModCommand, key.ModShift} {
			if b := input.EncodeKey(n, mods, term.Modes{}); b != nil {
				t.Errorf("%q with mods %v sent %q", n, mods, b)
			}
		}
	}
}
