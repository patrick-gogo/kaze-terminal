//go:build !darwin || ios

package ui

// systemClipImage is only needed on macOS, where Cmd+V stands in for Ctrl+V.
func systemClipImage() bool { return false }
