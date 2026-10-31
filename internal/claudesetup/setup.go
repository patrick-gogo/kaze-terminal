// Package claudesetup checks and repairs kaze's entries in Claude Code's
// settings.json: the Stop, Notification and UserPromptSubmit hooks that run `kaze notify` and
// the status line that runs `kaze statusline`. It never touches other entries.
package claudesetup

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// State is what one kaze entry looks like in the settings.
type State int

const (
	OK         State = iota // a kaze entry whose exe exists
	Missing                 // no kaze entry
	Broken                  // a kaze entry whose exe is gone
	Taken                   // status line only: another program's command is there
	Unreadable              // the file is empty, not JSON, or not shaped like settings
)

// Entry is one checked entry: the exe a kaze entry runs and its whole command
// (for Taken, the other program's command).
type Entry struct {
	State        State
	Exe, Command string
}

// Report is the check of all seven entries.
type Report struct {
	Stop, Needs, Prompt, StatusLine Entry
	AgentStart, AgentStop, Tools    Entry // the subagent hooks
}

// NeedsMatcher is the Notification matcher for the alerts that mean "Claude needs you".
const NeedsMatcher = "permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input"

const (
	argsDone   = "notify done"
	argsNeeds  = "notify needs"
	argsPrompt = "notify prompt"
	argsStatus = "statusline"

	argsAgentStart = "notify agent-start"
	argsAgentStop  = "notify agent-stop"
	argsTool       = "notify tool"
)

// Inspect checks the settings; exists reports whether an exe path is a file.
func Inspect(settings []byte, exists func(string) bool) Report {
	root, err := load(settings)
	if err != nil {
		u := Entry{State: Unreadable}
		return Report{Stop: u, Needs: u, Prompt: u, StatusLine: u, AgentStart: u, AgentStop: u, Tools: u}
	}
	hooks := root.Get("hooks")
	r := Report{
		Stop:   findHook(hooks.Get("Stop"), argsDone, exists),
		Needs:  findHook(hooks.Get("Notification"), argsNeeds, exists),
		Prompt: findHook(hooks.Get("UserPromptSubmit"), argsPrompt, exists),

		AgentStart: findHook(hooks.Get("SubagentStart"), argsAgentStart, exists),
		AgentStop:  findHook(hooks.Get("SubagentStop"), argsAgentStop, exists),
		Tools:      findHook(hooks.Get("PreToolUse"), argsTool, exists),
	}
	r.StatusLine = Entry{State: Missing}
	if s := root.Get("statusLine"); s != nil {
		// Anything there that is not kaze's is someone else's, even a blank command.
		cmd, _ := s.Get("command").Str()
		r.StatusLine = entryFor(cmd, argsStatus, exists)
		if r.StatusLine.State == Missing {
			r.StatusLine = Entry{State: Taken, Command: cmd}
		}
	}
	return r
}

// load parses settings and checks the parts kaze reads have the right shape.
func load(settings []byte) (*node, error) {
	root, err := parse(settings)
	if err != nil {
		return nil, err
	}
	if root.Kind != '{' {
		return nil, errors.New("settings are not a JSON object")
	}
	if h := root.Get("hooks"); h != nil {
		if h.Kind != '{' {
			return nil, errors.New(`"hooks" is not an object`)
		}
		for _, ev := range []string{"Stop", "Notification", "UserPromptSubmit"} {
			if g := h.Get(ev); g != nil && g.Kind != '[' {
				return nil, fmt.Errorf("hooks %q is not a list", ev)
			}
		}
	}
	if s := root.Get("statusLine"); s != nil && s.Kind != '{' {
		return nil, errors.New(`"statusLine" is not an object`)
	}
	return root, nil
}

// commands calls fn for every hook command node under an event's groups.
func commands(groups *node, fn func(cmd *node)) {
	if groups == nil {
		return
	}
	for _, g := range groups.Vals {
		for _, h := range g.Get("hooks").ListVals() {
			if c := h.Get("command"); c != nil {
				fn(c)
			}
		}
	}
}

// findHook is the event's kaze entry with args: an OK one wins over a broken one.
func findHook(groups *node, args string, exists func(string) bool) Entry {
	best := Entry{State: Missing}
	commands(groups, func(c *node) {
		cmd, ok := c.Str()
		if !ok {
			return
		}
		if e := entryFor(cmd, args, exists); e.State == OK && best.State != OK || e.State == Broken && best.State == Missing {
			best = e
		}
	})
	return best
}

func entryFor(cmd, args string, exists func(string) bool) Entry {
	exe, ok := kazeCommand(cmd, args)
	if !ok {
		return Entry{State: Missing}
	}
	st := Broken
	if exists(exe) {
		st = OK
	}
	return Entry{State: st, Exe: exe, Command: cmd}
}

// kazeCommand reports whether cmd runs kaze with exactly args, and the exe it
// runs. The exe may be quoted or not (an unquoted one may hold spaces) and use
// either slash.
func kazeCommand(cmd, args string) (string, bool) {
	cmd = strings.TrimSpace(cmd)
	if rest, ok := strings.CutPrefix(cmd, `"`); ok {
		exe, tail, ok := strings.Cut(rest, `"`)
		if !ok || !isKaze(exe) || strings.Join(strings.Fields(tail), " ") != args {
			return "", false
		}
		return exe, true
	}
	f := strings.Fields(cmd)
	for i := range f {
		if isKaze(f[i]) && strings.Join(f[i+1:], " ") == args {
			return strings.Join(f[:i+1], " "), true
		}
	}
	return "", false
}

func isKaze(exe string) bool {
	base := exe[strings.LastIndexAny(exe, `/\`)+1:]
	return strings.EqualFold(base, "kaze.exe") || strings.EqualFold(base, "kaze")
}

// command is the line Claude Code runs: the exe quoted, so a path with spaces works.
func command(exe, args string) string { return `"` + exe + `" ` + args }

func hookNode(exe, args string) *node {
	h := object()
	h.Set("type", scalar("command"))
	h.Set("command", scalar(command(exe, args)))
	h.Set("timeout", &node{Raw: "3"})
	return h
}

// Fix adds missing kaze hooks and a missing status line and repoints broken
// kaze entries to exe. It leaves OK and Taken entries and everything else
// alone. summary says what changed; with nothing to do it returns settings as they are.
func Fix(settings []byte, r Report, exe string) (out []byte, summary []string, err error) {
	root, err := load(settings)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot read the settings: %w", err)
	}
	hooks := root.Get("hooks")
	group := func(event string) *node {
		if hooks == nil {
			hooks = object()
			root.Set("hooks", hooks)
		}
		g := hooks.Get(event)
		if g == nil {
			g = &node{Kind: '['}
			hooks.Set(event, g)
		}
		return g
	}
	for _, h := range []struct {
		e            Entry
		event, args  string
		matcher, say string
	}{
		{r.Stop, "Stop", argsDone, "", "the Stop hook (alerts when Claude is done)"},
		{r.Needs, "Notification", argsNeeds, NeedsMatcher, "the Notification hook (alerts when Claude needs you)"},
		{r.Prompt, "UserPromptSubmit", argsPrompt, "", "the prompt hook (shows which command each session runs)"},
		{r.AgentStart, "SubagentStart", argsAgentStart, "", "the subagent start hook (shows subagents)"},
		{r.AgentStop, "SubagentStop", argsAgentStop, "", "the subagent stop hook (subagent reports)"},
		{r.Tools, "PreToolUse", argsTool, "*", "the tool hook (each subagent's steps)"},
	} {
		switch h.e.State {
		case Missing:
			g := object()
			if h.matcher != "" {
				g.Set("matcher", scalar(h.matcher))
			}
			g.Set("hooks", &node{Kind: '[', Vals: []*node{hookNode(exe, h.args)}})
			list := group(h.event)
			list.Vals = append(list.Vals, g)
			summary = append(summary, "Add "+h.say)
		case Broken:
			commands(group(h.event), func(c *node) {
				if cmd, ok := c.Str(); ok {
					if _, ok := kazeCommand(cmd, h.args); ok {
						*c = *scalar(command(exe, h.args))
					}
				}
			})
			summary = append(summary, "Repoint "+h.say+" from "+h.e.Exe+" to "+exe)
		}
	}
	switch r.StatusLine.State {
	case Missing:
		s := object()
		s.Set("type", scalar("command"))
		s.Set("command", scalar(command(exe, argsStatus)))
		root.Set("statusLine", s)
		summary = append(summary, "Add the status line (usage in the status bar)")
	case Broken:
		root.Get("statusLine").Set("command", scalar(command(exe, argsStatus)))
		summary = append(summary, "Repoint the status line from "+r.StatusLine.Exe+" to "+exe)
	}
	if len(summary) == 0 {
		return settings, nil, nil
	}
	return root.Encode(), summary, nil
}

// Replace puts kaze's status line in place of another program's and returns
// the command it removed; nil when the settings cannot be read.
func Replace(settings []byte, exe string) ([]byte, string) {
	root, err := load(settings)
	if err != nil {
		return nil, ""
	}
	s := root.Get("statusLine")
	if s == nil {
		s = object()
		root.Set("statusLine", s)
	}
	removed, _ := s.Get("command").Str()
	if s.Get("type") == nil {
		s.Set("type", scalar("command"))
	}
	s.Set("command", scalar(command(exe, argsStatus)))
	return root.Encode(), removed
}

// Write copies the current file to settings.json.bak-YYYYMMDD-HHMMSS, then
// replaces it with data through a temp file and a rename.
func Write(path string, data []byte, now time.Time) error {
	old, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := backup(path+".bak-"+now.Format("20060102-150405"), old); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	// The backup stays beside the path the user knows; the write goes to a symlink's target.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

// backup writes old to base, or base-2, base-3, ... when a write in the same
// second already made one: no backup is ever overwritten.
func backup(base string, old []byte) error {
	for n := 1; n <= 100; n++ {
		name := base
		if n > 1 {
			name = fmt.Sprintf("%s-%d", base, n)
		}
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return err
		}
		_, err = f.Write(old)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
	return errors.New("too many backups this second")
}

// Relayout reports whether writing the settings back would change more than
// the fixed entries: the file is not in Claude Code's own layout.
func Relayout(settings []byte) bool {
	n, err := parse(settings)
	if err != nil {
		return false
	}
	norm := bytes.ReplaceAll(settings, []byte("\r\n"), []byte("\n"))
	return !bytes.Equal(bytes.TrimRight(n.Encode(), "\n"), bytes.TrimRight(norm, "\n"))
}

// Diff is the lines that differ between the settings before (as Fix would
// lay them out) and after, "- " removed and "+ " added, with "…" between
// separate changes and at the end when more than max lines would show.
func Diff(before, after []byte, max int) []string {
	if n, err := parse(before); err == nil {
		before = n.Encode()
	}
	a, b := lines(before), lines(after)
	if len(a)*len(b) > 4_000_000 {
		return []string{"(too large to compare)"}
	}
	// Longest common subsequence, from the end.
	lcs := make([][]int32, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int32, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max32(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []string
	gap := false
	add := func(s string) {
		if gap && len(out) > 0 {
			out = append(out, "…")
		}
		gap = false
		out = append(out, s)
	}
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		switch {
		case i < len(a) && j < len(b) && a[i] == b[j]:
			gap = true
			i, j = i+1, j+1
		case j < len(b) && (i == len(a) || lcs[i][j+1] >= lcs[i+1][j]):
			add("+ " + b[j])
			j++
		default:
			add("- " + a[i])
			i++
		}
	}
	if len(out) > max && max > 0 {
		out = append(out[:max-1], "…")
	}
	return out
}

func lines(b []byte) []string {
	return strings.Split(strings.TrimRight(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), "\n")
}

func max32(a, b int32) int32 {
	if a > b {
		return a
	}
	return b
}
