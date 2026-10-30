package ui

import (
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const maxAgentArg = 40

// agentState is what the hook messages say about the Claude in one tab.
type agentState struct {
	claude, running bool
	command, arg    string // last slash command and its first argument
	shellAt         uint64 // cwdSeq(t) when the last hook message arrived
}

// agentOf is the zero value for an unknown tab.
func (win *window) agentOf(id tabs.TabID) agentState { return win.agents[id] }

// applyAgent folds one hook message into the tab's state; UI goroutine only.
func (win *window) applyAgent(m notify.Msg, t *tabs.Tab) {
	if t == nil {
		return
	}
	win.noteAgent(m, t.ID, cwdSeq(t))
}

func (win *window) noteAgent(m notify.Msg, id tabs.TabID, seq uint64) {
	a := win.agents[id]
	switch m.Kind {
	case notify.KindPrompt:
		a.claude, a.running = true, true
		if m.Command != "" {
			a.command = m.Command
			a.arg = firstWord(m.Args, maxAgentArg)
		}
		win.alerts.visited(id) // a new prompt answers whatever the tab was waiting on
	case notify.KindDone, notify.KindNeeds:
		a.claude, a.running = true, false
	case notify.KindUsage:
		a.claude = true
	default:
		return
	}
	a.shellAt = seq
	if win.agents == nil {
		win.agents = map[tabs.TabID]agentState{}
	}
	win.agents[id] = a
}

// firstWord is the first whitespace-separated word of s, cut to n runes.
func firstWord(s string, n int) string {
	f := strings.Fields(s)
	if len(f) == 0 {
		return ""
	}
	if r := []rune(f[0]); len(r) > n {
		return string(r[:n])
	}
	return f[0]
}

// leaveAgents ends Claude in tabs whose shell reported a folder since the
// last hook message: the prompt is back. The arg stays as the row's title.
func (win *window) leaveAgents(list []*tabs.Tab) {
	for _, t := range list {
		a, ok := win.agents[t.ID]
		if !ok || !a.claude || cwdSeq(t) <= a.shellAt {
			continue
		}
		a.claude, a.running, a.command = false, false, ""
		win.agents[t.ID] = a
	}
}

// pruneAgents forgets closed tabs, and the names the owner gave their rows.
func (win *window) pruneAgents(list []*tabs.Tab) {
	for id := range win.agents {
		if !tabIn(list, id) {
			delete(win.agents, id)
		}
	}
	for id := range win.names {
		if !tabIn(list, id) {
			delete(win.names, id)
		}
	}
}

func tabIn(list []*tabs.Tab, id tabs.TabID) bool {
	for _, t := range list {
		if t.ID == id {
			return true
		}
	}
	return false
}
