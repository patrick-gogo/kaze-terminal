//go:build darwin || linux

package ptyx_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/procs"
	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
)

// The program in front of the pty is the one Start ran; kaze names it from its group.
func TestForegroundIsTheRunningProgram(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip(err)
	}
	link := filepath.Join(t.TempDir(), "codex")
	if err := os.Symlink(sleep, link); err != nil {
		t.Fatal(err)
	}
	s, err := ptyx.Start([]string{link, "5"}, 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	fg, ok := s.(ptyx.Foregrounder)
	if !ok {
		t.Fatal("session has no Foreground")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pg := fg.Foreground(); pg > 0 && procs.Agent(pg) == "codex" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("foreground %d is not codex", fg.Foreground())
}
