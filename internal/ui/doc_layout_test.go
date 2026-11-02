package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/mdoc"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
	"image"
	"strings"
	"testing"
	"time"
)

func layoutTestDoc(t *testing.T, body string) (*window, *docView) {
	t.Helper()
	win := testWindow(t, &recStarter{})
	win.ui = newChrome()
	win.applyTheme(theme.Default())
	a, _ := win.mgr.New()
	return win, win.openDoc(a.ID, writeDoc(t, t.TempDir(), "a.md", body))
}
func TestDocLayoutWrapsWithinTheColumn(t *testing.T) {
	for _, body := range []string{strings.Repeat("word ", 200), strings.Repeat("a", 300), "```\n" + strings.Repeat("a", 500) + "\n```", "| a | b |\n|---|---|\n|" + strings.Repeat("long ", 40) + "|" + strings.Repeat("cell ", 40) + "|"} {
		win, d := layoutTestDoc(t, body)
		l := win.layoutDoc(pageContext(900, 600), d, 400)
		if len(l.lines) < 3 {
			t.Fatal("not wrapped")
		}
		for _, line := range l.lines {
			for _, r := range line.runs {
				if r.x+r.w > 400 {
					t.Fatal("overflow", r)
				}
			}
		}
		win.layoutDoc(pageContext(900, 600), d, 400)
		if d.layouts != 1 {
			t.Fatal("cache")
		}
		win.layoutDoc(pageContext(900, 600), d, 401)
		if d.layouts != 2 {
			t.Fatal("width")
		}
	}
}
func TestDocLayoutHeadingPositions(t *testing.T) {
	win, d := layoutTestDoc(t, "# A\ntext\n## B\nmore\n### C")
	l := win.layoutDoc(pageContext(900, 600), d, 400)
	if len(l.headY) != len(d.doc.Outline) {
		t.Fatal(l.headY)
	}
	for i := 1; i < len(l.headY); i++ {
		if l.headY[i] <= l.headY[i-1] {
			t.Fatal(l.headY)
		}
	}
}
func TestDocLayoutOfAHugeLineIsBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("large layout")
	}
	win, d := layoutTestDoc(t, "x")
	d.doc = mdoc.Parse([]byte(strings.Repeat("a", 2<<20)))
	done := make(chan bool, 1)
	go func() { win.layoutDoc(pageContext(900, 600), d, 400); done <- true }()
	select {
	case <-done:
	case <-time.After(20 * time.Second):
		t.Fatal("layout deadline")
	}
}
func TestDrawDocInEveryTheme(t *testing.T) {
	win, d := layoutTestDoc(t, "# A\nHello [world](https://example.com)")
	for _, th := range theme.All() {
		win.applyTheme(th)
		win.drawDoc(pageContext(900, 600), d, image.Rect(0, 0, 900, 600))
		if len(win.docs.texts) == 0 {
			t.Fatal("empty")
		}
		if win.docColor(roleBody) != win.pal.FG || win.docColor(roleLink) != win.pal.ANSI[4] {
			t.Fatal("colors")
		}
	}
	if d.layouts != 1 {
		t.Fatal("theme relayout", d.layouts)
	}
}
