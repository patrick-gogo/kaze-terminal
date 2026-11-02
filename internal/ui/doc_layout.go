package ui

import (
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/patrick-gogo/kaze-terminal/internal/mdoc"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type docRole uint8

const (
	roleBody docRole = iota
	roleHeading
	roleMuted
	roleCode
	roleLink
)

func (win *window) docColor(r docRole) color.NRGBA {
	switch r {
	case roleMuted:
		return win.ui.th.Muted
	case roleLink:
		return win.pal.ANSI[4]
	}
	return win.pal.FG
}

type docRun struct {
	text          string
	x, w          int
	face          font.Font
	size          unit.Sp
	role          docRole
	link          int
	under, strike bool
	code          bool
}
type docLine struct {
	y, h int
	runs []docRun
}
type docBoxKind uint8

const (
	boxCode docBoxKind = iota
	boxRule
	boxQuote
	boxTableLine
	boxTableHead
	boxTaskOpen
	boxTaskDone
)

type docBox struct {
	r    image.Rectangle
	kind docBoxKind
}
type docLayKey struct {
	doc    *mdoc.Doc
	width  int
	metric unit.Metric
	shaper *text.Shaper
}
type docLayout struct {
	key    docLayKey
	lines  []docLine
	boxes  []docBox
	links  []string
	headY  []int
	height int
}
type docAdvKey struct {
	r    rune
	face font.Font
	px   int
}

func (win *window) runeAdv(gtx layout.Context, f font.Font, size unit.Sp, r rune) fixed.Int26_6 {
	k := docAdvKey{r, f, gtx.Sp(size)}
	if win.docs.adv == nil || win.docs.shaper != win.ui.shaper || len(win.docs.adv) > 8192 {
		win.docs.adv = map[docAdvKey]fixed.Int26_6{}
		win.docs.shaper = win.ui.shaper
	}
	if w, ok := win.docs.adv[k]; ok {
		return w
	}
	win.ui.shaper.LayoutString(text.Parameters{Font: f, PxPerEm: fixed.I(k.px), MaxWidth: 1 << 16, MaxLines: 1, Locale: gtx.Locale}, string(r))
	var w fixed.Int26_6
	for g, ok := win.ui.shaper.NextGlyph(); ok; g, ok = win.ui.shaper.NextGlyph() {
		w += g.Advance
	}
	win.docs.adv[k] = w
	return w
}
func (win *window) fitDoc(d *mdoc.Doc) {
	fit := func(s string) string {
		parts := strings.Split(s, "\n")
		for i := range parts {
			parts[i] = win.ui.fit(parts[i])
		}
		return strings.Join(parts, "\n")
	}

	for bi := range d.Blocks {
		b := &d.Blocks[bi]
		for i := range b.Spans {
			b.Spans[i].Text = fit(b.Spans[i].Text)
		}
		if b.Code != "" {
			b.Code = fit(strings.ReplaceAll(b.Code, "	", "    "))
		}
		if b.Table != nil {
			for r := range b.Table.Rows {
				for c := range b.Table.Rows[r] {
					for i := range b.Table.Rows[r][c] {
						s := &b.Table.Rows[r][c][i]
						s.Text = fit(s.Text)
					}
				}
			}
		}
	}
}
func (win *window) layoutDoc(gtx layout.Context, d *docView, width int) *docLayout {
	width = max(1, width)
	k := docLayKey{d.doc, width, gtx.Metric, win.ui.shaper}
	if d.lay.key == k {
		return &d.lay
	}
	d.layouts++
	l := docLayout{key: k}
	dp := gtx.Dp
	y := 0
	// wrap measures each rune at most once per face/size and walks text only forwards.
	wrap := func(spans []mdoc.Span, x0, x1, y0, h int, size unit.Sp, base font.Font, role docRole, preserve bool) int {
		x, ly := x0, y0
		line := docLine{y: ly, h: h}
		flush := func() { l.lines = append(l.lines, line); ly += h; x = x0; line = docLine{y: ly, h: h} }
		for _, sp := range spans {
			face := base
			rr := role
			if sp.Style&mdoc.Bold != 0 {
				face.Weight = font.SemiBold
			}
			if sp.Style&mdoc.Italic != 0 {
				face.Style = font.Italic
				face.Weight = font.Medium
			}
			if sp.Style&mdoc.Code != 0 {
				face.Typeface = chromeFace
				rr = roleCode
			}
			link := -1
			if sp.Link != "" {
				link = len(l.links)
				l.links = append(l.links, sp.Link)
				rr = roleLink
			}
			emit := func(s string, w int) {
				if s == "" {
					return
				}
				line.runs = append(line.runs, docRun{text: s, x: x, w: w, face: face, size: size, role: rr, link: link, under: link >= 0, strike: sp.Style&mdoc.Strike != 0, code: sp.Style&mdoc.Code != 0})
				x += w
			}
			s := sp.Text
			for len(s) > 0 {
				if s[0] == '\n' {
					flush()
					s = s[1:]
					continue
				}
				r, n := utf8.DecodeRuneInString(s)
				space := unicode.IsSpace(r)
				end := n
				for end < len(s) {
					q, z := utf8.DecodeRuneInString(s[end:])
					if q == '\n' || unicode.IsSpace(q) != space {
						break
					}
					end += z
				}
				word := s[:end]
				s = s[end:]
				if space && !preserve {
					if x > x0 {
						w := win.runeAdv(gtx, face, size, ' ').Ceil()
						if x+w <= x1 {
							emit(" ", w)
						}
					}
					continue
				}
				var total fixed.Int26_6
				for _, q := range word {
					total += win.runeAdv(gtx, face, size, q)
				}
				w := total.Ceil()
				if w <= x1-x0 {
					if x+w > x1 {
						flush()
					}
					emit(word, w)
					continue
				}
				start, used := 0, fixed.Int26_6(0)
				for i, q := range word {
					adv := win.runeAdv(gtx, face, size, q)
					if x+(used+adv).Ceil() > x1 {
						if i > start {
							emit(word[start:i], used.Ceil())
						}
						flush()
						start = i
						used = 0
					}
					used += adv
				}
				if start < len(word) {
					emit(word[start:], min(used.Ceil(), max(1, x1-x)))
				}
			}
		}
		if len(line.runs) > 0 || ly == y0 {
			flush()
		}
		return ly
	}
	for _, b := range d.doc.Blocks {
		indent := min(dp(22)*b.Indent+dp(14)*b.Quote, max(0, width-dp(48)))
		x0 := indent
		start := y
		switch b.Kind {
		case mdoc.HeadingBlock:
			if y == 0 {
				y += dp(12)
			} else {
				y += dp(20)
			}
			l.headY = append(l.headY, y)
			sz := unit.Sp([]float32{24, 20, 17, 15, 14, 13}[max(0, min(b.Level-1, 5))])
			y = wrap(b.Spans, x0, width, y, gtx.Sp(sz)+dp(8), sz, font.Font{Typeface: sansFace, Weight: font.SemiBold}, roleHeading, false)
			if b.Level <= 2 {
				l.boxes = append(l.boxes, docBox{image.Rect(x0, y, width, y+1), boxRule})
				y += dp(4)
			}
			y += dp(8)
		case mdoc.CodeBlock:
			pad := min(dp(12), max(0, (width-x0-1)/3))
			top := y
			y += pad
			y = wrap([]mdoc.Span{{Text: strings.ReplaceAll(b.Code, "\t", "    ")}}, x0+pad, max(x0+pad+1, width-pad), y, dp(19), 12.5, font.Font{Typeface: chromeFace}, roleCode, true)
			y += pad
			l.boxes = append(l.boxes, docBox{image.Rect(x0, top, width, y), boxCode})
			y += dp(10)
		case mdoc.Rule:
			l.boxes = append(l.boxes, docBox{image.Rect(x0, y+dp(12), width, y+dp(12)+1), boxRule})
			y += dp(25)
		case mdoc.TableBlock:
			if b.Table == nil || len(b.Table.Align) == 0 {
				continue
			}
			n := len(b.Table.Align)
			minW := dp(48)
			natural := make([]int, n)
			for c := range natural {
				natural[c] = minW
			}
			face := font.Font{Typeface: sansFace}
			for _, row := range b.Table.Rows {
				for c, cell := range row {
					if c >= n {
						break
					}
					sum := 0
					for _, sp := range cell {
						for _, q := range sp.Text {
							sum += win.runeAdv(gtx, face, 13, q).Ceil()
						}
					}
					natural[c] = max(natural[c], sum+dp(16))
				}
			}
			available := max(width-x0, n*minW)
			total := 0
			for _, v := range natural {
				total += v
			}
			colW := make([]int, n)
			remaining := available - n*minW
			extra := max(1, total-n*minW)
			for i, v := range natural {
				colW[i] = minW + remaining*(v-minW)/extra
			}
			if total <= available {
				for i, v := range natural {
					colW[i] = max(minW, available*v/max(1, total))
				}
			}
			for ri, row := range b.Table.Rows {
				top, bottom, x := y, y+dp(34), x0
				for ci, cell := range row {
					if ci >= n {
						break
					}
					f := face
					if ri == 0 {
						f.Weight = font.SemiBold
					}
					cx := x + dp(8)
					end := x + colW[ci] - dp(8)
					before := len(l.lines)
					cy := wrap(cell, cx, max(cx+1, end), top+dp(8), dp(20), 13, f, roleBody, false)
					bottom = max(bottom, cy+dp(8))
					for li := before; li < len(l.lines); li++ {
						ln := &l.lines[li]
						if len(ln.runs) == 0 {
							continue
						}
						last := ln.runs[len(ln.runs)-1]
						used := last.x + last.w - cx
						shift := 0
						switch b.Table.Align[ci] {
						case mdoc.AlignCenter:
							shift = max(0, (end-cx-used)/2)
						case mdoc.AlignRight:
							shift = max(0, end-cx-used)
						}
						for r := range ln.runs {
							ln.runs[r].x += shift
						}
					}
					x += colW[ci]
				}
				if ri == 0 {
					l.boxes = append(l.boxes, docBox{image.Rect(x0, top, x0+available, bottom), boxTableHead})
				}
				x = x0
				for _, w := range colW {
					l.boxes = append(l.boxes, docBox{image.Rect(x, top, x+1, bottom), boxTableLine})
					x += w
				}
				l.boxes = append(l.boxes, docBox{image.Rect(x-1, top, x, bottom), boxTableLine}, docBox{image.Rect(x0, top, x, top+1), boxTableLine}, docBox{image.Rect(x0, bottom-1, x, bottom), boxTableLine})
				y = bottom
			}
			y += dp(10)
		default:
			if b.Kind == mdoc.Item {
				x0 = min(x0+dp(22), max(0, width-dp(24)))
				if b.Task != 0 {
					kind := boxTaskOpen
					if b.Task == 2 {
						kind = boxTaskDone
					}
					l.boxes = append(l.boxes, docBox{image.Rect(max(0, x0-dp(20)), y+dp(4), max(0, x0-dp(20))+dp(12), y+dp(16)), kind})
				} else {
					mw := 0
					for _, q := range b.Marker {
						mw += win.runeAdv(gtx, font.Font{Typeface: sansFace}, 14, q).Ceil()
					}
					l.lines = append(l.lines, docLine{y: y, h: dp(22), runs: []docRun{{text: b.Marker, x: max(0, x0-dp(8)-mw), w: mw, face: font.Font{Typeface: sansFace}, size: 14, role: roleMuted, link: -1}}})
				}
			}
			role := roleBody
			if b.Quote > 0 {
				role = roleMuted
			}
			y = wrap(b.Spans, x0, width, y, dp(22), 14, font.Font{Typeface: sansFace}, role, false)
			y += dp(10)
		}
		if b.Quote > 0 {
			l.boxes = append(l.boxes, docBox{image.Rect(max(0, indent-dp(12)), start, max(1, indent-dp(9)), y-dp(10)), boxQuote})
		}
	}
	sort.SliceStable(l.lines, func(i, j int) bool { return l.lines[i].y < l.lines[j].y })
	sort.SliceStable(l.boxes, func(i, j int) bool { return l.boxes[i].r.Min.Y < l.boxes[j].r.Min.Y })
	l.height = y
	d.lay = l
	return &d.lay
}
