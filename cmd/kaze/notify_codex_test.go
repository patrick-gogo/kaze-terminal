package main

import (
	"bytes"
	"encoding/json"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
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
