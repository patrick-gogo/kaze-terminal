//go:build !darwin && !linux

package procs

// processArgs is not available here yet (Windows has no foreground process group).
func processArgs(int) (string, []string) { return "", nil }
