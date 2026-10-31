package ui

import (
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
)

func TestSubInboxOverflowKeepsStopAndQuietAgentResume(t *testing.T) {
	win, _, _ := liveWindow(t, 1)
	id := int(win.mgr.Active().ID)
	for _, agent := range []string{"done", "quiet"} {
		win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStart, Session: "s", AgentID: agent})
	}
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindNeeds, Session: "s", AgentID: "quiet"})
	win.drainSubs(t0)
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStop, Session: "s", AgentID: "done", Report: "Final result"})
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindTool, Session: "s", AgentID: "quiet", Tool: "Read"})
	for i := 0; i < maxSubEvents+10; i++ {
		win.onNotify(notify.Msg{Tab: id, Kind: notify.KindTool, Session: "s", AgentID: "busy", Tool: "Read"})
	}
	if len(win.subIn.evs) > maxSubEvents {
		t.Fatalf("unbounded queue: %d", len(win.subIn.evs))
	}
	win.drainSubs(t0)
	a := win.subs.Get(subagents.Key{Session: "s", ID: "done"})
	if a.State != subagents.Done || a.Report != "Final result" {
		t.Fatalf("lost completion: state=%v report=%q", a.State, a.Report)
	}
	if a := win.subs.Get(subagents.Key{Session: "s", ID: "quiet"}); a.State != subagents.Running {
		t.Fatalf("lost resume: %v", a.State)
	}
}

func TestSubInboxOverflowKeepsSessionChangesInOrder(t *testing.T) {
	var b subInbox
	b.put(subagents.Event{Kind: subagents.EvSession, Src: 1, Tab: 1, Session: "old"})
	for i := 0; i < maxSubEvents; i++ {
		b.put(subagents.Event{Kind: subagents.EvTool, Src: 1, Tab: 1, Session: "old", AgentID: "a"})
	}
	b.put(subagents.Event{Kind: subagents.EvSession, Src: 1, Tab: 1, Session: "new"})
	b.put(subagents.Event{Kind: subagents.EvStart, Src: 1, Tab: 1, Session: "new", AgentID: "b"})
	for i := 0; i < maxSubEvents; i++ {
		b.put(subagents.Event{Kind: subagents.EvSession, Src: 1, Tab: 1, Session: "new"}, subagents.Event{Kind: subagents.EvTool, Src: 1, Tab: 1, Session: "new", AgentID: "b"})
	}
	var tr subagents.Tracker
	for _, e := range b.take() {
		tr.Apply(e)
	}
	if tr.Get(subagents.Key{Session: "old", ID: "a"}) != nil {
		t.Fatal("retained old conversation")
	}
	if tr.Get(subagents.Key{Session: "new", ID: "b"}) == nil {
		t.Fatal("lost new conversation")
	}
}

func TestLateStopAfterShellReturnKeepsReport(t *testing.T) {
	win, _, _ := liveWindow(t, 1)
	tab := win.mgr.Active()
	id := int(tab.ID)
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindPrompt, Session: "s"})
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStart, Session: "s", AgentID: "a"})
	win.drainAlerts(t0)
	win.drainSubs(t0)
	promptMark(t, tab)
	win.leaveAgents(win.mgr.Tabs())
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStop, Session: "s", AgentID: "a", Report: "Final result"})
	win.drainSubs(t0)
	if a := win.subs.Get(subagents.Key{Session: "s", ID: "a"}); a.Report != "Final result" {
		t.Fatalf("report lost after shell return: %+v", a)
	}
}
