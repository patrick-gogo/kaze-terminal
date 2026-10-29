package ui

import (
	"image"
	"path/filepath"
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
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

var groupTitles = []string{"api", "admin", "storefront", "api logs"}
var groupColors = []string{"cyan", "magenta", "yellow", "green"}

// groupWindow has an ordinary tab, the samurai run's four members and another
// ordinary tab, the first one active. It returns the members' IDs.
func groupWindow(t *testing.T) (*window, *recStarter, []tabs.TabID) {
	t.Helper()
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	win.focused = true
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	run := &wsRun{id: 1, workspace: "w1", name: "samurai"}
	var ids []tabs.TabID
	for i, title := range groupTitles {
		tab, err := win.mgr.NewOpts(tabs.StartOpts{Title: title, Color: groupColors[i],
			Group: &tabs.Group{Workspace: "w1", Name: "samurai", Run: 1, Member: i}})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, tab.ID)
		run.members = append(run.members, &wsMember{tab: tab.ID, title: title, dir: filepath.Join(`C:\home`, "samurai_cart_v3", title)})
	}
	win.runs = map[uint64]*wsRun{1: run}
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	win.mgr.SetActive(win.mgr.Tabs()[0].ID)
	return win, st, ids
}

// drawGroupBar draws the strip and, when open, the dropdown, as a frame would.
func drawGroupBar(win *window) {
	var shown tabs.TabID
	if t := win.shownTab(); t != nil {
		shown = t.ID
	}
	list := win.mgr.Tabs()
	win.pruneGroupMenu(list)
	win.drawBar(barContext(1000), list, shown, 38)
	win.drawGroupMenu(pageContext(1000, 600), list, 38)
}

func pressPt(p image.Point) pointer.Event {
	return pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(p.X), float32(p.Y))}
}

// chipOf is the one group chip the last frame drew.
func chipOf(t *testing.T, win *window) groupHit {
	t.Helper()
	if len(win.ui.hits.groups) != 1 {
		t.Fatalf("%d group chips drawn, want 1", len(win.ui.hits.groups))
	}
	return win.ui.hits.groups[0]
}

func centre(r image.Rectangle) image.Point { return r.Min.Add(r.Size().Div(2)) }

func keyPress(name key.Name) key.Event { return key.Event{Name: name, State: key.Press} }

func TestGroupTakesOneStripItem(t *testing.T) {
	win, _, _ := groupWindow(t)
	drawGroupBar(win)
	h := win.ui.hits
	if len(h.tabs) != 2 || len(h.groups) != 1 {
		t.Fatalf("%d tabs and %d groups drawn, want 2 and 1", len(h.tabs), len(h.groups))
	}
	if g := h.groups[0].rect; !(h.tabs[0].rect.Max.X <= g.Min.X && g.Max.X <= h.tabs[1].rect.Min.X) {
		t.Fatalf("group at %v, want between %v and %v", g, h.tabs[0].rect, h.tabs[1].rect)
	}
}

func TestGroupChipLabel(t *testing.T) {
	win, _, ids := groupWindow(t)
	drawGroupBar(win)
	if got := chipOf(t, win).label; got != "samurai" {
		t.Fatalf("chip %q with an ordinary tab shown, want samurai", got)
	}
	win.mgr.SetActive(ids[1])
	drawGroupBar(win)
	if got := chipOf(t, win).label; got != "samurai › admin" {
		t.Fatalf("chip %q with admin shown, want %q", got, "samurai › admin")
	}
}

func TestGroupChipShortensTheMemberFirst(t *testing.T) {
	if got := groupLabel("samurai", strings.Repeat("x", 40)); got != "samurai › "+strings.Repeat("x", maxMemberLabel-1)+"…" {
		t.Fatalf("label %q, want the member part shortened", got)
	}
	win, _, ids := groupWindow(t)
	list := win.mgr.Tabs()
	full := win.ui.layoutTabBar(barContext(1000), list, list[0].ID, nil, "pwsh", 38, 1000, settingsChip{})
	short := chipOf(t, win).rect
	reserved := full - win.ui.hits.tabs[1].rect.Max.X - 4 // the + button's room
	win.ui.layoutTabBar(barContext(1000), list, ids[2], nil, "pwsh", 38, 1000, settingsChip{})
	long := chipOf(t, win).rect
	// Room for the chip without "› storefront", not with it.
	limit := (short.Max.X+long.Max.X)/2 + reserved
	win.ui.layoutTabBar(barContext(1000), list, ids[2], nil, "pwsh", 38, limit, settingsChip{})
	if len(win.ui.hits.groups) != 1 || chipOf(t, win).label != "samurai" {
		t.Fatalf("narrow strip drew groups %+v, want the chip without its member part", win.ui.hits.groups)
	}
}

func TestGroupMenuPickByClick(t *testing.T) {
	win, _, ids := groupWindow(t)
	drawGroupBar(win)
	win.barPress(pressPt(centre(chipOf(t, win).rect)))
	if win.grp.run != 1 {
		t.Fatalf("menu run %d after a chip click, want 1", win.grp.run)
	}
	drawGroupBar(win)
	if len(win.grp.rows) != 4 {
		t.Fatalf("%d menu rows, want 4", len(win.grp.rows))
	}
	win.groupPress(pressPt(centre(win.grp.rows[1].rect)))
	if win.mgr.Active().ID != ids[1] || win.grp.run != 0 || !win.wantFocus {
		t.Fatalf("after picking admin: active %d menu %d focus %v", win.mgr.Active().ID, win.grp.run, win.wantFocus)
	}
}

func TestGroupMenuPickByNumberNeverReachesTheTerminal(t *testing.T) {
	win, st, ids := groupWindow(t)
	win.openGroupMenu(1)
	if win.focusTarget() != event.Tag(&win.grp.tag) {
		t.Fatal("keyboard focus stays on the terminal while the menu is open")
	}
	win.groupKey(layout.Context{}, keyPress("3"))
	if win.mgr.Active().ID != ids[2] || win.grp.run != 0 {
		t.Fatalf("after 3: active %d menu %d, want storefront and closed", win.mgr.Active().ID, win.grp.run)
	}
	if win.focusTarget() != event.Tag(win.termTag) {
		t.Fatal("focus did not return to the terminal")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, p := range st.ptys {
		if p.sent() != "" {
			t.Fatalf("shell %d got %q", i, p.sent())
		}
	}
}

func TestGroupMenuArrowsEnterAndEsc(t *testing.T) {
	win, _, ids := groupWindow(t)
	win.openGroupMenu(1)
	win.groupKey(layout.Context{}, keyPress(key.NameUpArrow)) // wraps to the last
	win.groupKey(layout.Context{}, keyPress(key.NameUpArrow))
	win.groupKey(layout.Context{}, keyPress(key.NameDownArrow))
	win.groupKey(layout.Context{}, keyPress(key.NameReturn))
	if win.mgr.Active().ID != ids[3] {
		t.Fatalf("active %d after up, up, down, Enter; want api logs", win.mgr.Active().ID)
	}
	before := win.mgr.Active().ID
	win.openGroupMenu(1)
	if win.grp.kb != 3 {
		t.Fatalf("menu opened on row %d, want the shown member's row 3", win.grp.kb)
	}
	win.groupKey(layout.Context{}, keyPress(key.NameEscape))
	if win.grp.run != 0 || win.mgr.Active().ID != before {
		t.Fatal("Esc did not just close the menu")
	}
	win.openGroupMenu(1)
	win.groupKey(layout.Context{}, keyPress("9")) // no ninth member
	if win.grp.run != 1 {
		t.Fatal("a number with no member closed the menu")
	}
}

func TestGroupMenuClickOutsideAndChipToggleClose(t *testing.T) {
	win, _, _ := groupWindow(t)
	drawGroupBar(win)
	chip := centre(chipOf(t, win).rect)
	win.barPress(pressPt(chip))
	drawGroupBar(win)
	win.groupOutside(pressPt(image.Pt(900, 500)))
	if win.grp.run != 0 {
		t.Fatal("a click outside left the menu open")
	}
	win.barPress(pressPt(chip))
	win.barPress(pressPt(chip))
	if win.grp.run != 0 {
		t.Fatal("a second chip click left the menu open")
	}
	win.barPress(pressPt(chip))
	win.groupOutside(pressPt(chip)) // the same click reaches the outside area too
	if win.grp.run != 1 {
		t.Fatal("the chip click that opened the menu also closed it")
	}
}

func TestCtrlTabEntersGroupMembers(t *testing.T) {
	win, _, ids := groupWindow(t)
	ctrlTab := key.Event{Name: key.NameTab, Modifiers: key.ModCtrl, State: key.Press}
	win.route(layout.Context{}, []event.Event{ctrlTab}, 8, 16)
	if got := win.mgr.Active().ID; got != ids[0] {
		t.Fatalf("Ctrl+Tab from the tab before the group went to %d, want member 1 (%d)", got, ids[0])
	}
	win.route(layout.Context{}, []event.Event{ctrlTab}, 8, 16)
	if got := win.mgr.Active().ID; got != ids[1] {
		t.Fatalf("second Ctrl+Tab went to %d, want member 2 (%d)", got, ids[1])
	}
}

func TestGroupBadgeFollowsAlerts(t *testing.T) {
	win, _, ids := groupWindow(t)
	win.alerts.marks = map[tabs.TabID]mark{ids[0]: markDone}
	drawGroupBar(win)
	if got := chipOf(t, win).mark; got != markDone {
		t.Fatalf("badge mark %v with a done member, want done", got)
	}
	win.alerts.marks[ids[2]] = markNeeds
	drawGroupBar(win)
	if got := chipOf(t, win).mark; got != markNeeds {
		t.Fatalf("badge mark %v with a needs member, want needs over done", got)
	}
	th := win.ui.th
	if win.ui.badgeColor(markNeeds) != th.Needs || win.ui.badgeColor(markDone) != th.Done || win.ui.badgeColor(markNone) != th.Muted {
		t.Fatal("badge colors are not the theme's needs, done and muted")
	}
}

func TestGroupRowCloseClosesOneMemberAndLastClosesTheGroup(t *testing.T) {
	win, _, ids := groupWindow(t)
	drawGroupBar(win)
	win.barPress(pressPt(centre(chipOf(t, win).rect)))
	drawGroupBar(win)
	win.groupPress(pressPt(centre(win.grp.rows[0].close)))
	if win.mgr.Has(ids[0]) || win.grp.run != 1 {
		t.Fatalf("row × left api open (%v) or closed the menu (%d)", win.mgr.Has(ids[0]), win.grp.run)
	}
	drawGroupBar(win)
	if len(win.grp.rows) != 3 || len(win.ui.hits.groups) != 1 {
		t.Fatalf("%d rows, %d groups after closing one member, want 3 and 1", len(win.grp.rows), len(win.ui.hits.groups))
	}
	for range 3 {
		win.groupPress(pressPt(centre(win.grp.rows[0].close)))
		drawGroupBar(win)
	}
	if len(win.ui.hits.groups) != 0 || win.grp.run != 0 || len(win.ui.hits.tabs) != 2 {
		t.Fatalf("after the last member: %d groups, menu %d, %d tabs", len(win.ui.hits.groups), win.grp.run, len(win.ui.hits.tabs))
	}
}

func TestGroupMenuWorksOverSettings(t *testing.T) {
	win, _, ids := groupWindow(t)
	win.openSettings()
	drawGroupBar(win)
	win.barPress(pressPt(centre(chipOf(t, win).rect)))
	if win.grp.run != 1 || win.focusTarget() != event.Tag(&win.grp.tag) {
		t.Fatal("the chip did not open its menu over the settings page")
	}
	win.groupKey(layout.Context{}, keyPress("2"))
	if win.set.shown || win.mgr.Active().ID != ids[1] {
		t.Fatalf("picking over settings: shown %v active %d", win.set.shown, win.mgr.Active().ID)
	}
}

func TestGroupMenuRepaintsOnThemeSwitch(t *testing.T) {
	win, _, ids := groupWindow(t)
	win.applyTheme(mustTheme(t, "midnight"))
	win.openGroupMenu(1)
	win.groupKey(layout.Context{}, keyPress(key.NameDownArrow))
	drawGroupBar(win)
	sumi := mustTheme(t, "sumi")
	win.applyTheme(sumi)
	drawGroupBar(win)
	if win.grp.run != 1 || win.grp.kb != 1 || len(win.grp.rows) != 4 {
		t.Fatalf("theme switch changed the menu: run %d kb %d rows %d", win.grp.run, win.grp.kb, len(win.grp.rows))
	}
	if win.grp.bg != sumi.UI.ToastBG {
		t.Fatalf("menu card %v, want Sumi's %v", win.grp.bg, sumi.UI.ToastBG)
	}
	if got := win.ui.memberColor("magenta"); got != win.pal.ANSI[5] {
		t.Fatalf("magenta member %v, want the theme's ANSI 5 %v", got, win.pal.ANSI[5])
	}
	win.groupKey(layout.Context{}, keyPress(key.NameReturn))
	if win.mgr.Active().ID != ids[1] {
		t.Fatal("the kept row was not the one picked")
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

// frameGroupWindow is groupWindow ready for whole headless frames: real tags,
// default settings and member 1 (api) shown and still waiting to launch.
func frameGroupWindow(t *testing.T) (*window, *recStarter, []tabs.TabID, *input.Router) {
	t.Helper()
	win, st, ids := groupWindow(t)
	win.termTag, win.barTag, win.capTag, win.toastTag = new(int), new(int), new(int), new(int)
	win.opt.Config = config.Default()
	t0 := time.Unix(1000, 0)
	win.wsr.now = func() time.Time { return t0 }
	for _, m := range win.runs[1].members {
		m.run = workspaces.TabRun{State: workspaces.Waiting, Wait: workspaces.WaitDocker, Started: t0}
	}
	win.mgr.SetActive(ids[0])
	r := new(input.Router)
	sizedFrame(t, win, r, 1000, 600)
	return win, st, ids, r
}

func routerClick(r *input.Router, p image.Point) {
	at := f32.Pt(float32(p.X), float32(p.Y))
	r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: at},
	)
}

// mouseMode turns on button reporting in a tab's app, as vim or Claude Code would.
func mouseMode(t *testing.T, p *recPTY, s *term.Session) {
	t.Helper()
	go p.w.Write([]byte("\x1b[?1000h"))
	deadline := time.Now().Add(3 * time.Second)
	for !modes(s).MouseButton {
		if time.Now().After(deadline) {
			t.Fatal("the app never turned mouse reporting on")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestGroupMenuClosingClickStaysOffTheTerminal(t *testing.T) {
	win, st, ids, r := frameGroupWindow(t)
	st.mu.Lock()
	p := st.ptys[1] // member api
	st.mu.Unlock()
	mouseMode(t, p, win.mgr.Lookup(ids[0]).Session)
	win.openGroupMenu(1)
	sizedFrame(t, win, r, 1000, 600)
	routerClick(r, image.Pt(900, 450)) // in the grid, away from the card and the banner
	sizedFrame(t, win, r, 1000, 600)
	if win.grp.run != 0 {
		t.Fatal("a click in the terminal left the menu open")
	}
	if s := win.runs[1].members[0].run.State; s != workspaces.Waiting {
		t.Fatalf("the closing click took the waiting tab over: state %v", s)
	}
	// Writes reach the shell on their own goroutine; give a stray one time to land.
	for end := time.Now().Add(300 * time.Millisecond); time.Now().Before(end); time.Sleep(5 * time.Millisecond) {
		if got := p.sent(); got != "" {
			t.Fatalf("the closing click reached the app as %q", got)
		}
	}
}

func TestGroupMenuClickOnAnotherTabSwitchesInOneClick(t *testing.T) {
	win, _, _, r := frameGroupWindow(t)
	win.openGroupMenu(1)
	sizedFrame(t, win, r, 1000, 600)
	other := win.ui.hits.tabs[1]
	routerClick(r, centre(other.rect).Sub(image.Pt(other.rect.Dx()/4, 0)))
	sizedFrame(t, win, r, 1000, 600)
	if win.grp.run != 0 || win.mgr.Active().ID != other.id {
		t.Fatalf("one click on another tab: menu %d active %d, want closed and %d", win.grp.run, win.mgr.Active().ID, other.id)
	}
}

func TestPasteWhileGroupMenuIsOpen(t *testing.T) {
	win, _, _ := groupWindow(t)
	win.termTag = new(int)
	win.openGroupMenu(1)
	var r input.Router
	gtx := pageContext(1000, 600)
	gtx.Source = r.Source()
	win.groupKey(gtx, key.Event{Name: "V", Modifiers: key.ModCtrl | key.ModShift, State: key.Press})
	if win.grp.run != 0 {
		t.Fatal("paste left the menu open")
	}
	if !r.ClipboardRequested() {
		t.Fatal("paste with the menu open never asked for the clipboard")
	}
}

func TestShiftDigitDoesNotPickAMember(t *testing.T) {
	win, _, ids := groupWindow(t)
	before := win.mgr.Active().ID
	win.openGroupMenu(1)
	win.groupKey(layout.Context{}, key.Event{Name: "3", Modifiers: key.ModShift, State: key.Press})
	if win.grp.run != 1 || win.mgr.Active().ID != before || win.mgr.Active().ID == ids[2] {
		t.Fatalf("Shift+3: menu %d active %d", win.grp.run, win.mgr.Active().ID)
	}
}

func TestGroupMenuClosesWhenItsChipOverflows(t *testing.T) {
	win, _, _ := groupWindow(t)
	win.openGroupMenu(1)
	drawGroupBar(win)
	list := win.mgr.Tabs()
	win.drawBar(barContext(120), list, list[0].ID, 38) // room for the first tab only
	win.drawGroupMenu(pageContext(120, 600), list, 38)
	if len(win.ui.hits.groups) != 0 {
		t.Fatal("the chip still fit; the test needs a narrower strip")
	}
	if win.grp.run != 0 {
		t.Fatal("the menu stayed open with its chip gone")
	}
}
