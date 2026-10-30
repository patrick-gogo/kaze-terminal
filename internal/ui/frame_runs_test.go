package ui

import "testing"

// The sidebar and the status bar draw the same frame's runs, built once.
func TestStatusBarUsesTheFramesRuns(t *testing.T) {
	win := testWindow(t, &recStarter{})
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	runs := []sideRun{{Run: 1, Name: "api", Members: []sideMember{{Status: "Running"}, {Down: true}}}}
	win.frameRuns = &runs
	if n := len(win.statusParts().Servers); n != 2 {
		t.Fatalf("status bar shows %d server dots, want the frame's 2", n)
	}
	win.frameRuns = nil
	if n := len(win.statusParts().Servers); n != 0 {
		t.Fatalf("outside a frame the status bar shows %d dots, want the live 0", n)
	}
}
