package ui

import (
	"io"
	"strings"
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/transfer"
)

func TestScratchShortcutKeepsFollowingInputWithItsDestination(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   string
		toggles int
		target  int
	}{
		{"open", "", 1, 4},
		{"hide", "shown", 1, 0},
		{"reopen", "hidden", 1, 4},
		{"open then hide", "", 2, 0},
		{"open hide reopen", "", 3, 4},
		{"hide then reopen", "shown", 2, 4},
		{"member main view is a no-op", "member", 1, 2},
		{"hide unfocused scratch", "unfocused", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			win, st, r, a, _, m1, _ := paneWindow(t)
			switch tc.setup {
			case "shown", "hidden", "unfocused":
				toggle(t, win, r)
				if tc.setup == "hidden" {
					toggle(t, win, r)
				}
				if tc.setup == "unfocused" {
					win.floatMember(m1.ID)
					floatFrames(t, win, r)
				}
			case "member":
				win.selectTab(m1.ID)
				floatFrames(t, win, r)
			}
			var input []event.Event
			for range tc.toggles {
				input = append(input, shortcutFor(t, win, actScratch))
			}
			input = append(input, key.EditEvent{Text: "echo destination"}, key.Event{Name: key.NameReturn, State: key.Press})
			r.Queue(input...)
			for range 10 {
				sideFrame(t, win, r)
			}
			waitSent(t, ptyOf(st, tc.target), "echo destination\r")
			for i := range st.ptys {
				if i != tc.target && ptyOf(st, i).sent() != "" {
					t.Fatalf("input reached unrelated pty %d: %q", i, ptyOf(st, i).sent())
				}
			}
			if tc.setup == "member" && scratchOf(win, a.ID) != nil {
				t.Fatal("member shortcut started a scratch shell")
			}
		})
	}
}

func TestScratchOpeningAcceptsNextFramesTyping(t *testing.T) {
	win, st, r, _, _, _, _ := paneWindow(t)
	r.Queue(shortcutFor(t, win, actScratch))
	sideFrame(t, win, r)
	r.Queue(key.EditEvent{Text: "next frame"})
	for range 4 {
		sideFrame(t, win, r)
	}
	waitSent(t, ptyOf(st, 4), "next frame")
	if got := ptyOf(st, 0).sent(); got != "" {
		t.Fatalf("next-frame input reached parent: %q", got)
	}
}

func TestScratchOpeningAcceptsImmediatePaste(t *testing.T) {
	win, st, r, _, _, _, _ := paneWindow(t)
	r.Queue(shortcutFor(t, win, actScratch), shortcutFor(t, win, actPaste))
	sideFrame(t, win, r)
	if !r.ClipboardRequested() {
		t.Fatal("opening scratch did not request clipboard")
	}
	r.Queue(transfer.DataEvent{Type: "application/text", Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("pasted")) }})
	for range 4 {
		sideFrame(t, win, r)
	}
	waitSent(t, ptyOf(st, 4), "pasted")
	if got := ptyOf(st, 0).sent(); got != "" {
		t.Fatalf("paste reached parent: %q", got)
	}
}
