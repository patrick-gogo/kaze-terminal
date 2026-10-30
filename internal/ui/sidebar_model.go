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
	Command               string // the last slash command while Claude runs in the tab
	Running               bool
	Claude                bool // Claude Code has reported from this tab
	Add, Del              int  // lines added and removed in the folder's repo
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
	Mark    mark // the strongest member mark, needs over done
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
	Clean    func(string) string             // drops runes the fonts lack before a title is shortened; nil keeps them
	Agent    func(tabs.TabID) agentState     // nil: no tab runs Claude
	Name     func(tabs.TabID) string         // the owner's name for a row; nil or "" for none
	Lines    func(tabs.TabID) (add, del int) // nil: no counts
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
	for _, s := range []string{r.Title, r.Command, r.Folder, r.Branch} {
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
		var a agentState
		if in.Agent != nil {
			a = in.Agent(t.ID)
		}
		row := sideRow{
			ID:      t.ID,
			Title:   in.label(t, loc, a),
			Folder:  tildeHome(loc.cwd, in.Home),
			Branch:  shortBranch(loc.branch),
			Command: a.command,
			Running: a.running,
			Claude:  a.claude,
			Mark:    in.Mark(t.ID),
		}
		if in.Lines != nil {
			row.Add, row.Del = in.Lines(t.ID)
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

// sessionRows is every row, groups then loose, ignoring fold; build the model
// with an empty Filter.
func sessionRows(m sideModel) []sideRow {
	var rows []sideRow
	for _, g := range m.Groups {
		rows = append(rows, g.Rows...)
	}
	return append(rows, m.Loose...)
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

// curRuns is this frame's runs while it is drawn, else a fresh sideRuns.
func (win *window) curRuns() []sideRun {
	if win.frameRuns != nil {
		return *win.frameRuns
	}
	return win.sideRuns()
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
			exited := t != nil && t.Exited
			if text == "Finished" || (exited && !failed) {
				mem.Status = "stopped" // an exited shell is not starting or waiting, whatever its run said
			}
			mem.Down = text == "Finished" || failed || exited
			if mem.Down {
				sr.Down++
			}
			if mem.Mark == markNeeds || mem.Mark == markDone && sr.Mark == markNone {
				sr.Mark = mem.Mark
			}
			sr.Members = append(sr.Members, mem)
		}
		if open {
			out = append(out, sr)
		}
	}
	return out
}

// label is t's row title by the title rule, cleaned first so dropped runes never eat
// the length budget.
func (in sideInput) label(t *tabs.Tab, loc locEntry, a agentState) string {
	if s := in.ruled(t, loc, a); s != "" {
		lt := *t
		lt.Title = s
		return tabLabelWith(&lt, in.DefTitle, in.clean)
	}
	return tabLabelWith(t, in.DefTitle, in.clean)
}

// renameText is the text a rename starts from: the title uncut and without the exit mark.
func (in sideInput) renameText(t *tabs.Tab, loc locEntry, a agentState) string {
	if s := in.ruled(t, loc, a); s != "" {
		return strings.TrimSpace(in.clean(s))
	}
	lt := *t
	lt.Exited = false
	return tabLabelWith(&lt, in.DefTitle, in.clean)
}

func (in sideInput) clean(s string) string {
	if in.Clean == nil {
		return s
	}
	return in.Clean(s)
}

// ruled is the title rule's pick before the tab's own title: the owner's name, the
// last command's argument (kept after Claude leaves), then the folder's name. A
// workspace tab keeps its fixed title.
func (in sideInput) ruled(t *tabs.Tab, loc locEntry, a agentState) string {
	if t.FixedTitle != "" {
		return ""
	}
	if in.Name != nil {
		if n := in.Name(t.ID); n != "" {
			return n
		}
	}
	if a.arg != "" {
		return a.arg
	}
	return folderName(loc.cwd)
}

// folderName is dir's last part for either separator; "" for a root or no folder.
func folderName(dir string) string {
	dir = strings.TrimRight(dir, `\/`)
	if i := strings.LastIndexAny(dir, `\/`); i >= 0 {
		dir = dir[i+1:]
	}
	if strings.HasSuffix(dir, ":") {
		return "" // a drive root
	}
	return dir
}

// stateWord is the word the row's third line ends with: needs you and done beat running.
func (r sideRow) stateWord() string {
	switch {
	case r.Mark == markNeeds:
		return "needs you"
	case r.Mark == markDone:
		return "done"
	case r.Running:
		return "running"
	}
	return ""
}
