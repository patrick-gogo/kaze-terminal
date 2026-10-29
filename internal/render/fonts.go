package render

import (
	"image"
	"io"
	"math"
	"os"
	"sync"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/patrick-gogo/kaze-terminal/assets/fonts"
)

type faceID uint8

const (
	faceMono faceID = iota
	faceMonoBold
	faceCJK
	faceMissing // in neither font: every such rune shares one .notdef mask
	faceBox     // box drawing and block elements, drawn procedurally
	faceSystem  // first system fallback font; faceSystem+i is systemFonts font i
)

// sysFonts parses the system fallback fonts lazily, each file at most once, for every size.
type sysFonts struct {
	mu     sync.Mutex
	paths  []string
	next   int // paths[next:] are not opened yet
	fonts  []*sfnt.Font
	srcs   []string // srcs[i] is the file fonts[i] came from
	parses int      // files parsed, read by tests
	calls  int      // font() calls, read by tests

	// parse reads one file; nil means sfnt.ParseCollectionReaderAt. Tests swap it.
	parse func(io.ReaderAt) (*sfnt.Collection, error)
}

var systemFonts = &sysFonts{paths: systemFontPaths()}

// maxSystemFonts keeps faceSystem+i inside faceID; a .ttc can hold many fonts.
const maxSystemFonts = 32

// font returns fallback font i, opening more files until it exists; nil once the list is exhausted.
func (s *sysFonts) font(i int) *sfnt.Font {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	for i >= len(s.fonts) && s.next < len(s.paths) && len(s.fonts) < maxSystemFonts {
		s.next++
		s.load(s.paths[s.next-1])
	}
	if i < len(s.fonts) {
		return s.fonts[i]
	}
	return nil
}

// load parses one file and appends its fonts. A missing, malformed or parser-crashing
// file adds nothing: a bad system font must cost one symbol, not the terminal.
func (s *sysFonts) load(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // not installed on this machine
	}
	s.parses++
	before := len(s.fonts)
	defer func() {
		if recover() != nil {
			s.fonts = s.fonts[:before]
			s.srcs = s.srcs[:before]
		}
		if len(s.fonts) == before {
			f.Close()
		}
	}()
	// Read through the open file rather than into memory: Segoe UI Emoji alone is ~10MB.
	parse := s.parse
	if parse == nil {
		parse = sfnt.ParseCollectionReaderAt
	}
	c, err := parse(f)
	if err != nil {
		return
	}
	for j := 0; j < c.NumFonts() && len(s.fonts) < maxSystemFonts; j++ {
		if ft, ferr := c.Font(j); ferr == nil {
			s.fonts = append(s.fonts, ft)
			s.srcs = append(s.srcs, path)
		}
	}
}

// hasRune reports whether a system font maps r, treating a panic in its cmap as no.
func hasRune(f *sfnt.Font, b *sfnt.Buffer, r rune) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	i, _ := f.GlyphIndex(b, r)
	return i != 0
}

// sysFaceKey names one system font at one pixel size.
type sysFaceKey struct {
	i  int
	px float64
}

// sysFaceCap bounds the per-size cache of system font faces (shrunk sizes add entries).
const sysFaceCap = 64

// fontSet holds the parsed bundled fonts at one pixel size.
type fontSet struct {
	mono, cjk *sfnt.Font
	faces     [3]xfont.Face
	buf       sfnt.Buffer
	sys       *sysFonts
	px        float64
	lookups   int // cmap lookups, read by tests
	sysFaces  map[sysFaceKey]xfont.Face

	cellW, cellH int
	ascent       int // baseline offset from the top of a cell
	underlineY   int
	strikeY      int
}

func mustParse(b []byte) *sfnt.Font {
	f, err := opentype.Parse(b)
	if err != nil {
		panic("render: bundled font does not parse: " + err.Error()) // embedded at build time, so this is a build bug
	}
	return f
}

func mustFace(f *sfnt.Font, px float64) xfont.Face {
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: xfont.HintingFull})
	if err != nil {
		panic("render: " + err.Error())
	}
	return face
}

// newFontSet sizes everything from the mono font; px is the font size in pixels and
// scale the line spacing, which changes only the cell height.
func newFontSet(px, scale float64) *fontSet {
	fs := &fontSet{mono: mustParse(fonts.MonoRegular), cjk: mustParse(fonts.CJK), sys: systemFonts, px: px}
	fs.faces[faceMono] = mustFace(fs.mono, px)
	fs.faces[faceMonoBold] = mustFace(mustParse(fonts.MonoBold), px)

	m := fs.faces[faceMono].Metrics()
	adv, _ := fs.faces[faceMono].GlyphAdvance('M')
	fs.cellW = max(adv.Ceil(), 1)
	base := max((m.Ascent + m.Descent).Ceil(), 1)
	fs.cellH = max(int(math.Round(float64(base)*scale)), 1)
	fs.ascent = m.Ascent.Ceil() + (fs.cellH-base)/2 // the text sits centered in the extra space
	fs.underlineY = min(fs.ascent+1, fs.cellH-1)
	fs.strikeY = max(fs.ascent-m.XHeight.Round()/2, 0)

	// Shrink the CJK face if its full-width glyphs would overflow two cells.
	cjkPx := px
	probe := mustFace(fs.cjk, px)
	if a, ok := probe.GlyphAdvance('日'); ok && a > fixed.I(2*fs.cellW) {
		cjkPx = px * float64(2*fs.cellW) / (float64(a) / 64)
	}
	fs.faces[faceCJK] = mustFace(fs.cjk, cjkPx)
	return fs
}

// faceFor picks the mono face when it has the rune, else the CJK face, else the first
// system font that has it (loading them only now), else faceMissing.
func (fs *fontSet) faceFor(r rune, bold bool) faceID {
	mono := faceMono
	if bold {
		mono = faceMonoBold
	}
	fs.lookups++
	if i, _ := fs.mono.GlyphIndex(&fs.buf, r); i != 0 {
		return mono
	}
	if i, _ := fs.cjk.GlyphIndex(&fs.buf, r); i != 0 {
		return faceCJK
	}
	for i := 0; ; i++ {
		f := fs.sys.font(i)
		if f == nil {
			return faceMissing
		}
		if hasRune(f, &fs.buf, r) {
			return faceSystem + faceID(i)
		}
	}
}

// sysFace returns system font i at px, made once per (font, size).
func (fs *fontSet) sysFace(i int, px float64) xfont.Face {
	k := sysFaceKey{i, px}
	if face, ok := fs.sysFaces[k]; ok {
		return face
	}
	f := fs.sys.font(i)
	if f == nil {
		return nil
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: px, DPI: 72, Hinting: xfont.HintingFull})
	if err != nil {
		return nil
	}
	if fs.sysFaces == nil || len(fs.sysFaces) >= sysFaceCap {
		fs.sysFaces = make(map[sysFaceKey]xfont.Face)
	}
	fs.sysFaces[k] = face
	return face
}

// drawFallback draws r from system font i into a w-wide cell mask, shrunk to fit and
// centered: their metrics differ from the mono font's, and ink must not spill into neighbors.
func (fs *fontSet) drawFallback(i int, r rune, w int) (mask *image.Alpha) {
	h := fs.cellH
	mask = image.NewAlpha(image.Rect(0, 0, w, h))
	defer func() {
		if recover() != nil {
			mask = image.NewAlpha(image.Rect(0, 0, w, h)) // a broken glyph draws blank
		}
	}()
	face := fs.sysFace(i, fs.px)
	if face == nil {
		return mask
	}
	b, _, ok := face.GlyphBounds(r)
	if !ok {
		return mask
	}
	gw, gh := float64(b.Max.X-b.Min.X)/64, float64(b.Max.Y-b.Min.Y)/64
	if gw > float64(w) || gh > float64(h) {
		s := 0.95 * min(float64(w)/max(gw, 1), float64(h)/max(gh, 1))
		if smaller := fs.sysFace(i, fs.px*s); smaller != nil {
			face = smaller
			b, _, _ = face.GlyphBounds(r)
		}
	}
	x := (fixed.I(w) - b.Min.X - b.Max.X) / 2
	y := fixed.I(fs.ascent) // keep the mono baseline when the ink fits around it
	if y+b.Min.Y < 0 || y+b.Max.Y > fixed.I(h) {
		y = (fixed.I(h) - b.Min.Y - b.Max.Y) / 2
	}
	d := xfont.Drawer{Dst: mask, Src: image.Opaque, Face: face, Dot: fixed.P(x.Round(), y.Round())}
	d.DrawString(string(r))
	return mask
}

// path returns the file of fallback font i, "" when it does not exist.
func (s *sysFonts) path(i int) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= 0 && i < len(s.srcs) {
		return s.srcs[i]
	}
	return ""
}

var (
	bundledOnce sync.Once
	bundledMono *sfnt.Font
	bundledCJK  *sfnt.Font
)

// BundledCovers reports whether the bundled mono or CJK font has a glyph for r.
func BundledCovers(r rune) bool {
	bundledOnce.Do(func() { bundledMono, bundledCJK = mustParse(fonts.MonoRegular), mustParse(fonts.CJK) })
	var b sfnt.Buffer
	return hasRune(bundledMono, &b, r) || hasRune(bundledCJK, &b, r)
}

// SystemFontFor returns the file of the first system fallback font that has r, or ""
// if none does. It shares the terminal grid's lazy font list, so each file is parsed once.
func SystemFontFor(r rune) string {
	var b sfnt.Buffer
	for i := 0; ; i++ {
		f := systemFonts.font(i)
		if f == nil {
			return ""
		}
		if hasRune(f, &b, r) {
			return systemFonts.path(i)
		}
	}
}
