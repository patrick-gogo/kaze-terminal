package pick

import (
	"os/exec"
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

// Windows PowerShell 5.1 can inherit an OEM console encoding. The picker is a
// byte-stream protocol, so its answer must be UTF-8 even under that legacy default.
// Exercise its setup without opening a folder dialog.
func TestFolderAnswerIsUTF8UnderLegacyConsoleEncoding(t *testing.T) {
	cmd := folderCmd("")
	setup := strings.SplitN(script, "\n", 2)[0]
	want := `C:\Users\me\日本 café`
	probe := exec.Command(cmd.Path, "-NoProfile", "-NonInteractive", "-Command",
		`[Console]::OutputEncoding = [System.Text.Encoding]::ASCII; `+setup+`; [Console]::Out.Write('`+want+`')`)
	got, err := probe.Output()
	if err != nil {
		t.Fatalf("encoding probe: %v", err)
	}
	if string(got) != want {
		t.Fatalf("picker answer %q, want UTF-8 %q", got, want)
	}
}
