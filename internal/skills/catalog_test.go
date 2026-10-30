package skills

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillFile(name, desc string) string {
	return "---\nname: " + name + "\ndescription: " + desc + "\n---\nbody\n"
}

func names(list []Skill) []string {
	var out []string
	for _, s := range list {
		out = append(out, s.Name)
	}
	return out
}

func find(list []Skill, name string) (Skill, bool) {
	for _, s := range list {
		if s.Name == name {
			return s, true
		}
	}
	return Skill{}, false
}

func TestLoadUserAndProject(t *testing.T) {
	home, proj := t.TempDir(), t.TempDir()
	put(t, filepath.Join(home, "skills", "a", "SKILL.md"), skillFile("a", "Skill a"))
	put(t, filepath.Join(home, "commands", "b.md"), "Command b\n")
	put(t, filepath.Join(home, "commands", "sub", "c.md"), "---\ndescription: Command c\n---\n")
	put(t, filepath.Join(proj, ".claude", "skills", "d", "SKILL.md"), skillFile("d", "Skill d"))
	got := NewCache().Load(Sources{ClaudeDir: home, Project: proj})
	if strings.Join(names(got), ",") != "a,b,c,d" {
		t.Fatalf("names %v", names(got))
	}
	want := map[string]Scope{"a": User, "b": User, "c": User, "d": Project}
	for _, s := range got {
		if s.Scope != want[s.Name] {
			t.Errorf("%s scope %v", s.Name, s.Scope)
		}
	}
	if c, _ := find(got, "c"); c.Desc != "Command c" {
		t.Errorf("c desc %q", c.Desc)
	}
	if b, _ := find(got, "b"); b.Desc != "Command b" || !strings.HasSuffix(b.Path, "b.md") {
		t.Errorf("b %+v", b)
	}
}

func TestLoadProjectWinsOverUser(t *testing.T) {
	home, proj := t.TempDir(), t.TempDir()
	put(t, filepath.Join(home, "skills", "x", "SKILL.md"), skillFile("x", "user"))
	put(t, filepath.Join(proj, ".claude", "commands", "x.md"), "project\n")
	got := NewCache().Load(Sources{ClaudeDir: home, Project: proj})
	if len(got) != 1 || got[0].Scope != Project || got[0].Desc != "project" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadSkillBeatsCommand(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, "commands", "x.md"), "command\n")
	put(t, filepath.Join(home, "skills", "x", "SKILL.md"), skillFile("x", "skill"))
	got := NewCache().Load(Sources{ClaudeDir: home})
	if len(got) != 1 || got[0].Desc != "skill" {
		t.Fatalf("got %+v", got)
	}
}

func TestLoadSkipsBadFiles(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, "skills", "good", "SKILL.md"), skillFile("good", "fine"))
	put(t, filepath.Join(home, "skills", "locked", "SKILL.md"), skillFile("locked", "x"))
	put(t, filepath.Join(home, "commands", "bin.md"), "ab\x00\x01\x02cd")
	put(t, filepath.Join(home, "commands", "huge.md"), "Huge first line\n"+strings.Repeat("y", 10<<20))
	put(t, filepath.Join(home, "commands", "never.md"), "---\nname: x\n")
	c := NewCache()
	var maxGot int64
	c.open = func(p string) (io.ReadCloser, error) {
		if strings.Contains(p, "locked") {
			return nil, errors.New("denied")
		}
		f, err := os.Open(p)
		if err != nil {
			return nil, err
		}
		return &countingReader{f: f, n: &maxGot}, nil
	}
	got := c.Load(Sources{ClaudeDir: home})
	if strings.Join(names(got), ",") != "good,huge,never" {
		t.Fatalf("names %v", names(got))
	}
	if maxGot > maxRead+4096 {
		t.Fatalf("read %d bytes of one file", maxGot)
	}
	if h, _ := find(got, "huge"); h.Desc != "Huge first line" {
		t.Errorf("huge desc %q", h.Desc)
	}
}

type countingReader struct {
	f   *os.File
	got int64
	n   *int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	k, err := r.f.Read(p)
	r.got += int64(k)
	if r.got > *r.n {
		*r.n = r.got
	}
	return k, err
}
func (r *countingReader) Close() error { return r.f.Close() }

func TestLoadIsBoundedInDepthAndCount(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "commands")
	for i := 1; i <= 6; i++ {
		put(t, filepath.Join(dir, fmt.Sprintf("deep%d.md", i)), "d\n")
		dir = filepath.Join(dir, fmt.Sprintf("l%d", i))
	}
	got := NewCache().Load(Sources{ClaudeDir: home})
	if strings.Join(names(got), ",") != "deep1,deep2,deep3" {
		t.Fatalf("depth: %v", names(got))
	}

	many := t.TempDir()
	for i := 0; i < 1200; i++ {
		put(t, filepath.Join(many, "commands", fmt.Sprintf("c%04d.md", i)), "x\n")
	}
	if got := NewCache().Load(Sources{ClaudeDir: many}); len(got) != MaxSkills {
		t.Fatalf("count %d", len(got))
	}
}

func TestLoadSkipsHiddenFolders(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, "commands", ".git", "x.md"), "x\n")
	put(t, filepath.Join(home, "commands", "has space", "y.md"), "y\n")
	put(t, filepath.Join(home, "commands", "ok.md"), "x\n")
	if got := NewCache().Load(Sources{ClaudeDir: home}); strings.Join(names(got), ",") != "ok" {
		t.Fatalf("names %v", names(got))
	}
}

func TestLoadRereadsOnlyChangedFiles(t *testing.T) {
	home := t.TempDir()
	put(t, filepath.Join(home, "skills", "a", "SKILL.md"), skillFile("a", "one"))
	put(t, filepath.Join(home, "commands", "b.md"), "b\n")
	c := NewCache()
	opens := 0
	c.open = func(p string) (io.ReadCloser, error) { opens++; return os.Open(p) }
	src := Sources{ClaudeDir: home}
	if len(c.Load(src)) != 2 || opens != 2 {
		t.Fatalf("first load opened %d", opens)
	}
	opens = 0
	c.Load(src)
	if opens != 0 {
		t.Fatalf("unchanged load opened %d", opens)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(home, "commands", "b.md"), later, later); err != nil {
		t.Fatal(err)
	}
	c.Load(src)
	if opens != 1 {
		t.Fatalf("one changed file opened %d", opens)
	}
	os.Remove(filepath.Join(home, "commands", "b.md"))
	if got := c.Load(src); strings.Join(names(got), ",") != "a" {
		t.Fatalf("after delete %v", names(got))
	}
	if len(c.files) != 1 {
		t.Fatalf("cache keeps %d entries", len(c.files))
	}
}

func snapshot(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, fmt.Sprintf("%s %d %d %v", p, fi.Size(), fi.ModTime().UnixNano(), fi.Mode()))
		return nil
	})
	sort.Strings(out)
	return out
}

func TestLoadNeverWrites(t *testing.T) {
	home, proj := t.TempDir(), t.TempDir()
	put(t, filepath.Join(home, "skills", "a", "SKILL.md"), skillFile("a", "x"))
	put(t, filepath.Join(home, "commands", "b.md"), "b\n")
	put(t, filepath.Join(proj, ".claude", "skills", "d", "SKILL.md"), skillFile("d", "x"))
	before := append(snapshot(t, home), snapshot(t, proj)...)
	c := NewCache()
	c.Load(Sources{ClaudeDir: home, Project: proj})
	c.Load(Sources{ClaudeDir: home, Project: proj})
	after := append(snapshot(t, home), snapshot(t, proj)...)
	if strings.Join(before, "\n") != strings.Join(after, "\n") {
		t.Fatalf("tree changed:\n%v\n%v", before, after)
	}
}

func TestLoadEmptySources(t *testing.T) {
	if got := NewCache().Load(Sources{}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
	if got := NewCache().Load(Sources{ClaudeDir: filepath.Join(t.TempDir(), "missing")}); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}
