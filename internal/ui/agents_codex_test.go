package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"testing"
)

func TestCodexInterruptEndsRunningWithoutAlert(t *testing.T) {
	win, played, flashes := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.publish(win.mgr.Active(), false)
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindPrompt, Agent: "codex", Command: "$review", Args: "auth"})
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindInterrupt, Agent: "codex"})
	win.drainAlerts(t0)
	a := win.agentOf(id)
	if a.agentName() != "codex" || a.claude || a.running || a.command != "$review" {
		t.Fatalf("state %+v", a)
	}
	if len(*played) != 0 || *flashes != 0 || win.alerts.markOf(id) != markNone || len(win.alerts.list()) != 0 {
		t.Fatal("interrupt raised alert")
	}
	report(t, win.mgr.Lookup(id), t.TempDir())
	win.leaveAgents(win.mgr.Tabs())
	if win.agentOf(id).agentName() != "" {
		t.Fatal("Codex survived shell prompt")
	}
}

func TestAgentSwitchDoesNotKeepOtherAgentsCommand(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	deliverNow(win, promptMsg(int(id), "/review", "old"), t0)
	deliverNow(win, notify.Msg{Tab: int(id), Kind: notify.KindPrompt, Agent: "codex"}, t0)
	a := win.agentOf(id)
	if a.agentName() != "codex" || a.command != "" || a.arg != "" {
		t.Fatalf("state %+v", a)
	}
	if typeHere(win.mgr.Lookup(id), true, false, a, markNone, false) {
		t.Fatal("Claude skill would be sent to Codex")
	}
}

func TestCodexIdentityInSidebarAndPalette(t *testing.T) {
	r := sideRow{ID: tabs.TabID(1), Title: "api", Agent: "codex", Command: "$review"}
	if !r.matches("codex") || agentCommand(r.Agent, r.Command) != "codex · $review" {
		t.Fatal("missing agent identity")
	}
	items := buildPalette(palInput{Sessions: []sideRow{r}, Query: "codex"})
	if len(items) == 0 || items[0].Agent != "codex" {
		t.Fatalf("palette %+v", items)
	}
	if sessionWord(r) != "idle" {
		t.Fatal("Codex shown as shell")
	}
}

func TestCodexScratchInterruptCountsOnParent(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	parent := win.mgr.Tabs()[1].ID
	scratch, err := win.mgr.NewOpts(tabs.StartOpts{Parent: parent})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []notify.Kind{notify.KindPrompt, notify.KindInterrupt} {
		win.onNotify(notify.Msg{Tab: int(scratch.ID), Kind: kind, Agent: "codex", Command: "$test"})
	}
	win.drainAlerts(t0)
	if a := win.agentOf(parent); a.running || a.agentName() != "codex" {
		t.Fatalf("parent %+v", a)
	}
	if win.agentOf(scratch.ID).agentName() != "" {
		t.Fatal("scratch kept separate agent state")
	}
}

func TestQueuedClaudeUsageCannotGiveCodexContext(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	tab := win.mgr.Tabs()[1]
	pct := 42.0
	win.onNotify(notify.Msg{Tab: int(tab.ID), Kind: notify.KindUsage, Usage: &notify.Usage{Ctx: &pct}})
	win.onNotify(notify.Msg{Tab: int(tab.ID), Kind: notify.KindPrompt, Agent: "codex"})
	win.drainAlerts(t0)
	win.drainUsage()
	if _, ok := win.ctx[tab.ID]; ok {
		t.Fatal("Codex got Claude context")
	}
	if win.agentOf(tab.ID).agentName() != "codex" {
		t.Fatal("usage replaced agent identity")
	}
}

// Issue #63: three Codex helpers show under their session and on the board,
// named from their spawn calls, and a finished one keeps its report.
func TestCodexHelpersShowInTreeAndBoard(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	msgs := []notify.Msg{
		{Kind: notify.KindPrompt},
		{Kind: notify.KindTool, Tool: "Agent", Description: "audit_ui", AgentType: "explorer"},
		{Kind: notify.KindTool, Tool: "Agent", Description: "audit_api", AgentType: "explorer"},
		{Kind: notify.KindTool, Tool: "Agent", Description: "write_docs", AgentType: "default"},
		{Kind: notify.KindAgentStart, AgentID: "t1", AgentType: "explorer"},
		{Kind: notify.KindAgentStart, AgentID: "t2", AgentType: "explorer"},
		{Kind: notify.KindAgentStart, AgentID: "t3", AgentType: "default"},
		{Kind: notify.KindTool, AgentID: "t2", AgentType: "explorer", Tool: "Bash", Summary: "go test ./..."},
		{Kind: notify.KindAgentStop, AgentID: "t3", AgentType: "default", Report: "Docs written."},
	}
	for _, m := range msgs {
		m.Tab, m.Session, m.Agent = int(id), "s", "codex"
		win.onNotify(m)
	}
	win.drainSubs(t0)
	shown, _ := win.subs.Tree(int(id))
	if len(shown) != 3 {
		t.Fatalf("tree has %d helpers", len(shown))
	}
	tasks := map[string]string{}
	for _, a := range shown {
		tasks[a.Key.ID] = a.Task
	}
	if tasks["t1"] != "audit_ui" || tasks["t2"] != "audit_api" || tasks["t3"] != "write_docs" {
		t.Fatalf("tasks %v", tasks)
	}
	running, _, done := win.subs.Board()
	if len(running) != 2 || len(done) != 1 || done[0].Report != "Docs written." {
		t.Fatalf("board running %d done %d", len(running), len(done))
	}
	if st := win.subs.Get(subagents.Key{Session: "s", ID: "t2"}).Steps; len(st) != 1 || st[0].Summary != "go test ./..." {
		t.Fatalf("t2 steps %+v", st)
	}
}
