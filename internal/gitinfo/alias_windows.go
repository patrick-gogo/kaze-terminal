package gitinfo

import (
	"golang.org/x/sys/windows"
	"path/filepath"
)

// storedAlias resolves Windows 8.3 components, which directory enumeration lists
// by their long name (for example RUNNER~1 versus runneradmin).
func storedAlias(dir, part string) (string, bool) {
	p, err := windows.UTF16PtrFromString(filepath.Join(dir, part))
	if err != nil {
		return "", false
	}
	buf := make([]uint16, 32768)
	n, err := windows.GetLongPathName(p, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 || n >= uint32(len(buf)) {
		return "", false
	}
	return filepath.Base(windows.UTF16ToString(buf[:n])), true
}
