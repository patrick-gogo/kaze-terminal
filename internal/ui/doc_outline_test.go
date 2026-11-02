package ui

import (
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"image"
	"strings"
	"testing"
)

func TestDocKeysScroll(t *testing.T) {
	win, d := layoutTestDoc(t, "# A\n"+strings.Repeat("paragraph\n\n", 150))
	gtx := pageContext(900, 600)
	win.drawDoc(gtx, d, image.Rect(0, 0, 900, 600))
	win.docKey(gtx, key.Event{Name: key.NameDownArrow, State: key.Press})
	if d.scroll != 40 {
		t.Fatal(d.scroll)
	}
	win.docKey(gtx, key.Event{Name: key.NameEnd, State: key.Press})
	if d.scroll != win.docs.hits.scrollMax {
		t.Fatal(d.scroll)
	}
	win.docKey(gtx, key.Event{Name: key.NameHome, State: key.Press})
	if d.scroll != 0 {
		t.Fatal(d.scroll)
	}
}
func TestOutlineClickScrollsToHeading(t *testing.T) {
	win, d := layoutTestDoc(t, "# A\n"+strings.Repeat("text\n\n", 20)+"## B\n"+strings.Repeat("last\n\n", 50))
	gtx := pageContext(900, 600)
	win.drawDoc(gtx, d, image.Rect(0, 0, 900, 600))
	h := win.docs.hits.heads[1]
	e := pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary}
	e.Position.X = float32(h.rect.Min.X + 5)
	e.Position.Y = float32(h.rect.Min.Y + 5)
	win.docPointer(gtx, e)
	if d.scroll != d.lay.headY[1]-12 {
		t.Fatal(d.scroll, d.lay.headY)
	}
	if win.docCurrent(gtx, d) != 1 {
		t.Fatal("highlight")
	}
	win.drawDoc(pageContext(500, 600), d, image.Rect(0, 0, 500, 600))
	if !win.docs.hits.outline.Empty() {
		t.Fatal("narrow outline")
	}
}
func TestDocViewTakesKeysInAFrame(t *testing.T) {
	win, st := sideWindow(t, false)
	a, _ := win.mgr.New()
	d := win.openDoc(a.ID, writeDoc(t, t.TempDir(), "a.md", strings.Repeat("text\n\n", 100)))
	var r input.Router
	sizedFrame(t, win, &r, 1000, 600)
	r.Queue(key.Event{Name: key.NameDownArrow, State: key.Press}, key.EditEvent{Text: "a"})
	sizedFrame(t, win, &r, 1000, 600)
	if d.scroll != 40 || st.ptys[0].sent() != "" {
		t.Fatal(d.scroll, st.ptys[0].sent())
	}
}
