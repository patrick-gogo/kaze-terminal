package screen_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

func scrollOff(s *screen.Screen, text string) {
	for _, r := range text {
		s.Print(r)
	}
	s.CarriageReturn()
	s.LineFeed()
}

// A history line keeps its width, but the blank cells after its text are not stored.
func TestHistoryLineDropsItsBlankTailButKeepsItsWidth(t *testing.T) {
	s := screen.NewWithScrollback(200, 1, 10)
	scrollOff(s, "hi")
	if n := len(s.ScrollbackLine(0)); n != 2 {
		t.Fatalf("history line stores %d cells, want 2 (blank tail dropped)", n)
	}
	if w := s.ScrollbackWidth(0); w != 200 {
		t.Fatalf("history width %d, want 200", w)
	}
	s.SetViewOffset(1)
	if c := s.VisibleCell(150, 0); c.Rune != ' ' || !c.FG.IsDefault() || !c.BG.IsDefault() {
		t.Fatalf("cell past the stored text = %+v, want a default blank", c)
	}
}

// Colored blanks are content (a status bar's background), so they stay.
func TestHistoryKeepsColoredBlanks(t *testing.T) {
	s := screen.NewWithScrollback(10, 1, 10)
	s.Pen().BG = screen.Indexed(4)
	s.EraseInLine(2) // fills the line with the pen's background
	s.Pen().BG = screen.DefaultColor
	s.Print('x')
	s.CarriageReturn()
	s.LineFeed()
	line := s.ScrollbackLine(0)
	if len(line) != 10 || line[9].BG != screen.Indexed(4) {
		t.Fatalf("colored tail lost: %d cells", len(line))
	}
}

// 10,000 short lines on a wide screen must not cost a full row of cells each.
func TestShortHistoryLinesStayCheapOnAWideScreen(t *testing.T) {
	const lines, cols = 10000, 240
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	s := screen.NewWithScrollback(cols, 1, lines)
	for i := 0; i < lines; i++ {
		scrollOff(s, "short line of output")
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	grew := float64(after.HeapAlloc-before.HeapAlloc) / (1 << 20)
	runtime.KeepAlive(s)
	if grew > 8 {
		t.Fatalf("10,000 short history lines hold %.1f MiB, want under 8 (a full 240-cell row each is %.1f)", grew, float64(lines*cols*16)/(1<<20))
	}
}

// A full ring reuses evicted lines' storage, but a short line must not keep a
// long line's allocation: history that once held long lines would stay large.
func TestShortLinesReplacingLongOnesFreeTheirStorage(t *testing.T) {
	const lines, cols = 10000, 240
	s := screen.NewWithScrollback(cols, 1, lines)
	long := strings.Repeat("x", cols-1)
	for i := 0; i < lines; i++ {
		scrollOff(s, long)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := 0; i < lines; i++ {
		scrollOff(s, "short")
	}
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(s)
	held := float64(after.HeapAlloc) / (1 << 20)
	if shrunk := float64(int64(before.HeapAlloc)-int64(after.HeapAlloc)) / (1 << 20); shrunk < 25 {
		t.Fatalf("replacing 10,000 long lines with short ones freed %.1f MiB (heap now %.1f), want at least 25", shrunk, held)
	}
}
