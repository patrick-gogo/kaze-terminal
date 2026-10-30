package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/skills"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

func palNames(items []palItem) []string {
	var out []string
	for _, it := range items {
		out = append(out, it.Name)
	}
	return out
}

func palSections(items []palItem) []palSection {
	var out []palSection
	for _, it := range items {
		if len(out) == 0 || out[len(out)-1] != it.Section {
			out = append(out, it.Section)
		}
	}
	return out
}

func samplePal() palInput {
	return palInput{
		Sessions: []sideRow{
			{ID: 1, Title: "V3-3348", Folder: "~/code/beta", Branch: "v3-3348", Command: "/ship-ticket", Mark: markNeeds},
			{ID: 2, Title: "api", Folder: "~/code/api", Running: true, Claude: true},
			{ID: 3, Title: "misc", Folder: "~/notes"},
		},
		Members: []sideMember{{ID: 7, Title: "web", Color: "cyan", Status: "running"}},
		Skills: []skills.Skill{
			{Name: "review-mine", Desc: "Review a branch"},
			{Name: "ship-ticket", Desc: "Open the PR for a ticket; asks for review"},
		},
		Actions: []palAction{{Name: "New session", Key: "Ctrl+Shift+T", Config: "new_tab", Act: actNewTab}},
	}
}

func TestPaletteSectionsInOrder(t *testing.T) {
	got := buildPalette(samplePal())
	want := []palSection{palWaiting, palSessions, palSkills, palKaze}
	if fmt.Sprint(palSections(got)) != fmt.Sprint(want) {
		t.Fatalf("sections %v, want %v", palSections(got), want)
	}
	if got[0].Key != "tab:1" || got[0].Detail != "needs you · go to session" || got[0].Command != "/ship-ticket" {
		t.Errorf("waiting item %+v", got[0])
	}
}

func TestNeedsSessionOnlyUnderWaiting(t *testing.T) {
	n := 0
	for _, it := range buildPalette(samplePal()) {
		if it.Tab == 1 {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("needs session shown %d times", n)
	}
}

func TestSessionDetails(t *testing.T) {
	in := samplePal()
	in.Sessions = append(in.Sessions, sideRow{ID: 4, Title: "idle", Claude: true}, sideRow{ID: 5, Title: "fin", Mark: markDone, Claude: true})
	want := map[string]string{
		"api": "running · go to session", "misc": "shell · go to session",
		"idle": "idle · go to session", "fin": "done · go to session", "web": "server · running",
	}
	for _, it := range buildPalette(in) {
		if w, ok := want[it.Name]; ok && it.Detail != w {
			t.Errorf("%s detail %q, want %q", it.Name, it.Detail, w)
		}
		if it.Name == "web" && (!it.Member || it.Color != "cyan" || it.Key != "tab:7") {
			t.Errorf("member %+v", it)
		}
	}
}

func TestPaletteQueryContainsAnywhere(t *testing.T) {
	in := samplePal()
	in.Query = "ticket"
	got := palNames(buildPalette(in))
	// "/ship-ticket" by name, V3-3348 by its command.
	if strings.Join(got, ",") != "V3-3348,/ship-ticket" {
		t.Fatalf("ticket: %v", got)
	}
	in.Query = "SHIP"
	if got := palNames(buildPalette(in)); len(got) != 2 {
		t.Fatalf("SHIP: %v", got)
	}
	in.Query = "  /ship "
	if got := palNames(buildPalette(in)); len(got) != 2 {
		t.Fatalf("/ship: %v", got)
	}
}

func TestPaletteRanksNamePrefixFirst(t *testing.T) {
	in := palInput{Query: "rev", Skills: []skills.Skill{
		{Name: "a-skill", Desc: "nothing here"},
		{Name: "ship-ticket", Desc: "asks for review"},
		{Name: "prevent", Desc: ""},
		{Name: "review-mine", Desc: ""},
	}}
	got := palNames(buildPalette(in))
	if strings.Join(got, ",") != "/review-mine,/prevent,/ship-ticket" {
		t.Fatalf("got %v", got)
	}
}

func TestPluginNameRanksByItsOwnPart(t *testing.T) {
	in := palInput{Query: "brain", Skills: []skills.Skill{
		{Name: "a-brainy", Desc: ""},
		{Name: "superpowers:brainstorming", Desc: ""},
	}}
	got := palNames(buildPalette(in))
	if strings.Join(got, ",") != "/superpowers:brainstorming,/a-brainy" {
		t.Fatalf("got %v", got)
	}
}

func TestSkillsRecentFirstThenByName(t *testing.T) {
	in := palInput{Recent: []string{"/b", "/gone"}, Skills: []skills.Skill{{Name: "c"}, {Name: "A"}, {Name: "b"}}}
	got := buildPalette(in)
	if strings.Join(palNames(got), ",") != "/b,/A,/c" {
		t.Fatalf("got %v", palNames(got))
	}
	if got[0].Prompt != "/b" || got[0].Key != "skill:b" {
		t.Fatalf("item %+v", got[0])
	}
}

func TestSessionsKeepSidebarOrder(t *testing.T) {
	in := palInput{Query: "o", Sessions: []sideRow{{ID: 3, Title: "zoo"}, {ID: 1, Title: "foo"}, {ID: 2, Title: "boo"}}}
	if got := palNames(buildPalette(in)); strings.Join(got, ",") != "zoo,foo,boo" {
		t.Fatalf("got %v", got)
	}
}

func TestRunWorkspaceItems(t *testing.T) {
	acts := workspaceActions([]workspaces.Workspace{{ID: "samurai", Name: "samurai"}})
	got := buildPalette(palInput{Actions: acts})
	if len(got) != 1 || got[0].Name != "Run workspace samurai" || got[0].Key != "ws:samurai" || got[0].Workspace != "samurai" {
		t.Fatalf("got %+v", got)
	}
}

func TestKazeItems(t *testing.T) {
	got := buildPalette(palInput{Query: "session", Actions: []palAction{
		{Name: "New session", Key: "Ctrl+Shift+T", Config: "new_tab", Act: actNewTab},
		{Name: "Copy", Key: "Ctrl+Shift+C", Config: "copy", Act: actCopy},
	}})
	if len(got) != 1 || got[0].Key != "act:new_tab" || got[0].Detail != "Ctrl+Shift+T" || got[0].Act != actNewTab {
		t.Fatalf("got %+v", got)
	}
}

func TestPaletteNoMatches(t *testing.T) {
	in := samplePal()
	in.Query = "zzz"
	if got := buildPalette(in); len(got) != 0 {
		t.Fatalf("got %v", palNames(got))
	}
}

func TestRecentKeepsTenDistinct(t *testing.T) {
	var r recentPrompts
	for i := 0; i < 12; i++ {
		r.add(fmt.Sprintf("/p%d", i))
	}
	r.add("/p5")
	if len(r.list) != maxRecent || r.list[0] != "/p5" {
		t.Fatalf("list %v", r.list)
	}
	seen := map[string]bool{}
	for _, p := range r.list {
		if seen[p] {
			t.Fatalf("duplicate %s in %v", p, r.list)
		}
		seen[p] = true
	}
}

func TestSessionRowsGroupsThenLoose(t *testing.T) {
	m := sideModel{
		Groups: []sideGroup{{Rows: []sideRow{{ID: 2}, {ID: 1}}}, {Rows: []sideRow{{ID: 5}}}},
		Loose:  []sideRow{{ID: 3}},
	}
	var ids []int
	for _, r := range sessionRows(m) {
		ids = append(ids, int(r.ID))
	}
	if fmt.Sprint(ids) != "[2 1 5 3]" {
		t.Fatalf("got %v", ids)
	}
}
