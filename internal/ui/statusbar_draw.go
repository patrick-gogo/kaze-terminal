package ui

import (
	"image"
	"image/color"
	"strconv"
	"strings"
	"time"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

// grpNone is no status group: nothing hovered.
const grpNone statusGroup = -1

// statusState is the status bar as last drawn, for input.
type statusState struct {
	tag   *int
	l     statusLayout
	m     statusModel
	y     int         // the bar's top in window px
	hover statusGroup // grpNone when the pointer is elsewhere
}

func statusDimsFor(gtx layout.Context) statusDims {
	return statusDims{H: gtx.Dp(32), Pad: gtx.Dp(6), ItemPad: gtx.Dp(9), Sep: gtx.Dp(9), Icon: gtx.Dp(13),
		Gap: gtx.Dp(6), Meter: gtx.Dp(38), PillPad: gtx.Dp(7), Dot: gtx.Dp(6)}
}

// clickable reports whether a status group does something when clicked (spec 6.2).
func clickable(g statusGroup) bool {
	return g == grpUpdate || g == grpAgents || g == grpServers || g == grpUsage
}

// statusRect is a group's rectangle in window px, empty when not shown.
func (win *window) statusRect(g statusGroup) image.Rectangle {
	r, ok := win.stat.l.Groups[g]
	if !ok {
		return image.Rectangle{}
	}
	return r.Add(image.Pt(0, win.stat.y))
}

// statusText measures text the way the status bar draws it.
func (c *chrome) statusMeasure(gtx layout.Context) func(string, bool) int {
	return func(s string, mono bool) int {
		if mono {
			_, sz := c.label(gtx, s, 11, c.th.Muted)
			return sz.X
		}
		_, sz := c.sans(gtx, s, 11, c.th.Muted, font.SemiBold)
		return sz.X
	}
}

// drawStatus lays out and draws the status bar at y, and takes its input.
func (win *window) drawStatus(gtx layout.Context, y int, now time.Time) {
	c, ops, th := win.ui, gtx.Ops, win.ui.th
	d := statusDimsFor(gtx)
	width := gtx.Constraints.Max.X
	m := win.statusModel(now)
	l := layoutStatusBar(m, width, c.statusMeasure(gtx), d)
	if _, ok := l.Groups[win.stat.hover]; !ok {
		win.stat.hover = grpNone
	}
	win.stat.l, win.stat.m, win.stat.y = l, m, y

	off := op.Offset(image.Pt(0, y)).Push(ops)
	paint.FillShape(ops, th.Chrome, clip.Rect{Max: image.Pt(width, d.H)}.Op())
	paint.FillShape(ops, th.Border, clip.Rect{Max: image.Pt(width, 1)}.Op())
	mid := d.H / 2
	ip, gap := d.ItemPad, d.Gap
	if l.Tight {
		ip, gap = d.ItemPad/2, d.Gap/2
	}
	for _, x := range l.Seps {
		h := gtx.Dp(14)
		paint.FillShape(ops, th.Border, clip.Rect(image.Rect(x, mid-h/2, x+max(gtx.Dp(1), 1), mid+h/2)).Op())
	}
	text := func(x int, s string, mono bool, col color.NRGBA) int {
		var call op.CallOp
		var sz image.Point
		if mono {
			call, sz = c.label(gtx, s, 11, col)
		} else {
			call, sz = c.sans(gtx, s, 11, col, font.SemiBold)
		}
		place(ops, call, image.Pt(x, mid-sz.Y/2))
		return x + sz.X
	}
	icon := func(x int, name string, col color.NRGBA) int {
		c.icon(gtx, name, image.Rect(x, mid-d.Icon/2, x+d.Icon, mid-d.Icon/2+d.Icon), col)
		return x + d.Icon
	}
	pill := func(r image.Rectangle, col color.NRGBA) image.Rectangle {
		h := gtx.Dp(18)
		box := image.Rect(r.Min.X+ip, mid-h/2, r.Max.X-ip, mid-h/2+h)
		paint.FillShape(ops, tint(col, 0x24), clip.UniformRRect(box, h/2).Op(ops))
		return box
	}
	for g, r := range l.Groups {
		if g == win.stat.hover && clickable(g) {
			hr := image.Rect(r.Min.X+gtx.Dp(2), mid-gtx.Dp(12), r.Max.X-gtx.Dp(2), mid+gtx.Dp(12))
			paint.FillShape(ops, th.Active, clip.UniformRRect(hr, gtx.Dp(7)).Op(ops))
		}
		x := r.Min.X + ip
		switch g {
		case grpFolder:
			x = icon(x, "folder", th.Muted) + gap
			text(x, l.FolderText, true, th.Folder)
		case grpBranch:
			x = icon(x, "branch", th.Branch) + gap
			x = text(x, l.BranchText, true, th.Branch)
			if !l.Compact.BranchCounts {
				if m.Add > 0 {
					x = text(x+gap, "+"+strconv.Itoa(m.Add), true, th.Done)
				}
				if m.Del > 0 {
					text(x+gap/2, "−"+strconv.Itoa(m.Del), true, th.UsageHigh)
				}
			}
		case grpUpdate:
			box := pill(r, th.Accent)
			x = icon(box.Min.X+d.PillPad, "up", th.Accent)
			if !l.Compact.Update {
				text(x+gap/2, "Update "+m.Update, false, th.Accent)
			}
		case grpAgents:
			col := th.Accent
			if m.Waiting > 0 {
				col = th.Needs
			}
			box := pill(r, col)
			dot := image.Rect(box.Min.X+d.PillPad, mid-d.Dot/2, box.Min.X+d.PillPad+d.Dot, mid-d.Dot/2+d.Dot)
			if m.Waiting > 0 {
				paint.FillShape(ops, col, clip.Ellipse(dot).Op(ops))
			} else {
				paint.FillShape(ops, col, runningRing(gtx, dot.Inset(-gtx.Dp(1))))
			}
			text(dot.Max.X+gap/2, agentsPill(m, l.Compact.Agents), false, col)
		case grpServers:
			dots := m.Servers
			if l.Compact.AggregateServers {
				agg := serverDot{Up: true}
				for _, s := range m.Servers {
					agg.Down = agg.Down || s.Down
					agg.Up = agg.Up && s.Up
				}
				dots = []serverDot{agg}
			}
			for _, s := range dots {
				col := th.Muted
				switch {
				case s.Down:
					col = th.UsageHigh
				case s.Up:
					col = th.Done
				}
				paint.FillShape(ops, col, clip.Ellipse(image.Rect(x, mid-d.Dot/2, x+d.Dot, mid-d.Dot/2+d.Dot)).Op(ops))
				x += d.Dot + d.Dot*2/3
			}
			if !l.Compact.ServersLabel {
				x = text(x-d.Dot*2/3+gap, serversCount(m.Servers), true, th.ActiveFG)
				c.statusLabel(gtx, x+gap, mid, "SERVERS")
			}
		case grpUsage:
			for i, k := range l.UsageKeys {
				u := usageByKey(m.Usage, k)
				if i > 0 {
					x += gap
				}
				if !l.Compact.UsageLabels {
					x = c.statusLabel(gtx, x, mid, strings.ToUpper(u.Key)) + gap/2
				}
				col := c.levelColor(u.Level)
				if !l.Compact.UsageMeters {
					x = c.meter(gtx, x, mid, d.Meter, u.Pct, col) + gap/2
				}
				pc := th.ActiveFG
				if u.Level != usage.Normal {
					pc = col
				}
				x = text(x, strconv.Itoa(u.Pct)+"%", true, pc)
			}
		case grpMem:
			x = icon(x, "chip", th.Muted) + gap
			text(x, strconv.Itoa(m.MemMB)+" MB", true, th.StatusFG)
		case grpTabs:
			x = icon(x, "tabs", th.Muted) + gap
			unit := " tabs"
			if m.Tabs == 1 {
				unit = " tab"
			}
			text(x, strconv.Itoa(m.Tabs)+unit, true, th.StatusFG)
		case grpNotice:
			text(x, l.NoticeText, true, th.StatusFG)
		}
	}
	off.Pop()

	if win.stat.tag == nil {
		win.stat.tag = new(int)
	}
	area := clip.Rect(image.Rect(0, y, width, y+d.H)).Push(ops)
	if clickable(win.stat.hover) {
		pointer.CursorPointer.Add(ops)
	}
	event.Op(ops, win.stat.tag)
	area.Pop()
}

// statusLabel draws a small uppercase label and returns its right edge.
func (c *chrome) statusLabel(gtx layout.Context, x, mid int, s string) int {
	call, sz := c.sans(gtx, s, 9.5, mix(c.th.Muted, c.th.Chrome, 0.25), font.Bold)
	place(gtx.Ops, call, image.Pt(x, mid-sz.Y/2))
	return x + sz.X
}

// meter draws a w px usage meter at pct and returns its right edge.
func (c *chrome) meter(gtx layout.Context, x, mid, w, pct int, col color.NRGBA) int {
	h := max(gtx.Dp(4), 2)
	track := image.Rect(x, mid-h/2, x+w, mid-h/2+h)
	paint.FillShape(gtx.Ops, mix(c.th.Border, c.th.Chrome, 0.2), clip.UniformRRect(track, h/2).Op(gtx.Ops))
	if fill := w * min(max(pct, 0), 100) / 100; fill > 0 {
		paint.FillShape(gtx.Ops, col, clip.UniformRRect(image.Rect(x, track.Min.Y, x+fill, track.Max.Y), h/2).Op(gtx.Ops))
	}
	return x + w
}

// levelColor is a usage level's colour: accent, amber or red.
func (c *chrome) levelColor(l usage.Level) color.NRGBA {
	switch l {
	case usage.Warn:
		return c.th.Needs
	case usage.High:
		return c.th.UsageHigh
	}
	return c.th.Accent
}

func usageByKey(ds []usage.Detail, k string) usage.Detail {
	for _, d := range ds {
		if d.Key == k {
			return d
		}
	}
	return usage.Detail{Key: k}
}

// statusEvents handles hover and clicks on the status bar (spec 6.2).
func (win *window) statusEvents(gtx layout.Context) {
	if win.stat.tag == nil {
		win.stat.tag = new(int) // before the first draw, so its filter is up when the first click comes
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: win.stat.tag, Kinds: pointer.Press | pointer.Move | pointer.Enter | pointer.Leave})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok && !win.bench.IgnoreInput() {
			win.statusPointer(e)
		}
	}
}

// statusPointer applies one pointer event in window px.
func (win *window) statusPointer(e pointer.Event) {
	p := e.Position.Round()
	at := grpNone
	for g := range win.stat.l.Groups {
		if p.In(win.statusRect(g)) {
			at = g
		}
	}
	switch e.Kind {
	case pointer.Leave:
		at = grpNone
	case pointer.Press:
		if e.Buttons.Contain(pointer.ButtonPrimary) {
			win.statusClick(at)
		}
	}
	if at != win.stat.hover {
		win.stat.hover = at
		if win.w != nil {
			win.w.Invalidate() // one frame for the hover fill or the card, never a loop
		}
	}
}

func (win *window) statusClick(g statusGroup) {
	switch g {
	case grpUpdate:
		win.updClick()
	case grpAgents:
		win.toggleBoard()
	case grpServers:
		win.showServer()
	case grpUsage:
		win.openSettings()
		for i, s := range win.sections() {
			if s.title == "Agents" {
				win.selectSection(i)
			}
		}
	}
}

// drawUsageCard is the card above the usage group while it is hovered (spec 6.2).
func (win *window) drawUsageCard(gtx layout.Context) {
	if win.stat.hover != grpUsage || len(win.stat.m.Usage) == 0 {
		return
	}
	c, ops, th := win.ui, gtx.Ops, win.ui.th
	r := win.statusRect(grpUsage)
	w := gtx.Dp(290)
	pad, lh := gtx.Dp(14), gtx.Dp(22)
	rows := len(win.stat.m.Usage)
	h := pad + gtx.Dp(22) + rows*(lh+gtx.Dp(14)) + gtx.Dp(30)
	x := min(max(r.Max.X-w, gtx.Dp(8)), gtx.Constraints.Max.X-w-gtx.Dp(8))
	box := image.Rect(x, r.Min.Y-gtx.Dp(8)-h, x+w, r.Min.Y-gtx.Dp(8))
	rad := gtx.Dp(11)
	paint.FillShape(ops, th.ToastBG, clip.UniformRRect(box, rad).Op(ops))
	rr := clip.UniformRRect(box, rad)
	paint.FillShape(ops, th.ToastLine, clip.Stroke{Path: rr.Path(ops), Width: float32(max(gtx.Dp(1), 1))}.Op())
	y := box.Min.Y + pad
	call, sz := c.sans(gtx, "Claude usage", 12.5, th.ToastFG, font.SemiBold)
	place(ops, call, image.Pt(box.Min.X+pad, y))
	y += sz.Y + gtx.Dp(8)
	for _, u := range win.stat.m.Usage {
		mid := y + lh/2
		call, sz := c.sans(gtx, u.Name, 11.5, th.Muted, font.Medium)
		place(ops, call, image.Pt(box.Min.X+pad, mid-sz.Y/2))
		col := c.levelColor(u.Level)
		mx := box.Min.X + pad + gtx.Dp(72)
		c.meter(gtx, mx, mid, box.Max.X-pad-gtx.Dp(48)-mx, u.Pct, col)
		pc := th.ToastFG
		if u.Level != usage.Normal {
			pc = col
		}
		pcall, psz := c.label(gtx, strconv.Itoa(u.Pct)+"%", 11, pc)
		place(ops, pcall, image.Pt(box.Max.X-pad-psz.X, mid-psz.Y/2))
		y += lh
		if u.Reset != "" {
			rc, rs := c.sans(gtx, u.Reset, 10.5, mix(th.Muted, th.ToastBG, 0.3), font.Normal)
			place(ops, rc, image.Pt(mx, y-gtx.Dp(4)))
			_ = rs
		}
		y += gtx.Dp(14)
	}
	paint.FillShape(ops, th.ToastLine, clip.Rect(image.Rect(box.Min.X+pad, y, box.Max.X-pad, y+1)).Op())
	fc, fs := c.sans(gtx, "From Claude Code's status line.", 10.5, th.Muted, font.Normal)
	place(ops, fc, image.Pt(box.Min.X+pad, y+(box.Max.Y-y-fs.Y)/2))
}
