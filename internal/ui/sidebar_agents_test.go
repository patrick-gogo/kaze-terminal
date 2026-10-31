package ui

import (
	"image"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func TestBuildSideAddsAgents(t *testing.T) {
	ts, locs := sideFixture()
	in := sideIn(ts, locs)
	in.Agents = func(id tabs.TabID) ([]sideAgent, int, int) {
		if id == 1 {
			return []sideAgent{{Type: "Explore", Task: "Find the auth middleware"}}, 2, 3
		}
		if id == 4 {
			return nil, 0, 1
		}
		return nil, 0, 0
	}
	m := buildSide(in)
	g := m.Groups[0]
	r := g.Rows[0]
	if r.ID != 1 || len(r.Agents) != 1 || r.MoreAgents != 2 || r.AgentCount != 3 {
		t.Fatalf("row %+v", r)
	}
	if g.AgentCount != 4 {
		t.Fatalf("group count %d", g.AgentCount)
	}
}

func TestFilterMatchesAgentTask(t *testing.T) {
	r := sideRow{Title: "x", Agents: []sideAgent{{Type: "Explore", Task: "Find the auth Middleware"}}}
	if !r.matches("middleware") || !r.matches("explore") || r.matches("nothing") {
		t.Fatal("filter")
	}
}

func TestElapsedText(t *testing.T) {
	a := &subagents.Agent{Started: t0, State: subagents.Running}
	for _, c := range []struct {
		d    time.Duration
		want string
	}{{30 * time.Second, "<1m"}, {4*time.Minute + 10*time.Second, "4m"}, {75 * time.Minute, "75m"}} {
		if got := agentElapsed(a, t0.Add(c.d)); got != c.want {
			t.Errorf("%v: %q want %q", c.d, got, c.want)
		}
	}
	a.State, a.Ended = subagents.Done, t0.Add(6*time.Minute+5*time.Second)
	if got := agentElapsed(a, t0.Add(time.Hour)); got != "took 6m" {
		t.Errorf("done: %q", got)
	}
	a.Ended = t0.Add(20 * time.Second)
	if got := agentElapsed(a, t0.Add(time.Hour)); got != "took <1m" {
		t.Errorf("quick: %q", got)
	}
}

func TestAgentChipText(t *testing.T) {
	if agentChip(0) != "" || agentChip(1) != "1 agent" || agentChip(3) != "3 agents" {
		t.Fatal("chip text")
	}
}

func TestSideAgentFromTracker(t *testing.T) {
	a := &subagents.Agent{Key: subagents.Key{Session: "s", ID: "a"}, Type: "Explore", Task: "Find", Started: t0, Depth: 1,
		Steps: []subagents.Step{{Tool: "Grep", Summary: "auth in src"}}}
	s := sideAgentOf(a, t0.Add(2*time.Minute))
	if s.Key != a.Key || s.Action != "Grep auth in src" || s.Elapsed != "2m" || s.Depth != 1 || s.Type != "Explore" {
		t.Fatalf("%+v", s)
	}
	a.Steps = nil
	if s := sideAgentOf(a, t0); s.Action != "Starting" {
		t.Fatalf("no steps: %q", s.Action)
	}
	a.State, a.Report = subagents.Done, "Found it in auth.ts\nmore"
	if s := sideAgentOf(a, t0); s.Action != "Found it in auth.ts" {
		t.Fatalf("done: %q", s.Action)
	}
}

func TestSidebarDrawsAgentLines(t *testing.T) {
	rows := sideRows(1, 2, "s")
	rows[0].Agents = []sideAgent{
		{Key: subagents.Key{ID: "a1"}, Type: "Explore", Task: "Find the auth middleware", Action: "Grep auth", Elapsed: "4m"},
		{Key: subagents.Key{ID: "a2"}, Type: "Plan", Task: "Plan it", Depth: 1, Elapsed: "<1m", State: subagents.Waiting},
	}
	rows[0].MoreAgents, rows[0].AgentCount = 3, 5
	m := sideModel{Loose: rows}
	h, size := sideDraw(t, m, sideState{}, 700)
	if len(h.agents) != 3 {
		t.Fatalf("agent hits %d, want 2 lines and a more line", len(h.agents))
	}
	first, second := h.rows[0].rect, h.rows[1].rect
	for i, a := range h.agents {
		if a.tab != 1 || a.rect.Min.Y < first.Max.Y || a.rect.Max.Y > second.Min.Y || a.rect.Max.X > size.X {
			t.Fatalf("hit %d %+v between %v and %v", i, a, first, second)
		}
	}
	if h.agents[0].key.ID != "a1" || h.agents[1].key.ID != "a2" || !h.agents[2].more {
		t.Fatalf("hits %+v", h.agents)
	}
	if h.agents[1].rect.Min.Y < h.agents[0].rect.Max.Y {
		t.Fatal("agent lines overlap")
	}
}

func TestRevealScrollCountsAgentLines(t *testing.T) {
	rows := sideRows(1, 6, "s")
	for i := range rows[:5] {
		rows[i].Agents = []sideAgent{{Type: "X"}, {Type: "Y"}}
	}
	m := sideModel{Loose: rows}
	gtx := pageContext(sidebarW, 300)
	st := sideState{shown: 6}
	h := newChrome().layoutSidebar(gtx, m, st, image.Pt(sidebarW, 300))
	sc, ok := revealScroll(gtx, m, st, h)
	if !ok {
		t.Fatal("not found")
	}
	st.scroll = sc
	h = newChrome().layoutSidebar(gtx, m, st, image.Pt(sidebarW, 300))
	for _, r := range h.rows {
		if r.id == 6 && r.rect.Dy() == gtx.Dp(sideRowH) {
			return
		}
	}
	t.Fatalf("row 6 not shown whole at scroll %d", sc)
}
