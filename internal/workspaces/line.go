package workspaces

import (
	"fmt"
	"strings"
	"unicode"
)

// ShellKind says how a shell chains commands, so a tab's startup commands can
// be typed as one line that stops at the first failure.
type ShellKind int

const (
	ShellUnknown ShellKind = iota
	ShellPowerShell
	ShellPOSIX
)

func (k ShellKind) String() string {
	switch k {
	case ShellPowerShell:
		return "PowerShell"
	case ShellPOSIX:
		return "zsh/bash/sh"
	}
	return "unknown"
}

// KindOf classifies a shell argv by its program name, ignoring path,
// extension and case. Backslashes are split by hand so Windows paths work anywhere.
func KindOf(argv []string) ShellKind {
	if len(argv) == 0 {
		return ShellUnknown
	}
	name := argv[0]
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	switch strings.ToLower(name) {
	case "pwsh", "powershell":
		return ShellPowerShell
	case "zsh", "bash", "sh":
		return ShellPOSIX
	}
	return ShellUnknown
}

// JoinLine builds the single line typed into the shell. Commands are copied
// verbatim; blank ones are skipped.
func JoinLine(k ShellKind, cmds []string) (string, error) {
	var parts []string
	for _, c := range cmds {
		if strings.TrimSpace(c) != "" {
			parts = append(parts, c)
		}
	}
	if len(parts) == 0 {
		return "", nil
	}
	var sep func(string) string
	switch k {
	case ShellPowerShell:
		sep = func(c string) string { return "; if ($?) { " + c + " }" }
	case ShellPOSIX:
		sep = func(c string) string { return " && " + c }
	default:
		return "", fmt.Errorf("commands can't run in this shell (%s): only PowerShell, zsh, bash and sh are supported", k)
	}
	var b strings.Builder
	b.WriteString(parts[0])
	for _, c := range parts[1:] {
		b.WriteString(sep(c))
	}
	return b.String(), nil
}

// CheckCommands rejects commands that type fine alone but break the joined
// line: a # comment swallows whatever is joined after it, and a POSIX command
// ending in & cannot be followed by &&.
func CheckCommands(k ShellKind, cmds []string) error {
	var parts []string
	for _, c := range cmds {
		if strings.TrimSpace(c) != "" {
			parts = append(parts, c)
		}
	}
	if len(parts) < 2 || k == ShellUnknown {
		return nil // nothing is joined
	}
	for i, c := range parts {
		if hasComment(k, c) {
			return fmt.Errorf("command %d has a # comment, which would hide the commands joined after it; remove the comment", i+1)
		}
		t := strings.TrimRightFunc(c, unicode.IsSpace)
		if k == ShellPOSIX && strings.HasSuffix(t, "&") && !strings.HasSuffix(t, `\&`) {
			return fmt.Errorf("command %d ends with &, which can't be joined to the next command with &&; run it as the last command of its own tab, or drop the &", i+1)
		}
	}
	return nil
}

// hasComment finds a # that starts a comment: outside quotes, not escaped, at
// the start of a word. PowerShell's closed <# #> block comments are fine.
func hasComment(k ShellKind, c string) bool {
	esc := '\\'
	if k == ShellPowerShell {
		esc = '`'
	}
	rs := []rune(c)
	var quote rune
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote == '\'':
			if r == '\'' {
				quote = 0
			}
		case quote == '"':
			if r == esc {
				i++
			} else if r == '"' {
				quote = 0
			}
		case r == esc:
			i++
		case r == '\'' || r == '"':
			quote = r
		case k == ShellPowerShell && r == '<' && i+1 < len(rs) && rs[i+1] == '#':
			j := i + 2
			for j+1 < len(rs) && (rs[j] != '#' || rs[j+1] != '>') {
				j++
			}
			if j+1 >= len(rs) {
				return true // an open block comment swallows the rest too
			}
			i = j + 1
		case r == '#' && (i == 0 || unicode.IsSpace(rs[i-1]) || strings.ContainsRune(";|&(){}", rs[i-1])):
			return true
		}
	}
	return false
}
