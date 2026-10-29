package ui

import (
	"fmt"
	"image"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

type bannerKind int

const (
	bannerBody bannerKind = iota
	bannerShell
	bannerRun
)

type bannerHit struct {
	rect image.Rectangle
	kind bannerKind
}

type bannerButton struct {
	kind bannerKind
	text string
}

// bannerButtons are the banner's buttons, left to right. A failure only offers to
// hide the banner; Run commands shows once the owner is asked about the prompt.
func bannerButtons(r *workspaces.TabRun) []bannerButton {
	switch r.State {
	case workspaces.Failed:
		return []bannerButton{{bannerShell, "Dismiss"}}
	case workspaces.NeedsConfirmation:
		return []bannerButton{{bannerRun, "Run commands"}, {bannerShell, "Use shell"}}
	}
	return []bannerButton{{bannerShell, "Use shell"}}
}

// bannerText is what a waiting or failed member's banner says.
func bannerText(r *workspaces.TabRun) string {
	switch {
	case r.State == workspaces.Failed:
		return r.Err
	case r.State == workspaces.Waiting && !r.DepReady && r.Wait == workspaces.WaitPort:
		return fmt.Sprintf("Waiting for port %d…", r.Port)
	case r.State == workspaces.Waiting && !r.DepReady:
		return "Waiting for Docker…"
	}
	return "Waiting for shell prompt"
}

func bannerShown(m *wsMember) bool {
	if m == nil {
		return false
	}
	switch m.run.State {
	case workspaces.Waiting, workspaces.NeedsConfirmation:
		return true
	case workspaces.Failed:
		return !m.hideBanner
	}
	return false
}

// drawWsBanner draws the shown tab's launch banner over the top of the
// terminal area, never into the shell, and records its buttons.
func (win *window) drawWsBanner(gtx layout.Context, t *tabs.Tab, termRect image.Rectangle) {
	win.wsr.hits = win.wsr.hits[:0]
	m := win.memberOf(t)
	if !bannerShown(m) {
		return
	}
	win.wsr.hitTab = t.ID
	c, ops := win.ui, gtx.Ops
	margin, padX, padY, gap := gtx.Dp(8), gtx.Dp(12), gtx.Dp(6), gtx.Dp(8)
	radius, line := gtx.Dp(8), max(gtx.Dp(1), 1)

	btns := bannerButtons(&m.run)
	r := image.Rect(termRect.Min.X+margin, termRect.Min.Y+margin, termRect.Max.X-margin, termRect.Min.Y+margin)
	btnsW := 0
	type placed struct {
		bannerButton
		w, h int
	}
	var bs []placed
	for _, b := range btns {
		_, sz := c.label(gtx, b.text, 12, c.th.ToastGo)
		bs = append(bs, placed{b, sz.X + 2*padX, sz.Y + 2*padY})
		btnsW += sz.X + 2*padX + gap
	}
	room := r.Dx() - 2*padX - btnsW
	headCol := c.th.ToastTitle
	if m.run.State == workspaces.Failed {
		headCol = c.th.Needs
	}
	head, headSz := c.fitLabel(gtx, bannerText(&m.run), room, headCol)
	body, bodySz := c.fitLabel(gtx, m.run.Line, room, c.th.Muted)
	r.Max.Y = r.Min.Y + headSz.Y + bodySz.Y + 2*padY + line
	if r.Dx() <= 0 {
		return
	}
	paint.FillShape(ops, c.th.ToastLine, clip.UniformRRect(r, radius).Op(ops))
	paint.FillShape(ops, c.th.ToastBG, clip.UniformRRect(r.Inset(line), max(radius-line, 0)).Op(ops))
	text := clip.Rect(image.Rect(r.Min.X+padX, r.Min.Y, r.Min.X+padX+max(room, 0), r.Max.Y)).Push(ops)
	place(ops, head, image.Pt(r.Min.X+padX, r.Min.Y+padY))
	place(ops, body, image.Pt(r.Min.X+padX, r.Min.Y+padY+headSz.Y+line))
	text.Pop()
	win.wsr.hits = append(win.wsr.hits, bannerHit{rect: r, kind: bannerBody})

	x := r.Max.X - padX
	for i := len(bs) - 1; i >= 0; i-- {
		b := bs[i]
		br := image.Rect(x-b.w, r.Min.Y+(r.Dy()-b.h)/2, x, r.Min.Y+(r.Dy()-b.h)/2+b.h)
		paint.FillShape(ops, c.th.ToastLine, clip.UniformRRect(br, radius/2).Op(ops))
		paint.FillShape(ops, c.th.ToastBG, clip.UniformRRect(br.Inset(line), max(radius/2-line, 0)).Op(ops))
		call, sz := c.label(gtx, b.text, 12, c.th.ToastGo)
		place(ops, call, image.Pt(br.Min.X+(br.Dx()-sz.X)/2, br.Min.Y+(br.Dy()-sz.Y)/2))
		win.wsr.hits = append(win.wsr.hits, bannerHit{rect: br, kind: b.kind})
		x -= b.w + gap
	}
	// Above the terminal's own area, so a click here never reaches the shell.
	area := clip.Rect(r).Push(ops)
	event.Op(ops, &win.wsr.tag)
	area.Pop()
}

func (win *window) bannerEvents(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &win.wsr.tag, Kinds: pointer.Press})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok {
			win.bannerPress(e)
		}
	}
}

// bannerPress handles a press on the banner: Use shell hands the tab to the
// owner (or hides a failure), Run commands confirms the prompt.
func (win *window) bannerPress(e pointer.Event) {
	if e.Kind != pointer.Press || !e.Buttons.Contain(pointer.ButtonPrimary) || win.bench.IgnoreInput() {
		return
	}
	t := win.mgr.Lookup(win.wsr.hitTab)
	m := win.memberOf(t)
	if m == nil {
		return
	}
	p := e.Position.Round()
	for i := len(win.wsr.hits) - 1; i >= 0; i-- { // buttons were added after the body
		h := win.wsr.hits[i]
		if !p.In(h.rect) {
			continue
		}
		switch h.kind {
		case bannerShell:
			if m.run.State == workspaces.Failed {
				m.hideBanner = true
			} else {
				win.wsEvent(m, t, workspaces.UserInputEvent{})
			}
		case bannerRun:
			win.wsEvent(m, t, workspaces.RunCommandsEvent{})
		}
		win.wantFocus = true
		if win.w != nil {
			win.w.Invalidate()
		}
		return
	}
}
