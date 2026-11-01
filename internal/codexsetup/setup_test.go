package codexsetup

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func present(string) bool { return true }
func hook(event, cmd string) []byte {
	b, _ := json.Marshal(map[string]any{"hooks": map[string]any{event: []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": cmd}}}}}})
	return b
}

func TestFixPreservesOtherHooksAndIsIdempotent(t *testing.T) {
	before := []byte(`{"notify":["leave","alone"],"custom":1.50,"hooks":{"Stop":[{"hooks":[{"type":"command","command":"other-tool","custom":true}]}],"SessionStart":[{"hooks":[{"type":"command","command":"startup"}]}]}}`)
	r, err := Inspect(before, nil, present)
	if err != nil {
		t.Fatal(err)
	}
	if r.Stop.State != Taken {
		t.Fatalf("stop %+v", r.Stop)
	}
	after, summary, err := Fix(before, nil, `/demo/kaze`, present)
	if err != nil || len(summary) != 8 {
		t.Fatalf("%v %v", summary, err)
	}
	for _, keep := range []string{`"notify"`, `"leave"`, `"other-tool"`, `"custom": true`, `"startup"`, `1.50`} {
		if !bytes.Contains(after, []byte(keep)) {
			t.Fatalf("lost %s: %s", keep, after)
		}
	}
	r, err = Inspect(after, nil, present)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range r.Entries() {
		if e.State != OK {
			t.Fatalf("entry %+v", e)
		}
	}
	again, summary, err := Fix(after, nil, `/demo/kaze`, present)
	if err != nil || len(summary) != 0 || !bytes.Equal(after, again) {
		t.Fatal("not idempotent")
	}
}

func TestInlineHooksPreventDuplicateInstallation(t *testing.T) {
	config := []byte("notify = ['other-tool']\n[[hooks.Stop]]\n[[hooks.Stop.hooks]]\ntype = 'command'\ncommand = '\"C:\\Program Files\\Kaze\\kaze.exe\" notify done --agent codex'\n")
	r, err := Inspect(nil, config, present)
	if err != nil || r.Stop.State != OK || r.Stop.Source != "config.toml" {
		t.Fatalf("%+v %v", r.Stop, err)
	}
	after, _, err := Fix(nil, config, "/demo/kaze", present)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), `"Stop"`) {
		t.Fatalf("added duplicate %s", after)
	}
	r, err = Inspect(nil, config, func(string) bool { return false })
	if err != nil || r.Stop.Fixable || r.Stop.State != Broken {
		t.Fatalf("broken inline %+v %v", r.Stop, err)
	}
	after, _, err = Fix(nil, config, "/demo/kaze", func(string) bool { return false })
	if err != nil || strings.Contains(string(after), `"Stop"`) {
		t.Fatalf("duplicated broken inline: %s %v", after, err)
	}
}

func TestFixReplacesRestrictedAndBrokenOwnHandlerOnly(t *testing.T) {
	before := []byte(`{"hooks":{"PermissionRequest":[{"matcher":"Bash","hooks":[{"type":"command","command":"other"},{"type":"command","command":"\"/gone/kaze\" notify needs --agent codex","async":true}]}]}}`)
	after, _, err := Fix(before, nil, "/demo/kaze", func(p string) bool { return p == "/demo/kaze" })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "/gone/kaze") || !strings.Contains(string(after), `"matcher": "Bash"`) || !strings.Contains(string(after), `"other"`) {
		t.Fatalf("bad repair %s", after)
	}
	r, err := Inspect(after, nil, present)
	if err != nil || r.Needs.State != OK {
		t.Fatalf("%+v %v", r.Needs, err)
	}
}

func TestMalformedSourcesNeverRewritten(t *testing.T) {
	for _, b := range []string{"", `[]`, `{"hooks":[]}`, `{"hooks":{"Stop":{}}}`, `{"hooks":{"Stop":[{"hooks":{}}]}}`} {
		if _, _, err := Fix([]byte(b), nil, "/demo/kaze", present); err == nil {
			t.Errorf("accepted %q", b)
		}
	}
	if _, _, err := Fix(nil, []byte("[hooks\n"), "/demo/kaze", present); err == nil {
		t.Fatal("ignored malformed inline source")
	}
}

func TestDuplicateSourcesRequireManualRepair(t *testing.T) {
	config := []byte("[[hooks.Stop]]\n[[hooks.Stop.hooks]]\ntype='command'\ncommand='kaze notify done --agent codex'\n")
	r, err := Inspect(hook("Stop", "kaze notify done --agent codex"), config, present)
	if err != nil || r.Stop.State != Broken || r.Stop.Fixable {
		t.Fatalf("duplicate %+v %v", r.Stop, err)
	}
}

func TestBrokenHandlerKeepsItsMetadata(t *testing.T) {
	before := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"\"/gone/kaze\" notify done --agent codex","statusMessage":"Reporting completion","custom":1.50}]}]}}`)
	out, _, err := Fix(before, nil, "/demo/kaze", func(p string) bool { return p == "/demo/kaze" })
	if err != nil || !bytes.Contains(out, []byte(`"statusMessage": "Reporting completion"`)) || !bytes.Contains(out, []byte(`1.50`)) {
		t.Fatalf("lost metadata: %s %v", out, err)
	}
}

func TestCompoundShellCommandsAreNotReplaced(t *testing.T) {
	before := hook("Stop", `other-tool && kaze notify done --agent codex`)
	r, err := Inspect(before, nil, present)
	if err != nil || r.Stop.State != Taken {
		t.Fatalf("misidentified shell command %+v %v", r.Stop, err)
	}
	out, _, err := Fix(before, nil, "/demo/kaze", present)
	if err != nil || !bytes.Contains(out, []byte(`other-tool && kaze notify done --agent codex`)) {
		t.Fatalf("replaced another program: %s %v", out, err)
	}
}
func TestWrongKazeEventCommandIsRepaired(t *testing.T) {
	before := hook("Stop", `"/demo/kaze" notify needs --agent codex`)
	r, err := Inspect(before, nil, present)
	if err != nil || r.Stop.State != Broken {
		t.Fatalf("wrong hook %+v %v", r.Stop, err)
	}
	out, _, err := Fix(before, nil, "/demo/kaze", present)
	var root map[string]any
	json.Unmarshal(out, &root)
	stop := root["hooks"].(map[string]any)["Stop"]
	b, _ := json.Marshal(stop)
	if err != nil || bytes.Contains(b, []byte("notify needs")) {
		t.Fatalf("kept wrong command %s %v", b, err)
	}
}

func TestAmbiguousOrMalformedHookDefinitionsAreNotFixed(t *testing.T) {
	for _, data := range []string{
		`{"hooks":{},"hooks":{}}`,
		`{"hooks":{"Stop":[{"matcher":false,"hooks":[{"type":"command","command":"kaze notify done --agent codex"}]}]}}`,
		`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":false}]}]}}`,
	} {
		if _, _, err := Fix([]byte(data), nil, "/demo/kaze", present); err == nil {
			t.Errorf("rewrote malformed source %s", data)
		}
	}
}

// Issue #63: helpers need SubagentStart, SubagentStop and PreToolUse too.
func TestFixAddsHelperHooks(t *testing.T) {
	after, _, err := Fix(nil, nil, "/demo/kaze", present)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"SubagentStart"`, `notify agent-start --agent codex`, `"SubagentStop"`, `notify agent-stop --agent codex`, `"PreToolUse"`, `notify tool --agent codex`} {
		if !strings.Contains(string(after), want) {
			t.Fatalf("missing %s in %s", want, after)
		}
	}
	r, err := Inspect(after, nil, present)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Entries()) != 8 || r.AgentStart.State != OK || r.AgentStop.State != OK || r.Tools.State != OK {
		t.Fatalf("report %+v", r)
	}
	// A tool hook limited to some tools would miss helper steps.
	limited := []byte(`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"\"/demo/kaze\" notify tool --agent codex"}]}]}}`)
	if r, _ := Inspect(limited, nil, present); r.Tools.State != Broken {
		t.Fatalf("limited tool hook %+v", r.Tools)
	}
}

func TestFixAddsSessionHook(t *testing.T) {
	after, _, err := Fix(nil, nil, "/demo/kaze", present)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), `"SessionStart"`) || !strings.Contains(string(after), `notify session --agent codex`) {
		t.Fatalf("no session hook in %s", after)
	}
	r, err := Inspect(after, nil, present)
	if err != nil || len(r.Entries()) != 8 || r.Session.State != OK {
		t.Fatalf("report %+v %v", r, err)
	}
}
