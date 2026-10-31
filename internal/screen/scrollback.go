package screen

const defaultScrollback = 10000

// reuseSlack lets a short line still reuse a slightly bigger evicted line.
const reuseSlack = 16

// scrollback is a ring of lines; once full, the oldest line's slice is reused for the newest.
// A line stores its cells up to the last one that is not a default blank, and its width apart.
type scrollback struct {
	lines  [][]Cell
	widths []int32 // each line's width when it scrolled off
	start  int     // index of the oldest line
	count  int
	max    int
	// evicted counts lines dropped off the top; it never goes back, so callers can diff it.
	evicted uint64
}

// push copies row in without its blank tail, reusing the evicted line's storage when the ring is full.
func (b *scrollback) push(row []Cell) {
	if b.max <= 0 {
		b.evicted++
		return
	}
	width := int32(len(row))
	n := len(row)
	for n > 0 && row[n-1] == blankCell {
		n--
	}
	row = row[:n]
	if b.count < b.max {
		b.lines = append(b.lines, append([]Cell(nil), row...))
		b.widths = append(b.widths, width)
		b.count++
		return
	}
	b.evicted++
	old := b.lines[b.start]
	// Reuse the evicted line unless it is far bigger than this one, so history
	// that once held long lines does not keep their storage behind short ones.
	if cap(old) >= len(row) && cap(old) <= 2*len(row)+reuseSlack {
		old = old[:len(row)]
	} else {
		old = make([]Cell, len(row))
	}
	copy(old, row)
	b.lines[b.start] = old
	b.widths[b.start] = width
	b.start = (b.start + 1) % b.max
}

func (b *scrollback) at(i int) []Cell { return b.lines[(b.start+i)%len(b.lines)] }

// NewWithScrollback is New with an explicit history cap in lines.
func NewWithScrollback(cols, rows, maxLines int) *Screen {
	s := New(cols, rows)
	if maxLines < 0 {
		maxLines = 0
	}
	s.sb.max = maxLines
	return s
}

// ScrollbackLen is the number of history lines currently held.
func (s *Screen) ScrollbackLen() int { return s.sb.count }

// ScrollbackLine returns history line i (0 = oldest) up to its last cell that is not a
// default blank; cells past it, up to ScrollbackWidth, are blank. Callers must not modify it.
func (s *Screen) ScrollbackLine(i int) []Cell {
	if i < 0 || i >= s.sb.count {
		return nil
	}
	return s.sb.at(i)
}

// ScrollbackWidth is the width history line i had when it scrolled off.
func (s *Screen) ScrollbackWidth(i int) int {
	if i < 0 || i >= s.sb.count {
		return 0
	}
	return int(s.sb.widths[(s.sb.start+i)%len(s.sb.widths)])
}

// SetViewOffset scrolls the view n lines back from the bottom, clamped to the history size.
func (s *Screen) SetViewOffset(n int) {
	s.viewOffset = clamp(n, 0, s.sb.count)
	s.MarkAllDirty()
}

func (s *Screen) ViewOffset() int { return s.viewOffset }

// VisibleCell is the cell the renderer draws at col,row once the view offset is applied.
func (s *Screen) VisibleCell(col, row int) Cell {
	if !s.inBounds(col, row) {
		return blankCell
	}
	idx := s.sb.count - s.viewOffset + row // virtual line: history first, then the live screen
	if idx >= s.sb.count {
		return s.Cell(col, idx-s.sb.count)
	}
	line := s.sb.at(idx)
	if col >= len(line) {
		return blankCell
	}
	return line[col]
}

// recordScrolled saves the top n rows before scrollUp discards them, keeping a scrolled-back view anchored.
func (s *Screen) recordScrolled(n int) {
	for r := 0; r < n; r++ {
		s.sb.push(s.row(r))
	}
	if s.viewOffset > 0 {
		s.viewOffset = min(s.viewOffset+n, s.sb.count) // a full ring evicts one line per push, so count alone can't track it
	}
}

// Evicted counts history lines dropped off the top since the screen was made:
// ring overflow, a zero-size ring, and ED 3. Absolute rows shift down by its growth.
func (s *Screen) Evicted() uint64 { return s.sb.evicted }
