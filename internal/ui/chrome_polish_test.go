package ui

import (
	"image"
	"strings"
	"testing"

	"gioui.org/io/input"
)

// A toggle in the middle of a frame must not leave the sidebar's area unpainted
// (the white flash): the frame decided the sidebar's width before the toggle.
func TestSidebarToggleMidFrameStillDrawsIt(t *testing.T) {
	win, _ := sideWindow(t, false)
	openIn(t, win, "")
	var r input.Router
	sideFrame(t, win, &r)
	rect := image.Rect(0, 42, sidebarW, 560)
	win.toggleSidebar() // as a click in this frame's events would
	win.drawBar(barContext(1000), 42)
	win.drawSide(pageContext(1000, 600), rect)
	if len(win.side.hits.rows) == 0 {
		t.Fatal("the sidebar's rectangle was left unpainted this frame")
	}
}

func TestCrumbShortensLongCommands(t *testing.T) {
	for in, want := range map[string]string{
		"/agentkits-marketing:checklist:analytics-monthly": "/analytics-monthly",
		"/ship-ticket":                "/ship-ticket",
		"/" + strings.Repeat("x", 40): "/" + strings.Repeat("x", 22) + "…",
		"":                            "",
	} {
		if got := crumbCommand(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
