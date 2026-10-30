package ui

import (
	"reflect"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

func sideTab(id tabs.TabID, title string) *tabs.Tab { return &tabs.Tab{ID: id, Title: title} }

// sideIn builds an input where locs maps a tab id to its location.
func sideIn(ts []*tabs.Tab, locs map[tabs.TabID]locEntry) sideInput {
	return sideInput{
		Tabs:     ts,
		Loc:      func(t *tabs.Tab) locEntry { return locs[t.ID] },
		Mark:     func(tabs.TabID) mark { return markNone },
		Member:   func(*tabs.Tab) (uint64, bool) { return 0, false },
		DefTitle: "shell",
	}
}

func sideIDs(rows []sideRow) []tabs.TabID {
	var ids []tabs.TabID
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

func sideFixture() ([]*tabs.Tab, map[tabs.TabID]locEntry) {
	ts := []*tabs.Tab{sideTab(1, "a"), sideTab(2, "b"), sideTab(3, "c"), sideTab(4, "d")}
	locs := map[tabs.TabID]locEntry{
		1: {cwd: `C:\home\x`, repo: "X", repoName: "x", branch: "feature/login"},
		2: {},
		3: {cwd: `C:\home\y`, repo: "Y", repoName: "y", branch: "main"},
		4: {cwd: `C:\home\x2`, repo: "X", repoName: "x", branch: "bugfix/Crash"},
	}
	return ts, locs
}

func TestSideGroupsByRepoInTabOrder(t *testing.T) {
	ts, locs := sideFixture()
	m := buildSide(sideIn(ts, locs))
	if len(m.Groups) != 2 || m.Groups[0].Key != "X" || m.Groups[1].Key != "Y" {
		t.Fatalf("groups %+v", m.Groups)
	}
	if got := sideIDs(m.Groups[0].Rows); !reflect.DeepEqual(got, []tabs.TabID{1, 4}) {
		t.Fatalf("X rows %v", got)
	}
	if m.Groups[0].Name != "x" || m.Groups[0].Count != 2 {
		t.Fatalf("X group %+v", m.Groups[0])
	}
	if got := sideIDs(m.Loose); !reflect.DeepEqual(got, []tabs.TabID{2}) {
		t.Fatalf("loose %v", got)
	}
	if b := m.Groups[0].Rows[0].Branch; b != "login" {
		t.Fatalf("branch %q, want prefix stripped", b)
	}
	if b := m.Groups[0].Rows[1].Branch; b != "Crash" {
		t.Fatalf("branch %q", b)
	}
}

func TestSideOneMemberStillMakesAGroup(t *testing.T) {
	ts, locs := sideFixture()
	m := buildSide(sideIn(ts[2:3], locs))
	if len(m.Groups) != 1 || m.Groups[0].Count != 1 || len(m.Loose) != 0 {
		t.Fatalf("model %+v", m)
	}
}

func TestSideNoFolderReportIsLoose(t *testing.T) {
	in := sideIn([]*tabs.Tab{sideTab(1, "")}, map[tabs.TabID]locEntry{1: {}})
	m := buildSide(in)
	if len(m.Groups) != 0 || len(m.Loose) != 1 || m.Loose[0].Title != "shell" {
		t.Fatalf("model %+v", m)
	}
}

func TestSideFolderIsTildeShortened(t *testing.T) {
	in := sideIn([]*tabs.Tab{sideTab(1, "a")}, map[tabs.TabID]locEntry{1: {cwd: `C:\home\proj`}})
	in.Home = `C:\home`
	m := buildSide(in)
	if got, want := m.Loose[0].Folder, tildeHome(`C:\home\proj`, `C:\home`); got != want || got == `C:\home\proj` {
		t.Fatalf("folder %q, want %q", got, want)
	}
}

func TestSideMembersAreNotSessions(t *testing.T) {
	ts, locs := sideFixture()
	in := sideIn(ts, locs)
	in.Member = func(t *tabs.Tab) (uint64, bool) { return 1, t.ID == 4 }
	m := buildSide(in)
	if got := sideIDs(m.Groups[0].Rows); !reflect.DeepEqual(got, []tabs.TabID{1}) || m.Groups[0].Count != 1 {
		t.Fatalf("X %+v", m.Groups[0])
	}
	if got := sessionOrder(m); !reflect.DeepEqual(got, []tabs.TabID{1, 3, 2}) {
		t.Fatalf("order %v", got)
	}
}

func TestSideWaitingCount(t *testing.T) {
	ts, locs := sideFixture()
	in := sideIn(ts, locs)
	in.Mark = func(id tabs.TabID) mark {
		switch id {
		case 4:
			return markNeeds
		case 1:
			return markDone
		}
		return markNone
	}
	m := buildSide(in)
	if m.Groups[0].Waiting != 1 || m.Groups[1].Waiting != 0 {
		t.Fatalf("waiting %d %d", m.Groups[0].Waiting, m.Groups[1].Waiting)
	}
	if m.Groups[0].Rows[1].Mark != markNeeds {
		t.Fatalf("row mark %v", m.Groups[0].Rows[1].Mark)
	}
}

func TestSideFoldIsKeptButFilterOpensIt(t *testing.T) {
	ts, locs := sideFixture()
	in := sideIn(ts, locs)
	in.Folded = map[string]bool{"X": true}
	if m := buildSide(in); !m.Groups[0].Folded || m.Groups[1].Folded {
		t.Fatalf("folded %v %v", m.Groups[0].Folded, m.Groups[1].Folded)
	}
	in.Filter = "a"
	if m := buildSide(in); m.Groups[0].Folded {
		t.Fatal("a filter must open the group")
	}
}

func TestSideFilterMatchesTitleFolderBranch(t *testing.T) {
	ts, locs := sideFixture()
	in := sideIn(ts, locs)
	in.Filter = "CRASH" // branch of d, case-insensitive
	m := buildSide(in)
	if len(m.Groups) != 1 || m.Groups[0].Key != "X" || m.Groups[0].Count != 2 {
		t.Fatalf("groups %+v", m.Groups)
	}
	if got := sideIDs(m.Groups[0].Rows); !reflect.DeepEqual(got, []tabs.TabID{4}) {
		t.Fatalf("rows %v", got)
	}
	if len(m.Loose) != 0 {
		t.Fatalf("loose %v", m.Loose)
	}
	in.Filter = `home\y` // folder of c
	m = buildSide(in)
	if len(m.Groups) != 1 || m.Groups[0].Key != "Y" {
		t.Fatalf("folder filter %+v", m.Groups)
	}
	in.Filter = "b" // title of the loose tab
	m = buildSide(in)
	if got := sideIDs(m.Loose); !reflect.DeepEqual(got, []tabs.TabID{2}) {
		t.Fatalf("loose %v", got)
	}
}

func TestSideOrderIgnoresFoldAndFilter(t *testing.T) {
	ts, locs := sideFixture()
	in := sideIn(ts, locs)
	in.Folded = map[string]bool{"X": true}
	if got := sessionOrder(buildSide(in)); !reflect.DeepEqual(got, []tabs.TabID{1, 4, 3, 2}) {
		t.Fatalf("order %v", got)
	}
}

func TestSideRunsFromWindowRuns(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	var tabsOpen []*tabs.Tab
	for i, title := range []string{"api", "web", "db", "gone"} {
		tab, err := win.mgr.NewOpts(tabs.StartOpts{Title: title, Color: "green",
			Group: &tabs.Group{Workspace: "w", Name: "samurai", Run: 2, Member: i}})
		if err != nil {
			t.Fatal(err)
		}
		tabsOpen = append(tabsOpen, tab)
	}
	launched := workspaces.TabRun{State: workspaces.Launched, ShellReady: true, Line: "x"}
	failed := workspaces.TabRun{State: workspaces.Failed, Line: "x"}
	win.runs = map[uint64]*wsRun{
		2: {id: 2, workspace: "w", name: "samurai", members: []*wsMember{
			{tab: tabsOpen[0].ID, title: "api", run: launched},
			{tab: tabsOpen[1].ID, title: "web", run: launched},
			{tab: tabsOpen[2].ID, title: "db", run: failed},
			{tab: tabsOpen[3].ID, title: "gone", run: launched},
		}},
		1: {id: 1, workspace: "w", name: "closed", members: []*wsMember{{tab: 999, title: "x"}}},
	}
	// web finishes: its shell reports a prompt after its line was typed.
	go st.ptys[1].w.Write([]byte("\x1b]133;A\x07PS> "))
	deadline := time.Now().Add(3 * time.Second)
	for promptCount(tabsOpen[1].Session) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("prompt never applied")
		}
		time.Sleep(time.Millisecond)
	}
	// gone: its shell exited.
	st.ptys[3].Close()
	for deadline = time.Now().Add(3 * time.Second); ; time.Sleep(time.Millisecond) {
		win.mgr.Tabs() // refreshes Exited
		if tabsOpen[3].Exited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shell exit never seen")
		}
	}

	got := win.sideRuns()
	if len(got) != 1 || got[0].Run != 2 || got[0].Name != "samurai" {
		t.Fatalf("runs %+v", got)
	}
	r := got[0]
	if len(r.Members) != 4 || r.Down != 3 {
		t.Fatalf("members %d down %d: %+v", len(r.Members), r.Down, r.Members)
	}
	if r.Members[0].Status != "Running" || r.Members[0].Down || r.Members[0].Color != "green" {
		t.Fatalf("api %+v", r.Members[0])
	}
	if r.Members[1].Status != "stopped" || !r.Members[1].Down {
		t.Fatalf("web %+v", r.Members[1])
	}
	if r.Members[2].Status != "Failed" || !r.Members[2].Down {
		t.Fatalf("db %+v", r.Members[2])
	}
	if !r.Members[3].Down || r.Members[3].ID != tabsOpen[3].ID {
		t.Fatalf("gone %+v", r.Members[3])
	}
}

func TestExitedMemberReadsStoppedNotRunning(t *testing.T) {
	win, st := sideWindow(t, true)
	m := openMember(t, win, "api")
	win.runs[1].members[0].run = workspaces.TabRun{Line: "x", State: workspaces.Launched}
	if got := win.sideRuns()[0].Members[0].Status; got != "Running" {
		t.Fatalf("live member reads %q, want Running", got)
	}
	st.ptys[0].Close()
	for deadline := time.Now().Add(3 * time.Second); ; time.Sleep(time.Millisecond) {
		win.mgr.Tabs() // refreshes Exited
		if m.Exited {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("shell exit never seen")
		}
	}
	mem := win.sideRuns()[0].Members[0]
	if mem.Status != "stopped" || !mem.Down {
		t.Fatalf("exited member %+v, want stopped and down", mem)
	}
	// Whatever its run said, an exited shell that did not fail reads stopped; Failed stays.
	for _, c := range []struct {
		run  workspaces.TabRun
		want string
	}{
		{workspaces.TabRun{Line: "x", State: workspaces.Waiting}, "stopped"},
		{workspaces.TabRun{Line: "x", State: workspaces.Waiting, ShellReady: true, Wait: workspaces.WaitDocker}, "stopped"},
		{workspaces.TabRun{Line: "x", State: workspaces.Cancelled}, "stopped"},
		{workspaces.TabRun{Line: "x", State: workspaces.NeedsConfirmation}, "stopped"},
		{workspaces.TabRun{State: workspaces.Waiting}, "stopped"},
		{workspaces.TabRun{Line: "x", State: workspaces.Failed}, "Failed"},
	} {
		win.runs[1].members[0].run = c.run
		if mem := win.sideRuns()[0].Members[0]; mem.Status != c.want || !mem.Down {
			t.Fatalf("exited member with run %+v reads %+v, want %s and down", c.run, mem, c.want)
		}
	}
}

func TestRowTitleRule(t *testing.T) {
	ts := []*tabs.Tab{sideTab(1, "t1"), sideTab(2, "t2"), sideTab(3, "t3"), sideTab(4, "t4"), sideTab(5, "t5")}
	locs := map[tabs.TabID]locEntry{
		1: {cwd: `C:\code\alpha`}, // Claude with an arg
		2: {cwd: `C:\code\beta`},  // named
		3: {cwd: `C:\code\gamma`}, // plain shell with a folder
		4: {},                     // plain shell, no folder report
		5: {cwd: `C:\code\delta`}, // Claude without an arg
	}
	in := sideIn(ts, locs)
	in.Agent = func(id tabs.TabID) agentState {
		switch id {
		case 1, 2:
			return agentState{claude: true, command: "/ship-ticket", arg: "V3-3348"}
		case 5:
			return agentState{claude: true}
		}
		return agentState{}
	}
	in.Name = func(id tabs.TabID) string {
		if id == 2 {
			return "api work"
		}
		return ""
	}
	m := buildSide(in)
	want := map[tabs.TabID]string{1: "V3-3348", 2: "api work", 3: "gamma", 4: "t4", 5: "delta"}
	for _, r := range m.Loose {
		if r.Title != want[r.ID] {
			t.Errorf("row %d title %q, want %q", r.ID, r.Title, want[r.ID])
		}
	}
	if len(m.Loose) != 5 {
		t.Fatalf("rows %+v", m.Loose)
	}
}

func TestRowShowsCommandAndRunning(t *testing.T) {
	in := sideIn([]*tabs.Tab{sideTab(1, "x")}, map[tabs.TabID]locEntry{1: {}})
	in.Agent = func(tabs.TabID) agentState {
		return agentState{claude: true, running: true, command: "/ship-ticket", arg: "V3-1"}
	}
	r := buildSide(in).Loose[0]
	if r.Command != "/ship-ticket" || !r.Running || !r.Claude {
		t.Fatalf("row %+v", r)
	}
	if w := r.stateWord(); w != "running" {
		t.Fatalf("state word %q, want running", w)
	}
}

func TestNeedsBeatsRunningWord(t *testing.T) {
	for mk, want := range map[mark]string{markNeeds: "needs you", markDone: "done", markNone: "running"} {
		r := sideRow{Running: true, Mark: mk}
		if w := r.stateWord(); w != want {
			t.Errorf("mark %v: word %q, want %q", mk, w, want)
		}
	}
	if w := (sideRow{}).stateWord(); w != "" {
		t.Fatalf("idle word %q", w)
	}
}

func TestFilterMatchesCommand(t *testing.T) {
	ts, locs := sideFixture()
	in := sideIn(ts, locs)
	in.Agent = func(id tabs.TabID) agentState {
		if id == 3 {
			return agentState{claude: true, command: "/review-mine"}
		}
		return agentState{}
	}
	in.Filter = "REVIEW"
	m := buildSide(in)
	if len(m.Groups) != 1 || m.Groups[0].Key != "Y" || len(m.Loose) != 0 {
		t.Fatalf("model %+v", m)
	}
}

func TestMembersAreNotRenamed(t *testing.T) {
	tab := &tabs.Tab{ID: 1, Title: "api", FixedTitle: "api"}
	in := sideIn([]*tabs.Tab{tab}, map[tabs.TabID]locEntry{1: {cwd: `C:\code\alpha`}})
	in.Agent = func(tabs.TabID) agentState { return agentState{claude: true, arg: "V3-1"} }
	in.Name = func(tabs.TabID) string { return "renamed" }
	if got := buildSide(in).Loose[0].Title; got != "api" {
		t.Fatalf("a workspace tab's title %q, want its fixed api", got)
	}
}
