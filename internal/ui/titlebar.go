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
	capGear // opens settings; sits left of minimize, outside Buttons
)

// barLayout is where things go in the top bar, in pixels.
type barLayout struct {
	Tabs    image.Rectangle    // where the tab strip may draw (ends before Drag)
	Drag    image.Rectangle    // empty space that moves the window; may be empty
	Band    image.Rectangle    // the strip above the tabs, also a move area; may be empty
	Buttons [3]image.Rectangle // indexed capMinimize-1, capMaximize-1, capClose-1
	Gear    image.Rectangle    // the settings button, left of Buttons; may be empty
}

// barDims are the bar's fixed sizes in pixels.
type barDims struct {
	buttonW int // one caption button
	gearW   int // the settings button; 0 leaves it out
	minDrag int // move area always kept between the tabs and the buttons
	band    int // height of the move strip above the tabs (the tabs' top margin)
}

// layoutBar splits a bar of the given size. Without custom buttons (macOS,
// decorated) the tabs take the whole bar and there is no drag area.
func layoutBar(size image.Point, tabsWidth int, d barDims, custom bool) barLayout {
	var b barLayout
	if !custom {
		b.Tabs = image.Rect(0, 0, size.X, size.Y)
		return b
	}
	bar := image.Rect(0, 0, size.X, size.Y)
	left := max(size.X-3*d.buttonW, 0)
	for i := range b.Buttons {
		x := left + i*d.buttonW
		// A bar narrower than three buttons cuts them off rather than overhang it.
		b.Buttons[i] = image.Rect(x, 0, x+d.buttonW, size.Y).Intersect(bar)
	}
	gearL := max(left-max(d.gearW, 0), 0)
	if gearL < left {
		b.Gear = image.Rect(gearL, 0, left, size.Y).Intersect(bar)
	}
	// Many tabs must still leave something to grab the window by.
	tabsEnd := min(max(tabsWidth, 0), max(gearL-d.minDrag, 0))
	band := min(max(d.band, 0), size.Y)
	b.Tabs = image.Rect(0, band, tabsEnd, size.Y)
	if band > 0 && tabsEnd > 0 {
		b.Band = image.Rect(0, 0, tabsEnd, band)
	}
	if tabsEnd < gearL {
		b.Drag = image.Rect(tabsEnd, 0, gearL, size.Y)
	}
	return b
}

// captionAt returns which caption button contains p.
func (b barLayout) captionAt(p image.Point) captionButton {
	for i, r := range b.Buttons {
		if p.In(r) {
			return captionButton(i + 1)
		}
	}
	if p.In(b.Gear) {
		return capGear
	}
	return capNone
}

// captionState tracks hover/press for the caption buttons across pointer events.
type captionState struct{ hover, pressed captionButton }

// pointer feeds one pointer event and returns the button to fire now. A click
// fires on release inside the button that was pressed.
func (s *captionState) pointer(kind pointer.Kind, p image.Point, b barLayout) captionButton {
	switch kind {
	case pointer.Press:
		s.hover = b.captionAt(p)
		s.pressed = s.hover
	case pointer.Release:
		fired := capNone
		if s.pressed != capNone && b.captionAt(p) == s.pressed {
			fired = s.pressed
		}
		s.pressed = capNone
		s.hover = b.captionAt(p)
		return fired
	case pointer.Move, pointer.Enter, pointer.Drag:
		s.hover = b.captionAt(p)
	case pointer.Leave:
		s.hover = capNone
	case pointer.Cancel:
		s.hover, s.pressed = capNone, capNone
	}
	return capNone
}

// actionFor maps a fired button to the window action, given the real maximized state.
// It returns 0 (no action) for capNone and capGear, which the window handles itself.
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
