// Package pick opens the system's own folder picker: Finder's on macOS,
// Explorer's on Windows. It blocks until the dialog closes, so callers run it
// on their own goroutine.
package pick

import (
	"bytes"
	"errors"
	"strings"
)

// ErrUnsupported means this system has no folder picker kaze knows how to open.
var ErrUnsupported = errors.New("pick: no folder picker on this system")

// Folder asks the owner for a folder, starting in start when it is not empty.
// Cancelling returns ok false and no error.
func Folder(start string) (path string, ok bool, err error) {
	cmd := folderCmd(start)
	if cmd == nil {
		return "", false, ErrUnsupported
	}
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	runErr := cmd.Run()
	return parse(out.Bytes(), stderr.Bytes(), runErr)
}

// parse reads the picker's answer. AppleScript reports a cancel as error -128;
// the Windows script prints nothing.
func parse(out, stderr []byte, runErr error) (string, bool, error) {
	if runErr != nil {
		msg := strings.TrimSpace(string(stderr))
		if strings.Contains(msg, "(-128)") {
			return "", false, nil
		}
		if msg == "" {
			msg = runErr.Error()
		}
		return "", false, errors.New(msg)
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return "", false, nil
	}
	// AppleScript ends folder paths with a separator; keep it on a bare root.
	if t := strings.TrimRight(p, `/\`); t != "" && !strings.HasSuffix(t, ":") {
		p = t
	}
	return p, true, nil
}
