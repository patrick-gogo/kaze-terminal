// Package mdoc resolves, reads and parses read-only markdown documents.
package mdoc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrNotFound = errors.New("not found")

type Roots struct {
	Dir, Home string
	Top       func(string) string
}

func IsMarkdown(path string) bool { return strings.EqualFold(filepath.Ext(path), ".md") }
func PathKey(path, goos string) string {
	path = filepath.Clean(path)
	if goos == "darwin" || goos == "windows" {
		path = strings.ToLower(path)
	}
	return path
}
func Resolve(ref string, r Roots) (string, error) {
	ref = filepath.FromSlash(ref)
	if strings.HasPrefix(ref, "~/") || strings.HasPrefix(ref, `~\`) {
		if r.Home == "" {
			return "", ErrNotFound
		}
		ref = filepath.Join(r.Home, ref[2:])
	}
	if !IsMarkdown(ref) {
		return "", ErrNotFound
	}
	var candidates []string
	if filepath.IsAbs(ref) {
		candidates = []string{ref}
	} else if r.Dir != "" {
		candidates = append(candidates, filepath.Join(r.Dir, ref))
		if r.Top != nil {
			if top := r.Top(r.Dir); top != "" && top != r.Dir {
				candidates = append(candidates, filepath.Join(top, ref))
			}
		}
	}
	for _, p := range candidates {
		if st, e := os.Stat(p); e == nil && st.Mode().IsRegular() {
			abs, e := filepath.Abs(p)
			return abs, e
		}
	}
	return "", ErrNotFound
}
