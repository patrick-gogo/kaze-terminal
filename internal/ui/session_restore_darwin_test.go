package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/sessionstate"
)

// Reproduce an app launched with a minimal PATH while zsh adds the agent's
// location during startup. Only temporary agents run; no real sessions open.
func TestResumeResolvesAgentFromZshStartup(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"claude", "codex"} {
		if err := os.WriteFile(filepath.Join(bin, agent), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte("export PATH=\"$ZDOTDIR/bin\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")
	t.Setenv("ZDOTDIR", dir)
	id := "01234567-89ab-cdef-0123-456789abcdef"
	for _, agent := range []string{"claude", "codex"} {
		win := &window{}
		win.opt.Shell = []string{"/bin/zsh"}
		line, err := win.resumeLine(sessionstate.Entry{Agent: agent, Session: id})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, err := exec.CommandContext(ctx, "/bin/zsh", "-d", "-i", "-c", line).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%s: %v: %s", agent, err, out)
		}
		verb := "resume"
		if agent == "claude" {
			verb = "--resume"
		}
		if string(out) != verb+"\n"+id+"\n" {
			t.Fatalf("%s got %q", agent, out)
		}
		if err := os.Remove(filepath.Join(bin, agent)); err != nil {
			t.Fatal(err)
		}
		ctx, cancel = context.WithTimeout(context.Background(), 3*time.Second)
		out, err = exec.CommandContext(ctx, "/bin/zsh", "-d", "-i", "-c", line).CombinedOutput()
		cancel()
		if err == nil || !strings.Contains(string(out), "command not found: "+agent) {
			t.Fatalf("missing %s must show the shell error: %v, %q", agent, err, out)
		}
	}
}
