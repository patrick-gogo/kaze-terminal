package render

import (
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

// Point is a grid position. Row counts from the oldest scrollback line, so a
// selection keeps pointing at the same text while the view scrolls.
type Point struct{ Col, Row int }

// Selection is a mouse selection on the grid. Active means a selection exists
// (including one still being dragged); an inactive Selection selects nothing.
type Selection struct {
	Start, End Point
	Active     bool
}

// before reports whether p sorts before b in reading order.
func (p Point) before(b Point) bool {
	return p.Row < b.Row || (p.Row == b.Row && p.Col < b.Col)
}

// bounds returns the ends in reading order; dragging up or left leaves Start after End.
func (s Selection) bounds() (lo, hi Point) {
	if s.End.before(s.Start) {
		return s.End, s.Start
	}
	return s.Start, s.End
}

// Contains reports whether p lies inside the selection, in reading order (stream selection).
func (s Selection) Contains(p Point) bool {
	if !s.Active {
		return false
	}
	lo, hi := s.bounds()
	return !p.before(lo) && !hi.before(p)
}

// rowWidth is the cell count of an absolute row. Scrollback lines keep the width they had when scrolled off.
func rowWidth(scr *screen.Screen, row int) int {
	if row < scr.ScrollbackLen() {
		return scr.ScrollbackWidth(row)
	}
	cols, _ := scr.Size()
	return cols
}

// cellAt reads the cell at an absolute row; anything outside the line is blank.
func cellAt(scr *screen.Screen, col, row int) screen.Cell {
	blank := screen.Cell{Rune: ' ', Width: 1}
	if col < 0 || row < 0 {
		return blank
	}
	sb := scr.ScrollbackLen()
	if row >= sb {
		return scr.Cell(col, row-sb)
	}
	line := scr.ScrollbackLine(row)
	if col >= len(line) {
		return blank
	}
	return line[col]
}

// totalRows is scrollback plus live screen.
func totalRows(scr *screen.Screen) int {
	_, rows := scr.Size()
	return scr.ScrollbackLen() + rows
}

// SelectedText returns the selected text, trailing spaces trimmed per line, lines joined with "\n".
// Wrapped lines come out as separate lines because wrap is not tracked yet.
func SelectedText(scr *screen.Screen, sel Selection) string {
	if !sel.Active {
		return ""
	}
	lo, hi := sel.bounds()
	lo.Row, hi.Row = max(lo.Row, 0), min(hi.Row, totalRows(scr)-1)
	var lines []string
	for row := lo.Row; row <= hi.Row; row++ {
		w := rowWidth(scr, row)
		from, to := 0, w-1
		if row == lo.Row {
			from = max(lo.Col, 0)
		}
		if row == hi.Row {
			to = min(hi.Col, w-1)
		}
		// Back up to the lead half so a selection starting on the continuation keeps the character.
		if from > 0 && cellAt(scr, from, row).Width == 0 {
			from--
		}
		var b strings.Builder
		for c := from; c <= to; c++ {
			cell := cellAt(scr, c, row)
			if cell.Width == 0 {
				continue
			}
			if cell.Rune == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(cell.Rune)
			}
		}
		lines = append(lines, strings.TrimRight(b.String(), " "))
	}
	return strings.Join(lines, "\n")
}

// wordStop reports whether r ends a word: whitespace, quotes, brackets and the pipe.
func wordStop(r rune) bool {
	return r <= ' ' || strings.ContainsRune(`"'()[]{}<>|`, r)
}

// WordAt selects the run of non-separator characters under p (double-click).
// On a separator it selects just that cell.
func WordAt(scr *screen.Screen, p Point) Selection {
	p.Row = min(max(p.Row, 0), totalRows(scr)-1)
	w := rowWidth(scr, p.Row)
	p.Col = min(max(p.Col, 0), max(w-1, 0))
	if p.Col > 0 && cellAt(scr, p.Col, p.Row).Width == 0 {
		p.Col--
	}
	if wordStop(cellAt(scr, p.Col, p.Row).Rune) {
		return Selection{Start: p, End: p, Active: true}
	}
	start, end := p.Col, p.Col
	for c := start - 1; c >= 0; c-- {
		cell := cellAt(scr, c, p.Row)
		if cell.Width == 0 {
			continue // its lead is next
		}
		if wordStop(cell.Rune) {
			break
		}
		start = c
	}
	for c := end + 1; c < w; c++ {
		cell := cellAt(scr, c, p.Row)
		if cell.Width != 0 && wordStop(cell.Rune) {
			break
		}
		end = c
	}
	return Selection{Start: Point{start, p.Row}, End: Point{end, p.Row}, Active: true}
}

// LineAt selects the whole row under p (triple-click).
func LineAt(scr *screen.Screen, p Point) Selection {
	row := min(max(p.Row, 0), totalRows(scr)-1)
	return Selection{
		Start:  Point{0, row},
		End:    Point{max(rowWidth(scr, row)-1, 0), row},
		Active: true,
	}
}
