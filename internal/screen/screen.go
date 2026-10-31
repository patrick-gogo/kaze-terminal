// Package screen is the terminal's cell grid with per-row dirty tracking.
package screen

// Screen stores cells in contiguous row buffers. Logical row references
// rotate during scrolling, and nothing allocates per cell.
type Screen struct {
	cols, rows int
	cells      []Cell // backing storage; logical row order is in lines
	lines      [][]Cell
	dirty      []bool
	curCol     int
	curRow     int

	pen         Cell // FG/BG/Attrs template for printed cells; Rune and Width are ignored
	autoWrap    bool
	wrapPending bool // cursor sits past the last column; the wrap happens on the next printable rune

	scrollTop, scrollBottom int // DECSTBM region, 0-based inclusive; the full screen by default
	saved                   savedCursor
	altSaved                savedCursor // mode 1049's own slot, so an app's DECSC can't clobber it

	sb         scrollback
	viewOffset int // lines scrolled back from the bottom

	alt        bool
	otherCells []Cell // the buffer that is not showing
	otherLines [][]Cell
}

// New returns a blank screen with every row dirty, so the first frame paints fully.
func New(cols, rows int) *Screen {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	s := &Screen{
		cols:  cols,
		rows:  rows,
		cells: make([]Cell, cols*rows),
		dirty: make([]bool, rows),
		pen:   blankCell,

		autoWrap:     true,
		scrollBottom: rows - 1,
		sb:           scrollback{max: defaultScrollback},
	}
	for i := range s.cells {
		s.cells[i] = blankCell
	}
	s.lines = gridLines(s.cells, cols, rows)
	s.MarkAllDirty()
	return s
}

func (s *Screen) Size() (cols, rows int) { return s.cols, s.rows }

func (s *Screen) inBounds(col, row int) bool {
	return col >= 0 && col < s.cols && row >= 0 && row < s.rows
}

// Cell returns the cell at col,row, or a blank cell when out of range.
func (s *Screen) Cell(col, row int) Cell {
	if !s.inBounds(col, row) {
		return blankCell
	}
	return s.row(row)[col]
}

// SetCell stores c and marks the row dirty only if the cell actually changed.
func (s *Screen) SetCell(col, row int, c Cell) {
	if !s.inBounds(col, row) {
		return
	}
	line := s.row(row)
	if line[col] == c {
		return
	}
	line[col] = c
	s.dirty[row] = true
}

func (s *Screen) Cursor() (col, row int) { return s.curCol, s.curRow }

// SetCursor moves the cursor, clamping to the grid.
func (s *Screen) SetCursor(col, row int) {
	s.curCol = clamp(col, 0, s.cols-1)
	s.curRow = clamp(row, 0, s.rows-1)
	s.wrapPending = false // any explicit cursor move cancels a pending wrap, as in xterm
}

// TakeDirty returns changed rows in ascending order and clears the flags.
func (s *Screen) TakeDirty() []int { return s.TakeDirtyInto(nil) }

// TakeDirtyInto appends changed rows to dst and clears their flags. A caller
// retaining its buffer can collect dirty rows without per-frame allocations.
func (s *Screen) TakeDirtyInto(out []int) []int {
	for r, d := range s.dirty {
		if d {
			out = append(out, r)
			s.dirty[r] = false
		}
	}
	return out
}

func (s *Screen) MarkAllDirty() {
	for r := range s.dirty {
		s.dirty[r] = true
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// gridLines indexes the backing allocation once. Scrolling rotates these
// references, keeping each row contiguous without copying all of its cells.
func gridLines(cells []Cell, cols, rows int) [][]Cell {
	lines := make([][]Cell, rows)
	for r := range lines {
		lines[r] = cells[r*cols : (r+1)*cols]
	}
	return lines
}

func (s *Screen) row(r int) []Cell { return s.lines[r] }
