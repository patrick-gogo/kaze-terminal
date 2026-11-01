package render

import (
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

// drawnCell is the cell the strips show at col, row.
func (h *harness) drawnCell(col, row int) screen.Cell { return h.r.drawn[row*h.r.cols+col] }

func isLink(c screen.Cell, fg screen.Color) bool {
	return c.Attrs&screen.AttrUnderline != 0 && c.FG == fg
}

func TestLinkTextIsBlueAndUnderlined(t *testing.T) {
	h := newHarness(t, 60, 4)
	h.frame()
	h.out("see https://example.com/x ok")
	h.frame()
	for col := 4; col <= 24; col++ {
		if c := h.drawnCell(col, 0); !isLink(c, linkFG) {
			t.Fatalf("col %d (%q) not drawn as a link: %+v", col, c.Rune, c)
		}
	}
	for _, col := range []int{0, 2, 26} {
		if c := h.drawnCell(col, 0); c.Attrs&screen.AttrUnderline != 0 || !c.FG.IsDefault() {
			t.Fatalf("col %d (%q) styled as a link: %+v", col, c.Rune, c)
		}
	}
}

// A program's own colour stays; the link only gains the underline.
func TestColoredLinkKeepsItsColor(t *testing.T) {
	h := newHarness(t, 60, 4)
	h.frame()
	h.out("\x1b[31mhttps://example.com\x1b[0m")
	h.frame()
	if c := h.drawnCell(3, 0); !isLink(c, screen.Indexed(1)) {
		t.Fatalf("got %+v", c)
	}
}

// A wrapped link is styled on both rows, even when only the second row changed.
func TestWrappedLinkStyledOnBothRows(t *testing.T) {
	h := newHarness(t, 20, 4)
	h.frame()
	h.out("https://example.com/")
	h.frame()
	h.out("abcdef")
	h.frame()
	if c := h.drawnCell(19, 0); !isLink(c, linkFG) {
		t.Fatalf("row 0 end: %+v", c)
	}
	if c := h.drawnCell(5, 1); !isLink(c, linkFG) || c.Rune != 'f' {
		t.Fatalf("row 1: %+v", c)
	}
}

// Text that stops being a link loses the style.
func TestLinkStyleGoesWithTheLink(t *testing.T) {
	h := newHarness(t, 40, 4)
	h.frame()
	h.out("https://example.com")
	h.frame()
	h.out("\r\x1b[2Khttps:/nope")
	h.frame()
	if c := h.drawnCell(2, 0); c.Attrs&screen.AttrUnderline != 0 || !c.FG.IsDefault() {
		t.Fatalf("still styled: %+v", c)
	}
}

func TestIdleFrameWithLinksRedrawsNothing(t *testing.T) {
	h := newHarness(t, 40, 4)
	h.frame()
	h.out("https://example.com")
	h.frame()
	if n := h.frame(); n != 0 {
		t.Fatalf("idle frame redrew %d rows", n)
	}
}
