package ui

import (
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

func promptMsg(id int, cmd, args string) notify.Msg {
	return notify.Msg{Tab: id, Kind: notify.KindPrompt, Command: cmd, Args: args}
}

func TestPromptMarksRunningWithCommand(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.deliver(promptMsg(int(id), "/ship-ticket", "V3-3348 now"), t0)
	a := win.agentOf(id)
	if !a.claude || !a.running || a.command != "/ship-ticket" || a.arg != "V3-3348" {
		t.Fatalf("state %+v", a)
	}
}

func TestPlainPromptKeepsTheCommand(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.deliver(promptMsg(int(id), "/ship-ticket", "V3"), t0)
	win.deliver(notify.Msg{Tab: int(id), Kind: notify.KindDone}, t0)
	win.deliver(promptMsg(int(id), "", "fix the bug"), t0)
	a := win.agentOf(id)
	if !a.running || a.command != "/ship-ticket" || a.arg != "V3" {
		t.Fatalf("state %+v", a)
	}
}

func TestStopEndsRunning(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.deliver(promptMsg(int(id), "", ""), t0)
	win.deliver(notify.Msg{Tab: int(id), Kind: notify.KindDone}, t0)
	if a := win.agentOf(id); a.running || !a.claude {
		t.Fatalf("state %+v", a)
	}
	if win.alerts.markOf(id) != markDone {
		t.Fatal("done did not mark")
	}
}

func TestNeedsEndsRunning(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.deliver(promptMsg(int(id), "", ""), t0)
	win.deliver(notify.Msg{Tab: int(id), Kind: notify.KindNeeds}, t0)
	if a := win.agentOf(id); a.running || !a.claude {
		t.Fatalf("state %+v", a)
	}
}

func TestPromptClearsTheMark(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.deliver(notify.Msg{Tab: int(id), Kind: notify.KindNeeds}, t0)
	if win.alerts.markOf(id) != markNeeds || len(win.alerts.list()) != 1 {
		t.Fatal("setup: needs did not mark")
	}
	win.deliver(promptMsg(int(id), "", ""), t0)
	if win.alerts.markOf(id) != markNone || len(win.alerts.list()) != 0 {
		t.Fatalf("mark %v toasts %d", win.alerts.markOf(id), len(win.alerts.list()))
	}
}

func TestPromptDoesNotChime(t *testing.T) {
	win, played, flashes := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.publish(win.mgr.Active(), false)
	win.opt.Config.ChimeDone = true
	win.onNotify(promptMsg(int(id), "/x", "y"))
	if len(*played) != 0 || *flashes != 0 {
		t.Fatalf("%d chimes %d flashes", len(*played), *flashes)
	}
	if win.inbox.size() != 1 {
		t.Fatal("prompt was not queued")
	}
}

func TestUsageMarksClaude(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.liveMgr.Store(win.mgr)
	pct := 10.0
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindUsage, Usage: &notify.Usage{Ctx: &pct}})
	win.drainUsage()
	if a := win.agentOf(id); !a.claude || a.running {
		t.Fatalf("state %+v", a)
	}
}

func TestShellPromptMeansClaudeLeft(t *testing.T) {
	win, _ := alertWindow(t, 2)
	tab := win.mgr.Tabs()[1]
	win.deliver(promptMsg(int(tab.ID), "/ship-ticket", "V3"), t0)
	win.leaveAgents(win.mgr.Tabs())
	if !win.agentOf(tab.ID).claude {
		t.Fatal("left before the shell reported")
	}
	report(t, tab, t.TempDir())
	win.leaveAgents(win.mgr.Tabs())
	a := win.agentOf(tab.ID)
	if a.claude || a.running || a.command != "" || a.arg != "V3" {
		t.Fatalf("state %+v", a)
	}
}

func TestInterruptStaysRunningWithoutATimer(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.deliver(promptMsg(int(id), "", "x"), t0)
	for range 3 {
		win.leaveAgents(win.mgr.Tabs())
		win.pruneAgents(win.mgr.Tabs())
	}
	if !win.agentOf(id).running {
		t.Fatal("running ended without a message")
	}
}

func TestAgentStatePrunedWithTheTab(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.deliver(promptMsg(int(id), "", ""), t0)
	win.pruneAgents(win.mgr.Tabs()[:1])
	if _, ok := win.agents[id]; ok {
		t.Fatal("closed tab kept")
	}
}
