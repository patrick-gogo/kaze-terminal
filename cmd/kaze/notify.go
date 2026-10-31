package main

import (
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

const (
	stdinCap       = 64 << 10
	stdinWait      = 300 * time.Millisecond
	notifyTotal    = time.Second // Claude Code waits on its hooks, so never take longer
	reasonMaxRunes = 120
)

// quietNotification are Claude Code notification types that don't need the
// owner: "idle for a minute" and a finished login. Any other type, or none
// (older Claude Code), is an alert.
var quietNotification = map[string]bool{"idle_prompt": true, "auth_success": true}

// runNotify is the quiet compatibility entry used by existing Claude tests.
// The command entry uses runNotifyOutput for Codex Stop JSON. It always
// returns 0 and prints nothing: an alert is a nicety and must never fail or
// slow the hook.
func runNotify(args []string, stdin io.Reader, env func(string) string) int {
	return runNotifyOutput(args, stdin, env, io.Discard)
}

func notifyArgs(args []string) (notify.Kind, string, bool) {
	agent := ""
	if len(args) == 3 && args[1] == "--agent" && (args[2] == "codex" || args[2] == "claude") {
		agent = args[2]
	} else if len(args) != 1 {
		return "", "", false
	}
	kind := notify.Kind(args[0])
	switch kind {
	case notify.KindDone, notify.KindNeeds, notify.KindPrompt:
		return kind, agent, true
	case notify.KindInterrupt:
		return kind, agent, agent == "codex"
	}
	return "", "", false
}

func runNotifyOutput(args []string, stdin io.Reader, env func(string) string, stdout io.Writer) (code int) {
	// Exit 2 from a hook is a blocking error for Claude Code, so a panic must still exit 0.
	defer func() {
		if recover() != nil {
			code = 0
		}
	}()
	kind, agent, ok := notifyArgs(args)
	if !ok {
		return 0
	}
	if kind == notify.KindDone && agent == "codex" {
		defer func() { _, _ = io.WriteString(stdout, "{}\n") }()
	}
	start := time.Now()
	addr, token, tab, ok := kazeTab(env)
	if !ok {
		return 0 // not run from a kaze tab
	}
	msg := notify.Msg{Tab: tab, Kind: kind, Agent: agent}
	if stdin != nil {
		var hook struct {
			Message string `json:"message"`
			Type    string `json:"notification_type"`
			Prompt  string `json:"prompt"`
		}
		if json.Unmarshal(readStdinLimited(stdin, stdinWait), &hook) == nil {
			if kind == notify.KindPrompt {
				msg.Command, msg.Args = agentPromptCommand(hook.Prompt, agent)
				if !notify.ValidAgentCommand(msg.Command, agent) {
					msg.Command = "" // an oversized word would push the message past the server's cap
				}
				msg.Args = notify.CleanArgs(msg.Args)
			}
			if kind == notify.KindNeeds && agent != "codex" && quietNotification[hook.Type] {
				return 0
			}
			msg.Reason = trimRunes(hook.Message, reasonMaxRunes)
		}
	}
	if left := notifyTotal - time.Since(start); left > 0 {
		_ = notify.Send(addr, token, msg, left)
	}
	return 0
}

// promptCommand splits a prompt that starts with a slash command into the
// command and the rest of its first line; any other prompt gives two empties.
func promptCommand(prompt string) (cmd, args string) { return agentPromptCommand(prompt, "claude") }

func agentPromptCommand(prompt, agent string) (cmd, args string) {
	p := strings.TrimLeftFunc(prompt, unicode.IsSpace)
	if len(p) < 2 || (p[0] != '/' && !(agent == "codex" && p[0] == '$')) {
		return "", ""
	}
	line := p
	if i := strings.IndexByte(p, 10); i >= 0 { // first line only
		line = p[:i]
	}
	line = strings.TrimSpace(line)
	if i := strings.IndexFunc(line, unicode.IsSpace); i >= 0 {
		return line[:i], strings.TrimSpace(line[i:])
	}
	return line, ""
}

// kazeTab is where to send from this tab; ok is false outside a kaze tab.
func kazeTab(env func(string) string) (addr, token string, tab int, ok bool) {
	addr, token = env("KAZE_NOTIFY"), env("KAZE_NOTIFY_TOKEN")
	tab, err := strconv.Atoi(env("KAZE_TAB_ID"))
	return addr, token, tab, addr != "" && token != "" && err == nil && tab > 0
}

// readStdinLimited returns what arrived within wait, at most stdinCap bytes. The
// reader goroutine may outlive the call when the pipe never closes; the process
// exits right after, so that is fine.
func readStdinLimited(r io.Reader, wait time.Duration) []byte {
	var (
		mu  sync.Mutex
		buf []byte
	)
	done := make(chan struct{})
	go func() {
		defer close(done)
		chunk := make([]byte, 4096)
		for {
			n, err := r.Read(chunk)
			mu.Lock()
			room := stdinCap - len(buf)
			buf = append(buf, chunk[:min(n, room)]...)
			full := len(buf) >= stdinCap
			mu.Unlock()
			if err != nil || full {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(wait):
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]byte(nil), buf...)
}

func trimRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// stdinForNotify is nil when stdin is a terminal or missing (windowsgui builds
// may have no console), so nothing waits on a keyboard.
func stdinForNotify() io.Reader {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	return os.Stdin
}
