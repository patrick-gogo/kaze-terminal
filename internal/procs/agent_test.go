package procs

import "testing"

func TestAgentFromArgs(t *testing.T) {
	for _, c := range []struct {
		exe  string
		args []string
		want string
	}{
		{"/Users/a/.local/share/claude/versions/2.1.0", []string{"claude"}, "claude"},
		{"/opt/homebrew/bin/claude", []string{"/opt/homebrew/bin/claude", "--resume"}, "claude"},
		{"/usr/local/bin/node", []string{"node", "/Users/a/.nvm/versions/node/v22/bin/codex"}, "codex"},
		{"/x/node_modules/@openai/codex/vendor/aarch64-apple-darwin/bin/codex", []string{"/x/bin/codex", "--no-daemon"}, "codex"},
		{"/usr/local/bin/node", []string{"node", "/x/node_modules/@anthropic-ai/claude-code/cli.js"}, "claude"},
		{`C:\Users\a\AppData\Roaming\npm\codex.exe`, []string{"codex.exe"}, "codex"},
		{"/bin/zsh", []string{"-zsh"}, ""},
		{"/usr/bin/vim", []string{"vim", "codex.md"}, ""},
		{"/usr/local/bin/node", []string{"node", "server.js"}, ""},
		{"", nil, ""},
	} {
		if got := agentFromArgs(c.exe, c.args); got != c.want {
			t.Errorf("%q %q: got %q, want %q", c.exe, c.args, got, c.want)
		}
	}
}
