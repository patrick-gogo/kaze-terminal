package ui

import (
	"gioui.org/layout"
	"os"
	"path/filepath"
	"testing"
)

func writeDoc(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(p, []byte(body), 0644); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestOpenDocShowsItUnderItsSession(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	a, _ := win.mgr.New()
	b, _ := win.mgr.New()
	win.mgr.SetActive(b.ID)
	p := writeDoc(t, t.TempDir(), "plan.md", "# Plan\n")
	d := win.openDoc(a.ID, p)
	if d == nil || win.shownDoc() != d || d.session != a.ID || win.mgr.Active().ID != a.ID || win.shownTab() != nil || win.focusTarget() != &win.docs.tag {
		t.Fatal("doc not shown")
	}
	win.openDoc(a.ID, p)
	if len(win.docs.list) != 1 || len(win.mgr.Tabs()) != 2 {
		t.Fatal("duplicate doc or terminal")
	}
	win.selectTab(a.ID)
	if win.shownDoc() != nil || win.shownTab() != a {
		t.Fatal("terminal not shown")
	}
	win.showDoc(d.id)
	win.runShortcut(layout.Context{}, actCloseTab)
	if win.shownDoc() != nil || len(win.docs.list) != 0 || win.mgr.Lookup(a.ID) == nil {
		t.Fatal("close doc")
	}
}
func TestPruneDocsDropsDocsOfGoneSessions(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	a, _ := win.mgr.New()
	b, _ := win.mgr.New()
	win.openDoc(a.ID, writeDoc(t, t.TempDir(), "a.md", "hi"))
	win.mgr.Close(a.ID)
	win.pruneDocs(win.mgr.Tabs())
	if len(win.docs.list) != 0 || win.shownDoc() != nil || win.shownTab() != b {
		t.Fatal("stale docs")
	}
}
func TestSettingsAndBoardHideTheDoc(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	a, _ := win.mgr.New()
	d := win.openDoc(a.ID, writeDoc(t, t.TempDir(), "a.md", "hi"))
	win.openSettings()
	if win.shownDoc() != nil {
		t.Fatal("settings")
	}
	win.showDoc(d.id)
	win.openBoard(0)
	if win.shownDoc() != nil {
		t.Fatal("board")
	}
}
