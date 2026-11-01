// Package codexsetup checks Codex hook sources and repairs only hooks.json.
package codexsetup

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/patrick-gogo/kaze-terminal/internal/claudesetup"
	j "github.com/patrick-gogo/kaze-terminal/internal/orderedjson"
)

type State = claudesetup.State

const (
	OK      = claudesetup.OK
	Missing = claudesetup.Missing
	Broken  = claudesetup.Broken
	Taken   = claudesetup.Taken
)

type Entry struct {
	State                        State
	Exe, Command, Source, Detail string
	Fixable                      bool
}
type Report struct{ Needs, Stop, Prompt, Interrupt, AgentStart, AgentStop, Tools Entry }

func (r Report) Entries() []Entry {
	return []Entry{r.Needs, r.Stop, r.Prompt, r.Interrupt, r.AgentStart, r.AgentStop, r.Tools}
}

// events is in Report field order. The last three show Codex helpers (#63).
var events = []struct{ event, kind string }{
	{"PermissionRequest", "needs"}, {"Stop", "done"}, {"UserPromptSubmit", "prompt"}, {"Interrupt", "interrupt"},
	{"SubagentStart", "agent-start"}, {"SubagentStop", "agent-stop"}, {"PreToolUse", "tool"},
}

func load(data []byte) (*j.Node, error) {
	if data == nil {
		return j.Object(), nil
	}
	root, err := j.Parse(data)
	if err != nil {
		return nil, err
	}
	if duplicateKeys(root) {
		return nil, fmt.Errorf("duplicate JSON keys; resolve them before fixing hooks")
	}
	if root.Kind != '{' {
		return nil, fmt.Errorf("hooks source must be an object")
	}
	h := root.Get("hooks")
	if h == nil {
		return root, nil
	}
	if h.Kind != '{' {
		return nil, fmt.Errorf("hooks must be an object")
	}
	for _, ev := range events {
		groups := h.Get(ev.event)
		if groups == nil {
			continue
		}
		if groups.Kind != '[' {
			return nil, fmt.Errorf("%s must be a list", ev.event)
		}
		for _, g := range groups.Vals {
			if g.Kind != '{' || g.Get("hooks") == nil || g.Get("hooks").Kind != '[' {
				return nil, fmt.Errorf("invalid %s hook group", ev.event)
			}
			if m := g.Get("matcher"); m != nil {
				if _, ok := m.Str(); !ok {
					return nil, fmt.Errorf("%s matcher must be a string", ev.event)
				}
			}
			for _, handler := range g.Get("hooks").Vals {
				if handler.Kind != '{' {
					return nil, fmt.Errorf("invalid %s handler", ev.event)
				}
				typ, ok := handler.Get("type").Str()
				if !ok {
					return nil, fmt.Errorf("%s handler type must be a string", ev.event)
				}
				if typ == "command" {
					if _, ok := handler.Get("command").Str(); !ok {
						return nil, fmt.Errorf("%s command must be a string", ev.event)
					}
				}
			}
		}
	}
	return root, nil
}
func duplicateKeys(n *j.Node) bool {
	seen := map[string]bool{}
	for _, key := range n.Keys {
		if seen[key] {
			return true
		}
		seen[key] = true
	}
	for _, v := range n.Vals {
		if duplicateKeys(v) {
			return true
		}
	}
	return false
}

func inline(data []byte) (*j.Node, error) {
	if len(data) == 0 {
		return j.Object(), nil
	}
	var cfg map[string]any
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return nil, fmt.Errorf("config.toml: %w", err)
	}
	// Hook trust state is separate from event groups; it is never edited here.
	b, err := json.Marshal(map[string]any{"hooks": cfg["hooks"]})
	if err != nil {
		return nil, err
	}
	if cfg["hooks"] == nil {
		return j.Object(), nil
	}
	return load(b)
}
func disabled(n *j.Node) bool {
	return n != nil && ((n.Get("disabled") != nil && n.Get("disabled").Raw == "true") || (n.Get("enabled") != nil && n.Get("enabled").Raw == "false"))
}

// own recognizes a direct Kaze hook command, never an arbitrary shell chain.
func own(cmd, kind string) (exe string, correct bool, recognized bool) {
	cmd = strings.TrimSpace(cmd)
	if cmd == "" {
		return
	}
	var tail string
	if cmd[0] == '"' || cmd[0] == '\'' {
		quote := cmd[:1]
		rest := cmd[1:]
		var ok bool
		exe, tail, ok = strings.Cut(rest, quote)
		if !ok {
			return "", false, false
		}
	} else {
		fields := strings.Fields(cmd)
		if len(fields) < 2 {
			return "", false, false
		}
		exe = fields[0]
		tail = strings.TrimSpace(strings.TrimPrefix(cmd, exe))
	}
	base := exe[strings.LastIndexAny(exe, `/\`)+1:]
	if !strings.EqualFold(base, "kaze") && !strings.EqualFold(base, "kaze.exe") {
		return "", false, false
	}
	args := strings.Join(strings.Fields(tail), " ")
	for _, ev := range events {
		prefix := "notify " + ev.kind
		if args == prefix || args == prefix+" --agent codex" || args == prefix+" --agent claude" {
			return exe, args == "notify "+kind+" --agent codex", true
		}
	}
	return "", false, false
}

type candidate struct {
	entry          Entry
	group, handler *j.Node
}

func candidates(root *j.Node, event, kind, source string, exists func(string) bool) (found []candidate, other bool) {
	for _, g := range root.Get("hooks").Get(event).ListVals() {
		for _, h := range g.Get("hooks").Vals {
			cmd, _ := h.Get("command").Str()
			exe, correct, recognized := own(cmd, kind)
			if !recognized {
				other = true
				continue
			}
			typ, _ := h.Get("type").Str()
			matcher, _ := g.Get("matcher").Str()
			// Kaze needs every permission prompt, helper and tool step, so a narrowed matcher is broken.
			matches := matcher == "" || matcher == "*" || matcher == ".*"
			state := Broken
			if correct && typ == "command" && matches && !disabled(root.Get("hooks")) && !disabled(g) && !disabled(h) && (h.Get("async") == nil || h.Get("async").Raw != "true") && exists(exe) {
				state = OK
			}
			found = append(found, candidate{Entry{State: state, Exe: exe, Command: cmd, Source: source, Fixable: source == "hooks.json"}, g, h})
		}
	}
	return
}
func inspectEntry(root, cfg *j.Node, event, kind string, exists func(string) bool) Entry {
	a, other := candidates(root, event, kind, "hooks.json", exists)
	b, _ := candidates(cfg, event, kind, "config.toml", exists)
	all := append(a, b...)
	if len(all) > 1 {
		return Entry{State: Broken, Source: "multiple hooks", Detail: "Multiple Kaze handlers; remove duplicates before fixing"}
	}
	if len(all) == 1 {
		e := all[0].entry
		if e.State == Broken && e.Source == "config.toml" {
			e.Detail = "Repair this hook in config.toml; no duplicate will be added"
		}
		if disabled(root.Get("hooks")) {
			e.Fixable = false
			e.Detail = "Hooks are disabled in hooks.json"
		}
		return e
	}
	state := Missing
	if other {
		state = Taken
	}
	e := Entry{State: state, Source: "hooks.json", Fixable: true}
	if disabled(root.Get("hooks")) {
		e.Fixable = false
		e.Detail = "Hooks are disabled in hooks.json"
	}
	return e
}

// Inspect merges read-only inline hooks with hooks.json. nil means absent.
func Inspect(data, config []byte, exists func(string) bool) (Report, error) {
	root, err := load(data)
	if err != nil {
		return Report{}, fmt.Errorf("hooks.json: %w", err)
	}
	cfg, err := inline(config)
	if err != nil {
		return Report{}, err
	}
	entries := make([]Entry, len(events))
	for i, ev := range events {
		entries[i] = inspectEntry(root, cfg, ev.event, ev.kind, exists)
	}
	return Report{entries[0], entries[1], entries[2], entries[3], entries[4], entries[5], entries[6]}, nil
}

// Fix adds missing handlers and replaces broken Kaze handlers. Other commands
// and inline hooks are preserved. Callers confirm and check stale sources first.
func Fix(data, config []byte, exe string, exists func(string) bool) ([]byte, []string, error) {
	r, err := Inspect(data, config, exists)
	if err != nil {
		return nil, nil, err
	}
	root, _ := load(data)
	var summary []string
	for i, e := range r.Entries() {
		if e.State == OK || !e.Fixable {
			continue
		}
		ev := events[i]
		hooks := root.Get("hooks")
		if hooks == nil {
			hooks = j.Object()
			root.Set("hooks", hooks)
		}
		groups := hooks.Get(ev.event)
		if groups == nil {
			groups = &j.Node{Kind: '['}
			hooks.Set(ev.event, groups)
		}
		var repaired *j.Node
		// Move only Kaze's broken handler out of restricted/shared groups. The
		// existing group's matcher and other handlers must retain their behavior.
		for _, g := range groups.Vals {
			hs := g.Get("hooks")
			kept := hs.Vals[:0]
			for _, h := range hs.Vals {
				cmd, _ := h.Get("command").Str()
				_, _, recognized := own(cmd, ev.kind)
				if !recognized {
					kept = append(kept, h)
				} else {
					repaired = h
				}
			}
			hs.Vals = kept
		}
		h := repaired
		if h == nil {
			h = j.Object()
		}
		for _, key := range []string{"disabled", "async"} {
			if h.Get(key) != nil {
				h.Set(key, j.Scalar(false))
			}
		}
		if h.Get("enabled") != nil {
			h.Set("enabled", j.Scalar(true))
		}
		h.Set("type", j.Scalar("command"))
		h.Set("command", j.Scalar(`"`+exe+`" notify `+ev.kind+` --agent codex`))
		h.Set("timeout", &j.Node{Raw: "3"})
		g := j.Object()
		g.Set("hooks", &j.Node{Kind: '[', Vals: []*j.Node{h}})
		groups.Vals = append(groups.Vals, g)
		summary = append(summary, "Set up "+ev.event+" for Codex")
	}
	if len(summary) == 0 {
		return data, nil, nil
	}
	return root.Encode(), summary, nil
}
