package ui

import (
	"sort"
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// sideRow is one session in the sidebar. Folder is already ~-shortened.
type sideRow struct {
	ID                    tabs.TabID
	Title, Folder, Branch string
	Mark                  mark
}

// sideGroup is one repo's sessions. Rows is filtered; Count is not.
type sideGroup struct {
	Key, Name string
	Rows      []sideRow
	Count     int  // all members, before the filter
	Waiting   int  // members with markNeeds
	Folded    bool // as drawn: false while a filter is active
}

// sideMember is one workspace run member.
type sideMember struct {
	ID           tabs.TabID // 0 when its shell never started
	Title, Color string
	Status       string // memberStatus text, "Finished" shown as "stopped"
	Down         bool   // stopped, failed or its shell exited
	Mark         mark
}

// sideRun is one workspace run's section.
type sideRun struct {
	Run     uint64
	Name    string
	Down    int
	Members []sideMember
}

type sideModel struct {
	Groups []sideGroup
	Loose  []sideRow
	Runs   []sideRun
}

type sideInput struct {
	Tabs     []*tabs.Tab
	Loc      func(*tabs.Tab) locEntry
	Mark     func(tabs.TabID) mark
	Member   func(*tabs.Tab) (run uint64, ok bool) // is t a workspace run member
	Runs     []sideRun                             // built by the window from win.runs
	Folded   map[string]bool                       // by group Key
	Filter   string
	Home     string
	DefTitle string
	Clean    func(string) string // drops runes the fonts lack before a title is shortened; nil keeps them
}

// branchPrefixes are the leading segments dropped from a branch name.
var branchPrefixes = []string{"feature/", "bugfix/", "fix/", "feat/", "chore/", "docs/"}

func shortBranch(b string) string {
	for _, p := range branchPrefixes {
		if strings.HasPrefix(b, p) {
			return b[len(p):]
		}
	}
	return b
}

func (r sideRow) matches(lowerFilter string) bool {
	if lowerFilter == "" {
		return true
	}
	for _, s := range []string{r.Title, r.Folder, r.Branch} {
		if strings.Contains(strings.ToLower(s), lowerFilter) {
			return true
		}
	}
	return false
}

// buildSide groups the sessions by repo in tab order; run members are not sessions.
func buildSide(in sideInput) sideModel {
	m := sideModel{Runs: in.Runs}
	filter := strings.ToLower(strings.TrimSpace(in.Filter))
	byKey := map[string]int{} // group Key to index in all
	var all []sideGroup
	var members [][]sideRow // every row per group, unfiltered
	var loose []sideRow
	for _, t := range in.Tabs {
		if _, ok := in.Member(t); ok {
			continue
		}
		loc := in.Loc(t)
		row := sideRow{
			ID:     t.ID,
			Title:  in.label(t),
			Folder: tildeHome(loc.cwd, in.Home),
			Branch: shortBranch(loc.branch),
			Mark:   in.Mark(t.ID),
		}
		if loc.cwd == "" || loc.repo == "" {
			loose = append(loose, row)
			continue
		}
		i, ok := byKey[loc.repo]
		if !ok {
			i = len(all)
			byKey[loc.repo] = i
			all = append(all, sideGroup{Key: loc.repo, Name: loc.repoName})
			members = append(members, nil)
		}
		members[i] = append(members[i], row)
	}
	for i, g := range all {
		g.Count = len(members[i])
		for _, r := range members[i] {
			if r.Mark == markNeeds {
				g.Waiting++
			}
			if r.matches(filter) {
				g.Rows = append(g.Rows, r)
			}
		}
		if len(g.Rows) == 0 {
			continue
		}
		g.Folded = in.Folded[g.Key] && filter == ""
		m.Groups = append(m.Groups, g)
	}
	for _, r := range loose {
		if r.matches(filter) {
			m.Loose = append(m.Loose, r)
		}
	}
	return m
}

// sessionOrder is every row, groups then loose, ignoring fold and filter; build
// the model with an empty Filter.
func sessionOrder(m sideModel) []tabs.TabID {
	var ids []tabs.TabID
	for _, g := range m.Groups {
		for _, r := range g.Rows {
			ids = append(ids, r.ID)
		}
	}
	for _, r := range m.Loose {
		ids = append(ids, r.ID)
	}
	return ids
}

// sideRuns lists the open workspace runs by id; a run with no member tab open is left out.
func (win *window) sideRuns() []sideRun {
	ids := make([]uint64, 0, len(win.runs))
	for id := range win.runs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var out []sideRun
	for _, id := range ids {
		r := win.runs[id]
		sr := sideRun{Run: r.id, Name: r.name}
		open := false
		for _, wm := range r.members {
			var t *tabs.Tab
			if wm.tab != 0 {
				t = win.mgr.Lookup(wm.tab)
			}
			open = open || t != nil
			text, _, failed := win.memberStatus(wm, t)
			mem := sideMember{ID: wm.tab, Title: wm.title, Status: text}
			if t != nil {
				mem.Color = t.Color
				mem.Mark = win.alerts.markOf(t.ID)
			}
			if text == "Finished" {
				mem.Status = "stopped"
			}
			mem.Down = text == "Finished" || failed || (t != nil && t.Exited)
			if mem.Down {
				sr.Down++
			}
			sr.Members = append(sr.Members, mem)
		}
		if open {
			out = append(out, sr)
		}
	}
	return out
}

// label is t's row title, cleaned first so dropped runes never eat the length budget.
func (in sideInput) label(t *tabs.Tab) string {
	if in.Clean == nil {
		return tabLabel(t, in.DefTitle)
	}
	return tabLabelWith(t, in.DefTitle, in.Clean)
}
