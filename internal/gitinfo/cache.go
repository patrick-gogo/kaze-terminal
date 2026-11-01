package gitinfo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Info is one discovery of a checkout and its current HEAD.
type Info struct{ Branch, Repo, Name string }

type caseKey struct{ dir, part string }
type caseEntry struct {
	parent os.FileInfo
	name   string
}

// Cache retains bounded path spellings, not HEAD or repository membership.
// Parent identity and modification time invalidate a spelling after a rename.
// It is safe for concurrent background lookups; no goroutine or timer is owned.
type Cache struct {
	mu    sync.Mutex
	paths map[caseKey]caseEntry
	names func(string, string) (string, bool) // optional test seam
}

const maxCasePaths = 512

func (c *Cache) Lookup(dir string) Info {
	c.mu.Lock()
	defer c.mu.Unlock()
	gd, top, ok := locate(dir)
	if !ok {
		return Info{}
	}
	key, name := repoAt(gd, top, c.trueCase)
	return Info{Branch: readHead(gd), Repo: key, Name: name}
}

func (c *Cache) trueCase(p string) string {
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
		key := caseKey{cur, part}
		parent, err := os.Stat(cur)
		cached, hit := c.paths[key]
		if err == nil && hit && os.SameFile(parent, cached.parent) && parent.ModTime().Equal(cached.parent.ModTime()) {
			cur = filepath.Join(cur, cached.name)
			continue
		}
		names := c.names
		if names == nil {
			names = storedName
		}
		name, ok := names(cur, part)
		if !ok {
			return filepath.Join(append([]string{cur}, parts[i:]...)...)
		}
		if err == nil {
			if c.paths == nil || len(c.paths) >= maxCasePaths {
				c.paths = make(map[caseKey]caseEntry)
			}
			c.paths[key] = caseEntry{parent: parent, name: name}
		}
		cur = filepath.Join(cur, name)
	}
	return cur
}
