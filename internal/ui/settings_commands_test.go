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
	i := slices.IndexFunc(win.sections(), func(s settingsSection) bool { return s.title == "Shortcuts" })
	if i < 0 {
		t.Fatal("missing Shortcuts section")
	}
	win.selectSection(i)
	win.drawSettings(pageContext(1000, 900), image.Rect(0, 38, 1000, 876))
	return strings.Join(win.set.texts, "\n")
}

func TestCommandsAfterAgents(t *testing.T) {
	var got []string
	for _, s := range defaultSections() {
		got = append(got, s.title)
	}
	if strings.Join(got, " ") != "Appearance Workspaces Agents Shortcuts Updates About" {
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
	for _, want := range []string{"SESSIONS", "KAZE", "EDITING AND MOUSE", "COMMAND LINE", "Shift + drag", "kaze --version", "kaze statusline"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
	if !hasKeys(win, "Ctrl", "Shift", "K") {
		t.Errorf("no Ctrl Shift K key caps in %q", win.set.texts)
	}
}

func TestCommandsShowsOverrides(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.binds, _ = resolveBindings("windows", map[string]string{"palette": "ctrl+shift+p"})
	text := commandsPage(t, win)
	if !hasKeys(win, "Ctrl", "Shift", "P") || hasKeys(win, "Ctrl", "Shift", "K") {
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

// hasKeys reports whether the page drew the key caps keys one after another.
func hasKeys(win *window, keys ...string) bool {
	t := win.set.texts
	for i := 0; i+len(keys) <= len(t); i++ {
		if slices.Equal(t[i:i+len(keys)], keys) {
			return true
		}
	}
	return false
}

func TestShortcutSearchNarrowsTheList(t *testing.T) {
	win, _ := alertWindow(t, 1)
	commandsPage(t, win)
	if len(controlsOf(win, "cmd_filter")) != 1 {
		t.Fatal("no search field")
	}
	win.set.cmdFilter = "scratch"
	text := commandsPage(t, win)
	if !strings.Contains(text, "Scratch shell") || strings.Contains(text, "New session") || strings.Contains(text, "SESSIONS") {
		t.Fatalf("search for scratch shows:\n%s", text)
	}
	win.set.cmdFilter = "zzz"
	if text := commandsPage(t, win); !strings.Contains(text, "No shortcut matches") {
		t.Fatalf("no empty message:\n%s", text)
	}
}
