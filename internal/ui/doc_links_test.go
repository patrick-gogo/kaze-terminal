package ui

import (
	"gioui.org/io/pointer"
	"image"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocLinks(t *testing.T) {
	win, d := layoutTestDoc(t, "[site](https://example.com/x) [b](b.md) [go](#later)\n\n"+strings.Repeat("text\n\n", 25)+"# Later\n"+strings.Repeat("tail\n\n", 50))
	writeDoc(t, filepath.Dir(d.path), "b.md", "# Part 2\n"+strings.Repeat("text\n\n", 50))
	var opened string
	win.openLink = func(s string) error { opened = s; return nil }
	gtx := pageContext(900, 600)
	r := image.Rect(0, 0, 900, 600)
	win.drawDoc(gtx, d, r)
	if len(win.docs.hits.links) < 3 {
		t.Fatal("missing links")
	}
	hit := win.docs.hits.links[0]
	e := pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary}
	e.Position.X = float32(hit.rect.Min.X + 1)
	e.Position.Y = float32(hit.rect.Min.Y + 1)
	win.docPointer(gtx, e)
	if opened != "https://example.com/x" {
		t.Fatal(opened)
	}
	win.followDocLink(d, "b.md#part-2")
	nd := win.shownDoc()
	if nd == d || nd.session != d.session || len(win.docs.list) != 2 {
		t.Fatal(nd)
	}
	win.drawDoc(gtx, nd, r)
	win.showDoc(d.id)
	win.followDocLink(d, "b.md")
	if len(win.docs.list) != 2 {
		t.Fatal("duplicate")
	}
	win.showDoc(d.id)
	win.followDocLink(d, "#later")
	win.drawDoc(gtx, d, r)
	if d.scroll != min(d.lay.headY[0], win.docs.hits.scrollMax) {
		t.Fatal(d.scroll)
	}
	win.followDocLink(d, "nope.md")
	if win.notice != "Can't find nope.md" {
		t.Fatal(win.notice)
	}
	for _, s := range []string{"mailto:a@b.c", "file:///x.md", "main.go", "javascript:x"} {
		win.followDocLink(d, s)
	}
	if len(win.docs.list) != 2 || opened != "https://example.com/x" {
		t.Fatal("unsafe target")
	}
}
