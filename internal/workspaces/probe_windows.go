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

// desktopCmd starts Docker Desktop shown: unlike `docker info`, it is a window
// the owner may need (first-run terms, sign-in) before Docker comes up.
func desktopCmd(path string) *exec.Cmd { return exec.Command(path) }

func startDockerDesktop() error {
	candidates := []string{`C:\Program Files\Docker\Docker\Docker Desktop.exe`}
	if la := os.Getenv("LOCALAPPDATA"); la != "" {
		candidates = append([]string{filepath.Join(la, "Programs", "DockerDesktop", "Docker Desktop.exe")}, candidates...)
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			cmd := desktopCmd(c)
			if err := cmd.Start(); err != nil {
				return err
			}
			return cmd.Process.Release()
		}
	}
	return ErrDockerMissing
}
