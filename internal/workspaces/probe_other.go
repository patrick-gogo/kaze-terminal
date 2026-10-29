//go:build !windows

package workspaces

import (
	"os"
	"os/exec"
	"runtime"
)

func hideWindow(*exec.Cmd) {}

func startDockerDesktop() error {
	if runtime.GOOS != "darwin" {
		return ErrDockerMissing
	}
	if _, err := os.Stat("/Applications/Docker.app"); err != nil {
		return ErrDockerMissing
	}
	return exec.Command("open", "-a", "Docker").Run()
}
