package main

import (
	"bytes"
	"encoding/json"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"io"
	"strings"
	"testing"
	"time"
)

func TestCodexNotifyEvents(t *testing.T) {
	for _, kind := range []string{"needs", "done", "prompt", "interrupt"} {
		t.Run(kind, func(t *testing.T) {
			srv, got := startServer(t)
			var out bytes.Buffer
			code := runNotifyOutput([]string{kind, "--agent", "codex"}, strings.NewReader(`{"prompt":"$review auth"}`), envOf(srv, "2"), &out)
			if code != 0 {
				t.Fatalf("exit %d", code)
			}
			want := ""
			if kind == "done" {
				want = "{}\n"
			}
			if out.String() != want {
				t.Fatalf("stdout %q", out.String())
			}
			select {
			case m := <-got:
				data, _ := json.Marshal(m)
				var wire map[string]any
				json.Unmarshal(data, &wire)
				if wire["agent"] != "codex" || string(m.Kind) != kind {
					t.Fatalf("message %s", data)
				}
				if kind == "prompt" && (m.Command != "$review" || m.Args != "auth") {
					t.Fatalf("prompt %+v", m)
				}
			case <-time.After(time.Second):
				t.Fatal("Codex event was not received")
			}
		})
	}
}

func TestCodexStopOutputOutsideKazeAndOnFailure(t *testing.T) {
	srv, _ := startServer(t)
	dead := envOf(srv, "1")
	srv.Close()
	for _, env := range []func(string) string{func(string) string { return "" }, dead, func(string) string { panic("environment failure") }} {
		var out bytes.Buffer
		if runNotifyOutput([]string{"done", "--agent", "codex"}, strings.NewReader(`{}`), env, &out) != 0 || out.String() != "{}\n" {
			t.Fatalf("stdout %q", out.String())
		}
	}
}

func TestNotifyInvalidAgentArgumentsAreQuiet(t *testing.T) {
	srv, got := startServer(t)
	for _, args := range [][]string{{"done", "--agent", "other"}, {"needs", "--agent"}, {"interrupt"}, {"done", "--agent", "codex", "extra"}} {
		var out bytes.Buffer
		runNotifyOutput(args, strings.NewReader(`{}`), envOf(srv, "1"), &out)
		if out.Len() != 0 {
			t.Fatalf("%v printed %q", args, out.String())
		}
	}
	select {
	case m := <-got:
		t.Fatalf("unexpected %+v", m)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestCodexCommandValidation(t *testing.T) {
	for _, cmd := range []string{"$review", "/prompts:fix", "$plugin:review"} {
		if !notify.ValidAgentCommand(cmd, "codex") {
			t.Errorf("rejected %q", cmd)
		}
	}
	for _, cmd := range []string{"$", "$x;echo", "/x\x1b[31m", strings.Repeat("a", 66)} {
		if notify.ValidAgentCommand(cmd, "codex") {
			t.Errorf("accepted %q", cmd)
		}
	}
	if notify.ValidCommand("$review") {
		t.Fatal("changed Claude slash-only behavior")
	}
}

// Codex helpers (issue #63): payload shapes from the Codex 0.166.0 source,
// codex-rs/hooks/src/schema.rs and core/src/tools/handlers/multi_agents*/spawn.rs.
func TestCodexSubagentEvents(t *testing.T) {
	cases := []struct {
		name, kind, in, out string
		check               func(notify.Msg) bool
	}{
		{"start", "agent-start", `{"session_id":"s1","agent_id":"t1","agent_type":"explorer"}`, "",
			func(m notify.Msg) bool { return m.AgentID == "t1" && m.AgentType == "explorer" && m.Session == "s1" }},
		{"stop", "agent-stop", `{"session_id":"s1","agent_id":"t1","agent_type":"explorer","last_assistant_message":"All good."}`, "{}\n",
			func(m notify.Msg) bool { return m.AgentID == "t1" && m.Report == "All good." }},
		{"helper step", "tool", `{"session_id":"s1","agent_id":"t1","agent_type":"explorer","tool_name":"Bash","tool_input":{"command":"go test ./...\nmore"}}`, "",
			func(m notify.Msg) bool { return m.AgentID == "t1" && m.Tool == "Bash" && m.Summary == "go test ./..." }},
		{"spawn v2", "tool", `{"session_id":"s1","tool_name":"spawn_agent","tool_input":{"task_name":"audit_ui","message":"Check the UI","agent_type":"explorer"}}`, "",
			func(m notify.Msg) bool {
				return m.AgentID == "" && m.Tool == "Agent" && m.Description == "audit_ui" && m.AgentType == "explorer" && m.TaskPrompt == "Check the UI"
			}},
		{"spawn v1", "tool", `{"session_id":"s1","tool_name":"spawn_agent","tool_input":{"message":"Check the UI\nin detail"}}`, "",
			func(m notify.Msg) bool {
				return m.Tool == "Agent" && m.Description == "Check the UI" && m.AgentType == "default" && m.TaskPrompt == "Check the UI\nin detail"
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv, got := startServer(t)
			var out bytes.Buffer
			runNotifyOutput([]string{c.kind, "--agent", "codex"}, strings.NewReader(c.in), envOf(srv, "2"), &out)
			if out.String() != c.out {
				t.Fatalf("stdout %q", out.String())
			}
			m := wait(t, got)
			if m.Agent != "codex" || string(m.Kind) != c.kind || !c.check(m) {
				t.Fatalf("message %+v", m)
			}
		})
	}
}

func TestCodexSubagentStopPrintsJSONOutsideKaze(t *testing.T) {
	var out bytes.Buffer
	runNotifyOutput([]string{"agent-stop", "--agent", "codex"}, strings.NewReader(`{}`), func(string) string { return "" }, &out)
	if out.String() != "{}\n" {
		t.Fatalf("stdout %q", out.String())
	}
}

// A helper's prompt is a new turn for that helper (Codex helpers take several),
// never the owner's prompt: it is sent as that helper starting again.
func TestCodexHelperPromptStartsItsNextTurn(t *testing.T) {
	srv, got := startServer(t)
	runNotifyOutput([]string{"prompt", "--agent", "codex"}, strings.NewReader(`{"session_id":"s1","agent_id":"t1","agent_type":"explorer","prompt":"$review"}`), envOf(srv, "2"), io.Discard)
	m := wait(t, got)
	if m.Kind != notify.KindAgentStart || m.AgentID != "t1" || m.AgentType != "explorer" || m.Command != "" || m.Session != "s1" {
		t.Fatalf("got %+v", m)
	}
}

func TestCodexMainToolCallIsNotSent(t *testing.T) {
	srv, got := startServer(t)
	runNotifyOutput([]string{"tool", "--agent", "codex"}, strings.NewReader(`{"session_id":"s1","tool_name":"Bash","tool_input":{"command":"ls"}}`), envOf(srv, "2"), io.Discard)
	select {
	case m := <-got:
		t.Fatalf("unexpected %+v", m)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestApplyPatchSummaryNamesTheFile(t *testing.T) {
	in := `{"command":"*** Begin Patch\n*** Update File: /repo/internal/ui/sidebar.go\n@@\n-x\n+y\n*** End Patch"}`
	if got := toolSummary("apply_patch", []byte(in)); got != "internal/ui/sidebar.go" {
		t.Fatalf("got %q", got)
	}
}
