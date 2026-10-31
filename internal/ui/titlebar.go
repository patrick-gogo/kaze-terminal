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
)

// barLayout is where things go in kaze's own top bar, in pixels. Without a custom
// title bar (macOS) it is empty: the toggle and gear sit in the sidebar instead.
type barLayout struct {
	Toggle  image.Rectangle    // the sidebar toggle at the left end
	Mark    image.Rectangle    // where the K mark is drawn; inside Drag, so it moves the window too
	Drag    image.Rectangle    // empty space that moves the window; left of Search when there is one
	Search  image.Rectangle    // the palette's search box column; empty when the bar is too narrow
	DragR   image.Rectangle    // empty space right of Search that moves the window too
	Scratch image.Rectangle    // the scratch shell button, left of Gear; may be empty
	Buttons [3]image.Rectangle // indexed capMinimize-1, capMaximize-1, capClose-1
	Gear    image.Rectangle    // the settings button, left of Buttons; may be empty
}

// barDims are the bar's fixed sizes in pixels.
type barDims struct {
	buttonW int // one caption button
	gearW   int // the settings button; 0 leaves it out
	toggleW int // the sidebar toggle; 0 leaves it out
	markW   int // the K mark's box, right of the toggle

	searchMin, searchMax int // the search box's width range; searchMax 0 leaves it out
	searchH              int // the search box's height
	scratchW             int // the scratch shell button; 0 leaves it out
}

// layoutBar splits a bar of the given size, left to right: toggle, mark, drag
// area, gear, caption buttons. Without custom buttons there is no bar.
func layoutBar(size image.Point, d barDims, custom bool) barLayout {
	var b barLayout
	if !custom {
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
	if l := gearL - max(d.scratchW, 0); d.scratchW > 0 && l >= 0 {
		b.Scratch = image.Rect(l, 0, gearL, size.Y)
		gearL = l
	}
	toggleR := min(max(d.toggleW, 0), gearL)
	if toggleR > 0 {
		b.Toggle = image.Rect(0, 0, toggleR, size.Y)
	}
	if toggleR < gearL {
		b.Drag = image.Rect(toggleR, 0, gearL, size.Y)
		b.Mark = image.Rect(toggleR, 0, min(toggleR+max(d.markW, 0), gearL), size.Y)
	}
	// The search box takes the middle of the space between the mark and the
	// gear, with room to drag on both sides; too little room leaves it out.
	room := gearL - b.Mark.Max.X
	if d.searchMax > 0 && !b.Mark.Empty() && room >= d.searchMin+d.searchMin/4 {
		w := min(max(room*2/3, d.searchMin), d.searchMax, room)
		x := b.Mark.Max.X + (room-w)/2
		b.Search = image.Rect(x, 0, x+w, size.Y)
		b.Drag.Max.X = x
		b.DragR = image.Rect(x+w, 0, gearL, size.Y)
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
	if p.In(b.Toggle) {
		return capToggle
	}
	if p.In(b.Search) {
		return capSearch
	}
	if p.In(b.Scratch) {
		return capScratch
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
