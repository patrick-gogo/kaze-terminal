//go:build !darwin && !linux

package ptyx

// foreground is unknown here: ConPTY has no foreground process group.
func foreground(uintptr) int { return 0 }
