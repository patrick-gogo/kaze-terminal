package ui

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"

	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

func palDraw(t *testing.T, c *chrome, items []palItem, main image.Rectangle, scale float32) palHits {
	t.Helper()
	gtx := pageContext(main.Max.X+40, main.Max.Y+40)
	gtx.Metric = unit.Metric{PxPerDp: scale, PxPerSp: scale}
	var ed widget.Editor
	ed.SingleLine = true
	return c.layoutPalette(gtx, palState{items: items, ed: &ed, hover: -1}, main)
}

func skillItems(n int) []palItem {
	var out []palItem
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("/skill-%03d", i)
		out = append(out, palItem{Section: palSkills, Key: "skill:" + name, Name: name, Detail: "does a thing", Prompt: name})
	}
	return out
}

func TestPaletteBoxIsCentered(t *testing.T) {
	c := newChrome()
	for _, scale := range []float32{1, 2} {
		main := image.Rect(260, 38, 1460, 838)
		if scale == 2 {
			main = image.Rect(520, 76, 2920, 1676)
		}
		h := palDraw(t, c, skillItems(3), main, scale)
		w := int(560 * scale)
		if h.box.Dx() != w {
			t.Errorf("scale %v: box %d wide, want %d", scale, h.box.Dx(), w)
		}
		if mid, want := (h.box.Min.X+h.box.Max.X)/2, (main.Min.X+main.Max.X)/2; mid < want-1 || mid > want+1 {
			t.Errorf("scale %v: box centred at %d, want %d", scale, mid, want)
		}
		if top := main.Min.Y + int(60*scale); h.box.Min.Y != top {
			t.Errorf("scale %v: box top %d, want %d", scale, h.box.Min.Y, top)
		}
		if !h.field.In(h.box) || !h.list.In(h.box) {
			t.Errorf("field %v or list %v outside the box %v", h.field, h.list, h.box)
		}
	}
}

func TestPaletteNarrowMainView(t *testing.T) {
	main := image.Rect(100, 38, 400, 300)
	h := palDraw(t, newChrome(), skillItems(40), main, 1)
	if !h.box.In(main) {
		t.Fatalf("box %v outside the main view %v", h.box, main)
	}
	tiny := image.Rect(0, 0, 20, 20)
	palDraw(t, newChrome(), skillItems(3), tiny, 1) // no panic
}

func TestPaletteRowsAndHeaders(t *testing.T) {
	items := []palItem{
		{Section: palSessions, Key: "tab:1", Name: "api", Detail: "running · go to session", Running: true},
		{Section: palSessions, Key: "tab:2", Name: "misc", Command: "/ship", Detail: "shell · go to session"},
	}
	items = append(items, skillItems(3)...)
	h := palDraw(t, newChrome(), items, image.Rect(260, 38, 1460, 838), 1)
	if len(h.rows) != 5 {
		t.Fatalf("%d rows", len(h.rows))
	}
	if !slices.Equal(h.headers, []string{"SESSIONS", "SKILLS"}) {
		t.Fatalf("headers %v", h.headers)
	}
	for i, r := range h.rows {
		if r.index != i || !r.rect.In(h.list) {
			t.Errorf("row %d: %+v outside %v", i, r, h.list)
		}
	}
}

func TestPaletteListScrolls(t *testing.T) {
	c := newChrome()
	main := image.Rect(260, 38, 1460, 838)
	h := palDraw(t, c, skillItems(200), main, 1)
	if h.scrollMax <= 0 || h.list.Dy() > 380 {
		t.Fatalf("scrollMax %d, list %d high", h.scrollMax, h.list.Dy())
	}
	if len(h.rows) > 380/28+2 {
		t.Fatalf("%d rows laid out for a 380dp list", len(h.rows))
	}
	gtx := pageContext(1500, 900)
	var ed widget.Editor
	h = c.layoutPalette(gtx, palState{items: skillItems(200), ed: &ed, hover: -1, scroll: 1 << 20}, main)
	if len(h.rows) == 0 || h.rows[len(h.rows)-1].index != 199 {
		t.Fatalf("scrolled to the end: last row %+v", h.rows)
	}
	for _, r := range h.rows {
		if r.rect.Max.Y <= h.list.Min.Y || r.rect.Min.Y >= h.list.Max.Y {
			t.Fatalf("row %d laid out outside the list", r.index)
		}
	}
}

func TestPaletteLongTextStaysInside(t *testing.T) {
	items := []palItem{
		{Section: palSkills, Key: "skill:x", Name: "/" + strings.Repeat("長い名前", 30), Detail: strings.Repeat("description ", 30)},
		{Section: palSessions, Key: "tab:1", Name: strings.Repeat("title", 40), Command: strings.Repeat("/cmd", 30), Detail: "shell · go to session"},
	}
	h := palDraw(t, newChrome(), items, image.Rect(0, 38, 700, 700), 1)
	for _, r := range h.rows {
		if !r.rect.In(h.box) {
			t.Fatalf("row %v outside the box %v", r.rect, h.box)
		}
	}
}

func TestPaletteNoMatchesLine(t *testing.T) {
	h := palDraw(t, newChrome(), nil, image.Rect(260, 38, 1460, 838), 1)
	if len(h.rows) != 0 || !slices.Equal(h.headers, []string{"NO MATCHES"}) {
		t.Fatalf("rows %d, headers %v", len(h.rows), h.headers)
	}
}

func TestPaletteDrawsInEveryTheme(t *testing.T) {
	win, _ := alertWindow(t, 1)
	items := append([]palItem{
		{Section: palWaiting, Key: "tab:1", Name: "a", Mark: markNeeds, Detail: "needs you · go to session"},
		{Section: palSessions, Key: "tab:2", Name: "web", Member: true, Color: "cyan", Detail: "server · running"},
	}, skillItems(2)...)
	for _, th := range theme.All() {
		win.applyTheme(th)
		if th.UI.Active == th.UI.ToastBG {
			t.Errorf("%s: the selected row would not show", th.Key)
		}
		palDraw(t, win.ui, items, image.Rect(260, 38, 1460, 838), 1)
	}
}
