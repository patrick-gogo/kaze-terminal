// Package skills lists the owner's Claude Code skills and slash commands for
// the palette. It only reads: nothing under ~/.claude or .claude is written.
package skills

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MaxSkills   = 1000     // entries listed; bounds a huge tree
	maxRead     = 32 << 10 // bytes read from one file
	maxCmdDepth = 3        // folder levels under commands/, the file's own included
	maxVisit    = 20000    // directory entries walked per Load
)

// Scope says where a skill was found.
type Scope int

const (
	User Scope = iota
	Project
	Plugin
)

// Skill is one entry the palette can start.
type Skill struct {
	Name  string // as typed after "/": "ship-ticket", or "superpowers:brainstorming" for a plugin
	Desc  string // one line, at most 200 runes
	Scope Scope
	Path  string
}

// Sources are the folders Load reads.
type Sources struct {
	ClaudeDir string // the owner's ~/.claude; "" lists no user or plugin entries
	Project   string // the current session's checkout top; "" lists no project entries
}

type fileEntry struct {
	mod        time.Time
	size       int64
	name, desc string
	ok         bool
}

// Cache keeps parsed files between loads, so an unchanged file (same
// modification time and size) is never read again. One goroutine at a time.
type Cache struct {
	files map[string]fileEntry
	raw   map[string]rawEntry // settings.json, installed_plugins.json
	open  func(string) (io.ReadCloser, error)
	seen  map[string]bool // files met by the current Load
	visit int
}

func NewCache() *Cache {
	return &Cache{
		files: map[string]fileEntry{},
		raw:   map[string]rawEntry{},
		open:  func(p string) (io.ReadCloser, error) { return os.Open(p) },
	}
}

// found is one candidate before duplicates are dropped; rank orders them,
// lower wins: project before user, a skill before a command.
type found struct {
	Skill
	rank int
}

// Load lists the skills and commands in src: deduped (a project entry beats
// a user one; in one scope a skill beats a command), sorted by name case
// insensitively, at most MaxSkills.
func (c *Cache) Load(src Sources) []Skill {
	c.seen, c.visit = map[string]bool{}, 0
	var all []found
	if src.Project != "" {
		dot := filepath.Join(src.Project, ".claude")
		all = append(all, c.scanSkills(filepath.Join(dot, "skills"), Project, "", 0)...)
		all = append(all, c.scanCommands(filepath.Join(dot, "commands"), Project, "", 1)...)
	}
	if src.ClaudeDir != "" {
		all = append(all, c.scanSkills(filepath.Join(src.ClaudeDir, "skills"), User, "", 2)...)
		all = append(all, c.scanCommands(filepath.Join(src.ClaudeDir, "commands"), User, "", 3)...)
		all = append(all, c.plugins(src.ClaudeDir)...)
	}
	for p := range c.files {
		if !c.seen[p] {
			delete(c.files, p)
		}
	}
	for p := range c.raw {
		if !c.seen[p] {
			delete(c.raw, p)
		}
	}
	best := map[string]found{}
	for _, f := range all {
		if b, ok := best[f.Name]; !ok || f.rank < b.rank {
			best[f.Name] = f
		}
	}
	out := make([]Skill, 0, len(best))
	for _, f := range best {
		out = append(out, f.Skill)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > MaxSkills {
		out = out[:MaxSkills]
	}
	return out
}

// scanSkills lists dir/*/SKILL.md, named by frontmatter or the folder.
func (c *Cache) scanSkills(dir string, scope Scope, prefix string, rank int) []found {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []found
	for _, e := range ents {
		if c.visit++; c.visit > maxVisit {
			break
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := filepath.Join(dir, e.Name(), "SKILL.md")
		if name, desc, ok := c.parse(p, e.Name()); ok {
			out = append(out, found{Skill{Name: prefix + name, Desc: desc, Scope: scope, Path: p}, rank})
		}
	}
	return out
}

// scanCommands lists dir/**/*.md, named by the file (a plugin's by its path). WalkDir does not follow
// symlinked folders, so a link loop can't trap it.
func (c *Cache) scanCommands(dir string, scope Scope, prefix string, rank int) []found {
	var out []found
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir {
				return fs.SkipDir
			}
			return nil
		}
		if c.visit++; c.visit > maxVisit {
			return fs.SkipAll
		}
		rel, _ := filepath.Rel(dir, p)
		depth := len(strings.Split(rel, string(filepath.Separator)))
		if d.IsDir() {
			if p != dir && (!validName.MatchString(d.Name()) || depth >= maxCmdDepth) {
				return fs.SkipDir
			}
			return nil
		}
		base, isMD := strings.CutSuffix(d.Name(), ".md")
		if !isMD || !validName.MatchString(base) {
			return nil
		}
		name := base
		if scope == Plugin {
			// Claude Code names a plugin's nested command by its folders too.
			name = strings.ReplaceAll(strings.TrimSuffix(filepath.ToSlash(rel), ".md"), "/", ":")
		}
		if _, desc, ok := c.parse(p, base); ok {
			out = append(out, found{Skill{Name: prefix + name, Desc: desc, Scope: scope, Path: p}, rank})
		}
		return nil
	})
	return out
}

// parse returns a file's name and description, from the cache when the file
// is unchanged. Missing, non-regular, unreadable or binary files are skipped.
func (c *Cache) parse(path, fallback string) (name, desc string, ok bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return "", "", false
	}
	c.seen[path] = true
	if e, hit := c.files[path]; hit && e.mod.Equal(fi.ModTime()) && e.size == fi.Size() {
		return e.name, e.desc, e.ok
	}
	e := fileEntry{mod: fi.ModTime(), size: fi.Size()}
	if data, err := c.read(path); err == nil && bytes.IndexByte(data, 0) < 0 {
		e.name, e.desc, e.ok = parseFile(data, fallback)
	}
	c.files[path] = e
	return e.name, e.desc, e.ok
}

func (c *Cache) read(path string) ([]byte, error) {
	f, err := c.open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxRead))
}

type rawEntry struct {
	mod  time.Time
	size int64
	data []byte
}

func readAllLimit(r io.Reader, buf []byte, limit int64) ([]byte, error) {
	b := bytes.NewBuffer(buf)
	_, err := b.ReadFrom(io.LimitReader(r, limit))
	return b.Bytes(), err
}
