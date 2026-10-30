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
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

// statusParts is what the status bar shows: the active tab's folder and branch
// on the left, the memory/tab count (and any notice) on the right.
type statusParts struct {
	Folder, Branch, Right string
	Usage                 []usage.Segment // Claude usage, drawn right-aligned before Right
	Servers               []serverDot     // one per workspace run member, drawn before Usage
}

// serverDot is one run member's status dot: red when Down, green when Up, muted otherwise.
type serverDot struct{ Down, Up bool }

// locEntry is the branch and repo found for one tab at one folder report.
type locEntry struct {
	cwd      string
	seq      uint64
	branch   string
	repo     string // common git folder, "" outside a repo
	repoName string
}

func (win *window) statusParts() statusParts {
	p := statusParts{Right: win.statusText(len(win.mgr.Tabs()))}
	for _, r := range win.curRuns() {
		for _, m := range r.Members {
			p.Servers = append(p.Servers, serverDot{Down: m.Down, Up: !m.Down && m.Status == "Running"})
		}
	}
	t := win.mgr.Active()
	if win.opt.Config.ShowUsage {
		p.Usage = win.usageSegments(t, time.Now())
	}
	if t == nil {
		return p
	}
	e := win.locOf(t)
	if e.cwd == "" {
		return p
	}
	p.Folder = cleanText(tildeHome(e.cwd, win.home))
	p.Branch = cleanText(e.branch)
	return p
}

// locOf returns t's folder, branch and repo. A new report (even for the same
// folder) or a new folder re-reads them; every other call reuses the cache, so
// no file I/O per frame. A tab with no reported folder yields a zero entry.
func (win *window) locOf(t *tabs.Tab) locEntry {
	s := t.Session
	s.Lock()
	cwd, seq := s.Applier().Cwd(), s.Applier().CwdSeq()
	s.Unlock()
	if cwd == "" {
		return locEntry{}
	}
	e, ok := win.locs[t.ID]
	if ok && e.cwd == cwd && e.seq == seq {
		return e
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
	delete(win.locs, t.ID)
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
