package screen

// UseAltScreen switches between the main and alternate buffers. Entering gives a
// blank alt buffer; leaving brings back the main content. Cursor save/restore
// (mode 1049) is the caller's job, since modes 47 and 1047 don't do it.
func (s *Screen) UseAltScreen(on bool) {
	if on == s.alt {
		return
	}
	s.alt = on
	s.cells, s.otherCells = s.otherCells, s.cells
	s.lines, s.otherLines = s.otherLines, s.lines
	if on {
		if s.cells == nil {
			s.cells = make([]Cell, len(s.otherCells))
			s.lines = gridLines(s.cells, s.cols, s.rows)
		}
		for i := range s.cells {
			s.cells[i] = blankCell
		}
	}
	s.viewOffset = 0
	s.wrapPending = false
	s.scrollTop, s.scrollBottom = 0, s.rows-1 // a margin set for one buffer must not confine the other
	s.MarkAllDirty()
}

// SaveAltCursor stores the cursor for mode 1049, apart from the DECSC slot.
func (s *Screen) SaveAltCursor() {
	s.altSaved = savedCursor{s.curCol, s.curRow, s.pen, true, s.wrapPending}
}

// RestoreAltCursor restores what SaveAltCursor stored; with nothing saved it goes home with the default pen.
func (s *Screen) RestoreAltCursor() { s.restore(s.altSaved) }

// OnAltScreen reports whether the alternate buffer is active.
func (s *Screen) OnAltScreen() bool { return s.alt }
