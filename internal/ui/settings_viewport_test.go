package ui

import (
	"gioui.org/f32"
	"gioui.org/io/pointer"
	"image"
	"testing"
)

func TestSettingsScrollReachesClippedClaudeSetup(t *testing.T) {
	f := newClaudeFixture(t, "", true)
	win := claudeWindow(t, f, 700, 350)
	draw := func() { win.drawSettings(pageContext(700, 350), image.Rect(0, 38, 700, 326)) }
	find := func(id string) image.Rectangle {
		t.Helper()
		for _, c := range win.set.controls {
			if c.id == id {
				return c.rect
			}
		}
		t.Fatalf("missing %s", id)
		return image.Rectangle{}
	}
	if find("show_usage").Max.Y <= 326 {
		t.Fatal("fixture should need scrolling")
	}
	win.settingsPointer(pointer.Event{Kind: pointer.Scroll, Scroll: f32.Pt(0, 1000)})
	draw()
	if r := find("show_usage"); r.Min.Y < 38 || r.Max.Y > 326 {
		t.Fatalf("last control still clipped after scrolling: %v", r)
	}
	win.settingsPointer(pointer.Event{Kind: pointer.Scroll, Scroll: f32.Pt(0, -1000)})
	draw()
	if r := find("claude_fix"); r.Min.Y < 38 || r.Max.Y > 326 {
		t.Fatalf("setup control not reachable after scrolling back: %v", r)
	}
}

func TestSettingsRevealsKeyboardFocusedControl(t *testing.T) {
	f := newClaudeFixture(t, "", true)
	win := claudeWindow(t, f, 700, 350)
	for i, c := range win.set.controls {
		if c.id == "claude_fix" {
			win.set.focus = i
		}
	}
	win.drawSettings(pageContext(700, 350), image.Rect(0, 38, 700, 326))
	r := win.set.controls[win.set.focus].rect
	if r.Min.Y < win.set.pane.Min.Y || r.Max.Y > win.set.pane.Max.Y {
		t.Fatalf("keyboard-focused control clipped: %v in %v", r, win.set.pane)
	}
}

func TestSettingsStacksWideControlsBelowTheirLabel(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.set.sections = []settingsSection{{title: "Test", rows: func(*window) []settingRow {
		return []settingRow{{label: "Line spacing", ctrls: []ctrl{{kind: ctlSegmented, id: "spacing", options: []string{"Tight", "Normal", "Roomy"}}}}}
	}}}
	win.openSettings()
	gtx := pageContext(520, 350)
	win.drawSettings(gtx, image.Rect(0, 38, 520, 326))
	_, title := win.ui.label(gtx, "Test", 16, win.ui.pageColors().title)
	_, label := win.ui.label(gtx, "Line spacing", 13, win.ui.pageColors().fg)
	labelBottom := win.set.pane.Min.Y + title.Y + gtx.Dp(4) + gtx.Dp(16) + gtx.Dp(11) + label.Y
	if got := win.set.controls[0].rect.Min.Y; got < labelBottom {
		t.Fatalf("control y=%d overlaps label ending y=%d", got, labelBottom)
	}
}
