package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetReplacesKeepingComments(t *testing.T) {
	head := "# my settings\n"
	tail := "\n[colors]\nbg = \"#101010\" # dark\n"
	p := write(t, head+"font_size = 14  # big\n"+tail)
	if err := Set(p, "font_size", 16); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if want := head + "font_size = 16  # big\n" + tail; string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSetInsertsAboveFirstTable(t *testing.T) {
	p := write(t, "font_size = 14\n\n[colors]\nbg = \"#101010\"\n")
	if err := Set(p, "theme", "sumi"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if strings.Index(string(got), "theme") > strings.Index(string(got), "[colors]") {
		t.Fatalf("theme landed below the table:\n%s", got)
	}
	cfg, warns, err := Load(p)
	if err != nil || len(warns) != 0 || cfg.Theme != "sumi" || cfg.Colors["bg"] != "#101010" {
		t.Fatalf("cfg %+v warns %v err %v", cfg, warns, err)
	}
}

func TestSetAppendsWithoutTable(t *testing.T) {
	p := write(t, "font_size = 14")
	if err := Set(p, "cursor_blink", true); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(p)
	if err != nil || !cfg.CursorBlink || cfg.FontSize != 14 {
		t.Fatalf("cfg %+v err %v", cfg, err)
	}
}

func TestSetKeepsCRLF(t *testing.T) {
	p := write(t, "font_size = 14\r\n\r\n[colors]\r\nbg = \"#101010\"\r\n")
	if err := Set(p, "theme", "tide"); err != nil {
		t.Fatal(err)
	}
	if err := Set(p, "font_size", 15); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if strings.Count(string(got), "\n") != strings.Count(string(got), "\r\n") {
		t.Fatalf("a bare LF crept in: %q", got)
	}
}

func TestSetRefusesBrokenFile(t *testing.T) {
	const body = "font_size = = 3\n"
	p := write(t, body)
	if err := Set(p, "theme", "sumi"); err == nil {
		t.Fatal("no error for an unparsable file")
	}
	got, _ := os.ReadFile(p)
	if string(got) != body {
		t.Fatalf("file changed: %q", got)
	}
}

func TestSetCreatesMissingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.toml")
	if err := Set(p, "theme", "washi"); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(p)
	if err != nil || cfg.Theme != "washi" {
		t.Fatalf("cfg %+v err %v", cfg, err)
	}
	if _, err := os.Stat(p + ".tmp"); err == nil {
		t.Fatal("temp file left behind")
	}
}

func TestSetQuotesStrings(t *testing.T) {
	p := write(t, "")
	const v = `a "quoted" back\slash`
	if err := Set(p, "font_family", v); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(p)
	if err != nil || cfg.FontFamily != v {
		t.Fatalf("font_family %q err %v", cfg.FontFamily, err)
	}
}

func TestSetIgnoresKeyInsideTable(t *testing.T) {
	p := write(t, "[keys]\ntheme = \"x\"\n")
	if err := Set(p, "theme", "sumi"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(got), "theme = \"sumi\"\n[keys]\ntheme = \"x\"") {
		t.Fatalf("got %q", got)
	}
}

func TestLoadNewKeys(t *testing.T) {
	cfg, warns, err := Load(write(t, ""))
	if err != nil || len(warns) != 0 {
		t.Fatal(err, warns)
	}
	if cfg.Theme != "midnight" || cfg.LineSpacing != "normal" || cfg.CursorStyle != "block" ||
		cfg.CursorBlink || !cfg.TaskbarFlash || cfg.DoneToast != "6s" || !cfg.ShowUsage {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	cfg, warns, _ = Load(write(t, "theme = \"sakura\"\nline_spacing = \"roomy\"\ncursor_style = \"bar\"\n"+
		"cursor_blink = true\ntaskbar_flash = false\ndone_toast = \"until_seen\"\nshow_usage = false\n"))
	if len(warns) != 0 || cfg.Theme != "sakura" || cfg.LineSpacing != "roomy" || cfg.CursorStyle != "bar" ||
		!cfg.CursorBlink || cfg.TaskbarFlash || cfg.DoneToast != "until_seen" || cfg.ShowUsage {
		t.Fatalf("cfg %+v warns %v", cfg, warns)
	}
	for _, body := range []string{
		"theme = \"neon\"", "line_spacing = \"huge\"", "cursor_style = \"star\"",
		"cursor_blink = 3", "taskbar_flash = \"yes\"", "done_toast = \"1h\"", "show_usage = 1",
	} {
		cfg, warns, _ := Load(write(t, body))
		if len(warns) != 1 {
			t.Fatalf("%s: warns %v, want 1", body, warns)
		}
		if d := Default(); cfg.Theme != d.Theme || cfg.LineSpacing != d.LineSpacing || cfg.CursorStyle != d.CursorStyle ||
			cfg.CursorBlink != d.CursorBlink || cfg.TaskbarFlash != d.TaskbarFlash || cfg.DoneToast != d.DoneToast || cfg.ShowUsage != d.ShowUsage {
			t.Fatalf("%s: not default: %+v", body, cfg)
		}
	}
}

func TestSetKeepsCommentedCRLFFileWithColors(t *testing.T) {
	const in = "# kaze settings\r\nfont_size = 14  # x\r\n\r\n# colors\r\n[colors]\r\nbg = \"#ffffff\"  # c\r\n"
	p := write(t, in)
	if err := Set(p, "theme", "sumi"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := "# kaze settings\r\nfont_size = 14  # x\r\ntheme = \"sumi\"\r\n\r\n# colors\r\n[colors]\r\nbg = \"#ffffff\"  # c\r\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	cfg, warns, err := Load(p)
	if err != nil || len(warns) != 0 || cfg.Theme != "sumi" || cfg.Colors["bg"] != "#ffffff" {
		t.Fatalf("cfg %+v warns %v err %v", cfg, warns, err)
	}
}

// A stale config.toml.tmp left by another writer must not block or corrupt the save.
func TestSetUsesAFreshTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, "theme", "tide"); err != nil {
		t.Fatalf("Set beside a stale temp: %v", err)
	}
	if b, _ := os.ReadFile(path); string(b) != "theme = \"tide\"\n" {
		t.Fatalf("file %q", b)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 2 {
		t.Fatalf("left behind %v, want only the file and the stale temp", ents)
	}
}

// symlinkOrSkip links link to target, or skips where the OS does not allow it.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not permitted here: %v", err)
	}
}

func TestSetFollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "config.toml")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("font_size = 14\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "config.toml")
	symlinkOrSkip(t, target, link)
	if err := Set(link, "theme", "tide"); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced by a plain file: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(target); string(b) != "font_size = 14\ntheme = \"tide\"\n" {
		t.Fatalf("target %q", b)
	}
}

func TestSetSkipsMultilineShellContent(t *testing.T) {
	for _, quote := range []string{"'''", `"""`} {
		for _, eol := range []string{"\n", "\r\n"} {
			t.Run(quote+eol, func(t *testing.T) {
				shell := "shell = ['/bin/sh', '-c', " + quote + "\ntheme = 'inside shell'\n[not_a_table]\n" + quote + "]\n"
				src := strings.ReplaceAll(shell+"\n[colors]\nbg = '#101010'\n", "\n", eol)
				p := write(t, src)
				before, _, err := Load(p)
				if err != nil {
					t.Fatal(err)
				}
				if err := Set(p, "theme", "tide"); err != nil {
					t.Fatal(err)
				}
				after, _, err := Load(p)
				if err != nil {
					t.Fatal(err)
				}
				if after.Theme != "tide" || strings.Join(after.Shell, "|") != strings.Join(before.Shell, "|") {
					t.Fatalf("theme %q shell changed: %q -> %q", after.Theme, before.Shell, after.Shell)
				}
				got, _ := os.ReadFile(p)
				if !strings.Contains(string(got), strings.ReplaceAll(shell, "\n", eol)) {
					t.Fatalf("shell source changed: %s", got)
				}
			})
		}
	}
}

func TestSetReplacesQuotedKeys(t *testing.T) {
	for _, key := range []string{`"theme"`, `'theme'`, `"\u0074heme"`} {
		t.Run(key, func(t *testing.T) {
			p := write(t, key+" = 'midnight' # keep\n")
			if err := Set(p, "theme", "tide"); err != nil {
				t.Fatal(err)
			}
			cfg, _, err := Load(p)
			if err != nil || cfg.Theme != "tide" {
				t.Fatalf("theme %q err %v", cfg.Theme, err)
			}
		})
	}
}

func TestSetSkipsTableHeaderInsideMultilineString(t *testing.T) {
	src := "shell = ['/bin/sh', '-c', '''\n[inside shell]\n''']\ntheme = 'midnight' # keep\n\n[colors]\nbg = '#101010'\n"
	p := write(t, src)
	if err := Set(p, "theme", "tide"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	want := strings.Replace(src, "theme = 'midnight'", `theme = "tide"`, 1)
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCheckUpdatesDefaultsOnAndCanBeTurnedOff(t *testing.T) {
	if !Default().CheckUpdates {
		t.Fatal("check_updates is off by default")
	}
	p := write(t, "check_updates = false\n")
	cfg, warns, err := Load(p)
	if err != nil || len(warns) != 0 || cfg.CheckUpdates {
		t.Fatalf("cfg %v warns %v err %v", cfg.CheckUpdates, warns, err)
	}
}
