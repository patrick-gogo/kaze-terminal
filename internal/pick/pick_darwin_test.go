package pick

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFolderCmdPassesStartAsArgument(t *testing.T) {
	start := "/Users/a b/\"x\"/日本"
	cmd := folderCmd(start)
	if filepath.Base(cmd.Path) != "osascript" {
		t.Fatalf("runs %q, want osascript", cmd.Path)
	}
	if cmd.Args[len(cmd.Args)-1] != start {
		t.Fatalf("last arg %q, want the start folder", cmd.Args[len(cmd.Args)-1])
	}
	for i, a := range cmd.Args[:len(cmd.Args)-1] {
		if strings.Contains(a, "日本") || strings.Contains(a, `"x"`) {
			t.Fatalf("arg %d %q carries the start folder into the script", i, a)
		}
	}
	if !slices.Contains(cmd.Args, "--") {
		t.Fatalf("args %q: the folder must follow --", cmd.Args)
	}
}

func TestFolderCmdWithoutStartHasNoDefaultLocation(t *testing.T) {
	cmd := folderCmd("")
	if strings.Contains(strings.Join(cmd.Args, " "), "default location") {
		t.Fatalf("args %q name a default location without a start folder", cmd.Args)
	}
}

// osascript does not bring itself forward, so without activate the dialog can
// open behind kaze's window.
func TestFolderCmdActivatesBeforeChoosing(t *testing.T) {
	args := strings.Join(folderCmd("/tmp").Args, "\n")
	a, c := strings.Index(args, "activate"), strings.Index(args, "choose folder")
	if a < 0 || a > c {
		t.Fatalf("args %q: activate must come before choose folder", args)
	}
}
