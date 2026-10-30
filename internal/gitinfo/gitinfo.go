// Package gitinfo reads the current branch straight from .git files, so the
// status bar never has to spawn git.
package gitinfo

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxDepth = 64   // parent levels to search; bounds the walk on hostile trees
	maxRead  = 4096 // HEAD and .git files are tiny; never read more
)

// locate walks up from dir to the checkout holding .git and returns its git
// folder and top folder. A .git file (worktree, submodule) is resolved once,
// not followed further, so a self-pointing file ends here.
func locate(dir string) (gitdir, top string, ok bool) {
	if dir == "" {
		return "", "", false
	}
	cur, err := filepath.Abs(dir)
	if err != nil {
		return "", "", false
	}
	for i := 0; i < maxDepth; i++ {
		git := filepath.Join(cur, ".git")
		if fi, err := os.Stat(git); err == nil {
			if fi.IsDir() {
				return git, cur, true
			}
			gd, ok := parseGitdir(git)
			if !ok {
				return "", "", false
			}
			if !filepath.IsAbs(gd) {
				gd = filepath.Join(cur, gd)
			}
			return filepath.Clean(gd), cur, true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", "", false
		}
		cur = parent
	}
	return "", "", false
}

// Branch returns the branch name for the repo containing dir, the first 7 hex
// chars for a detached HEAD, or "" when dir is not in a repo or is unreadable.
func Branch(dir string) string {
	gd, _, ok := locate(dir)
	if !ok {
		return ""
	}
	return readHead(gd)
}

// Repo identifies the repo containing dir. key is the absolute common git
// folder, shared by a repo's worktrees, "" outside a repo. name is the folder
// to show: the main checkout's folder, or the top folder for a submodule.
func Repo(dir string) (key, name string) {
	gd, top, ok := locate(dir)
	if !ok {
		return "", ""
	}
	if fi, err := os.Stat(gd); err != nil || !fi.IsDir() {
		return "", "" // a .git file pointing at a file, e.g. itself
	}
	common := gd
	if rel, ok := readSmall(filepath.Join(gd, "commondir")); ok && rel != "" {
		common = filepath.FromSlash(rel)
		if !filepath.IsAbs(common) {
			common = filepath.Join(gd, common)
		}
		common = filepath.Clean(common)
	}
	if filepath.Base(common) == ".git" {
		return common, filepath.Base(filepath.Dir(common))
	}
	return common, filepath.Base(top)
}

func readSmall(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxRead))
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

func parseGitdir(file string) (string, bool) {
	s, ok := readSmall(file)
	if !ok {
		return "", false
	}
	line, _, _ := strings.Cut(s, "\n")
	rest, found := strings.CutPrefix(line, "gitdir:")
	if !found {
		return "", false
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return "", false
	}
	return filepath.FromSlash(rest), true
}

func readHead(gitdir string) string {
	s, ok := readSmall(filepath.Join(gitdir, "HEAD"))
	if !ok {
		return ""
	}
	if ref, found := strings.CutPrefix(s, "ref:"); found {
		name, found := strings.CutPrefix(strings.TrimSpace(ref), "refs/heads/")
		if !found {
			return ""
		}
		return clean(name)
	}
	if len(s) >= 40 && isHex(s[:40]) {
		return s[:7]
	}
	return ""
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// clean drops control characters so a crafted branch name cannot inject
// escape sequences into the UI.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}
