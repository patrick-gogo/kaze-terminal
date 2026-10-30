package ui

import (
	"image"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/vt"
)

func centre(r image.Rectangle) image.Point { return r.Min.Add(r.Size().Div(2)) }

// sideWindow is a test window for whole headless frames with the sidebar on. A
// folder under code/<repo>/ belongs to that repo; cfgPath stays "" so nothing is saved.
func sideWindow(t *testing.T, custom bool) (*window, *recStarter) {
	t.Helper()
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	win.focused = true
	win.termTag, win.capTag, win.toastTag = new(int), new(int), new(int)
	win.opt.Config = config.Default()
	win.custom = custom
	win.branchFn = func(string) string { return "" }
	win.repoFn = func(dir string) (string, string) {
		parts := strings.Split(filepath.ToSlash(dir), "/")
		for i, p := range parts {
			if p == "code" && i+1 < len(parts) {
				return "key-" + parts[i+1], parts[i+1]
			}
		}
		return "", ""
	}
	return win, st
}

// osPath turns a slash path into an absolute one for this OS: /code/X/a is
// C:\code\X\a on Windows and stays /code/X/a elsewhere, so fixtures split and
// shorten the way real folders do on the machine running the test.
func osPath(p string) string {
	if runtime.GOOS == "windows" {
		return `C:` + filepath.FromSlash(p)
	}
	return p
}

// openIn opens a session whose shell reports dir; "" leaves it without a folder.
func openIn(t *testing.T, win *window, dir string) *tabs.Tab {
	t.Helper()
	tab, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	if dir != "" {
		report(t, tab, dir)
	}
	return tab
}

// openMember opens a workspace run member in run 1.
func openMember(t *testing.T, win *window, title string) *tabs.Tab {
	t.Helper()
	if win.runs == nil {
		win.runs = map[uint64]*wsRun{}
	}
	run := win.runs[1]
	if run == nil {
		run = &wsRun{id: 1, workspace: "w1", name: "samurai"}
		win.runs[1] = run
	}
	tab, err := win.mgr.NewOpts(tabs.StartOpts{Title: title, Color: "cyan",
		Group: &tabs.Group{Workspace: "w1", Name: "samurai", Run: 1, Member: len(run.members)}})
	if err != nil {
		t.Fatal(err)
	}
	run.members = append(run.members, &wsMember{tab: tab.ID, title: title})
	return tab
}

func sideFrame(t *testing.T, win *window, r *input.Router) { sizedFrame(t, win, r, 1000, 600) }

// sidePt is a sidebar-local point in window pixels.
func sidePt(win *window, p image.Point) f32.Point {
	p = p.Add(win.side.at)
	return f32.Pt(float32(p.X), float32(p.Y))
}

func sideClick(win *window, r *input.Router, rect image.Rectangle, b pointer.Buttons) {
	at := sidePt(win, centre(rect))
	r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: b, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
	)
}

func rowHit(t *testing.T, win *window, id tabs.TabID) sideRowHit {
	t.Helper()
	for _, h := range win.side.hits.rows {
		if h.id == id {
			return h
		}
	}
	t.Fatalf("no sidebar row for tab %d in %+v", id, win.side.hits.rows)
	return sideRowHit{}
}

func hasRow(win *window, id tabs.TabID) bool {
	for _, h := range win.side.hits.rows {
		if h.id == id {
			return true
		}
	}
	return false
}

func TestClickingARowShowsIt(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	b := openIn(t, win, "")
	win.mgr.SetActive(a.ID)
	var r input.Router
	sideFrame(t, win, &r)
	sideClick(win, &r, rowHit(t, win, b.ID).rect, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if got := win.mgr.Active().ID; got != b.ID {
		t.Fatalf("active %d after clicking b's row, want %d", got, b.ID)
	}
}

func TestClosingTheShownSessionSkipsMembers(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	openMember(t, win, "api")
	b := openIn(t, win, "")
	win.mgr.SetActive(a.ID)
	win.runShortcut(layout.Context{}, actCloseTab)
	if win.mgr.Has(a.ID) {
		t.Fatal("close left a open")
	}
	if got := win.mgr.Active().ID; got != b.ID {
		t.Fatalf("shown %d after closing a, want the next session b (%d), not the member", got, b.ID)
	}

	// The row's × and a middle click close through the same path.
	c := openIn(t, win, "")
	win.mgr.SetActive(b.ID)
	var r input.Router
	sideFrame(t, win, &r)
	sideClick(win, &r, rowHit(t, win, b.ID).rect, pointer.ButtonTertiary)
	sideFrame(t, win, &r)
	if win.mgr.Has(b.ID) || win.mgr.Active().ID != c.ID {
		t.Fatalf("middle click: b open %v, active %d, want c (%d)", win.mgr.Has(b.ID), win.mgr.Active().ID, c.ID)
	}
	d := openIn(t, win, "")
	win.mgr.SetActive(c.ID)
	sideFrame(t, win, &r)
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: sidePt(win, centre(rowHit(t, win, c.ID).rect))})
	sideFrame(t, win, &r)
	sideFrame(t, win, &r) // draws the × the hover asked for
	x := rowHit(t, win, c.ID).close
	if x.Empty() {
		t.Fatal("a hovered row shows no ×")
	}
	sideClick(win, &r, x, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if win.mgr.Has(c.ID) || win.mgr.Active().ID != d.ID {
		t.Fatalf("× click: c open %v, active %d, want d (%d)", win.mgr.Has(c.ID), win.mgr.Active().ID, d.ID)
	}
}

func TestClosingTheLastSessionShowsAMember(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	m := openMember(t, win, "api")
	win.mgr.SetActive(a.ID)
	win.closeTab(a.ID)
	if win.mgr.Has(a.ID) {
		t.Fatal("closeTab left a open")
	}
	if got := win.mgr.Active(); got == nil || got.ID != m.ID {
		t.Fatalf("shown %v after closing the last session, want member %d", got, m.ID)
	}
}

func TestNextPrevFollowSidebarOrder(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, osPath("/code/X/a"))
	b := openIn(t, win, "")
	openMember(t, win, "api")
	c := openIn(t, win, osPath("/code/X/c"))
	win.mgr.SetActive(a.ID)
	ctrlTab := key.Event{Name: key.NameTab, Modifiers: key.ModCtrl, State: key.Press}
	want := []tabs.TabID{c.ID, b.ID, a.ID}
	for i, w := range want {
		win.route(layout.Context{}, []event.Event{ctrlTab}, 8, 16)
		if got := win.mgr.Active().ID; got != w {
			t.Fatalf("Ctrl+Tab %d went to %d, want %d (order a, c, b; members skipped)", i+1, got, w)
		}
	}
	win.runShortcut(layout.Context{}, actPrevTab)
	if got := win.mgr.Active().ID; got != b.ID {
		t.Fatalf("prev from a went to %d, want b (%d)", got, b.ID)
	}

	// Settings sits at the start of the order while open, as before.
	win.openSettings()
	win.runShortcut(layout.Context{}, actNextTab)
	if win.set.shown || win.mgr.Active().ID != a.ID {
		t.Fatalf("next from settings: shown %v active %d, want a", win.set.shown, win.mgr.Active().ID)
	}
	win.runShortcut(layout.Context{}, actPrevTab)
	if !win.set.shown {
		t.Fatal("prev from the first session did not show settings")
	}
}

func TestNextFromAMemberGoesToTheFirstSession(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	m := openMember(t, win, "api")
	b := openIn(t, win, "")
	win.mgr.SetActive(m.ID)
	win.runShortcut(layout.Context{}, actNextTab)
	if got := win.mgr.Active().ID; got != a.ID {
		t.Fatalf("next from a member went to %d, want the first session %d", got, a.ID)
	}
	win.mgr.SetActive(m.ID)
	win.runShortcut(layout.Context{}, actPrevTab)
	if got := win.mgr.Active().ID; got != b.ID {
		t.Fatalf("prev from a member went to %d, want the last session %d", got, b.ID)
	}
}

func TestGroupHeaderFolds(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, osPath("/code/X/a"))
	c := openIn(t, win, osPath("/code/X/c"))
	win.mgr.SetActive(a.ID)
	var r input.Router
	sideFrame(t, win, &r)
	if len(win.side.hits.groups) != 1 {
		t.Fatalf("%d group headers, want 1", len(win.side.hits.groups))
	}
	sideClick(win, &r, win.side.hits.groups[0].rect, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	sideFrame(t, win, &r)
	if !win.side.folded["key-X"] || hasRow(win, a.ID) || hasRow(win, c.ID) {
		t.Fatalf("after one click: folded %v, rows %+v", win.side.folded, win.side.hits.rows)
	}
	sideClick(win, &r, win.side.hits.groups[0].rect, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	sideFrame(t, win, &r)
	if win.side.folded["key-X"] || !hasRow(win, a.ID) || !hasRow(win, c.ID) {
		t.Fatalf("after two clicks: folded %v, rows %+v", win.side.folded, win.side.hits.rows)
	}
}

func TestToggleSidebarHidesAndSaves(t *testing.T) {
	win, _ := sideWindow(t, true)
	openIn(t, win, "")
	win.cfgPath = filepath.Join(t.TempDir(), "config.toml")
	var r input.Router
	sideFrame(t, win, &r)
	if win.side.hidden || len(win.side.hits.rows) != 1 {
		t.Fatalf("sidebar hidden %v with %d rows at start", win.side.hidden, len(win.side.hits.rows))
	}
	ctrlShiftB := key.Event{Name: "B", Modifiers: key.ModCtrl | key.ModShift, State: key.Press}
	win.route(layout.Context{}, []event.Event{ctrlShiftB}, 8, 16)
	sideFrame(t, win, &r)
	if !win.side.hidden || win.opt.Config.Sidebar {
		t.Fatalf("after Ctrl+Shift+B: hidden %v, config %v", win.side.hidden, win.opt.Config.Sidebar)
	}
	if win.gridMin.X != 8 {
		t.Fatalf("grid starts at x=%d with the sidebar hidden, want 8", win.gridMin.X)
	}
	b, err := os.ReadFile(win.cfgPath)
	if err != nil || !strings.Contains(string(b), "sidebar = false") {
		t.Fatalf("config file %q (%v), want sidebar = false", b, err)
	}
	win.route(layout.Context{}, []event.Event{ctrlShiftB}, 8, 16)
	sideFrame(t, win, &r)
	if win.side.hidden || !win.opt.Config.Sidebar {
		t.Fatal("a second Ctrl+Shift+B did not show the sidebar")
	}
	if b, _ := os.ReadFile(win.cfgPath); !strings.Contains(string(b), "sidebar = true") {
		t.Fatalf("config file %q, want sidebar = true", b)
	}
}

func TestNarrowWindowHidesTheSidebarWithoutSaving(t *testing.T) {
	win, _ := sideWindow(t, true)
	openIn(t, win, "")
	win.cfgPath = filepath.Join(t.TempDir(), "config.toml")
	var r input.Router
	sizedFrame(t, win, &r, 500, 600)
	if !win.side.hidden || len(win.side.hits.rows) != 0 || win.gridMin.X != 8 {
		t.Fatalf("500px wide: hidden %v, %d rows, grid x %d", win.side.hidden, len(win.side.hits.rows), win.gridMin.X)
	}
	if !win.opt.Config.Sidebar {
		t.Fatal("a narrow window turned the setting off")
	}
	if _, err := os.Stat(win.cfgPath); !os.IsNotExist(err) {
		t.Fatalf("a narrow window wrote the config (%v)", err)
	}
	sizedFrame(t, win, &r, 1000, 600)
	if win.side.hidden || len(win.side.hits.rows) != 1 {
		t.Fatal("widening the window did not bring the sidebar back")
	}
}

func TestHoverInvalidatesOnlyOnChange(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	b := openIn(t, win, "")
	var r input.Router
	sideFrame(t, win, &r)
	ra, rb := rowHit(t, win, a.ID).rect, rowHit(t, win, b.ID).rect
	for i := range 5 {
		p := centre(ra).Add(image.Pt(i, 0))
		r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: sidePt(win, p)})
	}
	sideFrame(t, win, &r)
	if win.side.redraws != 1 || win.side.st.hover != a.ID {
		t.Fatalf("5 moves over one row: %d redraws, hover %d; want 1 and %d", win.side.redraws, win.side.st.hover, a.ID)
	}
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: sidePt(win, centre(rb))})
	sideFrame(t, win, &r)
	if win.side.redraws != 2 || win.side.st.hover != b.ID {
		t.Fatalf("move to the next row: %d redraws, hover %d; want 2 and %d", win.side.redraws, win.side.st.hover, b.ID)
	}
}

func TestKeyboardSelectionScrollsIntoView(t *testing.T) {
	win, _ := sideWindow(t, true)
	var ids []tabs.TabID
	for range 40 {
		ids = append(ids, openIn(t, win, "").ID)
	}
	win.mgr.SetActive(ids[0])
	var r input.Router
	sideFrame(t, win, &r)
	if hasRow(win, ids[39]) {
		t.Fatal("the last of 40 rows is visible unscrolled; the test needs more rows")
	}
	win.runShortcut(layout.Context{}, actPrevTab)
	sideFrame(t, win, &r)
	if win.mgr.Active().ID != ids[39] {
		t.Fatalf("prev from the first went to %d, want the last", win.mgr.Active().ID)
	}
	h := rowHit(t, win, ids[39])
	if !h.rect.In(win.side.hits.list) || h.rect.Dy() != 60 {
		t.Fatalf("row %v is not wholly inside the list %v", h.rect, win.side.hits.list)
	}
	win.runShortcut(layout.Context{}, actNextTab)
	sideFrame(t, win, &r)
	if h := rowHit(t, win, ids[0]); h.rect.Dy() != 60 {
		t.Fatalf("wrapping to the first row left it cut: %v", h.rect)
	}
}

func TestMacHasNoTopBar(t *testing.T) {
	win, _ := sideWindow(t, false)
	openIn(t, win, "")
	var r input.Router
	sideFrame(t, win, &r)
	if win.gridMin.Y != 4 {
		t.Fatalf("grid starts at y=%d, want 4 (terminal at y=0)", win.gridMin.Y)
	}
	if win.side.at != (image.Point{}) {
		t.Fatalf("sidebar at %v, want the window's top-left", win.side.at)
	}
	h := win.side.hits
	if h.toggle.Empty() || h.gear.Empty() {
		t.Fatalf("toggle %v gear %v, want both in the sidebar", h.toggle, h.gear)
	}
	if !win.bar.Gear.Empty() || !win.bar.Toggle.Empty() {
		t.Fatalf("a top bar on macOS: %+v", win.bar)
	}
	sideClick(win, &r, h.gear, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if !win.set.shown {
		t.Fatal("the sidebar's gear did not open settings")
	}
	win.hideSettings()

	// Hidden, a 36dp rail keeps the toggle and the gear.
	sideClick(win, &r, win.side.hits.toggle, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	sideFrame(t, win, &r)
	if !win.side.hidden || win.opt.Config.Sidebar {
		t.Fatal("the sidebar's toggle did not hide it")
	}
	if win.gridMin.X != 36+8 || win.side.hits.toggle.Empty() || win.side.hits.gear.Empty() || len(win.side.hits.rows) != 0 {
		t.Fatalf("rail: grid x %d, hits %+v", win.gridMin.X, win.side.hits)
	}
	sideClick(win, &r, win.side.hits.toggle, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	sideFrame(t, win, &r)
	if win.side.hidden {
		t.Fatal("the rail's toggle did not bring the sidebar back")
	}
}

// Windows keeps the 38dp bar: toggle, mark, drag area, gear, caption buttons.
func TestWindowsBarHasTheToggleAndGear(t *testing.T) {
	win, _ := sideWindow(t, true)
	openIn(t, win, "")
	var r input.Router
	sideFrame(t, win, &r)
	b := win.bar
	if b.Toggle.Empty() || b.Toggle.Min.X != 0 || b.Gear.Empty() || b.Drag.Empty() {
		t.Fatalf("bar %+v", b)
	}
	if !(b.Toggle.Max.X <= b.Drag.Min.X && b.Drag.Max.X <= b.Gear.Min.X && b.Gear.Max.X <= b.Buttons[0].Min.X) {
		t.Fatalf("bar out of order: %+v", b)
	}
	if win.side.at != image.Pt(0, 38) || win.gridMin != image.Pt(260+8, 38+4) {
		t.Fatalf("sidebar at %v, grid at %v", win.side.at, win.gridMin)
	}
	if !win.side.hits.toggle.Empty() {
		t.Fatal("the sidebar draws its own toggle under a custom title bar")
	}
	click(win, b.Toggle)
	sideFrame(t, win, &r)
	if !win.side.hidden {
		t.Fatal("the title bar's toggle did not hide the sidebar")
	}
}

func TestSettingsRowShowsAndCloses(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	b := openIn(t, win, "")
	win.mgr.SetActive(a.ID)
	var r input.Router
	sideFrame(t, win, &r)
	if !win.side.hits.settings.Empty() {
		t.Fatal("a Settings row with settings closed")
	}
	win.openSettings()
	sideFrame(t, win, &r)
	if win.side.hits.settings.Empty() {
		t.Fatal("no Settings row with settings open")
	}
	sideClick(win, &r, rowHit(t, win, b.ID).rect, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if !win.set.open || win.set.shown || win.mgr.Active().ID != b.ID {
		t.Fatalf("row click over settings: open %v shown %v active %d", win.set.open, win.set.shown, win.mgr.Active().ID)
	}
	sideClick(win, &r, win.side.hits.settings, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if !win.set.shown {
		t.Fatal("clicking the Settings row did not show settings")
	}
	sideClick(win, &r, win.side.hits.settingsClose, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	sideFrame(t, win, &r)
	if win.set.open || !win.side.hits.settings.Empty() {
		t.Fatalf("the Settings row's ×: open %v row %v", win.set.open, win.side.hits.settings)
	}
}

func TestPlusOpensASessionAndMemberClickShowsIt(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	m := openMember(t, win, "api")
	win.mgr.SetActive(a.ID)
	var r input.Router
	sideFrame(t, win, &r)
	if len(win.side.hits.members) != 1 {
		t.Fatalf("%d member hits, want 1", len(win.side.hits.members))
	}
	sideClick(win, &r, win.side.hits.members[0].rect, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if win.mgr.Active().ID != a.ID || win.fl.owner(m.ID) != a.ID {
		t.Fatalf("member click: active %d, floating over %d; want it floating over %d", win.mgr.Active().ID, win.fl.owner(m.ID), a.ID)
	}
	n := len(win.mgr.Tabs())
	sideClick(win, &r, win.side.hits.plus, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if len(win.mgr.Tabs()) != n+1 {
		t.Fatalf("+ left %d tabs, want %d", len(win.mgr.Tabs()), n+1)
	}
	// A run header folds its members away.
	sideFrame(t, win, &r)
	if len(win.side.hits.runs) != 1 {
		t.Fatalf("%d run headers, want 1", len(win.side.hits.runs))
	}
	sideClick(win, &r, win.side.hits.runs[0].rect, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	sideFrame(t, win, &r)
	if !win.side.st.runFolded[1] || len(win.side.hits.members) != 0 {
		t.Fatalf("run header click: folded %v, %d members", win.side.st.runFolded, len(win.side.hits.members))
	}
}

func TestWheelScrollsTheList(t *testing.T) {
	win, _ := sideWindow(t, true)
	for range 30 {
		openIn(t, win, "")
	}
	var r input.Router
	sideFrame(t, win, &r)
	at := sidePt(win, centre(win.side.hits.list))
	r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: at, Scroll: f32.Pt(0, 120)})
	sideFrame(t, win, &r)
	if win.side.st.scroll != 120 {
		t.Fatalf("scroll %d after one wheel notch, want 120", win.side.st.scroll)
	}
	r.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse, Position: at, Scroll: f32.Pt(0, 1<<16)})
	sideFrame(t, win, &r)
	if win.side.st.scroll != win.side.hits.scrollMax {
		t.Fatalf("scroll %d past the end, want clamped to %d", win.side.st.scroll, win.side.hits.scrollMax)
	}
}

// An alert shows on the session's sidebar row and on a member's line.
func TestAlertMarksShowInTheSidebar(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	m := openMember(t, win, "api")
	win.alerts.marks = map[tabs.TabID]mark{a.ID: markNeeds, m.ID: markDone}
	sm := buildSide(win.sideIn())
	if len(sm.Loose) != 1 || sm.Loose[0].Mark != markNeeds {
		t.Fatalf("rows %+v, want a marked needs", sm.Loose)
	}
	runs := win.sideRuns()
	if len(runs) != 1 || runs[0].Members[0].Mark != markDone {
		t.Fatalf("runs %+v, want api marked done", runs)
	}
}

// Runes no font has are dropped before the title is shortened, so they never eat the budget.
func TestSidebarTitleDropsUnusableRunesFirst(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	s := a.Session
	s.Lock()
	vt.NewParser(s.Applier()).Feed([]byte("\x1b]0;" + strings.Repeat("\u200B", 40) + "short\x07"))
	s.Unlock()
	sm := buildSide(win.sideIn())
	if len(sm.Loose) != 1 || sm.Loose[0].Title != "short" {
		t.Fatalf("rows %+v, want the title short", sm.Loose)
	}
}

func TestMemberColorFollowsTheTheme(t *testing.T) {
	win, _ := sideWindow(t, true)
	win.applyTheme(mustTheme(t, "sumi"))
	if got := win.ui.memberColor("magenta"); got != win.pal.ANSI[5] {
		t.Fatalf("magenta member %v, want the theme's ANSI 5 %v", got, win.pal.ANSI[5])
	}
}

func TestShortFolderKeepsTheLastTwoParts(t *testing.T) {
	if got := shortFolder(filepath.Join(`C:\Users\john\Desktop`, "samurai_cart_v3", "backend")); got != filepath.Join("samurai_cart_v3", "backend") {
		t.Fatalf("short folder %q", got)
	}
	if got := shortFolder(""); got != "" {
		t.Fatalf("empty folder became %q", got)
	}
}

func memberHit(t *testing.T, win *window, id tabs.TabID) sideRowHit {
	t.Helper()
	for _, h := range win.side.hits.members {
		if h.id == id {
			return h
		}
	}
	t.Fatalf("no member line for tab %d in %+v", id, win.side.hits.members)
	return sideRowHit{}
}

func TestHoveredMemberShowsAnXAndMiddleClickCloses(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	m := openMember(t, win, "api")
	n := openMember(t, win, "web")
	win.runs[1].cancel = func() {} // closing members can settle or end the run
	win.mgr.SetActive(a.ID)
	var r input.Router
	sideFrame(t, win, &r)
	if !memberHit(t, win, m.ID).close.Empty() {
		t.Fatal("a member line shows a × without hover")
	}
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: sidePt(win, centre(memberHit(t, win, m.ID).rect))})
	sideFrame(t, win, &r)
	sideFrame(t, win, &r) // draws the × the hover asked for
	if win.side.st.hover != m.ID {
		t.Fatalf("hover %d over the member line, want %d", win.side.st.hover, m.ID)
	}
	x := memberHit(t, win, m.ID).close
	if x.Empty() {
		t.Fatal("a hovered member line shows no ×")
	}
	sideClick(win, &r, x, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if win.mgr.Has(m.ID) || win.mgr.Active().ID != a.ID {
		t.Fatalf("× on a member: open %v, active %d, want closed and a (%d) shown", win.mgr.Has(m.ID), win.mgr.Active().ID, a.ID)
	}
	sideFrame(t, win, &r)
	sideClick(win, &r, memberHit(t, win, n.ID).rect, pointer.ButtonTertiary)
	sideFrame(t, win, &r)
	if win.mgr.Has(n.ID) {
		t.Fatal("a middle click on a member line did not close it")
	}
}

func TestMemberMarksShowOnTheirLinesAndAFoldedHeader(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, "")
	m := openMember(t, win, "api")
	n := openMember(t, win, "web")
	win.mgr.SetActive(a.ID)
	win.alerts.marks = map[tabs.TabID]mark{m.ID: markDone, n.ID: markNeeds}
	if got := win.sideRuns()[0].Mark; got != markNeeds {
		t.Fatalf("run mark %v, want needs over done", got)
	}
	var r input.Router
	sideFrame(t, win, &r)
	if got := memberHit(t, win, m.ID).mark; got != markDone {
		t.Fatalf("api line drew mark %v, want done", got)
	}
	if got := memberHit(t, win, n.ID).mark; got != markNeeds {
		t.Fatalf("web line drew mark %v, want needs", got)
	}
	if got := win.side.hits.runs[0].mark; got != markNone {
		t.Fatalf("an open run header drew mark %v; its lines show the marks", got)
	}
	sideClick(win, &r, win.side.hits.runs[0].rect, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	sideFrame(t, win, &r)
	if len(win.side.hits.runs) != 1 || win.side.hits.runs[0].mark != markNeeds {
		t.Fatalf("folded run header hits %+v, want it marked needs", win.side.hits.runs)
	}
}

func TestToggleInANarrowWindowChangesNothing(t *testing.T) {
	for _, custom := range []bool{true, false} {
		win, _ := sideWindow(t, custom)
		openIn(t, win, "")
		win.cfgPath = filepath.Join(t.TempDir(), "config.toml")
		var r input.Router
		sizedFrame(t, win, &r, 500, 600)
		win.route(layout.Context{}, []event.Event{key.Event{Name: "B", Modifiers: key.ModCtrl | key.ModShift, State: key.Press}}, 8, 16)
		sizedFrame(t, win, &r, 500, 600)
		if custom {
			click(win, win.bar.Toggle)
		} else {
			sideClick(win, &r, win.side.hits.toggle, pointer.ButtonPrimary)
		}
		sizedFrame(t, win, &r, 500, 600)
		if !win.opt.Config.Sidebar {
			t.Fatalf("custom %v: the toggle in a narrow window turned the setting off", custom)
		}
		if _, err := os.Stat(win.cfgPath); !os.IsNotExist(err) {
			t.Fatalf("custom %v: the toggle in a narrow window wrote the config (%v)", custom, err)
		}
		sizedFrame(t, win, &r, 1000, 600)
		if win.side.hidden {
			t.Fatalf("custom %v: widening did not show the sidebar", custom)
		}
	}
}

// A Stop or Notification alert re-reads a background session's branch and repo.
func TestAlertRereadsABackgroundBranch(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, osPath("/code/X/a"))
	b := openIn(t, win, osPath("/code/X/b"))
	win.mgr.SetActive(a.ID)
	branch := "one"
	win.branchFn = func(string) string { return branch }
	var r input.Router
	sideFrame(t, win, &r)
	branch = "two"
	for _, k := range []notify.Kind{notify.KindDone, notify.KindNeeds} {
		deliverNow(win, notify.Msg{Tab: int(b.ID), Kind: k}, time.Unix(1000, 0))
		groups := buildSide(win.sideIn()).Groups
		if len(groups) != 1 {
			t.Fatalf("after a %s alert: %d groups, want a and b in one", k, len(groups))
		}
		var got string
		for _, row := range groups[0].Rows {
			if row.ID == b.ID {
				got = row.Branch
			}
		}
		if got != branch {
			t.Fatalf("after a %s alert b's row reads branch %q, want %q", k, got, branch)
		}
		branch = "three"
	}
}

func TestSideTabChangesGroupOnNewReport(t *testing.T) {
	win, _ := sideWindow(t, true)
	a := openIn(t, win, osPath("/code/X/a"))
	openIn(t, win, osPath("/code/X/b"))
	if g := buildSide(win.sideIn()).Groups; len(g) != 1 || g[0].Key != "key-X" || len(g[0].Rows) != 2 {
		t.Fatalf("groups %+v, want both in X", g)
	}
	report(t, a, osPath("/code/Y/a"))
	g := buildSide(win.sideIn()).Groups
	byKey := map[string][]sideRow{}
	for _, x := range g {
		byKey[x.Key] = x.Rows
	}
	if len(g) != 2 || len(byKey["key-Y"]) != 1 || byKey["key-Y"][0].ID != a.ID || len(byKey["key-X"]) != 1 {
		t.Fatalf("groups %+v, want a moved to Y", g)
	}
}

// needsWindow opens n sessions and marks the ones at needs as waiting on the owner.
func needsWindow(t *testing.T, n int, needs ...int) (*window, []tabs.TabID) {
	t.Helper()
	win, _ := sideWindow(t, true)
	var ids []tabs.TabID
	for range n {
		ids = append(ids, openIn(t, win, "").ID)
	}
	win.alerts.marks = map[tabs.TabID]mark{}
	for _, i := range needs {
		win.alerts.marks[ids[i]] = markNeeds
	}
	win.mgr.SetActive(ids[0])
	return win, ids
}

func TestNextWaitingCycles(t *testing.T) {
	win, ids := needsWindow(t, 4, 1, 3)
	for i, want := range []tabs.TabID{ids[1], ids[3], ids[1]} {
		win.nextWaiting()
		if got := win.mgr.Active().ID; got != want {
			t.Fatalf("press %d showed %d, want %d", i+1, got, want)
		}
	}
}

func TestNextWaitingNoneShowsNotice(t *testing.T) {
	win, ids := needsWindow(t, 3)
	win.mgr.SetActive(ids[1])
	win.nextWaiting()
	if win.notice != "Nothing is waiting on you." || !win.noticeUntil.After(time.Now()) {
		t.Fatalf("notice %q until %v", win.notice, win.noticeUntil)
	}
	if got := win.mgr.Active().ID; got != ids[1] {
		t.Fatalf("shown tab changed to %d", got)
	}
}

func TestNextWaitingButton(t *testing.T) {
	win, ids := needsWindow(t, 3, 2)
	var r input.Router
	sideFrame(t, win, &r)
	h := win.side.hits
	if h.next.Empty() {
		t.Fatal("no Next waiting button was drawn")
	}
	if h.next.Overlaps(h.list) {
		t.Fatalf("button %v overlaps the list %v", h.next, h.list)
	}
	sideClick(win, &r, h.next, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if got := win.mgr.Active().ID; got != ids[2] {
		t.Fatalf("click showed %d, want %d", got, ids[2])
	}
}

func TestNextWaitingButtonSitsAboveWorkspaces(t *testing.T) {
	win, _ := needsWindow(t, 2, 1)
	openMember(t, win, "api")
	var r input.Router
	sideFrame(t, win, &r)
	h := win.side.hits
	if h.next.Empty() || len(h.members) == 0 {
		t.Fatalf("button %v members %d", h.next, len(h.members))
	}
	if h.next.Max.Y > h.members[0].rect.Min.Y {
		t.Fatalf("button %v is not above the workspaces section (first member %v)", h.next, h.members[0].rect)
	}
}

func TestMacSidebarPaletteButton(t *testing.T) {
	win, _ := sideWindow(t, false)
	openIn(t, win, "")
	var r input.Router
	sideFrame(t, win, &r)
	h := win.side.hits
	if h.palette.Empty() || h.palette.Min.X < h.toggle.Max.X || h.palette.Max.X > h.gear.Min.X || h.palette.Overlaps(h.toggle) || h.palette.Overlaps(h.gear) {
		t.Fatalf("palette %v, toggle %v, gear %v", h.palette, h.toggle, h.gear)
	}
	sideClick(win, &r, h.palette, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if !win.palette.open {
		t.Fatal("the palette button did not open the palette")
	}
}

func TestMacRailPaletteButton(t *testing.T) {
	win, _ := sideWindow(t, false)
	openIn(t, win, "")
	var r input.Router
	win.toggleSidebar()
	sideFrame(t, win, &r)
	h := win.side.hits
	if !win.side.hidden || h.palette.Empty() || h.palette.Min.Y < h.toggle.Max.Y || h.palette.Max.Y > h.gear.Min.Y {
		t.Fatalf("hidden %v: toggle %v, palette %v, gear %v", win.side.hidden, h.toggle, h.palette, h.gear)
	}
	sideClick(win, &r, h.palette, pointer.ButtonPrimary)
	sideFrame(t, win, &r)
	if !win.palette.open {
		t.Fatal("the rail's palette button did not open the palette")
	}
}
