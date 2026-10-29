package workspaces

import "testing"

func TestKindOf(t *testing.T) {
	cases := map[string]ShellKind{
		`C:\Program Files\PowerShell\7\pwsh.exe`: ShellPowerShell,
		`powershell.exe`:                         ShellPowerShell,
		`PWSH`:                                   ShellPowerShell,
		`/bin/zsh`:                               ShellPOSIX,
		`bash`:                                   ShellPOSIX,
		`/bin/sh`:                                ShellPOSIX,
		`cmd.exe`:                                ShellUnknown,
		`fish`:                                   ShellUnknown,
	}
	for exe, want := range cases {
		if got := KindOf([]string{exe, "-l"}); got != want {
			t.Errorf("KindOf(%q) = %v, want %v", exe, got, want)
		}
	}
	if KindOf(nil) != ShellUnknown {
		t.Error("empty argv should be unknown")
	}
}

func TestJoinLine(t *testing.T) {
	three := []string{"a", "b", "c"}
	tests := []struct {
		name string
		k    ShellKind
		cmds []string
		want string
	}{
		{"single ps", ShellPowerShell, []string{"claude"}, "claude"},
		{"single posix", ShellPOSIX, []string{"claude"}, "claude"},
		{"three ps", ShellPowerShell, three, "a; if ($?) { b }; if ($?) { c }"},
		{"three posix", ShellPOSIX, three, "a && b && c"},
		{"blank skipped", ShellPOSIX, []string{"a", "  ", "", "b"}, "a && b"},
		{"verbatim ps", ShellPowerShell, []string{`echo "hi $x` + "`n" + `" 'é日本'`, "b"}, `echo "hi $x` + "`n" + `" 'é日本'; if ($?) { b }`},
		{"verbatim posix", ShellPOSIX, []string{`echo "$HOME" 'x' ` + "`date`" + ` 日本`, "b"}, `echo "$HOME" 'x' ` + "`date`" + ` 日本 && b`},
		{"empty", ShellUnknown, nil, ""},
	}
	for _, tc := range tests {
		got, err := JoinLine(tc.k, tc.cmds)
		if err != nil || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := JoinLine(ShellUnknown, []string{"a"}); err == nil {
		t.Error("unknown shell with commands should error")
	}
}
