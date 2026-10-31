package ui

import (
	"sync"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// maxSubEvents bounds the events waiting for a frame (a minimized window gets
// none); past it the oldest go, a lost step at worst.
const maxSubEvents = 4096

// subInbox carries subagent events from the notify goroutines to the UI
// goroutine. Unlike the alert inbox it keeps every event: each step counts.
type subInbox struct {
	mu  sync.Mutex
	evs []subagents.Event
}

func (b *subInbox) put(es ...subagents.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.evs = append(b.evs, es...)
	if n := len(b.evs); n > maxSubEvents {
		b.evs = append(b.evs[:0:0], b.evs[n-maxSubEvents:]...)
	}
}

func (b *subInbox) take() []subagents.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	evs := b.evs
	b.evs = nil
	return evs
}

// subKind reports whether k is one of the subagent kinds, which are never alerts.
func subKind(k notify.Kind) bool {
	return k == notify.KindAgentStart || k == notify.KindAgentStop || k == notify.KindTool
}

// eventsOf is what one hook message tells the tracker: its session first,
// then the event itself. src is the tab the hook ran in; m.Tab is the row.
func eventsOf(m notify.Msg, src int, now time.Time) []subagents.Event {
	if m.Agent == "codex" {
		return nil
	}
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
		e.Kind = subagents.EvStart
	case notify.KindAgentStop:
		e.Kind, e.Report = subagents.EvStop, m.Report
	case notify.KindTool:
		e.Kind, e.Tool, e.Summary, e.Desc = subagents.EvTool, m.Tool, m.Summary, m.Description
		if m.AgentID == "" {
			e.Kind = subagents.EvAgentCall
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
}
