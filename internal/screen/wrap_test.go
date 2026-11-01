package screen

import "testing"

func printStr(s *Screen, text string) {
	for _, r := range text {
		s.Print(r)
	}
}

// A row that auto-wrapped marks its last cell; an explicit newline does not.
func TestAutoWrapMarksTheRow(t *testing.T) {
	s := New(5, 3)
	printStr(s, "abcdefg")
	if s.Cell(4, 0).Attrs&AttrWrapped == 0 {
		t.Fatal("wrapped row not marked")
	}
	if s.Cell(1, 1).Attrs&AttrWrapped != 0 || s.Cell(4, 1).Attrs&AttrWrapped != 0 {
		t.Fatal("unwrapped row marked")
	}
	s = New(5, 3)
	printStr(s, "abcde")
	s.CarriageReturn()
	s.LineFeed()
	printStr(s, "fg")
	if s.Cell(4, 0).Attrs&AttrWrapped != 0 {
		t.Fatal("a full row ended by a newline was marked wrapped")
	}
}

func TestWideCharWrapMarksTheRow(t *testing.T) {
	s := New(5, 2)
	printStr(s, "abcd界")
	if s.Cell(4, 0).Attrs&AttrWrapped == 0 || s.Cell(0, 1).Rune != '界' {
		t.Fatalf("wide wrap: %+v %+v", s.Cell(4, 0), s.Cell(0, 1))
	}
}

func TestWrapMarkSurvivesIntoScrollbackAndClearsOnOverwrite(t *testing.T) {
	s := NewWithScrollback(5, 2, 10)
	printStr(s, "abcdefghijklm")
	if s.ScrollbackLen() == 0 {
		t.Fatal("setup: nothing scrolled")
	}
	line := s.ScrollbackLine(0)
	if len(line) != 5 || line[4].Attrs&AttrWrapped == 0 {
		t.Fatalf("scrollback lost the mark: %+v", line)
	}
	s = New(5, 2)
	printStr(s, "abcdef")
	s.SetCursor(4, 0)
	s.Print('Z')
	if s.Cell(4, 0).Attrs&AttrWrapped != 0 {
		t.Fatal("overwriting the last cell kept the mark")
	}
}
