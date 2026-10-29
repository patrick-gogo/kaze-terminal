package render

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func segoeSymbol(t *testing.T) string {
	t.Helper()
	if runtime.GOOS != "windows" {
		t.Skip("the fallback symbol test uses the Windows font list")
	}
	p := filepath.Join(os.Getenv("WINDIR"), "Fonts", "seguisym.ttf")
	if _, err := os.Stat(p); err != nil {
		t.Skip("Segoe UI Symbol is not installed")
	}
	return p
}

func TestFallbackDrawsClaudeSymbols(t *testing.T) {
	segoeSymbol(t)
	c := NewRenderer(15, DefaultPalette(), Options{}).glyphs
	w, h := c.fs.cellW, c.fs.cellH
	notdef := c.get(notdefProbe, false, false)
	for _, r := range []rune{'⏺', '⎿', '✻', '✔'} {
		g := c.get(r, false, false)
		if g.face == faceMissing || g == notdef {
			t.Errorf("%U resolved to .notdef", r)
			continue
		}
		if g.empty {
			t.Errorf("%U drew no ink", r)
		}
		if b := g.mask.Bounds(); b.Dx() != w || b.Dy() != h {
			t.Errorf("%U mask is %v, want one %dx%d cell", r, b, w, h)
		}
	}
}

func TestRuneMissingEverywhereIsNotdef(t *testing.T) {
	c := NewRenderer(16, DefaultPalette(), Options{}).glyphs
	a := c.get(0x10FFFD, false, false) // plane 16 private use: no font maps it
	b := c.get(0xE01EF, false, false)  // variation selector supplement: never drawn by these fonts
	if a.face != faceMissing || a != b {
		t.Fatalf("missing runes: faces %d/%d, same mask %v", a.face, b.face, a == b)
	}
}

func TestSystemFontParsedOnlyOnMiss(t *testing.T) {
	path := segoeSymbol(t)
	sys := &sysFonts{paths: []string{filepath.Join(t.TempDir(), "absent.ttf"), path}}
	fs := newFontSet(13, 1)
	fs.sys = sys
	c := &glyphCache{fs: fs, m: make(map[glyphKey]*glyph)}
	for _, r := range []rune{'A', '日', '─', '→'} {
		c.get(r, false, false)
	}
	if sys.parses != 0 {
		t.Fatalf("%d system fonts parsed before any rune missed the bundled fonts", sys.parses)
	}
	if g := c.get('⏺', false, false); g.face == faceMissing {
		t.Fatal("⏺ not found in Segoe UI Symbol")
	}
	c.get('✻', false, false)
	c.get(0x10FFFD, false, false)
	if sys.parses != 1 {
		t.Fatalf("parsed %d system fonts, want exactly 1 (the absent file is skipped)", sys.parses)
	}
}

func TestFallbackLookupIsMemoized(t *testing.T) {
	path := segoeSymbol(t)
	sys := &sysFonts{paths: []string{path}}
	fs := newFontSet(13, 1)
	fs.sys = sys
	c := &glyphCache{fs: fs, m: make(map[glyphKey]*glyph)}
	for _, r := range []rune{'⏺', '✻', 0x10FFFD, '日'} {
		c.get(r, false, false)
		calls, lookups := sys.calls, fs.lookups
		for i := 0; i < 3; i++ {
			c.get(r, false, false)
			c.get(r, true, false)
		}
		if sys.calls != calls || fs.lookups != lookups {
			t.Errorf("%U redrawn: %d more sys.font calls, %d more cmap lookups, want 0",
				r, sys.calls-calls, fs.lookups-lookups)
		}
	}
}

func TestBadSystemFontIsSkipped(t *testing.T) {
	path := segoeSymbol(t)
	dir := t.TempDir()
	garbage := filepath.Join(dir, "garbage.ttf")
	if err := os.WriteFile(garbage, []byte("true\x00\x10not a font at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	truncated := filepath.Join(dir, "truncated.ttf")
	if err := os.WriteFile(truncated, good[:min(len(good), 4096)], 0o644); err != nil {
		t.Fatal(err)
	}
	panicky := filepath.Join(dir, "panicky.ttf")
	if err := os.WriteFile(panicky, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sys := &sysFonts{paths: []string{garbage, truncated, panicky, path}}
	sys.parse = func(r io.ReaderAt) (*sfnt.Collection, error) {
		if f, ok := r.(*os.File); ok && filepath.Base(f.Name()) == "panicky.ttf" {
			panic("parser bug")
		}
		return sfnt.ParseCollectionReaderAt(r)
	}
	fs := newFontSet(13, 1)
	fs.sys = sys
	c := &glyphCache{fs: fs, m: make(map[glyphKey]*glyph)}
	if g := c.get('⏺', false, false); g.face == faceMissing || g.empty {
		t.Fatalf("⏺ not drawn from the good font after bad ones: face %d", g.face)
	}
	if g := c.get(0x10FFFD, false, false); g.face != faceMissing {
		t.Fatalf("missing rune got face %d", g.face)
	}
}

func TestSystemFontForReportsCoveringFile(t *testing.T) {
	if !BundledCovers('a') || !BundledCovers('日') {
		t.Fatal("bundled fonts should cover Latin and Japanese")
	}
	if BundledCovers('✳') {
		t.Skip("bundled font covers the symbol")
	}
	p := SystemFontFor('✳')
	if p == "" {
		t.Skip("no system font with the symbol on this machine")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("returned path does not exist: %v", err)
	}
	if SystemFontFor(0x10FFFF) != "" {
		t.Fatal("a noncharacter should have no font")
	}
}

// Kanji outside the bundled cp932 subset must come from a system Japanese font, not draw as a box.
func TestRareKanjiFallsBackToSystemJapaneseFont(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the rare-kanji test uses the Windows font list")
	}
	p := filepath.Join(os.Getenv("WINDIR"), "Fonts", "YuGothM.ttc")
	if _, err := os.Stat(p); err != nil {
		t.Skip("Yu Gothic Medium is not installed")
	}
	c := NewRenderer(15, DefaultPalette(), Options{}).glyphs
	for _, r := range []rune{0x20B9F, 0x4FF1} { // 𠮟 and 俱: JIS X 0213, not in cp932
		if BundledCovers(r) {
			t.Errorf("%U is in the bundled fonts; pick a rarer kanji", r)
			continue
		}
		g := c.get(r, false, true)
		if g.face < faceSystem {
			t.Errorf("%U came from face %d, want a system font", r, g.face)
			continue
		}
		if g.empty {
			t.Errorf("%U drew no ink", r)
		}
		if got := filepath.Base(systemFonts.path(int(g.face - faceSystem))); !strings.EqualFold(got, "YuGothM.ttc") {
			t.Errorf("%U came from %s, want YuGothM.ttc", r, got)
		}
		if b := g.mask.Bounds(); b.Dx() != 2*c.fs.cellW {
			t.Errorf("%U mask is %d wide, want two cells (%d)", r, b.Dx(), 2*c.fs.cellW)
		}
	}
}
