package ui

import (
	"gioui.org/io/pointer"
	"image"
	"testing"
)

func TestDocRowClickShowsTheDoc(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	a, _ := win.mgr.New()
	d := win.openDoc(a.ID, writeDoc(t, t.TempDir(), "plan.md", "hi"))
	win.selectTab(a.ID)
	in := win.sideIn()
	in.Filter = "plan"
	m := buildSide(in)
	if len(sessionOrder(m)) != 1 {
		t.Fatal("filter lost doc")
	}
	win.drawSide(pageContext(260, 600), image.Rect(0, 0, 260, 600))
	if len(win.side.hits.docs) != 1 {
		t.Fatal("missing doc line")
	}
	hit := win.side.hits.docs[0]
	p := hit.rect.Min.Add(image.Pt(5, 5))
	win.sidePress(p, pointer.ButtonPrimary, 0)
	if win.shownDoc() != d {
		t.Fatal("not shown")
	}
	win.sidePress(p, pointer.ButtonTertiary, 0)
	if len(win.docs.list) != 0 || win.mgr.Lookup(a.ID) == nil {
		t.Fatal("closed session")
	}
}
func TestDocRowHoverRedrawsOnlyOnChange(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	a, _ := win.mgr.New()
	win.openDoc(a.ID, writeDoc(t, t.TempDir(), "plan.md", "hi"))
	win.drawSide(pageContext(260, 600), image.Rect(0, 0, 260, 600))
	h := win.side.hits.docs[0]
	e := pointer.Event{Kind: pointer.Move}
	e.Position.X = float32(h.rect.Min.X + 3)
	e.Position.Y = float32(h.rect.Min.Y + 3)
	before := win.side.redraws
	win.sidePointer(e)
	win.sidePointer(e)
	if win.side.redraws != before+1 {
		t.Fatal("hover redraws", win.side.redraws-before)
	}
}
