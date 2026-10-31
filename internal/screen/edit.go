package screen

import "slices"

// savedCursor is what DECSC stores: position and pen.
type savedCursor struct {
	col, row int
	pen      Cell
	valid    bool
	wrap     bool // pending-wrap state, restored by DECRC
}

const tabWidth = 8

// blank is an erased cell; it keeps the pen background, as xterm does.
func (s *Screen) blank() Cell {
	c := blankCell
	c.BG = s.pen.BG
	return c
}

func (s *Screen) fullRegion() bool { return s.scrollTop == 0 && s.scrollBottom == s.rows-1 }

// scrollUp is the single place lines leave the top. Only a full-screen scroll feeds scrollback.
func (s *Screen) scrollUp(n int) {
	n = min(n, s.scrollBottom-s.scrollTop+1)
	if n < 1 {
		return
	}
	if s.fullRegion() && !s.alt {
		s.recordScrolled(n)
	}
	s.shiftRows(s.scrollTop, s.scrollBottom, -n)
}

func (s *Screen) scrollDownIn(top, bottom, n int) {
	n = min(n, bottom-top+1)
	if n < 1 {
		return
	}
	s.shiftRows(top, bottom, n)
}

// shiftRows moves rows top..bottom by delta (positive = down), blanks the vacated rows and dirties only that range.
func (s *Screen) shiftRows(top, bottom, delta int) {
	region := s.lines[top : bottom+1]
	n := -delta
	if delta > 0 {
		n = len(region) - delta
	}
	// Rotate the row references left by n. Only the vacated rows' cells
	// are touched; partial scroll regions use the same bounded rotation.
	slices.Reverse(region[:n])
	slices.Reverse(region[n:])
	slices.Reverse(region)
	if delta < 0 {
		s.blankRows(bottom+1+delta, bottom)
	} else {
		s.blankRows(top, top+delta-1)
	}
	for r := top; r <= bottom; r++ {
		s.dirty[r] = true
	}
}

func (s *Screen) blankRows(from, to int) {
	b := s.blank()
	for r := from; r <= to; r++ {
		line := s.row(r)
		for c := range line {
			line[c] = b
		}
	}
}

// ScrollUp scrolls the scroll region up n lines (SU).
func (s *Screen) ScrollUp(n int) { s.scrollUp(n) }

// ScrollDown scrolls the scroll region down n lines (SD).
func (s *Screen) ScrollDown(n int) { s.scrollDownIn(s.scrollTop, s.scrollBottom, n) }

// SetScrollRegion sets DECSTBM (0-based, inclusive) and homes the cursor. An empty or out-of-range region is ignored.
func (s *Screen) SetScrollRegion(top, bottom int) {
	bottom = min(bottom, s.rows-1)
	if top < 0 || top >= bottom {
		return
	}
	s.scrollTop, s.scrollBottom = top, bottom
	s.SetCursor(0, 0)
}

// ReverseIndex moves up a row, scrolling the region down at its top.
func (s *Screen) ReverseIndex() {
	s.wrapPending = false
	if s.curRow == s.scrollTop {
		s.ScrollDown(1)
		return
	}
	if s.curRow > 0 {
		s.curRow--
	}
}

// InsertLines inserts blank lines at the cursor row, pushing the rest of the region down.
func (s *Screen) InsertLines(n int) {
	if s.curRow < s.scrollTop || s.curRow > s.scrollBottom {
		return
	}
	s.scrollDownIn(s.curRow, s.scrollBottom, n)
	s.CarriageReturn()
}

// DeleteLines removes lines at the cursor row, pulling the rest of the region up.
func (s *Screen) DeleteLines(n int) {
	if s.curRow < s.scrollTop || s.curRow > s.scrollBottom {
		return
	}
	n = min(n, s.scrollBottom-s.curRow+1)
	if n < 1 {
		return
	}
	s.shiftRows(s.curRow, s.scrollBottom, -n)
	s.CarriageReturn()
}

// fixWide blanks any half of a wide char left without its partner after an edit.
func (s *Screen) fixWide(row int) {
	b := s.blank()
	line := s.row(row)
	for c := 0; c < s.cols; c++ {
		cell := line[c]
		switch {
		case cell.Width == 0 && (c == 0 || line[c-1].Width != 2):
			s.SetCell(c, row, b)
		case cell.Width == 2 && (c == s.cols-1 || line[c+1].Width != 0):
			s.SetCell(c, row, b)
		}
	}
}

// eraseSpan blanks columns [from, to) of one row.
func (s *Screen) eraseSpan(row, from, to int) {
	b := s.blank()
	for c := max(from, 0); c < min(to, s.cols); c++ {
		s.SetCell(c, row, b)
	}
	s.fixWide(row)
}

// EraseInLine implements EL: 0 cursor to end, 1 start to cursor, 2 whole line.
func (s *Screen) EraseInLine(mode int) {
	s.wrapPending = false
	switch mode {
	case 0:
		s.eraseSpan(s.curRow, s.curCol, s.cols)
	case 1:
		s.eraseSpan(s.curRow, 0, s.curCol+1)
	case 2:
		s.eraseSpan(s.curRow, 0, s.cols)
	}
}

// EraseInDisplay implements ED: 0 cursor to end, 1 start to cursor, 2 all, 3 scrollback only.
func (s *Screen) EraseInDisplay(mode int) {
	s.wrapPending = false
	switch mode {
	case 0:
		s.eraseSpan(s.curRow, s.curCol, s.cols)
		for r := s.curRow + 1; r < s.rows; r++ {
			s.eraseSpan(r, 0, s.cols)
		}
	case 1:
		for r := 0; r < s.curRow; r++ {
			s.eraseSpan(r, 0, s.cols)
		}
		s.eraseSpan(s.curRow, 0, s.curCol+1)
	case 2:
		for r := 0; r < s.rows; r++ {
			s.eraseSpan(r, 0, s.cols)
		}
	case 3: // scrollback only; the visible screen stays
		s.sb = scrollback{max: s.sb.max, evicted: s.sb.evicted + uint64(s.sb.count)}
		s.viewOffset = 0
		s.MarkAllDirty()
	}
}

// InsertChars shifts the rest of the row right from the cursor and blanks n cells (ICH).
func (s *Screen) InsertChars(n int) {
	s.wrapPending = false
	n = min(n, s.cols-s.curCol)
	if n < 1 {
		return
	}
	row := s.row(s.curRow)
	copy(row[s.curCol+n:], row[s.curCol:s.cols-n])
	b := s.blank()
	for c := s.curCol; c < s.curCol+n; c++ {
		row[c] = b
	}
	s.dirty[s.curRow] = true
	s.fixWide(s.curRow)
}

// DeleteChars pulls the rest of the row left over n cells at the cursor (DCH).
func (s *Screen) DeleteChars(n int) {
	s.wrapPending = false
	n = min(n, s.cols-s.curCol)
	if n < 1 {
		return
	}
	row := s.row(s.curRow)
	copy(row[s.curCol:], row[s.curCol+n:])
	b := s.blank()
	for c := s.cols - n; c < s.cols; c++ {
		row[c] = b
	}
	s.dirty[s.curRow] = true
	s.fixWide(s.curRow)
}

// EraseChars blanks n cells from the cursor without moving anything (ECH).
func (s *Screen) EraseChars(n int) {
	s.wrapPending = false
	s.eraseSpan(s.curRow, s.curCol, s.curCol+n)
}

// SaveCursor stores the cursor position and pen (DECSC).
func (s *Screen) SaveCursor() { s.saved = savedCursor{s.curCol, s.curRow, s.pen, true, s.wrapPending} }

// RestoreCursor restores them (DECRC); with nothing saved it goes home with the default pen.
func (s *Screen) RestoreCursor() { s.restore(s.saved) }

func (s *Screen) restore(c savedCursor) {
	if !c.valid {
		s.SetCursor(0, 0)
		s.pen = blankCell
		return
	}
	s.SetCursor(c.col, c.row)
	s.pen = c.pen
	s.wrapPending = c.wrap
}

// TabForward moves to the nth next tab stop (every 8 columns), stopping at the last column.
func (s *Screen) TabForward(n int) {
	col := s.curCol
	for ; n > 0; n-- {
		if col >= s.cols-1 {
			break
		}
		col = (col/tabWidth + 1) * tabWidth
	}
	s.SetCursor(col, s.curRow)
}

// Reset is RIS: main screen, blanked, home cursor, default pen and modes. Scrollback is kept.
func (s *Screen) Reset() {
	s.UseAltScreen(false)
	s.altSaved = savedCursor{}
	s.pen = blankCell
	s.autoWrap = true
	s.wrapPending = false
	s.scrollTop, s.scrollBottom = 0, s.rows-1
	s.saved = savedCursor{}
	s.curCol, s.curRow = 0, 0
	for i := range s.cells {
		s.cells[i] = blankCell
	}
	s.MarkAllDirty()
}

// CursorUp moves up n rows (CUU). It stops at the top margin when the cursor starts at or below it.
func (s *Screen) CursorUp(n int) {
	limit := 0
	if s.curRow >= s.scrollTop {
		limit = s.scrollTop
	}
	s.SetCursor(s.curCol, max(s.curRow-n, limit))
}

// CursorDown moves down n rows (CUD). It stops at the bottom margin when the cursor starts at or above it.
func (s *Screen) CursorDown(n int) {
	limit := s.rows - 1
	if s.curRow <= s.scrollBottom {
		limit = s.scrollBottom
	}
	s.SetCursor(s.curCol, min(s.curRow+n, limit))
}
