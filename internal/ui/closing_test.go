package ui

import (
	"testing"
	"time"
)

// On macOS Gio reports a closed window as ClosingEvent and only sends
// DestroyEvent once AppKit frees the view, which can be never. kaze must shut
// its shells down on the close itself, or a closed window leaves them running.
func TestClosingTheWindowShutsTheShellsDown(t *testing.T) {
	for _, exit := range []bool{true, false} {
		st := &recStarter{}
		win := testWindow(t, st)
		if _, err := win.mgr.New(); err != nil {
			t.Fatal(err)
		}
		was := exitOnClosing
		exitOnClosing = exit
		done := win.closing()
		exitOnClosing = was
		if done != exit {
			t.Fatalf("exitOnClosing %v: closing() = %v", exit, done)
		}
		select {
		case <-st.ptys[0].done:
			if !exit {
				t.Fatal("closing() closed the shell where DestroyEvent does that")
			}
		case <-time.After(100 * time.Millisecond):
			if exit {
				t.Fatal("closing() left the shell running")
			}
		}
	}
}
