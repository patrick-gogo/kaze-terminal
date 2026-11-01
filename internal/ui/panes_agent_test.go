package ui

import (
	"strings"
	"testing"

	"gioui.org/font"

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

func paneTexts(a *subagents.Agent) string {
	var got []string
	for _, l := range agentPaneLines(a) {
		got = append(got, l.text)
	}
	return strings.Join(got, "|")
}

func TestAgentPaneLines(t *testing.T) {
	a := &subagents.Agent{Type: "Explore", State: subagents.Done, Dropped: 3, Report: "line one\nline two",
		Prompt:   "Find auth.\n\nReport file:line.",
		Steps:    []subagents.Step{{At: t0, Tool: "Grep", Summary: "auth"}, {At: t0.Add(61e9), Tool: "Read", Summary: "a.go"}},
		Activity: subagents.Activity{Searches: 13, Commands: 15, Reads: 1}}
	// The prompt and report are one block each; wrapping splits them into lines.
	want := "Prompt|Find auth.\n\nReport file:line.|Searched for 13 patterns, ran 15 shell commands, read 1 file|Report|line one\nline two"
	if got := paneTexts(a); got != want {
		t.Fatalf("lines %q", got)
	}
}

func TestAgentPanePromptKeepsIndentation(t *testing.T) {
	a := &subagents.Agent{State: subagents.Running, Prompt: "\n  Steps:\n\t- one\n    - two  \n\n"}
	if got := agentPaneLines(a)[1].text; got != "  Steps:\n\t- one\n    - two" {
		t.Fatalf("prompt block %q", got)
	}
}

func TestWrapBlockKeepsIndentAndBlankLines(t *testing.T) {
	win, _, _, _, _, _, _ := paneWindow(t)
	gtx := pageContext(1000, 600)
	got := win.wrapBlock(gtx, "a\n\n    b\n\tc", 12, 400)
	if strings.Join(got, "|") != "a||    b|    c" {
		t.Fatalf("lines %q", got)
	}
	// A wrapped indented line keeps its indent on every piece.
	got = win.wrapBlock(gtx, "  "+strings.Repeat("word ", 60), 12, 200)
	if len(got) < 3 {
		t.Fatalf("not wrapped: %q", got)
	}
	for _, l := range got {
		if !strings.HasPrefix(l, "  w") {
			t.Fatalf("indent lost: %q", got)
		}
	}
}

func TestWrapSansBreaksLongWords(t *testing.T) {
	win, _, _, _, _, _, _ := paneWindow(t)
	gtx := pageContext(1000, 600)
	url := "https://example.com/" + strings.Repeat("path/", 56)
	for _, s := range []string{url, "see " + url + " now"} {
		got := win.wrapSans(gtx, s, 12, 440)
		if len(got) < 2 {
			t.Fatalf("not broken: %q", got)
		}
		for _, l := range got {
			if _, sz := win.ui.sans(gtx, l, 12, win.ui.th.Muted, font.Normal); sz.X > 440 {
				t.Fatalf("line %d px wide: %q", sz.X, l)
			}
		}
		if strings.ReplaceAll(strings.Join(got, ""), " ", "") != strings.ReplaceAll(s, " ", "") {
			t.Fatalf("text lost: %q", got)
		}
	}
}

func TestAgentPaneStepReusesPromptAndReportLayout(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	win.liveMgr.Store(win.mgr)
	send := func(m notify.Msg) { m.Tab, m.Session = int(a.ID), "s"; win.onNotify(m) }
	send(notify.Msg{Kind: notify.KindTool, Tool: "Agent", AgentType: "Explore", TaskPrompt: strings.Repeat("look ", 300)})
	send(notify.Msg{Kind: notify.KindAgentStart, AgentID: "x", AgentType: "Explore"})
	floatFrames(t, win, r)
	k := subagents.Key{Session: "s", ID: "x"}
	win.openAgentPane(a.ID, k)
	floatFrames(t, win, r)
	p := win.fl.set(a.ID).find(win.fl.agentID(k))
	if p == nil || len(p.promptWrap.lines) < 3 {
		t.Fatalf("prompt not laid out: %+v", p)
	}
	before := &p.promptWrap.lines[0]
	send(notify.Msg{Kind: notify.KindTool, AgentID: "x", AgentType: "Explore", Tool: "Bash", Summary: "ls"})
	floatFrames(t, win, r)
	if &p.promptWrap.lines[0] != before {
		t.Fatal("a step laid the prompt out again")
	}
	if !strings.Contains(p.wrapped[len(p.wrapped)-1].text, "Ran 1 shell command") {
		t.Fatalf("summary not updated: %+v", p.wrapped[len(p.wrapped)-1])
	}
}

func TestAgentPaneActivityWording(t *testing.T) {
	for _, c := range []struct {
		act  subagents.Activity
		want string
	}{
		{subagents.Activity{}, ""},
		{subagents.Activity{Searches: 1}, "Searched for 1 pattern"},
		{subagents.Activity{Commands: 1, Edits: 2}, "Ran 1 shell command, edited 2 files"},
		{subagents.Activity{Reads: 2, Web: 1, Other: 3}, "Read 2 files, made 1 web request, used 3 other tools"},
		{subagents.Activity{Web: 2, Other: 1}, "Made 2 web requests, used 1 other tool"},
	} {
		if got := activityText(c.act); got != c.want {
			t.Errorf("%+v: got %q want %q", c.act, got, c.want)
		}
	}
}

func TestAgentPaneRunningWithoutStepsOrPrompt(t *testing.T) {
	if got := paneTexts(&subagents.Agent{State: subagents.Running}); got != "Working…" {
		t.Fatalf("lines %q", got)
	}
	a := &subagents.Agent{State: subagents.Running, Activity: subagents.Activity{Commands: 2}}
	if got := paneTexts(a); got != "Ran 2 shell commands so far" {
		t.Fatalf("lines %q", got)
	}
	if got := paneTexts(&subagents.Agent{State: subagents.Done, Report: "ok\n"}); got != "Report|ok" {
		t.Fatalf("lines %q", got)
	}
}

func TestAgentPaneShowsNoToolCalls(t *testing.T) {
	a := &subagents.Agent{State: subagents.Running, Prompt: "p", Dropped: 9,
		Steps: []subagents.Step{{At: t0, Tool: "Bash", Summary: "grep -rn secret"}}, Activity: subagents.Activity{Commands: 10}}
	if got := paneTexts(a); strings.Contains(got, "grep") || strings.Contains(got, "earlier") {
		t.Fatalf("tool calls shown: %q", got)
	}
}

func TestAgentPaneWaitingStrip(t *testing.T) {
	a := &subagents.Agent{State: subagents.Waiting, Prompt: "p"}
	l := agentPaneLines(a)
	if len(l) == 0 || l[len(l)-1].text != "Waiting for your approval in the session below" || l[len(l)-1].kind != lineWaiting {
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
		{Tab: int(a.ID), Kind: notify.KindTool, Session: "s", Tool: "Agent", AgentType: "Explore", Description: "Find auth",
			TaskPrompt: strings.Repeat("look ", 200)},
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
	prompt := 0
	for _, l := range p.wrapped {
		if l.kind == linePrompt {
			prompt++
		}
	}
	if prompt < 3 {
		t.Fatalf("prompt not shown or not wrapped: %d lines", prompt)
	}
	if win.fl.keys != 0 {
		t.Fatal("the live pane took the keyboard")
	}
}
