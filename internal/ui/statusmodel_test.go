package ui

import (
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func TestStatusModelCounts(t *testing.T) {
	win, _ := sideWindow(t, false)
	win.branchFn = func(string) string { return "main" }
	a := openIn(t, win, osPath("/code/alpha/a"))
	win.mgr.SetActive(a.ID)
	win.lc.counts = map[tabs.TabID]lineCount{a.ID: {add: 12, del: 3}}
	win.liveMgr.Store(win.mgr)
	for _, m := range []notify.Msg{
		{Kind: notify.KindAgentStart, AgentID: "x", AgentType: "X"},
		{Kind: notify.KindAgentStart, AgentID: "y", AgentType: "Y"},
		{Kind: notify.KindNeeds, AgentID: "y"},
	} {
		m.Tab, m.Session = int(a.ID), "s"
		win.onNotify(m)
	}
	win.drainSubs(t0)
	m := win.statusModel(time.Now())
	if m.Agents != 2 || m.Waiting != 1 {
		t.Fatalf("agents %d waiting %d", m.Agents, m.Waiting)
	}
	if m.Branch == "" && m.Folder == "" {
		t.Fatalf("no location: %+v", m)
	}
	if m.Add != 12 || m.Del != 3 {
		t.Fatalf("lines +%d -%d", m.Add, m.Del)
	}
	if m.Tabs != len(win.mgr.Tabs()) {
		t.Fatalf("tabs %d", m.Tabs)
	}
}

func TestStatusModelHidesUnknown(t *testing.T) {
	win, _ := sideWindow(t, false)
	a := openIn(t, win, "")
	win.mgr.SetActive(a.ID)
	win.opt.Config.ShowUsage = false
	m := win.statusModel(time.Now())
	if len(m.Usage) != 0 || m.Branch != "" || m.Add != 0 || m.Del != 0 || m.Update != "" || m.Agents != 0 {
		t.Fatalf("model %+v", m)
	}
}
