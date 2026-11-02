package mdoc

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveAndLoad(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sub")
	os.MkdirAll(dir, 0755)
	os.MkdirAll(filepath.Join(root, "docs"), 0755)
	p := filepath.Join(root, "docs", "a.md")
	os.WriteFile(p, []byte("# A\n"), 0644)
	for _, ref := range []string{"docs/a.md", "./docs/a.md", "~/docs/a.md", p} {
		got, e := Resolve(ref, Roots{Dir: root, Home: root})
		if e != nil || got != p {
			t.Fatalf("%s: %q %v", ref, got, e)
		}
	}
	if got, e := Resolve("docs/a.md", Roots{Dir: dir, Top: func(string) string { return root }}); e != nil || got != p {
		t.Fatal(got, e)
	}
	for _, ref := range []string{"missing.md", "sub.md", "a.txt"} {
		if _, e := Resolve(ref, Roots{Dir: root}); !errors.Is(e, ErrNotFound) {
			t.Fatal(ref, e)
		}
	}
	if _, e := Resolve("docs/a.md", Roots{}); !errors.Is(e, ErrNotFound) {
		t.Fatal(e)
	}
	f, e := Load(p)
	if e != nil || string(f.Data) != "# A\n" || f.Cut || f.Total != 4 || f.Stamp.Size != 4 {
		t.Fatal(f, e)
	}
	old, _ := StatFile(p)
	os.WriteFile(p, []byte("changed"), 0644)
	os.Chtimes(p, time.Now(), time.Now().Add(2*time.Second))
	now, _ := StatFile(p)
	if old == now {
		t.Fatal("unchanged stamp")
	}
	if _, e = Load(root); e == nil {
		t.Fatal("folder loaded")
	}
}
func TestPathKey(t *testing.T) {
	for _, sys := range []string{"darwin", "windows", "linux"} {
		same := PathKey("/A/./b.md", sys) == PathKey("/a/B.md", sys)
		if same != (sys != "linux") {
			t.Fatal(sys)
		}
	}
}
func TestLoadCapsAtTwoMegabytes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a.md")
	data := bytes.Repeat([]byte("123456789\n"), MaxBytes/10+500)
	os.WriteFile(p, data, 0644)
	f, e := Load(p)
	if e != nil || !f.Cut || len(f.Data) > MaxBytes || f.Data[len(f.Data)-1] != '\n' || f.Total != int64(len(data)) {
		t.Fatal(len(f.Data), e)
	}
}
