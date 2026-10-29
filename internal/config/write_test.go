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
