package skills

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const maxJSON = 4 << 20 // settings.json and installed_plugins.json

var semverish = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)([-+].*)?$`)

// enabledPlugins lists the "name@marketplace" keys of settings.json's
// enabledPlugins whose value is exactly true, sorted.
func enabledPlugins(settings []byte) []string {
	var s struct {
		EnabledPlugins map[string]any `json:"enabledPlugins"`
	}
	if json.Unmarshal(settings, &s) != nil {
		return nil
	}
	var out []string
	for k, v := range s.EnabledPlugins {
		if b, ok := v.(bool); ok && b {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// splitKey splits "name@marketplace" at the last "@"; both parts must be
// plain names, so a key can never point outside the plugin cache.
func splitKey(key string) (name, market string, ok bool) {
	i := strings.LastIndex(key, "@")
	if i <= 0 {
		return "", "", false
	}
	name, market = key[:i], key[i+1:]
	return name, market, validName.MatchString(name) && validName.MatchString(market)
}

// pluginRoot finds the folder of a plugin's newest installed version:
// Claude Code's own record first, else the highest version folder in the cache.
func pluginRoot(claudeDir, key string, installed []byte) (string, bool) {
	name, market, ok := splitKey(key)
	if !ok {
		return "", false
	}
	base := filepath.Join(claudeDir, "plugins")
	var rec struct {
		Plugins map[string][]struct {
			InstallPath string `json:"installPath"`
			Version     string `json:"version"`
			LastUpdated string `json:"lastUpdated"`
		} `json:"plugins"`
	}
	if json.Unmarshal(installed, &rec) == nil {
		best, bestV, bestT := "", "", ""
		for _, e := range rec.Plugins[key] {
			if !insideDir(base, e.InstallPath) || !isDir(e.InstallPath) {
				continue
			}
			if best == "" || newerVersion(e.Version, bestV) ||
				(!newerVersion(bestV, e.Version) && e.LastUpdated > bestT) {
				best, bestV, bestT = filepath.Clean(e.InstallPath), e.Version, e.LastUpdated
			}
		}
		if best != "" {
			return best, true
		}
	}
	dir := filepath.Join(base, "cache", market, name)
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	best, bestMod := "", time.Time{}
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		fi, err := os.Stat(p)
		if err != nil || !fi.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if best == "" || newerFolder(e.Name(), fi.ModTime(), best, bestMod) {
			best, bestMod = e.Name(), fi.ModTime()
		}
	}
	if best == "" {
		return "", false
	}
	return filepath.Join(dir, best), true
}

// newerFolder orders cache folders: a semver-ish name beats any other; among
// other names the later modification time wins, then the larger name.
func newerFolder(a string, aMod time.Time, b string, bMod time.Time) bool {
	as, bs := semverish.MatchString(a), semverish.MatchString(b)
	switch {
	case as && bs:
		return newerVersion(a, b)
	case as != bs:
		return as
	case !aMod.Equal(bMod):
		return aMod.After(bMod)
	}
	return a > b
}

// newerVersion reports whether a is a newer version than b. Parts compare
// numerically, missing parts count as 0, and "1.0.0-beta" is older than
// "1.0.0". A semver-ish version beats one that is not.
func newerVersion(a, b string) bool {
	ma, mb := semverish.FindStringSubmatch(a), semverish.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		return ma != nil && mb == nil
	}
	pa, pb := strings.Split(ma[1], "."), strings.Split(mb[1], ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		x, y := part(pa, i), part(pb, i)
		if x != y {
			return x > y
		}
	}
	preA, preB := strings.HasPrefix(ma[2], "-"), strings.HasPrefix(mb[2], "-")
	return !preA && preB
}

func part(p []string, i int) int {
	if i >= len(p) {
		return 0
	}
	n, err := strconv.Atoi(p[i])
	if err != nil {
		return 0
	}
	return n
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

// insideDir reports whether p is a folder under base, comparing resolved
// paths so a symlinked temp or home folder still matches.
func insideDir(base, p string) bool {
	if p == "" || !filepath.IsAbs(p) {
		return false
	}
	for _, pair := range [][2]string{{base, p}, {resolve(base), resolve(p)}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// plugins lists the skills and commands of every enabled, installed plugin,
// from its newest version only, named "plugin:name".
func (c *Cache) plugins(claudeDir string) []found {
	settings := c.readRaw(filepath.Join(claudeDir, "settings.json"))
	keys := enabledPlugins(settings)
	if len(keys) == 0 {
		return nil
	}
	installed := c.readRaw(filepath.Join(claudeDir, "plugins", "installed_plugins.json"))
	var out []found
	for _, key := range keys {
		root, ok := pluginRoot(claudeDir, key, installed)
		if !ok {
			continue
		}
		name, _, _ := splitKey(key)
		prefix := name + ":"
		out = append(out, c.scanSkills(filepath.Join(root, "skills"), Plugin, prefix, 4)...)
		out = append(out, c.scanCommands(filepath.Join(root, "commands"), Plugin, prefix, 5)...)
	}
	return out
}

// readRaw reads a small JSON file, kept by modification time and size like
// the skill files.
func (c *Cache) readRaw(path string) []byte {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	c.seen[path] = true
	if e, hit := c.raw[path]; hit && e.mod.Equal(fi.ModTime()) && e.size == fi.Size() {
		return e.data
	}
	var data []byte
	if f, err := c.open(path); err == nil {
		buf := make([]byte, 0, min(fi.Size(), maxJSON))
		data, _ = readAllLimit(f, buf, maxJSON)
		f.Close()
	}
	c.raw[path] = rawEntry{mod: fi.ModTime(), size: fi.Size(), data: data}
	return data
}
