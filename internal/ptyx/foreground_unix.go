//go:build darwin || linux

package ptyx

import "golang.org/x/sys/unix"

// foreground asks the pty master which process group owns the terminal.
func foreground(fd uintptr) int {
	pg, err := unix.IoctlGetInt(int(fd), unix.TIOCGPGRP)
	if err != nil || pg <= 0 {
		return 0
	}
	return pg
}
