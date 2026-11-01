package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"github.com/patrick-gogo/kaze-terminal/internal/resources"
)

func resourceValues(v resources.Value, partial bool) (string, string) {
	cpu, ram := "n/a", "n/a"
	if v.CPUValid {
		cpu = strconv.Itoa(int(math.Round(v.CPU))) + "%"
		if v.MissingCPU > 0 {
			cpu += "*"
		}
	}
	if v.RAMValid {
		ram = resources.Size(v.Bytes)
		if v.MissingRAM > 0 {
			ram += "*"
		}
	}
	if partial {
		if v.CPUValid && v.MissingCPU == 0 {
			cpu += "*"
		}
		if v.RAMValid && v.MissingRAM == 0 {
			ram += "*"
		}
	}
	return cpu, ram
}
func (c *chrome) resourceColor(level int) color.NRGBA {
	if level == 2 {
		return c.th.UsageHigh
	}
	if level == 1 {
		return c.th.Needs
	}
	return c.th.Accent
}

// Keep small card details readable on every theme's popup surface.
func (c *chrome) resourceMuted() color.NRGBA { return mix(c.th.Muted, c.th.ToastFG, 0.2) }

type resourceCardState struct {
	tag                   int
	box, viewport, bridge image.Rectangle
	scroll, maxScroll     int
	hovered               bool
}

func (win *window) resourceCardEvents(gtx layout.Context) {
	c := &win.stat.card
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: &c.tag, Kinds: pointer.Move | pointer.Enter | pointer.Leave | pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || win.bench.IgnoreInput() {
			continue
		}
		old := c.hovered
		c.hovered = e.Kind != pointer.Leave && (e.Position.Round().In(c.box) || e.Position.Round().In(c.bridge))
		if e.Kind == pointer.Leave && !c.hovered && !e.Position.Round().In(win.statusRect(grpMem)) && win.stat.hover == grpMem {
			win.stat.hover = grpNone
		}

		if e.Kind == pointer.Scroll {
			c.scroll = min(max(0, c.scroll+int(e.Scroll.Y)), c.maxScroll)
		}
		if old != c.hovered || e.Kind == pointer.Scroll {
			if win.w != nil {
				win.w.Invalidate()
			}
		}
	}
}
func (win *window) drawResourceCard(gtx layout.Context) {
	cs := &win.stat.card
	if win.stat.hover != grpMem && !cs.hovered {
		cs.box = image.Rectangle{}
		cs.bridge = image.Rectangle{}
		return
	}
	anchor := win.statusRect(grpMem)
	if anchor.Empty() {
		cs.hovered = false
		cs.box = image.Rectangle{}
		cs.bridge = image.Rectangle{}
		return
	}
	s := win.stat.m.Resources
	c, ops, th := win.ui, gtx.Ops, win.ui.th
	muted := c.resourceMuted()
	pad, gap := gtx.Dp(14), gtx.Dp(8)
	w := min(gtx.Dp(360), gtx.Constraints.Max.X-2*gap)
	if w < gtx.Dp(180) {
		return
	}
	inner := w - 2*pad
	p := &page{gtx: gtx, c: c, st: &settingsState{}, col: c.pageColors()}
	footer := "Includes Kaze and observed shells, agents and local servers. " + resources.MemoryMeaning() + " MB = 1,048,576 bytes; GB = 1,024 MB. Refreshes on activity, at most every 2 s; paused while minimized. GPU not measured. Processes detached before observation may be missed."
	if s.Total.MissingCPU > 0 || s.Total.MissingRAM > 0 {
		footer = fmt.Sprintf("Unavailable: CPU for %d, RAM for %d processes. ", s.Total.MissingCPU, s.Total.MissingRAM) + footer
	}
	if s.Partial {
		footer = "* Partial totals. " + footer
	}
	if s.Error != "" {
		footer = cleanText(s.Error) + ". " + footer
	}
	lines := p.wrap(footer, 10.5, inner)
	_, fs := c.sans(gtx, "Ag", 10.5, muted, font.Normal)
	lineH := max(fs.Y, gtx.Dp(14))
	rowH := gtx.Dp(22)
	headerH := gtx.Dp(144)
	footerH := len(lines)*lineH + gtx.Dp(12)
	available := anchor.Min.Y - 2*gap
	if available < gtx.Dp(40) {
		return
	}
	whole := available < headerH+footerH+2*pad+rowH
	visible := min(len(s.Rows), max(1, (available-headerH-footerH-2*pad)/rowH))
	h := min(available, headerH+visible*rowH+footerH+2*pad)
	x := max(gap, min(anchor.Max.X-w, gtx.Constraints.Max.X-w-gap))
	box := image.Rect(x, anchor.Min.Y-gap-h, x+w, anchor.Min.Y-gap)
	cs.box = box
	cs.bridge = image.Rect(min(box.Min.X, anchor.Min.X), box.Max.Y, max(box.Max.X, anchor.Max.X), anchor.Max.Y)
	viewportH := visible * rowH
	cs.maxScroll = max(0, len(s.Rows)*rowH-viewportH)
	if whole {
		cs.maxScroll = max(0, headerH+len(s.Rows)*rowH+footerH+2*pad-h)
	}
	cs.scroll = min(max(cs.scroll, 0), cs.maxScroll)
	paint.FillShape(ops, th.ToastBG, clip.UniformRRect(box, gtx.Dp(11)).Op(ops))
	stroke(ops, box, gtx.Dp(11), th.ToastLine)
	cl := clip.Rect(box.Inset(1)).Push(ops)
	y := box.Min.Y + pad
	if whole {
		y -= cs.scroll
	}
	top := y
	text := func(s string, size unit.Sp, col color.NRGBA, weight font.Weight) {
		call, sz := c.sans(gtx, s, size, col, weight)
		place(ops, call, image.Pt(box.Min.X+pad, y))
		y += sz.Y + gtx.Dp(6)
	}
	text("Kaze resource usage", 12.5, th.ToastFG, font.SemiBold)
	cpu, ram := resourceValues(s.Total, s.Partial)
	value := func(label, v string, meter bool) {
		call, sz := c.sans(gtx, label, 11.5, muted, font.Medium)
		place(ops, call, image.Pt(box.Min.X+pad, y))
		vc, vs := c.label(gtx, v, 11, th.ToastFG)
		place(ops, vc, image.Pt(box.Max.X-pad-vs.X, y))
		if meter && inner > gtx.Dp(180) {
			pct := 0
			if s.Total.CPUValid {
				pct = int(s.Total.CPU)
			}
			mx := box.Min.X + pad + gtx.Dp(48)
			c.meter(gtx, mx, y+max(sz.Y, vs.Y)/2, max(1, inner-gtx.Dp(110)), pct, c.resourceColor(s.Level))
		}
		y += max(sz.Y, vs.Y) + gtx.Dp(6)
	}
	value("CPU", cpu, true)
	cores := "CPU core count unavailable"
	if s.Cores > 0 {
		cores = fmt.Sprintf("100%% = all %d logical cores busy", s.Cores)
	}
	text(cores, 10.5, muted, font.Normal)
	value("RAM", ram, false)
	age := "Waiting for a reading"
	if !s.At.IsZero() {
		age = "Sampled " + ago(max(time.Since(s.At), 0))
	}
	text(age, 10.5, muted, font.Normal)
	y = top + headerH - rowH
	text("BY SESSION", 9.5, muted, font.Bold)
	// Column headers and values use the same anchors.
	ramRight, cpuRight := box.Max.X-pad, box.Max.X-pad-gtx.Dp(86)
	for _, col := range []struct {
		text  string
		right int
	}{{"CPU", cpuRight}, {"RAM", ramRight}} {
		call, sz := c.sans(gtx, col.text, 9.5, muted, font.Bold)
		place(ops, call, image.Pt(col.right-sz.X, top+headerH-rowH))
	}
	rowTop := top + headerH
	footerTop := box.Max.Y - pad - footerH
	if whole {
		footerTop = rowTop + len(s.Rows)*rowH
	}
	cs.viewport = image.Rect(box.Min.X+pad, rowTop, box.Max.X-pad, footerTop)
	if whole {
		cs.viewport = box.Inset(pad)
	}
	rowClip := clip.Rect(cs.viewport).Push(ops)
	for i, r := range s.Rows {
		yy := rowTop + i*rowH
		if !whole {
			yy -= cs.scroll
		}
		if yy+rowH <= cs.viewport.Min.Y || yy >= cs.viewport.Max.Y {
			continue
		}
		name := r.Name
		if r.Closed {
			name += " (closed)"
		}
		a, b := resourceValues(r.Value, false)
		if r.CPUValid {
			a = strconv.FormatFloat(r.CPU, 'f', 1, 64) + "%"
			if r.MissingCPU > 0 {
				a += "*"
			}
		}
		bc, bs := c.label(gtx, b, 11, th.ToastFG)
		place(ops, bc, image.Pt(ramRight-bs.X, yy))
		ac, as := c.label(gtx, a, 11, th.ToastFG)
		place(ops, ac, image.Pt(cpuRight-as.X, yy))
		nc, _ := c.sans(gtx, p.fitSans(name, 11.5, max(1, inner-gtx.Dp(156)), font.Medium), 11.5, th.ToastFG, font.Medium)
		place(ops, nc, image.Pt(box.Min.X+pad, yy))
	}
	rowClip.Pop()
	y = footerTop + gtx.Dp(8)
	paint.FillShape(ops, th.ToastLine, clip.Rect(image.Rect(box.Min.X+pad, footerTop, box.Max.X-pad, footerTop+1)).Op())
	for _, line := range lines {
		fc, _ := c.sans(gtx, line, 10.5, muted, font.Normal)
		place(ops, fc, image.Pt(box.Min.X+pad, y))
		y += lineH
	}
	if cs.maxScroll > 0 {
		track := cs.viewport
		if whole {
			track = box.Inset(pad)
		}
		thumbH := max(gtx.Dp(18), track.Dy()*track.Dy()/max(1, track.Dy()+cs.maxScroll))
		thumbY := track.Min.Y + (track.Dy()-thumbH)*cs.scroll/cs.maxScroll
		paint.FillShape(ops, th.ToastLine, clip.UniformRRect(image.Rect(box.Max.X-gtx.Dp(6), thumbY, box.Max.X-gtx.Dp(3), thumbY+thumbH), gtx.Dp(2)).Op(ops))
	}

	cl.Pop()
	// Pass pointer events through so this hover-only surface cannot block actions
	// on the status bar or terminal beneath it. Scroll is consumed by its filter.
	pass := pointer.PassOp{}.Push(ops)
	hit := clip.Rect(box.Union(cs.bridge)).Push(ops)
	event.Op(ops, &cs.tag)
	hit.Pop()
	pass.Pop()
}
