package claudesetup

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	kazeExe = `C:\Users\john\Desktop\kaze-app\kaze.exe`
	nodeCmd = `"C:\Program Files\nodejs\node.exe" "C:\Users\john\.claude\agent-flow\hook.js"`
	newExe  = `C:\Program Files\kaze\kaze.exe`
)

// ownerSettings is the shape of the owner's real file: agent-flow node hooks on
// every event, kaze's Stop and Notification hooks, kaze's status line and other keys.
func ownerSettings(stop, needs, status string) string {
	return ownerWithPrompt(stop, needs, status, false)
}

// ownerWithPrompt is ownerSettings with kaze's prompt hook left out when noPrompt is set.
func ownerWithPrompt(stop, needs, status string, noPrompt bool) string {
	node := `{"type": "command", "command": ` + jsonString(nodeCmd) + `}`
	group := func(cmds ...string) string {
		return `{"hooks": [` + strings.Join(cmds, ", ") + `]}`
	}
	stopGroups := group(node)
	if stop != "" {
		stopGroups += `, {"hooks": [{"type": "command", "command": ` + jsonString(stop) + `, "timeout": 3}]}`
	}
	notif := group(node)
	if needs != "" {
		notif += `, {"matcher": "permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input", "hooks": [{"type": "command", "command": ` + jsonString(needs) + `, "timeout": 3}]}`
	}
	promptGroups := group(node)
	if !noPrompt {
		promptGroups += `, {"hooks": [{"type": "command", "command": ` + jsonString(promptCmd) + `, "timeout": 3}]}`
	}
	statusLine := ""
	if status != "" {
		statusLine = `, "statusLine": {"type": "command", "command": ` + jsonString(status) + `}`
	}
	return `{
  "permissions": {"allow": ["Bash(git status:*)", "Read(//c/Users/**)"], "deny": [], "defaultMode": "auto"},
  "env": {"MAX_THINKING_TOKENS": "31999", "A&B": "<x>"},
  "enabledPlugins": {"superpowers@claude-plugins-official": true, "devbox@devbox": true},
  "hooks": {
    "SessionStart": [` + group(node) + `],
    "SessionEnd": [` + group(node) + `],
    "Stop": [` + stopGroups + `],
    "Notification": [` + notif + `],
    "UserPromptSubmit": [` + promptGroups + `],
    "PreToolUse": [{"matcher": "*", "hooks": [` + node + `]}],
    "PostToolUse": [{"matcher": "*", "hooks": [` + node + `]}]
  },
  "cleanupPeriodDays": 99999,
  "alwaysThinkingEnabled": true` + statusLine + `
}`
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func q(p string) string { return `"` + p + `"` }

var (
	stopCmd   = q(kazeExe) + " notify done"
	needsCmd  = q(kazeExe) + " notify needs"
	statusCmd = q(kazeExe) + " statusline"
	promptCmd = q(kazeExe) + " notify prompt"
)

func onlyKaze(p string) bool { return p == kazeExe || p == newExe }

func decode(t *testing.T, b []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, b)
	}
	return v
}

func hooksOf(t *testing.T, b []byte, event string) []any {
	t.Helper()
	m := decode(t, b).(map[string]any)
	return m["hooks"].(map[string]any)[event].([]any)
}

func TestOwnerShapeIsOK(t *testing.T) {
	r := Inspect([]byte(ownerSettings(stopCmd, needsCmd, statusCmd)), onlyKaze)
	for name, e := range map[string]Entry{"stop": r.Stop, "needs": r.Needs, "status": r.StatusLine} {
		if e.State != OK || e.Exe != kazeExe {
			t.Errorf("%s: %+v, want OK with %s", name, e, kazeExe)
		}
	}
	if r.Stop.Command != stopCmd || r.Needs.Command != needsCmd || r.StatusLine.Command != statusCmd {
		t.Errorf("commands %q %q %q", r.Stop.Command, r.Needs.Command, r.StatusLine.Command)
	}
}

func TestCommandForms(t *testing.T) {
	cases := []struct {
		cmd, exe string
		ok       bool
	}{
		{`"C:\Program Files\kaze\kaze.exe" notify done`, `C:\Program Files\kaze\kaze.exe`, true},
		{`C:\Program Files\kaze\kaze.exe notify done`, `C:\Program Files\kaze\kaze.exe`, true},
		{`C:/Users/john/Desktop/kaze-app/kaze.exe   notify   done`, `C:/Users/john/Desktop/kaze-app/kaze.exe`, true},
		{`/usr/local/bin/kaze notify done`, `/usr/local/bin/kaze`, true},
		{`"C:\x\KAZE.EXE" notify done`, `C:\x\KAZE.EXE`, true},
		{`C:\x\notkaze.exe notify done`, "", false},
		{`C:\x\kaze.exe notify needs`, "", false},
		{`C:\x\kaze.exe notify done --loud`, "", false},
	}
	for _, c := range cases {
		exe, ok := kazeCommand(c.cmd, "notify done")
		if ok != c.ok || exe != c.exe {
			t.Errorf("%q: %q %v, want %q %v", c.cmd, exe, ok, c.exe, c.ok)
		}
	}
}

func TestMissingStopFixAddsExactlyOne(t *testing.T) {
	in := []byte(ownerSettings("", needsCmd, statusCmd))
	r := Inspect(in, onlyKaze)
	if r.Stop.State != Missing || r.Needs.State != OK || r.StatusLine.State != OK {
		t.Fatalf("report %+v", r)
	}
	out, summary, err := Fix(in, r, newExe)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 1 || !strings.Contains(summary[0], "Stop") {
		t.Fatalf("summary %q", summary)
	}
	before, after := hooksOf(t, in, "Stop"), hooksOf(t, out, "Stop")
	if len(after) != len(before)+1 || !reflect.DeepEqual(after[:len(before)], before) {
		t.Fatalf("Stop groups before %v after %v", before, after)
	}
	want := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": q(newExe) + " notify done", "timeout": 3.0}}}
	if !reflect.DeepEqual(after[len(before)], want) {
		t.Fatalf("added %v, want %v", after[len(before)], want)
	}
	// Everything else is equal after re-marshal.
	bm, am := decode(t, in).(map[string]any), decode(t, out).(map[string]any)
	bm["hooks"].(map[string]any)["Stop"] = nil
	am["hooks"].(map[string]any)["Stop"] = nil
	if !reflect.DeepEqual(bm, am) {
		t.Fatal("Fix changed something besides the Stop hook")
	}
	if r2 := Inspect(out, onlyKaze); r2.Stop.State != OK || r2.Stop.Exe != newExe {
		t.Fatalf("after Fix: %+v", r2.Stop)
	}
	if !strings.Contains(string(out), "\n  \"hooks\": {\n") || !strings.Contains(string(out), `"A&B": "<x>"`) {
		t.Fatalf("not 2-space indented or HTML-escaped:\n%s", out)
	}
}

func TestMissingNeedsGetsTheMatcher(t *testing.T) {
	in := []byte(ownerSettings(stopCmd, "", statusCmd))
	r := Inspect(in, onlyKaze)
	if r.Needs.State != Missing {
		t.Fatalf("needs %+v", r.Needs)
	}
	out, _, err := Fix(in, r, newExe)
	if err != nil {
		t.Fatal(err)
	}
	groups := hooksOf(t, out, "Notification")
	last := groups[len(groups)-1].(map[string]any)
	if last["matcher"] != "permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input" {
		t.Fatalf("matcher %v", last["matcher"])
	}
	h := last["hooks"].([]any)[0].(map[string]any)
	if h["command"] != q(newExe)+" notify needs" || h["timeout"] != 3.0 {
		t.Fatalf("hook %v", h)
	}
}

func TestBrokenEntryIsRepointed(t *testing.T) {
	gone := `C:\old\kaze.exe`
	in := []byte(ownerSettings(q(gone)+" notify done", needsCmd, q(gone)+" statusline"))
	r := Inspect(in, onlyKaze)
	if r.Stop.State != Broken || r.Stop.Exe != gone || r.StatusLine.State != Broken {
		t.Fatalf("report %+v", r)
	}
	out, summary, err := Fix(in, r, newExe)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 2 {
		t.Fatalf("summary %q", summary)
	}
	stop := hooksOf(t, out, "Stop")
	if len(stop) != 2 {
		t.Fatalf("Stop has %d groups, want 2 (repointed, not added)", len(stop))
	}
	h := stop[1].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if h["command"] != q(newExe)+" notify done" || h["timeout"] != 3.0 {
		t.Fatalf("repointed hook %v", h)
	}
	r2 := Inspect(out, onlyKaze)
	if r2.Stop.State != OK || r2.StatusLine.State != OK || r2.Needs.Exe != kazeExe {
		t.Fatalf("after Fix %+v", r2)
	}
}

func TestDifferentExistingExeIsOK(t *testing.T) {
	in := []byte(ownerSettings(q(newExe)+" notify done", needsCmd, statusCmd))
	r := Inspect(in, onlyKaze)
	if r.Stop.State != OK || r.Stop.Exe != newExe {
		t.Fatalf("stop %+v", r.Stop)
	}
	out, summary, err := Fix(in, r, `C:\elsewhere\kaze.exe`)
	if err != nil || len(summary) != 0 || string(out) != string(in) {
		t.Fatalf("Fix with nothing to do: summary %q err %v changed %v", summary, err, string(out) != string(in))
	}
}

func TestTakenStatusLine(t *testing.T) {
	in := []byte(ownerSettings(stopCmd, needsCmd, "bash x.sh"))
	r := Inspect(in, onlyKaze)
	if r.StatusLine.State != Taken || r.StatusLine.Command != "bash x.sh" {
		t.Fatalf("status %+v", r.StatusLine)
	}
	out, summary, err := Fix(in, r, newExe)
	if err != nil || len(summary) != 0 || string(out) != string(in) {
		t.Fatalf("Fix touched a taken status line: %q %v", summary, err)
	}
	out, removed := Replace(in, newExe)
	if removed != "bash x.sh" {
		t.Fatalf("removed %q", removed)
	}
	r2 := Inspect(out, onlyKaze)
	if r2.StatusLine.State != OK || r2.StatusLine.Command != q(newExe)+" statusline" {
		t.Fatalf("after Replace %+v", r2.StatusLine)
	}
	m := decode(t, out).(map[string]any)
	if m["statusLine"].(map[string]any)["type"] != "command" {
		t.Fatal("status line lost its type")
	}
}

func TestMissingStatusLineIsAdded(t *testing.T) {
	in := []byte(ownerSettings(stopCmd, needsCmd, ""))
	r := Inspect(in, onlyKaze)
	if r.StatusLine.State != Missing {
		t.Fatalf("status %+v", r.StatusLine)
	}
	out, summary, err := Fix(in, r, newExe)
	if err != nil || len(summary) != 1 {
		t.Fatalf("summary %q err %v", summary, err)
	}
	m := decode(t, out).(map[string]any)
	want := map[string]any{"type": "command", "command": q(newExe) + " statusline"}
	if !reflect.DeepEqual(m["statusLine"], want) {
		t.Fatalf("status line %v", m["statusLine"])
	}
}

func TestEmptyObjectGetsEverything(t *testing.T) {
	r := Inspect([]byte("{}"), onlyKaze)
	if r.Stop.State != Missing || r.Needs.State != Missing || r.Prompt.State != Missing || r.StatusLine.State != Missing {
		t.Fatalf("report %+v", r)
	}
	out, summary, err := Fix([]byte("{}"), r, newExe)
	if err != nil || len(summary) != 4 {
		t.Fatalf("summary %q err %v", summary, err)
	}
	if r2 := Inspect(out, onlyKaze); r2.Stop.State != OK || r2.Needs.State != OK || r2.StatusLine.State != OK {
		t.Fatalf("after Fix %+v", r2)
	}
}

func TestUnreadable(t *testing.T) {
	for _, in := range []string{"", "   ", "{not json", `["a"]`, `{"hooks": []}`, `{"a": 1} trailing`} {
		r := Inspect([]byte(in), onlyKaze)
		if r.Stop.State != Unreadable || r.Needs.State != Unreadable || r.StatusLine.State != Unreadable {
			t.Errorf("%q: %+v", in, r)
			continue
		}
		if _, _, err := Fix([]byte(in), r, newExe); err == nil {
			t.Errorf("%q: Fix gave no error", in)
		}
		if out, removed := Replace([]byte(in), newExe); out != nil || removed != "" {
			t.Errorf("%q: Replace wrote %q", in, out)
		}
	}
}

func TestEncodeKeepsOrderAndNumbers(t *testing.T) {
	in := "{\n  \"z\": 1.50,\n  \"a\": [],\n  \"m\": {},\n  \"k\": [\n    true,\n    null,\n    \"\\u00e9\\n\"\n  ]\n}\n"
	n, err := parse([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"z\": 1.50,\n  \"a\": [],\n  \"m\": {},\n  \"k\": [\n    true,\n    null,\n    \"é\\n\"\n  ]\n}\n"
	if got := string(n.Encode()); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestDiffShowsOnlyChangedLines(t *testing.T) {
	in := []byte(ownerSettings("", needsCmd, statusCmd))
	out, _, err := Fix(in, Inspect(in, onlyKaze), newExe)
	if err != nil {
		t.Fatal(err)
	}
	lines := Diff(in, out, 40)
	plus, minus := 0, 0
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "+ "):
			plus++
		case strings.HasPrefix(l, "- "):
			minus++
		}
	}
	// Only the comma after the old last Stop group goes away; the rest is added.
	if minus > 1 || plus < 5 || !strings.Contains(strings.Join(lines, "\n"), "notify done") {
		t.Fatalf("diff %q", lines)
	}
	if short := Diff(in, out, 3); len(short) != 3 || short[2] != "…" {
		t.Fatalf("capped diff %q", short)
	}
}

func TestWriteBacksUpThenWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 29, 14, 5, 9, 0, time.Local)
	if err := Write(path, []byte("new"), now); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new" {
		t.Fatalf("file %q", b)
	}
	if b, err := os.ReadFile(path + ".bak-20261029-140509"); err != nil || string(b) != "old" {
		t.Fatalf("backup %q %v", b, err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 2 {
		t.Fatalf("left %d files, want 2 (no temp file)", len(ents))
	}
}

func TestWriteWithoutOldFileMakesNoBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := Write(path, []byte("{}\n"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 1 {
		t.Fatalf("%d files", len(ents))
	}
}

func TestWriteKeepsEveryBackupInOneSecond(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	now := time.Date(2026, 10, 29, 14, 5, 9, 0, time.Local)
	for _, s := range []string{"one", "two", "three", "four"} {
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Write(path, []byte("new"), now); err != nil {
			t.Fatal(err)
		}
	}
	base := path + ".bak-20261029-140509"
	for suffix, want := range map[string]string{"": "one", "-2": "two", "-3": "three", "-4": "four"} {
		if b, err := os.ReadFile(base + suffix); err != nil || string(b) != want {
			t.Errorf("backup %q: %q %v, want %q", suffix, b, err, want)
		}
	}
}

func TestRelayout(t *testing.T) {
	claudeStyle := "{\n  \"a\": [\n    1\n  ],\n  \"b\": {}\n}\n"
	if Relayout([]byte(claudeStyle)) || Relayout([]byte(strings.ReplaceAll(claudeStyle, "\n", "\r\n"))) {
		t.Fatal("Claude Code's own layout flagged as rewritten")
	}
	if !Relayout([]byte("{\n    \"a\": [1],\n    \"b\": {}\n}\n")) {
		t.Fatal("a 4-space file not flagged")
	}
}

func TestWriteFollowsASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not permitted here: %v", err)
	}
	if err := Write(link, []byte("new"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced by a plain file: %v %v", fi, err)
	}
	if b, _ := os.ReadFile(target); string(b) != "new" {
		t.Fatalf("target %q", b)
	}
}

func TestInspectFindsThePromptHook(t *testing.T) {
	r := Inspect([]byte(ownerSettings(stopCmd, needsCmd, statusCmd)), onlyKaze)
	if r.Prompt.State != OK || r.Prompt.Exe != kazeExe || r.Prompt.Command != promptCmd {
		t.Fatalf("prompt %+v, want OK", r.Prompt)
	}
	r = Inspect([]byte(ownerWithPrompt(stopCmd, needsCmd, statusCmd, true)), onlyKaze)
	if r.Prompt.State != Missing {
		t.Fatalf("prompt %+v, want Missing", r.Prompt)
	}
	r = Inspect([]byte(ownerSettings(stopCmd, needsCmd, statusCmd)), func(string) bool { return false })
	if r.Prompt.State != Broken {
		t.Fatalf("prompt %+v, want Broken", r.Prompt)
	}
	if r := Inspect([]byte("nope"), onlyKaze); r.Prompt.State != Unreadable {
		t.Fatalf("prompt %+v, want Unreadable", r.Prompt)
	}
	bad := `{"hooks": {"UserPromptSubmit": {}}}`
	if r := Inspect([]byte(bad), onlyKaze); r.Prompt.State != Unreadable {
		t.Fatalf("a non-list UserPromptSubmit: %+v", r.Prompt)
	}
}

func TestFixAddsThePromptHook(t *testing.T) {
	in := []byte(ownerWithPrompt(stopCmd, needsCmd, statusCmd, true))
	r := Inspect(in, onlyKaze)
	out, summary, err := Fix(in, r, newExe)
	if err != nil {
		t.Fatal(err)
	}
	if len(summary) != 1 || !strings.Contains(summary[0], "the prompt hook (shows which command each session runs)") {
		t.Fatalf("summary %q", summary)
	}
	groups := hooksOf(t, out, "UserPromptSubmit")
	before := hooksOf(t, in, "UserPromptSubmit")
	if len(groups) != len(before)+1 || !reflect.DeepEqual(groups[:len(before)], before) {
		t.Fatalf("groups before %v after %v", before, groups)
	}
	last := groups[len(groups)-1].(map[string]any)
	if _, has := last["matcher"]; has {
		t.Fatalf("prompt hook has a matcher: %v", last)
	}
	h := last["hooks"].([]any)[0].(map[string]any)
	if h["command"] != q(newExe)+" notify prompt" || h["timeout"] != 3.0 {
		t.Fatalf("hook %v", h)
	}
	bm, am := decode(t, in).(map[string]any), decode(t, out).(map[string]any)
	bm["hooks"].(map[string]any)["UserPromptSubmit"] = nil
	am["hooks"].(map[string]any)["UserPromptSubmit"] = nil
	if !reflect.DeepEqual(bm, am) {
		t.Fatal("Fix changed something besides the prompt hook")
	}
}

func TestBrokenPromptHookIsRepointed(t *testing.T) {
	in := []byte(ownerSettings(stopCmd, needsCmd, statusCmd))
	r := Inspect(in, func(p string) bool { return false })
	if r.Prompt.State != Broken {
		t.Fatalf("prompt %+v", r.Prompt)
	}
	out, summary, err := Fix(in, r, newExe)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(summary, "|"), "Repoint the prompt hook") {
		t.Fatalf("summary %q", summary)
	}
	if r2 := Inspect(out, func(p string) bool { return p == newExe }); r2.Prompt.State != OK || r2.Prompt.Exe != newExe {
		t.Fatalf("after Fix: %+v", r2.Prompt)
	}
}
