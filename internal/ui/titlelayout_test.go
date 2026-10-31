package ui

import (
	"image"
	"testing"
	"unicode/utf8"
)

// titleTestDims are titleDims at 1 px per dp.
var titleTestDims = titleDims{BarH: 42, Btn: 30, BtnH: 28, CapW: 46, Mark: 26, Pad: 14, LeadPad: 8, Gap: 8,
	SearchMin: 190, SearchMax: 400, SearchH: 28, KeyPad: 12}

// sevenPx measures every rune as 7 px.
func sevenPx(s string, mono bool) int { return 7 * utf8.RuneCountInString(s) }

var fullCrumb = crumbParts{Repo: "todo-api", Title: "TODO-142", Cmd: "/ship-ticket", State: "running"}

func titleAt(w int, mac bool, side int) titleLayout {
	in := titleIn{Size: image.Pt(w, 42), Mac: mac, SideW: side, Crumb: fullCrumb, Key: "⌘K", Dims: titleTestDims, Measure: sevenPx}
	if mac {
		in.LightsW = 68
	}
	return layoutTitle(in)
}

func TestTitleSameLayoutBothPlatforms(t *testing.T) {
	mac, win := titleAt(1200, true, 248), titleAt(1200+3*46, false, 248)
	for name, pair := range map[string][2]image.Rectangle{
		"Search": {mac.Search, win.Search}, "Scratch": {mac.Scratch, win.Scratch},
		"Agents": {mac.Agents, win.Agents}, "Gear": {mac.Gear, win.Gear}, "Plus": {mac.Plus, win.Plus},
	} {
		if pair[0].Empty() || pair[0] != pair[1] {
			t.Errorf("%s: mac %v windows %v", name, pair[0], pair[1])
		}
	}
	if mac.Mark != (image.Rectangle{}) || win.Mark.Empty() {
		t.Fatal("the K mark is Windows only")
	}
}

func TestTitleLeadMatchesSidebar(t *testing.T) {
	l := titleAt(1200, true, 248)
	if l.Lead.Dx() != 248 || l.Plus.Max.X != 248-8 {
		t.Fatalf("lead %v plus %v", l.Lead, l.Plus)
	}
	h := titleAt(1200, true, 0)
	if !h.Plus.Empty() || h.Lead.Max.X != h.Toggle.Max.X+8 {
		t.Fatalf("hidden sidebar: lead %v toggle %v plus %v", h.Lead, h.Toggle, h.Plus)
	}
	if h.Crumb.Min.X > h.Lead.Max.X+14 {
		t.Fatalf("crumb %v should start right after the lead %v", h.Crumb, h.Lead)
	}
}

func TestTitleMacReservesLights(t *testing.T) {
	l := titleAt(1200, true, 248)
	if l.Toggle.Min.X < 68 {
		t.Fatalf("toggle %v under the lights", l.Toggle)
	}
	in := titleIn{Size: image.Pt(1200, 42), Mac: true, SideW: 248, Crumb: fullCrumb, Key: "⌘K", Dims: titleTestDims, Measure: sevenPx}
	if f := layoutTitle(in); f.Toggle.Min.X != 14 {
		t.Fatalf("full screen toggle %v", f.Toggle)
	}
}

func TestTitleWindowsButtons(t *testing.T) {
	w := titleAt(1200, false, 248)
	for i, b := range w.Buttons {
		if b.Dx() != 46 || b.Max.X != 1200-(2-i)*46 || b.Dy() != 42 {
			t.Fatalf("button %d %v", i, b)
		}
	}
	if w.Gear.Max.X > w.Buttons[0].Min.X {
		t.Fatal("gear over the buttons")
	}
	if m := titleAt(1200, true, 248); m.Buttons != [3]image.Rectangle{} {
		t.Fatal("caption buttons on Mac")
	}
}

func TestTitleSearchClamp(t *testing.T) {
	l := titleAt(1600, true, 248)
	if l.Search.Dx() != 400 {
		t.Fatalf("wide search %v", l.Search)
	}
	left, right := l.Crumb.Max.X+8, l.Scratch.Min.X-8
	if mid := (l.Search.Min.X + l.Search.Max.X) / 2; abs(mid-(left+right)/2) > 1 {
		t.Fatalf("search %v not centred in %d..%d", l.Search, left, right)
	}
	if n := titleAt(800, true, 248); n.Search.Dx() < 190 && !n.SearchIconOnly {
		t.Fatalf("narrow search %v", n.Search)
	}
	if s := titleAt(600, true, 248); !s.SearchIconOnly {
		t.Fatalf("600px search %v should be compact", s.Search)
	}
}

func TestTitleCrumbDropOrder(t *testing.T) {
	prev := titleAt(1600, true, 248).CrumbShow
	if prev != fullCrumb {
		t.Fatalf("wide crumb %+v", prev)
	}
	sawNoCmd, sawNoRepo := false, false
	for w := 1600; w >= 600; w -= 10 {
		l := titleAt(w, true, 248)
		c := l.CrumbShow
		if c.Cmd == "" {
			sawNoCmd = true
		}
		if c.Repo == "" {
			sawNoRepo = true
			if !sawNoCmd {
				t.Fatalf("%dpx: repo dropped before the command", w)
			}
		}
		for name, r := range map[string]image.Rectangle{"toggle": l.Toggle, "plus": l.Plus, "search": l.Search, "scratch": l.Scratch, "agents": l.Agents, "gear": l.Gear} {
			if r.Empty() {
				t.Fatalf("%dpx: %s gone", w, name)
			}
		}
		if l.Crumb.Max.X > l.Search.Min.X || l.Search.Max.X > l.Scratch.Min.X {
			t.Fatalf("%dpx: overlap crumb %v search %v scratch %v", w, l.Crumb, l.Search, l.Scratch)
		}
	}
	if !sawNoCmd || !sawNoRepo {
		t.Fatal("never dropped the command and repo")
	}
}

func TestTitleLongShortcutFits(t *testing.T) {
	in := titleIn{Size: image.Pt(600, 42), Mac: false, SideW: 248, Crumb: fullCrumb, Key: "Ctrl+Shift+Alt+K", Dims: titleTestDims, Measure: sevenPx}
	l := layoutTitle(in)
	if !l.SearchIconOnly || l.SearchKey {
		t.Fatalf("a long key cap should go first: %+v", l)
	}
	if l.Search.Dx() != 30 || l.Search.Min.X < l.Lead.Max.X || l.Search.Max.X > l.Scratch.Min.X {
		t.Fatalf("compact search %v lead %v scratch %v", l.Search, l.Lead, l.Scratch)
	}
}

func TestTitleDragCoversEmptySpace(t *testing.T) {
	for _, mac := range []bool{true, false} {
		l := titleAt(1200, mac, 248)
		buttons := []image.Rectangle{l.Toggle, l.Plus, l.Search, l.Scratch, l.Agents, l.Gear, l.Buttons[0], l.Buttons[1], l.Buttons[2]}
		for x := 0; x < 1200; x++ {
			p := image.Pt(x, 21)
			inBtn, inDrag := false, false
			for _, b := range buttons {
				inBtn = inBtn || p.In(b)
			}
			for _, d := range l.Drag {
				inDrag = inDrag || p.In(d)
			}
			lights := mac && x < 68
			if inBtn && inDrag {
				t.Fatalf("mac=%v x=%d is both a button and drag space", mac, x)
			}
			if !inBtn && !inDrag && !lights {
				// Button rects are shorter than the bar; only full-height gaps count.
				if !nearButton(p, buttons) {
					t.Fatalf("mac=%v x=%d is neither", mac, x)
				}
			}
			if lights && inDrag {
				t.Fatalf("x=%d: the traffic lights are not drag space", x)
			}
		}
	}
}

// nearButton reports whether p is in the column of a button (above or below it).
func nearButton(p image.Point, bs []image.Rectangle) bool {
	for _, b := range bs {
		if !b.Empty() && p.X >= b.Min.X && p.X < b.Max.X {
			return true
		}
	}
	return false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func TestTitleHitLookup(t *testing.T) {
	w := titleAt(1200, false, 248)
	mid := func(r image.Rectangle) image.Point { return r.Min.Add(r.Size().Div(2)) }
	for want, r := range map[captionButton]image.Rectangle{
		capToggle: w.Toggle, capPlus: w.Plus, capSearch: w.Search, capScratch: w.Scratch,
		capAgents: w.Agents, capGear: w.Gear, capMinimize: w.Buttons[0], capMaximize: w.Buttons[1], capClose: w.Buttons[2],
	} {
		if got := w.at(mid(r)); got != want {
			t.Errorf("at %v: %v want %v", r, got, want)
		}
	}
	if got := w.at(mid(w.Crumb)); got != capNone {
		t.Fatalf("crumb is drag space, got %v", got)
	}
}
