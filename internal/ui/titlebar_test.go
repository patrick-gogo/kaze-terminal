package ui

import (
	"image"
	"testing"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

var testDims = barDims{buttonW: 46, gearW: 46, toggleW: 46, markW: 30}

func TestButtonsAreRightAlignedInOrder(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), barDims{buttonW: 46}, true)
	if b.Buttons[2] != image.Rect(954, 0, 1000, 38) {
		t.Fatalf("close %v", b.Buttons[2])
	}
	if b.Buttons[1] != image.Rect(908, 0, 954, 38) {
		t.Fatalf("max %v", b.Buttons[1])
	}
	if b.Buttons[0] != image.Rect(862, 0, 908, 38) {
		t.Fatalf("min %v", b.Buttons[0])
	}
}

// Left to right: toggle, mark, drag area, gear, caption buttons.
func TestBarOrderToggleMarkDragGearButtons(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), testDims, true)
	if b.Toggle != image.Rect(0, 0, 46, 38) {
		t.Fatalf("toggle %v", b.Toggle)
	}
	if b.Mark != image.Rect(46, 0, 76, 38) || !b.Mark.In(b.Drag) {
		t.Fatalf("mark %v, drag %v", b.Mark, b.Drag)
	}
	if b.Drag != image.Rect(46, 0, 816, 38) {
		t.Fatalf("drag %v, want from the toggle to the gear", b.Drag)
	}
	if b.Gear != image.Rect(816, 0, 862, 38) {
		t.Fatalf("gear %v, want just left of minimize", b.Gear)
	}
}

func TestRegionsNeverOverlap(t *testing.T) {
	for _, w := range []int{60, 120, 200, 400, 1000} {
		b := layoutBar(image.Pt(w, 38), testDims, true)
		rs := []image.Rectangle{b.Toggle, b.Drag, b.Gear, b.Buttons[0], b.Buttons[1], b.Buttons[2]}
		bar := image.Rect(0, 0, w, 38)
		for i, r := range rs {
			if !r.Empty() && !r.In(bar) {
				t.Fatalf("width %d: region %d %v outside the bar", w, i, r)
			}
			for j := i + 1; j < len(rs); j++ {
				if r.Overlaps(rs[j]) {
					t.Fatalf("width %d: region %d %v overlaps %d %v", w, i, r, j, rs[j])
				}
			}
		}
	}
}

func TestRegionsScaleWithDp(t *testing.T) {
	b := layoutBar(image.Pt(1500, 57), barDims{buttonW: 69, toggleW: 69}, true) // 150%
	if b.Buttons[2].Dx() != 69 || b.Buttons[2].Dy() != 57 || b.Toggle.Dx() != 69 {
		t.Fatalf("%v %v", b.Buttons[2], b.Toggle)
	}
}

func TestDecoratedHasNoBar(t *testing.T) {
	if b := layoutBar(image.Pt(1000, 38), testDims, false); b != (barLayout{}) {
		t.Fatalf("%+v", b)
	}
}

func TestClickFiresOnReleaseInside(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), testDims, true)
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
	b := layoutBar(image.Pt(1000, 38), testDims, true)
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

func TestNarrowBarKeepsButtonsInside(t *testing.T) {
	bar := image.Rect(0, 0, 100, 38) // narrower than three 46px buttons
	b := layoutBar(bar.Max, testDims, true)
	for i, r := range b.Buttons {
		if !r.In(bar) {
			t.Fatalf("button %d %v outside bar %v", i, r, bar)
		}
	}
	if !b.Toggle.Empty() && b.Toggle.Overlaps(b.Buttons[0]) {
		t.Fatalf("toggle %v overlaps %v", b.Toggle, b.Buttons[0])
	}
}

func TestPressOnOneButtonReleaseOnAnotherFiresNothing(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), testDims, true)
	var s captionState
	s.pointer(pointer.Press, b.Buttons[0].Min.Add(image.Pt(5, 5)), b)
	if got := s.pointer(pointer.Release, b.Buttons[2].Min.Add(image.Pt(5, 5)), b); got != capNone {
		t.Fatalf("fired %v", got)
	}
}

func TestCancelClearsState(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), testDims, true)
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
	b := layoutBar(image.Pt(1000, 38), testDims, true)
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
	win.drawBar(barContext(width), 38)
	return win, &got
}

func barContext(width int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(width, 38)),
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
