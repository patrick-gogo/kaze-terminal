package ui

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codexWindow(t *testing.T) *window {
	t.Helper()
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 1100, 850)
	win.codexDir = filepath.Join(f.dir, ".codex")
	if err := os.Mkdir(win.codexDir, 0700); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(win.codexDir, "config.toml"), []byte("notify = ['leave-alone']\n"), 0600)
	win.loadAgents()
	win.drawSettings(pageContext(1100, 850), image.Rect(0, 38, 1100, 826))
	return win
}
func TestCodexFixRequiresConfirmationAndTrustReminder(t *testing.T) {
	win := codexWindow(t)
	path := filepath.Join(win.codexDir, "hooks.json")
	pressAny(t, win, "codex_fix", 0)
	if win.codex.dialog == nil {
		t.Fatal("no preview")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("wrote before confirmation")
	}
	pressAny(t, win, "dialog_cancel", 0)
	if win.codex.dialog != nil {
		t.Fatal("cancel left preview")
	}
	pressAny(t, win, "codex_fix", 0)
	pressAny(t, win, "dialog_write", 0)
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(b, []byte("--agent codex")) {
		t.Fatalf("write %s %v", b, err)
	}
	if !win.codex.trust {
		t.Fatal("missing trust reminder")
	}
	cfg, _ := os.ReadFile(filepath.Join(win.codexDir, "config.toml"))
	if string(cfg) != "notify = ['leave-alone']\n" {
		t.Fatal("config changed")
	}
	win.loadAgents()
	if !win.codex.trust {
		t.Fatal("refresh dismissed reminder")
	}
	pressAny(t, win, "codex_trust_dismiss", 0)
	if win.codex.trust {
		t.Fatal("reminder not dismissed")
	}
}
func TestCodexFixDoesNotOverwriteChangedSources(t *testing.T) {
	for _, name := range []string{"hooks.json", "config.toml"} {
		t.Run(name, func(t *testing.T) {
			win := codexWindow(t)
			win.openCodexFix()
			changed := []byte("{}\n")
			if name == "config.toml" {
				changed = []byte("notify=['changed']\n")
			}
			path := filepath.Join(win.codexDir, name)
			os.WriteFile(path, changed, 0600)
			win.writeCodex()
			got, _ := os.ReadFile(path)
			if !bytes.Equal(got, changed) || !strings.Contains(win.notice, "changed") {
				t.Fatalf("stale write %s %q", got, win.notice)
			}
		})
	}
}
func TestCodexBackupAndMissingInstall(t *testing.T) {
	win := codexWindow(t)
	path := filepath.Join(win.codexDir, "hooks.json")
	before := []byte(`{"description":"keep this"}`)
	os.WriteFile(path, before, 0600)
	win.loadAgents()
	win.openCodexFix()
	win.writeCodex()
	backups, _ := filepath.Glob(path + ".bak-*")
	if len(backups) != 1 {
		t.Fatalf("backups %v", backups)
	}
	old, _ := os.ReadFile(backups[0])
	if !bytes.Equal(old, before) {
		t.Fatal("incorrect backup")
	}
	win.codexDir = filepath.Join(t.TempDir(), "missing")
	win.loadAgents()
	for _, r := range win.codexRows() {
		for _, c := range r.ctrls {
			if c.id == "codex_fix" {
				t.Fatal("offered Fix without Codex")
			}
		}
	}
}
