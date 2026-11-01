package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStartDirConfigAndResolution(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "Documents 日本語")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, "file")
	os.WriteFile(file, []byte("x"), 0600)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	cases := []struct {
		name, setting, want string
		warning             bool
	}{
		{"missing", "", "", false}, {"empty", `start_dir = ""`, "", false},
		{"tilde", `start_dir = "~"`, "~", false},
		{"tilde folder", `start_dir = "~/Documents 日本語"`, "~/Documents 日本語", false},
		{"absolute", "start_dir = " + fmt.Sprintf("%q", dir), dir, false},
		{"missing folder", `start_dir = "~/gone"`, "", true},
		{"file", "start_dir = " + fmt.Sprintf("%q", file), "", true},
		{"wrong type", "start_dir = 42", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, w, err := Load(write(t, c.setting))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.StartDir != c.want || (len(w) > 0) != c.warning {
				t.Fatalf("dir=%q warnings=%v", cfg.StartDir, w)
			}
			if c.warning && !strings.Contains(strings.Join(w, " "), "start_dir") {
				t.Fatal(w)
			}
		})
	}
	for _, input := range []string{"~/Documents 日本語", `~\Documents 日本語`, dir} {
		got, err := ResolveStartDir(input, home)
		if err != nil || got != dir {
			t.Fatalf("%q => %q %v", input, got, err)
		}
	}
	got, err := ResolveStartDir("", home)
	if err != nil || got != home {
		t.Fatalf("empty => %q %v", got, err)
	}
}
