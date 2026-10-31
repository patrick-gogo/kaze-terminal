//go:build !darwin || ios

package ui

import "gioui.org/io/event"

func macView(event.Event) (uintptr, bool) { return 0, false }

// Kaze draws every window button itself here; there are no native lights.
func newMacBridge(run func(func()), invalidate func(), barH, left float64) *macBridge { return nil }

// MacFrameless is false off macOS: Windows decides through CustomTitleBar.
func MacFrameless() bool { return false }
