package ui

import (
	"image"
	"image/color"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	kinput "github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const (
	// maxMemberLabel bounds the "› member" part of a group chip; it is cut before the chip drops.
	maxMemberLabel = 16
	// maxStackDots bounds the chip's dot stack, so a big run cannot widen it.
	maxStackDots = 8
)

// groupHit is where the last frame drew a run's chip, with what it showed.
type groupHit struct {
	run   uint64
	rect  image.Rectangle
	label string
	mark  mark // the strongest alert among the members
}

// groupMenu is the open dropdown of a tab group; run 0 means closed. UI goroutine only.
type groupMenu struct {
	run               uint64
	kb                int // the row the arrow keys are on
	tag               int // the card: its clicks and, while open, the keyboard
	outTag            int // everywhere else: a click there closes the menu
	rows              []menuRow
	card              image.Rectangle
	bg                color.NRGBA // the card color the last frame painted; the theme-switch test reads it
	scroll, scrollMax int         // row scroll offset; the header stays fixed
	reveal            bool        // reveal the keyboard row on open or after arrow navigation
}

type menuRow struct {
	id             tabs.TabID
	rect, close    image.Rectangle
	detail, status string // the second line as drawn: what the member runs and how far it got
}

func (b barHits) groupAt(p image.Point) (uint64, bool) {
	for _, g := range b.groups {
		if p.In(g.rect) {
			return g.run, true
		}
	}
	return 0, false
}

// memberPart is the chip's "› member" text, shortened first when room is short.
func memberPart(member string) string {
	return "› " + cutRunes(member, maxMemberLabel)
}

// groupLabel is the chip's text: the workspace name, and the shown member when there is one.
func groupLabel(name, member string) string {
	if member == "" {
		return name
	}
	return name + " " + memberPart(member)
}

// cutRunes shortens s to n runes with a trailing "…"; short text is not copied.
func cutRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// shortFolder is the last two parts of a member's folder, enough to tell members apart.
func shortFolder(dir string) string {
	if dir == "" {
		return ""
	}
	dir = filepath.Clean(dir)
	parent := filepath.Dir(dir)
	if parent == dir || filepath.Base(parent) == parent || parent == filepath.VolumeName(parent)+string(filepath.Separator) {
		return filepath.Base(dir)
	}
	return filepath.Join(filepath.Base(parent), filepath.Base(dir))
}

func inRun(t *tabs.Tab, run uint64) bool { return t.Group != nil && t.Group.Run == run }

func memberCount(list []*tabs.Tab, run uint64) int {
	n := 0
	for _, t := range list {
		if inRun(t, run) {
			n++
		}
	}
	return n
}

// firstOfRun reports whether list[i] is its run's first tab in display order, where the chip sits.
func firstOfRun(list []*tabs.Tab, i int) bool {
	for _, t := range list[:i] {
		if inRun(t, list[i].Group.Run) {
			return false
		}
	}
	return true
}

// memberColor maps a recipe color through the theme's ANSI colors.
func (c *chrome) memberColor(name string) color.NRGBA {
	switch name {
	case "red":
		return c.ansi[1]
	case "green":
		return c.ansi[2]
	case "yellow":
		return c.ansi[3]
	case "blue":
		return c.ansi[4]
	case "magenta":
		return c.ansi[5]
	case "cyan":
		return c.ansi[6]
	}
	return c.th.Muted
}

// badgeColor is the chip count's color: amber while a member needs you, green when one is done.
func (c *chrome) badgeColor(m mark) color.NRGBA {
	switch m {
	case markNeeds:
		return c.th.Needs
	case markDone:
		return c.th.Done
	}
	return c.th.Muted
}

// memberDot draws a member's color dot, ringed in its alert color.
func (c *chrome) memberDot(gtx layout.Context, d image.Rectangle, col string, mk mark) {
	ops := gtx.Ops
	paint.FillShape(ops, c.memberColor(col), clip.Ellipse(d).Op(ops))
	if mk != markNone {
		in := gtx.Dp(2)
		ring := image.Rect(d.Min.X-in, d.Min.Y-in, d.Max.X+in, d.Max.Y+in)
		paint.FillShape(ops, c.badgeColor(mk), clip.Stroke{Path: clip.Ellipse(ring).Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
	}
}

// groupChip draws the chip for list[first]'s run at x if it fits before room, and
// returns its width. Without room for "› member" it draws without it.
func (c *chrome) groupChip(gtx layout.Context, list []*tabs.Tab, first int, active tabs.TabID, marks func(tabs.TabID) mark, def string, x, room int) (int, bool) {
	ops := gtx.Ops
	g := list[first].Group
	top, height := gtx.Dp(tabTop), gtx.Constraints.Max.Y
	padX, gap, dot, step, radius := gtx.Dp(12), gtx.Dp(7), gtx.Dp(7), gtx.Dp(5), gtx.Dp(9)
	n, agg := 0, markNone
	var cur *tabs.Tab
	for _, t := range list[first:] {
		if !inRun(t, g.Run) {
			continue
		}
		n++
		if marks != nil {
			agg = max(agg, marks(t.ID)) // needs outranks done
		}
		if t.ID == active {
			cur = t
		}
	}
	fg := c.th.Muted
	if cur != nil {
		fg = c.th.ActiveFG
	}
	name := cutRunes(g.Name, maxLabel)
	nameCall, nameSize := c.label(gtx, name, 12, fg)
	countCall, countSize := c.label(gtx, strconv.Itoa(n), 10, c.badgeColor(agg))
	chevCall, chevSize := c.label(gtx, "▾", 9, c.th.Muted)
	dots := min(n, maxStackDots)
	stackW := dot + (dots-1)*step
	badgePad := gtx.Dp(6)
	badgeW := countSize.X + 2*badgePad
	w := padX + stackW + gap + nameSize.X + gap + badgeW + gap + chevSize.X + padX
	sub := ""
	var subCall op.CallOp
	var subSize image.Point
	if cur != nil {
		sub = memberPart(c.tabLabelFit(cur, def))
		subCall, subSize = c.label(gtx, sub, 12, c.th.Muted)
		if x+w+gap+subSize.X <= room {
			w += gap + subSize.X
		} else {
			sub = ""
		}
	}
	if x+w > room {
		return 0, false
	}
	r := image.Rect(x, top, x+w, height)
	if cur != nil {
		c.activeTab(gtx, r, radius)
	} else {
		paint.FillShape(ops, c.th.Border, clip.Stroke{Path: clip.RRect{Rect: r, NW: radius, NE: radius}.Path(ops), Width: 1}.Op())
	}
	mid := top + (height-top)/2
	cx := x + padX
	k := 0
	for _, t := range list[first:] {
		if k == dots {
			break
		}
		if !inRun(t, g.Run) {
			continue
		}
		var mk mark
		if marks != nil {
			mk = marks(t.ID)
		}
		d := image.Rect(cx+k*step, mid-dot/2, cx+k*step+dot, mid-dot/2+dot)
		c.memberDot(gtx, d, t.Color, mk)
		k++
	}
	cx += stackW + gap
	place(ops, nameCall, image.Pt(cx, mid-nameSize.Y/2))
	cx += nameSize.X + gap
	label := name
	if sub != "" {
		place(ops, subCall, image.Pt(cx, mid-subSize.Y/2))
		cx += subSize.X + gap
		label += " " + sub
	}
	bh := countSize.Y
	badge := image.Rect(cx, mid-bh/2, cx+badgeW, mid-bh/2+bh)
	paint.FillShape(ops, c.th.Border, clip.UniformRRect(badge, bh/2).Op(ops))
	place(ops, countCall, image.Pt(cx+badgePad, mid-countSize.Y/2))
	cx += badgeW + gap
	place(ops, chevCall, image.Pt(cx, mid-chevSize.Y/2))
	c.hits.groups = append(c.hits.groups, groupHit{run: g.Run, rect: r, label: label, mark: agg})
	return w, true
}

// openGroupMenu opens run's dropdown on the shown member's row and gives it the keyboard.
func (win *window) openGroupMenu(run uint64) {
	g := &win.grp
	g.run, g.kb = run, 0
	g.scroll, g.reveal = 0, true
	if t := win.shownTab(); t != nil && inRun(t, run) {
		i := 0
		for _, o := range win.mgr.Tabs() {
			if o.ID == t.ID {
				g.kb = i
			}
			if inRun(o, run) {
				i++
			}
		}
	}
	win.endGesture() // a held terminal drag must not stay stuck under the menu
	win.wantFocus = true
	win.w.Invalidate()
}

func (win *window) closeGroupMenu() {
	if win.grp.run != 0 {
		win.grp.run = 0
		win.wantFocus = true
		win.w.Invalidate()
	}
}

// pruneGroupMenu closes the menu once its run has no tab left.
func (win *window) pruneGroupMenu(list []*tabs.Tab) {
	g := &win.grp
	if g.run == 0 {
		return
	}
	n := memberCount(list, g.run)
	if n == 0 {
		win.closeGroupMenu()
		return
	}
	g.kb = min(g.kb, n-1)
}

// groupIDs is the open menu's members in display order.
func (win *window) groupIDs() []tabs.TabID {
	var ids []tabs.TabID
	for _, t := range win.mgr.Tabs() {
		if inRun(t, win.grp.run) {
			ids = append(ids, t.ID)
		}
	}
	return ids
}

// pickMember shows a member, as a tab click does, and closes the menu.
func (win *window) pickMember(id tabs.TabID) {
	win.closeGroupMenu()
	win.selectTab(id)
}

// focusTarget is who gets the keyboard: the open menu, a settings text field,
// the settings page or the terminal.
func (win *window) focusTarget() event.Tag {
	switch {
	case win.grp.run != 0:
		return &win.grp.tag
	case win.set.shown:
		if f := win.set.fields[win.set.editing]; f != nil && win.set.editing != "" {
			return &f.ed
		}
		return &win.set.tag
	}
	return win.termTag
}

// groupEvents takes the menu's keys and clicks, and closes it on a click anywhere else.
func (win *window) groupEvents(gtx layout.Context) {
	tag := &win.grp.tag
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
			win.groupKey(gtx, e)
		case key.EditEvent:
			edited = true
		case pointer.Event:
			win.groupPress(e)
		}
	}
	if edited {
		// The number keys arrive as text too; drop Gio's copy so it cannot pile up.
		gtx.Execute(key.SnippetCmd{Tag: tag})
		gtx.Execute(key.SelectionCmd{Tag: tag})
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &win.grp.outTag, Kinds: pointer.Press})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok && !win.bench.IgnoreInput() {
			win.groupOutside(e)
		}
	}
}

// groupKey handles a key while the menu is open: 1-9, the arrows, Enter and Esc.
// kaze's own shortcuts close the menu and then do what they always do.
func (win *window) groupKey(gtx layout.Context, e key.Event) {
	if e.State != key.Press || win.grp.run == 0 {
		return
	}
	if a, ok := win.km[kinput.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
		win.closeGroupMenu()
		win.runShortcut(gtx, a)
		return
	}
	if e.Modifiers&^key.ModShift != 0 {
		return
	}
	ids := win.groupIDs()
	n := len(ids)
	if n == 0 {
		win.closeGroupMenu()
		return
	}
	g := &win.grp
	switch e.Name {
	case key.NameEscape:
		win.closeGroupMenu()
	case key.NameUpArrow:
		g.kb = (g.kb - 1 + n) % n
		g.reveal = true
	case key.NameDownArrow:
		g.kb = (g.kb + 1) % n
		g.reveal = true
	case key.NameReturn, key.NameEnter:
		win.pickMember(ids[min(g.kb, n-1)])
	default:
		// Only a bare digit picks: Shift+3 is a symbol on most layouts.
		if s := string(e.Name); e.Modifiers == 0 && len(s) == 1 && s[0] >= '1' && s[0] <= '9' && int(s[0]-'1') < n {
			win.pickMember(ids[s[0]-'1'])
		}
	}
	win.w.Invalidate()
}

// groupPress handles a click on the card: a row shows its member, its × closes it.
func (win *window) groupPress(e pointer.Event) {
	if e.Kind == pointer.Scroll && win.grp.run != 0 {
		g := &win.grp
		g.scroll = min(max(g.scroll+int(e.Scroll.Y), 0), g.scrollMax)
		g.reveal = false
		win.w.Invalidate()
		return
	}
	if e.Kind != pointer.Press || !e.Buttons.Contain(pointer.ButtonPrimary) || win.grp.run == 0 {
		return
	}
	p := e.Position.Round()
	for _, r := range win.grp.rows {
		switch {
		case p.In(r.close):
			win.mgr.Close(r.id) // the menu stays open for the others
			win.wantFocus = true
			win.w.Invalidate()
			return
		case p.In(r.rect):
			win.pickMember(r.id)
			return
		}
	}
}

// groupOutside closes the menu on a click outside it. A click on a chip is the
// strip's: it toggles or switches the menu there.
func (win *window) groupOutside(e pointer.Event) {
	p := e.Position.Round()
	if e.Kind != pointer.Press || p.In(win.grp.card) {
		return
	}
	if _, on := win.ui.hits.groupAt(p); on {
		return
	}
	win.closeGroupMenu()
}

// drawGroupMenu draws the open dropdown under its chip and takes its input:
// the card's clicks, and the clicks elsewhere that close it.
func (win *window) drawGroupMenu(gtx layout.Context, list []*tabs.Tab, top int) {
	g := &win.grp
	g.rows = g.rows[:0]
	if g.run == 0 {
		g.card = image.Rectangle{}
		return
	}
	c, ops := win.ui, gtx.Ops
	th := c.th
	size := gtx.Constraints.Max
	pad, rowH, hdH := gtx.Dp(6), gtx.Dp(46), gtx.Dp(28)
	w := max(min(gtx.Dp(380), size.X-gtx.Dp(16)), 0)
	x, drawn := 0, false
	for _, h := range c.hits.groups {
		if h.run == g.run {
			x, drawn = h.rect.Min.X+gtx.Dp(8), true
		}
	}
	if !drawn {
		win.closeGroupMenu() // its chip overflowed the strip: nothing to hang the menu from
		g.card = image.Rectangle{}
		return
	}
	x = max(min(x, size.X-w-gtx.Dp(8)), 0)
	n := memberCount(list, g.run)
	h := min(hdH+n*rowH+2*pad, max(size.Y-top-gtx.Dp(8), 0))
	card := image.Rect(x, top, x+w, top+h)
	g.card, g.bg = card, th.ToastBG

	rr := clip.UniformRRect(card, gtx.Dp(10))
	paint.FillShape(ops, th.ToastBG, rr.Op(ops))
	paint.FillShape(ops, th.ToastLine, clip.Stroke{Path: rr.Path(ops), Width: 1}.Op())
	cl := clip.Rect(card).Push(ops)

	var name string
	var shown tabs.TabID
	if t := win.shownTab(); t != nil {
		shown = t.ID
	}
	for _, t := range list {
		if inRun(t, g.run) {
			name = t.Group.Name
			break
		}
	}
	unit := "tabs"
	if n == 1 {
		unit = "tab"
	}
	hx, hmid := card.Min.X+pad+gtx.Dp(10), card.Min.Y+pad+hdH/2
	hdCall, hdSize := c.label(gtx, cutRunes(name, maxLabel)+" · "+strconv.Itoa(n)+" "+unit, 11, th.Muted)
	place(ops, hdCall, image.Pt(hx, hmid-hdSize.Y/2))
	state, needs := win.runState(g.run)
	stCol := th.Muted
	if needs {
		stCol = th.Needs
	}
	stCall, stSize := c.label(gtx, state, 11, stCol)
	place(ops, stCall, image.Pt(card.Max.X-pad-gtx.Dp(10)-stSize.X, hmid-stSize.Y/2))
	line := card.Min.Y + pad + hdH
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(card.Min.X+pad, line-1, card.Max.X-pad, line)).Op())
	viewport := image.Rect(card.Min.X+pad, line, card.Max.X-pad, max(line, card.Max.Y-pad))
	g.scrollMax = max(0, n*rowH-viewport.Dy())
	g.scroll = min(max(g.scroll, 0), g.scrollMax)
	if g.reveal {
		g.scroll = min(g.scroll, g.kb*rowH)
		g.scroll = max(g.scroll, (g.kb+1)*rowH-viewport.Dy())
		g.scroll = min(max(g.scroll, 0), g.scrollMax)
		g.reveal = false
	}
	rowsClip := clip.Rect(viewport).Push(ops)

	closeCall, closeSize := c.label(gtx, "×", 12, th.Muted)
	dot, gap, hitPad := gtx.Dp(7), gtx.Dp(8), gtx.Dp(4)
	y, i := line-g.scroll, 0
	for _, t := range list {
		if !inRun(t, g.run) {
			continue
		}
		r := image.Rect(card.Min.X+pad, y, card.Max.X-pad, y+rowH)
		if !r.Overlaps(viewport) {
			y += rowH
			i++
			continue
		}
		switch {
		case t.ID == shown:
			paint.FillShape(ops, pillTint(th.Accent), clip.UniformRRect(r, gtx.Dp(7)).Op(ops))
		case i == g.kb:
			paint.FillShape(ops, th.Border, clip.UniformRRect(r, gtx.Dp(7)).Op(ops))
		}
		mid := r.Min.Y + gtx.Dp(16)
		cx := r.Min.X + gtx.Dp(10)
		if i < 9 {
			kCall, kSize := c.label(gtx, strconv.Itoa(i+1), 11, th.Muted)
			place(ops, kCall, image.Pt(cx, mid-kSize.Y/2))
		}
		cx += gtx.Dp(18)
		mk := win.alerts.markOf(t.ID)
		c.memberDot(gtx, image.Rect(cx, mid-dot/2, cx+dot, mid-dot/2+dot), t.Color, mk)
		cx += dot + gap

		right := r.Max.X - gtx.Dp(10)
		cxClose := right - closeSize.X
		place(ops, closeCall, image.Pt(cxClose, mid-closeSize.Y/2))
		closeRect := image.Rect(cxClose-hitPad, mid-closeSize.Y/2-hitPad, right+hitPad, mid+closeSize.Y/2+hitPad)
		right = cxClose - gap
		if mk != markNone {
			txt := "needs you"
			if mk == markDone {
				txt = "done"
			}
			sCall, sSize := c.label(gtx, txt, 11, c.badgeColor(mk))
			right -= sSize.X
			place(ops, sCall, image.Pt(right, mid-sSize.Y/2))
			right -= gap
		}
		// Title and folder share what is left; a long one is clipped, never over the ×.
		text := clip.Rect(image.Rect(cx, r.Min.Y, max(right, cx), r.Max.Y)).Push(ops)
		tCall, tSize := c.label(gtx, c.tabLabelFit(t, win.defTitle), 12, th.ToastFG)
		place(ops, tCall, image.Pt(cx, mid-tSize.Y/2))
		m := win.memberOf(t)
		if m != nil && m.dir != "" {
			fCall, fSize := c.label(gtx, shortFolder(m.dir), 11, th.Muted)
			place(ops, fCall, image.Pt(cx+tSize.X+gap, mid-fSize.Y/2))
		}
		text.Pop()
		row := menuRow{id: t.ID, rect: r.Intersect(viewport), close: closeRect.Intersect(viewport)}
		if m != nil {
			// Second line: what the member runs, and how far it got at the right.
			mid2 := r.Min.Y + gtx.Dp(32)
			st, needs, failed := win.memberStatus(m, t)
			stCol := th.Muted
			switch {
			case needs:
				stCol = th.Needs
			case failed:
				stCol = th.UsageHigh
			}
			sCall, sSize := c.label(gtx, st, 11, stCol)
			sx := r.Max.X - gtx.Dp(10) - sSize.X
			place(ops, sCall, image.Pt(sx, mid2-sSize.Y/2))
			row.status = st
			if len(m.cmds) > 0 {
				row.detail = cleanText(strings.Join(m.cmds, " → "))
				dCall, dSize := c.fitLabel(gtx, row.detail, max(sx-gap-cx, 0), th.Muted)
				place(ops, dCall, image.Pt(cx, mid2-dSize.Y/2))
			}
		}
		g.rows = append(g.rows, row)
		y += rowH
		i++
	}
	rowsClip.Pop()
	cl.Pop()

	// Outside the card a click closes the menu. On the tab strip it also does what
	// it would anyway (switch tab, +, Settings); below it never reaches the terminal.
	pass := pointer.PassOp{}.Push(ops)
	area := clip.Rect{Max: image.Pt(size.X, top)}.Push(ops)
	event.Op(ops, &g.outTag)
	area.Pop()
	pass.Pop()
	area = clip.Rect{Min: image.Pt(0, top), Max: size}.Push(ops)
	event.Op(ops, &g.outTag)
	area.Pop()
	area = clip.Rect(card).Push(ops)
	event.Op(ops, &g.tag)
	area.Pop()
}
