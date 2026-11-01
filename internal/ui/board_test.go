package ui

import (
	"image"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

var boardRect = image.Rect(260, 0, 1200, 760)

func TestAgentsBoardDefaultKeys(t *testing.T) {
	mac := defaultBindings("darwin")["agents_board"]
	win := defaultBindings("windows")["agents_board"]
	if mac != (input.Shortcut{Name: "A", Mods: key.ModCommand | key.ModShift}) || win != (input.Shortcut{Name: "A", Mods: key.ModCtrl | key.ModShift}) {
		t.Fatalf("mac %+v windows %+v", mac, win)
	}
	if actionByName["agents_board"] != actAgentsBoard {
		t.Fatal("agents_board not bound to the board action")
	}
}

func TestPaletteListsShowAgents(t *testing.T) {
	if !slices.ContainsFunc(kazeCommands, func(c kazeCommand) bool {
		return c.Config == "agents_board" && c.Label == "Show agents" && c.Act == actAgentsBoard
	}) {
		t.Fatal("no Show agents command")
	}
}

// boardWindow has tab 2 with a running, a waiting and a done subagent.
func boardWindow(t *testing.T) (*window, tabs.TabID) {
	t.Helper()
	win, _, _ := liveWindow(t, 2)
	id := win.mgr.Tabs()[1].ID
	for _, m := range []notify.Msg{
		{Kind: notify.KindAgentStart, AgentID: "run", AgentType: "Explore"},
		{Kind: notify.KindAgentStart, AgentID: "wait", AgentType: "Plan"},
		{Kind: notify.KindNeeds, AgentID: "wait"},
		{Kind: notify.KindAgentStart, AgentID: "done", AgentType: "general-purpose"},
		{Kind: notify.KindAgentStop, AgentID: "done", Report: "All good\nmore"},
	} {
		m.Tab, m.Session = int(id), "s"
		win.onNotify(m)
	}
	win.drainSubs(t0)
	return win, id
}

func TestBoardColumns(t *testing.T) {
	win, id := boardWindow(t)
	cols := win.boardColumns()
	ids := func(cs []boardCard) (out []string) {
		for _, c := range cs {
			out = append(out, c.key.ID)
		}
		return
	}
	if strings.Join(ids(cols[0]), ",") != "run" || strings.Join(ids(cols[1]), ",") != "wait" || strings.Join(ids(cols[2]), ",") != "done" {
		t.Fatalf("columns %v %v %v", ids(cols[0]), ids(cols[1]), ids(cols[2]))
	}
	if cols[2][0].line != "All good" || cols[2][0].tab != id {
		t.Fatalf("done card %+v", cols[2][0])
	}
	// A wait with no subagent named shows the session in Waiting.
	win.onNotify(notify.Msg{Tab: int(id), Kind: notify.KindNeeds, Session: "s"})
	win.drainSubs(t0)
	cols = win.boardColumns()
	if len(cols[1]) != 2 || !cols[1][1].session || cols[1][1].line != "a subagent may be asking" {
		t.Fatalf("waiting %+v", cols[1])
	}
}

func TestBoardFilteredToTab(t *testing.T) {
	win, id := boardWindow(t)
	other := win.mgr.Tabs()[0].ID
	win.onNotify(notify.Msg{Tab: int(other), Kind: notify.KindAgentStart, Session: "o", AgentID: "x", AgentType: "X"})
	win.drainSubs(t0)
	win.openBoard(id)
	if n := len(win.boardColumns()[0]); n != 1 {
		t.Fatalf("filtered running %d", n)
	}
	win.closeBoard()
	win.openBoard(0)
	if n := len(win.boardColumns()[0]); n != 2 {
		t.Fatalf("all running %d", n)
	}
}

func TestBoardCardOpensPane(t *testing.T) {
	win, id := boardWindow(t)
	win.openBoard(0)
	win.drawBoard(pageContext(1200, 760), boardRect)
	i := slices.IndexFunc(win.board.cards, func(h boardHit) bool { return h.card.key.ID == "run" })
	if i < 0 {
		t.Fatal("no card hit")
	}
	r := win.board.cards[i].rect
	win.boardPress(r.Min.Add(r.Size().Div(2)))
	if win.board.open || win.mgr.Active().ID != id {
		t.Fatal("board still open or wrong session")
	}
	if p := win.fl.set(id).find(win.fl.agentID(subagents.Key{Session: "s", ID: "run"})); p == nil {
		t.Fatal("no live pane")
	}
}

func TestBoardEscAndKeyClose(t *testing.T) {
	win, id := boardWindow(t)
	start := win.mgr.Active().ID
	win.runShortcut(pageContext(800, 600), actAgentsBoard)
	if !win.board.open || win.shownTab() != nil {
		t.Fatal("key did not open the board over the session")
	}
	win.runShortcut(pageContext(800, 600), actAgentsBoard)
	if win.board.open || win.mgr.Active().ID != start {
		t.Fatal("key did not close it back to the session")
	}
	win.openBoard(0)
	win.boardKey(key.Event{Name: key.NameEscape, State: key.Press})
	if win.board.open {
		t.Fatal("Esc did not close")
	}
	win.openBoard(id)
	win.drawBoard(pageContext(1200, 760), boardRect)
	r := win.board.back
	if r.Empty() {
		t.Fatal("no back button")
	}
	win.boardPress(r.Min.Add(r.Size().Div(2)))
	if win.board.open || win.mgr.Active().ID != start {
		t.Fatal("back did not return to the session it came from")
	}
}

func TestBoardExclusiveWithSettings(t *testing.T) {
	win, _ := boardWindow(t)
	win.openBoard(0)
	win.openSettings()
	if win.board.open {
		t.Fatal("settings left the board open")
	}
	win.openBoard(0)
	if win.set.shown {
		t.Fatal("the board left settings in front")
	}
	win.selectTab(win.mgr.Tabs()[0].ID)
	if win.board.open {
		t.Fatal("picking a session left the board open")
	}
}

func TestBoardHooksMissingHint(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	win.closeSettings()
	// Take the tool hook out.
	data := strings.Replace(readFile(t, f.settings), "notify tool", "notify nothing", 1)
	if err := os.WriteFile(f.settings, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	win.openBoard(0)
	win.drawBoard(pageContext(1200, 760), boardRect)
	if !slices.Contains(win.board.texts, "kaze needs three more Claude Code hooks to see subagents.") || win.board.setup.Empty() {
		t.Fatalf("no hint in %q", win.board.texts)
	}
	r := win.board.setup
	win.boardPress(r.Min.Add(r.Size().Div(2)))
	if !win.set.shown || win.sections()[win.set.section].title != "Agents" {
		t.Fatal("the hint's button did not open Settings, Agents")
	}
}

func TestStatusAgentsText(t *testing.T) {
	if agentsRunning(0) != "" || agentsRunning(1) != "1 agent running" || agentsRunning(4) != "4 agents running" {
		t.Fatal("status text")
	}
	win, _ := boardWindow(t)
	if win.statusModel(time.Now()).Agents != 2 {
		t.Fatalf("status count %d", win.statusModel(time.Now()).Agents)
	}
}

func TestMoreAgentsOpensFilteredBoard(t *testing.T) {
	win, id := boardWindow(t)
	win.side.hits.agents = []sideAgentHit{{tab: id, more: true, rect: image.Rect(10, 10, 100, 30)}}
	win.sidePress(image.Pt(20, 20), 1, 0)
	if !win.board.open || win.board.tab != id {
		t.Fatalf("board %+v", win.board)
	}
}

func TestCloseTabKeyClosesTheBoardNotTheSession(t *testing.T) {
	win, _ := boardWindow(t)
	n := len(win.mgr.Tabs())
	win.openBoard(0)
	win.runShortcut(pageContext(800, 600), actCloseTab)
	if win.board.open || len(win.mgr.Tabs()) != n {
		t.Fatalf("board open %v, tabs %d -> %d", win.board.open, n, len(win.mgr.Tabs()))
	}
}

func TestAgentLineUnderActiveRowClosesTheBoard(t *testing.T) {
	win, id := boardWindow(t)
	win.mgr.SetActive(id)
	win.openBoard(0)
	win.openAgentPane(id, subagents.Key{Session: "s", ID: "run"})
	if win.board.open {
		t.Fatal("the pane opened behind the board")
	}
}

func TestNewTabClosesTheBoard(t *testing.T) {
	win, _ := boardWindow(t)
	win.openBoard(0)
	win.newTab()
	if win.board.open {
		t.Fatal("new tab opened behind the board")
	}
}

func TestClaudeLeavingEndsItsSubagents(t *testing.T) {
	win, id := boardWindow(t)
	tab := win.mgr.Lookup(id)
	deliverNow(win, promptMsg(int(id), "", ""), t0) // Claude is running in the tab
	promptMark(t, tab)                              // the shell prompt is back: Claude exited
	win.leaveAgents(win.mgr.Tabs())
	if win.subs.Active(0) != 0 {
		t.Fatalf("%d subagents still running after Claude left", win.subs.Active(0))
	}
	if _, ok := win.subTimer(t0); ok {
		t.Fatal("the minute timer outlived Claude")
	}
}

func TestBoardNamesSessionsLikeTheSidebar(t *testing.T) {
	win, id := boardWindow(t)
	deliverNow(win, promptMsg(int(id), "/ship-ticket", "TODO-142 login"), t0)
	title, _ := win.sessionName(id)
	if want := win.sideIn().label(win.mgr.Lookup(id), win.locOf(win.mgr.Lookup(id)), win.agentOf(id)); title != want || title != "TODO-142" {
		t.Fatalf("board says %q, sidebar says %q", title, want)
	}
}

// A done card's × removes it and keeps the board open; running cards have none.
func TestBoardDismissDoneCard(t *testing.T) {
	win, _ := boardWindow(t)
	win.openBoard(0)
	win.drawBoard(pageContext(1200, 760), boardRect)
	var x image.Rectangle
	for _, h := range win.board.cards {
		if h.card.key.ID == "run" && !h.close.Empty() {
			t.Fatal("a running card has a close button")
		}
		if h.card.key.ID == "done" {
			x = h.close
		}
	}
	if x.Empty() {
		t.Fatal("the done card has no close button")
	}
	win.boardPress(x.Min.Add(x.Size().Div(2)))
	if !win.board.open {
		t.Fatal("closing a card closed the board")
	}
	if win.subs.Get(subagents.Key{Session: "s", ID: "done"}) != nil {
		t.Fatal("card not removed")
	}
	if len(win.fl.sets) != 0 {
		for _, s := range win.fl.sets {
			if len(s.panes) != 0 {
				t.Fatal("dismissing opened a pane")
			}
		}
	}
}

// Clear empties the Done column; running and waiting cards stay.
func TestBoardClearDone(t *testing.T) {
	win, _ := boardWindow(t)
	win.openBoard(0)
	win.drawBoard(pageContext(1200, 760), boardRect)
	c := win.board.clear
	if c.Empty() {
		t.Fatal("no Clear button")
	}
	win.boardPress(c.Min.Add(c.Size().Div(2)))
	cols := win.boardColumns()
	if len(cols[2]) != 0 || len(cols[0]) != 1 || len(cols[1]) != 1 {
		t.Fatalf("columns %d %d %d", len(cols[0]), len(cols[1]), len(cols[2]))
	}
	win.drawBoard(pageContext(1200, 760), boardRect)
	if !win.board.clear.Empty() {
		t.Fatal("Clear shown with nothing done")
	}
}
