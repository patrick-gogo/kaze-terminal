package ui

import (
	"image/color"
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/key"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

func TestGridSizeFromWindow(t *testing.T) {
	for _, tc := range []struct {
		w, h, cw, ch, cols, rows int
	}{
		{800, 600, 8, 16, 100, 37},
		{7, 15, 8, 16, 1, 1},                 // smaller than one cell
		{0, -40, 8, 16, 1, 1},                // minimized window
		{800, 600, 0, 0, 1, 1},               // no font measured yet
		{1 << 30, 1 << 30, 1, 1, 4096, 4096}, // absurd sizes stay bounded
	} {
		c, r := gridSize(tc.w, tc.h, tc.cw, tc.ch)
		if c != tc.cols || r != tc.rows {
			t.Errorf("gridSize(%d,%d,%d,%d) = %d,%d want %d,%d", tc.w, tc.h, tc.cw, tc.ch, c, r, tc.cols, tc.rows)
		}
	}
}

func TestWheelMapsGioSignToLinesUp(t *testing.T) {
	var w wheel
	if n := w.notches(-120, 120); n != 1 {
		t.Fatalf("Gio scroll -120 (wheel away) = %d notches, want +1 (up)", n)
	}
	if n := w.notches(240, 120); n != -2 {
		t.Fatalf("Gio scroll +240 = %d, want -2 (down)", n)
	}
	// Touchpads send small steps; they add up to a notch.
	got := 0
	for i := 0; i < 3; i++ {
		got += w.notches(-40, 120)
	}
	if got != 1 {
		t.Fatalf("three -40 steps = %d notches, want 1", got)
	}
	if n := w.notches(-60, 0); n != 0 {
		t.Fatalf("zero unit = %d, want 0", n)
	}
}

func TestDefaultBindingsCoverKnownActions(t *testing.T) {
	for _, goos := range []string{"windows", "darwin"} {
		km, warns := buildKeymap(goos, nil)
		if len(warns) != 0 {
			t.Fatalf("%s warnings: %v", goos, warns)
		}
		seen := map[action]bool{}
		for _, a := range km {
			seen[a] = true
		}
		for name := range config.KnownActions {
			if !seen[actionByName[name]] || actionByName[name] == actNone {
				t.Errorf("%s: action %q has no default binding", goos, name)
			}
		}
	}
}

func TestParseShortcut(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want input.Shortcut
		ok   bool
	}{
		{"ctrl+shift+t", input.Shortcut{Name: "T", Mods: key.ModCtrl | key.ModShift}, true},
		{"Cmd+T", input.Shortcut{Name: "T", Mods: key.ModCommand}, true},
		{"ctrl+tab", input.Shortcut{Name: key.NameTab, Mods: key.ModCtrl}, true},
		{"alt+pageup", input.Shortcut{Name: key.NamePageUp, Mods: key.ModAlt}, true},
		{"ctrl + shift + /", input.Shortcut{Name: "/", Mods: key.ModCtrl | key.ModShift}, true},
		{"ctrl+f5", input.Shortcut{Name: key.NameF5, Mods: key.ModCtrl}, true},
		{"ctrl+", input.Shortcut{}, false},
		{"hyper+t", input.Shortcut{}, false},
		{"ctrl+t+y", input.Shortcut{}, false},
	} {
		got, ok := parseShortcut(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("parseShortcut(%q) = %+v,%v want %+v,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestConfigKeysOverrideDefaults(t *testing.T) {
	km, warns := buildKeymap("windows", map[string]string{
		"new_tab":   "ctrl+alt+n",
		"close_tab": "nonsense+key",
	})
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want 1 (the bad close_tab)", warns)
	}
	if km[input.Shortcut{Name: "N", Mods: key.ModCtrl | key.ModAlt}] != actNewTab {
		t.Fatal("new_tab override not bound")
	}
	if _, ok := km[input.Shortcut{Name: "T", Mods: key.ModCtrl | key.ModShift}]; ok {
		t.Fatal("the replaced default still bound")
	}
	if km[input.Shortcut{Name: "W", Mods: key.ModCtrl | key.ModShift}] != actCloseTab {
		t.Fatal("bad override must keep the default")
	}
}

func TestConfigKeysRejectClashes(t *testing.T) {
	km, warns := buildKeymap("windows", map[string]string{"paste": "ctrl+shift+t"})
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want 1", warns)
	}
	if km[input.Shortcut{Name: "T", Mods: key.ModCtrl | key.ModShift}] != actNewTab {
		t.Fatal("a clashing override replaced new_tab")
	}
	if km[input.Shortcut{Name: "V", Mods: key.ModCtrl | key.ModShift}] != actPaste {
		t.Fatal("paste lost its default after a clash")
	}
}

func TestRouteKey(t *testing.T) {
	winKM, _ := buildKeymap("windows", nil)
	macKM, _ := buildKeymap("darwin", nil)
	none := term.Modes{}
	for _, tc := range []struct {
		goos string
		name key.Name
		mods key.Modifiers
		act  action
		data string
		alt  bool
	}{
		{"windows", "C", key.ModCtrl, actNone, "\x03", false},
		{"windows", "C", key.ModCtrl | key.ModShift, actCopy, "", false},
		{"windows", "V", key.ModCtrl | key.ModShift, actPaste, "", false},
		{"windows", "T", key.ModCtrl | key.ModShift, actNewTab, "", false},
		{"windows", "W", key.ModCtrl | key.ModShift, actCloseTab, "", false},
		{"windows", key.NameTab, key.ModCtrl, actNextTab, "", false},
		{"windows", key.NameTab, key.ModCtrl | key.ModShift, actPrevTab, "", false},
		{"windows", key.NameTab, 0, actNone, "\t", false},
		{"windows", "A", 0, actNone, "", false},            // the text event types it
		{"windows", "A", key.ModShift, actNone, "", false}, // likewise
		{"windows", key.NameSpace, 0, actNone, "", false},  // likewise
		{"windows", key.NameSpace, key.ModCtrl, actNone, "\x00", false},
		{"windows", "Q", key.ModCtrl | key.ModAlt, actNone, "", false}, // AltGr: the text event has the character
		{"windows", "A", key.ModAlt, actNone, "\x1ba", true},
		{"windows", "/", key.ModCtrl | key.ModShift, actNone, "\x7f", false}, // Ctrl+? on a US layout
		{"windows", "/", key.ModCtrl, actNone, "\x1f", false},
		{"windows", key.NameReturn, 0, actNone, "\r", false},
		{"windows", key.NameUpArrow, 0, actNone, "\x1b[A", false},
		{"windows", key.NameCtrl, key.ModCtrl, actNone, "", false},
		{"darwin", "T", key.ModCommand, actNewTab, "", false},
		{"darwin", "V", key.ModCommand, actPaste, "", false},
		{"darwin", key.NameTab, key.ModCtrl, actNextTab, "", false},
		{"darwin", "A", key.ModAlt, actNone, "\x1ba", true}, // Option as Alt
		{"darwin", "K", key.ModCommand, actNone, "", false},
		{"darwin", "C", key.ModCtrl, actNone, "\x03", false},
	} {
		km := winKM
		if tc.goos == "darwin" {
			km = macKM
		}
		got := routeKey(tc.goos, km, tc.name, tc.mods, none)
		if got.act != tc.act || string(got.data) != tc.data || got.alt != tc.alt {
			t.Errorf("%s %q mods=%v: got act=%v data=%q alt=%v; want act=%v data=%q alt=%v",
				tc.goos, tc.name, tc.mods, got.act, got.data, got.alt, tc.act, tc.data, tc.alt)
		}
	}
}

// Option+A on macOS arrives as a key press and then a text event "å"; only the ESC a may reach the shell.
func TestAltKeySuppressesItsTextEvent(t *testing.T) {
	var g textGate
	g.onKey(true, keyOut{data: []byte("\x1ba"), alt: true})
	if g.allow() {
		t.Fatal("text after an Alt-encoded press must be dropped")
	}
	g.onKey(false, keyOut{}) // the release
	if !g.allow() {
		t.Fatal("text must flow again after the key is released")
	}
	g.onKey(true, keyOut{}) // a plain press whose text comes as an event
	if !g.allow() {
		t.Fatal("plain typing suppressed")
	}
}

func TestIMEPreeditIsNotSentOnlyCommits(t *testing.T) {
	active := key.CompositionEvent{Start: 0, End: 2}
	done := key.CompositionEvent{Start: -1, End: -1}
	edit := func(s string) key.EditEvent { return key.EditEvent{Text: s} }
	for _, tc := range []struct {
		name      string
		evs       []event.Event
		composing bool
		want      string
		after     bool
	}{
		{"plain typing", []event.Event{edit("a"), edit("b")}, false, "ab", false},
		{"preedit", []event.Event{edit("に"), active}, false, "", true},
		{"preedit update while composing", []event.Event{edit("にほ")}, true, "", true},
		{"commit", []event.Event{edit("日本"), done}, true, "日本", false},
		{"commit then new preedit", []event.Event{edit("日本"), done, edit("ご"), active}, true, "日本", true},
		{"cancel", []event.Event{done}, true, "", false},
	} {
		got, after := committedText(tc.evs, tc.composing)
		if got != tc.want || after != tc.after {
			t.Errorf("%s: got %q composing=%v, want %q composing=%v", tc.name, got, after, tc.want, tc.after)
		}
	}
}

func TestConfigColorsOverridePalette(t *testing.T) {
	p := applyColors(render.DefaultPalette(), map[string]string{"bg": "#000000", "color1": "#FF0010", "cursor": "bad"})
	if p.BG != (color.NRGBA{A: 0xff}) {
		t.Fatalf("BG = %v", p.BG)
	}
	if p.ANSI[1] != (color.NRGBA{R: 0xff, G: 0, B: 0x10, A: 0xff}) {
		t.Fatalf("ANSI[1] = %v", p.ANSI[1])
	}
	if p.Cursor != render.DefaultPalette().Cursor {
		t.Fatal("a bad color must keep the default")
	}
}

func TestTabLabel(t *testing.T) {
	if got := tabLabel(&tabs.Tab{Title: ""}, "pwsh"); got != "pwsh" {
		t.Fatalf("empty title = %q", got)
	}
	path := "C:\\Program Files\\WindowsApps\\Microsoft.PowerShell_7.6.9.0_x64__8wekyb3d8bbwe\\pwsh.exe"
	if got := tabLabel(&tabs.Tab{Title: path}, "x"); got != "pwsh.exe" {
		t.Fatalf("path title = %q, want the file name", got)
	}
	long := "a very long window title that an app might set for itself"
	got := tabLabel(&tabs.Tab{Title: long}, "pwsh")
	if r := []rune(got); len(r) > maxLabel || r[len(r)-1] != '…' {
		t.Fatalf("long title = %q", got)
	}
	if got := tabLabel(&tabs.Tab{Title: "claude", Exited: true}, "pwsh"); got != "claude · exited" {
		t.Fatalf("exited = %q", got)
	}
}

func TestToggleSidebarDefaults(t *testing.T) {
	if got := defaultBindings("windows")["toggle_sidebar"]; got != (input.Shortcut{Name: "B", Mods: key.ModCtrl | key.ModShift}) {
		t.Errorf("windows toggle_sidebar = %+v, want ctrl+shift+B", got)
	}
	if got := defaultBindings("darwin")["toggle_sidebar"]; got != (input.Shortcut{Name: "B", Mods: key.ModCommand}) {
		t.Errorf("darwin toggle_sidebar = %+v, want cmd+B", got)
	}
	if !config.KnownActions["toggle_sidebar"] {
		t.Error("toggle_sidebar is not a known action")
	}
	if actionByName["toggle_sidebar"] != actToggleSidebar || actToggleSidebar == actNone {
		t.Error("toggle_sidebar does not map to actToggleSidebar")
	}
}

func TestNextWaitingDefaults(t *testing.T) {
	if got := defaultBindings("windows")["next_waiting"]; got != (input.Shortcut{Name: "J", Mods: key.ModCtrl | key.ModShift}) {
		t.Errorf("windows next_waiting = %+v, want ctrl+shift+J", got)
	}
	if got := defaultBindings("darwin")["next_waiting"]; got != (input.Shortcut{Name: "J", Mods: key.ModCommand}) {
		t.Errorf("darwin next_waiting = %+v, want cmd+J", got)
	}
	if !config.KnownActions["next_waiting"] {
		t.Error("next_waiting is not a known action")
	}
	if actionByName["next_waiting"] != actNextWaiting || actNextWaiting == actNone {
		t.Error("next_waiting does not map to actNextWaiting")
	}
}
