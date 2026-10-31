package ui

import (
	"image"
	"testing"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)







func TestClickFiresOnReleaseInside(t *testing.T) {
	b := titleAt(1000, false, 248)
	var s captionState
	p := b.Buttons[0].Min.Add(image.Pt(5, 5))
	if s.pointer(pointer.Press, p, b) != capNone {
		t.Fatal("fired on press")
	}
	if s.pointer(pointer.Release, p, b) != capMinimize {
		t.Fatal("no fire on release")
	}
	p = b.Toggle.Min.Add(image.Pt(5, 5))
	s.pointer(pointer.Press, p, b)
	if s.pointer(pointer.Release, p, b) != capToggle {
		t.Fatal("the toggle did not fire on release")
	}
}

func TestReleaseOutsideButtonDoesNothing(t *testing.T) {
	b := titleAt(1000, false, 248)
	var s captionState
	s.pointer(pointer.Press, b.Buttons[2].Min.Add(image.Pt(5, 5)), b)
	if s.pointer(pointer.Release, image.Pt(500, 20), b) != capNone {
		t.Fatal("fired outside")
	}
}

func TestMaximizeTogglesWithRealState(t *testing.T) {
	if actionFor(capMaximize, false) != system.ActionMaximize {
		t.Fatal("max")
	}
	if actionFor(capMaximize, true) != system.ActionUnmaximize {
		t.Fatal("restore")
	}
	if actionFor(capMinimize, true) != system.ActionMinimize || actionFor(capClose, false) != system.ActionClose {
		t.Fatal("min/close")
	}
	if actionFor(capToggle, false) != 0 {
		t.Fatal("the toggle maps to a window action")
	}
}


func TestPressOnOneButtonReleaseOnAnotherFiresNothing(t *testing.T) {
	b := titleAt(1000, false, 248)
	var s captionState
	s.pointer(pointer.Press, b.Buttons[0].Min.Add(image.Pt(5, 5)), b)
	if got := s.pointer(pointer.Release, b.Buttons[2].Min.Add(image.Pt(5, 5)), b); got != capNone {
		t.Fatalf("fired %v", got)
	}
}

func TestCancelClearsState(t *testing.T) {
	b := titleAt(1000, false, 248)
	var s captionState
	p := b.Buttons[2].Min.Add(image.Pt(5, 5))
	s.pointer(pointer.Press, p, b)
	s.pointer(pointer.Cancel, p, b)
	if s.hover != capNone || s.pressed != capNone {
		t.Fatalf("%+v", s)
	}
	if s.pointer(pointer.Release, p, b) != capNone {
		t.Fatal("fired after cancel")
	}
}

func TestLeaveClearsHover(t *testing.T) {
	b := titleAt(1000, false, 248)
	var s captionState
	s.pointer(pointer.Move, b.Buttons[1].Min.Add(image.Pt(5, 5)), b)
	if s.hover != capMaximize {
		t.Fatalf("hover %v", s.hover)
	}
	s.pointer(pointer.Leave, image.Pt(-1, -1), b)
	if s.hover != capNone {
		t.Fatalf("hover %v after leave", s.hover)
	}
}

// captionWindow is a test window with a custom title bar laid out at the given width,
// whose window actions are recorded instead of performed.
func captionWindow(t *testing.T, width int) (*window, *[]system.Action) {
	t.Helper()
	win := testWindow(t, &recStarter{})
	win.custom = true
	win.ui = newChrome()
	var got []system.Action
	win.performFn = func(a system.Action) { got = append(got, a) }
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	win.drawBar(barContext(width), 42)
	return win, &got
}

func barContext(width int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, 42)),
	}
}

func click(win *window, r image.Rectangle) {
	p := f32.Pt(float32(r.Min.X+5), float32(r.Min.Y+5))
	win.captionEvent(pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: p})
	win.captionEvent(pointer.Event{Kind: pointer.Release, Position: p})
}

func TestCaptionClickPerformsAction(t *testing.T) {
	win, got := captionWindow(t, 1000)
	click(win, win.bar.Buttons[capClose-1])
	if len(*got) != 1 || (*got)[0] != system.ActionClose {
		t.Fatalf("performed %v, want one ActionClose", *got)
	}
}

func TestMaximizeButtonUsesConfigState(t *testing.T) {
	win, got := captionWindow(t, 1000)
	win.onConfig(app.ConfigEvent{Config: app.Config{Mode: app.Maximized}})
	click(win, win.bar.Buttons[capMaximize-1])
	win.onConfig(app.ConfigEvent{Config: app.Config{Mode: app.Windowed}})
	click(win, win.bar.Buttons[capMaximize-1])
	if len(*got) != 2 || (*got)[0] != system.ActionUnmaximize || (*got)[1] != system.ActionMaximize {
		t.Fatalf("performed %v, want Unmaximize then Maximize", *got)
	}
}

func TestLosingFocusClearsCaptionHover(t *testing.T) {
	win, _ := captionWindow(t, 1000)
	win.focused = true
	win.side.st.hover = 7
	r := win.bar.Buttons[capClose-1]
	win.captionEvent(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(r.Min.X+5), float32(r.Min.Y+5))})
	if win.caption.hover != capClose {
		t.Fatalf("hover %v before blur", win.caption.hover)
	}
	win.setFocused(false)
	if win.caption.hover != capNone || win.side.st.hover != 0 {
		t.Fatalf("caption hover %v, sidebar hover %d after the window lost focus", win.caption.hover, win.side.st.hover)
	}
}





func TestClickingSearchOpensThePalette(t *testing.T) {
	win, _ := sideWindow(t, true)
	openIn(t, win, "")
	var r input.Router
	sizedFrame(t, win, &r, 1200, 600)
	if win.bar.Search.Empty() {
		t.Fatal("no search box in a 1200px bar")
	}
	c := centre(win.bar.Search)
	p := f32.Pt(float32(c.X), float32(c.Y))
	win.captionEvent(pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: p})
	if win.palette.open {
		t.Fatal("opened on press; a click fires on release")
	}
	win.captionEvent(pointer.Event{Kind: pointer.Release, Position: p})
	if !win.palette.open {
		t.Fatal("clicking the search box did not open the palette")
	}
}
