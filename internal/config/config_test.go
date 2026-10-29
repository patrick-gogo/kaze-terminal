package config

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMissingFileGivesDefaults(t *testing.T) {
	cfg, warns, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v", warns)
	}
	d := Default()
	if cfg.FontSize != 14 || cfg.ScrollbackLines != 10000 || cfg.FontFamily != "" || len(cfg.Shell) != 0 {
		t.Fatalf("cfg = %+v, want defaults %+v", cfg, d)
	}
}

func TestBadValuesFallBackWithWarning(t *testing.T) {
	p := write(t, "font_size = -3\n[colors]\nfg = \"#zzz\"\n")
	cfg, warns, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FontSize != 14 {
		t.Fatalf("FontSize = %v, want 14", cfg.FontSize)
	}
	if _, ok := cfg.Colors["fg"]; ok {
		t.Fatalf("bad color kept: %v", cfg.Colors)
	}
	if len(warns) != 2 {
		t.Fatalf("warnings = %v, want 2", warns)
	}
}

func TestScrollbackIsCapped(t *testing.T) {
	cfg, warns, err := Load(write(t, "scrollback_lines = 5_000_000\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ScrollbackLines != 100000 {
		t.Fatalf("ScrollbackLines = %d", cfg.ScrollbackLines)
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want 1", warns)
	}
}

func TestPartialFileKeepsOtherDefaults(t *testing.T) {
	cfg, warns, err := Load(write(t, "font_size = 18\nshell = [\"pwsh\", \"-NoLogo\"]\n[colors]\nbg = \"#1A2B3c\"\n[keys]\nnew_tab = \"ctrl+shift+t\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v", warns)
	}
	if cfg.FontSize != 18 || cfg.ScrollbackLines != 10000 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if len(cfg.Shell) != 2 || cfg.Shell[0] != "pwsh" {
		t.Fatalf("Shell = %v", cfg.Shell)
	}
	if cfg.Colors["bg"] != "#1A2B3c" || cfg.Keys["new_tab"] != "ctrl+shift+t" {
		t.Fatalf("colors=%v keys=%v", cfg.Colors, cfg.Keys)
	}
}

func TestUnknownKeysWarn(t *testing.T) {
	cfg, warns, err := Load(write(t, "bogus = 1\n[colors]\ncolor16 = \"#000000\"\ncolor15 = \"#ffffff\"\n[keys]\nfly = \"ctrl+f\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 3 {
		t.Fatalf("warnings = %v, want 3", warns)
	}
	if cfg.Colors["color15"] != "#ffffff" || len(cfg.Colors) != 1 || len(cfg.Keys) != 0 {
		t.Fatalf("colors=%v keys=%v", cfg.Colors, cfg.Keys)
	}
}

func TestWrongTypeWarns(t *testing.T) {
	cfg, warns, err := Load(write(t, "font_size = \"big\"\nshell = \"pwsh\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FontSize != 14 || len(cfg.Shell) != 0 || len(warns) != 2 {
		t.Fatalf("cfg=%+v warns=%v", cfg, warns)
	}
}

func TestSyntaxErrorFails(t *testing.T) {
	if _, _, err := Load(write(t, "font_size = = 3\n")); err == nil {
		t.Fatal("want error")
	}
}

func TestPathEndsInKazeConfig(t *testing.T) {
	p := filepath.ToSlash(Path())
	if p != "" && !strings.HasSuffix(p, "kaze/config.toml") {
		t.Fatalf("Path = %q", p)
	}
}

func TestFontSizeRejectsNaNInfAndCapsHuge(t *testing.T) {
	for _, tc := range []struct {
		body string
		want float32
	}{
		{"font_size = nan\n", 14},
		{"font_size = inf\n", 14},
		{"font_size = -inf\n", 14},
		{"font_size = 1e30\n", 72},
		{"font_size = 500\n", 72},
		{"font_size = 72\n", 72},
	} {
		cfg, warns, err := Load(write(t, tc.body))
		if err != nil {
			t.Fatalf("%q: %v", tc.body, err)
		}
		if cfg.FontSize != tc.want {
			t.Errorf("%q: FontSize = %v, want %v", tc.body, cfg.FontSize, tc.want)
		}
		wantWarns := 1
		if tc.body == "font_size = 72\n" {
			wantWarns = 0
		}
		if len(warns) != wantWarns {
			t.Errorf("%q: warnings = %v, want %d", tc.body, warns, wantWarns)
		}
	}
}

func TestShellRejectsEmptyProgram(t *testing.T) {
	for _, body := range []string{"shell = [\"\"]\n", "shell = [\"  \", \"-NoLogo\"]\n"} {
		cfg, warns, err := Load(write(t, body))
		if err != nil {
			t.Fatal(err)
		}
		if len(cfg.Shell) != 0 {
			t.Errorf("%q: Shell = %q, want empty (platform default)", body, cfg.Shell)
		}
		if len(warns) != 1 {
			t.Errorf("%q: warnings = %v, want 1", body, warns)
		}
	}
}

func TestKnownActionsAreTheRealOnes(t *testing.T) {
	want := []string{"close_tab", "copy", "new_tab", "next_tab", "open_settings", "paste", "prev_tab"}
	var got []string
	for k := range KnownActions {
		got = append(got, k)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("KnownActions = %v, want %v", got, want)
	}
}

func TestCopyOnSelect(t *testing.T) {
	if Default().CopyOnSelect {
		t.Fatal("copy_on_select defaults to on, want off")
	}
	cfg, warns, err := Load(write(t, "copy_on_select = true\n"))
	if err != nil || len(warns) != 0 || !cfg.CopyOnSelect {
		t.Fatalf("copy_on_select = true: cfg %v, warns %v, err %v", cfg.CopyOnSelect, warns, err)
	}
	cfg, warns, err = Load(write(t, "copy_on_select = \"yes\"\n"))
	if err != nil || len(warns) != 1 || cfg.CopyOnSelect {
		t.Fatalf("copy_on_select = \"yes\": cfg %v, warns %v, err %v", cfg.CopyOnSelect, warns, err)
	}
}

func TestFontSizeHasAMinimum(t *testing.T) {
	cfg, warns, err := Load(write(t, "font_size = 1\n"))
	if err != nil || cfg.FontSize != 4 || len(warns) != 1 {
		t.Fatalf("font_size = 1: size %v, warns %v, err %v; want 4 with a warning", cfg.FontSize, warns, err)
	}
	cfg, warns, _ = Load(write(t, "font_size = 4\n"))
	if cfg.FontSize != 4 || len(warns) != 0 {
		t.Fatalf("font_size = 4: size %v, warns %v", cfg.FontSize, warns)
	}
}

// The bundled font is the only one wired up, so a set font_family must not pass silently.
func TestFontFamilyWarnsNotSupported(t *testing.T) {
	cfg, warns, err := Load(write(t, "font_family = \"Cascadia Mono\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := "font_family is not supported yet; using the bundled font"
	if len(warns) != 1 || warns[0] != want {
		t.Fatalf("warnings = %q, want [%q]", warns, want)
	}
	if cfg.FontFamily != "Cascadia Mono" {
		t.Fatalf("FontFamily = %q, want the value kept", cfg.FontFamily)
	}
}

func TestChimeDefaults(t *testing.T) {
	cfg, warns, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if !cfg.ChimeNeeds || cfg.ChimeDone || cfg.ChimeFile != "" {
		t.Fatalf("chime defaults wrong: %+v", cfg)
	}
}

func TestChimeKeysLoad(t *testing.T) {
	f := filepath.Join(t.TempDir(), "my.wav")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "chime_needs = false\nchime_done = true\nchime_file = '" + f + "'\n"
	cfg, warns, err := Load(write(t, body))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if cfg.ChimeNeeds || !cfg.ChimeDone || cfg.ChimeFile != f {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestChimeInvalidTypesWarn(t *testing.T) {
	cfg, warns, err := Load(write(t, "chime_needs = \"yes\"\nchime_done = 3\nchime_file = 7\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 3 {
		t.Fatalf("warnings = %v, want 3", warns)
	}
	if !cfg.ChimeNeeds || cfg.ChimeDone || cfg.ChimeFile != "" {
		t.Fatalf("did not fall back to defaults: %+v", cfg)
	}
}

func TestChimeMissingFileWarnsAndFallsBack(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone.wav")
	cfg, warns, err := Load(write(t, "chime_file = '"+missing+"'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || cfg.ChimeFile != "" {
		t.Fatalf("warns=%v file=%q", warns, cfg.ChimeFile)
	}
}

func TestChimeFileExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	f := filepath.Join(home, "my.wav")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warns, err := Load(write(t, "chime_file = '~/my.wav'"+string(rune(10))))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if cfg.ChimeFile != f {
		t.Fatalf("ChimeFile = %q, want %q", cfg.ChimeFile, f)
	}
}
