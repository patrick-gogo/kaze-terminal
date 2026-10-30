package ui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/skills"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// palWindow has three sessions a, b, c with a shown, fake skills, and its
// config and Claude folders in a temp dir.
func palWindow(t *testing.T) (*window, *recStarter, *input.Router, [3]*tabs.Tab) {
	t.Helper()
	win, st := sideWindow(t, true)
	tmp := t.TempDir()
	win.cfgPath = filepath.Join(tmp, "kaze", "config.toml")
	win.claudePath = filepath.Join(tmp, ".claude", "settings.json")
	win.opt.Shell = []string{"/bin/zsh"}
	win.skillsFn = func(skills.Sources) []skills.Skill {
		return []skills.Skill{{Name: "review-mine", Desc: "Review a branch"}, {Name: "ship-ticket", Desc: "Open the PR"}}
	}
	a := openIn(t, win, osPath("/code/alpha/aaa"))
	b := openIn(t, win, osPath("/code/alpha/bbb"))
	c := openIn(t, win, osPath("/code/beta/ccc"))
	win.mgr.SetActive(a.ID)
	win.wantFocus = true // the terminal has the keyboard, as in the running app
	r := new(input.Router)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	return win, st, r, [3]*tabs.Tab{a, b, c}
}

// openPal presses the palette key and waits, bounded, for the skill scan.
func openPal(t *testing.T, win *window, r *input.Router) {
	t.Helper()
	sc := shortcutFor(t, win, actPalette)
	pressKey(t, win, r, sc.Name, sc.Modifiers)
	if !win.palette.open {
		t.Fatal("the palette did not open")
	}
	waitSkills(t, win, r)
}

func waitSkills(t *testing.T, win *window, r *input.Router) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for win.palette.scanning {
		if time.Now().After(deadline) {
			t.Fatal("the skill scan never finished")
		}
		time.Sleep(time.Millisecond)
		sideFrame(t, win, r)
	}
	sideFrame(t, win, r)
}

func palType(t *testing.T, win *window, r *input.Router, s string) {
	t.Helper()
	n := win.palette.ed.Len()
	r.Queue(key.EditEvent{Range: key.Range{Start: n, End: n}, Text: s})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
}

func palKeys(win *window) []string {
	var out []string
	for _, it := range win.palette.items {
		out = append(out, it.Key)
	}
	return out
}

func selected(win *window) palItem {
	i := win.palette.selIndex()
	if i < 0 {
		return palItem{}
	}
	return win.palette.items[i]
}

func TestPaletteKeyOpensAndCloses(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	if !r.Source().Focused(&win.palette.ed) {
		t.Fatal("the query field does not have the keyboard")
	}
	sc := shortcutFor(t, win, actPalette)
	pressKey(t, win, r, sc.Name, sc.Modifiers)
	if win.palette.open || !r.Source().Focused(win.termTag) {
		t.Fatalf("after the key again: open %v, terminal focused %v", win.palette.open, r.Source().Focused(win.termTag))
	}
}

func TestPaletteListsSkillsFromTheTree(t *testing.T) {
	win, _, r, _ := palWindow(t)
	win.skillsFn = nil // the real catalog, over a fake ~/.claude
	dir := filepath.Join(filepath.Dir(win.claudePath), "skills", "review-mine")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: review-mine\ndescription: Review\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	openPal(t, win, r)
	if !strings.Contains(strings.Join(palKeys(win), ","), "skill:review-mine") {
		t.Fatalf("items %v", palKeys(win))
	}
}

func TestSkillsScannedOnlyOnOpen(t *testing.T) {
	win, _, r, _ := palWindow(t)
	var mu sync.Mutex
	calls := 0
	release := make(chan struct{})
	win.skillsFn = func(skills.Sources) []skills.Skill {
		mu.Lock()
		calls++
		mu.Unlock()
		<-release
		return nil
	}
	defer close(release)
	count := func() int { mu.Lock(); defer mu.Unlock(); return calls }
	for range 10 {
		sideFrame(t, win, r)
	}
	if n := count(); n != 0 {
		t.Fatalf("%d scans while closed", n)
	}
	win.openPalette()
	sideFrame(t, win, r)
	deadline := time.Now().Add(2 * time.Second)
	for count() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	win.closePalette()
	win.openPalette()
	sideFrame(t, win, r)
	time.Sleep(20 * time.Millisecond)
	if n := count(); n != 1 {
		t.Fatalf("%d scans, want 1 while one is in flight", n)
	}
}

func TestPaletteIdleCostsNothing(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	before := win.palette.redraws
	for range 5 {
		sideFrame(t, win, r)
	}
	if win.palette.redraws != before || win.palette.scanning {
		t.Fatalf("idle frames asked for %d redraws", win.palette.redraws-before)
	}
}

func palMove(t *testing.T, win *window, r *input.Router, i int) {
	t.Helper()
	for _, h := range win.palette.hits.rows {
		if h.index == i {
			c := centre(h.rect)
			r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(float32(c.X), float32(c.Y))})
			sideFrame(t, win, r)
			return
		}
	}
	t.Fatalf("no row %d", i)
}

func TestPaletteHoverRedrawsOnChange(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	before := win.palette.redraws
	for range 5 {
		palMove(t, win, r, 1)
	}
	if got := win.palette.redraws - before; got != 1 {
		t.Fatalf("%d redraws over one row, want 1", got)
	}
	palMove(t, win, r, 2)
	if got := win.palette.redraws - before; got != 2 {
		t.Fatalf("%d redraws after the next row, want 2", got)
	}
	if win.palette.selIndex() != 2 {
		t.Fatalf("hover did not move the selection: %d", win.palette.selIndex())
	}
}

func TestArrowsAndEnterActOnASession(t *testing.T) {
	win, _, r, tb := palWindow(t)
	openPal(t, win, r)
	pressKey(t, win, r, key.NameDownArrow, 0)
	if got := selected(win); got.Tab != tb[1].ID {
		t.Fatalf("selected %+v, want session b", got)
	}
	pressKey(t, win, r, key.NameUpArrow, 0)
	pressKey(t, win, r, key.NameUpArrow, 0) // clamped at the top
	pressKey(t, win, r, key.NameDownArrow, 0)
	pressKey(t, win, r, key.NameReturn, 0)
	if win.palette.open || win.mgr.Active().ID != tb[1].ID {
		t.Fatalf("open %v, active %d", win.palette.open, win.mgr.Active().ID)
	}
}

func TestEnterOnASkillTypesIt(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	openPal(t, win, r)
	palType(t, win, r, "ship")
	if got := selected(win); got.Key != "skill:ship-ticket" {
		t.Fatalf("selected %+v", got)
	}
	pressKey(t, win, r, key.NameReturn, 0)
	waitSent(t, st.ptys[0], "/ship-ticket ")
	if win.palette.open || win.palette.recent.list[0] != "/ship-ticket" {
		t.Fatalf("open %v, recent %v", win.palette.open, win.palette.recent.list)
	}
}

func TestShiftEnterOpensANewSession(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	openPal(t, win, r)
	palType(t, win, r, "ship")
	pressKey(t, win, r, key.NameReturn, key.ModShift)
	if n := len(win.mgr.Tabs()); n != 4 {
		t.Fatalf("%d tabs, want a new one", n)
	}
	if len(win.launches) != 1 {
		t.Fatal("no launch is waiting for the new shell")
	}
	time.Sleep(20 * time.Millisecond)
	if got := st.ptys[0].sent(); got != "" {
		t.Fatalf("the current session got %q", got)
	}
}

func TestClickOutsideCloses(t *testing.T) {
	win, _, r, tb := palWindow(t)
	openPal(t, win, r)
	row := rowHit(t, win, tb[2].ID)
	sideClick(win, r, row.rect, pointer.ButtonPrimary)
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if win.palette.open {
		t.Fatal("still open")
	}
	if win.mgr.Active().ID != tb[0].ID {
		t.Fatal("the click went through to the sidebar row")
	}
}

func TestClickOnARowActs(t *testing.T) {
	win, _, r, tb := palWindow(t)
	openPal(t, win, r)
	for _, h := range win.palette.hits.rows {
		if win.palette.items[h.index].Tab == tb[2].ID {
			c := centre(h.rect)
			p := f32.Pt(float32(c.X), float32(c.Y))
			r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p},
				pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p})
			sideFrame(t, win, r)
			sideFrame(t, win, r)
			if win.palette.open || win.mgr.Active().ID != tb[2].ID {
				t.Fatalf("open %v, active %d", win.palette.open, win.mgr.Active().ID)
			}
			return
		}
	}
	t.Fatal("no row for session c")
}

func TestEscClosesAndReturnsKeys(t *testing.T) {
	win, st, r, _ := palWindow(t)
	openPal(t, win, r)
	palType(t, win, r, "zz")
	pressKey(t, win, r, key.NameEscape, 0)
	if win.palette.open || win.palette.ed.Text() != "" {
		t.Fatalf("open %v, text %q", win.palette.open, win.palette.ed.Text())
	}
	typedReaches(t, win, r, st.ptys[0], "x")
}

func TestKazeActionRuns(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	palType(t, win, r, "new session")
	if got := selected(win); got.Key != "act:new_tab" || got.Detail != "Ctrl+Shift+T" {
		t.Fatalf("selected %+v", got)
	}
	pressKey(t, win, r, key.NameReturn, 0)
	if n := len(win.mgr.Tabs()); n != 4 || win.palette.open {
		t.Fatalf("%d tabs, open %v", n, win.palette.open)
	}
}

func TestPaletteLeavesItselfOut(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	if strings.Contains(strings.Join(palKeys(win), ","), "act:palette") {
		t.Fatal("the palette lists Open the palette")
	}
}

func TestRunWorkspaceFromThePalette(t *testing.T) {
	win, _, r, _ := palWindow(t)
	ws := workspaces.Workspace{ID: "ws_samurai", Name: "samurai", Tabs: []workspaces.Tab{{ID: "t1", Title: "api", Folder: t.TempDir()}}}
	if err := os.MkdirAll(filepath.Dir(win.cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := workspaces.Save(workspaces.Path(win.cfgPath), workspaces.Document{Workspaces: []workspaces.Workspace{ws}}, workspaces.Revision{}); err != nil {
		t.Fatal(err)
	}
	openPal(t, win, r)
	palType(t, win, r, "samurai")
	if got := selected(win); got.Key != "ws:ws_samurai" || got.Name != "Run workspace samurai" {
		t.Fatalf("selected %+v of %v", got, palKeys(win))
	}
	pressKey(t, win, r, key.NameReturn, 0)
	if len(win.runs) != 1 || len(win.mgr.Tabs()) != 4 {
		t.Fatalf("runs %d, tabs %d", len(win.runs), len(win.mgr.Tabs()))
	}
}

func TestOtherShortcutClosesThenRuns(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	sc := shortcutFor(t, win, actNewTab)
	pressKey(t, win, r, sc.Name, sc.Modifiers)
	if win.palette.open || len(win.mgr.Tabs()) != 4 {
		t.Fatalf("open %v, tabs %d", win.palette.open, len(win.mgr.Tabs()))
	}
}

func TestQueryFiltersLive(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	pressKey(t, win, r, key.NameDownArrow, 0)
	palType(t, win, r, "rev")
	keys := palKeys(win)
	if len(keys) == 0 || keys[0] != "skill:review-mine" {
		t.Fatalf("items %v", keys)
	}
	for _, k := range keys {
		if strings.HasPrefix(k, "tab:") {
			t.Fatalf("a session matched rev: %v", keys)
		}
	}
	if win.palette.selIndex() != 0 {
		t.Fatalf("selection %d, want back on the first", win.palette.selIndex())
	}
}

func TestSelectionFollowsItsItem(t *testing.T) {
	win, _, r, tb := palWindow(t)
	openPal(t, win, r)
	pressKey(t, win, r, key.NameDownArrow, 0)
	pressKey(t, win, r, key.NameDownArrow, 0)
	if selected(win).Tab != tb[2].ID {
		t.Fatalf("selected %+v", selected(win))
	}
	win.alerts.apply(notify.Msg{Tab: int(tb[1].ID), Kind: notify.KindNeeds}, 0, false, func(tabs.TabID) bool { return true }, time.Now())
	sideFrame(t, win, r)
	if win.palette.items[0].Tab != tb[1].ID {
		t.Fatalf("b is not first under Waiting: %v", palKeys(win))
	}
	if selected(win).Tab != tb[2].ID {
		t.Fatalf("selection moved to %+v", selected(win))
	}
}
