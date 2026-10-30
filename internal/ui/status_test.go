package ui

import (
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/vt"
)

// report makes the tab's shell announce dir with OSC 7, as a prompt hook would.
func report(t *testing.T, tab *tabs.Tab, dir string) {
	t.Helper()
	p := filepath.ToSlash(dir)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	u := url.URL{Path: p}
	s := tab.Session
	s.Lock()
	vt.NewParser(s.Applier()).Feed([]byte("\x1b]7;file://host" + u.EscapedPath() + "\x07"))
	s.Unlock()
}

func statusWindow(t *testing.T) (*window, *tabs.Tab, *[]string) {
	t.Helper()
	win := testWindow(t, &recStarter{})
	tab, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	win.branchFn = func(dir string) string { calls = append(calls, dir); return "main" }
	return win, tab, &calls
}

func TestStatusShowsHomeAsTilde(t *testing.T) {
	win, tab, _ := statusWindow(t)
	home := tmp(t)
	win.home = home
	report(t, tab, home)
	if got := win.statusParts().Folder; got != "~" {
		t.Fatalf("home: got %q", got)
	}
	report(t, tab, filepath.Join(home, "src", "kaze"))
	want := "~" + string(filepath.Separator) + filepath.Join("src", "kaze")
	if got := win.statusParts().Folder; got != want {
		t.Fatalf("under home: got %q, want %q", got, want)
	}
	other := tmp(t)
	report(t, tab, other)
	if got := win.statusParts().Folder; got != other {
		t.Fatalf("outside home: got %q, want %q", got, other)
	}
}

func TestHomePrefixNeedsASeparator(t *testing.T) {
	win, tab, _ := statusWindow(t)
	home := tmp(t)
	win.home = home
	report(t, tab, home+"-other")
	if got := win.statusParts().Folder; strings.HasPrefix(got, "~") {
		t.Fatalf("got %q", got)
	}
}

func TestStatusShowsBranchWhenInRepo(t *testing.T) {
	win, tab, _ := statusWindow(t)
	report(t, tab, tmp(t))
	if got := win.statusParts().Branch; got != "main" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusHidesBranchOutsideRepo(t *testing.T) {
	win, tab, _ := statusWindow(t)
	win.branchFn = func(string) string { return "" }
	report(t, tab, tmp(t))
	if got := win.statusParts().Branch; got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestStatusEmptyBeforeAnyReport(t *testing.T) {
	win, _, calls := statusWindow(t)
	p := win.statusParts()
	if p.Folder != "" || p.Branch != "" || len(*calls) != 0 {
		t.Fatalf("parts %+v, lookups %v", p, *calls)
	}
}

func TestBranchLookedUpOnlyOnCwdChange(t *testing.T) {
	win, tab, calls := statusWindow(t)
	a, b := tmp(t), tmp(t)
	report(t, tab, a)
	for i := 0; i < 5; i++ {
		win.statusParts()
	}
	if len(*calls) != 1 {
		t.Fatalf("5 frames, same folder: %d lookups", len(*calls))
	}
	report(t, tab, a) // the prompt re-reports after `git switch`
	win.statusParts()
	win.statusParts()
	if len(*calls) != 2 {
		t.Fatalf("same folder re-reported: %d lookups, want 2", len(*calls))
	}
	report(t, tab, b)
	win.statusParts()
	if len(*calls) != 3 || (*calls)[2] != b {
		t.Fatalf("new folder: lookups %v", *calls)
	}
}

func TestBranchCacheIsPerTab(t *testing.T) {
	win, tab, calls := statusWindow(t)
	report(t, tab, tmp(t))
	win.statusParts()
	second, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	report(t, second, tmp(t))
	win.statusParts() // second is active
	win.mgr.SetActive(tab.ID)
	win.statusParts()
	win.statusParts()
	if len(*calls) != 2 {
		t.Fatalf("frames without a switch re-read the branch: %v", *calls)
	}
}

// Claude Code can switch the branch without a prompt redraw, so the rare
// moments the owner looks again (tab switch, window focus) re-read it once.
func TestBranchRereadOnTabSwitch(t *testing.T) {
	win, tab, calls := statusWindow(t)
	report(t, tab, tmp(t))
	win.switchedTo(tab)
	win.statusParts()
	second, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	report(t, second, tmp(t))
	win.switchedTo(second)
	win.statusParts()
	n := len(*calls)
	branch := "feature/x"
	win.branchFn = func(dir string) string { *calls = append(*calls, dir); return branch }
	win.mgr.SetActive(tab.ID)
	win.switchedTo(tab)
	for range 3 {
		win.statusParts()
	}
	if len(*calls) != n+1 {
		t.Fatalf("switching back: %d lookups, want 1", len(*calls)-n)
	}
	if got := win.statusParts().Branch; got != branch {
		t.Fatalf("branch %q, want %q", got, branch)
	}
}

func TestBranchRereadOnFocusGain(t *testing.T) {
	win, tab, calls := statusWindow(t)
	win.focused = true
	report(t, tab, tmp(t))
	win.statusParts()
	win.setFocused(false)
	win.statusParts()
	if len(*calls) != 1 {
		t.Fatalf("losing focus re-read the branch: %v", *calls)
	}
	win.setFocused(true)
	for range 3 {
		win.statusParts()
	}
	if len(*calls) != 2 {
		t.Fatalf("focus gain: %d lookups, want 2", len(*calls))
	}
}

func TestFolderTruncatesFromLeft(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"abcdef", 6, "abcdef"},
		{"abcdef", 4, "…def"},
		{"日本語のフォルダ", 4, "…ォルダ"},
		{"abc", 1, "…"},
		{"abc", 0, "…"},
	}
	for _, c := range cases {
		if got := truncateLeft(c.in, c.max); got != c.want {
			t.Errorf("truncateLeft(%q,%d) = %q, want %q", c.in, c.max, got, c.want)
		}
	}
}

func TestStatusTextHasNoControlChars(t *testing.T) {
	win, tab, _ := statusWindow(t)
	win.branchFn = func(string) string { return "a\x1b[31mb\u009bc" }
	report(t, tab, tmp(t))
	p := win.statusParts()
	for _, s := range []string{p.Folder, p.Branch} {
		for _, r := range s {
			if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
				t.Fatalf("control rune %U in %q", r, s)
			}
		}
	}
	if p.Branch != "a[31mbc" {
		t.Fatalf("branch %q", p.Branch)
	}
}

// tmp is a temp dir with native separators (GOTMPDIR may use slashes).
func tmp(t *testing.T) string { return filepath.Clean(t.TempDir()) }

func TestPruneLocsDropsClosedTabsEvenWhenCountMatches(t *testing.T) {
	win, tab, _ := statusWindow(t)
	second, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	win.locs = map[tabs.TabID]locEntry{tab.ID: {cwd: "a"}, 9999: {cwd: "gone"}}
	win.pruneLocs([]*tabs.Tab{tab, second})
	if _, ok := win.locs[9999]; ok || len(win.locs) != 1 {
		t.Fatalf("stale entry kept: %v", win.locs)
	}
}

func TestBranchLabelHasNoLeadingSpaceWhenGlyphIsDropped(t *testing.T) {
	c := &chrome{covered: map[rune]bool{'⎇': false}}
	if got := c.branchLabel("main"); got != "main" {
		t.Fatalf("got %q", got)
	}
	c = &chrome{covered: map[rune]bool{'⎇': true}}
	if got := c.branchLabel("main"); got != "⎇ main" {
		t.Fatalf("got %q", got)
	}
}

func TestLocOfCachesPerFolderReport(t *testing.T) {
	win, tab, branchCalls := statusWindow(t)
	var repoCalls []string
	win.repoFn = func(dir string) (string, string) { repoCalls = append(repoCalls, dir); return "/r/.git", "r" }
	report(t, tab, tmp(t))
	var e locEntry
	for range 3 {
		e = win.locOf(tab)
	}
	if len(*branchCalls) != 1 || len(repoCalls) != 1 {
		t.Fatalf("branch %d, repo %d lookups, want 1 each", len(*branchCalls), len(repoCalls))
	}
	if e.repo != "/r/.git" || e.repoName != "r" || e.branch != "main" {
		t.Fatalf("entry %+v", e)
	}
	report(t, tab, e.cwd)
	win.locOf(tab)
	if len(*branchCalls) != 2 || len(repoCalls) != 2 {
		t.Fatalf("new CwdSeq: branch %d, repo %d, want 2 each", len(*branchCalls), len(repoCalls))
	}
}
