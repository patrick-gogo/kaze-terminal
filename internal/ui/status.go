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
}

// locEntry is the branch found for one tab at one folder report.
type locEntry struct {
	cwd    string
	seq    uint64
	branch string
}

func (win *window) statusParts() statusParts {
	p := statusParts{Right: win.statusText(len(win.mgr.Tabs()))}
	t := win.mgr.Active()
	if win.opt.Config.ShowUsage {
		p.Usage = win.usageSegments(t, time.Now())
	}
	if t == nil {
		return p
	}
	s := t.Session
	s.Lock()
	cwd, seq := s.Applier().Cwd(), s.Applier().CwdSeq()
	s.Unlock()
	if cwd == "" {
		return p
	}
	// A new report (even for the same folder) or a new folder re-reads the
	// branch; every other frame reuses it, so no file I/O per frame.
	e, ok := win.locs[t.ID]
	if !ok || e.cwd != cwd || e.seq != seq {
		lookup := win.branchFn
		if lookup == nil {
			lookup = gitinfo.Branch
		}
		e = locEntry{cwd: cwd, seq: seq, branch: lookup(cwd)}
		if win.locs == nil {
			win.locs = map[tabs.TabID]locEntry{}
		}
		win.locs[t.ID] = e
	}
	p.Folder = cleanText(tildeHome(cwd, win.home))
	p.Branch = cleanText(e.branch)
	return p
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
