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

func TestButtonsAreRightAlignedInOrder(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
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

func TestDragFillsSpaceBetweenTabsAndButtons(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
	if b.Drag != image.Rect(300, 0, 862, 38) {
		t.Fatalf("drag %v", b.Drag)
	}
}

func TestRegionsNeverOverlapButtons(t *testing.T) {
	b := layoutBar(image.Pt(400, 38), 900, barDims{buttonW: 46}, true) // tabs want more than fits
	if b.Tabs.Max.X > b.Buttons[0].Min.X {
		t.Fatalf("tabs %v overlap min %v", b.Tabs, b.Buttons[0])
	}
	if !b.Drag.Empty() {
		t.Fatalf("no room for drag, got %v", b.Drag)
	}
}

func TestRegionsScaleWithDp(t *testing.T) {
	b := layoutBar(image.Pt(1500, 57), 300, barDims{buttonW: 69}, true) // 150%
	if b.Buttons[2].Dx() != 69 || b.Buttons[2].Dy() != 57 {
		t.Fatalf("%v", b.Buttons[2])
	}
}

func TestDecoratedHasNoButtonsOrDrag(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, false)
	if !b.Buttons[0].Empty() || !b.Drag.Empty() || !b.Gear.Empty() || b.Tabs.Max.X != 1000 {
		t.Fatalf("%+v", b)
	}
}

func TestTabsAreNotMoveArea(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
	if b.Drag.Overlaps(b.Tabs) {
		t.Fatalf("drag %v overlaps tabs %v", b.Drag, b.Tabs)
	}
}

func TestClickFiresOnReleaseInside(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
	var s captionState
	p := b.Buttons[0].Min.Add(image.Pt(5, 5))
	if s.pointer(pointer.Press, p, b) != capNone {
		t.Fatal("fired on press")
	}
	if s.pointer(pointer.Release, p, b) != capMinimize {
		t.Fatal("no fire on release")
	}
}

func TestReleaseOutsideButtonDoesNothing(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
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
}

func TestNarrowBarKeepsButtonsInside(t *testing.T) {
	bar := image.Rect(0, 0, 100, 38) // narrower than three 46px buttons
	b := layoutBar(bar.Max, 300, barDims{buttonW: 46}, true)
	for i, r := range b.Buttons {
		if !r.In(bar) {
			t.Fatalf("button %d %v outside bar %v", i, r, bar)
		}
	}
	if b.Tabs.Max.X > b.Buttons[0].Min.X && !b.Buttons[0].Empty() {
		t.Fatalf("tabs %v overlap %v", b.Tabs, b.Buttons[0])
	}
}

func TestNegativeTabsWidthIsZero(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), -50, barDims{buttonW: 46}, true)
	if b.Tabs != image.Rect(0, 0, 0, 38) || b.Drag != image.Rect(0, 0, 862, 38) {
		t.Fatalf("tabs %v drag %v", b.Tabs, b.Drag)
	}
}

func TestPressOnOneButtonReleaseOnAnotherFiresNothing(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
	var s captionState
	s.pointer(pointer.Press, b.Buttons[0].Min.Add(image.Pt(5, 5)), b)
	if got := s.pointer(pointer.Release, b.Buttons[2].Min.Add(image.Pt(5, 5)), b); got != capNone {
		t.Fatalf("fired %v", got)
	}
}

func TestCancelClearsState(t *testing.T) {
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
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
	b := layoutBar(image.Pt(1000, 38), 300, barDims{buttonW: 46}, true)
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
	win.drawBar(barContext(width), win.mgr.Tabs(), win.mgr.Active().ID, 38)
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

func TestTabStripStopsBeforeButtons(t *testing.T) {
	for _, width := range []int{120, 300, 600} {
		win, _ := captionWindow(t, width)
		for range 12 {
			if _, err := win.mgr.New(); err != nil {
				t.Fatal(err)
			}
		}
		win.drawBar(barContext(width), win.mgr.Tabs(), win.mgr.Active().ID, 38)
		limit := win.bar.Buttons[0].Min.X
		for _, h := range win.ui.hits.tabs {
			if h.rect.Max.X > limit || h.close.Max.X > limit {
				t.Fatalf("width %d: tab %v / close %v past buttons at %d", width, h.rect, h.close, limit)
			}
		}
		if win.ui.hits.plus.Max.X > limit {
			t.Fatalf("width %d: + %v past buttons at %d", width, win.ui.hits.plus, limit)
		}
		if win.bar.Drag.Overlaps(win.ui.hits.plus) {
			t.Fatalf("width %d: drag %v covers + %v", width, win.bar.Drag, win.ui.hits.plus)
		}
	}
}

func TestManyTabsKeepAMinimumDragZone(t *testing.T) {
	// Tab and + widths depend on the font, so sweep widths and counts until the
	// strip lands anywhere near the buttons.
	for width := 1000; width <= 1200; width += 9 {
		win, _ := captionWindow(t, width)
		win.mgr.Active().Title = "claude"
		for range 30 {
			tab, err := win.mgr.New()
			if err != nil {
				t.Fatal(err)
			}
			tab.Title = "claude"
			gtx := barContext(width)
			win.drawBar(gtx, win.mgr.Tabs(), win.mgr.Active().ID, 38)
			if minDrag := gtx.Dp(48); win.bar.Drag.Dx() < minDrag {
				t.Fatalf("width %d, %d tabs: drag %v is narrower than %dpx", width, len(win.mgr.Tabs()), win.bar.Drag, minDrag)
			}
			limit := win.bar.Drag.Min.X
			for _, h := range win.ui.hits.tabs {
				if h.rect.Max.X > limit {
					t.Fatalf("tab %v runs into the drag zone at %d", h.rect, limit)
				}
			}
		}
	}
}

func TestBandAboveTabsMovesTheWindow(t *testing.T) {
	win, _ := captionWindow(t, 1000)
	gtx := barContext(1000)
	win.drawBar(gtx, win.mgr.Tabs(), win.mgr.Active().ID, 38)
	if win.bar.Band.Empty() {
		t.Fatal("no move band above the tabs")
	}
	first := win.ui.hits.tabs[0].rect
	if above := image.Pt(first.Min.X+5, 1); !above.In(win.bar.Band) {
		t.Fatalf("%v above the first tab is not in band %v", above, win.bar.Band)
	}
	for _, h := range win.ui.hits.tabs {
		if win.bar.Band.Overlaps(h.rect) || win.bar.Band.Overlaps(h.close) {
			t.Fatalf("band %v covers tab %v / close %v", win.bar.Band, h.rect, h.close)
		}
	}
	if win.bar.Band.Overlaps(win.ui.hits.plus) || win.bar.Band.Overlaps(win.bar.Tabs) {
		t.Fatalf("band %v covers + %v or tabs %v", win.bar.Band, win.ui.hits.plus, win.bar.Tabs)
	}
	for i, r := range win.bar.Buttons {
		if win.bar.Band.Overlaps(r) {
			t.Fatalf("band %v covers button %d %v", win.bar.Band, i, r)
		}
	}
}

func TestLosingFocusClearsCaptionHover(t *testing.T) {
	win, _ := captionWindow(t, 1000)
	win.focused = true
	r := win.bar.Buttons[capClose-1]
	win.captionEvent(pointer.Event{Kind: pointer.Move, Position: f32.Pt(float32(r.Min.X+5), float32(r.Min.Y+5))})
	if win.caption.hover != capClose {
		t.Fatalf("hover %v before blur", win.caption.hover)
	}
	win.setFocused(false)
	if win.caption.hover != capNone {
		t.Fatalf("hover %v after the window lost focus", win.caption.hover)
	}
}
