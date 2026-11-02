package ui

import (
	"image"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"golang.org/x/image/math/fixed"

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
	linePromptHead
	linePrompt
	lineActivity
	lineReportHead
	lineReport
	lineMessage
)

// agentLine is one line of a live pane's body before wrapping.
type agentLine struct {
	kind agentLineKind
	text string
}

// agentPaneLines is what a live pane says about a, read like a conversation:
// the prompt it was given, one line of what it has done (never the tool
// calls themselves), the waiting strip and, once done, its report (#78).
// The prompt and the report are one line each; agentWrapped splits them.
func agentPaneLines(a *subagents.Agent) []agentLine {
	var out []agentLine
	if p := trimBlock(a.Prompt); p != "" {
		out = append(out, agentLine{kind: linePromptHead, text: "Prompt"})
		out = append(out, agentLine{kind: linePrompt, text: p})
	}
	act := activityText(a.Activity)
	switch {
	case a.State != subagents.Done && act == "":
		out = append(out, agentLine{kind: lineActivity, text: "Working…"})
	case a.State != subagents.Done:
		out = append(out, agentLine{kind: lineActivity, text: act + " so far"})
	case act != "":
		out = append(out, agentLine{kind: lineActivity, text: act})
	}
	if a.State == subagents.Waiting {
		out = append(out, agentLine{kind: lineWaiting, text: "Waiting for your approval in the session below"})
	}
	if a.State == subagents.Done {
		out = append(out, agentLine{kind: lineReportHead, text: "Report"})
		out = append(out, agentLine{kind: lineReport, text: new(subagents.Transcript).Report(trimBlock(a.Report))})
	}
	return out
}

// trimBlock drops blank lines around s and trailing spaces, but keeps the
// first line's indent: a prompt that opens with a code block stays indented.
func trimBlock(s string) string {
	s = strings.TrimRight(s, " \t\r\n")
	for {
		line, rest, ok := strings.Cut(s, "\n")
		if !ok || strings.TrimSpace(line) != "" {
			return s
		}
		s = rest
	}
}

// activityText sums up a subagent's steps in one sentence, like Claude
// Code's collapsed view: "Searched for 13 patterns, ran 15 shell commands".
func activityText(a subagents.Activity) string {
	var parts []string
	add := func(n int, verb, one, many string) {
		if n == 0 {
			return
		}
		noun := many
		if n == 1 {
			noun = one
		}
		parts = append(parts, verb+" "+strconv.Itoa(n)+" "+noun)
	}
	add(a.Searches, "searched for", "pattern", "patterns")
	add(a.Commands, "ran", "shell command", "shell commands")
	add(a.Reads, "read", "file", "files")
	add(a.Edits, "edited", "file", "files")
	add(a.Web, "made", "web request", "web requests")
	add(a.Other, "used", "other tool", "other tools")
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, ", ")
	return strings.ToUpper(s[:1]) + s[1:]
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
	if a := win.mgr.Active(); a == nil || a.ID != tab || win.set.shown || win.board.open || win.docs.shown != 0 {
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
	for key := range win.fl.agentIDs {
		if win.subs.Get(key) == nil {
			delete(win.fl.agentIDs, key)
		}
	}
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
	win.pollAgentPane(gtx, p, a)
	ops, c, th := gtx.Ops, win.ui, win.ui.th
	p.hits = c.layoutPane(gtx, p.hits.outer, win.agentLook(a, front))
	body := p.hits.body
	inner := agentInner(gtx, body)
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
		case linePromptHead:
			call, sz := c.sans(gtx, l.text, 11, th.Accent, font.SemiBold)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
		case linePrompt:
			// The prompt is what you sent: a quiet bar on its left sets it apart from the answer.
			paint.FillShape(ops, th.Border, clip.Rect(image.Rect(x, top, x+gtx.Dp(2), y)).Op())
			call, sz := c.sans(gtx, l.text, 12, th.Muted, font.Normal)
			place(ops, call, image.Pt(x+gtx.Dp(10), mid-sz.Y/2))
		case lineActivity:
			call, sz := c.sans(gtx, l.text, 11, th.Muted, font.Normal)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
		case lineReportHead:
			paint.FillShape(ops, th.Border, clip.Rect(image.Rect(x, top+gtx.Dp(2), inner.Max.X, top+gtx.Dp(2)+1)).Op())
			call, sz := c.sans(gtx, l.text, 11, th.Done, font.SemiBold)
			place(ops, call, image.Pt(x, mid-sz.Y/2+gtx.Dp(2)))
		case lineReport, lineMessage:
			call, sz := c.sans(gtx, l.text, 12, th.ActiveFG, font.Normal)
			place(ops, call, image.Pt(x, mid-sz.Y/2))
		}
	}
	cl.Pop()
	area := clip.Rect(body).Push(ops)
	event.Op(ops, &p.body)
	area.Pop()
}

// agentWrapped is a's lines wrapped to w px. The whole result is cached on p
// until contents, activity or layout changes, so a still pane shapes nothing new;
// the prompt and report have caches of their own, so a new step lays out
// only the one-line summary again (PR 79 review).
func (win *window) agentWrapped(gtx layout.Context, p *pane, a *subagents.Agent, w int) []agentLine {
	key := paneWrapKey{width: w, prompt: a.Prompt, report: a.Report, activity: a.Activity, state: a.State, metric: gtx.Metric, locale: gtx.Locale, shaper: win.ui.shaper}
	if p.transcript != nil {
		key.streamVersion = p.transcript.Version
		key.streamAvailable = p.transcript.Available
	}
	if p.wrapKey == key && p.wrapped != nil {
		return p.wrapped
	}
	// Only a changed pane needs fallback discovery. Do it before consulting
	// the block caches, since adding a fallback changes the font collection.
	win.ui.fit(a.Prompt)
	win.ui.fit(a.Report)
	out := p.wrapped[:0]
	for _, l := range paneConversation(p, a) {
		switch l.kind {
		case linePrompt:
			out = append(out, win.wrapCached(gtx, &p.promptWrap, l, w-gtx.Dp(10))...) // room for the bar
		case lineReport:
			out = append(out, win.wrapCached(gtx, &p.reportWrap, l, w)...)
		case lineMessage:
			for _, s := range win.wrapBlock(gtx, l.text, 12, w) {
				out = append(out, agentLine{kind: l.kind, text: s})
			}
		case lineActivity:
			for _, s := range win.wrapSans(gtx, l.text, 11, w) {
				out = append(out, agentLine{kind: l.kind, text: s})
			}
		default:
			out = append(out, l)
		}
	}
	key.shaper = win.ui.shaper
	p.wrapKey, p.wrapped = key, out
	return out
}

// wrapCached is block l wrapped to w, from c unless its text or w changed.
func (win *window) wrapCached(gtx layout.Context, c *wrapCache, l agentLine, w int) []agentLine {
	if c.lines != nil && c.w == w && c.src == l.text && c.metric == gtx.Metric && c.locale == gtx.Locale && c.shaper == win.ui.shaper {
		return c.lines
	}
	lines := make([]agentLine, 0, strings.Count(l.text, "\n")+1)
	for _, s := range win.wrapBlock(gtx, l.text, 12, w) {
		lines = append(lines, agentLine{kind: l.kind, text: s})
	}
	c.src, c.w, c.lines = l.text, w, lines
	c.metric, c.locale, c.shaper = gtx.Metric, gtx.Locale, win.ui.shaper
	return lines
}

// wrapBlock wraps each line of s to w px, keeping blank lines and each
// line's indent (a tab counts as four spaces) on all of its pieces.
func (win *window) wrapBlock(gtx layout.Context, s string, size unit.Sp, w int) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		rest := strings.TrimLeft(line, " \t")
		if strings.TrimSpace(rest) == "" {
			out = append(out, "")
			continue
		}
		lead := line[:len(line)-len(rest)]
		indent := strings.ReplaceAll(lead, "\t", "    ")
		iw := 0
		if indent != "" {
			iw = win.ui.textWidth(gtx, indent, size, font.Font{Typeface: sansFace})
			if iw > w/2 {
				indent, iw = "", 0 // deep nesting in a narrow pane: the words matter more
			}
		}
		for _, part := range win.wrapSans(gtx, rest, size, w-iw) {
			out = append(out, indent+part)
		}
	}
	return out
}

// wrapSans breaks s into lines no wider than w in the UI font. A word wider
// than w on its own, such as a long path or URL, is broken where it fills
// the line, so nothing runs off the edge.
func (win *window) wrapSans(gtx layout.Context, s string, size unit.Sp, w int) []string {
	// Shape once at the actual width. Gio breaks oversized words at grapheme
	// boundaries and emits line rune counts; no temporary paint ops are needed.
	s = win.ui.fit(strings.Join(strings.Fields(s), " "))
	if s == "" {
		return nil
	}
	win.ui.shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: sansFace},
		PxPerEm: fixed.I(gtx.Sp(size)), MaxWidth: max(w-1, 1), Locale: gtx.Locale}, s)
	var lines []string
	start, end, runes := 0, 0, 0
	for g, ok := win.ui.shaper.NextGlyph(); ok; g, ok = win.ui.shaper.NextGlyph() {
		runes += int(g.Runes)
		if g.Flags&text.FlagLineBreak == 0 {
			continue
		}
		for ; runes > 0 && end < len(s); runes-- {
			_, n := utf8.DecodeRuneInString(s[end:])
			end += n
		}
		if line := strings.TrimSpace(s[start:end]); line != "" {
			lines = append(lines, line)
		}
		start = end
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
		win.agentPanePointer(gtx, p, e)
		if win.docs.shown != 0 {
			return
		}
		win.w.Invalidate()
	}
}

// copyReport puts a's report on the clipboard.
func (win *window) copyReport(gtx layout.Context, a *subagents.Agent) {
	if win.toClipboard != nil {
		win.toClipboard(win.agentReport(a))
	} else {
		gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(win.agentReport(a)))})
	}
	win.note("Report copied")
}
