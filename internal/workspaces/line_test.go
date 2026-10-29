package workspaces

import (
	"strings"
	"testing"
)

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

func TestCheckCommandsRejectsLinesTheJoinWouldBreak(t *testing.T) {
	bad := []struct {
		name string
		k    ShellKind
		cmds []string
		want string // part of the error
	}{
		{"ps trailing comment", ShellPowerShell, []string{"npm run dev # start", "b"}, "comment"},
		{"ps comment on the last", ShellPowerShell, []string{"a", "b  #note  "}, "comment"},
		{"ps comment after semicolon", ShellPowerShell, []string{"a;#x", "b"}, "comment"},
		{"posix background", ShellPOSIX, []string{"server &", "b"}, "&"},
		{"posix background last", ShellPOSIX, []string{"a", "tail -f log &  "}, "&"},
		{"posix trailing comment", ShellPOSIX, []string{"a # x", "b"}, "comment"},
	}
	for _, c := range bad {
		err := CheckCommands(c.k, c.cmds)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: CheckCommands = %v, want an error about %q", c.name, err, c.want)
		}
	}
	good := []struct {
		name string
		k    ShellKind
		cmds []string
	}{
		{"single with comment", ShellPowerShell, []string{"claude # me"}},
		{"single background", ShellPOSIX, []string{"server &"}},
		{"hash inside quotes", ShellPowerShell, []string{`echo "#1"`, `echo '#2'`}},
		{"hash inside a word", ShellPOSIX, []string{"echo a#b", "git log --format=%h#x"}},
		{"ps closed block comment", ShellPowerShell, []string{"a <# x #> ", "b"}},
		{"posix and", ShellPOSIX, []string{"a && b", "c"}},
		{"posix escaped hash", ShellPOSIX, []string{`echo \#x`, "b"}},
		{"ps hash in a variable", ShellPowerShell, []string{"$a#b", "c"}},
	}
	for _, c := range good {
		if err := CheckCommands(c.k, c.cmds); err != nil {
			t.Errorf("%s: CheckCommands = %v, want nil", c.name, err)
		}
	}
}
