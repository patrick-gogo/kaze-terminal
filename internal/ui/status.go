package ui

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/patrick-gogo/kaze-terminal/internal/gitinfo"
	"github.com/patrick-gogo/kaze-terminal/internal/resources"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

// serverDot is one run member's status dot: red when Down, green when Up, muted otherwise.
type serverDot struct{ Down, Up bool }

// locEntry is the branch and repo found for one tab at one folder report.
type locEntry struct {
	pending, stale bool
	ticket         uint64
	cwd            string
	seq            uint64
	branch         string
	repo           string // common git folder, "" outside a repo
	repoName       string
}

// locOf returns the folder immediately and queues background Git discovery on
// a new report or invalidation. Known same-folder metadata stays visible until
// refreshed. Tests may inject synchronous lookups. Unchanged frames do no I/O.
func (win *window) locOf(t *tabs.Tab) locEntry {
	s := t.Session
	s.Lock()
	cwd, seq := s.Applier().Cwd(), s.Applier().CwdSeq()
	s.Unlock()
	if cwd == "" {
		return locEntry{}
	}
	e, ok := win.locs[t.ID]
	if ok && e.cwd == cwd && e.seq == seq && !e.stale {
		return e
	}
	if win.branchFn == nil && win.repoFn == nil {
		return win.requestLocation(t, cwd, seq)
	}
	branch := win.branchFn
	if branch == nil {
		branch = gitinfo.Branch
	}
	repo := win.repoFn
	if repo == nil {
		repo = gitinfo.Repo
	}
	e = locEntry{cwd: cwd, seq: seq, branch: branch(cwd)}
	e.repo, e.repoName = repo(cwd)
	if win.locs == nil {
		win.locs = map[tabs.TabID]locEntry{}
	}
	win.locs[t.ID] = e
	return e
}

// staleBranch makes the next frame re-read t's branch. Claude Code can switch
// branches with no prompt redraw, so tab switch and focus gain (both rare) re-check.
func (win *window) staleBranch(t *tabs.Tab) {
	if win.branchFn != nil || win.repoFn != nil {
		delete(win.locs, t.ID)
		return
	}
	if e, ok := win.locs[t.ID]; ok {
		e.stale = true
		win.locs[t.ID] = e
	}
}

// pruneLocs forgets closed tabs.
func (win *window) pruneLocs(list []*tabs.Tab) {
	for id := range win.locs { // deleting during range is safe in Go
		if !slices.ContainsFunc(list, func(t *tabs.Tab) bool { return t.ID == id }) {
			delete(win.locs, id)
		}
	}
	for id := range win.ctx {
		if !slices.ContainsFunc(list, func(t *tabs.Tab) bool { return t.ID == id }) {
			delete(win.ctx, id)
		}
	}
}

// userHome is the home folder, "" when unknown.
func userHome() string {
	h, _ := os.UserHomeDir()
	return h
}

// tildeHome shows the home folder as "~". Windows paths compare case-insensitively.
func tildeHome(p, home string) string {
	if home == "" {
		return p
	}
	same := func(a, b string) bool {
		if runtime.GOOS == "windows" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	home = strings.TrimRight(home, `/\`)
	if len(p) < len(home) || !same(p[:len(home)], home) {
		return p
	}
	rest := p[len(home):]
	if rest == "" {
		return "~"
	}
	if rest[0] == '/' || rest[0] == filepath.Separator {
		return "~" + rest
	}
	return p
}

// cleanText drops control characters (C0, DEL, C1) before text is drawn.
func cleanText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, s)
}

// truncateLeft keeps the last max runes behind a leading "…", so the end of a
// path (the part that matters) stays visible.
func truncateLeft(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	if max < 1 {
		max = 1
	}
	r := []rune(s)
	return "…" + string(r[len(r)-(max-1):])
}

// branchLabel is "⎇ branch"; when no font has the glyph, fit drops it and the
// leftover space is trimmed so the name stays flush.
func (c *chrome) branchLabel(branch string) string {
	return strings.TrimLeft(c.fit("⎇ "+branch), " ")
}

// showServer shows the first down run member, or the first member when none is down.
func (win *window) showServer() {
	var first, down tabs.TabID
	for _, r := range win.sideRuns() {
		for _, m := range r.Members {
			if m.ID == 0 || !win.mgr.Has(m.ID) {
				continue
			}
			if first == 0 {
				first = m.ID
			}
			if m.Down && down == 0 {
				down = m.ID
			}
		}
	}
	if down == 0 {
		down = first
	}
	if down != 0 {
		win.floatMember(down)
		win.w.Invalidate()
	}
}

// statusModel is what the redesigned status bar shows (spec 6.1).
type statusModel struct {
	Folder, Branch  string
	Add, Del        int
	Update          string // the newer version, "" for none
	Agents, Waiting int    // running and waiting subagents; waiting includes session waits
	Servers         []serverDot
	Usage           []usage.Detail
	Tabs            int
	Resources       resources.Snapshot
	Notice          string
}

func (win *window) statusModel(now time.Time) statusModel {
	m := statusModel{Tabs: len(win.mgr.Tabs()), Agents: win.subs.Active(0)}
	if win.resources != nil {
		if win.resources.needRoots(now) {
			win.resources.tick(now, win.resourceRoots())
		}
		m.Resources = win.resources.snapshot
	}

	if _, ok := win.noticeWake(now); ok {
		m.Notice = win.notice
	}
	if win.updLabel() != "" {
		m.Update = win.upd.rel.Version
	}
	_, waiting, _ := win.subs.Board()
	m.Waiting = len(waiting) + len(win.subs.SessionWaits())
	for _, r := range win.curRuns() {
		for _, mem := range r.Members {
			m.Servers = append(m.Servers, serverDot{Down: mem.Down, Up: !mem.Down && mem.Status == "Running"})
		}
	}
	t := win.mgr.Active()
	if win.opt.Config.ShowUsage {
		u := win.acct
		if t != nil {
			if e, ok := win.ctx[t.ID]; ok && e.cwdSeq == cwdSeq(t) {
				u.Ctx = &e.pct
			}
		}
		m.Usage = usage.Details(u, now)
	}
	if t == nil {
		return m
	}
	e := win.locOf(t)
	if e.cwd == "" {
		return m
	}
	m.Folder = cleanText(tildeHome(e.cwd, win.home))
	m.Branch = cleanText(e.branch)
	if m.Branch != "" {
		m.Add, m.Del = win.linesOf(t.ID)
	}
	return m
}
