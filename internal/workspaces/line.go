package workspaces

import (
	"fmt"
	"strings"
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
