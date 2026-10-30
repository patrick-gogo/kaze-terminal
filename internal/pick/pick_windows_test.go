package pick

import (
	"slices"
	"strings"
	"testing"
)

func TestFolderCmdPassesStartInEnv(t *testing.T) {
	start := `C:\Users\a b\'x'\日本`
	cmd := folderCmd(start)
	if !slices.Contains(cmd.Env, "KAZE_PICK_START="+start) {
		t.Fatal("the start folder is not in KAZE_PICK_START")
	}
	for i, a := range cmd.Args {
		if strings.Contains(a, "日本") {
			t.Fatalf("arg %d %q carries the start folder", i, a)
		}
	}
}
