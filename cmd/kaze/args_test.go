package main

import (
	"bytes"
	"strings"
	"testing"
)

// workspaceArg runs in main before the window is created; a nameless
// --workspace must end the process there with exit code 2.
func TestWorkspaceArg(t *testing.T) {
	for _, c := range []struct {
		args []string
		name string
		code int
	}{
		{nil, "", 0},
		{[]string{"--workspace", "My project"}, "My project", 0},
		{[]string{"--workspace=samurai"}, "samurai", 0},
		{[]string{"--workspace"}, "", 2},
		{[]string{"--workspace", ""}, "", 2},
		{[]string{"--workspace", "  "}, "", 2},
		{[]string{"--workspace="}, "", 2},
		{[]string{"--workspace", "--other"}, "", 2},
	} {
		var errOut bytes.Buffer
		name, code := workspaceArg(c.args, &errOut)
		if name != c.name || code != c.code {
			t.Errorf("%q: got %q exit %d, want %q exit %d", c.args, name, code, c.name, c.code)
		}
		if code != 0 && !strings.Contains(errOut.String(), "--workspace needs a workspace name") {
			t.Errorf("%q: stderr %q", c.args, errOut.String())
		}
		if code == 0 && errOut.Len() != 0 {
			t.Errorf("%q: unexpected stderr %q", c.args, errOut.String())
		}
	}
}
