// Package subagents folds Claude Code subagent and Codex helper hook messages into what the
// sidebar tree, the live pane and the board show. It has no UI and reads no
// clock: every event carries its time.
package subagents

import (
	"slices"
	"time"
)

// Limits from the spec.
const (
	maxQueue = 16  // pending Agent calls per source tab and session
	maxSteps = 200 // per subagent
	maxTree  = 6   // lines per session in the tree
	maxDone  = 50  // finished subagents kept for the board
)

type State uint8

const (
	Running State = iota
	Waiting
	Done
)

// Key names a subagent: its id is only unique within its session.
type Key struct{ Session, ID string }

type Step struct {
	At            time.Time
	Tool, Summary string
}

type Agent struct {
	Key            Key
	Tab            int // the row it shows under
	Type, Task     string
	Parent         Key // zero when the main conversation started it
	Depth          int // 0 = top level; deeper nesting draws at 1
	State          State
	Started, Ended time.Time
	Steps          []Step // the newest maxSteps
	Dropped        int    // steps dropped off the front
	Report         string
	InTree         bool // false once the session's next prompt came after it finished

	src     int  // the tab the hooks ran in
	stopped bool // true only after a real stop; EndTab makes a provisional Done entry
}

type EventKind uint8

const (
	EvStart     EventKind = iota // SubagentStart
	EvStop                       // SubagentStop
	EvTool                       // a step inside a subagent
	EvAgentCall                  // an Agent tool call: names the next start
	EvNeeds                      // a permission prompt or question
	EvPrompt                     // the owner sent a prompt in the session
	EvSession                    // the session id the hook ran in
)

type Event struct {
	Kind                        EventKind
	Tab                         int // the row's tab (after hookTab): Tree and Board use it
	Src                         int // the tab the hook ran in (a scratch shell's own id): session resets use it
	Session, AgentID, AgentType string
	Tool, Summary, Desc, Report string
	At                          time.Time
}

type call struct {
	desc, typ string
	parent    Key
}

type queueKey struct {
	src     int
	session string
}

// Tracker is every subagent kaze knows about. The zero value is ready.
type Tracker struct {
	agents map[Key]*Agent
	order  []*Agent // start order
	done   []*Agent // stop order, oldest first
	queues map[queueKey][]call
	seen   map[int]string // source tab → its last session
	waits  map[int]bool   // row tab → waiting with no subagent named
}

func (t *Tracker) init() {
	if t.agents == nil {
		t.agents, t.queues = map[Key]*Agent{}, map[queueKey][]call{}
		t.seen, t.waits = map[int]string{}, map[int]bool{}
	}
}

// Apply folds one event in and reports whether anything shown changed.
func (t *Tracker) Apply(e Event) bool {
	t.init()
	k := Key{e.Session, e.AgentID}
	switch e.Kind {
	case EvSession:
		return t.session(e)
	case EvAgentCall:
		q := queueKey{e.Src, e.Session}
		c := call{desc: e.Desc, typ: e.AgentType}
		if e.AgentID != "" {
			c.parent = k
		}
		t.queues[q] = append(t.queues[q], c)
		if n := len(t.queues[q]); n > maxQueue {
			t.queues[q] = t.queues[q][n-maxQueue:]
		}
		return false
	case EvStart:
		if e.AgentID == "" || t.agents[k] != nil {
			return false
		}
		a := t.add(e)
		q := queueKey{e.Src, e.Session}
		if i := slices.IndexFunc(t.queues[q], func(c call) bool { return c.typ == e.AgentType }); i >= 0 {
			c := t.queues[q][i]
			t.queues[q] = slices.Delete(t.queues[q], i, i+1)
			if c.desc != "" {
				a.Task = c.desc
			}
			if p := t.agents[c.parent]; p != nil {
				a.Parent, a.Depth = c.parent, min(p.Depth+1, 1)
			}
		}
		return true
	case EvTool:
		if e.AgentID == "" {
			return false
		}
		a := t.get(e)
		step := Step{At: e.At, Tool: e.Tool, Summary: e.Summary}
		if len(a.Steps) == maxSteps {
			// Keep the public chronological slice without allocating a new
			// 200-step history for every tool event after it fills.
			copy(a.Steps, a.Steps[1:])
			a.Steps[maxSteps-1] = step
			a.Dropped++
		} else {
			a.Steps = append(a.Steps, step)
		}
		if a.State == Waiting {
			a.State = Running
		}
		return true
	case EvNeeds:
		if e.AgentID == "" {
			if t.Active(e.Tab) == 0 || t.waits[e.Tab] {
				return false
			}
			t.waits[e.Tab] = true
			return true
		}
		a := t.get(e)
		if a.State != Running {
			return false
		}
		a.State = Waiting
		return true
	case EvStop:
		if e.AgentID == "" {
			return false
		}
		a := t.get(e)
		if a.stopped {
			return false
		}
		if a.State == Done {
			// A shell return can beat delivery of SubagentStop. Complete
			// that provisional entry without keeping a second board card.
			t.done = slices.DeleteFunc(t.done, func(old *Agent) bool { return old == a })
		}
		a.stopped = true
		a.State, a.Ended, a.Report = Done, e.At, e.Report
		t.done = append(t.done, a)
		if len(t.done) > maxDone {
			t.drop(func(x *Agent) bool { return x == t.done[0] })
		}
		return true
	case EvPrompt:
		changed := t.waits[e.Tab]
		delete(t.waits, e.Tab)
		for _, a := range t.order {
			if a.Tab == e.Tab && a.State == Done && a.InTree {
				a.InTree, changed = false, true
			}
		}
		return changed
	}
	return false
}

// session handles a session report: a new one in a source tab ends that
// tab's old conversation (spec 5.0). Finished subagents stay on the board.
func (t *Tracker) session(e Event) bool {
	old, ok := t.seen[e.Src]
	if e.Session == "" || old == e.Session {
		return false
	}
	t.seen[e.Src] = e.Session
	if !ok {
		return false
	}
	delete(t.queues, queueKey{e.Src, old})
	delete(t.waits, e.Tab)
	fromOld := func(a *Agent) bool { return a.src == e.Src && a.Key.Session == old }
	for _, a := range t.order {
		if fromOld(a) {
			a.InTree = false
		}
	}
	t.drop(func(a *Agent) bool { return fromOld(a) && a.State != Done })
	return true
}

func (t *Tracker) add(e Event) *Agent {
	a := &Agent{Key: Key{e.Session, e.AgentID}, Tab: e.Tab, src: e.Src, Type: e.AgentType, Task: e.AgentType,
		Started: e.At, InTree: true}
	if a.Task == "" {
		a.Task = "subagent"
	}
	t.agents[a.Key] = a
	t.order = append(t.order, a)
	return a
}

// get finds the event's subagent, or starts one kaze missed the start of.
func (t *Tracker) get(e Event) *Agent {
	if a := t.agents[Key{e.Session, e.AgentID}]; a != nil {
		return a
	}
	return t.add(e)
}

// drop forgets every agent gone says so.
func (t *Tracker) drop(gone func(*Agent) bool) {
	for _, a := range t.order {
		if gone(a) {
			delete(t.agents, a.Key)
		}
	}
	t.order = slices.DeleteFunc(t.order, gone)
	t.done = slices.DeleteFunc(t.done, gone)
}

// CloseTab forgets the agents that show under tab or ran in it.
func (t *Tracker) CloseTab(tab int) {
	t.init()
	t.drop(func(a *Agent) bool { return a.Tab == tab || a.src == tab })
	for q := range t.queues {
		if q.src == tab {
			delete(t.queues, q)
		}
	}
	delete(t.seen, tab)
	delete(t.waits, tab)
}

// Tree is tab's subagents for the sidebar: parents before their children,
// finished ones only while another still runs,
// otherwise in start order, at most maxTree; more counts the rest.
func (t *Tracker) Tree(tab int) (shown []*Agent, more int) {
	var all []*Agent
	in := map[Key]bool{}
	busy := t.Active(tab) > 0 // finished ones stay only beside a sibling still at work
	for _, a := range t.order {
		if a.Tab == tab && a.InTree && (busy || a.State != Done) {
			all = append(all, a)
			in[a.Key] = true
		}
	}
	var walk func(parent Key, root bool)
	walk = func(parent Key, root bool) {
		for _, a := range all {
			if root && !in[a.Parent] || !root && a.Parent == parent && in[parent] {
				shown = append(shown, a)
				walk(a.Key, false)
			}
		}
	}
	walk(Key{}, true)
	if len(shown) > maxTree {
		return shown[:maxTree], len(shown) - maxTree
	}
	return shown, 0
}

// Board is every subagent by column: running and waiting in start order,
// done newest first.
func (t *Tracker) Board() (running, waiting, done []*Agent) {
	for _, a := range t.order {
		switch a.State {
		case Running:
			running = append(running, a)
		case Waiting:
			waiting = append(waiting, a)
		}
	}
	for i := len(t.done) - 1; i >= 0; i-- {
		done = append(done, t.done[i])
	}
	return running, waiting, done
}

// SessionWaits are the tabs waiting on the owner with no subagent named,
// while one of their subagents runs (spec 5.4's fallback).
func (t *Tracker) SessionWaits() []int {
	var tabs []int
	for tab, on := range t.waits {
		if on && t.Active(tab) > 0 {
			tabs = append(tabs, tab)
		}
	}
	slices.Sort(tabs)
	return tabs
}

func (t *Tracker) Get(k Key) *Agent { return t.agents[k] }

// Active counts running and waiting subagents in tab, or in every tab for 0.
func (t *Tracker) Active(tab int) int {
	n := 0
	for _, a := range t.order {
		if a.State != Done && (tab == 0 || a.Tab == tab) {
			n++
		}
	}
	return n
}

// EndTab marks the subagents that ran in tab done: Claude left without stopping them.
func (t *Tracker) EndTab(tab int, at time.Time) bool {
	changed := false
	for _, a := range t.order {
		if a.src == tab && a.State != Done {
			a.State, a.Ended, changed = Done, at, true
			t.done = append(t.done, a)
		}
	}
	if len(t.done) > maxDone {
		over := t.done[:len(t.done)-maxDone]
		t.drop(func(a *Agent) bool { return slices.Contains(over, a) })
	}
	delete(t.waits, tab)
	return changed
}
