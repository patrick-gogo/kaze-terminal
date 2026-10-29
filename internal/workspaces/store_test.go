package workspaces

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// samurai is the proposal's example, with an absolute folder for the second tab.
func samurai(t *testing.T) (string, Document) {
	home := t.TempDir()
	return home, Document{Workspaces: []Workspace{{
		ID: "ws_aaaa1111", Name: "My project",
		Tabs: []Tab{
			{ID: "t_00000001", Title: "Code", Folder: "~/src/project", Commands: []string{"claude"}, Color: "magenta", Wait: WaitNone},
			{ID: "t_00000002", Title: "Frontend", Folder: filepath.Join(home, "frontend"), Commands: []string{"npm run dev", `echo "a # b"`}, Color: "cyan", Wait: WaitPort, Port: 8000},
		},
	}}}
}

func TestPath(t *testing.T) {
	cfg := filepath.Join("x", "kaze", "config.toml")
	if got := Path(cfg); got != filepath.Join("x", "kaze", "workspaces.toml") {
		t.Fatal(got)
	}
}

func TestRoundTrip(t *testing.T) {
	_, d := samurai(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	p := filepath.Join(t.TempDir(), "workspaces.toml")
	rev, err := Save(p, d, Revision{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	s := string(raw)
	for _, want := range []string{"# Written by kaze's settings page. Edits here are fine, but comments are not kept.", "schema_version = 1", "[[workspaces]]", "[[workspaces.tabs]]"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
	got, rev2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if rev2 != rev || !reflect.DeepEqual(got, d) {
		t.Fatalf("round trip differs:\n%+v\n%+v", got, d)
	}
	// A second save with the new revision works and is stable.
	if _, err := Save(p, got, rev2); err != nil {
		t.Fatal(err)
	}
}

func TestMissingFileIsEmpty(t *testing.T) {
	d, rev, err := Load(filepath.Join(t.TempDir(), "workspaces.toml"))
	if err != nil || len(d.Workspaces) != 0 || rev != (Revision{}) {
		t.Fatalf("%v %v %v", d, rev, err)
	}
}

func TestSaveDropsBlankCommandsAndTrims(t *testing.T) {
	_, d := samurai(t)
	d.Workspaces[0].Name = "  My project  "
	d.Workspaces[0].Tabs[0].Commands = []string{"", "  ", "claude"}
	p := filepath.Join(t.TempDir(), "w.toml")
	if _, err := Save(p, d, Revision{}); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Workspaces[0].Name != "My project" || !reflect.DeepEqual(got.Workspaces[0].Tabs[0].Commands, []string{"claude"}) {
		t.Fatalf("%+v", got.Workspaces[0])
	}
}

const validHead = "schema_version = 1\n[[workspaces]]\nid = \"w1\"\nname = \"A\"\n"

func tabToml(folder, extra string) string {
	return "[[workspaces.tabs]]\nid = \"t1\"\ntitle = \"T\"\nfolder = '" + folder + "'\ncommands = [\"ls\"]\ncolor = \"red\"\nwait = \"none\"\n" + extra
}

func TestStrictLoadErrors(t *testing.T) {
	abs := filepath.ToSlash(t.TempDir())
	long := ""
	for i := 0; i < 21; i++ {
		long += strings.Replace(tabToml(abs, ""), `"t1"`, `"t`+string(rune('a'+i))+`"`, 1)
	}
	cases := map[string]string{
		"unknown top":     validHead + tabToml(abs, "") + "bogus = 1\n",
		"unknown nested":  "schema_version = 1\nextra = true\n",
		"schema 2":        "schema_version = 2\n",
		"no schema":       "[[workspaces]]\nid = \"w\"\nname = \"x\"\n",
		"bad toml":        "schema_version = [",
		"dup ids":         validHead + tabToml(abs, "") + "[[workspaces]]\nid = \"w1\"\nname = \"B\"\n" + tabToml(abs, ""),
		"dup names":       validHead + tabToml(abs, "") + "[[workspaces]]\nid = \"w2\"\nname = \"a\"\n" + tabToml(abs, ""),
		"bad color":       validHead + strings.Replace(tabToml(abs, ""), "red", "pink", 1),
		"port 0":          validHead + strings.Replace(tabToml(abs, "port = 0\n"), `"none"`, `"port"`, 1),
		"port 65536":      validHead + strings.Replace(tabToml(abs, "port = 65536\n"), `"none"`, `"port"`, 1),
		"port with none":  validHead + tabToml(abs, "port = 80\n"),
		"CR in command":   validHead + strings.Replace(tabToml(abs, ""), `"ls"`, `"ls\r"`, 1),
		"empty title":     validHead + strings.Replace(tabToml(abs, ""), `"T"`, `"  "`, 1),
		"no tabs":         validHead,
		"21 tabs":         validHead + long,
		"relative folder": validHead + tabToml("src/x", ""),
	}
	for name, body := range cases {
		p := filepath.Join(t.TempDir(), "workspaces.toml")
		writeFile(t, p, body)
		d, rev, err := Load(p)
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		if len(d.Workspaces) != 0 || rev != (Revision{}) {
			t.Errorf("%s: invalid file must yield nothing and a zero revision", name)
		}
		if !strings.Contains(err.Error(), p) {
			t.Errorf("%s: error should name the file: %v", name, err)
		}
		// Save with the zero revision must refuse and keep the bytes.
		_, d2 := samurai(t)
		if _, err := Save(p, d2, rev); !errors.Is(err, ErrInvalidFile) {
			t.Errorf("%s: Save over an invalid file: %v", name, err)
		}
		if got, _ := os.ReadFile(p); string(got) != body {
			t.Errorf("%s: bytes were replaced", name)
		}
	}
}

func TestLoadAcceptsCRLFAndTilde(t *testing.T) {
	p := filepath.Join(t.TempDir(), "workspaces.toml")
	body := strings.ReplaceAll(validHead+tabToml("~/src", ""), "\n", "\r\n")
	writeFile(t, p, body)
	d, rev, err := Load(p)
	if err != nil || len(d.Workspaces) != 1 || rev == (Revision{}) {
		t.Fatalf("%v %v", d, err)
	}
}

func TestLoadTooLarge(t *testing.T) {
	p := filepath.Join(t.TempDir(), "workspaces.toml")
	writeFile(t, p, "# "+strings.Repeat("x", MaxFileSize))
	if _, _, err := Load(p); err == nil {
		t.Fatal("want an error for a file over 1 MiB")
	}
}

func TestSaveStaleLeavesFileUntouched(t *testing.T) {
	_, d := samurai(t)
	p := filepath.Join(t.TempDir(), "workspaces.toml")
	rev, err := Save(p, d, Revision{})
	if err != nil {
		t.Fatal(err)
	}
	edited := append(mustRead(t, p), []byte("# edited elsewhere\n")...)
	writeFile(t, p, string(edited))
	if _, err := Save(p, d, rev); !errors.Is(err, ErrStale) {
		t.Fatalf("want ErrStale, got %v", err)
	}
	if !bytes.Equal(mustRead(t, p), edited) {
		t.Fatal("stale save changed the file")
	}
	// Saving as new over an existing file with a zero revision is also stale.
	if _, err := Save(p, d, Revision{}); !errors.Is(err, ErrStale) {
		t.Fatalf("want ErrStale, got %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestSaveRefusesInvalid(t *testing.T) {
	_, d := samurai(t)
	d.Workspaces[0].Tabs[0].Color = "pink"
	p := filepath.Join(t.TempDir(), "workspaces.toml")
	if _, err := Save(p, d, Revision{}); err == nil {
		t.Fatal("want a validation error")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("nothing should be written")
	}
}

func TestSaveFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.toml")
	link := filepath.Join(dir, "workspaces.toml")
	writeFile(t, real, "")
	if err := os.Symlink(real, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need privileges here:", err)
		}
		t.Fatal(err)
	}
	_, d := samurai(t)
	_, rev, err := Load(link)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Save(link, d, rev); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
	if !strings.Contains(string(mustRead(t, real)), "schema_version = 1") {
		t.Fatal("target not updated")
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestEmptyColorDefaultsToCyan(t *testing.T) {
	abs := filepath.ToSlash(t.TempDir())
	p := filepath.Join(t.TempDir(), "workspaces.toml")
	writeFile(t, p, validHead+strings.Replace(tabToml(abs, ""), "color = \"red\"\n", "", 1))
	d, _, err := Load(p)
	if err != nil || d.Workspaces[0].Tabs[0].Color != "cyan" {
		t.Fatalf("%v %+v", err, d)
	}
	_, d2 := samurai(t)
	d2.Workspaces[0].Tabs[0].Color = ""
	p2 := filepath.Join(t.TempDir(), "w.toml")
	if _, err := Save(p2, d2, Revision{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mustRead(t, p2)), `color = "cyan"`) {
		t.Fatal("save should write color = cyan")
	}
}

func TestInvalidFileErrorIsDistinct(t *testing.T) {
	_, d := samurai(t)
	p := filepath.Join(t.TempDir(), "workspaces.toml")
	writeFile(t, p, "schema_version = 2\n")
	_, err := Save(p, d, Revision{})
	if !errors.Is(err, ErrInvalidFile) || errors.Is(err, ErrStale) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "fix it or delete it before saving") {
		t.Fatal(err)
	}
}
