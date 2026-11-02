package ui

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestDocReloadsWhenTheFileChanges(t *testing.T) {
	win, d := layoutTestDoc(t, "# Old\n"+strings.Repeat("text\n\n", 100))
	old := d.doc
	now := time.Now().Add(time.Second)
	win.docs.checked = now
	win.docTick(now.Add(time.Second))
	if d.doc != old {
		t.Fatal("unchanged reparsed")
	}
	d.scroll = 200
	os.WriteFile(d.path, []byte("# New\n"+strings.Repeat("text\n\n", 100)), 0644)
	os.Chtimes(d.path, now, now.Add(2*time.Second))
	win.docTick(now.Add(2 * time.Second))
	if d.doc == old || d.doc.Outline[0].Text != "New" || d.scroll != 200 {
		t.Fatal("reload")
	}
	old = d.doc
	stamp := d.file.Stamp
	os.WriteFile(d.path, []byte("# Different size\n"), 0644)
	os.Chtimes(d.path, stamp.Mod, stamp.Mod)
	win.docTick(now.Add(3 * time.Second))
	if d.doc == old {
		t.Fatal("size change")
	}
}
func TestDocComesBackAfterAReplace(t *testing.T) {
	win, d := layoutTestDoc(t, "# Old")
	old := d.doc
	now := time.Now()
	os.Remove(d.path)
	win.refreshDoc(d, now)
	if !d.removed || d.doc != old {
		t.Fatal("removed content lost")
	}
	os.WriteFile(d.path, []byte("# Back"), 0644)
	win.refreshDoc(d, now)
	if d.removed || d.doc.Outline[0].Text != "Back" {
		t.Fatal("not back")
	}
}
func TestDocTimerOnlyWhileShown(t *testing.T) {
	win, d := layoutTestDoc(t, "# A")
	now := time.Now()
	at, ok := win.docTick(now)
	if !ok || at != win.docs.checked.Add(time.Second) {
		t.Fatal("timer")
	}
	win.hideDoc()
	if _, ok := win.docTick(now); ok {
		t.Fatal("hidden timer")
	}
	os.WriteFile(d.path, []byte("# Longer heading"), 0644)
	win.showDoc(d.id)
	if d.doc.Outline[0].Text != "Longer heading" {
		t.Fatal("show reload")
	}
	os.WriteFile(d.path, []byte("# Focus"), 0644)
	win.setFocused(false)
	win.setFocused(true)
	if d.doc.Outline[0].Text != "Focus" {
		t.Fatal("focus reload")
	}
}
