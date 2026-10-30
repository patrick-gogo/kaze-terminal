package ui

import (
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/vt"
)

func promptMsg(id int, cmd, args string) notify.Msg {
	return notify.Msg{Tab: id, Kind: notify.KindPrompt, Command: cmd, Args: args}
}

func TestPromptMarksRunningWithCommand(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	deliverNow(win, promptMsg(int(id), "/ship-ticket", "V3-3348 now"), t0)
	a := win.agentOf(id)
	if !a.claude || !a.running || a.command != "/ship-ticket" || a.arg != "V3-3348" {
		t.Fatalf("state %+v", a)
	}
}

func TestPlainPromptKeepsTheCommand(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	deliverNow(win, promptMsg(int(id), "/ship-ticket", "V3"), t0)
	deliverNow(win, notify.Msg{Tab: int(id), Kind: notify.KindDone}, t0)
	deliverNow(win, promptMsg(int(id), "", "fix the bug"), t0)
	a := win.agentOf(id)
	if !a.running || a.command != "/ship-ticket" || a.arg != "V3" {
		t.Fatalf("state %+v", a)
	}
}

func TestStopEndsRunning(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	deliverNow(win, promptMsg(int(id), "", ""), t0)
	deliverNow(win, notify.Msg{Tab: int(id), Kind: notify.KindDone}, t0)
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
	deliverNow(win, promptMsg(int(id), "", ""), t0)
	deliverNow(win, notify.Msg{Tab: int(id), Kind: notify.KindNeeds}, t0)
	if a := win.agentOf(id); a.running || !a.claude {
		t.Fatalf("state %+v", a)
	}
}

func TestPromptClearsTheMark(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	deliverNow(win, notify.Msg{Tab: int(id), Kind: notify.KindNeeds}, t0)
	if win.alerts.markOf(id) != markNeeds || len(win.alerts.list()) != 1 {
		t.Fatal("setup: needs did not mark")
	}
	deliverNow(win, promptMsg(int(id), "", ""), t0)
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
	deliverNow(win, promptMsg(int(tab.ID), "/ship-ticket", "V3"), t0)
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
	deliverNow(win, promptMsg(int(id), "", "x"), t0)
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
	deliverNow(win, promptMsg(int(id), "", ""), t0)
	win.pruneAgents(win.mgr.Tabs()[:1])
	if _, ok := win.agents[id]; ok {
		t.Fatal("closed tab kept")
	}
}

// promptMark feeds an OSC 133 A, the prompt-start mark of shell integration.
func promptMark(t *testing.T, tab *tabs.Tab) {
	t.Helper()
	s := tab.Session
	s.Lock()
	vt.NewParser(s.Applier()).Feed([]byte("\x1b]133;A\x07"))
	s.Unlock()
}

// Hook state is stamped when the message arrives, and Claude's exit is checked
// after the frame's drains, so a shell report that lands in the same batch counts.
func TestPromptDoneAndShellReportInOneFrameLeaveClaude(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	win.opt.Config = config.Default()
	win.cfgPath = filepath.Join(t.TempDir(), "config.toml")
	var r input.Router
	sizedFrame(t, win, &r, 800, 600)
	tab := win.mgr.Tabs()[1]
	win.onNotify(promptMsg(int(tab.ID), "/ship-ticket", "V3"))
	win.onNotify(notify.Msg{Tab: int(tab.ID), Kind: notify.KindDone})
	report(t, tab, t.TempDir())
	sizedFrame(t, win, &r, 800, 600)
	if a := win.agentOf(tab.ID); a.claude || a.running {
		t.Fatalf("state %+v, want Claude left", a)
	}
}

// A shell that sends only OSC 133 A still ends Claude, even when an earlier
// OSC 7 put its folder count ahead of its prompt count.
func TestPromptMarkAloneMeansClaudeLeft(t *testing.T) {
	win, _ := alertWindow(t, 2)
	tab := win.mgr.Tabs()[1]
	report(t, tab, t.TempDir())
	deliverNow(win, promptMsg(int(tab.ID), "/ship-ticket", "V3"), t0)
	win.leaveAgents(win.mgr.Tabs())
	if !win.agentOf(tab.ID).claude {
		t.Fatal("left before the shell reported")
	}
	promptMark(t, tab)
	win.leaveAgents(win.mgr.Tabs())
	if a := win.agentOf(tab.ID); a.claude || a.running {
		t.Fatalf("state %+v, want Claude left", a)
	}
}

// The inbox keeps a slash command even when a done follows before a frame.
func TestPromptThenDoneInOneFrameKeepsTheCommand(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.onNotify(promptMsg(int(id), "/ship-ticket", "V3-3348"))
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindDone})
	win.drainAlerts(t0)
	if a := win.agentOf(id); a.running || a.command != "/ship-ticket" || a.arg != "V3-3348" {
		t.Fatalf("state %+v", a)
	}
}

// needs, prompt, done in one batch: the prompt answered the needs, so the row ends done.
func TestNeedsPromptDoneInOneFrameEndsDone(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindNeeds})
	win.onNotify(promptMsg(int(id), "/ship-ticket", "V3"))
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindDone})
	win.drainAlerts(t0)
	if m := win.alerts.markOf(id); m != markDone {
		t.Fatalf("mark %v, want done", m)
	}
	if a := win.agentOf(id); a.running || a.command != "/ship-ticket" {
		t.Fatalf("state %+v", a)
	}
}

// deliverNow applies m as a message that arrived just now.
func deliverNow(win *window, m notify.Msg, now time.Time) {
	var shell uint64
	if t := win.mgr.Lookup(tabs.TabID(m.Tab)); t != nil {
		shell = shellSeq(t)
	}
	win.deliver(queued{m: m, shell: shell}, now)
}

// A plain prompt after a slash command in one batch keeps the command, and
// still answers a needs mark left from an earlier frame.
func TestCommandThenPlainPromptThenDoneInOneFrame(t *testing.T) {
	win, _, _ := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindNeeds})
	win.drainAlerts(t0)
	win.onNotify(promptMsg(int(id), "/ship-ticket", "V3"))
	win.onNotify(promptMsg(int(id), "", "and the tests"))
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindDone})
	win.drainAlerts(t0)
	if m := win.alerts.markOf(id); m != markDone {
		t.Fatalf("mark %v, want done", m)
	}
	if a := win.agentOf(id); a.running || a.command != "/ship-ticket" || a.arg != "V3" {
		t.Fatalf("state %+v", a)
	}
}
