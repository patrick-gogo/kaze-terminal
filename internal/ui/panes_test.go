package ui

import (
	"image"
	"slices"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func paneIDs(ps []*pane) []tabs.TabID {
	var out []tabs.TabID
	for _, p := range ps {
		out = append(out, p.id)
	}
	return out
}

func TestOpenCascadesMemberPanes(t *testing.T) {
	var f floats
	var z uint64
	for i, id := range []tabs.TabID{11, 12, 13} {
		p, dropped := f.open(1, id, paneMember)
		want := image.Pt(80+30*i, 40+30*i)
		if dropped != 0 || p.r.Min != want || p.r.Size() != memberSizeDp {
			t.Fatalf("pane %d: %v dropped %d, want at %v", i, p.r, dropped, want)
		}
		if p.z <= z {
			t.Fatalf("z %d not above %d", p.z, z)
		}
		z = p.z
	}
}

func TestOpenAgainRaisesAndUnhides(t *testing.T) {
	var f floats
	a, _ := f.open(1, 11, paneMember)
	b, _ := f.open(1, 12, paneMember)
	a.hidden = true
	if again, _ := f.open(1, 11, paneMember); again != a || a.hidden || a.z <= b.z {
		t.Fatalf("again %p a %+v b.z %d", again, a, b.z)
	}
	if n := len(f.set(1).panes); n != 2 {
		t.Fatalf("%d panes", n)
	}
}

func TestFifthPaneReplacesTheOldestMember(t *testing.T) {
	var f floats
	f.open(1, 99, paneScratch)
	var dropped tabs.TabID
	for _, id := range []tabs.TabID{11, 12, 13, 14} {
		_, dropped = f.open(1, id, paneMember)
	}
	s := f.set(1)
	if dropped != 11 || len(s.panes) != maxPanes || s.scratch() == nil || s.find(11) != nil {
		t.Fatalf("dropped %d, panes %v", dropped, paneIDs(s.panes))
	}
}

func TestScratchIsNeverReplaced(t *testing.T) {
	var f floats
	f.open(1, 11, paneMember)
	sc, _ := f.open(1, 99, paneScratch)
	sc.hidden = true
	var dropped tabs.TabID
	for _, id := range []tabs.TabID{12, 13, 14} {
		_, dropped = f.open(1, id, paneMember)
	}
	s := f.set(1)
	if dropped != 11 || len(s.panes) != maxPanes || s.scratch() != sc {
		t.Fatalf("dropped %d, panes %v", dropped, paneIDs(s.panes))
	}
}

func TestMemberFloatsInOneSessionAtATime(t *testing.T) {
	var f floats
	f.open(1, 11, paneMember)
	f.open(2, 11, paneMember)
	if f.owner(11) != 2 || f.set(1).find(11) != nil {
		t.Fatalf("owner %d", f.owner(11))
	}
}

func TestVisibleSkipsHiddenInZOrder(t *testing.T) {
	var f floats
	a, _ := f.open(1, 11, paneMember)
	b, _ := f.open(1, 12, paneMember)
	sc, _ := f.open(1, 99, paneScratch)
	sc.hidden = true
	f.raise(1, 12)
	f.raise(1, 11)
	s := f.set(1)
	if got := paneIDs(s.visible()); !slices.Equal(got, []tabs.TabID{12, 11}) {
		t.Fatalf("visible %v", got)
	}
	if s.front() != a || b.z >= a.z {
		t.Fatalf("front %v", s.front())
	}
	if (&paneSet{}).front() != nil {
		t.Fatal("an empty set has a front pane")
	}
}

func TestRemoveClearsTheKeyboard(t *testing.T) {
	var f floats
	f.open(1, 11, paneMember)
	f.keys = 11
	if s, ok := f.remove(11); !ok || s != 1 || f.keys != 0 {
		t.Fatalf("remove: %d %v keys %d", s, ok, f.keys)
	}
	if _, ok := f.remove(11); ok {
		t.Fatal("removed twice")
	}
}

func TestPruneDropsGoneTabsAndReturnsOrphanScratch(t *testing.T) {
	var f floats
	f.open(1, 11, paneMember)
	f.open(1, 99, paneScratch)
	f.open(2, 12, paneMember)
	f.open(2, 13, paneMember)
	f.keys = 13
	alive := map[tabs.TabID]bool{2: true, 11: true, 99: true, 12: true}
	orphans, released := f.prune(func(id tabs.TabID) bool { return alive[id] })
	if !slices.Equal(orphans, []tabs.TabID{99}) || !slices.Equal(released, []tabs.TabID{11}) {
		t.Fatalf("orphans %v released %v", orphans, released)
	}
	if f.sets[1] != nil || f.set(2).find(13) != nil || f.set(2).find(12) == nil || f.keys != 0 {
		t.Fatalf("sets %v keys %d", f.sets, f.keys)
	}
}

func TestFitPaneShrinksAndMoves(t *testing.T) {
	area := image.Rect(0, 0, 800, 500)
	got := fitPane(image.Rect(500, 400, 1100, 700), area, paneMinDp)
	if got != image.Rect(200, 200, 800, 500) {
		t.Fatalf("got %v", got)
	}
	small := image.Rect(10, 20, 410, 220)
	if got := fitPane(image.Rect(500, 400, 1100, 700), small, paneMinDp); got != small {
		t.Fatalf("small area: got %v", got)
	}
	if got := fitPane(image.Rect(-50, -40, 550, 260), area, paneMinDp); got.Min != (image.Point{}) {
		t.Fatalf("above-left: got %v", got)
	}
}

func TestFitPaneKeepsAFittingPane(t *testing.T) {
	r := image.Rect(80, 40, 640, 300)
	if got := fitPane(r, image.Rect(0, 0, 800, 500), paneMinDp); got != r {
		t.Fatalf("got %v", got)
	}
}

func TestEdgeAtZones(t *testing.T) {
	r := image.Rect(100, 100, 500, 300)
	cases := []struct {
		p  image.Point
		e  edge
		ok bool
	}{
		{image.Pt(102, 102), edgeN | edgeW, true},
		{image.Pt(110, 290), edgeS | edgeW, true},
		{image.Pt(495, 295), edgeS | edgeE, true},
		{image.Pt(300, 102), edgeN, true},
		{image.Pt(498, 200), edgeE, true},
		{image.Pt(300, 297), edgeS, true},
		{image.Pt(101, 200), edgeW, true},
		{image.Pt(300, 200), 0, true},
		{image.Pt(50, 50), 0, false},
	}
	for _, c := range cases {
		if e, ok := edgeAt(r, c.p, 6, 14); e != c.e || ok != c.ok {
			t.Errorf("%v: got %v %v, want %v %v", c.p, e, ok, c.e, c.ok)
		}
	}
}

func TestDragRectMovesAndResizes(t *testing.T) {
	r := image.Rect(100, 100, 500, 300)
	if got := dragRect(r, 0, image.Pt(10, 5), paneMinDp); got != image.Rect(110, 105, 510, 305) {
		t.Fatalf("move: %v", got)
	}
	if got := dragRect(r, edgeS|edgeE, image.Pt(50, 20), paneMinDp); got != image.Rect(100, 100, 550, 320) {
		t.Fatalf("grow: %v", got)
	}
	if got := dragRect(r, edgeW, image.Pt(1000, 0), paneMinDp); got.Dx() != paneMinDp.X || got.Max.X != r.Max.X {
		t.Fatalf("west past the minimum: %v", got)
	}
	if got := dragRect(r, edgeN, image.Pt(0, 1000), paneMinDp); got.Dy() != paneMinDp.Y || got.Max.Y != r.Max.Y {
		t.Fatalf("north past the minimum: %v", got)
	}
	if got := dragRect(r, edgeE|edgeS, image.Pt(-1000, -1000), paneMinDp); got.Size() != paneMinDp || got.Min != r.Min {
		t.Fatalf("shrink past the minimum: %v", got)
	}
}
