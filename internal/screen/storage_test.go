package screen

import (
	"slices"
	"testing"
)

func TestDirtyRowsReuseDestination(t *testing.T) {
	s := New(120, 60)
	dst := make([]int, 0, 60)
	n := testing.AllocsPerRun(100, func() { s.MarkAllDirty(); dst = s.TakeDirtyInto(dst[:0]) })
	if n != 0 || len(dst) != 60 || dst[0] != 0 || dst[59] != 59 {
		t.Fatalf("dirty rows: allocs=%v rows=%v", n, dst)
	}
	if got := s.TakeDirtyInto(dst[:0]); len(got) != 0 {
		t.Fatal("flags not cleared")
	}
}

func TestScrollMovesRowStorage(t *testing.T) {
	s := New(8, 4)
	for r := 0; r < 4; r++ {
		s.SetCell(0, r, Cell{Rune: rune('A' + r), Width: 1})
	}
	first, third := &s.row(0)[0], &s.row(2)[0]
	s.ScrollUp(2)
	if &s.row(0)[0] != third || &s.row(2)[0] != first {
		t.Fatal("scroll copied cells instead of reusing row storage")
	}
	if s.Cell(0, 0).Rune != 'C' || s.Cell(0, 1).Rune != 'D' || s.Cell(0, 2) != blankCell {
		t.Fatal("scroll content changed")
	}
	if s.ScrollbackLine(0)[0].Rune != 'A' || s.ScrollbackLine(1)[0].Rune != 'B' {
		t.Fatal("history lost logical row order")
	}
	s.UseAltScreen(true)
	s.SetCell(0, 0, Cell{Rune: 'Z', Width: 1})
	s.ScrollUp(1)
	s.UseAltScreen(false)
	if s.Cell(0, 0).Rune != 'C' {
		t.Fatal("alternate scroll changed main rows")
	}
	s.Resize(10, 5)
	if s.Cell(0, 0).Rune != 'C' || s.Cell(0, 1).Rune != 'D' {
		t.Fatal("resize copied physical instead of logical rows")
	}
}

func TestRowRotationMatchesRegionCopies(t *testing.T) {
	s := New(7, 6)
	want := make([][]Cell, 6)
	for r := range want {
		want[r] = make([]Cell, 7)
		for c := range want[r] {
			want[r][c] = Cell{Rune: rune('A' + r*7 + c), Width: 1}
			s.SetCell(c, r, want[r][c])
		}
	}
	for i := 0; i < 80; i++ {
		top, bottom := i%3, 3+i%3
		n := 1 + i%3
		n = min(n, bottom-top+1)
		old := make([][]Cell, len(want))
		for r := range want {
			old[r] = slices.Clone(want[r])
		}
		delta := n
		if i%2 == 0 {
			delta = -n
		}
		s.shiftRows(top, bottom, delta)
		for r := top; r <= bottom; r++ {
			from := r - delta
			if from >= top && from <= bottom {
				copy(want[r], old[from])
			} else {
				for c := range want[r] {
					want[r][c] = blankCell
				}
			}
		}
		for r := range want {
			for c, v := range want[r] {
				if s.Cell(c, r) != v {
					t.Fatalf("iteration %d row %d col %d", i, r, c)
				}
			}
		}
	}
}

func BenchmarkDirtyRows(b *testing.B) {
	s := New(120, 60)
	dst := make([]int, 0, 60)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		s.MarkAllDirty()
		dst = s.TakeDirtyInto(dst[:0])
	}
}
