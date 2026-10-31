package ui

import (
	"image"

	"gioui.org/io/pointer"
	"gioui.org/io/system"
)

// CustomTitleBar reports whether kaze draws its own title bar here; main then
// opens the window undecorated.
const CustomTitleBar = customTitleBar

type captionButton int

const (
	capNone captionButton = iota
	capMinimize
	capMaximize // shows "restore" when maximized
	capClose
	capGear    // opens settings; sits left of minimize, outside Buttons
	capToggle  // shows or hides the sidebar; at the bar's left end
	capSearch  // opens the palette; centred between the mark and the gear
	capScratch // shows or hides the session's scratch shell; left of the gear
	capPlus    // opens a new session; at the lead strip's right end
	capAgents  // opens or closes the agents board; left of the gear
)

// captionState tracks hover/press for the caption buttons across pointer events.
type captionState struct{ hover, pressed captionButton }

// pointer feeds one pointer event and returns the button to fire now. A click
// fires on release inside the button that was pressed.
func (s *captionState) pointer(kind pointer.Kind, p image.Point, b titleLayout) captionButton {
	switch kind {
	case pointer.Press:
		s.hover = b.at(p)
		s.pressed = s.hover
	case pointer.Release:
		fired := capNone
		if s.pressed != capNone && b.at(p) == s.pressed {
			fired = s.pressed
		}
		s.pressed = capNone
		s.hover = b.at(p)
		return fired
	case pointer.Move, pointer.Enter, pointer.Drag:
		s.hover = b.at(p)
	case pointer.Leave:
		s.hover = capNone
	case pointer.Cancel:
		s.hover, s.pressed = capNone, capNone
	}
	return capNone
}

// actionFor maps a fired button to the window action, given the real maximized state.
// It returns 0 (no action) for capNone, capGear, capToggle and capSearch, which the window handles itself.
func actionFor(c captionButton, maximized bool) system.Action {
	switch c {
	case capMinimize:
		return system.ActionMinimize
	case capMaximize:
		if maximized {
			return system.ActionUnmaximize
		}
		return system.ActionMaximize
	case capClose:
		return system.ActionClose
	}
	return 0
}

// titleDims are the shared title bar's fixed sizes, in px.
type titleDims struct {
	BarH, Btn, BtnH, CapW, Mark int // bar height, icon button width and height, caption button width, K mark box
	Pad, LeadPad, Gap           int // outer padding, padding inside the lead strip, gap between parts
	SearchMin, SearchMax        int
	SearchH, KeyPad             int // search field height; padding around the key cap text
}

// crumbParts are the session crumb's texts. State is "", "running", "needs" or "done".
type crumbParts struct{ Repo, Title, Cmd, State string }

// titleIn is what the shared title bar is laid out from (spec 5.1).
type titleIn struct {
	Size    image.Point // the bar, px
	Mac     bool
	LightsW int // px the native traffic lights take from the left; 0 off Mac or in full screen
	SideW   int // the sidebar's width; 0 while hidden
	Crumb   crumbParts
	Key     string // the palette key cap text
	Dims    titleDims
	Measure func(s string, mono bool) int
}

// titleLayout is where the shared title bar's parts go, in px.
type titleLayout struct {
	Lead               image.Rectangle // the strip over the sidebar
	Mark, Toggle, Plus image.Rectangle // Mark on Windows only; Plus only while the sidebar shows
	Crumb              image.Rectangle
	CrumbShow          crumbParts // the parts that fit (spec 5.3); Title may still need shortening to Crumb
	Search             image.Rectangle
	SearchIconOnly     bool // the compact search: an icon, and the key cap when SearchKey
	SearchKey          bool
	Scratch, Agents    image.Rectangle
	Gear               image.Rectangle
	Buttons            [3]image.Rectangle // Windows only: minimize, maximize, close
	Drag               []image.Rectangle  // empty bar space, the crumb included
}

// statePill is the crumb's state word.
func statePill(state string) string {
	switch state {
	case "running":
		return "running"
	case "needs":
		return "needs you"
	case "done":
		return "done"
	}
	return ""
}

// crumbWidth is the width the crumb needs to show c in full.
func crumbWidth(c crumbParts, d titleDims, m func(string, bool) int) int {
	w := 0
	add := func(px int) {
		if w > 0 {
			w += d.Gap
		}
		w += px
	}
	if c.State != "" {
		add(10) // the state mark
	}
	if c.Repo != "" {
		add(m(c.Repo, false) + d.Gap + m("/", false))
	}
	if c.Title != "" {
		add(m(c.Title, false))
	}
	if c.Cmd != "" {
		add(m(c.Cmd, true))
	}
	if p := statePill(c.State); p != "" {
		add(m(p, false) + 2*d.KeyPad)
	}
	return w
}

// layoutTitle lays out the shared title bar (spec 5.1 and 5.3). Buttons are
// placed first and never dropped; the search field, then the crumb, give way.
func layoutTitle(in titleIn) titleLayout {
	var l titleLayout
	d, h := in.Dims, in.Size.Y
	btn := func(x int) image.Rectangle {
		y := (h - d.BtnH) / 2
		return image.Rect(x, y, x+d.Btn, y+d.BtnH)
	}

	// Left: the lights or the mark, the toggle, and the lead strip.
	x := d.Pad
	if in.Mac && in.LightsW > 0 {
		x = in.LightsW + d.Gap
	}
	if !in.Mac {
		y := (h - d.Mark) / 2
		l.Mark = image.Rect(d.Pad, y, d.Pad+d.Mark, y+d.Mark)
		x = l.Mark.Max.X + d.Gap/2
	}
	l.Toggle = btn(x)
	if in.SideW > 0 {
		right := max(in.SideW, l.Toggle.Max.X+d.Gap+d.Btn+d.LeadPad)
		l.Lead = image.Rect(0, 0, right, h)
		l.Plus = btn(right - d.LeadPad - d.Btn)
	} else {
		l.Lead = image.Rect(0, 0, l.Toggle.Max.X+d.LeadPad, h)
	}

	// Right: the window buttons, then gear, agents and scratch, right to left.
	r := in.Size.X
	if !in.Mac {
		for i := range l.Buttons {
			bx := in.Size.X - (3-i)*d.CapW
			l.Buttons[i] = image.Rect(bx, 0, bx+d.CapW, h)
		}
		r = l.Buttons[0].Min.X
	}
	r -= d.LeadPad
	l.Gear = btn(r - d.Btn)
	l.Agents = btn(l.Gear.Min.X - 2 - d.Btn)
	l.Scratch = btn(l.Agents.Min.X - 2 - d.Btn)

	// The middle: the crumb and the search field share what is left.
	lo, hi := l.Lead.Max.X+d.Pad, l.Scratch.Min.X-d.Gap
	avail := max(hi-lo, 0)
	keyW := in.Measure(in.Key, true) + 2*d.KeyPad
	iconKey := d.Btn + keyW
	c := in.Crumb
	sw := 0
	switch cw := crumbWidth(c, d, in.Measure); {
	case avail-cw-d.Gap >= d.SearchMin:
		sw = min(avail-cw-d.Gap, d.SearchMax)
	default:
		l.SearchIconOnly, l.SearchKey, sw = true, true, iconKey
		if avail-sw-d.Gap < 0 {
			l.SearchKey, sw = false, d.Btn // even the key cap has no room
		}
		for _, drop := range []func(*crumbParts){func(p *crumbParts) { p.Cmd = "" }, func(p *crumbParts) { p.Repo = "" }} {
			if crumbWidth(c, d, in.Measure) <= avail-sw-d.Gap {
				break
			}
			drop(&c)
		}
	}
	l.CrumbShow = c
	cw := min(crumbWidth(c, d, in.Measure), max(avail-sw-d.Gap, 0))
	l.Crumb = image.Rect(lo, 0, lo+cw, h)
	// Centre the search in the space between the crumb and the right buttons.
	left := l.Crumb.Max.X + d.Gap
	if cw == 0 {
		left = lo
	}
	sx := left + max(hi-left-sw, 0)/2
	sy := (h - d.SearchH) / 2
	if sx+sw <= hi+d.Gap { // never over the right buttons; the palette key still works
		l.Search = image.Rect(sx, sy, sx+sw, sy+d.SearchH)
	} else {
		l.SearchIconOnly, l.SearchKey = true, false
	}

	// Drag: every full-height gap between the controls, minus the traffic lights.
	cols := []image.Rectangle{l.Toggle, l.Plus, l.Search, l.Scratch, l.Agents, l.Gear, l.Buttons[0], l.Buttons[1], l.Buttons[2]}
	from := 0
	if in.Mac {
		from = in.LightsW
	}
	for gx := from; gx < in.Size.X; {
		next, end := in.Size.X, in.Size.X
		for _, b := range cols {
			if !b.Empty() && b.Max.X > gx && b.Min.X < next {
				next, end = max(b.Min.X, gx), b.Max.X
			}
		}
		if next > gx {
			l.Drag = append(l.Drag, image.Rect(gx, 0, next, h))
		}
		if next == in.Size.X {
			break
		}
		gx = end
	}
	return l
}

// at is the title bar button under p, capNone for drag space.
func (l titleLayout) at(p image.Point) captionButton {
	for _, b := range []struct {
		c captionButton
		r image.Rectangle
	}{
		{capToggle, l.Toggle}, {capPlus, l.Plus}, {capSearch, l.Search}, {capScratch, l.Scratch},
		{capAgents, l.Agents}, {capGear, l.Gear},
		{capMinimize, l.Buttons[0]}, {capMaximize, l.Buttons[1]}, {capClose, l.Buttons[2]},
	} {
		if p.In(b.r) {
			return b.c
		}
	}
	return capNone
}
