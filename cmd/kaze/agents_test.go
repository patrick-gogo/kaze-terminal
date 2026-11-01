package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

func hookJSON(t *testing.T, v any) io.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(b)
}

func TestNotifyForwardsSessionIdentity(t *testing.T) {
	srv, got := startServer(t)
	for _, kind := range []string{"done", "needs", "prompt"} {
		in := hookJSON(t, map[string]any{"session_id": "s1", "transcript_path": "/p/s1.jsonl", "cwd": "/p", "prompt": "/x"})
		runNotify([]string{kind}, in, envOf(srv, "2"))
		m := wait(t, got)
		if m.Session != "s1" || m.Transcript != "/p/s1.jsonl" || m.Cwd != "/p" {
			t.Fatalf("%s: got %+v", kind, m)
		}
	}
	runNotify([]string{"done"}, hookJSON(t, map[string]any{"session_id": strings.Repeat("s", 500)}), envOf(srv, "2"))
	if m := wait(t, got); len(m.Session) != notify.MaxSession {
		t.Fatalf("session len %d", len(m.Session))
	}
}

func TestNotifyMissingSessionStillSends(t *testing.T) {
	srv, got := startServer(t)
	runNotify([]string{"done"}, strings.NewReader(`{"session_id":7}`), envOf(srv, "2"))
	if m := wait(t, got); m.Kind != notify.KindDone || m.Session != "" {
		t.Fatalf("got %+v", m)
	}
}

func TestNotifyToolInSubagent(t *testing.T) {
	srv, got := startServer(t)
	in := hookJSON(t, map[string]any{"session_id": "s1", "agent_id": "a1", "agent_type": "Explore", "tool_name": "Grep",
		"tool_input": map[string]any{"pattern": "auth", "path": "src"}})
	runNotify([]string{"tool"}, in, envOf(srv, "3"))
	m := wait(t, got)
	if m.Kind != notify.KindTool || m.AgentID != "a1" || m.AgentType != "Explore" || m.Tool != "Grep" || m.Summary != "auth in src" || m.Session != "s1" {
		t.Fatalf("got %+v", m)
	}
}

func TestNotifyToolMainAgentCall(t *testing.T) {
	srv, got := startServer(t)
	for _, name := range []string{"Agent", "Task"} {
		in := hookJSON(t, map[string]any{"tool_name": name,
			"tool_input": map[string]any{"description": "Find the auth middleware", "subagent_type": "Explore", "prompt": "long"}})
		runNotify([]string{"tool"}, in, envOf(srv, "3"))
		m := wait(t, got)
		if m.Kind != notify.KindTool || m.AgentID != "" || m.Tool != "Agent" || m.Description != "Find the auth middleware" || m.AgentType != "Explore" {
			t.Fatalf("%s: got %+v", name, m)
		}
	}
}

func TestNotifyToolMainOtherNeverDials(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var dialed atomic.Bool
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			dialed.Store(true)
			c.Close()
		}
	}()
	env := map[string]string{"KAZE_NOTIFY": ln.Addr().String(), "KAZE_NOTIFY_TOKEN": "t", "KAZE_TAB_ID": "1"}
	in := hookJSON(t, map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "ls"}})
	if code := runNotify([]string{"tool"}, in, func(k string) string { return env[k] }); code != 0 {
		t.Fatalf("exit %d", code)
	}
	time.Sleep(100 * time.Millisecond)
	if dialed.Load() {
		t.Fatal("a main-conversation tool call must not connect")
	}
}

func TestNotifyAgentStartStop(t *testing.T) {
	srv, got := startServer(t)
	runNotify([]string{"agent-start"}, hookJSON(t, map[string]any{"agent_id": "a1", "agent_type": "Plan"}), envOf(srv, "1"))
	if m := wait(t, got); m.Kind != notify.KindAgentStart || m.AgentID != "a1" || m.AgentType != "Plan" {
		t.Fatalf("got %+v", m)
	}
	rep := strings.Repeat("あ", 50<<10/3)
	runNotify([]string{"agent-stop"}, hookJSON(t, map[string]any{"agent_id": "a1", "agent_type": "Plan", "last_assistant_message": rep}), envOf(srv, "1"))
	m := wait(t, got)
	if m.Kind != notify.KindAgentStop || len(m.Report) > notify.MaxReport || len(m.Report) < notify.MaxReport-3 || !utf8.ValidString(m.Report) {
		t.Fatalf("kind %q report len %d", m.Kind, len(m.Report))
	}
}

func TestNotifyHugeToolInputBounded(t *testing.T) {
	srv, got := startServer(t)
	in := hookJSON(t, map[string]any{"agent_id": "a1", "tool_name": "Write",
		"tool_input": map[string]any{"file_path": "/a/b/c/d.go", "content": strings.Repeat("x", 8<<20)}})
	var out bytes.Buffer
	start := time.Now()
	if code := runNotifyOutput([]string{"tool"}, in, envOf(srv, "1"), &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if d := time.Since(start); d > notifyTotal+200*time.Millisecond {
		t.Fatalf("took %v", d)
	}
	if out.Len() != 0 {
		t.Fatalf("printed %q", out.String())
	}
	select {
	case m := <-got:
		if m.Kind != notify.KindTool {
			t.Fatalf("got %+v", m)
		}
	case <-time.After(300 * time.Millisecond): // the step may be lost; nothing may hang
	}
}

func TestNotifyNeedsCarriesAgentID(t *testing.T) {
	srv, got := startServer(t)
	in := hookJSON(t, map[string]any{"notification_type": "permission_prompt", "message": "m", "agent_id": "a1", "agent_type": "Explore"})
	runNotify([]string{"needs"}, in, envOf(srv, "1"))
	if m := wait(t, got); m.AgentID != "a1" {
		t.Fatalf("got %+v", m)
	}
}

// Codex helpers use the same kinds as Claude subagents (issue #63).
func TestNotifyAgentKindsForBothAgents(t *testing.T) {
	for _, k := range []string{"tool", "agent-start", "agent-stop"} {
		if _, _, ok := notifyArgs([]string{k, "--agent", "codex"}); !ok {
			t.Fatalf("%s rejected for codex", k)
		}
		if _, _, ok := notifyArgs([]string{k}); !ok {
			t.Fatalf("%s rejected for claude", k)
		}
	}
}

func TestToolSummaryTable(t *testing.T) {
	for _, c := range []struct {
		tool, input, want string
	}{
		{"Bash", `{"command":"git status\ngit diff"}`, "git status"},
		{"Read", `{"file_path":"/Users/x/repo/src/server/auth.ts"}`, "src/server/auth.ts"},
		{"Edit", `{"file_path":"auth.ts"}`, "auth.ts"},
		{"Write", `{"file_path":"/a/b.go"}`, "a/b.go"},
		{"Grep", `{"pattern":"auth"}`, "auth"},
		{"Glob", `{"pattern":"**/*.go","path":"internal"}`, "**/*.go in internal"},
		{"WebFetch", `{"url":"https://code.claude.com/docs/en/hooks?x=1"}`, "code.claude.com/docs/en/hooks"},
		{"WebSearch", `{"query":"gio text"}`, "gio text"},
		{"TodoWrite", `{"todos":[]}`, ""},
		{"Bash", `{"command":"echo \u001b[31mred"}`, "echo [31mred"},
		{"Bash", `not json`, ""},
	} {
		if got := toolSummary(c.tool, json.RawMessage(c.input)); got != c.want {
			t.Errorf("%s %s: got %q want %q", c.tool, c.input, got, c.want)
		}
	}
	long, _ := json.Marshal(map[string]string{"command": strings.Repeat("a", 500)})
	if n := len([]rune(toolSummary("Bash", long))); n != 120 {
		t.Fatalf("%d runes", n)
	}
}

func TestNotifyAgentCallCarriesPrompt(t *testing.T) {
	srv, got := startServer(t)
	in := hookJSON(t, map[string]any{"tool_name": "Agent",
		"tool_input": map[string]any{"description": "Find auth", "subagent_type": "Explore", "prompt": "Find the auth middleware.\n\nReport file:line."}})
	runNotify([]string{"tool"}, in, envOf(srv, "3"))
	if m := wait(t, got); m.TaskPrompt != "Find the auth middleware.\n\nReport file:line." {
		t.Fatalf("prompt %q", m.TaskPrompt)
	}
}

func TestNotifyAgentCallHugePromptFits(t *testing.T) {
	srv, got := startServer(t)
	in := hookJSON(t, map[string]any{"tool_name": "Agent",
		"tool_input": map[string]any{"description": "Big", "prompt": strings.Repeat("<a>\"", 50<<10)}})
	runNotify([]string{"tool"}, in, envOf(srv, "3"))
	m := wait(t, got)
	if m.TaskPrompt == "" || len(m.TaskPrompt) > notify.MaxPrompt || !utf8.ValidString(m.TaskPrompt) {
		t.Fatalf("prompt len %d", len(m.TaskPrompt))
	}
}

func TestNotifyStepInSubagentHasNoPrompt(t *testing.T) {
	srv, got := startServer(t)
	in := hookJSON(t, map[string]any{"agent_id": "a1", "agent_type": "Explore", "tool_name": "Agent",
		"tool_input": map[string]any{"description": "nested", "prompt": "inner"}})
	runNotify([]string{"tool"}, in, envOf(srv, "3"))
	if m := wait(t, got); m.TaskPrompt != "" {
		t.Fatalf("a step carried a prompt: %q", m.TaskPrompt)
	}
}
