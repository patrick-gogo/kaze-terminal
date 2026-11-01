// Package procs names the coding agent a process runs, from its executable
// path and arguments. It is how kaze shows Claude or Codex on a session's row
// without any hook.
package procs

import (
	"path/filepath"
	"strings"
)

// runtimes start an agent's script: the agent is named by the script, argv[1].
var runtimes = map[string]bool{"node": true, "bun": true, "deno": true}

// Agent is "claude" or "codex" when process pid runs one, else "". An exited or
// unreadable process is "".
func Agent(pid int) string {
	if pid <= 0 {
		return ""
	}
	exe, args := processArgs(pid)
	return agentFromArgs(exe, args)
}

// agentFromArgs names the agent from the executable and argv: the program's
// own name, or for a script runtime, the script's.
func agentFromArgs(exe string, args []string) string {
	var names []string
	if len(args) > 0 {
		names = append(names, args[0])
	}
	names = append(names, exe)
	if len(args) > 1 && runtimes[base(firstOf(args[0], exe))] {
		names = append(names, args[1])
		// npm installs run the package's own script: .../@anthropic-ai/claude-code/cli.js.
		switch p := filepath.ToSlash(args[1]); {
		case strings.Contains(p, "/@anthropic-ai/claude-code/"):
			return "claude"
		case strings.Contains(p, "/@openai/codex/"):
			return "codex"
		}
	}
	for _, n := range names {
		switch base(n) {
		case "claude":
			return "claude"
		case "codex":
			return "codex"
		}
	}
	return ""
}

// base is a path's last element, without a Windows .exe and a login shell's "-".
func base(p string) string {
	p = p[strings.LastIndexAny(p, `/\`)+1:]
	p = strings.TrimPrefix(p, "-")
	return strings.TrimSuffix(strings.ToLower(p), ".exe")
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
