package ui

import (
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func TestAgentPaneIDsStableAndNegative(t *testing.T) {
	var f floats
	k1, k2 := subagents.Key{Session: "s", ID: "a"}, subagents.Key{Session: "s", ID: "b"}
	a, b := f.agentID(k1), f.agentID(k2)
	if a >= 0 || b >= 0 || a == b || f.agentID(k1) != a {
		t.Fatalf("ids %d %d", a, b)
	}
}

func TestOpenAgentPaneRaisesExisting(t *testing.T) {
	var f floats
	id := f.agentID(subagents.Key{Session: "s", ID: "a"})
	f.open(1, f.agentID(subagents.Key{Session: "s", ID: "b"}), paneAgent)
	f.open(1, id, paneAgent)
	f.open(1, f.agentID(subagents.Key{Session: "s", ID: "c"}), paneAgent)
	p, _ := f.open(1, id, paneAgent)
	if n := len(f.set(1).panes); n != 3 {
		t.Fatalf("panes %d", n)
	}
	if f.set(1).front() != p {
		t.Fatal("not raised")
	}
}

func TestAgentPaneReplacesOldestNotScratch(t *testing.T) {
	var f floats
	f.open(1, 50, paneScratch)
	var ids []tabs.TabID
	for _, s := range []string{"a", "b", "c", "d"} {
		id := f.agentID(subagents.Key{Session: "s", ID: s})
		ids = append(ids, id)
		_, dropped := f.open(1, id, paneAgent)
		if s == "d" && dropped != ids[0] {
			t.Fatalf("dropped %d, want the oldest agent pane %d", dropped, ids[0])
		}
	}
	if f.set(1).scratch() == nil || len(f.set(1).panes) != maxPanes {
		t.Fatal("scratch shell dropped or set over the cap")
	}
}

func TestAgentPaneLines(t *testing.T) {
	a := &subagents.Agent{Type: "Explore", State: subagents.Done, Dropped: 3, Report: "line one\nline two",
		Steps: []subagents.Step{{At: t0, Tool: "Grep", Summary: "auth"}, {At: t0.Add(61e9), Tool: "Read", Summary: "a.go"}}}
	var got []string
	for _, l := range agentPaneLines(a) {
		got = append(got, l.text)
	}
	want := []string{"3 earlier steps", "09:00:00  Grep  auth", "09:01:01  Read  a.go", "Report", "line one", "line two"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines %q", got)
	}
}

func TestAgentPaneWaitingStrip(t *testing.T) {
	a := &subagents.Agent{State: subagents.Waiting}
	l := agentPaneLines(a)
	if len(l) == 0 || l[0].text != "Waiting for your approval in the session below" || l[0].kind != lineWaiting {
		t.Fatalf("lines %+v", l)
	}
}

func TestAgentPaneFollowsUnlessScrolled(t *testing.T) {
	p := &pane{follow: true}
	p.agentScroll(0, 500) // a frame: following keeps it at the bottom
	if p.scroll != 500 {
		t.Fatalf("scroll %d", p.scroll)
	}
	p.agentScroll(-100, 500) // the owner scrolls up
	if p.scroll != 400 || p.follow {
		t.Fatalf("scroll %d follow %v", p.scroll, p.follow)
	}
	p.agentScroll(0, 700) // a new step: stays put
	if p.scroll != 400 {
		t.Fatalf("moved to %d while scrolled up", p.scroll)
	}
	p.agentScroll(1000, 700) // back to the bottom: follows again
	if p.scroll != 700 || !p.follow {
		t.Fatalf("scroll %d follow %v", p.scroll, p.follow)
	}
	p.agentScroll(0, 900)
	if p.scroll != 900 {
		t.Fatal("not following again")
	}
}

func agentWindow(t *testing.T) (*window, *tabs.Tab, *tabs.Tab, subagents.Key) {
	t.Helper()
	win, _, _ := liveWindow(t, 2)
	a, b := win.mgr.Tabs()[0], win.mgr.Tabs()[1]
	win.onNotify(notify.Msg{Tab: int(b.ID), Kind: notify.KindAgentStart, Session: "s", AgentID: "x", AgentType: "Explore"})
	win.drainSubs(t0)
	return win, a, b, subagents.Key{Session: "s", ID: "x"}
}

func TestOpenAgentPaneSwitchesSession(t *testing.T) {
	win, a, b, k := agentWindow(t)
	if win.mgr.Active().ID != a.ID {
		t.Fatal("setup")
	}
	win.openAgentPane(b.ID, k)
	if win.mgr.Active().ID != b.ID {
		t.Fatal("did not switch to the agent's session")
	}
	p := win.fl.set(b.ID).find(win.fl.agentID(k))
	if p == nil || p.kind != paneAgent || p.agent != k || !p.follow {
		t.Fatalf("pane %+v", p)
	}
	if win.fl.keys != 0 {
		t.Fatal("an agent pane took the keyboard")
	}
}

func TestAgentPaneClosesWithSession(t *testing.T) {
	win, _, b, k := agentWindow(t)
	win.openAgentPane(b.ID, k)
	win.fl.prune(func(id tabs.TabID) bool { return id != b.ID })
	if win.fl.owner(win.fl.agentID(k)) != 0 {
		t.Fatal("agent pane outlived its session")
	}
}

func TestAgentPaneKeptWhileSessionLives(t *testing.T) {
	win, _, b, k := agentWindow(t)
	win.openAgentPane(b.ID, k)
	win.paneStep(win.mgr.Active())
	if win.fl.owner(win.fl.agentID(k)) != b.ID {
		t.Fatal("agent pane pruned although its session lives")
	}
	win.subs.CloseTab(int(b.ID)) // the tracker forgot it
	win.paneStep(win.mgr.Active())
	if win.fl.owner(win.fl.agentID(k)) != 0 {
		t.Fatal("pane kept for a forgotten subagent")
	}
}

func TestCloseAgentPane(t *testing.T) {
	win, _, b, k := agentWindow(t)
	win.openAgentPane(b.ID, k)
	win.closePane(win.fl.agentID(k))
	if win.fl.owner(win.fl.agentID(k)) != 0 {
		t.Fatal("× kept the pane")
	}
}

func TestAgentPaneDrawsInAFrame(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	win.liveMgr.Store(win.mgr)
	for _, m := range []notify.Msg{
		{Tab: int(a.ID), Kind: notify.KindAgentStart, Session: "s", AgentID: "x", AgentType: "Explore"},
		{Tab: int(a.ID), Kind: notify.KindTool, Session: "s", AgentID: "x", Tool: "Grep", Summary: "auth in src"},
		{Tab: int(a.ID), Kind: notify.KindAgentStop, Session: "s", AgentID: "x", Report: strings.Repeat("word ", 400)},
	} {
		win.onNotify(m)
	}
	floatFrames(t, win, r)
	k := subagents.Key{Session: "s", ID: "x"}
	win.openAgentPane(a.ID, k)
	floatFrames(t, win, r)
	p := win.fl.set(a.ID).find(win.fl.agentID(k))
	if p == nil || p.hits.body.Empty() || p.hits.copy.Empty() {
		t.Fatalf("pane not drawn: %+v", p)
	}
	if len(p.wrapped) < 10 || !p.follow || p.scroll == 0 {
		t.Fatalf("report not wrapped or not at the bottom: %d lines, scroll %d", len(p.wrapped), p.scroll)
	}
	if win.fl.keys != 0 {
		t.Fatal("the live pane took the keyboard")
	}
}
