package ui

import (
	"image"
	"io"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

var agentSizeDp = image.Pt(460, 300)

// agentID is the pane id for subagent k: negative, so no tab can share it,
// and the same for k while kaze runs.
func (f *floats) agentID(k subagents.Key) tabs.TabID {
	if id, ok := f.agentIDs[k]; ok {
		return id
	}
	if f.agentIDs == nil {
		f.agentIDs = map[subagents.Key]tabs.TabID{}
	}
	f.lastAgent--
	f.agentIDs[k] = f.lastAgent
	return f.lastAgent
}

type agentLineKind uint8

const (
	lineWaiting agentLineKind = iota
	lineEarlier
	lineStep
	lineReportHead
	lineReport
)

// agentLine is one line of a live pane's body before wrapping.
type agentLine struct {
	kind              agentLineKind
	text              string
	at, tool, summary string // steps only
}

// agentPaneLines is what a live pane says about a: the waiting strip, the
// steps and, once done, the report (spec 6.2).
func agentPaneLines(a *subagents.Agent) []agentLine {
	var out []agentLine
	if a.State == subagents.Waiting {
		out = append(out, agentLine{kind: lineWaiting, text: "Waiting for your approval in the session below"})
	}
	if a.Dropped > 0 {
		out = append(out, agentLine{kind: lineEarlier, text: strconv.Itoa(a.Dropped) + " earlier steps"})
	}
	for _, s := range a.Steps {
		at := s.At.Format("15:04:05")
		text := at + "  " + s.Tool
		if s.Summary != "" {
			text += "  " + s.Summary
		}
		out = append(out, agentLine{kind: lineStep, text: text, at: at, tool: s.Tool, summary: s.Summary})
	}
	if a.State == subagents.Done {
		out = append(out, agentLine{kind: lineReportHead, text: "Report"})
		for _, l := range strings.Split(strings.TrimRight(a.Report, "\n"), "\n") {
			out = append(out, agentLine{kind: lineReport, text: l})
		}
	}
	return out
}

// agentScroll moves a live pane's body by d px with max the furthest scroll:
// at the bottom it follows new steps, scrolled up it stays put.
func (p *pane) agentScroll(d, bottom int) {
	if p.follow && d == 0 {
		p.scroll = bottom
		return
	}
	p.scroll = min(max(p.scroll+d, 0), bottom)
	p.follow = p.scroll >= bottom
}

// openAgentPane shows subagent k's live pane over its session, switching to
// that session first. The keyboard stays where it was: the pane only scrolls.
func (win *window) openAgentPane(tab tabs.TabID, k subagents.Key) {
	if win.subs.Get(k) == nil || win.mgr.Lookup(tab) == nil {
		return
	}
	if a := win.mgr.Active(); a == nil || a.ID != tab || win.set.shown || win.board.open {
		win.closeBoard()
		win.selectTab(tab)
	}
	id := win.fl.agentID(k)
	p, dropped := win.fl.open(tab, id, paneAgent)
	if dropped > 0 {
		win.mgr.Dock(dropped)
	}
	if p.agent != k {
		p.agent, p.follow = k, true
	}
	if win.w != nil {
		win.w.Invalidate()
	}
}

// dropGoneAgents closes live panes whose subagent the tracker forgot.
func (win *window) dropGoneAgents() {
	for _, s := range win.fl.sets {
		for _, p := range append([]*pane(nil), s.panes...) {
			if p.kind == paneAgent && win.subs.Get(p.agent) == nil {
				win.fl.remove(p.id)
			}
		}
	}
}

// agentLook is a live pane's header.
func (win *window) agentLook(a *subagents.Agent, front bool) paneLook {
	lk := paneLook{title: a.Type + "  " + a.Task, status: agentElapsed(a, time.Now()), front: front, bg: win.pal.BG, copyBtn: a.State == subagents.Done}
	switch a.State {
	case subagents.Waiting:
		lk.mark = markNeeds
	case subagents.Done:
		lk.mark = markDone
	}
	return lk
}

// drawAgentPane draws live pane p for a and takes its input areas.
func (win *window) drawAgentPane(gtx layout.Context, p *pane, a *subagents.Agent, front bool) {
	ops, c, th := gtx.Ops, win.ui, win.ui.th
	p.hits = c.layoutPane(gtx, p.hits.outer, win.agentLook(a, front))
	body := p.hits.body
	pad := gtx.Dp(10)
	inner := image.Rect(body.Min.X+pad, body.Min.Y+gtx.Dp(6), body.Max.X-pad, body.Max.Y-gtx.Dp(6))
	if inner.Empty() {
		return
	}
	lines := win.agentWrapped(gtx, p, a, inner.Dx())
	lh := gtx.Dp(18)
	total := len(lines) * lh
	p.agentScroll(0, max(total-inner.Dy(), 0))
	cl := clip.Rect(inner).Push(ops)
	y := inner.Min.Y - p.scroll
	for _, l := range lines {
		top := y
		y += lh
		if y < inner.Min.Y || top > inner.Max.Y {
			continue
		}
		mid := top + lh/2
		x := inner.Min.X
		switch l.kind {
		case lineWaiting:
			strip := image.Rect(inner.Min.X, top+gtx.Dp(1), inner.Max.X, y-gtx.Dp(1))
			paint.FillShape(ops, tint(th.Needs, 0x22), clip.UniformRRect(strip, gtx.Dp(4)).Op(ops))
			call, sz := c.sans(gtx, l.text, 11, th.Needs, font.Medium)
			place(ops, call, image.Pt(x+gtx.Dp(8), mid-sz.Y/2))
		case lineEarlier:
			call, sz := c.sans(gtx, l.text, 11, th.Muted, font.Normal)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
		case lineStep:
			call, sz := c.label(gtx, l.at, 10.5, th.Muted)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
			x += sz.X + gtx.Dp(10)
			call, sz = c.label(gtx, l.tool, 10.5, th.Accent)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
			x += sz.X + gtx.Dp(10)
			call, sz = c.fitSide(gtx, l.summary, 11, inner.Max.X-x, th.Folder)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
		case lineReportHead:
			paint.FillShape(ops, th.Border, clip.Rect(image.Rect(x, top+gtx.Dp(2), inner.Max.X, top+gtx.Dp(2)+1)).Op())
			call, sz := c.sans(gtx, l.text, 11, th.Done, font.SemiBold)
			place(ops, call, image.Pt(x, mid-sz.Y/2+gtx.Dp(2)))
		case lineReport:
			call, sz := c.sans(gtx, l.text, 12, th.ActiveFG, font.Normal)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
		}
	}
	cl.Pop()
	area := clip.Rect(body).Push(ops)
	event.Op(ops, &p.body)
	area.Pop()
}

// agentWrapped is a's lines with the report wrapped to w px, cached on p
// until a, its step count or w changes, so a still pane shapes nothing new.
func (win *window) agentWrapped(gtx layout.Context, p *pane, a *subagents.Agent, w int) []agentLine {
	key := [4]int{w, len(a.Steps) + a.Dropped, int(a.State), len(a.Report)}
	if p.wrapKey == key && p.wrapped != nil {
		return p.wrapped
	}
	var out []agentLine
	for _, l := range agentPaneLines(a) {
		if l.kind != lineReport {
			out = append(out, l)
			continue
		}
		parts := win.wrapSans(gtx, l.text, 12, w)
		if len(parts) == 0 {
			parts = []string{""}
		}
		for _, s := range parts {
			out = append(out, agentLine{kind: lineReport, text: s})
		}
	}
	p.wrapKey, p.wrapped = key, out
	return out
}

// wrapSans breaks s into lines no wider than w in the UI font.
func (win *window) wrapSans(gtx layout.Context, s string, size unit.Sp, w int) []string {
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		try := word
		if cur != "" {
			try = cur + " " + word
		}
		if _, sz := win.ui.sans(gtx, try, size, win.ui.th.Muted, font.Normal); sz.X > w && cur != "" {
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

// agentPaneEvents scrolls live panes and raises one on a click.
func (win *window) agentPaneEvents(gtx layout.Context, p *pane) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &p.body, Kinds: pointer.Press | pointer.Scroll,
			ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			return
		}
		e, ok := ev.(pointer.Event)
		if !ok || win.bench.IgnoreInput() {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			if o := win.fl.owner(p.id); o != 0 {
				win.fl.raise(o, p.id)
			}
		case pointer.Scroll:
			p.agentScroll(int(e.Scroll.Y), max(len(p.wrapped)*gtx.Dp(18)-p.hits.body.Dy()+gtx.Dp(12), 0))
		}
		win.w.Invalidate()
	}
}

// copyReport puts a's report on the clipboard.
func (win *window) copyReport(gtx layout.Context, a *subagents.Agent) {
	if win.toClipboard != nil {
		win.toClipboard(a.Report)
	} else {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(a.Report))})
	}
	win.note("Report copied")
}
