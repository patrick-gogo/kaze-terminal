package ui

import (
	"gioui.org/font"
	"image"
	"image/color"
	"runtime/debug"
	"slices"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// settingsState is the Settings page: a special tab with no shell behind it.
// open means its row is in the sidebar; shown means it is in front instead of the terminal.
type settingsState struct {
	aboutCopy string // pending clipboard content, consumed by the About layout
	cmdCopy   string // the same for the Shortcuts section
	cmdFilter string // the Shortcuts section's search text

	open, shown       bool
	section           int             // index into the sections; never a later one
	focus             int             // index into controls; -1 when no control has focus
	navFocus          bool            // focus is on a sidebar entry instead of a control
	navIndex          int             // index into selectable nav entries
	scroll, scrollMax int             // vertical pane position, clamped after each layout
	scrollFocus       int             // last control revealed by keyboard focus
	pane              image.Rectangle // visible content area for pointer hits
	tag               int             // its address is the page's event tag

	// Laid out each frame.
	controls []control
	nav      []navHit
	texts    []string          // every string drawn, so tests can read the page
	cards    []image.Rectangle // every card drawn this frame, for tests
	// memberFolderX is where each list member's folder starts, so tests can check the columns.
	memberFolderX []int

	build    string            // the build line, read when the page opens
	sections []settingsSection // nil means defaultSections; tests swap in their own

	fields  map[string]*textField // text field editors by control id, kept across frames
	editing string                // the field that has the keyboard; "" when the page has it
}

type navHit struct {
	rect    image.Rectangle
	section int
}

// settingRow is one line of a section: label and dim hint on the left, controls on the right.
// A row with heading set is a small section label instead: the rows after it, up to the
// next heading, sit together in one card.
type settingRow struct {
	heading     string
	label, hint string
	monoHint    bool     // the hint is a path or command, drawn in the terminal font
	icon        string   // a line icon in a tile left of the label (see chrome.icon)
	pill, tone  string   // a status pill left of the controls, and its kind (ok, warn, off, run, err)
	preview     bool     // a terminal preview in the current theme, font size and cursor
	newCard     bool     // starts a card of its own even without a heading (one workspace each)
	lines       []string // extra lines between the label and the hint (the setup check)
	ctrls       []ctrl
	block       bool        // the controls sit under the label across the pane (theme swatches)
	members     []rowMember // a workspace's tabs, drawn as a table under the label; controls go below
}

// rowMember is one workspace tab in the list: its color, title, folder, commands
// in order, and what it waits for ("" for nothing).
type rowMember struct {
	color, title, folder string
	cmds                 []string
	tag                  string
}

// settingsSection is one entry of the page's menu. rows builds its content each
// frame from the window's state; act runs one of its controls (see activate);
// show runs each time the section comes into view; dialog, when it returns one,
// covers the page and takes every click. leave, when set, runs before the page
// leaves the section: false keeps it (the section asks first, then runs then).
type settingsSection struct {
	title, hint string
	icon        string                   // the menu's line icon (see chrome.icon)
	group       string                   // a menu group heading above it; "" for the first group
	badge       func(win *window) string // a count or word in a pill beside it in the menu, "" for none
	actions     func(win *window) []ctrl // buttons on the right of the section's heading
	later       bool                     // greyed with "coming next" and not selectable
	draw        func(*page, image.Rectangle, *window) int
	rows        func(win *window) []settingRow
	act         func(win *window, id string, arg int)
	show        func(win *window)
	dialog      func(win *window) *dialogBox
	leave       func(win *window, then func()) bool
}

// dialogBox is a confirm box over the page: a title, plain lines, code lines and buttons.
type dialogBox struct {
	title       string
	lines, code []string
	buttons     []ctrl
}

// defaultSections are the page's menu; each section fills in its own rows and act.
func defaultSections() []settingsSection {
	secs := []settingsSection{
		appearanceSection(),
		workspacesSection(),
		claudeSection(),
		commandsSection(),
		updatesSection(),
		aboutSection(),
	}
	icons := []string{"appearance", "workspaces", "claude", "shortcuts", "updates", "about"}
	for i := range secs {
		secs[i].icon = icons[i]
	}
	secs[4].group, secs[5].group = "Kaze", "Kaze"
	return secs
}

// readBuild is debug.ReadBuildInfo unless a test replaces it.
var readBuild = debug.ReadBuildInfo

func (win *window) sections() []settingsSection {
	if win.set.sections != nil {
		return win.set.sections
	}
	return defaultSections()
}

// openSettings opens the Settings tab, or shows it when it is already open.
func (win *window) openSettings() {
	win.dropSideEdit()
	win.board.open = false
	s := &win.set
	if !s.open {
		s.open, s.focus = true, -1
		s.navFocus = false
		s.section = 0
		for i, sec := range win.sections() {
			if !sec.later {
				s.section = i
				break
			}
		}
		info, ok := readBuild()
		s.build = version.Read(info, ok).Line()
	}
	if !s.shown {
		s.shown = true
		win.showSection()
		win.endGesture() // a held terminal drag must not stay stuck under the page
		win.wantFocus = true
	}
	win.w.Invalidate()
}

// hideSettings puts the terminal back in front; the Settings tab stays open.
func (win *window) hideSettings() {
	if win.set.shown {
		win.set.shown = false
		win.wantFocus = true
		win.w.Invalidate()
	}
}

// dialogShown reports whether a confirm dialog is up on the shown page.
func (win *window) dialogShown() bool {
	s := &win.set
	if !s.shown {
		return false
	}
	secs := win.sections()
	if s.section >= len(secs) || secs[s.section].dialog == nil {
		return false
	}
	return secs[s.section].dialog(win) != nil
}

// mayLeave asks the shown section whether the page may leave it; when it may
// not, the section has put up its question and runs then once answered.
func (win *window) mayLeave(then func()) bool {
	secs := win.sections()
	if !win.set.open || win.set.section >= len(secs) || secs[win.set.section].leave == nil {
		return true
	}
	return secs[win.set.section].leave(win, then)
}

// closeSettings closes the Settings tab, unless the shown section asks first;
// then the page comes to the front so the question is seen.
func (win *window) closeSettings() {
	if !win.mayLeave(win.closeSettings) {
		win.openSettings()
		return
	}
	win.hideSettings()
	s := &win.set
	s.open = false
	s.controls, s.nav = s.controls[:0], s.nav[:0]
	s.fields, s.editing = nil, ""
	win.w.Invalidate()
}

// cycleTab steps through the sessions in sidebar order, wrapping, with the open
// Settings row at the start. Workspace members are skipped; from a shown member
// next goes to the first session and prev to the last.
func (win *window) cycleTab(d int) {
	order := win.sessionIDs()
	if len(order) == 0 {
		return
	}
	edge := func() tabs.TabID {
		if d < 0 {
			return order[len(order)-1]
		}
		return order[0]
	}
	if win.set.shown {
		win.selectSession(edge())
		return
	}
	i := -1
	if a := win.mgr.Active(); a != nil {
		i = slices.Index(order, a.ID)
	}
	switch {
	case i < 0:
		win.selectSession(edge())
	case win.set.open && (d > 0 && i == len(order)-1 || d < 0 && i == 0):
		win.openSettings()
	default:
		win.selectSession(order[(i+d+len(order))%len(order)])
	}
}

// activate runs a control: the shown section decides what id and arg mean.
func (win *window) activate(id string, arg int) {
	secs := win.sections()
	if win.set.section < len(secs) {
		if act := secs[win.set.section].act; act != nil {
			act(win, id, arg)
		}
	}
	win.w.Invalidate()
}

func (win *window) selectSection(i int) {
	secs := win.sections()
	if i < 0 || i >= len(secs) || secs[i].later || i == win.set.section {
		return
	}
	if !win.mayLeave(func() { win.selectSection(i) }) {
		win.w.Invalidate()
		return
	}
	win.set.section, win.set.focus = i, -1
	if win.set.editing != "" {
		win.editField("")
	}
	win.set.scroll, win.set.scrollFocus = 0, -1
	win.showSection()
	win.w.Invalidate()
}

// showSection tells the shown section it came into view.
func (win *window) showSection() {
	if secs := win.sections(); win.set.section < len(secs) && secs[win.set.section].show != nil {
		secs[win.set.section].show(win)
	}
}

// settingsEvents takes the page's clicks and keys; the terminal gets none of them.
func (win *window) settingsEvents(gtx layout.Context) {
	tag := &win.set.tag
	edited := false
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: tag},
			key.Filter{Focus: tag, Optional: allMods},
			key.Filter{Focus: tag, Name: key.NameTab, Optional: allMods},
			pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}},
		)
		if !ok {
			break
		}
		if win.bench.IgnoreInput() {
			continue
		}
		switch e := ev.(type) {
		case key.Event:
			win.settingsKey(gtx, e)
		case key.EditEvent:
			edited = true
		case pointer.Event:
			win.settingsPointer(e)
		}
	}
	win.fieldEvents(gtx)
	if edited {
		// Typed text has nowhere to go here; drop Gio's copy so it cannot pile up.
		gtx.Execute(key.SnippetCmd{Tag: tag})
		gtx.Execute(key.SelectionCmd{Tag: tag})
	}
}

// settingsPointer handles a press on the page: a menu entry or a control.
func (win *window) settingsPointer(e pointer.Event) {
	if e.Kind == pointer.Scroll {
		if !win.dialogShown() {
			win.set.scroll = min(max(win.set.scroll+int(e.Scroll.Y), 0), win.set.scrollMax)
			win.w.Invalidate()
		}
		return
	}
	if e.Kind != pointer.Press || !e.Buttons.Contain(pointer.ButtonPrimary) {
		return
	}
	win.wantFocus = true
	win.dropSideEdit()
	win.set.editing = "" // a press the page gets is off every field's editor
	p := e.Position.Round()
	for i, n := range win.set.nav {
		if p.In(n.rect) {
			win.set.navFocus, win.set.navIndex = true, i
			win.set.focus = -1
			win.selectSection(n.section)
			return
		}
	}
	if !win.dialogShown() && !p.In(win.set.pane) {
		return
	}
	for i, c := range win.set.controls {
		if p.In(c.rect) {
			win.set.focus, win.set.navFocus = i, false
			if c.kind == ctlText {
				win.editField(c.id) // its padding, around the editor
			} else if !c.dead {
				win.activate(c.id, c.arg)
			}
			return
		}
	}
}

// settingsKey handles a key on the page: kaze's own shortcuts, Tab / Shift+Tab
// between controls, and Enter / Space to press the focused one.
func (win *window) settingsKey(gtx layout.Context, e key.Event) {
	if e.State != key.Press {
		return
	}
	s := &win.set
	if a, ok := win.km[input.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
		win.runShortcut(gtx, a)
		return
	}
	n := len(s.controls)
	if s.focus >= n {
		s.focus = -1 // the section changed under it
	}
	switch e.Name {
	case key.NameTab:
		switch e.Modifiers {
		case key.ModShift:
			win.moveFocus(-1)
		case 0:
			win.moveFocus(1)
		}
		return
	case key.NameReturn, key.NameEnter, key.NameSpace:
		if s.navFocus && s.navIndex < len(s.nav) {
			win.selectSection(s.nav[s.navIndex].section)
		} else if s.focus >= 0 && s.controls[s.focus].kind == ctlText {
			win.editField(s.controls[s.focus].id)
		} else if s.focus >= 0 && !s.controls[s.focus].dead {
			c := s.controls[s.focus]
			win.activate(c.id, c.arg)
		}
	default:
		return
	}
	win.w.Invalidate()
}

// moveFocus steps the page focus d places through the controls, then the menu,
// wrapping. A text field it lands on gets the keyboard; leaving one gives it back.
func (win *window) moveFocus(d int) {
	s := &win.set
	n := len(s.controls)
	total := n + len(s.nav)
	if s.focus >= n {
		s.focus = -1 // the section changed under it
	}
	if total == 0 {
		return
	}
	at := s.focus
	if s.navFocus {
		at = n + s.navIndex
	}
	if d < 0 && at < 0 {
		at = total
	}
	at = ((at+d)%total + total) % total
	s.navFocus = at >= n
	if s.navFocus {
		s.navIndex, s.focus = at-n, -1
	} else {
		s.focus = at
	}
	switch {
	case !s.navFocus && s.controls[at].kind == ctlText:
		win.editField(s.controls[at].id)
	case s.editing != "":
		win.editField("")
	}
	win.w.Invalidate()
}

// pageColors are the theme tokens the page draws with.
type pageColors struct {
	bg, nav, fg, title, dim, muted, line, accent, err color.NRGBA
	card, raised, line2, needs, done                  color.NRGBA
}

func (c *chrome) pageColors() pageColors {
	th := c.th
	card := mix(th.Active, th.ToastBG, 0.45)
	return pageColors{
		bg: th.Active, nav: th.Chrome, fg: th.ToastFG, title: th.ActiveFG,
		dim: th.StatusFG, muted: th.Muted, line: th.Border, accent: th.Accent, err: th.UsageHigh,
		card: card, raised: mix(card, th.ActiveFG, 0.07), line2: th.ToastLine, needs: th.Needs, done: th.Done,
	}
}

// tint is col at the given alpha, for selected fills.
func tint(col color.NRGBA, a uint8) color.NRGBA {
	col.A = a
	return col
}

// drawSettings lays out the page in r: the section menu on the left, the shown section on the right.
func (win *window) drawSettings(gtx layout.Context, r image.Rectangle) {
	s := &win.set
	s.controls, s.nav, s.texts, s.cards = s.controls[:0], s.nav[:0], s.texts[:0], s.cards[:0]
	s.memberFolderX = s.memberFolderX[:0]
	p := &page{gtx: gtx, c: win.ui, win: win, st: s, col: win.ui.pageColors()}
	ops := gtx.Ops
	cl := clip.Rect(r).Push(ops)
	defer cl.Pop()
	paint.FillShape(ops, p.col.bg, clip.Rect(r).Op())
	area := clip.Rect(r).Push(ops)
	event.Op(ops, &s.tag)
	area.Pop()

	secs := win.sections()
	navW := min(gtx.Dp(220), r.Dx())
	p.nav(image.Rect(r.Min.X, r.Min.Y, r.Min.X+navW, r.Max.Y), secs)
	// The content is a column up to 760 dp wide, centered in what the menu leaves.
	content := image.Rect(r.Min.X+navW, r.Min.Y, r.Max.X, r.Max.Y)
	colW := max(min(content.Dx()-2*gtx.Dp(36), gtx.Dp(760)), 0)
	x := content.Min.X + (content.Dx()-colW)/2
	pane := image.Rect(x, r.Min.Y+gtx.Dp(30), x+colW, r.Max.Y)
	s.pane = image.Rect(content.Min.X, pane.Min.Y, content.Max.X, content.Max.Y)
	if pane.Dx() > 0 && s.section < len(secs) {
		from := len(s.controls)
		rec := op.Record(ops)
		bottom := p.pane(pane, secs[s.section], win)
		call := rec.Stop()
		s.scrollMax = max(0, bottom+gtx.Dp(16)-pane.Max.Y)
		s.scroll = min(max(s.scroll, 0), s.scrollMax)
		if s.focus != s.scrollFocus && s.focus >= from && s.focus < len(s.controls) {
			focus := s.controls[s.focus].rect
			if focus.Max.Y-s.scroll > pane.Max.Y {
				s.scroll = min(s.scrollMax, focus.Max.Y-pane.Max.Y)
			}
			if focus.Min.Y-s.scroll < pane.Min.Y {
				s.scroll = max(0, focus.Min.Y-pane.Min.Y)
			}
		}
		s.scrollFocus = s.focus
		s.shift(from, len(s.controls), image.Pt(0, -s.scroll))
		win.pruneFields()
		paneClip := clip.Rect(s.pane).Push(ops)
		off := op.Offset(image.Pt(0, -s.scroll)).Push(ops)
		call.Add(ops)
		off.Pop()
		paneClip.Pop()
		if dlg := secs[s.section].dialog; dlg != nil {
			if d := dlg(win); d != nil {
				// Only the dialog's buttons answer while it is open.
				s.controls, s.nav = s.controls[:0], s.nav[:0]
				if s.editing != "" {
					win.editField("")
				}
				p.dialog(r, d)
				// Over the fields' editors, so the page takes every click.
				area := clip.Rect(r).Push(ops)
				event.Op(ops, &s.tag)
				area.Pop()
			}
		}
	}
}

// pruneFields drops the editors of fields this frame's rows no longer have.
func (win *window) pruneFields() {
	s := &win.set
	for id, f := range s.fields {
		if !f.seen {
			delete(s.fields, id)
			if s.editing == id {
				win.editField("")
			}
		}
		f.seen = false
	}
}

// truncateMiddle keeps both ends of a long line, so a command keeps its arguments.
func truncateMiddle(s string, max int) string {
	r := []rune(s)
	if len(r) <= max || max < 3 {
		return s
	}
	head := (max - 1) / 2
	return string(r[:head]) + "…" + string(r[len(r)-(max-1-head):])
}

// dialog dims the page and draws d centered in r, its buttons bottom right.
func (p *page) dialog(r image.Rectangle, d *dialogBox) {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	paint.FillShape(ops, tint(col.nav, 0xc0), clip.Rect(r).Op())
	pad, gap := gtx.Dp(20), gtx.Dp(8)
	w := min(gtx.Dp(640), r.Dx()-gtx.Dp(32))
	if w <= 2*pad {
		return
	}
	inner := w - 2*pad
	type line struct {
		call op.CallOp
		size image.Point
	}
	var body []line
	add := func(s string, size unit.Sp, c color.NRGBA) {
		call, sz := p.text(s, size, c)
		body = append(body, line{call, sz})
	}
	add(d.title, 14, col.title)
	for _, l := range d.lines {
		for _, wl := range p.wrap(l, 12, inner) {
			add(wl, 12, col.fg)
		}
	}
	codeAt := len(body)
	for _, l := range d.code {
		add(truncateMiddle(l, 110), 11, col.muted)
	}
	var btns []line
	var from []int
	bw := 0
	for i, k := range d.buttons {
		call, size, f := p.ctrl(k, 0)
		btns, from = append(btns, line{call, size}), append(from, f)
		if i > 0 {
			bw += gap
		}
		bw += size.X
	}
	h := pad
	for i, l := range body {
		if i == 1 || i == codeAt && codeAt < len(body) {
			h += gap
		}
		h += l.size.Y
	}
	bh := 0
	for _, b := range btns {
		bh = max(bh, b.size.Y)
	}
	h += pad + bh + pad
	h = min(h, r.Dy()-gtx.Dp(16))
	// Not image.Rect: it swaps corners when min > max, which put the box at the origin.
	at := image.Pt(r.Min.X+(r.Dx()-w)/2, r.Min.Y+(r.Dy()-h)/2)
	box := image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}
	paint.FillShape(ops, col.bg, clip.UniformRRect(box, gtx.Dp(10)).Op(ops))
	stroke(ops, box, gtx.Dp(10), col.line)

	// Buttons first, so a long change never pushes them off the box.
	x, by := box.Max.X-pad-bw, box.Max.Y-pad-bh
	for i, b := range btns {
		at := image.Pt(x, by+(bh-b.size.Y)/2)
		end := len(p.st.controls)
		if i+1 < len(from) {
			end = from[i+1]
		}
		p.st.shift(from[i], end, at)
		place(ops, b.call, at)
		x += b.size.X + gap
	}
	cl := clip.Rect(image.Rect(box.Min.X+pad, box.Min.Y+pad, box.Max.X-pad, by-gap)).Push(ops)
	y := box.Min.Y + pad
	for i, l := range body {
		if i == 1 || i == codeAt && codeAt < len(body) {
			y += gap
		}
		place(ops, l.call, image.Pt(box.Min.X+pad, y))
		y += l.size.Y
	}
	cl.Pop()
}

// page draws one frame of the settings page and records its hit areas and text.
type page struct {
	gtx    layout.Context
	c      *chrome
	win    *window
	st     *settingsState
	col    pageColors
	fieldH int // a text field's line height, measured once a frame
}

func (p *page) text(s string, size unit.Sp, col color.NRGBA) (op.CallOp, image.Point) {
	p.st.texts = append(p.st.texts, s)
	return p.c.sans(p.gtx, s, size, col, font.Normal)
}

// wrap breaks s into lines no wider than w, at spaces; a word wider than w gets its own line.
func (p *page) wrap(s string, size unit.Sp, w int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		try := word
		if cur != "" {
			try = cur + " " + word
		}
		if _, sz := p.c.sans(p.gtx, try, size, p.col.dim, font.Normal); sz.X > w && cur != "" {
			lines = append(lines, cur)
			try = word
		}
		cur = try
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// nav draws the menu: the page's title, the sections with their icons under
// their group headings, and the version and update state at the bottom.
func (p *page) nav(r image.Rectangle, secs []settingsSection) {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	paint.FillShape(ops, col.nav, clip.Rect(r).Op())
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)).Op())
	pad, inner := gtx.Dp(12), gtx.Dp(10)
	x0, x1 := r.Min.X+pad, r.Max.X-pad
	y := r.Min.Y + gtx.Dp(20)
	call, size := p.textW("Settings", 18, col.title, font.SemiBold)
	place(ops, call, image.Pt(x0+inner, y))
	y += size.Y + gtx.Dp(2)
	call, size = p.text("Saved to config.toml as you go", 11, col.dim)
	place(ops, call, image.Pt(x0+inner, y))
	y += size.Y + gtx.Dp(14)
	group := ""
	for i, sec := range secs {
		if sec.later {
			continue
		}
		if sec.group != group {
			group = sec.group
			y += gtx.Dp(12)
			call, size := p.textW(strings.ToUpper(group), 10, col.dim, font.SemiBold)
			place(ops, call, image.Pt(x0+inner, y))
			y += size.Y + gtx.Dp(6)
		}
		item := image.Rect(x0, y, x1, y+gtx.Dp(34))
		fg, ic := col.muted, col.muted
		if i == p.st.section {
			paint.FillShape(ops, tint(col.accent, 0x2e), clip.UniformRRect(item, gtx.Dp(8)).Op(ops))
			stroke(ops, item, gtx.Dp(8), tint(col.accent, 0x55))
			fg, ic = col.title, col.accent
		}
		mid := item.Min.Y + item.Dy()/2
		is := gtx.Dp(16)
		p.c.icon(gtx, sec.icon, image.Rect(x0+inner, mid-is/2, x0+inner+is, mid-is/2+is), ic)
		call, size := p.textW(sec.title, 13, fg, font.Medium)
		place(ops, call, image.Pt(x0+inner+is+gtx.Dp(11), mid-size.Y/2))
		if sec.badge != nil {
			if b := sec.badge(p.win); b != "" {
				bc, bs := p.textW(b, 10, col.bg, font.SemiBold)
				h := bs.Y + gtx.Dp(2)
				box := image.Rect(x1-inner-bs.X-gtx.Dp(12), mid-h/2, x1-inner, mid-h/2+h)
				paint.FillShape(ops, col.accent, clip.UniformRRect(box, h/2).Op(ops))
				place(ops, bc, image.Pt(box.Min.X+gtx.Dp(6), mid-bs.Y/2))
			}
		}
		if p.st.navFocus && p.st.navIndex == len(p.st.nav) {
			stroke(ops, item.Inset(-gtx.Dp(2)), gtx.Dp(9), col.accent)
		}
		p.st.nav = append(p.st.nav, navHit{rect: item, section: i})
		y = item.Max.Y + gtx.Dp(2)
	}

	// Version block, kept at the bottom unless the menu needs the room.
	vc, vs := p.textW("kaze "+version.Number, 12.5, col.title, font.SemiBold)
	state := p.win.updFoot()
	var sc op.CallOp
	var ss image.Point
	if state != "" {
		sc, ss = p.text(state, 11, col.dim)
	}
	blockH := gtx.Dp(12) + max(gtx.Dp(22), vs.Y+ss.Y)
	top := max(r.Max.Y-gtx.Dp(14)-blockH, y+gtx.Dp(14))
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(x0, top, x1, top+1)).Op())
	y = top + gtx.Dp(12)
	tx := x0 + inner
	mark := gtx.Dp(22)
	if mop, img := markImage(mark); img != nil {
		t := op.Offset(image.Pt(tx, y+(vs.Y+ss.Y-mark)/2)).Push(ops)
		mop.Add(ops)
		paint.PaintOp{}.Add(ops)
		t.Pop()
		tx += mark + gtx.Dp(10)
	}
	place(ops, vc, image.Pt(tx, y))
	if state != "" {
		place(ops, sc, image.Pt(tx, y+vs.Y))
	}
}

// pane draws the shown section: title, hint and its rows.
func (p *page) pane(r image.Rectangle, sec settingsSection, win *window) int {
	if sec.draw != nil {
		return sec.draw(p, r, win)
	}
	y := p.heading(r, sec.title, sec.hint)
	if sec.actions != nil {
		p.headerActions(r, sec.actions(win))
	}
	if sec.rows == nil {
		return y
	}
	return p.rows(r, y, sec.rows(win))
}

// heading draws a section's title and hint and returns where its rows start.
func (p *page) heading(r image.Rectangle, title, hint string) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	y := r.Min.Y
	call, size := p.textW(title, 24, col.title, font.SemiBold)
	place(ops, call, image.Pt(r.Min.X, y))
	y += size.Y + gtx.Dp(2)
	if hint != "" {
		call, size = p.text(hint, 13, col.muted)
		place(ops, call, image.Pt(r.Min.X, y))
		y += size.Y
	}
	return y + gtx.Dp(22)
}

// headerActions draws buttons at the right of a section's heading.
func (p *page) headerActions(r image.Rectangle, cs []ctrl) {
	gtx, ops := p.gtx, p.gtx.Ops
	x := r.Max.X
	for i := len(cs) - 1; i >= 0; i-- {
		call, size, from := p.ctrl(cs[i], 0)
		x -= size.X
		at := image.Pt(x, r.Min.Y+gtx.Dp(8))
		p.st.shift(from, len(p.st.controls), at)
		place(ops, call, at)
		x -= gtx.Dp(8)
	}
}

// rows draws rows from y in r and returns where the next would start: each
// heading as a small label, and the rows between headings together in a card.
func (p *page) rows(r image.Rectangle, y int, rows []settingRow) int {
	drawn := false
	for i := 0; i < len(rows); {
		if rows[i].heading != "" {
			if drawn {
				y += p.gtx.Dp(26)
			}
			y = p.sectionLabel(r, y, rows[i].heading)
			i++
			continue
		}
		j := i + 1
		for j < len(rows) && rows[j].heading == "" && !rows[j].newCard {
			j++
		}
		if drawn && (i == 0 || rows[i-1].heading == "") {
			y += p.gtx.Dp(12)
		}
		y = p.card(r, y, rows[i:j])
		drawn = true
		i = j
	}
	return y
}

// row draws one row from y and returns where the next starts.
func (p *page) row(r image.Rectangle, y int, row settingRow) int {
	if len(row.members) > 0 {
		return p.memberRow(r, y, row)
	}
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	lx := r.Min.X
	var tc op.CallOp
	var tsz image.Point
	if row.icon != "" {
		tc, tsz = p.iconTile(row.icon)
		lx += tsz.X + gtx.Dp(14)
	}
	lc, ls := p.textW(row.label, 13.5, col.title, font.SemiBold)
	var hc op.CallOp
	var hs image.Point
	type text struct {
		call op.CallOp
		size image.Point
	}
	var extra []text
	extraH := 0
	for _, l := range row.lines {
		for _, w := range p.wrap(l, 12, max(r.Max.X-lx, 1)) {
			call, size := p.text(w, 12, col.muted)
			extra = append(extra, text{call, size})
			extraH += size.Y
		}
	}
	gap := gtx.Dp(8)
	type laid struct {
		call     op.CallOp
		size     image.Point
		from, to int // its hit areas, at the macro's origin until placed
	}
	var cs []laid
	w, h := 0, 0
	if row.pill != "" {
		call, size := p.pill(row.pill, row.tone)
		cs = append(cs, laid{call, size, len(p.st.controls), len(p.st.controls)})
		w, h = size.X, size.Y
	}
	for _, k := range row.ctrls {
		call, size, from := p.ctrl(k, 0)
		cs = append(cs, laid{call, size, from, len(p.st.controls)})
		if w > 0 {
			w += gap
		}
		w += size.X
		h = max(h, size.Y)
	}
	r.Min.X = lx
	// The hint goes last: a long one wraps in the room the controls leave.
	switch {
	case row.hint != "" && row.monoHint:
		hc, hs = p.mono(row.hint, 11, col.muted)
	case row.hint != "":
		room := r.Dx()
		if w > 0 {
			room -= w + 2*gap
		}
		if _, full := p.c.sans(gtx, row.hint, 12, col.muted, font.Normal); full.X > room && room > gtx.Dp(160) {
			for _, l := range p.wrap(row.hint, 12, room) {
				call, size := p.text(l, 12, col.muted)
				extra = append(extra, text{call, size})
				extraH += size.Y
			}
		} else {
			hc, hs = p.text(row.hint, 12, col.muted)
		}
	}
	labelH := ls.Y + extraH + hs.Y
	labelW := max(ls.X, hs.X)
	for _, e := range extra {
		labelW = max(labelW, e.size.X)
	}
	stacked := labelW+gap+w > r.Dx()
	rowH := max(max(labelH, h, tsz.Y)+gtx.Dp(28), gtx.Dp(58))
	if stacked {
		rowH = labelH + gap + h + gtx.Dp(28)
	}
	ty := y + (rowH-labelH)/2
	if stacked {
		ty = y + gtx.Dp(14)
	}
	if row.icon != "" {
		place(ops, tc, image.Pt(r.Min.X-tsz.X-gtx.Dp(14), y+(rowH-tsz.Y)/2))
	}
	place(ops, lc, image.Pt(r.Min.X, ty))
	ty += ls.Y
	for _, e := range extra {
		place(ops, e.call, image.Pt(r.Min.X, ty))
		ty += e.size.Y
	}
	if hs.Y > 0 {
		place(ops, hc, image.Pt(r.Min.X, ty))
	}
	x := r.Max.X - w
	cy := y + (rowH-h)/2
	if stacked {
		x, cy = r.Min.X, y+gtx.Dp(14)+labelH+gap
	}
	for _, c := range cs {
		at := image.Pt(x, cy+(h-c.size.Y)/2)
		p.st.shift(c.from, c.to, at)
		place(ops, c.call, at)
		x += c.size.X + gap
	}
	return y + rowH
}

// blockRow draws a small heading and its controls side by side across the pane.
func (p *page) blockRow(r image.Rectangle, y int, row settingRow) int {
	gtx, ops := p.gtx, p.gtx.Ops
	call, size := p.textW(row.label, 13.5, p.col.title, font.SemiBold)
	place(ops, call, image.Pt(r.Min.X, y))
	y += size.Y + gtx.Dp(2)
	if row.hint != "" {
		hc, hs := p.text(row.hint, 12, p.col.muted)
		place(ops, hc, image.Pt(r.Min.X, y))
		y += hs.Y
	}
	y += gtx.Dp(12)
	n := len(row.ctrls)
	if n == 0 {
		return y
	}
	gap := gtx.Dp(10)
	w := max((r.Dx()-(n-1)*gap)/n, 1)
	h := 0
	for i, k := range row.ctrls {
		c, size, from := p.ctrl(k, w)
		at := image.Pt(r.Min.X+i*(w+gap), y)
		p.st.shift(from, len(p.st.controls), at)
		place(ops, c, at)
		h = max(h, size.Y)
	}
	return y + h + gtx.Dp(10)
}
