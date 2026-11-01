package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveStartDir returns the cwd for a plain tab. Empty means home; other
// paths must name an existing directory. Relative paths use Kaze's launch cwd.
func ResolveStartDir(value, home string) (string, error) {
	if value == "" {
		return home, nil
	}
	dir := value
	if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, `~\`) {
		if home == "" {
			return "", fmt.Errorf("the home folder is unavailable")
		}
		dir = filepath.Join(home, strings.TrimLeft(dir[1:], `/\`))
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", fmt.Errorf("%q is not an existing folder", value)
	}
	return dir, nil
}
