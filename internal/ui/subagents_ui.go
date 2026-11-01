package ui

import (
	"sync"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// maxSubEvents bounds the events waiting for a frame (a minimized window gets
// none). On overflow, redundant session reports and superseded tool steps
// make room for lifecycle events and each agent's latest step.
const maxSubEvents = 4096

// subInbox carries subagent events from the notify goroutines to the UI
// goroutine. It keeps step history up to the cap without allowing a busy
// agent's steps to displace another agent's completion or latest action.
type subInbox struct {
	mu       sync.Mutex
	evs      []subagents.Event
	sessions map[int]string // last pending session per source; rebuilt on compaction
	drop     []bool         // compaction scratch, at most maxSubEvents+1 entries
	latest   map[subSourceAgent]bool
}

type subSourceAgent struct {
	src         int
	session, id string
}

func (b *subInbox) put(es ...subagents.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range es {
		if e.Kind == subagents.EvSession {
			if old, ok := b.sessions[e.Src]; ok && old == e.Session {
				continue
			}
			if b.sessions == nil {
				b.sessions = make(map[int]string)
			}
			b.sessions[e.Src] = e.Session
		}
		b.evs = append(b.evs, e)
		if len(b.evs) > maxSubEvents {
			b.compact()
		}
	}
}

// compact removes obsolete tool history in a batch, making room for a burst
// instead of scanning and shifting all 4096 events on every overflowing put.
// The order of every retained event stays unchanged. Lifecycle events and
// each agent's newest tool/stop survive an ordinary tool flood.
func (b *subInbox) compact() {
	if cap(b.drop) < len(b.evs) {
		b.drop = make([]bool, len(b.evs))
	}
	b.drop = b.drop[:len(b.evs)]
	clear(b.drop)
	if b.latest == nil {
		b.latest = make(map[subSourceAgent]bool)
	}
	clear(b.latest)
	for i := len(b.evs) - 1; i >= 0; i-- {
		e := b.evs[i]
		if e.Kind != subagents.EvTool && e.Kind != subagents.EvStop {
			continue
		}
		k := subSourceAgent{e.Src, e.Session, e.AgentID}
		if e.Kind == subagents.EvTool && b.latest[k] {
			b.drop[i] = true
		}
		b.latest[k] = true
	}
	clear(b.latest) // scratch must not retain session/agent strings
	removed, want := 0, len(b.evs)-maxSubEvents/2
	for i := range b.drop {
		if b.drop[i] {
			if removed < want {
				removed++
			} else {
				b.drop[i] = false
			}
		}
	}
	// More distinct agents/control events than slots is unavoidable loss:
	// sacrifice only enough oldest steps, then controls, to keep the hard cap.
	for _, toolsOnly := range []bool{true, false} {
		for i, e := range b.evs {
			if len(b.evs)-removed <= maxSubEvents {
				break
			}
			if !b.drop[i] && (!toolsOnly || e.Kind == subagents.EvTool) {
				b.drop[i] = true
				removed++
			}
		}
	}
	clear(b.sessions)
	n := 0
	for i, e := range b.evs {
		if b.drop[i] {
			continue
		}
		b.evs[n] = e
		n++
		if e.Kind == subagents.EvSession {
			if b.sessions == nil {
				b.sessions = make(map[int]string)
			}
			b.sessions[e.Src] = e.Session
		}
	}
	clear(b.evs[n:])
	b.evs = b.evs[:n]
}

func (b *subInbox) take() []subagents.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	evs := b.evs
	b.evs = nil
	clear(b.sessions)
	return evs
}

// subKind reports whether k is one of the subagent kinds, which are never alerts.
func subKind(k notify.Kind) bool {
	return k == notify.KindAgentStart || k == notify.KindAgentStop || k == notify.KindTool
}

// eventsOf is what one hook message tells the tracker: its session first,
// then the event itself. src is the tab the hook ran in; m.Tab is the row.
// Claude subagents and Codex helpers share the same hook shapes.
func eventsOf(m notify.Msg, src int, now time.Time) []subagents.Event {
	base := subagents.Event{Tab: m.Tab, Src: src, Session: m.Session, AgentID: m.AgentID, AgentType: m.AgentType, At: now}
	var out []subagents.Event
	if m.Session != "" {
		s := base
		s.Kind = subagents.EvSession
		out = append(out, s)
	}
	e := base
	switch m.Kind {
	case notify.KindAgentStart:
		e.Kind, e.Prompt = subagents.EvStart, m.TaskPrompt
	case notify.KindAgentStop:
		e.Kind, e.Report = subagents.EvStop, m.Report
	case notify.KindTool:
		e.Kind, e.Tool, e.Summary, e.Desc = subagents.EvTool, m.Tool, m.Summary, m.Description
		if m.AgentID == "" {
			e.Kind, e.Prompt = subagents.EvAgentCall, m.TaskPrompt
		}
	case notify.KindNeeds:
		e.Kind = subagents.EvNeeds
	case notify.KindPrompt:
		e.Kind = subagents.EvPrompt
	default:
		return out
	}
	return append(out, e)
}

// drainSubs applies the queued subagent events; UI goroutine only.
func (win *window) drainSubs(now time.Time) {
	for _, e := range win.subIn.take() {
		if win.mgr.Lookup(tabs.TabID(e.Tab)) == nil {
			continue // closed while queued
		}
		if e.At.IsZero() {
			e.At = now // arrival time is kept: a minimized window drains late
		}
		if win.subTabs == nil {
			win.subTabs = map[int]bool{}
		}
		win.subTabs[e.Tab], win.subTabs[e.Src] = true, true
		win.subs.Apply(e)
	}
}

// subTimer is the next whole minute while a subagent runs, for the elapsed
// times; nothing otherwise, so an idle kaze never wakes for it.
func (win *window) subTimer(now time.Time) (time.Time, bool) {
	if win.subs.Active(0) == 0 {
		return time.Time{}, false
	}
	return now.Truncate(time.Minute).Add(time.Minute), true
}

// pruneSubs forgets the subagents of closed tabs.
func (win *window) pruneSubs(list []*tabs.Tab) {
	for id := range win.subTabs {
		if !tabIn(list, tabs.TabID(id)) {
			win.subs.CloseTab(id)
			delete(win.subTabs, id)
		}
	}
	win.dropGoneAgents()
}
