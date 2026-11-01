package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

// writeInput keeps overload visible for keys, text, paste and mouse input.
// A message is either queued whole or rejected; the UI never waits for room.
func (win *window) writeInput(t *tabs.Tab, p []byte) bool {
	r := t.Session.Write(p)
	if r == term.QueueFull {
		win.note("Input not sent: the terminal is busy. Try again when it responds.")
	}
	return r == term.Accepted
}
