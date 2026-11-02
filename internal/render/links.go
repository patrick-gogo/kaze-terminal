package render

import (
	"strings"
	"unicode"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

// maxWrapRows bounds how many rows one wrapped link may span, so a screen full
// of text with no spaces never makes a scan walk the whole history.
const maxWrapRows = 16

// LinkSpan is the part of a web link or .md path on one row: Row is absolute (scrollback
// first), From and To are inclusive columns. A wrapped link has one per row.
type LinkSpan struct{ Row, From, To int }

// Links finds web links and .md paths in the grid text. A link that auto-wrapped
// onto the next row is found whole; a full row ended by a newline is not joined. The zero value is ready; its buffers are
// reused, so a frame with no links allocates nothing. Not safe for concurrent use.
type Links struct {
	rs    []rune
	pos   []Point
	found [][2]int
	spans []LinkSpan
}

// At is the link under p, an absolute grid point. Call with the session locked.
func (l *Links) At(scr *screen.Screen, p Point) (string, bool) {
	if p.Row < 0 || p.Row >= totalRows(scr) {
		return "", false
	}
	if p.Col > 0 && cellAt(scr, p.Col, p.Row).Width == 0 {
		p.Col-- // the right half of a wide character
	}
	start, end := p.Row, p.Row
	for i := 0; i < maxWrapRows && start > 0 && continues(scr, start-1); i++ {
		start--
	}
	for i := 0; i < maxWrapRows && continues(scr, end); i++ {
		end++
	}
	l.line(scr, start, end)
	l.found = appendLinks(l.found[:0], l.rs)
	for _, f := range l.found {
		for k := f[0]; k < f[1]; k++ {
			if l.pos[k] == p {
				return string(l.rs[f[0]:f[1]]), true
			}
		}
	}
	return "", false
}

// Visible is every link part on rows [top, top+n), in row order. The result is
// reused by the next call. Call with the session locked.
func (l *Links) Visible(scr *screen.Screen, top, n int) []LinkSpan {
	l.spans = l.spans[:0]
	last := min(top+n, totalRows(scr)) - 1
	row := max(top, 0)
	for i := 0; i < maxWrapRows && row > 0 && continues(scr, row-1); i++ {
		row--
	}
	for row <= last {
		end := row
		for i := 0; i < maxWrapRows && end < last && continues(scr, end); i++ {
			end++
		}
		l.line(scr, row, end)
		l.found = appendLinks(l.found[:0], l.rs)
		for _, f := range l.found {
			l.addSpans(scr, f, top, last)
		}
		row = end + 1
	}
	return l.spans
}

// addSpans splits one found link into per-row spans inside [top, last].
func (l *Links) addSpans(scr *screen.Screen, f [2]int, top, last int) {
	for k := f[0]; k < f[1]; {
		r := l.pos[k].Row
		j := k
		for j+1 < f[1] && l.pos[j+1].Row == r {
			j++
		}
		if r >= top && r <= last {
			to := l.pos[j].Col
			if cellAt(scr, to, r).Width == 2 {
				to++ // a wide last character covers two columns
			}
			l.spans = append(l.spans, LinkSpan{Row: r, From: l.pos[k].Col, To: to})
		}
		k = j + 1
	}
}

// line loads rows start..end as one run of text, one rune per character.
func (l *Links) line(scr *screen.Screen, start, end int) {
	l.rs, l.pos = l.rs[:0], l.pos[:0]
	for row := start; row <= end; row++ {
		w := rowWidth(scr, row)
		for col := 0; col < w; col++ {
			c := cellAt(scr, col, row)
			if c.Width == 0 {
				continue
			}
			r := c.Rune
			if r == 0 {
				r = ' '
			}
			l.rs = append(l.rs, r)
			l.pos = append(l.pos, Point{Col: col, Row: row})
		}
	}
}

// continues reports whether row's text runs on into the next row: the screen
// marked it as auto-wrapped, and the next row starts with a link character.
func continues(scr *screen.Screen, row int) bool {
	if row+1 >= totalRows(scr) {
		return false
	}
	w := rowWidth(scr, row)
	if w == 0 {
		return false
	}
	return cellAt(scr, w-1, row).Attrs&screen.AttrWrapped != 0 && (urlRune(cellAt(scr, 0, row+1).Rune) || cellAt(scr, 0, row+1).Rune == '\\')
}

// findLinks is appendLinks into a new slice.
func findLinks(rs []rune) [][2]int { return appendLinks(nil, rs) }

// appendLinks appends the [start, end) rune ranges of the http and https links in rs.
func appendLinks(out [][2]int, rs []rune) [][2]int {
	for i := 0; i < len(rs); {
		n := schemeLen(rs[i:])
		if n > 0 && (i == 0 || !unicode.IsLetter(rs[i-1]) && !unicode.IsDigit(rs[i-1])) {
			j := i + n
			for j < len(rs) && urlRune(rs[j]) {
				j++
			}
			end := trimURLEnd(rs, i+n, j)
			if end > i+n {
				out = append(out, [2]int{i, end})
			}
			i = max(j, i+1)
			continue
		}
		if !pathRune(rs[i]) {
			i++
			continue
		}
		end, next := pathAt(rs, i)
		if end > i {
			out = append(out, [2]int{i, end})
		}
		i = max(next, i+1)
	}
	return out
}

func IsWebLink(s string) bool {
	return len(s) >= 7 && (strings.EqualFold(s[:7], "http://") || len(s) >= 8 && strings.EqualFold(s[:8], "https://"))
}
func pathRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-/\\~+@", r) || r > 127 && unicode.IsLetter(r) && unicode.In(r, unicode.Latin, unicode.Greek, unicode.Cyrillic)
}
func pathAt(rs []rune, i int) (end, next int) {
	j := i
	for j < len(rs) {
		if j > i && !unicode.IsLetter(rs[j-1]) && !unicode.IsDigit(rs[j-1]) && schemeLen(rs[j:]) > 0 {
			break
		}
		if pathRune(rs[j]) {
			j++
			continue
		}
		if j == i+1 && ((rs[i] >= 'a' && rs[i] <= 'z') || (rs[i] >= 'A' && rs[i] <= 'Z')) && rs[j] == ':' && j+1 < len(rs) && (rs[j+1] == '/' || rs[j+1] == '\\') {
			j++
			continue
		}
		break
	}
	end, next = j, j
	for end > i && rs[end-1] == '.' {
		end--
	}
	if end == j {
		for range 2 {
			if next+1 >= len(rs) || rs[next] != ':' || rs[next+1] < '0' || rs[next+1] > '9' {
				break
			}
			next++
			for next < len(rs) && rs[next] >= '0' && rs[next] <= '9' {
				next++
			}
		}
	}
	if end-i < 4 || rs[end-3] != '.' || unicode.ToLower(rs[end-2]) != 'm' || unicode.ToLower(rs[end-1]) != 'd' || strings.ContainsRune("/\\.", rs[end-4]) {
		return i, next
	}
	return end, next
}

// schemeLen is the length of "http://" or "https://" at the start of rs, any case, or 0.
func schemeLen(rs []rune) int {
	if len(rs) < 7 || rs[0] != 'h' && rs[0] != 'H' || rs[4] != ':' && rs[5] != ':' {
		return 0 // the common case, checked without case folding
	}
	for _, s := range []string{"https://", "http://"} {
		if len(rs) < len(s) {
			continue
		}
		ok := true
		for k := 0; k < len(s); k++ {
			if unicode.ToLower(rs[k]) != rune(s[k]) {
				ok = false
				break
			}
		}
		if ok {
			return len(s)
		}
	}
	return 0
}

// trimURLEnd drops sentence punctuation and an unmatched closing bracket from
// the end of rs[start:end], so "(see https://x.io/a)." gives https://x.io/a.
func trimURLEnd(rs []rune, start, end int) int {
	for end > start {
		r := rs[end-1]
		if strings.ContainsRune(".,;:!?'*", r) {
			end--
			continue
		}
		open := rune(0)
		switch r {
		case ')':
			open = '('
		case ']':
			open = '['
		}
		if open != 0 {
			opens, closes := 0, 0
			for _, c := range rs[start:end] {
				if c == open {
					opens++
				} else if c == r {
					closes++
				}
			}
			if closes > opens {
				end--
				continue
			}
		}
		break
	}
	return end
}

// urlRune reports whether r can be part of a link: no spaces, controls,
// quotes, angle brackets or the box-drawing characters TUIs frame text with.
func urlRune(r rune) bool {
	switch {
	case r <= ' ' || r == 0x7f || r >= 0x80 && r < 0xa0:
		return false
	case r >= 0x2500 && r <= 0x259f:
		return false
	case unicode.IsSpace(r):
		return false
	}
	return !strings.ContainsRune("\"<>`\\^{}|", r)
}

// LinkText uses byte offsets into the clicked text. LinksIn allocates only on
// clicks; the terminal's frame scan continues to reuse its rune buffers.
type LinkText struct {
	Start, End int
	Text       string
}

func LinksIn(s string) []LinkText {
	rs := []rune(s)
	positions := make([]int, 0, len(rs)+1)
	for i := range s {
		positions = append(positions, i)
	}
	positions = append(positions, len(s))
	var out []LinkText
	for _, sp := range findLinks(rs) {
		start, end := positions[sp[0]], positions[sp[1]]
		out = append(out, LinkText{start, end, s[start:end]})
	}
	return out
}
