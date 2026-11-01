package ui

import (
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func TestAgentKindsNeverChime(t *testing.T) {
	win, played, flashes := liveWindow(t, 2)
	other := int(win.mgr.Tabs()[1].ID)
	win.publish(win.mgr.Active(), false)
	for _, m := range []notify.Msg{
		{Tab: other, Kind: notify.KindAgentStart, Session: "s1", AgentID: "a1", AgentType: "Explore"},
		{Tab: other, Kind: notify.KindTool, Session: "s1", AgentID: "a1", Tool: "Read"},
		{Tab: other, Kind: notify.KindAgentStop, Session: "s1", AgentID: "a1", Report: "r"},
	} {
		win.onNotify(m)
	}
	if len(*played) != 0 || *flashes != 0 || win.inbox.size() != 0 {
		t.Fatalf("%d chimes %d flashes %d queued alerts", len(*played), *flashes, win.inbox.size())
	}
	win.drainSubs(t0)
	if win.alerts.markOf(tabs.TabID(other)) != markNone || len(win.alerts.list()) != 0 {
		t.Fatal("a subagent message marked or toasted")
	}
	if _, _, done := win.subs.Board(); len(done) != 1 {
		t.Fatal("the messages did not reach the tracker")
	}
}

func TestDrainFeedsTracker(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	id := int(win.mgr.Tabs()[1].ID)
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindTool, Session: "s1", Tool: "Agent", AgentType: "Explore", Description: "Find auth", TaskPrompt: "Find the auth code."})
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStart, Session: "s1", AgentID: "a1", AgentType: "Explore"})
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindTool, Session: "s1", AgentID: "a1", Tool: "Grep", Summary: "auth"})
	win.drainSubs(t0)
	shown, _ := win.subs.Tree(id)
	if len(shown) != 1 || shown[0].Task != "Find auth" || shown[0].Prompt != "Find the auth code." ||
		len(shown[0].Steps) != 1 || shown[0].Steps[0].Summary != "auth" {
		t.Fatalf("tree %+v", shown)
	}
}

func TestNeedsAndPromptReachTracker(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	id := int(win.mgr.Tabs()[1].ID)
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStart, Session: "s1", AgentID: "a1", AgentType: "X"})
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindNeeds, Session: "s1", AgentID: "a1"})
	win.drainSubs(t0)
	if a := win.subs.Get(subagents.Key{Session: "s1", ID: "a1"}); a.State != subagents.Waiting {
		t.Fatalf("state %v", a.State)
	}
	if win.inbox.size() != 1 {
		t.Fatal("needs must still alert as today")
	}
}

func TestSessionIdentityStored(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	deliverNow(win, notify.Msg{Tab: int(id), Kind: notify.KindDone, Session: "s1", Transcript: "/p/s1.jsonl", Cwd: "/p"}, t0)
	if a := win.agentOf(id); a.session != "s1" || a.transcript != "/p/s1.jsonl" || a.cwd != "/p" {
		t.Fatalf("state %+v", a)
	}
}

func TestScratchMessagesUseSourceSession(t *testing.T) {
	win, _, _ := liveWindow(t, 1)
	main := win.mgr.Tabs()[0]
	s, err := win.mgr.NewOpts(tabs.StartOpts{Parent: main.ID})
	if err != nil {
		t.Fatal(err)
	}
	win.onNotify(notify.Msg{Tab: int(main.ID), Kind: notify.KindAgentStart, Session: "m1", AgentID: "a1", AgentType: "X"})
	win.onNotify(notify.Msg{Tab: int(s.ID), Kind: notify.KindAgentStart, Session: "x1", AgentID: "b1", AgentType: "Y"})
	win.drainSubs(t0)
	if shown, _ := win.subs.Tree(int(main.ID)); len(shown) != 2 {
		t.Fatalf("both should show on the session row, got %d", len(shown))
	}
	win.onNotify(notify.Msg{Tab: int(s.ID), Kind: notify.KindDone, Session: "x2"}) // /clear in the scratch shell
	win.drainSubs(t0)
	if win.subs.Get(subagents.Key{Session: "m1", ID: "a1"}) == nil || win.subs.Get(subagents.Key{Session: "x1", ID: "b1"}) != nil {
		t.Fatal("the scratch shell's new session reset the wrong agents")
	}
}

func TestTimerOnlyWhileActive(t *testing.T) {
	win, _, _ := liveWindow(t, 1)
	id := int(win.mgr.Tabs()[0].ID)
	now := t0.Add(20 * time.Second)
	if _, ok := win.subTimer(now); ok {
		t.Fatal("a timer with no subagents")
	}
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStart, Session: "s", AgentID: "a", AgentType: "X"})
	win.drainSubs(now)
	if at, ok := win.subTimer(now); !ok || !at.Equal(t0.Add(time.Minute)) {
		t.Fatalf("timer %v %v", at, ok)
	}
	win.onNotify(notify.Msg{Tab: id, Kind: notify.KindAgentStop, Session: "s", AgentID: "a"})
	win.drainSubs(now)
	if _, ok := win.subTimer(now); ok {
		t.Fatal("a timer after the last stop")
	}
}

func TestCloseTabDropsAgents(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindAgentStart, Session: "s", AgentID: "a", AgentType: "X"})
	win.drainSubs(t0)
	win.pruneAgents(win.mgr.Tabs()[:1])
	if win.subs.Active(0) != 0 {
		t.Fatal("closed tab's agents kept")
	}
	if _, ok := win.subTimer(t0); ok {
		t.Fatal("the timer outlived the tab")
	}
}

func TestSubInboxIsBounded(t *testing.T) {
	var b subInbox
	for i := 0; i < maxSubEvents+10; i++ {
		b.put(subagents.Event{Tool: "x"})
	}
	if n := len(b.take()); n != maxSubEvents {
		t.Fatalf("kept %d", n)
	}
}
