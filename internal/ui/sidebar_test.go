package ui

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"gioui.org/io/input"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

func sideRows(first, n int, prefix string) []sideRow {
	var rs []sideRow
	for i := 0; i < n; i++ {
		rs = append(rs, sideRow{ID: tabs.TabID(first + i), Title: fmt.Sprintf("%s%d", prefix, i), Folder: "~/code/app", Branch: "main"})
	}
	return rs
}

// sideDraw lays the sidebar out in a 260 x h box with default theme.
func sideDraw(t *testing.T, m sideModel, st sideState, h int) (sideHits, image.Point) {
	t.Helper()
	size := image.Pt(sidebarW, h)
	gtx := pageContext(size.X, size.Y)
	return newChrome().layoutSidebar(gtx, m, st, size), size
}

func twoGroupsOneLoose() sideModel {
	return sideModel{
		Groups: []sideGroup{
			{Key: "a", Name: "alpha", Rows: sideRows(1, 2, "a"), Count: 2},
			{Key: "b", Name: "beta", Rows: sideRows(10, 1, "b"), Count: 1, Waiting: 1},
		},
		Loose: sideRows(20, 1, "loose"),
	}
}

func TestSidebarHitsMatchRows(t *testing.T) {
	h, size := sideDraw(t, twoGroupsOneLoose(), sideState{}, 600)
	if len(h.rows) != 4 || len(h.groups) != 2 {
		t.Fatalf("rows %d groups %d, want 4 and 2", len(h.rows), len(h.groups))
	}
	box := image.Rectangle{Max: size}
	for _, r := range h.rows {
		if !r.rect.In(box) || r.rect.Empty() {
			t.Fatalf("row %d rect %v outside %v", r.id, r.rect, box)
		}
	}
	for _, g := range h.groups {
		if !g.rect.In(box) || g.rect.Empty() {
			t.Fatalf("group %q rect %v outside %v", g.key, g.rect, box)
		}
	}
	if h.filter.Empty() {
		t.Fatalf("filter %v", h.filter)
	}
}

func TestSidebarFoldedGroupHasNoRows(t *testing.T) {
	m := twoGroupsOneLoose()
	m.Groups[0].Folded = true
	h, _ := sideDraw(t, m, sideState{}, 600)
	if len(h.rows) != 2 { // beta's one and the loose one
		t.Fatalf("%d rows, want 2", len(h.rows))
	}
	for _, r := range h.rows {
		if r.id == 1 || r.id == 2 {
			t.Fatalf("folded group's row %d is drawn", r.id)
		}
	}
	if len(h.groups) != 2 || h.groups[0].key != "a" {
		t.Fatalf("groups %+v, want the folded header kept", h.groups)
	}
}

func TestSidebarCloseOnlyWhenHovered(t *testing.T) {
	h, _ := sideDraw(t, twoGroupsOneLoose(), sideState{hover: 1}, 600)
	for _, r := range h.rows {
		if (r.id == 1) == r.close.Empty() {
			t.Fatalf("row %d close %v with hover on 1", r.id, r.close)
		}
		if !r.close.Empty() && !r.close.In(r.rect) {
			t.Fatalf("close %v outside its row %v", r.close, r.rect)
		}
	}
}

func TestSidebarScrollsAndClampsAt(t *testing.T) {
	m := sideModel{Loose: sideRows(1, 40, "r")}
	h, _ := sideDraw(t, m, sideState{}, 600)
	if h.scrollMax <= 0 {
		t.Fatalf("scrollMax %d with 40 rows in 600px", h.scrollMax)
	}
	first := h.rows[0]
	if first.id != 1 {
		t.Fatalf("first visible row %d at scroll 0", first.id)
	}
	for _, r := range h.rows {
		if !r.rect.In(h.list) {
			t.Fatalf("row %d rect %v leaks out of list %v", r.id, r.rect, h.list)
		}
	}
	far, _ := sideDraw(t, m, sideState{scroll: 1 << 20}, 600)
	last := m.Loose[len(m.Loose)-1].ID
	if far.rows[len(far.rows)-1].id != last {
		t.Fatalf("clamped scroll ends at %d, want the last row %d", far.rows[len(far.rows)-1].id, last)
	}
	if far.rows[len(far.rows)-1].rect.Max.Y != far.list.Max.Y {
		t.Fatalf("last row ends at %d, list at %d: not clamped to the content", far.rows[len(far.rows)-1].rect.Max.Y, far.list.Max.Y)
	}
	for _, r := range far.rows {
		if r.id == 1 {
			t.Fatal("a row scrolled above the list is still hit")
		}
		if !r.rect.In(far.list) {
			t.Fatalf("row %d rect %v leaks out of list %v", r.id, r.rect, far.list)
		}
	}
	neg, _ := sideDraw(t, m, sideState{scroll: -50}, 600)
	if neg.rows[0].id != 1 {
		t.Fatal("a negative scroll moved the list")
	}
}

func fourMemberRun() sideRun {
	r := sideRun{Run: 7, Name: "dev stack", Down: 1}
	for i, st := range []string{"Running", "Running", "stopped", "Starting"} {
		r.Members = append(r.Members, sideMember{ID: tabs.TabID(100 + i), Title: fmt.Sprintf("srv%d", i), Color: "green", Status: st, Down: st == "stopped"})
	}
	return r
}

func TestSidebarSectionAtTheBottom(t *testing.T) {
	m := twoGroupsOneLoose()
	m.Runs = []sideRun{fourMemberRun()}
	h, size := sideDraw(t, m, sideState{}, 600)
	if len(h.members) != 4 || len(h.runs) != 1 {
		t.Fatalf("members %d runs %d, want 4 and 1", len(h.members), len(h.runs))
	}
	lastRow := 0
	for _, r := range h.rows {
		lastRow = max(lastRow, r.rect.Max.Y)
	}
	for _, mem := range h.members {
		if mem.rect.Min.Y < lastRow || mem.rect.Max.Y > size.Y {
			t.Fatalf("member %d at %v, rows end at %d, sidebar %d high", mem.id, mem.rect, lastRow, size.Y)
		}
	}
	if h.list.Max.Y > h.members[0].rect.Min.Y {
		t.Fatalf("list %v overlaps the section at %v", h.list, h.members[0].rect)
	}
}

func TestSidebarSettingsRowFirst(t *testing.T) {
	h, _ := sideDraw(t, twoGroupsOneLoose(), sideState{settings: true, settingsOn: true}, 600)
	if h.settings.Empty() || h.settingsClose.Empty() {
		t.Fatalf("settings %v close %v", h.settings, h.settingsClose)
	}
	if h.settings.Max.Y > h.groups[0].rect.Min.Y {
		t.Fatalf("settings %v not above the first group %v", h.settings, h.groups[0].rect)
	}
	none, _ := sideDraw(t, twoGroupsOneLoose(), sideState{}, 600)
	if !none.settings.Empty() {
		t.Fatalf("settings rect %v while closed", none.settings)
	}
}

func TestSidebarLongTitleStaysInside(t *testing.T) {
	m := sideModel{Loose: []sideRow{
		{ID: 1, Title: strings.Repeat("w", 200), Folder: strings.Repeat("~/dir", 40), Branch: strings.Repeat("b", 200)},
		{ID: 2, Title: strings.Repeat("日本語のタイトル", 30), Folder: "~/日本", Branch: "main"},
	}}
	m.Runs = []sideRun{{Run: 1, Name: strings.Repeat("n", 120), Members: []sideMember{{ID: 5, Title: strings.Repeat("長", 100), Status: "Running"}}}}
	h, size := sideDraw(t, m, sideState{hover: 1, filter: strings.Repeat("f", 200)}, 400)
	box := image.Rectangle{Max: size}
	if len(h.rows) != 2 {
		t.Fatalf("%d rows", len(h.rows))
	}
	for _, r := range h.rows {
		if !r.rect.In(box) {
			t.Fatalf("row %d rect %v outside %v", r.id, r.rect, box)
		}
	}
}

func TestSidebarDrawsInEveryTheme(t *testing.T) {
	for _, th := range theme.All() {
		t.Run(th.Key, func(t *testing.T) {
			c := newChrome()
			c.setTheme(th)
			m := twoGroupsOneLoose()
			m.Runs = []sideRun{fourMemberRun()}
			size := image.Pt(sidebarW, 600)
			h := c.layoutSidebar(pageContext(size.X, size.Y), m, sideState{shown: 2, settings: true}, size)
			if len(h.rows) == 0 {
				t.Fatal("nothing drawn")
			}
			if th.UI.Active == th.UI.Chrome {
				t.Fatalf("selected row fill %v equals the sidebar background", th.UI.Active)
			}
		})
	}
}

func TestSidebarFoldedRunHasNoMembers(t *testing.T) {
	m := sideModel{Runs: []sideRun{fourMemberRun()}}
	open, _ := sideDraw(t, m, sideState{}, 600)
	folded, _ := sideDraw(t, m, sideState{runFolded: map[uint64]bool{7: true}}, 600)
	if len(open.members) != 4 || len(folded.members) != 0 {
		t.Fatalf("members open %d folded %d, want 4 and 0", len(open.members), len(folded.members))
	}
	if len(folded.runs) != 1 || folded.runs[0].run != 7 {
		t.Fatalf("folded run header hit %+v", folded.runs)
	}
	if folded.list.Max.Y <= open.list.Max.Y {
		t.Fatalf("folded section did not shrink: list ends %d vs %d", folded.list.Max.Y, open.list.Max.Y)
	}
}

func TestSidebarMemberWithoutShellHasNoHit(t *testing.T) {
	r := fourMemberRun()
	r.Members[1].ID = 0
	h, _ := sideDraw(t, sideModel{Runs: []sideRun{r}}, sideState{}, 600)
	if len(h.members) != 3 {
		t.Fatalf("%d member hits, want 3", len(h.members))
	}
}

func TestSidebarCappedSectionKeepsRoomForMore(t *testing.T) {
	r := sideRun{Run: 1, Name: "big"}
	for i := 0; i < 30; i++ {
		r.Members = append(r.Members, sideMember{ID: tabs.TabID(100 + i), Title: "m", Status: "Running"})
	}
	h, size := sideDraw(t, sideModel{Runs: []sideRun{r}}, sideState{}, 400)
	if len(h.members) == 0 || len(h.members) >= 30 {
		t.Fatalf("%d member hits in a capped section", len(h.members))
	}
	for _, m := range h.members {
		if m.rect.Max.Y > size.Y-24 {
			t.Fatalf("member %v leaves no room for the +N more line", m.rect)
		}
	}
}

// The running ring is drawn still: laying a running row out never asks for another frame.
func TestRunningMarkIsStill(t *testing.T) {
	m := sideModel{Loose: []sideRow{{ID: 1, Title: "V3-1", Command: "/ship-ticket", Running: true}}}
	c := newChrome()
	r := new(input.Router)
	for i := 0; i < 2; i++ {
		gtx := pageContext(sidebarW, 400)
		gtx.Source = r.Source()
		c.layoutSidebar(gtx, m, sideState{}, image.Pt(sidebarW, 400))
		r.Frame(gtx.Ops)
		if _, wake := r.WakeupTime(); wake {
			t.Fatalf("frame %d asked for another frame", i)
		}
	}
}

func TestGroupHeaderDotsDoNotOverlap(t *testing.T) {
	const dot, ring = 7, 2 // dp at scale 1; a running ring reaches ring past its dot, on either neighbour
	rs := headerDots(20, 40, dot, 12)
	if len(rs) != maxStackDots {
		t.Fatalf("%d dots for 12 rows, want the cap %d", len(rs), maxStackDots)
	}
	for i := 1; i < len(rs); i++ {
		if gap := rs[i].Min.X - rs[i-1].Max.X; gap < 2*ring+1 {
			t.Fatalf("dots %d and %d are %d px apart, want room for two rings: %v", i-1, i, gap, rs)
		}
		if rs[i].Dy() != dot || rs[i].Min.Y != rs[0].Min.Y {
			t.Fatalf("dot %d is %v, want one row of %d px dots", i, rs[i], dot)
		}
	}
	if w := rs[len(rs)-1].Max.X - rs[0].Min.X; w > 60 {
		t.Fatalf("the dots take %d px, too wide for the header", w)
	}
}
