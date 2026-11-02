package ui

import (
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"path/filepath"
	"testing"
	"time"
)

func TestShortcutClickOpensADocPath(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	var opened []string
	win.openLink = func(u string) error { opened = append(opened, u); return nil }
	dir := t.TempDir()
	want := writeDoc(t, dir, filepath.Join("docs", "plan.md"), "# Plan\n")
	tab, _ := shellTab(t, win, st, "wrote docs/plan.md ok", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 20, 'k')()
	})
	report(t, tab, dir)
	r := &pointerRun{t: t, win: win, tab: tab}
	r.send(pointer.Press, pointer.ButtonPrimary, 0, 8, 0)
	r.send(pointer.Release, 0, 0, 8, 0)
	if win.shownDoc() != nil {
		t.Fatal("a plain click opened the doc")
	}
	r.at += time.Second
	r.send(pointer.Press, pointer.ButtonPrimary, key.ModShortcut, 8, 0)
	r.send(pointer.Release, 0, key.ModShortcut, 8, 0)
	d := win.shownDoc()
	if d == nil || d.path != want || d.session != tab.ID {
		t.Fatalf("doc %+v, want %s under %d", d, want, tab.ID)
	}
	if len(opened) != 0 {
		t.Fatalf("a doc path went to the OS: %v", opened)
	}
}
func TestOpenNeverHandsANonWebURLToTheOS(t *testing.T) {
	win := testWindow(t, &recStarter{})
	var got []string
	win.openLink = func(s string) error { got = append(got, s); return nil }
	for _, s := range []string{"file:///etc/passwd", "docs/a.md", "javascript:alert(1)", `C:\x.md`} {
		win.open(s)
	}
	if len(got) != 0 {
		t.Fatal(got)
	}
	win.open("https://a.io")
	if len(got) != 1 {
		t.Fatal(got)
	}
}
func TestDocPathFallsBackToTheRepoRoot(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	a, _ := win.mgr.New()
	root := t.TempDir()
	p := writeDoc(t, root, "docs/a.md", "hi")
	a.Dir = filepath.Join(root, "sub")
	win.topFn = func(string) string { return root }
	win.openDocRef(a, false, "docs/a.md")
	if d := win.shownDoc(); d == nil || d.path != p {
		t.Fatal(d)
	}
	win.selectTab(a.ID)
	win.openDocRef(a, false, "nope.md")
	if win.notice != "Can't find nope.md" {
		t.Fatal(win.notice)
	}
}
