package ui

import (
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/procs"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// detectEvery is the most often one tab's foreground program is checked.
const detectEvery = time.Second

// agentProbe looks at what runs in a tab; tests replace it.
type agentProbe struct {
	fg   func(*tabs.Tab) int    // the process group in front of the tab's pty, 0 if unknown
	out  func(*tabs.Tab) uint64 // moves whenever the tab printed
	name func(pid int) string   // "claude", "codex" or ""
}

// detection is the last check of one tab.
type detection struct {
	at    time.Time
	out   uint64
	pg    int
	agent string
}

func defaultProbe() agentProbe {
	return agentProbe{
		fg:   func(t *tabs.Tab) int { return t.Session.Foreground() },
		out:  func(t *tabs.Tab) uint64 { return t.Session.OutputSeq() },
		name: procs.Agent,
	}
}

// detectAgents names the agent in front of each tab that printed since its last
// check, at most once per detectEvery, so an idle kaze does no work. It returns
// when a skipped check falls due, or zero when none is waiting. UI goroutine only.
func (win *window) detectAgents(now time.Time) (wake time.Time) {
	if win.probe.fg == nil {
		win.probe = defaultProbe()
	}
	if win.detected == nil {
		win.detected = map[tabs.TabID]detection{}
	}
	live := map[tabs.TabID]bool{}
	for _, t := range win.mgr.Tabs() {
		live[t.ID] = true
		d, seen := win.detected[t.ID]
		out := win.probe.out(t)
		if seen && out == d.out {
			continue // nothing printed: nothing can have started or quit
		}
		if due := d.at.Add(detectEvery); seen && now.Before(due) {
			if wake.IsZero() || due.Before(wake) {
				wake = due
			}
			continue
		}
		d.at, d.out = now, out
		if pg := win.probe.fg(t); pg != d.pg || !seen {
			was := d.agent
			d.pg, d.agent = pg, win.probe.name(pg)
			if seen && was != "" && d.agent == "" && pg != 0 {
				win.restoreAgentQuit(t.ID) // the shell is back: nothing left to resume
			}
		}
		win.detected[t.ID] = d
	}
	for id := range win.detected {
		if !live[id] {
			delete(win.detected, id)
		}
	}
	return wake
}

// rowAgent is a tab's agent state as its row shows it: the hooks' when they named
// an agent, else the program found in front of the tab.
func (win *window) rowAgent(id tabs.TabID) agentState {
	a := win.agentOf(id)
	if a.agentName() == "" {
		a.agent = win.detected[id].agent
	}
	return a
}
