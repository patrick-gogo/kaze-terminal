package ui

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/patrick-gogo/kaze-terminal/internal/claudesetup"
	kinput "github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// boardState is the agents board (spec 6.3): a page over the terminal area
// listing every subagent by column.
type boardState struct {
	open    bool
	tab     tabs.TabID // the session it is filtered to; 0 for all
	back    image.Rectangle
	all     image.Rectangle // "Show all" while filtered
	setup   image.Rectangle // the hooks hint's button
	clear   image.Rectangle // "Clear" on the Done column; empty when nothing is done
	cards   []boardHit
	texts   []string // drawn text, for tests
	scroll  int
	missing bool // the subagent hooks are not all set up
	tag     int
}

type boardCard struct {
	key                          subagents.Key
	tab                          tabs.TabID
	session                      bool // a session wait with no subagent named
	state                        subagents.State
	typ, task, where, time, line string
}

type boardHit struct {
	card  boardCard
	rect  image.Rectangle
	close image.Rectangle // the × on a done card; empty on the others
}

// agentsRunning is the status bar's text for n running and waiting subagents.
func agentsRunning(n int) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "1 agent running"
	}
	return strconv.Itoa(n) + " agents running"
}

func (win *window) openBoard(tab tabs.TabID) {
	win.dropSideEdit()
	win.hideSettings()
	b := &win.board
	b.open, b.tab, b.scroll = true, tab, 0
	b.missing = true
	if win.claudePath != "" {
		if _, r, err := win.readClaude(); err == nil {
			b.missing = false
			for _, e := range []claudesetup.Entry{r.AgentStart, r.AgentStop, r.Tools} {
				b.missing = b.missing || e.State != claudesetup.OK
			}
		}
	}
	win.endGesture()
	win.wantFocus = true
	if win.w != nil {
		win.w.Invalidate()
	}
}

func (win *window) closeBoard() {
	if !win.board.open {
		return
	}
	win.board.open = false
	win.wantFocus = true
	if win.w != nil {
		win.w.Invalidate()
	}
}

func (win *window) toggleBoard() {
	if win.board.open {
		win.closeBoard()
		return
	}
	win.openBoard(0)
}

// sessionName is a session's title as the sidebar shows it, and its repo.
func (win *window) sessionName(id tabs.TabID) (title, repo string) {
	t := win.mgr.Lookup(id)
	if t == nil {
		return "", ""
	}
	loc := win.locOf(t)
	title = win.sideIn().label(t, loc, win.agentOf(id)) // the sidebar's rule: a rename, the command's argument, the folder
	if title == "" {
		title = "session"
	}
	return title, loc.repoName
}

// boardColumns is the board's cards: running, waiting on you, done.
func (win *window) boardColumns() [3][]boardCard {
	var cols [3][]boardCard
	now := time.Now()
	run, wait, done := win.subs.Board()
	add := func(col int, as []*subagents.Agent) {
		for _, a := range as {
			if win.board.tab != 0 && tabs.TabID(a.Tab) != win.board.tab {
				continue
			}
			s := sideAgentOf(a, now)
			title, repo := win.sessionName(tabs.TabID(a.Tab))
			where := title
			if repo != "" && repo != title {
				where += " · " + repo
			}
			cols[col] = append(cols[col], boardCard{key: a.Key, tab: tabs.TabID(a.Tab), state: a.State,
				typ: a.Type, task: a.Task, where: where, time: s.Elapsed, line: s.Action})
		}
	}
	add(0, run)
	add(1, wait)
	for _, tab := range win.subs.SessionWaits() {
		id := tabs.TabID(tab)
		if win.board.tab != 0 && id != win.board.tab {
			continue
		}
		title, repo := win.sessionName(id)
		cols[1] = append(cols[1], boardCard{tab: id, session: true, state: subagents.Waiting, typ: "Session",
			task: title, where: repo, line: "a subagent may be asking"})
	}
	add(2, done)
	return cols
}

// drawBoard lays the board out in r.
func (win *window) drawBoard(gtx layout.Context, r image.Rectangle) {
	b := &win.board
	b.cards, b.texts = b.cards[:0], b.texts[:0]
	b.back, b.all, b.setup, b.clear = image.Rectangle{}, image.Rectangle{}, image.Rectangle{}, image.Rectangle{}
	c, ops := win.ui, gtx.Ops
	col := c.pageColors()
	text := func(s string, size unit.Sp, cl color.NRGBA, w font.Weight) (op.CallOp, image.Point) {
		b.texts = append(b.texts, s)
		return c.sans(gtx, s, size, cl, w)
	}
	cl := clip.Rect(r).Push(ops)
	defer cl.Pop()
	paint.FillShape(ops, col.bg, clip.Rect(r).Op())
	area := clip.Rect(r).Push(ops)
	event.Op(ops, &b.tag)
	area.Pop()

	pad := gtx.Dp(28)
	x0, x1 := r.Min.X+pad, r.Max.X-pad
	y := r.Min.Y + gtx.Dp(24)
	cols := win.boardColumns()

	// Header: the title and counts on the left, Back on the right.
	tc, ts := text("Agents", 22, col.title, font.SemiBold)
	place(ops, tc, image.Pt(x0, y))
	sub := strconv.Itoa(len(cols[0])) + " running · " + strconv.Itoa(len(cols[1])) + " waiting on you · " + strconv.Itoa(len(cols[2])) + " done"
	sc, ss := text(sub, 12, col.muted, font.Normal)
	place(ops, sc, image.Pt(x0, y+ts.Y+gtx.Dp(2)))
	if back, _ := win.sessionName(win.boardBack()); back != "" {
		bc, bs := text("Back to "+back+"  ×", 12, col.fg, font.Medium)
		bx := x1 - bs.X - 2*gtx.Dp(12)
		b.back = image.Rect(bx, y, x1, y+bs.Y+2*gtx.Dp(8))
		paint.FillShape(ops, col.raised, clip.UniformRRect(b.back, gtx.Dp(8)).Op(ops))
		place(ops, bc, image.Pt(bx+gtx.Dp(12), y+gtx.Dp(8)))
	}
	y += ts.Y + ss.Y + gtx.Dp(14)
	if b.tab != 0 {
		title, _ := win.sessionName(b.tab)
		fc, fs := text("Showing "+title+" only.", 12, col.muted, font.Normal)
		place(ops, fc, image.Pt(x0, y))
		ac, as := text("Show all", 12, col.accent, font.Medium)
		place(ops, ac, image.Pt(x0+fs.X+gtx.Dp(8), y))
		b.all = image.Rect(x0+fs.X+gtx.Dp(4), y-gtx.Dp(4), x0+fs.X+gtx.Dp(12)+as.X, y+as.Y+gtx.Dp(4))
		y += fs.Y + gtx.Dp(12)
	}
	if b.missing {
		hc, hs := text("kaze needs three more Claude Code hooks to see subagents.", 13, col.needs, font.Medium)
		place(ops, hc, image.Pt(x0, y+gtx.Dp(6)))
		bc, bs := text("Set up in Settings", 12, col.title, font.Medium)
		bx := x0 + hs.X + gtx.Dp(12)
		b.setup = image.Rect(bx, y, bx+bs.X+2*gtx.Dp(12), y+bs.Y+2*gtx.Dp(6))
		paint.FillShape(ops, col.raised, clip.UniformRRect(b.setup, gtx.Dp(8)).Op(ops))
		place(ops, bc, image.Pt(bx+gtx.Dp(12), y+gtx.Dp(6)))
		y += b.setup.Dy() + gtx.Dp(12)
	}

	// Columns, side by side when there is room, else stacked.
	names := [3]string{"RUNNING", "WAITING ON YOU", "DONE"}
	gap := gtx.Dp(16)
	wide := (x1 - x0) >= gtx.Dp(720)
	colW := x1 - x0
	if wide {
		colW = (x1 - x0 - 2*gap) / 3
	}
	cardH := gtx.Dp(84)
	body := image.Rect(r.Min.X, y, r.Max.X, r.Max.Y)
	bc := clip.Rect(body).Push(ops)
	top := y - b.scroll
	maxY := y
	cy := top
	for i := range cols {
		cx := x0
		if wide {
			cx = x0 + i*(colW+gap)
			cy = top
		}
		hc, hs := text(names[i]+"  "+strconv.Itoa(len(cols[i])), 11, col.dim, font.SemiBold)
		place(ops, hc, image.Pt(cx, cy))
		if i == 2 && len(cols[i]) > 0 {
			// Clear removes every finished card (only this session's while filtered).
			cc, cs := text("Clear", 11, col.accent, font.Medium)
			place(ops, cc, image.Pt(cx+colW-cs.X, cy))
			pd := gtx.Dp(6)
			if r := image.Rect(cx+colW-cs.X-pd, cy-pd, cx+colW+pd, cy+cs.Y+pd); r.Overlaps(body) {
				b.clear = r.Intersect(body)
			}
		}
		cy += hs.Y + gtx.Dp(8)
		if len(cols[i]) == 0 {
			ec, es := text("Nothing here.", 12, col.muted, font.Normal)
			place(ops, ec, image.Pt(cx, cy))
			cy += es.Y + gtx.Dp(12)
		}
		for _, card := range cols[i] {
			rect := image.Rect(cx, cy, cx+colW, cy+cardH)
			x := win.drawBoardCard(gtx, rect, card, text)
			if rect.Overlaps(body) {
				h := boardHit{card: card, rect: rect.Intersect(body)}
				if x.Overlaps(body) {
					h.close = x.Intersect(body)
				}
				b.cards = append(b.cards, h)
			}
			cy += cardH + gtx.Dp(10)
		}
		cy += gtx.Dp(14)
		maxY = max(maxY, cy)
	}
	bc.Pop()
	b.scroll = min(max(b.scroll, 0), max(0, maxY+b.scroll-r.Max.Y))
}

// drawBoardCard draws one card: type and task, where, time and its line. A done
// card gets a × in its top right corner; drawBoardCard returns where it is.
func (win *window) drawBoardCard(gtx layout.Context, r image.Rectangle, card boardCard, text func(string, unit.Sp, color.NRGBA, font.Weight) (op.CallOp, image.Point)) (close image.Rectangle) {
	c, ops := win.ui, gtx.Ops
	col := c.pageColors()
	paint.FillShape(ops, col.card, clip.UniformRRect(r, gtx.Dp(10)).Op(ops))
	stroke(ops, r, gtx.Dp(10), col.line)
	if card.state == subagents.Waiting {
		edge := image.Rect(r.Min.X, r.Min.Y+gtx.Dp(10), r.Min.X+gtx.Dp(3), r.Max.Y-gtx.Dp(10))
		paint.FillShape(ops, col.needs, clip.UniformRRect(edge, gtx.Dp(1)).Op(ops))
	}
	pad := gtx.Dp(14)
	x, w := r.Min.X+pad, r.Dx()-2*pad
	y := r.Min.Y + gtx.Dp(10)
	if card.state == subagents.Done && !card.session {
		xc, xs := text("×", 14, col.dim, font.Normal)
		place(ops, xc, image.Pt(r.Max.X-pad-xs.X, y-gtx.Dp(2)))
		hit := gtx.Dp(8)
		close = image.Rect(r.Max.X-pad-xs.X-hit, r.Min.Y, r.Max.X, y+xs.Y+hit)
		w -= xs.X + gtx.Dp(8) // keep the text clear of the ×
	}
	tc, ts := c.fitSide(gtx, card.typ, 12, w/2, c.memberColor("magenta"))
	place(ops, tc, image.Pt(x, y))
	win.board.texts = append(win.board.texts, card.typ, card.task)
	kc, _ := c.fitSide(gtx, card.task, 12, w-ts.X-gtx.Dp(8), col.title)
	place(ops, kc, image.Pt(x+ts.X+gtx.Dp(8), y))
	y += ts.Y + gtx.Dp(6)
	wc, ws := c.fitSide(gtx, card.where, 11, w, col.muted)
	place(ops, wc, image.Pt(x, y))
	y += ws.Y + gtx.Dp(6)
	line := card.line
	if card.time != "" {
		line = card.time + "  ·  " + line
	}
	lc, _ := c.fitSide(gtx, strings.TrimSpace(line), 11, w, col.dim)
	place(ops, lc, image.Pt(x, y))
	return close
}

// boardBack is the session the board returns to.
func (win *window) boardBack() tabs.TabID {
	if a := win.mgr.Active(); a != nil {
		return a.ID
	}
	return 0
}

func (win *window) boardEvents(gtx layout.Context) {
	tag := &win.board.tag
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: tag},
			key.Filter{Focus: tag, Optional: allMods},
			pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}},
		)
		if !ok {
			return
		}
		if win.bench.IgnoreInput() {
			continue
		}
		switch e := ev.(type) {
		case key.Event:
			if e.State == key.Press {
				if a, ok := win.km[kinput.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
					win.runShortcut(gtx, a)
					continue
				}
			}
			win.boardKey(e)
		case pointer.Event:
			switch {
			case e.Kind == pointer.Scroll:
				win.board.scroll += int(e.Scroll.Y)
				win.w.Invalidate()
			case e.Buttons.Contain(pointer.ButtonPrimary):
				win.boardPress(e.Position.Round())
			}
		}
	}
}

// boardKey handles a key on the board: Esc closes it.
func (win *window) boardKey(e key.Event) {
	if e.State == key.Press && e.Name == key.NameEscape {
		win.closeBoard()
	}
}

// boardPress handles a click on the board.
func (win *window) boardPress(p image.Point) {
	b := &win.board
	switch {
	case p.In(b.back):
		win.closeBoard()
		return
	case p.In(b.clear):
		win.subs.ClearDone(int(b.tab))
		win.dropGoneAgents()
		win.w.Invalidate()
		return
	case p.In(b.all):
		b.tab = 0
		win.w.Invalidate()
		return
	case p.In(b.setup):
		win.closeBoard()
		win.openSettings()
		for i, s := range win.sections() {
			if s.title == "Agents" {
				win.selectSection(i)
			}
		}
		return
	}
	for _, h := range b.cards {
		if p.In(h.close) {
			win.subs.Dismiss(h.card.key)
			win.dropGoneAgents()
			win.w.Invalidate()
			return
		}
	}
	for _, h := range b.cards {
		if !p.In(h.rect) {
			continue
		}
		win.closeBoard()
		if h.card.session {
			win.selectTab(h.card.tab)
		} else {
			win.openAgentPane(h.card.tab, h.card.key)
		}
		return
	}
}
