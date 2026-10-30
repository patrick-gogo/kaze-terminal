package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewerVersion(t *testing.T) {
	yes := [][2]string{
		{"1.0.1", "1.0.0"}, {"0.12.0", "0.11.0"}, {"1.10", "1.9"}, {"v2", "1.99"},
		{"1.0.0", "1.0.0-beta"}, {"0.1.0", "f8c5e7a1dcb7"}, {"1.0.0.1", "1.0"},
	}
	for _, c := range yes {
		if !newerVersion(c[0], c[1]) {
			t.Errorf("%s should be newer than %s", c[0], c[1])
		}
		if newerVersion(c[1], c[0]) {
			t.Errorf("%s should not be newer than %s", c[1], c[0])
		}
	}
	if newerVersion("1.0.0", "1.0.0") || newerVersion("1.0", "1.0.0") {
		t.Error("equal versions are not newer")
	}
}

func TestEnabledPlugins(t *testing.T) {
	got := enabledPlugins([]byte(`{"enabledPlugins":{"b@m":true,"x@m":false,"c@m":"yes","a@m":true}}`))
	if strings.Join(got, ",") != "a@m,b@m" {
		t.Fatalf("got %v", got)
	}
	if enabledPlugins([]byte(`{nope`)) != nil || enabledPlugins([]byte(`{"theme":"dark"}`)) != nil {
		t.Fatal("bad or missing settings should list nothing")
	}
}

type rec struct {
	InstallPath string `json:"installPath"`
	Version     string `json:"version"`
	LastUpdated string `json:"lastUpdated"`
}

func record(t *testing.T, plugins map[string][]rec) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"version": 2, "plugins": plugins})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func mkdir(t *testing.T, p string) string {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPluginRootFromInstalledRecord(t *testing.T) {
	home := t.TempDir()
	v100 := mkdir(t, filepath.Join(home, "plugins", "cache", "m", "rl", "1.0.0"))
	v101 := mkdir(t, filepath.Join(home, "plugins", "cache", "m", "rl", "1.0.1"))
	installed := record(t, map[string][]rec{"rl@m": {
		{v101, "1.0.1", "2026-01-01T00:00:00Z"},
		{v100, "1.0.0", "2026-06-01T00:00:00Z"},
	}})
	if got, ok := pluginRoot(home, "rl@m", installed); !ok || got != v101 {
		t.Fatalf("got %q %v, want %q", got, ok, v101)
	}

	// A record pointing outside ~/.claude/plugins, or at nothing, falls back
	// to scanning the cache.
	outside := mkdir(t, filepath.Join(t.TempDir(), "elsewhere"))
	for _, bad := range []string{outside, filepath.Join(home, "plugins", "cache", "m", "rl", "9.9.9")} {
		installed = record(t, map[string][]rec{"rl@m": {{bad, "9.9.9", ""}}})
		if got, ok := pluginRoot(home, "rl@m", installed); !ok || got != v101 {
			t.Fatalf("bad path %q: got %q %v", bad, got, ok)
		}
	}
}

func TestPluginRootScansTheCache(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, "plugins", "cache", "m", "rl", "1.0.0"))
	v101 := mkdir(t, filepath.Join(home, "plugins", "cache", "m", "rl", "1.0.1"))
	if got, ok := pluginRoot(home, "rl@m", nil); !ok || got != v101 {
		t.Fatalf("semver: got %q %v", got, ok)
	}
	a := mkdir(t, filepath.Join(home, "plugins", "cache", "m", "sc", "1da406f428c2"))
	b := mkdir(t, filepath.Join(home, "plugins", "cache", "m", "sc", "f8c5e7a1dcb7"))
	old, newer := time.Now().Add(-time.Hour), time.Now()
	os.Chtimes(b, old, old)
	os.Chtimes(a, newer, newer)
	if got, ok := pluginRoot(home, "sc@m", []byte(`{bad json`)); !ok || got != a {
		t.Fatalf("hashes: got %q %v, want %q", got, ok, a)
	}
	if _, ok := pluginRoot(home, "none@m", nil); ok {
		t.Fatal("a plugin that is not installed has no root")
	}
	if _, ok := pluginRoot(home, "../../etc@m", nil); ok {
		t.Fatal("a key with a path in it has no root")
	}
}

func TestLoadPluginSkills(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, "settings.json"), `{"enabledPlugins":{"superpowers@m":true}}`)
	root := filepath.Join(home, "plugins", "cache", "m", "superpowers")
	put(t, filepath.Join(root, "7.0.0", "skills", "brainstorming", "SKILL.md"), skillFile("brainstorming", "Think first"))
	put(t, filepath.Join(root, "7.0.0", "commands", "help.md"), "Help\n")
	put(t, filepath.Join(root, "6.0.0", "skills", "old", "SKILL.md"), skillFile("old", "Old"))
	got := NewCache().Load(Sources{ClaudeDir: home})
	if strings.Join(names(got), ",") != "superpowers:brainstorming,superpowers:help" {
		t.Fatalf("names %v", names(got))
	}
	if b, _ := find(got, "superpowers:brainstorming"); b.Scope != Plugin || b.Desc != "Think first" {
		t.Fatalf("got %+v", b)
	}
}

func TestLoadSkipsDisabledAndMissingPlugins(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, "settings.json"), `{"enabledPlugins":{"off@m":false,"gone@m":true}}`)
	put(t, filepath.Join(home, "plugins", "cache", "m", "off", "1.0.0", "skills", "x", "SKILL.md"), skillFile("x", "x"))
	put(t, filepath.Join(home, "skills", "mine", "SKILL.md"), skillFile("mine", "Mine"))
	got := NewCache().Load(Sources{ClaudeDir: home})
	if strings.Join(names(got), ",") != "mine" {
		t.Fatalf("names %v", names(got))
	}
}

// Claude Code names a plugin's nested command by its folders too
// ("plugin:analytics:funnel"), so same-named files in two folders both list.
func TestPluginNestedCommandsKeepTheirFolder(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, "settings.json"), `{"enabledPlugins":{"mk@m":true}}`)
	root := filepath.Join(home, "plugins", "cache", "m", "mk", "1.0.0", "commands")
	put(t, filepath.Join(root, "analytics", "funnel.md"), "Funnel\n")
	put(t, filepath.Join(root, "training-de", "start.md"), "De\n")
	put(t, filepath.Join(root, "training-fr", "start.md"), "Fr\n")
	put(t, filepath.Join(home, "commands", "sub", "mine.md"), "Mine\n")
	got := NewCache().Load(Sources{ClaudeDir: home})
	if strings.Join(names(got), ",") != "mine,mk:analytics:funnel,mk:training-de:start,mk:training-fr:start" {
		t.Fatalf("names %v", names(got))
	}
}
