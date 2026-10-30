package ui

import (
	"testing"

	"gioui.org/io/key"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/input"
)

func TestResolveBindingsKeepsOverrides(t *testing.T) {
	b, warns := resolveBindings("windows", map[string]string{"palette": "ctrl+shift+p"})
	if len(warns) != 0 || b["palette"] != (input.Shortcut{Name: "P", Mods: key.ModCtrl | key.ModShift}) {
		t.Fatalf("binds %+v, warns %v", b["palette"], warns)
	}
	b, warns = resolveBindings("windows", map[string]string{"palette": "ctrl+nope+p"})
	if len(warns) != 1 || b["palette"] != defaultBindings("windows")["palette"] {
		t.Fatalf("bad text: binds %+v, warns %v", b["palette"], warns)
	}
}

func TestFormatShortcut(t *testing.T) {
	cs := key.ModCtrl | key.ModShift
	cases := []struct {
		goos string
		sc   input.Shortcut
		want string
	}{
		{"windows", input.Shortcut{Name: "K", Mods: cs}, "Ctrl+Shift+K"},
		{"darwin", input.Shortcut{Name: "K", Mods: key.ModCommand}, "Cmd+K"},
		{"windows", input.Shortcut{Name: key.NameTab, Mods: key.ModCtrl}, "Ctrl+Tab"},
		{"darwin", input.Shortcut{Name: "`", Mods: key.ModCtrl}, "Ctrl+`"},
		{"windows", input.Shortcut{Name: "T", Mods: cs}, "Ctrl+Shift+T"},
		{"darwin", input.Shortcut{Name: ",", Mods: key.ModCommand}, "Cmd+,"},
		{"windows", input.Shortcut{Name: "X", Mods: key.ModSuper}, "Win+X"},
		{"darwin", input.Shortcut{Name: "A", Mods: key.ModAlt}, "Option+A"},
		{"windows", input.Shortcut{Name: "A", Mods: key.ModAlt}, "Alt+A"},
		{"darwin", input.Shortcut{Name: key.NameTab, Mods: cs}, "Ctrl+Shift+Tab"},
		{"windows", input.Shortcut{Name: key.NameReturn, Mods: key.ModCtrl}, "Ctrl+Enter"},
		{"windows", input.Shortcut{Name: key.NameEscape}, "Esc"},
		{"windows", input.Shortcut{Name: key.NameUpArrow, Mods: key.ModShift}, "Shift+Up"},
		{"windows", input.Shortcut{Name: key.NamePageUp}, "PageUp"},
		{"windows", input.Shortcut{Name: key.NameF5}, "F5"},
		{"darwin", input.Shortcut{Name: "K", Mods: key.ModCommand | key.ModShift | key.ModCtrl | key.ModAlt}, "Ctrl+Option+Shift+Cmd+K"},
	}
	for _, c := range cases {
		if got := formatShortcut(c.goos, c.sc); got != c.want {
			t.Errorf("%s %+v: got %q, want %q", c.goos, c.sc, got, c.want)
		}
	}
}

func TestEveryKnownActionIsListed(t *testing.T) {
	seen := map[string]int{}
	for _, c := range kazeCommands {
		seen[c.Config]++
		if c.Act != actionByName[c.Config] || c.Label == "" {
			t.Errorf("%+v does not match actionByName", c)
		}
	}
	for name := range config.KnownActions {
		if seen[name] != 1 {
			t.Errorf("%s listed %d times", name, seen[name])
		}
	}
	if len(kazeCommands) != len(config.KnownActions) {
		t.Errorf("%d commands for %d actions", len(kazeCommands), len(config.KnownActions))
	}
	if kazeCommands[0].Config != "new_tab" || kazeCommands[len(kazeCommands)-1].Config != "palette" {
		t.Errorf("order: first %s, last %s", kazeCommands[0].Config, kazeCommands[len(kazeCommands)-1].Config)
	}
}
