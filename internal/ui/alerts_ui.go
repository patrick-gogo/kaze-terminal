package ui

import (
	"cmp"
	"image"
	"image/color"
	"slices"
	"sync"
	"time"
	"unicode/utf8"

	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// toastHit is where a toast, its text and its buttons were drawn, in window pixels.
type toastHit struct {
	id                            tabs.TabID
	run                           uint64
	rect, text, goRect, closeRect image.Rectangle
}

// inbox carries alerts from notify goroutines to the UI goroutine. It keeps
// only what changes the outcome per tab (its latest alert, its latest "needs"
// and its latest prompt), so a burst while minimized, when no frames run, can
// neither block the sender nor lose a tab's last word.
type inbox struct {
	mu    sync.Mutex
	seq   uint64
	byTab map[tabs.TabID]*pending
}

type queued struct {
	m     notify.Msg
	seq   uint64
	shell uint64 // shellSeq of the tab when the message arrived
}

type pending struct {
	needs, prompt *queued
	last          queued
}

// put queues m; shell is the tab's shellSeq now, at arrival.
func (b *inbox) put(m notify.Msg, shell uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	id := tabs.TabID(m.Tab)
	if b.byTab == nil {
		b.byTab = map[tabs.TabID]*pending{}
	}
	p := b.byTab[id]
	if p == nil {
		p = &pending{}
		b.byTab[id] = p
	}
	q := queued{m, b.seq, shell}
	switch m.Kind {
	case notify.KindNeeds:
		p.needs = &q
	case notify.KindPrompt:
		if m.Command == "" && p.prompt != nil {
			// A plain prompt keeps the shown command, so carry it as delivering both would.
			if q.m.Agent == p.prompt.m.Agent {
				q.m.Command, q.m.Args = p.prompt.m.Command, p.prompt.m.Args
			}
		}
		p.prompt = &q
	}
	p.last = q
}

// items is the tab's kept messages in arrival order.
func (p *pending) items() []queued {
	out := []queued{p.last}
	for _, q := range []*queued{p.needs, p.prompt} {
		if q != nil && q.seq != p.last.seq {
			out = append(out, *q)
		}
	}
	slices.SortFunc(out, func(a, b queued) int { return cmp.Compare(a.seq, b.seq) })
	return out
}

// take empties the inbox and returns the alerts in arrival order.
func (b *inbox) take() []queued {
	b.mu.Lock()
	all := b.byTab
	b.byTab = nil
	b.mu.Unlock()
	var qs []queued
	for _, p := range all {
		qs = append(qs, p.items()...)
	}
	slices.SortFunc(qs, func(a, b queued) int { return cmp.Compare(a.seq, b.seq) })
	return qs
}

func (b *inbox) size() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := 0
	for _, p := range b.byTab {
		n += len(p.items())
	}
	return n
}

// publish mirrors the active tab and focus for onNotify, which runs off the
// UI goroutine. The UI goroutine calls it whenever either may have changed.
func (win *window) publish(active *tabs.Tab, focused bool) {
	var id tabs.TabID
	if active != nil {
		id = active.ID
	}
	win.snapActive.Store(int64(id))
	win.snapFocused.Store(focused)
}

// onNotify runs on a notify connection goroutine and must not block. The
// chime and taskbar flash are decided here, not on a frame: Gio sends no
// frames to a minimized window, which is exactly when the owner is away.
func (win *window) onNotify(m notify.Msg) {
	id := tabs.TabID(m.Tab)
	src := m.Tab // the tab the hook ran in, before a scratch shell maps to its session
	var t *tabs.Tab
	if mgr := win.liveMgr.Load(); mgr != nil {
		// A scratch shell's messages count on its session's row (spec 8.3).
		if p := hookTab(mgr, id, m.Kind); p != id {
			id, m.Tab = p, int(p)
		}
		t = mgr.Lookup(id)
	}
	if t == nil {
		return // closed or unknown tab
	}
	// Taken now, so a shell prompt that comes back before the next frame still
	// counts as after this message: it hides ctx and ends Claude.
	shell := shellSeq(t)
	if m.Kind != notify.KindUsage {
		win.subIn.put(eventsOf(m, src, time.Now())...)
	}
	if subKind(m.Kind) {
		if win.w != nil {
			win.w.Invalidate() // state only: never a chime, flash, toast or mark
		}
		return
	}
	if m.Kind == notify.KindUsage {
		// Never an alert: no chime, flash or mark.
		if m.Usage != nil {
			win.usageIn.put(id, *m.Usage, cwdSeq(t), shell)
		}
		if win.w != nil {
			win.w.Invalidate()
		}
		return
	}
	if m.Kind == notify.KindPrompt || m.Kind == notify.KindInterrupt {
		win.inbox.put(m, shell) // state only: never a chime, flash or toast
		if win.w != nil {
			win.w.Invalidate()
		}
		return
	}
	focused := win.snapFocused.Load()
	if id != tabs.TabID(win.snapActive.Load()) || !focused {
		// The settings page changes these on the UI goroutine.
		win.alertMu.Lock()
		cfg, chime := &win.opt.Config, win.chime // only these fields: others change unlocked
		ring := m.Kind == notify.KindNeeds && cfg.ChimeNeeds || m.Kind == notify.KindDone && cfg.ChimeDone
		flash := m.Kind == notify.KindNeeds && !focused && cfg.TaskbarFlash
		win.alertMu.Unlock()
		if chime != nil && ring {
			chime(time.Now()) // non-blocking; the player coalesces bursts
		}
		if flash && win.flash != nil {
			win.flash()
		}
	}
	win.inbox.put(m, shell)
	if win.w != nil {
		win.w.Invalidate()
	}
}

// drainAlerts applies every queued alert; UI goroutine only.
func (win *window) drainAlerts(now time.Time) {
	for _, q := range win.inbox.take() {
		win.deliver(q, now)
	}
}

// deliver updates the agent state, marks and toasts for one alert; the chime
// already happened in onNotify.
func (win *window) deliver(q queued, now time.Time) {
	m := q.m
	var active tabs.TabID // none while settings hides the terminal
	if t := win.shownTab(); t != nil {
		active = t.ID
	}
	if t := win.mgr.Lookup(tabs.TabID(m.Tab)); t != nil {
		win.noteAgent(m, t.ID, q.shell)
		win.wantCount(t)
		win.staleBranch(t) // a background row has no other reason to re-read its branch
	}
	win.alerts.doneToast = win.opt.Config.DoneToast
	win.alerts.apply(m, active, win.focused, win.mgr.Has, now)
}

// selectTab is the one tab switch for a tab click and a toast's "Go ›".
func (win *window) selectTab(id tabs.TabID) {
	win.dropSideEdit()
	win.hideSettings()
	win.closeBoard()
	win.mgr.SetActive(id)
	win.wantFocus = true
}

// visit clears the shown tab's alert, but only when the owner can see it.
func (win *window) visit(t *tabs.Tab) {
	if t != nil && win.focused {
		win.alerts.visited(t.ID)
	}
}

// toastEvent handles a press on a toast: "Go ›" switches, × dismisses.
func (win *window) toastEvent(e pointer.Event) {
	if e.Kind != pointer.Press || !e.Buttons.Contain(pointer.ButtonPrimary) || win.bench.IgnoreInput() {
		return
	}
	p := e.Position.Round()
	for _, h := range win.ui.toastHits {
		switch {
		case p.In(h.closeRect):
			win.alerts.dismiss(h.id, h.run)
			win.wantFocus = true
			win.w.Invalidate()
			return
		case p.In(h.goRect):
			win.selectTab(h.id)
			win.w.Invalidate()
			return
		case p.In(h.rect):
			return
		}
	}
}

// fitLabel shortens s from the right until it fits room; empty when nothing
// does. Bounded so a bad measure cannot spin.
func (c *chrome) fitLabel(gtx layout.Context, s string, room int, col color.NRGBA) (op.CallOp, image.Point) {
	call, sz := c.label(gtx, s, 12, col)
	for i := 0; i < 8 && sz.X > room && s != ""; i++ {
		n := utf8.RuneCountInString(s)
		keep := min(n*max(room, 0)/max(sz.X, 1), n-1)
		if keep <= 1 {
			s = ""
		} else {
			s = truncateRight(s, keep)
		}
		call, sz = c.label(gtx, s, 12, col)
	}
	if sz.X > room {
		call, sz = c.label(gtx, "", 12, col)
		sz.X = 0
	}
	return call, sz
}

// drawToasts draws the toasts bottom-right of area, the terminal's part of the window
// right of the sidebar, newest on top, and records where they are for clicks. When the
// room is short the oldest cards are left out; when it is too narrow, all of them.
func (win *window) drawToasts(gtx layout.Context, area image.Rectangle) {
	c := win.ui
	c.toastHits = c.toastHits[:0]
	list := win.alerts.list()
	if len(list) == 0 {
		return
	}
	titles := map[tabs.TabID]string{}
	for _, t := range win.mgr.Tabs() {
		titles[t.ID] = tabLabelWith(t, win.defTitle, c.fit)
	}
	ops := gtx.Ops
	margin, gap := gtx.Dp(14), gtx.Dp(8)
	padX, padY, inner := gtx.Dp(12), gtx.Dp(10), gtx.Dp(10)
	dot, radius, line := gtx.Dp(7), gtx.Dp(10), max(gtx.Dp(1), 1)
	maxW := max(min(gtx.Dp(460), area.Dx()-2*margin), 0)

	goCall, goSz := c.label(gtx, "Go ›", 12, c.th.ToastGo)
	xCall, xSz := c.label(gtx, "×", 12, c.th.Muted)
	lead := padX + dot + inner
	tail := inner + goSz.X + inner + xSz.X + padX
	textRoom := maxW - lead - tail
	if textRoom < gtx.Dp(24) {
		return // too narrow for a readable card; the sidebar's marks still show
	}

	type card struct {
		id              tabs.TabID
		run             uint64
		kind            notify.Kind
		title, rest     op.CallOp
		titleSz, restSz image.Point
		w, h            int
	}
	top, bottom := area.Min.Y+gap, area.Max.Y-gtx.Dp(16)
	var cards []card
	total := 0
	for _, t := range list {
		k := card{id: t.Tab, run: t.Run, kind: t.Kind}
		title, rest := titles[t.Tab], " is done"
		if t.Run != 0 {
			title, rest = t.Title, t.Text // a workspace run's summary
		}
		k.title, k.titleSz = c.fitLabel(gtx, title, textRoom, c.th.ToastTitle)
		if t.Run == 0 && t.Kind == notify.KindNeeds {
			rest = " needs you"
			if t.Text != "" {
				rest += " · " + t.Text
			}
		}
		k.rest, k.restSz = c.fitLabel(gtx, rest, textRoom-k.titleSz.X, c.th.ToastFG)
		k.w = lead + k.titleSz.X + k.restSz.X + tail
		k.h = max(k.titleSz.Y, k.restSz.Y, goSz.Y) + 2*padY
		if total+k.h > bottom-top {
			break // newest first, so the oldest are the ones left out
		}
		total += k.h + gap
		cards = append(cards, k)
	}
	y := bottom - (total - gap)
	for _, k := range cards {
		r := image.Rect(area.Max.X-margin-k.w, y, area.Max.X-margin, y+k.h)
		paint.FillShape(ops, c.th.ToastLine, clip.UniformRRect(r, radius).Op(ops))
		paint.FillShape(ops, c.th.ToastBG, clip.UniformRRect(r.Inset(line), max(radius-line, 0)).Op(ops))
		mid := r.Min.Y + k.h/2
		x := r.Min.X + padX
		dc := c.th.Done
		if k.kind == notify.KindNeeds {
			dc = c.th.Needs
		}
		paint.FillShape(ops, dc, clip.Ellipse(image.Rect(x, mid-dot/2, x+dot, mid-dot/2+dot)).Op(ops))
		x += dot + inner
		// Buttons are pinned to the right edge so they stay clickable.
		xr := r.Max.X - padX - xSz.X
		gx := xr - inner - goSz.X
		text := image.Rect(x, r.Min.Y, min(x+k.titleSz.X+k.restSz.X, gx-inner), r.Max.Y)
		cl := clip.Rect(text).Push(ops) // a mismeasured label still never runs under Go
		place(ops, k.title, image.Pt(x, mid-k.titleSz.Y/2))
		place(ops, k.rest, image.Pt(x+k.titleSz.X, mid-k.restSz.Y/2))
		cl.Pop()
		place(ops, goCall, image.Pt(gx, mid-goSz.Y/2))
		place(ops, xCall, image.Pt(xr, mid-xSz.Y/2))
		hp := gtx.Dp(4)
		c.toastHits = append(c.toastHits, toastHit{
			id:        k.id,
			run:       k.run,
			rect:      r,
			text:      text,
			goRect:    image.Rect(gx-hp, r.Min.Y+line, gx+goSz.X+hp/2, r.Max.Y-line),
			closeRect: image.Rect(xr-hp/2, r.Min.Y+line, min(xr+xSz.X+hp, r.Max.X-line), r.Max.Y-line),
		})
		y += k.h + gap
	}
}

// truncateRight keeps the first max runes and ends with "…".
func truncateRight(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	if max < 1 {
		max = 1
	}
	r := []rune(s)
	return string(r[:max-1]) + "…"
}
