package ui

import (
	"os"
	"path/filepath"
	"testing"

	"gioui.org/font"
	"gioui.org/text"

	"github.com/patrick-gogo/kaze-terminal/assets/fonts"
	"github.com/patrick-gogo/kaze-terminal/internal/render"
)

// missingGlyphs counts glyphs the shaper drew as .notdef (ID 0) for s.
func missingGlyphs(c *chrome, s string) int {
	c.shaper.LayoutString(text.Parameters{Font: font.Font{Typeface: chromeFace}, PxPerEm: 16, MaxLines: 1, MaxWidth: 1 << 20}, s)
	n := 0
	for {
		g, ok := c.shaper.NextGlyph()
		if !ok {
			return n
		}
		if g.ID&0xffffffff == 0 { // low 32 bits hold the font glyph index; the face sits above
			n++
		}
	}
}

func TestFitKeepsJapaneseAndLatin(t *testing.T) {
	c := newChrome()
	if got := c.fit("日本語 abc"); got != "日本語 abc" {
		t.Fatalf("got %q", got)
	}
	if len(c.loaded) != 0 {
		t.Fatal("system fonts loaded for text the bundled fonts cover")
	}
}

func TestFitDrawsSymbolWithoutMissingGlyph(t *testing.T) {
	if render.BundledCovers('✳') || render.SystemFontFor('✳') == "" {
		t.Skip("no system font with the symbol")
	}
	c := newChrome()
	got := c.fit("✳ Claude Code")
	if got != "✳ Claude Code" {
		t.Fatalf("symbol dropped: %q", got)
	}
	if n := missingGlyphs(c, got); n != 0 {
		t.Fatalf("%d missing glyphs", n)
	}
}

func TestFitDropsRunesNoFontHas(t *testing.T) {
	c := newChrome()
	if got := c.fit("a\U0010FFFFb\x07c"); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestFitRebuildsShaperOnlyForNewFont(t *testing.T) {
	// Two symbols; skip unless one system file covers both.
	a, b := '✳', '✴'
	pa, pb := render.SystemFontFor(a), render.SystemFontFor(b)
	if render.BundledCovers(a) || render.BundledCovers(b) || pa == "" || pa != pb {
		t.Skip("no single system font covers both symbols")
	}
	c := newChrome()
	c.fit(string(a))
	sh := c.shaper
	c.fit(string(b))
	if c.shaper != sh {
		t.Fatal("shaper rebuilt although the font was already loaded")
	}
}

func TestFitCoveredTextIsUnchangedWithoutAllocating(t *testing.T) {
	c := newChrome()
	c.fit("日本語 abc")
	if n := testing.AllocsPerRun(50, func() { c.fit("日本語 abc") }); n != 0 {
		t.Fatalf("%v allocs", n)
	}
}

func TestAddFontSkipsOversizedFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "mono.ttf")
	if err := os.WriteFile(f, fonts.MonoRegular, 0o600); err != nil {
		t.Fatal(err)
	}
	old := maxFontFile
	defer func() { maxFontFile = old }()

	maxFontFile = int64(len(fonts.MonoRegular)) - 1
	if ok, fresh := newChrome().addFont(f); ok || fresh {
		t.Fatal("a valid font over the cap was loaded")
	}
	maxFontFile = int64(len(fonts.MonoRegular)) + 1
	if ok, fresh := newChrome().addFont(f); !ok || !fresh {
		t.Fatal("the same font under the cap was rejected")
	}
}

func TestFitCoveredCacheIsBounded(t *testing.T) {
	c := newChrome()
	for r := rune(0x4e00); r < 0x4e00+maxCovered+50; r++ {
		c.fit(string(r))
	}
	if len(c.covered) > maxCovered {
		t.Fatalf("cache %d", len(c.covered))
	}
}

func TestFitWhitespaceControlsBecomeSpaces(t *testing.T) {
	c := newChrome()
	if got := c.fit("a\tb\nc\rd"); got != "a b c d" {
		t.Fatalf("got %q", got)
	}
}

func TestFitDropsFormatAndLineSeparators(t *testing.T) {
	c := newChrome()
	in := "a\u202Eb\u2066c\u2069d\u200Be\u2028f\u2029g"
	if got := c.fit(in); got != "abcdefg" {
		t.Fatalf("got %q", got)
	}
}
