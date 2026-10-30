//go:build windows

package gitstat

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow stops git from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
