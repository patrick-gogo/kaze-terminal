package pick

import "os/exec"

// folderCmd runs AppleScript's choose folder. The start folder travels as an
// argument after --, never inside the script, so no path can change the script.
func folderCmd(start string) *exec.Cmd {
	choose := `POSIX path of (choose folder with prompt "Choose the tab's folder")`
	if start != "" {
		choose = `POSIX path of (choose folder with prompt "Choose the tab's folder" default location (POSIX file (item 1 of argv)))`
	}
	return exec.Command("osascript", "-e", "on run argv", "-e", choose, "-e", "end run", "--", start)
}
