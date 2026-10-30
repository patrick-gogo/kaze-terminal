//go:build !windows

package gitstat

import "os/exec"

func hideWindow(*exec.Cmd) {}
