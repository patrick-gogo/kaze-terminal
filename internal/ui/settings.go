package ui

import (
	"image"
	"image/color"
	"runtime/debug"
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
	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// settingsState is the Settings page: a special tab with no shell behind it.
// open means its chip is in the tab strip; shown means it is in front instead of the terminal.
type settingsState struct {
	open, shown bool
	section     int // index into the sections; never a later one
	focus       int // index into controls; -1 when no control has focus
	tag         int // its address is the page's event tag

	// Laid out each frame.
	controls []control
	nav      []navHit
	texts    []string // every string drawn, so tests can read the page

	build    string            // the build line, read when the page opens
	sections []settingsSection // nil means defaultSections; tests swap in their own
}

// settingsChip is what the tab strip needs to draw the Settings chip.
type settingsChip struct{ open, shown bool }

type navHit struct {
	rect    image.Rectangle
	section int
}

// settingRow is one line of a section: label and dim hint on the left, controls on the right.
type settingRow struct {
	label, hint string
	ctrls       []ctrl
	block       bool // the controls sit under the label across the pane (theme swatches)
}

// settingsSection is one entry of the page's menu. rows builds its content each
// frame from the window's state; act runs one of its controls (see activate).
type settingsSection struct {
	title, hint string
	later       bool // greyed with "coming next" and not selectable
	rows        func(win *window) []settingRow
	act         func(win *window, id string, arg int)
}

// defaultSections are the page's menu; the Appearance and Claude sections fill in rows and act.
func defaultSections() []settingsSection {
	return []settingsSection{
		appearanceSection(),
		{title: "Workspaces", later: true},
		{title: "Claude", hint: "Alerts come from Claude Code hooks; usage comes from its status line."},
	}
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
	s := &win.set
	if !s.open {
		s.open, s.focus = true, -1
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

// closeSettings closes the Settings tab.
func (win *window) closeSettings() {
	win.hideSettings()
	s := &win.set
	s.open = false
	s.controls, s.nav = s.controls[:0], s.nav[:0]
	win.w.Invalidate()
}

// cycleTab steps through the tabs as the strip shows them, the Settings tab last.
func (win *window) cycleTab(d int) {
	list := win.mgr.Tabs()
	if len(list) == 0 {
		return
	}
	if win.set.shown {
		i := 0
		if d < 0 {
			i = len(list) - 1
		}
		win.selectTab(list[i].ID)
		return
	}
	if win.set.open {
		if a := win.mgr.Active(); a != nil && (d > 0 && a == list[len(list)-1] || d < 0 && a == list[0]) {
			win.openSettings()
			return
		}
	}
	if d > 0 {
		win.mgr.Next()
	} else {
		win.mgr.Prev()
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
	win.set.section, win.set.focus = i, -1
	win.w.Invalidate()
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
			pointer.Filter{Target: tag, Kinds: pointer.Press},
		)
		if !ok {
			break
		}
		if win.bench.IgnoreInput() {
			continue
		}
		switch e := ev.(type) {
		case key.Event:
			win.settingsKey(e)
		case key.EditEvent:
			edited = true
		case pointer.Event:
			win.settingsPointer(e)
		}
	}
	if edited {
		// Typed text has nowhere to go here; drop Gio's copy so it cannot pile up.
		gtx.Execute(key.SnippetCmd{Tag: tag})
		gtx.Execute(key.SelectionCmd{Tag: tag})
	}
}

// settingsPointer handles a press on the page: a menu entry or a control.
func (win *window) settingsPointer(e pointer.Event) {
	if e.Kind != pointer.Press || !e.Buttons.Contain(pointer.ButtonPrimary) {
		return
	}
	win.wantFocus = true
	p := e.Position.Round()
	for _, n := range win.set.nav {
		if p.In(n.rect) {
			win.selectSection(n.section)
			return
		}
	}
	for i, c := range win.set.controls {
		if p.In(c.rect) {
			win.set.focus = i
			if !c.dead {
				win.activate(c.id, c.arg)
			}
			return
		}
	}
}

// settingsKey handles a key on the page: kaze's own shortcuts, Tab / Shift+Tab
// between controls, and Enter / Space to press the focused one.
func (win *window) settingsKey(e key.Event) {
	if e.State != key.Press {
		return
	}
	s := &win.set
	if a, ok := win.km[input.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
		switch a {
		case actNewTab:
			win.newTab()
		case actCloseTab:
			win.closeSettings()
		case actNextTab:
			win.cycleTab(1)
		case actPrevTab:
			win.cycleTab(-1)
		}
		return
	}
	n := len(s.controls)
	if s.focus >= n {
		s.focus = -1 // the section changed under it
	}
	switch e.Name {
	case key.NameTab:
		if n == 0 {
			return
		}
		switch {
		case e.Modifiers == key.ModShift && s.focus < 0:
			s.focus = n - 1
		case e.Modifiers == key.ModShift:
			s.focus = (s.focus - 1 + n) % n
		case e.Modifiers == 0:
			s.focus = (s.focus + 1) % n
		}
	case key.NameReturn, key.NameEnter, key.NameSpace:
		if s.focus >= 0 && !s.controls[s.focus].dead {
			c := s.controls[s.focus]
			win.activate(c.id, c.arg)
		}
	default:
		return
	}
	win.w.Invalidate()
}

// pageColors are the theme tokens the page draws with.
type pageColors struct {
	bg, nav, fg, title, dim, muted, line, accent color.NRGBA
}

func (c *chrome) pageColors() pageColors {
	th := c.th
	return pageColors{
		bg: th.Active, nav: th.Chrome, fg: th.ToastFG, title: th.ActiveFG,
		dim: th.StatusFG, muted: th.Muted, line: th.Border, accent: th.Accent,
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
	s.controls, s.nav, s.texts = s.controls[:0], s.nav[:0], s.texts[:0]
	p := &page{gtx: gtx, c: win.ui, st: s, col: win.ui.pageColors()}
	ops := gtx.Ops
	cl := clip.Rect(r).Push(ops)
	defer cl.Pop()
	paint.FillShape(ops, p.col.bg, clip.Rect(r).Op())
	area := clip.Rect(r).Push(ops)
	event.Op(ops, &s.tag)
	area.Pop()

	secs := win.sections()
	navW := min(gtx.Dp(190), r.Dx())
	p.nav(image.Rect(r.Min.X, r.Min.Y, r.Min.X+navW, r.Max.Y), secs)
	pane := image.Rect(r.Min.X+navW+gtx.Dp(28), r.Min.Y+gtx.Dp(22), r.Max.X-gtx.Dp(28), r.Max.Y)
	if pane.Dx() > 0 && s.section < len(secs) {
		p.pane(pane, secs[s.section], win)
	}
}

// page draws one frame of the settings page and records its hit areas and text.
type page struct {
	gtx layout.Context
	c   *chrome
	st  *settingsState
	col pageColors
}

func (p *page) text(s string, size unit.Sp, col color.NRGBA) (op.CallOp, image.Point) {
	p.st.texts = append(p.st.texts, s)
	return p.c.label(p.gtx, s, size, col)
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
		if _, sz := p.c.label(p.gtx, try, size, p.col.dim); sz.X > w && cur != "" {
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

// nav draws the section menu, the "Later" line and the version block at the bottom.
func (p *page) nav(r image.Rectangle, secs []settingsSection) {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	paint.FillShape(ops, col.nav, clip.Rect(r).Op())
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y)).Op())
	pad, inner := gtx.Dp(10), gtx.Dp(12)
	x0, x1 := r.Min.X+pad, r.Max.X-pad
	textW := x1 - x0 - 2*inner
	y := r.Min.Y + gtx.Dp(16)
	for i, sec := range secs {
		item := image.Rect(x0, y, x1, y+gtx.Dp(30))
		fg := col.muted
		switch {
		case sec.later:
			fg = col.dim
		case i == p.st.section:
			paint.FillShape(ops, tint(col.accent, 0x26), clip.UniformRRect(item, gtx.Dp(7)).Op(ops))
			fg = col.title
		}
		call, size := p.text(sec.title, 13, fg)
		mid := item.Min.Y + item.Dy()/2
		place(ops, call, image.Pt(x0+inner, mid-size.Y/2))
		if sec.later {
			nc, ns := p.text("coming next", 10, col.dim)
			if x0+inner+size.X+gtx.Dp(8)+ns.X <= x1-inner {
				place(ops, nc, image.Pt(x1-inner-ns.X, mid-ns.Y/2))
			}
		} else {
			p.st.nav = append(p.st.nav, navHit{rect: item, section: i})
		}
		y = item.Max.Y + gtx.Dp(2)
	}
	y += gtx.Dp(14)
	for _, line := range p.wrap("Later: Terminal, Shell setup, Shortcuts", 11, textW) {
		call, size := p.text(line, 11, col.dim)
		place(ops, call, image.Pt(x0+inner, y))
		y += size.Y
	}

	// Version block, kept at the bottom unless the menu needs the room.
	vc, vs := p.text("kaze "+version.Number, 12, col.fg)
	lines := p.wrap(p.st.build, 11, textW)
	_, ls := p.c.label(gtx, "build", 11, col.dim)
	blockH := gtx.Dp(12) + vs.Y + len(lines)*ls.Y
	top := max(r.Max.Y-gtx.Dp(16)-blockH, y+gtx.Dp(14))
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(x0, top, x1, top+1)).Op())
	y = top + gtx.Dp(12)
	place(ops, vc, image.Pt(x0+inner, y))
	y += vs.Y
	for _, line := range lines {
		call, _ := p.text(line, 11, col.dim)
		place(ops, call, image.Pt(x0+inner, y))
		y += ls.Y
	}
}

// pane draws the shown section: title, hint and its rows.
func (p *page) pane(r image.Rectangle, sec settingsSection, win *window) {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	y := r.Min.Y
	call, size := p.text(sec.title, 16, col.title)
	place(ops, call, image.Pt(r.Min.X, y))
	y += size.Y + gtx.Dp(4)
	if sec.hint != "" {
		call, size = p.text(sec.hint, 12, col.dim)
		place(ops, call, image.Pt(r.Min.X, y))
		y += size.Y
	}
	y += gtx.Dp(16)
	if sec.rows == nil {
		return
	}
	for _, row := range sec.rows(win) {
		if row.block {
			y = p.blockRow(r, y, row)
		} else {
			y = p.row(r, y, row)
		}
	}
}

// row draws one row from y and returns where the next starts.
func (p *page) row(r image.Rectangle, y int, row settingRow) int {
	gtx, ops, col := p.gtx, p.gtx.Ops, p.col
	paint.FillShape(ops, col.line, clip.Rect(image.Rect(r.Min.X, y, r.Max.X, y+1)).Op())
	lc, ls := p.text(row.label, 13, col.fg)
	var hc op.CallOp
	var hs image.Point
	if row.hint != "" {
		hc, hs = p.text(row.hint, 11, col.dim)
	}
	gap := gtx.Dp(8)
	type laid struct {
		call     op.CallOp
		size     image.Point
		from, to int // its hit areas, at the macro's origin until placed
	}
	var cs []laid
	w, h := 0, 0
	for i, k := range row.ctrls {
		call, size, from := p.ctrl(k, 0)
		cs = append(cs, laid{call, size, from, len(p.st.controls)})
		if i > 0 {
			w += gap
		}
		w += size.X
		h = max(h, size.Y)
	}
	labelH := ls.Y + hs.Y
	rowH := max(labelH, h) + gtx.Dp(22)
	place(ops, lc, image.Pt(r.Min.X, y+(rowH-labelH)/2))
	if row.hint != "" {
		place(ops, hc, image.Pt(r.Min.X, y+(rowH-labelH)/2+ls.Y))
	}
	x := r.Max.X - w
	for _, c := range cs {
		at := image.Pt(x, y+(rowH-c.size.Y)/2)
		p.st.shift(c.from, c.to, at)
		place(ops, c.call, at)
		x += c.size.X + gap
	}
	return y + rowH
}

// blockRow draws a small heading and its controls side by side across the pane.
func (p *page) blockRow(r image.Rectangle, y int, row settingRow) int {
	gtx, ops := p.gtx, p.gtx.Ops
	call, size := p.text(row.label, 12, p.col.dim)
	place(ops, call, image.Pt(r.Min.X, y))
	y += size.Y + gtx.Dp(6)
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
