package ui

import (
	"image"
	"slices"
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

func commandsPage(t *testing.T, win *window) string {
	t.Helper()
	win.openSettings()
	i := slices.IndexFunc(win.sections(), func(s settingsSection) bool { return s.title == "Commands" })
	if i < 0 {
		t.Fatal("missing Commands section")
	}
	win.selectSection(i)
	win.drawSettings(pageContext(1000, 900), image.Rect(0, 38, 1000, 876))
	return strings.Join(win.set.texts, "\n")
}

func TestCommandsAfterClaude(t *testing.T) {
	var got []string
	for _, s := range defaultSections() {
		got = append(got, s.title)
	}
	if strings.Join(got, " ") != "Appearance Workspaces Claude Commands Updates About" {
		t.Fatalf("sections %v", got)
	}
}

func TestCommandsListsEveryShortcut(t *testing.T) {
	win, _ := alertWindow(t, 1)
	text := commandsPage(t, win)
	for _, c := range kazeCommands {
		if !strings.Contains(text, c.Label) || !strings.Contains(text, c.Config) {
			t.Errorf("missing %s / %s", c.Label, c.Config)
		}
	}
	for _, want := range []string{"SHORTCUTS", "COMMAND LINE", "Ctrl+Shift+K", "Shift + drag", "kaze --version", "kaze statusline"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestCommandsShowsOverrides(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.binds, _ = resolveBindings("windows", map[string]string{"palette": "ctrl+shift+p"})
	text := commandsPage(t, win)
	if !strings.Contains(text, "Ctrl+Shift+P") || strings.Contains(text, "Ctrl+Shift+K") {
		t.Fatalf("override not shown:\n%s", text)
	}
}

func TestCommandsCopy(t *testing.T) {
	win, _ := alertWindow(t, 1)
	commandsPage(t, win)
	var copied string
	win.toClipboard = func(s string) { copied = s }
	found := false
	for _, c := range win.set.controls {
		if c.id == "cmd_copy" && commandLines[c.arg].Example == "kaze --workspace <name>" {
			commandsAct(win, c.id, c.arg)
			found = true
		}
	}
	if !found {
		t.Fatal("no Copy for --workspace")
	}
	commandsPage(t, win)
	if copied != `kaze --workspace "<name>"` || !strings.HasPrefix(win.notice, "Copied: ") {
		t.Fatalf("copied %q, notice %q", copied, win.notice)
	}
}

func TestHookCommandsHaveNoCopy(t *testing.T) {
	win, _ := alertWindow(t, 1)
	commandsPage(t, win)
	for _, c := range win.set.controls {
		if c.id != "cmd_copy" {
			continue
		}
		if ex := commandLines[c.arg].Example; strings.Contains(ex, "notify") || strings.Contains(ex, "statusline") {
			t.Errorf("%q has a Copy button", ex)
		}
	}
}

func TestCommandsDrawsInEveryTheme(t *testing.T) {
	win, _ := alertWindow(t, 1)
	for _, th := range theme.All() {
		win.applyTheme(th)
		commandsPage(t, win)
	}
}
