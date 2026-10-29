//go:build windows

package workspaces

import "testing"

// Docker Desktop is a window the owner needs to see while it starts; only
// `docker info` runs hidden.
func TestDockerDesktopLaunchIsNotHidden(t *testing.T) {
	cmd := desktopCmd(`C:\nowhere\Docker Desktop.exe`) // built, never started
	if cmd.SysProcAttr != nil && (cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&createNoWindow != 0) {
		t.Fatalf("Docker Desktop starts hidden: %+v", cmd.SysProcAttr)
	}
}
