// Package render draws a terminal session into a Gio window.
//
// The grid is drawn on the CPU into horizontal image strips (the fastest method the
// speed spikes found). Each strip is one paint.ImageOp; a strip only gets a new op,
// and so a new GPU upload, when one of its rows changed.
package render

import (
	"image"
	"image/color"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

const (
	// stripCount trades upload size on local changes against per-texture cost under streams.
	stripCount = 8
	// syncHold is how long a synchronized update may hold back drawing before it is drawn anyway.
	syncHold = 150 * time.Millisecond
)

// Stats counts drawing work; tests and the bench harness read it.
type Stats struct {
	FramesDrawn    int // frames that were not held back by synchronized output
	RowsRedrawn    int // rows with at least one cell rasterized
	GlyphsCached   int // distinct glyph masks rasterized
	StripsUploaded int // strips given a new image (each one a GPU upload)
}

type strip struct {
	img      *image.RGBA
	op       paint.ImageOp
	row0     int
	nrows    int
	dirty    bool
	hasImage bool
}

type cursorState struct {
	col, row int
	cell     screen.Cell
	visible  bool
	focused  bool
}

// Renderer draws one session. It is used from the UI goroutine only.
type Renderer struct {
	pal    Palette
	fs     *fontSet
	glyphs *glyphCache
	cellW  int
	cellH  int

	cols, rows   int
	full         bool          // next snapshot copies every row
	viewOffset   int           // the view offset the strips show
	needStrips   bool          // strips must be rebuilt for cols x rows, outside the lock
	drawn        []screen.Cell // what the strips show, row-major
	snap         []screen.Cell // rows copied out under the session lock
	snapRows     []int
	snapSel      []bool // per cell: selected in the snapshot
	drawnSel     []bool // per cell: drawn selected
	lastSel      Selection
	rowsPerStrip int
	strips       []strip

	focused   bool
	cur       cursorState
	curDrawn  cursorState
	curOp     paint.ImageOp
	curImg    *image.RGBA       // the image curOp shows
	curImgs   [2][2]*image.RGBA // [narrow, wide] x two, alternated so the shown one is never redrawn
	curFlip   int
	edges     [4]image.Rectangle // outline cursor scratch, kept to avoid a per-frame allocation
	holdUntil time.Time

	fixed map[uint64]color.NRGBA // readable text colors by packed (fg, bg)
	defFG color.NRGBA            // readable(pal.FG, pal.BG, ink): most cells, skips the map
	ink   color.NRGBA            // inkOf(pal), the dark end readable pulls toward

	now   func() time.Time // replaceable so tests can drive the sync timeout
	stats Stats
}

// NewRenderer builds a renderer for a font size in pixels (the caller applies DPI scale).
// Renderers of the same size share fonts and glyphs.
func NewRenderer(fontSize float32, palette Palette) *Renderer {
	g := sharedGlyphs(fontSize)
	r := &Renderer{
		pal:     palette,
		fs:      g.fs,
		glyphs:  g,
		cellW:   g.fs.cellW,
		cellH:   g.fs.cellH,
		focused: true,
		now:     time.Now,
		ink:     inkOf(palette),
		defFG:   readable(palette.FG, palette.BG, inkOf(palette)),
	}
	for i := range r.curImgs {
		r.curImgs[i][0] = image.NewRGBA(image.Rect(0, 0, r.cellW, r.cellH))
		r.curImgs[i][1] = image.NewRGBA(image.Rect(0, 0, 2*r.cellW, r.cellH))
	}
	return r
}

// Release drops the strip images and what they showed, for a tab that is not
// visible; the next Layout rebuilds them.
func (r *Renderer) Release() {
	r.cols, r.rows = 0, 0
	r.strips, r.drawn, r.snap, r.snapRows = nil, nil, nil, nil
	r.snapSel, r.drawnSel, r.lastSel = nil, nil, Selection{}
	r.curImg, r.curDrawn = nil, cursorState{}
}

// CellSize is one cell in pixels, for computing cols and rows from a window size.
func (r *Renderer) CellSize() (w, h int) { return r.cellW, r.cellH }

// SetFocused switches the cursor between a block (focused) and an outline.
func (r *Renderer) SetFocused(f bool) { r.focused = f }

func (r *Renderer) Stats() Stats {
	st := r.stats
	st.GlyphsCached = r.glyphs.len()
	return st
}

// Layout draws the session. sel is the current selection, or nil for none.
func (r *Renderer) Layout(gtx layout.Context, s *term.Session, sel *Selection) layout.Dimensions {
	held := r.snapshot(s, sel)
	if r.needStrips {
		r.buildStrips()
	}
	if held {
		gtx.Execute(op.InvalidateCmd{At: r.holdUntil}) // draw anyway once the hold runs out
	} else {
		r.rasterizeDirty()
		r.stats.FramesDrawn++
	}
	r.paint(gtx)
	return layout.Dimensions{Size: image.Pt(r.cols*r.cellW, r.rows*r.cellH)}
}

// snapshot copies the dirty rows and cursor out under the lock, so the reader
// goroutine is not blocked while they are rasterized. It reports true when a
// synchronized update holds this frame back.
func (r *Renderer) snapshot(s *term.Session, sel *Selection) (held bool) {
	s.Lock()
	defer s.Unlock()
	a, scr := s.Applier(), s.Screen()
	m := a.Modes()
	// Hold only when something is already drawn; with no strips (first frame, or
	// right after Release) holding would show just the background.
	drawnAny := r.cols != 0 && len(r.strips) != 0
	if since := a.SyncSince(); drawnAny && m.Synchronized && !since.IsZero() && r.now().Sub(since) < syncHold {
		r.holdUntil = since.Add(syncHold)
		return true
	}

	cols, rows := scr.Size()
	if cols != r.cols || rows != r.rows {
		r.resize(cols, rows)
	}
	dirty := scr.TakeDirty()
	r.snapRows = r.snapRows[:0]
	if off := scr.ViewOffset(); off != r.viewOffset {
		r.viewOffset, r.full = off, true
	}
	// A scrolled-back view shows history, which dirty flags don't track; the cell diff keeps a full copy cheap.
	if r.full || r.viewOffset > 0 {
		r.full = false
		for row := 0; row < rows; row++ {
			r.snapRows = append(r.snapRows, row)
		}
	} else {
		r.snapRows = append(r.snapRows, dirty...)
	}
	base := scr.ScrollbackLen() - scr.ViewOffset() // absolute row of visible row 0
	cur := Selection{}
	if sel != nil && sel.Active {
		cur = *sel
	}
	if cur != r.lastSel {
		r.addSelectionRows(r.lastSel, cur, base)
		r.lastSel = cur
	}
	for _, row := range r.snapRows {
		line := r.snap[row*cols : (row+1)*cols]
		sline := r.snapSel[row*cols : (row+1)*cols]
		for c := range line {
			line[c] = scr.VisibleCell(c, row)
			sline[c] = cur.Contains(Point{c, base + row})
		}
		for c := range line {
			// A wide char is selected as a whole, whichever half the selection touches.
			if line[c].Width == 2 && c+1 < cols {
				sline[c] = sline[c] || sline[c+1]
				sline[c+1] = sline[c]
			}
		}
	}

	col, row := scr.Cursor()
	cell := scr.VisibleCell(col, row)
	if cell.Width == 0 && col > 0 && scr.VisibleCell(col-1, row).Width == 2 {
		col--
		cell = scr.VisibleCell(col, row)
	}
	r.cur = cursorState{col: col, row: row, cell: cell,
		visible: m.CursorVisible && scr.ViewOffset() == 0, focused: r.focused}
	return false
}

// addSelectionRows queues the visible rows covered by the old and new selection for redraw.
func (r *Renderer) addSelectionRows(old, cur Selection, base int) {
	queued := make([]bool, r.rows)
	for _, row := range r.snapRows {
		queued[row] = true
	}
	for _, sel := range []Selection{old, cur} {
		if !sel.Active {
			continue
		}
		lo, hi := sel.bounds()
		for row := max(lo.Row-base, 0); row <= min(hi.Row-base, r.rows-1); row++ {
			if !queued[row] {
				queued[row] = true
				r.snapRows = append(r.snapRows, row)
			}
		}
	}
}

// resize runs under the session lock, so it only sizes the cell copy; the strip
// images, the large allocation, are made by buildStrips after the lock is released.
func (r *Renderer) resize(cols, rows int) {
	r.cols, r.rows = cols, rows
	r.full, r.needStrips = true, true
	r.snap = make([]screen.Cell, cols*rows)
	r.snapSel = make([]bool, cols*rows)
}

func (r *Renderer) buildStrips() {
	cols, rows := r.cols, r.rows
	r.needStrips = false
	r.drawn = make([]screen.Cell, cols*rows)
	for i := range r.drawn {
		r.drawn[i].Rune = -1 // matches no real cell, so every cell is drawn once
	}
	r.drawnSel = make([]bool, cols*rows)
	r.rowsPerStrip = (rows + stripCount - 1) / stripCount
	r.strips = r.strips[:0]
	for row0 := 0; row0 < rows; row0 += r.rowsPerStrip {
		n := min(r.rowsPerStrip, rows-row0)
		r.strips = append(r.strips, strip{
			img:  image.NewRGBA(image.Rect(0, 0, cols*r.cellW, n*r.cellH)),
			row0: row0, nrows: n,
		})
	}
	r.curDrawn = cursorState{}
}

// rasterizeDirty draws the snapshot's changed cells into their strips and gives each
// changed strip a new image op. Clean strips keep their op and so their GPU texture.
func (r *Renderer) rasterizeDirty() {
	cols := r.cols
	for _, row := range r.snapRows {
		now := r.snap[row*cols : (row+1)*cols]
		was := r.drawn[row*cols : (row+1)*cols]
		selNow := r.snapSel[row*cols : (row+1)*cols]
		selWas := r.drawnSel[row*cols : (row+1)*cols]
		st := &r.strips[row/r.rowsPerStrip]
		y := (row - st.row0) * r.cellH
		changed := false
		for c := 0; c < cols; c++ {
			cell := now[c]
			diff := cell != was[c] || selNow[c] != selWas[c]
			if cell.Width == 2 && c+1 < cols {
				diff = diff || now[c+1] != was[c+1] // the lead owns its right half's pixels
			}
			if cell.Width == 0 && c > 0 && now[c-1].Width == 2 {
				continue
			}
			if !diff {
				continue
			}
			var over *colors
			if selNow[c] {
				over = &colors{fg: r.selectedFG(cell), bg: r.pal.Selection}
			}
			r.drawCell(st.img, c*r.cellW, y, cell, over)
			changed = true
		}
		copy(was, now)
		copy(selWas, selNow)
		if changed {
			r.stats.RowsRedrawn++
			st.dirty = true
		}
	}
	for i := range r.strips {
		st := &r.strips[i]
		if !st.dirty {
			continue
		}
		// Gio keys GPU textures by op handle and cannot update one in place, so a changed strip needs a new op.
		st.op = paint.NewImageOp(st.img)
		st.op.Filter = paint.FilterNearest
		st.dirty, st.hasImage = false, true
		r.stats.StripsUploaded++
	}
	r.updateCursor()
}

// colors overrides the cell's own colors (the block cursor draws inverted).
type colors struct{ fg, bg color.NRGBA }

// drawCell composites one cell (two for a wide char) at x,y: background, glyph, then lines.
func (r *Renderer) drawCell(img *image.RGBA, x, y int, cell screen.Cell, over *colors) {
	var fg, bg color.NRGBA
	if over != nil {
		fg, bg = over.fg, over.bg
	} else {
		fg, bg = r.colorsOf(cell)
	}
	cells := 1
	if cell.Width == 2 {
		cells = 2
	}
	w := min(cells*r.cellW, img.Rect.Dx()-x)
	h := r.cellH
	fillRect(img, x, y, w, h, bg)

	if cell.Rune > ' ' && cell.Width != 0 {
		g := r.glyphs.get(cell.Rune, cell.Attrs&screen.AttrBold != 0, cells == 2)
		if !g.empty {
			blendMask(img, x, y, w, h, g.mask, fg, bg)
		}
	}
	if cell.Attrs&screen.AttrUnderline != 0 {
		fillRect(img, x, y+r.fs.underlineY, w, 1, fg)
	}
	if cell.Attrs&screen.AttrStrike != 0 {
		fillRect(img, x, y+r.fs.strikeY, w, 1, fg)
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.NRGBA) {
	if w <= 0 {
		return
	}
	row := img.Pix[img.PixOffset(x, y):]
	for i := 0; i < w; i++ {
		row[i*4], row[i*4+1], row[i*4+2], row[i*4+3] = c.R, c.G, c.B, 0xff
	}
	first := row[:w*4]
	for j := 1; j < h; j++ {
		copy(img.Pix[img.PixOffset(x, y+j):], first)
	}
}

// blendMask mixes fg over the opaque bg by the mask's coverage.
func blendMask(img *image.RGBA, x, y, w, h int, m *image.Alpha, fg, bg color.NRGBA) {
	for j := 0; j < h; j++ {
		src := m.Pix[j*m.Stride : j*m.Stride+w]
		dst := img.Pix[img.PixOffset(x, y+j):]
		for i, a := range src {
			if a == 0 {
				continue
			}
			k := uint32(a)
			o := i * 4
			dst[o] = uint8((uint32(fg.R)*k + uint32(bg.R)*(255-k) + 127) / 255)
			dst[o+1] = uint8((uint32(fg.G)*k + uint32(bg.G)*(255-k) + 127) / 255)
			dst[o+2] = uint8((uint32(fg.B)*k + uint32(bg.B)*(255-k) + 127) / 255)
		}
	}
}

// updateCursor redraws the block cursor's own small image only when it changed,
// so a cursor change never re-uploads a strip.
func (r *Renderer) updateCursor() {
	c := r.cur
	if c == r.curDrawn && r.curImg != nil {
		return
	}
	r.curDrawn = c
	if !c.visible || !c.focused {
		return // hidden, or an outline drawn as plain ops in paint
	}
	cells := 1
	if c.cell.Width == 2 {
		cells = 2
	}
	r.curFlip ^= 1
	r.curImg = r.curImgs[r.curFlip][cells-1]
	r.drawCell(r.curImg, 0, 0, c.cell, &colors{fg: r.pal.BG, bg: r.pal.Cursor})
	r.curOp = paint.NewImageOp(r.curImg)
	r.curOp.Filter = paint.FilterNearest
}

func (r *Renderer) paint(gtx layout.Context) {
	ops := gtx.Ops
	paint.FillShape(ops, r.pal.BG, clip.Rect{Max: gtx.Constraints.Max}.Op()) // margins past the grid
	for i := range r.strips {
		st := &r.strips[i]
		if !st.hasImage {
			continue
		}
		t := op.Offset(image.Pt(0, st.row0*r.cellH)).Push(ops)
		st.op.Add(ops)
		paint.PaintOp{}.Add(ops)
		t.Pop()
	}

	c := r.curDrawn
	if !c.visible || c.row >= r.rows || c.col >= r.cols {
		return
	}
	p := image.Pt(c.col*r.cellW, c.row*r.cellH)
	if c.focused && r.curImg != nil {
		t := op.Offset(p).Push(ops)
		r.curOp.Add(ops)
		paint.PaintOp{}.Add(ops)
		t.Pop()
		return
	}
	w := r.cellW
	if c.cell.Width == 2 {
		w *= 2
	}
	box := image.Rectangle{Min: p, Max: p.Add(image.Pt(w, r.cellH))}
	r.edges = [4]image.Rectangle{
		{Min: box.Min, Max: image.Pt(box.Max.X, box.Min.Y+1)},
		{Min: image.Pt(box.Min.X, box.Max.Y-1), Max: box.Max},
		{Min: box.Min, Max: image.Pt(box.Min.X+1, box.Max.Y)},
		{Min: image.Pt(box.Max.X-1, box.Min.Y), Max: box.Max},
	}
	for _, edge := range r.edges {
		paint.FillShape(ops, r.pal.Cursor, clip.Rect(edge).Op())
	}
}
