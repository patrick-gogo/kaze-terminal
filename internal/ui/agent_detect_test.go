package ui

import (
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// fakeProbe stands in for the pty and process lookups and counts them.
type fakeProbe struct {
	fg      map[tabs.TabID]int
	out     map[tabs.TabID]uint64
	names   map[int]string
	fgCalls int
	lookups int
}

func (f *fakeProbe) install(win *window) {
	win.probe = agentProbe{
		fg:   func(t *tabs.Tab) int { f.fgCalls++; return f.fg[t.ID] },
		out:  func(t *tabs.Tab) uint64 { return f.out[t.ID] },
		name: func(pid int) string { f.lookups++; return f.names[pid] },
	}
}

func newProbe() *fakeProbe {
	return &fakeProbe{fg: map[tabs.TabID]int{}, out: map[tabs.TabID]uint64{}, names: map[int]string{}}
}

func rowFor(t *testing.T, win *window, id tabs.TabID) sideRow {
	t.Helper()
	m := buildSide(win.sideIn())
	for _, g := range m.Groups {
		for _, r := range g.Rows {
			if r.ID == id {
				return r
			}
		}
	}
	for _, r := range m.Loose {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no row for tab %d", id)
	return sideRow{}
}

// Codex in front of a tab names its row, with no hook at all.
func TestDetectedAgentNamesTheRow(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	p := newProbe()
	p.install(win)
	p.fg[id], p.names[100], p.out[id] = 100, "codex", 1
	win.detectAgents(t0)
	if r := rowFor(t, win, id); r.Agent != "codex" {
		t.Fatalf("row agent %q", r.Agent)
	}
	if win.agentOf(id).agentName() != "" {
		t.Fatal("detection must not fake hook state")
	}
	// Quitting to the shell clears it on the next check.
	p.fg[id], p.out[id] = 200, 2
	win.detectAgents(t0.Add(time.Second))
	if r := rowFor(t, win, id); r.Agent != "" {
		t.Fatalf("after quit: %q", r.Agent)
	}
}

// A hook's agent wins over the process check.
func TestHookAgentWinsOverDetection(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	p := newProbe()
	p.install(win)
	p.fg[id], p.names[100], p.out[id] = 100, "codex", 1
	win.detectAgents(t0)
	deliverNow(win, notify.Msg{Tab: int(id), Kind: notify.KindPrompt, Command: "/x"}, t0)
	if r := rowFor(t, win, id); r.Agent != "claude" || r.Command != "/x" {
		t.Fatalf("row %+v", r)
	}
}

// Only a tab that printed is checked, at most once a second; a skipped check
// asks for a wake-up, and an idle tab asks for none.
func TestDetectionThrottleAndIdle(t *testing.T) {
	win, _ := alertWindow(t, 1)
	id := win.mgr.Tabs()[0].ID
	p := newProbe()
	p.install(win)
	p.fg[id], p.out[id] = 100, 1
	if wake := win.detectAgents(t0); !wake.IsZero() || p.fgCalls != 1 {
		t.Fatalf("first check: wake %v calls %d", wake, p.fgCalls)
	}
	if wake := win.detectAgents(t0.Add(300 * time.Millisecond)); !wake.IsZero() || p.fgCalls != 1 {
		t.Fatalf("no output since: wake %v calls %d", wake, p.fgCalls)
	}
	p.out[id] = 2
	wake := win.detectAgents(t0.Add(500 * time.Millisecond))
	if p.fgCalls != 1 || !wake.Equal(t0.Add(time.Second)) {
		t.Fatalf("throttled: wake %v calls %d", wake, p.fgCalls)
	}
	if wake := win.detectAgents(t0.Add(time.Second)); !wake.IsZero() || p.fgCalls != 2 {
		t.Fatalf("due: wake %v calls %d", wake, p.fgCalls)
	}
}

// The process is read only when the group in front changes.
func TestDetectionLooksUpEachGroupOnce(t *testing.T) {
	win, _ := alertWindow(t, 1)
	id := win.mgr.Tabs()[0].ID
	p := newProbe()
	p.install(win)
	p.fg[id], p.names[100] = 100, "claude"
	for i := range 5 {
		p.out[id] = uint64(i + 1)
		win.detectAgents(t0.Add(time.Duration(i) * time.Second))
	}
	if p.lookups != 1 || p.fgCalls != 5 {
		t.Fatalf("lookups %d fg calls %d", p.lookups, p.fgCalls)
	}
}

func TestDetectionForgetsClosedTabs(t *testing.T) {
	win, _ := alertWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	p := newProbe()
	p.install(win)
	p.fg[id], p.names[100], p.out[id] = 100, "codex", 1
	win.detectAgents(t0)
	win.mgr.Close(id)
	win.detectAgents(t0.Add(time.Second))
	if _, ok := win.detected[id]; ok {
		t.Fatal("closed tab kept")
	}
}
