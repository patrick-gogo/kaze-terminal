package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
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
