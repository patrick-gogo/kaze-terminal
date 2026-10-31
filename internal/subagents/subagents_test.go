package subagents

import (
	"fmt"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)

func at(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }

// ev is an event in tab 1, session s1, from tab 1.
func ev(k EventKind, id, typ string) Event {
	return Event{Kind: k, Tab: 1, Src: 1, Session: "s1", AgentID: id, AgentType: typ, At: t0}
}

func agentCall(desc, typ string) Event { e := ev(EvAgentCall, "", typ); e.Desc = desc; return e }

func tool(id, name string) Event { e := ev(EvTool, id, ""); e.Tool = name; return e }

func treeIDs(tr *Tracker, tab int) []string {
	shown, _ := tr.Tree(tab)
	var ids []string
	for _, a := range shown {
		ids = append(ids, a.Key.ID)
	}
	return ids
}

func TestPairsCallsInOrder(t *testing.T) {
	var tr Tracker
	tr.Apply(agentCall("A", "Explore"))
	tr.Apply(agentCall("B", "Explore"))
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.Apply(ev(EvStart, "a2", "Explore"))
	if a, b := tr.Get(Key{"s1", "a1"}), tr.Get(Key{"s1", "a2"}); a.Task != "A" || b.Task != "B" {
		t.Fatalf("tasks %q %q", a.Task, b.Task)
	}
}

func TestPairingMatchesType(t *testing.T) {
	var tr Tracker
	tr.Apply(agentCall("plan it", "Plan"))
	tr.Apply(agentCall("look", "Explore"))
	tr.Apply(ev(EvStart, "a1", "Explore"))
	if a := tr.Get(Key{"s1", "a1"}); a.Task != "look" {
		t.Fatalf("task %q", a.Task)
	}
}

func TestStartWithoutCallUsesType(t *testing.T) {
	var tr Tracker
	if !tr.Apply(ev(EvStart, "a1", "Explore")) {
		t.Fatal("start changed nothing")
	}
	if a := tr.Get(Key{"s1", "a1"}); a.Task != "Explore" || a.State != Running || a.Tab != 1 {
		t.Fatalf("%+v", a)
	}
}

func TestNestedParentAndDepth(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "general-purpose"))
	c := agentCall("child", "Explore")
	c.AgentID = "a1"
	tr.Apply(c)
	tr.Apply(ev(EvStart, "a2", "Explore"))
	g := agentCall("grandchild", "Explore")
	g.AgentID = "a2"
	tr.Apply(g)
	tr.Apply(ev(EvStart, "a3", "Explore"))
	a2, a3 := tr.Get(Key{"s1", "a2"}), tr.Get(Key{"s1", "a3"})
	if a2.Parent != (Key{"s1", "a1"}) || a2.Depth != 1 || a2.Task != "child" {
		t.Fatalf("a2 %+v", a2)
	}
	if a3.Parent != (Key{"s1", "a2"}) || a3.Depth != 1 {
		t.Fatalf("a3 %+v", a3)
	}
	if got := fmt.Sprint(treeIDs(&tr, 1)); got != "[a1 a2 a3]" {
		t.Fatalf("tree %s", got)
	}
}

func TestTreeKeepsChildrenUnderParents(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "X"))
	tr.Apply(ev(EvStart, "b1", "X"))
	c := agentCall("child", "Y")
	c.AgentID = "a1"
	tr.Apply(c)
	tr.Apply(ev(EvStart, "a2", "Y"))
	if got := fmt.Sprint(treeIDs(&tr, 1)); got != "[a1 a2 b1]" {
		t.Fatalf("tree %s", got)
	}
}

func TestStepsCapAt200(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	for i := 1; i <= 205; i++ {
		e := tool("a1", fmt.Sprint("T", i))
		e.At = at(i)
		tr.Apply(e)
	}
	a := tr.Get(Key{"s1", "a1"})
	if len(a.Steps) != 200 || a.Dropped != 5 || a.Steps[199].Tool != "T205" || a.Steps[0].Tool != "T6" {
		t.Fatalf("steps %d dropped %d", len(a.Steps), a.Dropped)
	}
}

func TestNeedsWithAgentID(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.Apply(ev(EvNeeds, "a1", ""))
	if a := tr.Get(Key{"s1", "a1"}); a.State != Waiting {
		t.Fatalf("state %v", a.State)
	}
	if w := tr.SessionWaits(); len(w) != 0 {
		t.Fatalf("session waits %v", w)
	}
	tr.Apply(tool("a1", "Bash"))
	if a := tr.Get(Key{"s1", "a1"}); a.State != Running {
		t.Fatalf("state %v after a step", a.State)
	}
}

func TestNeedsWithoutAgentID(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvNeeds, "", "")) // nothing running: not a subagent wait
	if w := tr.SessionWaits(); len(w) != 0 {
		t.Fatalf("waits with nothing running %v", w)
	}
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.Apply(ev(EvNeeds, "", ""))
	if a := tr.Get(Key{"s1", "a1"}); a.State != Running {
		t.Fatalf("an unnamed wait marked the agent: %v", a.State)
	}
	if w := tr.SessionWaits(); fmt.Sprint(w) != "[1]" {
		t.Fatalf("session waits %v", w)
	}
	tr.Apply(ev(EvPrompt, "", ""))
	if w := tr.SessionWaits(); len(w) != 0 {
		t.Fatalf("prompt kept the wait %v", w)
	}
}

func TestStopSetsReport(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	s := ev(EvStop, "a1", "Explore")
	s.Report, s.At = "found it", at(360)
	tr.Apply(s)
	a := tr.Get(Key{"s1", "a1"})
	if a.State != Done || a.Report != "found it" || !a.Ended.Equal(at(360)) {
		t.Fatalf("%+v", a)
	}
}

func TestPromptHidesFinishedFromTree(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.Apply(ev(EvStart, "a2", "Explore"))
	tr.Apply(ev(EvStop, "a1", "Explore"))
	tr.Apply(ev(EvPrompt, "", ""))
	if got := fmt.Sprint(treeIDs(&tr, 1)); got != "[a2]" {
		t.Fatalf("tree %s", got)
	}
	if _, _, done := tr.Board(); len(done) != 1 || done[0].Key.ID != "a1" {
		t.Fatalf("done %v", done)
	}
}

func TestNewSessionResets(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvSession, "", ""))
	tr.Apply(agentCall("queued", "Plan"))
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.Apply(ev(EvStart, "a2", "Explore"))
	tr.Apply(ev(EvStop, "a2", "Explore"))
	n := ev(EvSession, "", "")
	n.Session = "s2"
	if !tr.Apply(n) {
		t.Fatal("a new session changed nothing")
	}
	run, wait, done := tr.Board()
	if len(run)+len(wait) != 0 || len(done) != 1 || len(treeIDs(&tr, 1)) != 0 {
		t.Fatalf("run %v wait %v done %v", run, wait, done)
	}
	s := ev(EvStart, "b1", "Plan")
	s.Session = "s2"
	tr.Apply(s)
	if a := tr.Get(Key{"s2", "b1"}); a.Task != "Plan" {
		t.Fatalf("old queue survived: %q", a.Task)
	}
	if tr.Apply(n) {
		t.Fatal("the same session again counted as new")
	}
}

func TestScratchSessionKeptApart(t *testing.T) {
	var tr Tracker
	main := ev(EvSession, "", "")
	tr.Apply(main)
	tr.Apply(ev(EvStart, "a1", "Explore"))
	scratch := Event{Kind: EvSession, Tab: 1, Src: 9, Session: "x1", At: t0}
	tr.Apply(scratch)
	tr.Apply(Event{Kind: EvStart, Tab: 1, Src: 9, Session: "x1", AgentID: "b1", AgentType: "Plan", At: t0})
	scratch.Session = "x2"
	tr.Apply(scratch)
	if tr.Get(Key{"s1", "a1"}) == nil || tr.Get(Key{"x1", "b1"}) != nil {
		t.Fatal("the scratch shell's new session must reset only its own agents")
	}
	tr.Apply(main) // the main session again: not new
	if tr.Get(Key{"s1", "a1"}) == nil {
		t.Fatal("main reset by the scratch shell's session")
	}
}

func TestBackgroundAgentOutlivesStop(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.Apply(ev(EvPrompt, "", "")) // the next turn starts while it runs
	if tr.Active(1) != 1 || len(treeIDs(&tr, 1)) != 1 {
		t.Fatal("a running background agent left the tree")
	}
	tr.Apply(ev(EvStop, "a1", "Explore"))
	if tr.Active(1) != 0 || tr.Active(0) != 0 {
		t.Fatal("still active after its stop")
	}
}

func TestUnknownAgentIsCreated(t *testing.T) {
	var tr Tracker
	e := tool("late", "Read")
	e.AgentType = "Explore"
	tr.Apply(e)
	if a := tr.Get(Key{"s1", "late"}); a == nil || a.Task != "Explore" || len(a.Steps) != 1 {
		t.Fatalf("%+v", a)
	}
}

func TestTreeLimit(t *testing.T) {
	var tr Tracker
	for i := 0; i < 8; i++ {
		tr.Apply(ev(EvStart, fmt.Sprint("a", i), "Explore"))
	}
	shown, more := tr.Tree(1)
	if len(shown) != 6 || more != 2 {
		t.Fatalf("shown %d more %d", len(shown), more)
	}
}

func TestDoneCap(t *testing.T) {
	var tr Tracker
	for i := 0; i < 55; i++ {
		id := fmt.Sprint("a", i)
		tr.Apply(ev(EvStart, id, "Explore"))
		s := ev(EvStop, id, "Explore")
		s.At = at(i)
		tr.Apply(s)
	}
	_, _, done := tr.Board()
	if len(done) != 50 || done[0].Key.ID != "a54" || done[49].Key.ID != "a5" {
		t.Fatalf("done %d first %s", len(done), done[0].Key.ID)
	}
	if tr.Get(Key{"s1", "a0"}) != nil {
		t.Fatal("an agent past the done cap is still kept")
	}
}

func TestQueueCap(t *testing.T) {
	var tr Tracker
	for i := 0; i < 20; i++ {
		tr.Apply(agentCall(fmt.Sprint("d", i), "Explore"))
	}
	tr.Apply(ev(EvStart, "a1", "Explore"))
	if a := tr.Get(Key{"s1", "a1"}); a.Task != "d4" {
		t.Fatalf("task %q, want the oldest kept call d4", a.Task)
	}
}

func TestCloseTab(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.Apply(Event{Kind: EvStart, Tab: 2, Src: 2, Session: "s2", AgentID: "b1", AgentType: "X", At: t0})
	tr.Apply(Event{Kind: EvStart, Tab: 2, Src: 5, Session: "s5", AgentID: "c1", AgentType: "X", At: t0})
	tr.Apply(ev(EvStop, "a1", "Explore"))
	tr.CloseTab(1)
	if tr.Get(Key{"s1", "a1"}) != nil || tr.Active(0) != 2 {
		t.Fatal("tab 1 not closed")
	}
	tr.CloseTab(5) // the scratch shell closes alone
	if tr.Get(Key{"s5", "c1"}) != nil || tr.Get(Key{"s2", "b1"}) == nil {
		t.Fatal("closing a source tab")
	}
}

func TestBoardColumns(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "X"))
	tr.Apply(ev(EvStart, "a2", "X"))
	tr.Apply(ev(EvStart, "a3", "X"))
	tr.Apply(ev(EvNeeds, "a2", ""))
	tr.Apply(ev(EvStop, "a3", "X"))
	run, wait, done := tr.Board()
	if len(run) != 1 || run[0].Key.ID != "a1" || len(wait) != 1 || wait[0].Key.ID != "a2" || len(done) != 1 {
		t.Fatalf("run %d wait %d done %d", len(run), len(wait), len(done))
	}
}
