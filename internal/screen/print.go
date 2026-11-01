package screen

import "github.com/mattn/go-runewidth"

// Ambiguous-width runes are one column, matching Claude Code's assumptions.
var width = runewidth.Condition{EastAsianWidth: false}

// Pen returns the template whose FG, BG and Attrs are applied to printed cells.
func (s *Screen) Pen() *Cell { return &s.pen }

// SetAutoWrap controls whether printing past the last column wraps to the next line.
func (s *Screen) SetAutoWrap(on bool) {
	s.autoWrap = on
	if !on {
		s.wrapPending = false // a pending wrap must not survive to fire after wrap is re-enabled
	}
}

// CarriageReturn moves the cursor to column 0.
func (s *Screen) CarriageReturn() {
	s.curCol = 0
	s.wrapPending = false
}

// LineFeed moves down one row, scrolling the region up when the cursor is on its bottom row.
func (s *Screen) LineFeed() {
	s.wrapPending = false
	if s.curRow == s.scrollBottom {
		s.scrollUp(1)
		return
	}
	if s.curRow < s.rows-1 {
		s.curRow++
	}
}

// Print writes r at the cursor with the current pen and advances, wrapping as needed.
func (s *Screen) Print(r rune) {
	w := width.RuneWidth(r)
	if w == 0 { // combining marks are dropped in v1
		return
	}
	if s.wrapPending && s.autoWrap {
		s.markWrapped()
		s.CarriageReturn()
		s.LineFeed()
	}
	if s.curCol+w > s.cols {
		if !s.autoWrap || w > s.cols {
			return
		}
		s.clearWideHalves(s.curCol, 1) // blanking a continuation half must not orphan its lead
		s.SetCell(s.curCol, s.curRow, blankCell)
		s.markWrapped()
		s.CarriageReturn()
		s.LineFeed()
	}
	s.clearWideHalves(s.curCol, w)
	s.SetCell(s.curCol, s.curRow, Cell{Rune: r, Width: uint8(w), FG: s.pen.FG, BG: s.pen.BG, Attrs: s.pen.Attrs})
	if w == 2 {
		s.SetCell(s.curCol+1, s.curRow, Cell{Width: 0, FG: s.pen.FG, BG: s.pen.BG, Attrs: s.pen.Attrs})
	}
	s.curCol += w
	if s.curCol >= s.cols {
		s.curCol = s.cols - 1
		s.wrapPending = s.autoWrap
	}
}

// markWrapped flags the cursor row as running on into the next row.
func (s *Screen) markWrapped() {
	c := s.Cell(s.cols-1, s.curRow)
	c.Attrs |= AttrWrapped
	s.SetCell(s.cols-1, s.curRow, c)
}

// clearWideHalves blanks the partner of any wide char that a write over [col, col+w) would split.
func (s *Screen) clearWideHalves(col, w int) {
	row := s.curRow
	if s.Cell(col, row).Width == 0 {
		s.SetCell(col-1, row, blankCell)
	}
	last := col + w - 1
	if s.Cell(last, row).Width == 2 {
		s.SetCell(last+1, row, blankCell)
	}
}
