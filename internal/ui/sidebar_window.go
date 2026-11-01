package ui

import (
	"image"
	"slices"
	"strings"
	"time"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	kinput "github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const (
	// minSideWindow is the narrowest window, in dp, that still shows the sidebar.
	minSideWindow = 560
	// doubleClick is the longest gap between two presses on a row's title that renames it.
	doubleClick = 500 * time.Millisecond
)

// sideUI is the window's sidebar: the state the drawing needs, the folds kept
// while kaze runs and where the last frame put things. UI goroutine only.
type sideUI struct {
	st        sideState
	folded    map[string]bool // repo groups the owner folded, by key
	hidden    bool            // off in the settings, or the window is too narrow
	w         int             // this frame's sidebar width in px, set once at the frame's start
	hits      sideHits        // sidebar-local, like the pointer events on tag
	at        image.Point     // the sidebar's top-left in the window
	tag       int             // pointer events on the sidebar or the rail
	reveal    bool            // scroll the shown row into view next frame
	redraws   int             // frames the sidebar's pointer input asked for; tests count them
	ed        widget.Editor   // the filter field
	filtering bool            // the filter field has the keyboard
	narrow    bool            // the window is too narrow for the sidebar
	filters   []event.Filter  // the filter's own keys, built once
	ren       widget.Editor   // the title of the row being renamed
	renaming  tabs.TabID      // 0 when no row is being renamed
	renFrom   string          // the text the rename started with
	renKeys   []event.Filter  // the rename editor's own keys, built once
	lastClick titleClick      // the last press on a row's title, for a double click
}

// titleClick is a press on a session row's title.
type titleClick struct {
	id tabs.TabID
	at time.Duration
}

// isMember reports whether t belongs to an open workspace run; members are not sessions.
func (win *window) isMember(t *tabs.Tab) (uint64, bool) {
	if t.Group != nil && win.runs[t.Group.Run] != nil {
		return t.Group.Run, true
	}
	return 0, false
}

// sideIn is the sidebar model's input, unfiltered and without the workspaces section.
func (win *window) sideIn() sideInput {
	in := sideInput{
		Tabs:     win.mgr.Tabs(),
		Loc:      win.locOf,
		Mark:     win.alerts.markOf,
		Member:   win.isMember,
		Folded:   win.side.folded,
		Home:     win.home,
		DefTitle: win.defTitle,
		Agent:    win.rowAgent,
		Name:     func(id tabs.TabID) string { return win.names[id] },
		Lines:    win.linesOf,
		Agents:   win.sideAgents,
	}
	if win.ui != nil {
		in.Clean = win.ui.fit
	}
	return in
}

// sessionIDs is every session in sidebar order, folded groups included, members left out.
func (win *window) sessionIDs() []tabs.TabID {
	return sessionOrder(buildSide(win.sideIn()))
}

// selectSession shows a session picked by keyboard and scrolls its row into view.
func (win *window) selectSession(id tabs.TabID) {
	win.selectTab(id)
	win.side.reveal = true
	win.w.Invalidate()
}

// closeTab closes a tab. When it is the shown one, the next session in sidebar
// order (else the one before) is shown first, so closing never lands on a member
// while a session is left; with none left, the first open member is shown.
func (win *window) closeTab(id tabs.TabID) {
	win.closeScratch(id)
	if a := win.mgr.Active(); a != nil && a.ID == id {
		if next := win.afterClose(id); next != 0 {
			win.mgr.SetActive(next)
			win.side.reveal = true
		}
	}
	win.mgr.Close(id)
	win.w.Invalidate()
}

func (win *window) afterClose(id tabs.TabID) tabs.TabID {
	order := win.sessionIDs()
	switch i := slices.Index(order, id); {
	case i >= 0 && i+1 < len(order):
		return order[i+1]
	case i > 0:
		return order[i-1]
	case i < 0 && len(order) > 0:
		return order[0]
	}
	for _, t := range win.mgr.Tabs() {
		if _, ok := win.isMember(t); ok && t.ID != id && t.Parent == 0 {
			return t.ID
		}
	}
	return 0
}

// toggleSidebar shows or hides the sidebar and saves the choice. While only the
// window's width hides it, flipping the setting would change nothing visible, so it is a no-op.
func (win *window) toggleSidebar() {
	if win.side.narrow && win.opt.Config.Sidebar {
		return
	}
	win.dropSideEdit()
	on := !win.opt.Config.Sidebar
	win.opt.Config.Sidebar = on
	win.side.st.hover = 0
	win.saveSetting("sidebar", on)
}

// sideWidth is how wide the sidebar is in a window this wide (0 while hidden),
// and sets hidden. A narrow window hides it without touching the setting.
func (win *window) sideWidth(gtx layout.Context, width int) int {
	win.side.narrow = width < gtx.Dp(minSideWindow)
	on := win.opt.Config.Sidebar && !win.side.narrow
	win.side.hidden = !on
	if !on {
		win.dropSideEdit() // an invisible field must not keep the keyboard
	}
	if !on {
		return 0 // its buttons live in the title bar
	}
	return min(gtx.Dp(sidebarW), width)
}

// drawSide draws the sidebar into r and
// takes its pointer input there.
func (win *window) drawSide(gtx layout.Context, r image.Rectangle) {
	s := &win.side
	s.at = r.Min
	if r.Empty() {
		s.hits, s.st.hover, s.reveal = sideHits{}, 0, false
		return
	}
	ops := gtx.Ops
	off := op.Offset(r.Min).Push(ops)
	defer off.Pop()
	sg := gtx
	sg.Constraints = layout.Exact(r.Size())
	{ // r is this frame's sidebar, decided before any toggle in it: draw it, never leave it bare
		if s.renaming != 0 && win.mgr.Lookup(s.renaming) == nil {
			win.endRename(false) // its tab closed
		}
		in := win.sideIn()
		s.st.filter, s.st.ed = s.ed.Text(), &s.ed
		s.st.renaming, s.st.ren = s.renaming, &s.ren
		in.Filter = s.st.filter
		in.Runs = win.curRuns()
		m := buildSide(in)
		s.st.shown = 0
		if a := win.mgr.Active(); a != nil {
			s.st.shown = a.ID
		}
		s.st.settings, s.st.settingsOn = win.set.open, win.set.shown
		s.st.floating = 0
		if set := win.fl.sets[win.paneOwner()]; set != nil {
			if f := set.front(); f != nil && f.kind == paneMember {
				s.st.floating = f.id
			}
		}
		s.st.nextKey = win.nextWaitingKey()
		rec := op.Record(ops)
		h := win.ui.layoutSidebar(sg, m, s.st, r.Size())
		call := rec.Stop()
		if s.reveal {
			s.reveal = false
			if sc, ok := revealScroll(sg, m, s.st, h); ok && sc != s.st.scroll {
				// Laid out again at the new scroll; only this recording is drawn.
				s.st.scroll = sc
				rec = op.Record(ops)
				h = win.ui.layoutSidebar(sg, m, s.st, r.Size())
				call = rec.Stop()
			}
		}
		call.Add(ops)
		s.hits = h
		if s.renaming != 0 && h.rename.Empty() {
			// Gio drops focus from an editor it no longer lays out; give the keyboard back.
			win.endRename(true)
		}
		s.st.scroll = min(max(s.st.scroll, 0), h.scrollMax)
	}
	area := clip.Rect{Max: r.Size()}.Push(ops)
	event.Op(ops, &s.tag)
	area.Pop()
}

// revealScroll is the scroll that shows the shown row whole, if it is in an open group.
func revealScroll(gtx layout.Context, m sideModel, st sideState, h sideHits) (int, bool) {
	gH, rH := gtx.Dp(sideGroupH), gtx.Dp(sideRowH)
	y, top, found := 0, 0, false
	rows := func(rs []sideRow) {
		for _, r := range rs {
			if r.ID == st.shown && !found {
				top, found = y, true
			}
			y += rowSpan(gtx, r)
		}
	}
	for _, g := range m.Groups {
		y += gH
		if !g.Folded {
			rows(g.Rows)
		}
	}
	rows(m.Loose)
	if !found {
		return 0, false
	}
	sc := min(max(st.scroll, 0), h.scrollMax)
	sc = max(min(sc, top), top+rH-h.list.Dy())
	return min(max(sc, 0), h.scrollMax), true
}

func (win *window) sideEvents(gtx layout.Context) {
	kinds := pointer.Press | pointer.Scroll | pointer.Move | pointer.Enter | pointer.Leave
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &win.side.tag, Kinds: kinds, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok && !win.bench.IgnoreInput() {
			win.sidePointer(e)
		}
	}
}

// edKeys are a sidebar editor's own keys, taken before the editor sees them: Esc,
// Tab and kaze's Ctrl / Cmd shortcuts.
func (win *window) edKeys(ed *widget.Editor) []event.Filter {
	fs := []event.Filter{
		key.Filter{Focus: ed, Name: key.NameEscape, Optional: allMods},
		key.Filter{Focus: ed, Name: key.NameTab, Optional: allMods},
	}
	for sc := range win.km {
		if sc.Mods&(key.ModCtrl|key.ModCommand) != 0 {
			fs = append(fs, key.Filter{Focus: ed, Name: sc.Name, Required: sc.Mods})
		}
	}
	return fs
}

// edShortcuts runs the keys edKeys caught for ed, esc on Esc; it reports whether a
// kaze shortcut ran.
func (win *window) edShortcuts(gtx layout.Context, ed *widget.Editor, keys []event.Filter, esc func()) bool {
	for {
		ev, ok := gtx.Event(keys...)
		if !ok {
			return false
		}
		if e, ok := ev.(key.Event); ok && e.State == key.Press && !win.bench.IgnoreInput() && win.edKey(gtx, ed, e, esc) {
			return true
		}
	}
}

// filterEvents takes Esc, Enter and kaze's Ctrl / Cmd shortcuts before the editor,
// which gets every other key.
func (win *window) filterEvents(gtx layout.Context) {
	s := &win.side
	s.ed.SingleLine, s.ed.Submit = true, true
	if s.filters == nil {
		s.filters = win.edKeys(&s.ed)
	}
	clearFilter := func() {
		s.ed.SetText("")
		win.leaveFilter()
	}
	if win.edShortcuts(gtx, &s.ed, s.filters, clearFilter) {
		return // a shortcut ran; the layout drains what is left
	}
	// Layout drains Update itself and would drop a change still queued; take it first.
	for {
		ev, ok := s.ed.Update(gtx)
		if !ok {
			return
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			win.ui.fit(s.ed.Text()) // loads a fallback font for any rune the bundled ones lack
			s.st.scroll = 0
			win.sideRedraw()
		case widget.SubmitEvent:
			win.filterEnter()
		}
	}
}

// edKey handles one key edShortcuts caught; it reports whether a kaze shortcut ran.
func (win *window) edKey(gtx layout.Context, ed *widget.Editor, e key.Event, esc func()) bool {
	if a, ok := win.km[kinput.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
		switch a {
		case actPaste:
			gtx.Execute(clipboard.ReadCmd{Tag: ed})
		case actCopy:
			win.writeClipboard(gtx, ed.SelectedText())
		default:
			win.runShortcut(gtx, a)
			return true
		}
		return false
	}
	if e.Name == key.NameEscape {
		esc()
	}
	return false
}

// renameEvents runs the rename editor's keys the way filterEvents runs the filter's:
// Enter saves, Esc cancels, kaze's shortcuts still work.
func (win *window) renameEvents(gtx layout.Context) {
	s := &win.side
	s.ren.SingleLine, s.ren.Submit, s.ren.MaxLen = true, true, maxSideText
	if s.renKeys == nil {
		s.renKeys = win.edKeys(&s.ren)
	}
	if win.edShortcuts(gtx, &s.ren, s.renKeys, func() { win.endRename(false) }) {
		return
	}
	for {
		ev, ok := s.ren.Update(gtx)
		if !ok {
			return
		}
		switch ev.(type) {
		case widget.ChangeEvent:
			win.ui.fit(s.ren.Text()) // loads a fallback font for any rune the bundled ones lack
			win.sideRedraw()
		case widget.SubmitEvent:
			win.endRename(true)
		}
	}
}

// startRename puts an editor over id's title, holding the title all selected, and
// gives it the keyboard. Workspace tabs keep their fixed titles.
func (win *window) startRename(id tabs.TabID) {
	t := win.mgr.Lookup(id)
	if t == nil || t.FixedTitle != "" {
		return
	}
	s := &win.side
	s.renaming = id
	s.renFrom = win.sideIn().renameText(t, win.locOf(t), win.agentOf(id))
	s.ren.SetText(s.renFrom)
	s.ren.SetCaret(s.ren.Len(), 0)
	win.wantFocus = true
	win.sideRedraw()
}

// endRename closes the rename editor; save keeps a changed text as the row's name,
// and a blank one gives the row back to the title rule.
func (win *window) endRename(save bool) {
	s := &win.side
	id := s.renaming
	if id == 0 {
		return
	}
	s.renaming = 0
	win.wantFocus = true
	win.sideRedraw()
	name := strings.TrimSpace(s.ren.Text())
	if !save || name == s.renFrom {
		return // an untouched title must not pin what the rule would change later
	}
	if name == "" {
		delete(win.names, id)
		return
	}
	if win.names == nil {
		win.names = map[tabs.TabID]string{}
	}
	win.names[id] = name
}

// dropSideEdit gives the keyboard back from the sidebar's editors when the main view
// changes or the sidebar goes away: the filter keeps its text, a rename is saved.
func (win *window) dropSideEdit() {
	if win.side.filtering {
		win.side.filtering = false
		win.wantFocus = true
	}
	win.endRename(true)
}

// leaveFilter gives the keyboard back to the shown session or the settings page.
func (win *window) leaveFilter() {
	win.side.filtering = false
	win.wantFocus = true
	win.sideRedraw()
}

// filterEnter shows the first session the filter leaves visible and gives it the
// keyboard; on an empty filter it only gives the keyboard back.
func (win *window) filterEnter() {
	in := win.sideIn()
	in.Filter = win.side.ed.Text()
	in.Runs = win.sideRuns()
	if strings.TrimSpace(in.Filter) != "" {
		if ids := sessionOrder(buildSide(in)); len(ids) > 0 {
			win.selectSession(ids[0])
		}
	}
	win.leaveFilter()
}

// sideRedraw asks for one frame for a change the sidebar's input made.
func (win *window) sideRedraw() {
	win.side.redraws++
	win.w.Invalidate()
}

// sidePointer handles one pointer event on the sidebar. Hover redraws only when
// the hovered row changes, so moving the mouse over the sidebar stays cheap.
func (win *window) sidePointer(e pointer.Event) {
	s := &win.side
	p := e.Position.Round()
	switch e.Kind {
	case pointer.Move, pointer.Enter, pointer.Leave:
		var hover tabs.TabID
		if r, ok := s.hits.lineAt(p); ok && e.Kind != pointer.Leave {
			hover = r.id
		}
		if hover != s.st.hover {
			s.st.hover = hover
			win.sideRedraw()
		}
	case pointer.Scroll:
		if !p.In(s.hits.list) {
			return
		}
		if sc := min(max(s.st.scroll+int(e.Scroll.Y), 0), s.hits.scrollMax); sc != s.st.scroll {
			s.st.scroll = sc
			win.sideRedraw()
		}
	case pointer.Press:
		win.sidePress(p, e.Buttons, e.Time)
	}
}

// sidePress handles a click: a row or member line shows its tab, its × or a middle
// click closes it, a double click on a row's title renames it; headers fold; + opens
// a session. A click off the rename editor saves the rename.
func (win *window) sidePress(p image.Point, b pointer.Buttons, at time.Duration) {
	primary, middle := b.Contain(pointer.ButtonPrimary), b.Contain(pointer.ButtonTertiary)
	if !primary && !middle {
		return
	}
	h := &win.side.hits
	if win.side.renaming != 0 && p.In(h.rename) {
		return // a click inside the editor keeps renaming
	}
	win.endRename(true)
	last := win.side.lastClick
	win.side.lastClick = titleClick{}
	win.wantFocus = true
	win.side.filtering = primary && p.In(h.filter) // any other click leaves the field
	defer win.sideRedraw()
	switch {
	case p.In(h.settings):
		switch {
		case middle, primary && p.In(h.settingsClose):
			win.closeSettings()
		case primary:
			win.openSettings()
		}
		return
	case p.In(h.next):
		if primary {
			win.nextWaiting()
		}
		return
	case p.In(h.filter):
		return // the frame gives the field the keyboard
	}
	for _, a := range h.agents {
		if primary && p.In(a.rect) {
			if a.more {
				win.openBoard(a.tab)
			} else {
				win.openAgentPane(a.tab, a.key)
			}
			return
		}
	}
	if r, ok := h.lineAt(p); ok {
		switch {
		case middle, p.In(r.close):
			win.closeTab(r.id)
		default:
			if t := win.mgr.Lookup(r.id); t != nil {
				if _, member := win.isMember(t); member {
					win.floatMember(r.id)
					return
				}
			}
			win.selectTab(r.id)
			if !primary || !p.In(r.title) {
				break
			}
			if last.id == r.id && at >= last.at && at-last.at < doubleClick {
				win.startRename(r.id)
			} else {
				win.side.lastClick = titleClick{r.id, at}
			}
		}
		return
	}
	if !primary {
		return
	}
	for _, g := range h.groups {
		if p.In(g.rect) {
			if win.side.folded == nil {
				win.side.folded = map[string]bool{}
			}
			flip(win.side.folded, g.key)
			return
		}
	}
	for _, r := range h.runs {
		if p.In(r.rect) {
			if win.side.st.runFolded == nil {
				win.side.st.runFolded = map[uint64]bool{}
			}
			flip(win.side.st.runFolded, r.run)
			return
		}
	}
}

// lineAt is the session row or workspace member line under p.
func (h *sideHits) lineAt(p image.Point) (sideRowHit, bool) {
	for _, list := range [2][]sideRowHit{h.rows, h.members} {
		for _, r := range list {
			if p.In(r.rect) {
				return r, true
			}
		}
	}
	return sideRowHit{}, false
}

// flip folds k, or unfolds it by forgetting it.
func flip[K comparable](m map[K]bool, k K) {
	if m[k] {
		delete(m, k)
	} else {
		m[k] = true
	}
}

// nextWaiting shows the next session after the shown one that needs the owner,
// wrapping; with none it only says so in the status bar.
func (win *window) nextWaiting() {
	order := win.sessionIDs()
	from := -1
	if a := win.mgr.Active(); a != nil {
		from = slices.Index(order, a.ID)
	}
	for i := 1; i <= len(order); i++ {
		id := order[(from+i+len(order))%len(order)]
		if win.alerts.markOf(id) == markNeeds {
			win.selectSession(id)
			return
		}
	}
	win.note("Nothing is waiting on you.")
	win.w.Invalidate()
}

// nextWaitingKey is the bound shortcut's text for the button, "" when unbound.
func (win *window) nextWaitingKey() string {
	for sc, a := range win.km {
		if a == actNextWaiting {
			return shortcutText(sc)
		}
	}
	return ""
}

// shortcutText writes a shortcut the way the platform names it, like Ctrl+Shift+J or Cmd+J.
func shortcutText(sc kinput.Shortcut) string {
	var parts []string
	for _, m := range []struct {
		bit  key.Modifiers
		name string
	}{{key.ModCtrl, "Ctrl"}, {key.ModAlt, "Alt"}, {key.ModShift, "Shift"}, {key.ModCommand, "Cmd"}, {key.ModSuper, "Super"}} {
		if sc.Mods&m.bit != 0 {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, string(sc.Name)), "+")
}
