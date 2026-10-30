//go:build !windows && !darwin

package ui

// processMB is not measured outside Windows and macOS yet. The caller falls back to Go's own
// estimate of memory held from the OS (Sys - HeapReleased), the nearest it has to private bytes.
func processMB() (float64, bool) { return 0, false }
