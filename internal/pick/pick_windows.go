package pick

import (
	"os"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// script shows the folder dialog; PowerShell 7 draws it Explorer-style. The
// start folder comes from the environment, never from the script text.
const script = `Add-Type -AssemblyName System.Windows.Forms
$d = New-Object System.Windows.Forms.FolderBrowserDialog
$d.Description = "Choose the tab's folder"
if ($d.PSObject.Properties["UseDescriptionForTitle"]) { $d.UseDescriptionForTitle = $true }
if ($env:KAZE_PICK_START) { $d.SelectedPath = $env:KAZE_PICK_START }
if ($d.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { [Console]::Out.Write($d.SelectedPath) }`

// folderCmd runs the dialog in PowerShell 7 when installed, else Windows PowerShell,
// without flashing a console window.
func folderCmd(start string) *exec.Cmd {
	exe := "powershell"
	if p, err := exec.LookPath("pwsh"); err == nil {
		exe = p
	}
	cmd := exec.Command(exe, "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.Env = append(os.Environ(), "KAZE_PICK_START="+start)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}
