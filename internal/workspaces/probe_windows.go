//go:build windows

package workspaces

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow stops `docker info` from flashing a console window.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}

func startDockerDesktop() error {
	candidates := []string{`C:\Program Files\Docker\Docker\Docker Desktop.exe`}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		candidates = append([]string{filepath.Join(la, "Programs", "DockerDesktop", "Docker Desktop.exe")}, candidates...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			cmd := exec.Command(c)
			hideWindow(cmd)
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Process.Release()
		}
	}
	return ErrDockerMissing
}
