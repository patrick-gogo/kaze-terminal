//go:build !darwin && !windows

package pick

import "os/exec"

// folderCmd: no picker kaze knows how to open here.
func folderCmd(string) *exec.Cmd { return nil }
