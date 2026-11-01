//go:build darwin || linux

package procs

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A real process started as "codex" is named codex; this test process is not an agent.
func TestAgentOfARealProcess(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip(err)
	}
	link := filepath.Join(t.TempDir(), "codex")
	if err := os.Symlink(sleep, link); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(link, "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	if got := Agent(cmd.Process.Pid); got != "codex" {
		t.Fatalf("got %q", got)
	}
	if got := Agent(os.Getpid()); got != "" {
		t.Fatalf("the test binary is %q", got)
	}
	if got := Agent(0); got != "" {
		t.Fatalf("pid 0 is %q", got)
	}
}
