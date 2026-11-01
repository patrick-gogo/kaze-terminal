// Package gitinfo reads the current branch straight from .git files, so the
// status bar never has to spawn git.
package gitinfo

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
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
	return repoAt(gd, top, trueCase)
}

func repoAt(gd, top string, canonical func(string) string) (key, name string) {
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
	common = canonical(common)
	if filepath.Base(common) == ".git" {
		return common, filepath.Base(filepath.Dir(common))
	}
	return common, filepath.Base(canonical(top))
}

// caseless is true where the usual disks ignore letter case, so one folder can
// be typed several ways.
var caseless = runtime.GOOS == "darwin" || runtime.GOOS == "windows"

// trueCase spells an absolute path the way the disk stores it, so a folder
// typed as ~/documents and ~/Documents gives one repo key. Symlinks are kept.
// A part it cannot read is left as typed. No-op on case-sensitive systems.
func trueCase(p string) string {
	if !caseless || !filepath.IsAbs(p) {
		return p
	}
	vol := filepath.VolumeName(p)
	if runtime.GOOS == "windows" {
		vol = strings.ToUpper(vol)
	}
	parts := strings.Split(p[len(filepath.VolumeName(p)):], string(filepath.Separator))
	cur := vol + string(filepath.Separator)
	for i, part := range parts {
		if part == "" {
			continue
		}
		name, ok := storedName(cur, part)
		if !ok {
			return filepath.Join(append([]string{cur}, parts[i:]...)...)
		}
		cur = filepath.Join(cur, name)
	}
	return cur
}

// storedName finds part in dir: an exact match first, then one that differs
// only in case.
func storedName(dir, part string) (string, bool) {
	f, err := os.Open(dir)
	if err != nil {
		return "", false
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return "", false
	}
	found := ""
	for _, n := range names {
		if n == part {
			return n, true
		}
		if found == "" && strings.EqualFold(n, part) {
			found = n
		}
	}
	if found != "" {
		return found, true
	}
	return storedAlias(dir, part)
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

// Top returns the top folder of the checkout containing dir (a worktree's own
// top, not the main checkout's), or "" outside a repo.
func Top(dir string) string {
	_, top, ok := locate(dir)
	if !ok {
		return ""
	}
	return top
}
