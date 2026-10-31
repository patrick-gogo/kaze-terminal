package ui

import (
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const maxAgentArg = 40

// agentState is what the hook messages say about the agent in one tab.
type agentState struct {
	claude, running bool
	agent           string
	command, arg    string // last submitted command and its first argument
	shellAt         uint64 // shellSeq(t) when the last hook message arrived
	// The Claude session the latest message came from (issue #23 Part 1).
	session, transcript, cwd string
}

// agentName keeps old in-memory/test states compatible with Claude-only rows.
func (a agentState) agentName() string {
	if a.agent != "" {
		return a.agent
	}
	if a.claude {
		return "claude"
	}
	return ""
}

func agentCommand(agent, command string) string {
	if agent == "" {
		return command
	}
	if command == "" {
		return agent
	}
	return agent + " · " + command
}

// agentOf is the zero value for an unknown tab.
func (win *window) agentOf(id tabs.TabID) agentState { return win.agents[id] }

// shellSeq counts the shell's prompt reports, OSC 7 and OSC 133 A alike. A sum,
// not a max, so a move in either counts even when the other is ahead.
func shellSeq(t *tabs.Tab) uint64 {
	s := t.Session
	s.Lock()
	defer s.Unlock()
	a := s.Applier()
	return a.CwdSeq() + a.PromptSeq()
}

// noteAgent folds one hook message into the tab's state; UI goroutine only.
// seq is shellSeq when the message arrived, not when a frame applies it.
func (win *window) noteAgent(m notify.Msg, id tabs.TabID, seq uint64) {
	a := win.agents[id]
	agent := m.Agent
	if agent == "" {
		agent = "claude"
	}
	if m.Kind == notify.KindUsage && a.agentName() == "codex" {
		return
	}
	if old := a.agentName(); old != "" && old != agent {
		a.command, a.arg = "", ""
	}
	switch m.Kind {
	case notify.KindPrompt:
		a.claude, a.running = agent == "claude", true
		if m.Command != "" {
			a.command = m.Command
			a.arg = firstWord(m.Args, maxAgentArg)
		}
		win.alerts.visited(id) // a new prompt answers whatever the tab was waiting on
	case notify.KindDone, notify.KindNeeds, notify.KindInterrupt:
		a.claude, a.running = agent == "claude", false
	case notify.KindUsage:
		a.claude = true
	default:
		return
	}
	a.agent = agent
	if m.Session != "" {
		a.session, a.transcript, a.cwd = m.Session, m.Transcript, m.Cwd
	}
	if agent == "codex" {
		delete(win.ctx, id)
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

// leaveAgents ends the agent in tabs whose shell reported a prompt since the last
// hook message: the prompt is back. The arg stays as the row's title.
func (win *window) leaveAgents(list []*tabs.Tab) {
	for _, t := range list {
		a, ok := win.agents[t.ID]
		if !ok || a.agentName() == "" || shellSeq(t) <= a.shellAt {
			continue
		}
		a.claude, a.running, a.command, a.agent = false, false, "", ""
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
	win.pruneSubs(list)
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
