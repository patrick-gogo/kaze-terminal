package workspaces

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Execute the generated line: text comparisons alone cannot catch shell
// operator precedence changing the boundary between recipe fields.
func TestJoinedPOSIXFieldsRespectFailureBoundary(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "sh"} {
		t.Run(shell, func(t *testing.T) {
			path, err := exec.LookPath(shell)
			if err != nil {
				t.Skip(err)
			}
			for _, cmds := range [][]string{
				{"false", "false || printf SHOULD_NOT_RUN"},
				{"false", "printf SHOULD_NOT_RUN; true"},
				{"false", "true", "false || printf SHOULD_NOT_RUN"},
			} {
				line, err := JoinLine(ShellPOSIX, cmds)
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				out, _ := exec.CommandContext(ctx, path, "-c", line).CombinedOutput()
				cancel()
				if strings.Contains(string(out), "SHOULD_NOT_RUN") {
					t.Fatalf("second field ran after failure: %q -> %s", line, out)
				}
			}
		})
	}
}

func TestJoinedPOSIXFieldsKeepCurrentShellContext(t *testing.T) {
	for _, shell := range []string{"zsh", "bash", "sh"} {
		t.Run(shell, func(t *testing.T) {
			path, err := exec.LookPath(shell)
			if err != nil {
				t.Skip(err)
			}
			dir := t.TempDir()
			quoted := "'" + strings.ReplaceAll(dir, "'", "'\\''") + "'"
			line, err := JoinLine(ShellPOSIX, []string{"export KAZE_JOIN_TEST=ready; cd " + quoted + ";", "printf '%s:' \"$KAZE_JOIN_TEST\"; pwd"})
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, path, "-c", line+"; printf '%s' \"$KAZE_JOIN_TEST\"").CombinedOutput()
			if err != nil || string(out) != "ready:"+dir+"\nready" {
				t.Fatalf("line %q: output %q, err %v", line, out, err)
			}
		})
	}
}
