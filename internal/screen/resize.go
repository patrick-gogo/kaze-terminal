package screen

// Resize changes the grid to cols x rows by truncating or padding; there is no reflow.
// When rows shrink below the cursor, the top rows scroll off (into history on the
// main screen) so the line being edited stays visible.
func (s *Screen) Resize(cols, rows int) {
	cols, rows = max(cols, 1), max(rows, 1)
	if cols == s.cols && rows == s.rows {
		return
	}
	drop := max(0, s.curRow-(rows-1))
	if drop > 0 && !s.alt {
		s.recordScrolled(drop)
	}
	s.cells = resizeGrid(s.lines, s.cols, s.rows, cols, rows, drop)
	if s.otherCells != nil {
		s.otherCells = resizeGrid(s.otherLines, s.cols, s.rows, cols, rows, 0)
	}
	s.lines = gridLines(s.cells, cols, rows)
	if s.otherCells != nil {
		s.otherLines = gridLines(s.otherCells, cols, rows)
	}
	if s.cols != cols {
		s.wrapPending = false                        // the pending wrap belonged to the old right edge
		s.saved.wrap, s.altSaved.wrap = false, false // so did the one DECSC/1049 stored
	}
	s.cols, s.rows = cols, rows
	s.dirty = make([]bool, rows)
	s.curCol = clamp(s.curCol, 0, cols-1)
	s.curRow = clamp(s.curRow-drop, 0, rows-1)
	s.saved.col = clamp(s.saved.col, 0, cols-1)
	s.saved.row -= drop // the DECSC slot belongs to whichever screen is showing, and the grid shifted
	s.saved.row = clamp(s.saved.row, 0, rows-1)
	s.altSaved.col = clamp(s.altSaved.col, 0, cols-1) // the main cursor, which drop never shifts while on alt
	s.altSaved.row = clamp(s.altSaved.row, 0, rows-1)
	s.scrollTop, s.scrollBottom = 0, rows-1
	s.viewOffset = clamp(s.viewOffset, 0, s.sb.count)
	s.MarkAllDirty()
}

// resizeGrid copies old rows [drop, ...) into a blank cols x rows grid and blanks any wide-char half the new edge split.
func resizeGrid(old [][]Cell, oldCols, oldRows, cols, rows, drop int) []Cell {
	out := make([]Cell, cols*rows)
	for i := range out {
		out[i] = blankCell
	}
	w := min(cols, oldCols)
	for r := 0; r < rows && r+drop < oldRows; r++ {
		copy(out[r*cols:r*cols+w], old[r+drop][:w])
		if last := out[r*cols+w-1]; last.Width == 2 {
			out[r*cols+w-1] = blankCell
		}
	}
	return out
}
