package ui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/skills"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

type palSection int

const (
	palWaiting palSection = iota
	palSessions
	palSkills
	palKaze
)

// palItem is one palette row.
type palItem struct {
	Section         palSection
	Key             string // stable across rebuilds: "tab:<id>", "skill:<name>", "act:<config>", "ws:<id>"
	Agent           string
	Name, Command   string // Command: the row's " · /command" part, sessions only
	Detail          string // right-aligned dim text
	Mark            mark
	Running, Member bool
	Claude          bool
	Color           string // member color
	Tab             tabs.TabID
	Prompt          string // "/" + skill name
	Act             action
	Workspace       string // workspace id for "Run workspace <name>"
}

// palAction is one entry of the kaze section.
type palAction struct {
	Name, Key, Config string // Key: formatShortcut text, "" for none
	Act               action
	Workspace         string
}

type palInput struct {
	Query    string
	Sessions []sideRow    // sidebar order, unfiltered
	Members  []sideMember // by run
	Skills   []skills.Skill
	Recent   []string // prompts, most recent first
	Actions  []palAction
}

// palQuery is the query as matched: lowercased, trimmed, one leading "/" dropped.
func palQuery(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	return strings.TrimPrefix(q, "/")
}

// palTier ranks a match inside its section, lower first: the name starts with
// q (for "plugin:name" any part after a ":" too), the name contains q, only
// the other text contains q. ok is false for no match.
func palTier(q, name string, other ...string) (tier int, ok bool) {
	if q == "" {
		return 0, true
	}
	n := strings.ToLower(name)
	if strings.HasPrefix(n, q) {
		return 0, true
	}
	for i := strings.IndexByte(n, ':'); i >= 0; {
		n2 := n[i+1:]
		if strings.HasPrefix(n2, q) {
			return 0, true
		}
		j := strings.IndexByte(n2, ':')
		if j < 0 {
			break
		}
		i += 1 + j
	}
	if strings.Contains(n, q) {
		return 1, true
	}
	for _, o := range other {
		if strings.Contains(strings.ToLower(o), q) {
			return 2, true
		}
	}
	return 0, false
}

type ranked struct {
	palItem
	tier int
}

// buildPalette lists the items matching in.Query in four sections: waiting on
// you, sessions, skills, kaze. Inside a section name-prefix matches come first,
// then name matches, then the rest, each keeping the section's base order.
func buildPalette(in palInput) []palItem {
	q := palQuery(in.Query)
	var waiting, sessions, skl, kaze []ranked
	for _, r := range in.Sessions {
		tier, ok := palTier(q, r.Title, r.Command, r.Folder, r.Branch, r.Agent)
		if !ok {
			continue
		}
		it := palItem{
			Key: "tab:" + strconv.Itoa(int(r.ID)), Name: r.Title, Command: r.Command, Agent: r.Agent,
			Mark: r.Mark, Running: r.Running, Claude: r.Claude, Tab: r.ID,
		}
		if r.Mark == markNeeds {
			it.Section, it.Detail = palWaiting, "needs you · go to session"
			waiting = append(waiting, ranked{it, tier})
			continue
		}
		it.Section, it.Detail = palSessions, sessionWord(r)+" · go to session"
		sessions = append(sessions, ranked{it, tier})
	}
	for _, m := range in.Members {
		if m.ID == 0 {
			continue // its shell never started: there is nothing to go to
		}
		tier, ok := palTier(q, m.Title, m.Status)
		if !ok {
			continue
		}
		sessions = append(sessions, ranked{palItem{
			Section: palSessions, Key: "tab:" + strconv.Itoa(int(m.ID)), Name: m.Title,
			Detail: "server · " + m.Status, Mark: m.Mark, Member: true, Color: m.Color, Tab: m.ID,
		}, tier})
	}
	for _, s := range orderSkills(in.Skills, in.Recent) {
		tier, ok := palTier(q, s.Name, s.Desc)
		if !ok {
			continue
		}
		skl = append(skl, ranked{palItem{
			Section: palSkills, Key: "skill:" + s.Name, Name: "/" + s.Name, Detail: s.Desc, Prompt: "/" + s.Name,
		}, tier})
	}
	for _, a := range in.Actions {
		tier, ok := palTier(q, a.Name, a.Key)
		if !ok {
			continue
		}
		it := palItem{Section: palKaze, Key: "act:" + a.Config, Name: a.Name, Detail: a.Key, Act: a.Act, Workspace: a.Workspace}
		if a.Workspace != "" {
			it.Key = "ws:" + a.Workspace
		}
		kaze = append(kaze, ranked{it, tier})
	}
	var out []palItem
	for _, sec := range [][]ranked{waiting, sessions, skl, kaze} {
		sort.SliceStable(sec, func(i, j int) bool { return sec[i].tier < sec[j].tier })
		for _, r := range sec {
			out = append(out, r.palItem)
		}
	}
	return out
}

// sessionWord is a session's state in the palette: done, running, idle for a
// Claude session at rest, shell otherwise.
func sessionWord(r sideRow) string {
	switch {
	case r.Mark == markDone:
		return "done"
	case r.Running:
		return "running"
	case r.Claude || r.Agent != "":
		return "idle"
	}
	return "shell"
}

// orderSkills puts recently started skills first, most recent first, then the
// rest by name, case-insensitively.
func orderSkills(list []skills.Skill, recent []string) []skills.Skill {
	pos := map[string]int{}
	for i, p := range recent {
		pos[strings.TrimPrefix(p, "/")] = i + 1
	}
	out := append([]skills.Skill(nil), list...)
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := pos[out[i].Name], pos[out[j].Name]
		if (pi > 0) != (pj > 0) {
			return pi > 0
		}
		if pi > 0 {
			return pi < pj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// workspaceActions are the kaze section's "Run workspace <name>" items.
func workspaceActions(spaces []workspaces.Workspace) []palAction {
	var out []palAction
	for _, ws := range spaces {
		name := strings.TrimSpace(ws.Name)
		if name == "" {
			name = ws.ID
		}
		out = append(out, palAction{Name: "Run workspace " + name, Workspace: ws.ID})
	}
	return out
}

const maxRecent = 10

// recentPrompts are the last prompts started from the palette, most recent
// first, in memory only.
type recentPrompts struct{ list []string }

func (r *recentPrompts) add(p string) {
	out := []string{p}
	for _, q := range r.list {
		if q != p && len(out) < maxRecent {
			out = append(out, q)
		}
	}
	r.list = out
}
