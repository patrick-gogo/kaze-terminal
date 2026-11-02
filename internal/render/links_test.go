package render

import (
	"strings"
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/screen"
)

func TestFindURLs(t *testing.T) {
	cases := []struct{ in, want string }{
		{"see https://github.com/x/y/pull/68 now", "https://github.com/x/y/pull/68"},
		{"http://localhost:3000/", "http://localhost:3000/"},
		{"(https://example.com/a_(b))", "https://example.com/a_(b)"},
		{"(https://example.com/a)", "https://example.com/a"},
		{"Go to https://example.com.", "https://example.com"},
		{"<https://example.com/q?a=1&b=2>", "https://example.com/q?a=1&b=2"},
		{`"https://example.com/x"`, "https://example.com/x"},
		{"│ https://example.com/x │", "https://example.com/x"},
		{"HTTPS://EXAMPLE.COM", "HTTPS://EXAMPLE.COM"},
		{"https://", ""},
		{"ftp://example.com", ""},
		{"xhttps://example.com", ""},
		{"no links here", ""},
	}
	for _, c := range cases {
		var got []string
		rs := []rune(c.in)
		for _, sp := range findLinks(rs) {
			got = append(got, string(rs[sp[0]:sp[1]]))
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFindURLsTwoOnALine(t *testing.T) {
	rs := []rune("a https://a.io b http://b.io/c")
	if sp := findLinks(rs); len(sp) != 2 || string(rs[sp[1][0]:sp[1][1]]) != "http://b.io/c" {
		t.Fatalf("got %v", sp)
	}
}

func TestLinkAtFindsTheURLUnderTheCell(t *testing.T) {
	s := screen.New(40, 2)
	printAll(s, "open https://example.com/a now")
	var l Links
	if u, ok := l.At(s, Point{Col: 10, Row: 0}); !ok || u != "https://example.com/a" {
		t.Fatalf("got %q %v", u, ok)
	}
	if _, ok := l.At(s, Point{Col: 2, Row: 0}); ok {
		t.Fatal("found a link on plain text")
	}
	if _, ok := l.At(s, Point{Col: 27, Row: 0}); ok {
		t.Fatal("found a link after its end")
	}
}

// A link longer than the row wraps; both halves open the whole link.
func TestLinkAtJoinsAWrappedLink(t *testing.T) {
	s := screen.New(20, 3)
	printAll(s, "x https://example.com/a/very/long/path y")
	want := "https://example.com/a/very/long/path"
	var l Links
	for _, p := range []Point{{Col: 5, Row: 0}, {Col: 3, Row: 1}} {
		if u, ok := l.At(s, p); !ok || u != want {
			t.Fatalf("%v: got %q %v", p, u, ok)
		}
	}
}

func TestLinkAtReadsScrollback(t *testing.T) {
	s := screen.NewWithScrollback(30, 2, 10)
	printAll(s, "https://old.example.com")
	for range 3 {
		s.LineFeed()
	}
	if s.ScrollbackLen() == 0 {
		t.Fatal("setup: nothing scrolled back")
	}
	var l Links
	if u, ok := l.At(s, Point{Col: 3, Row: 0}); !ok || u != "https://old.example.com" {
		t.Fatalf("got %q %v", u, ok)
	}
}

func TestVisibleSpansCoverEachRowOfALink(t *testing.T) {
	s := screen.New(20, 3)
	printAll(s, "x https://example.com/a/very/long/path y")
	var l Links
	got := l.Visible(s, 0, 3)
	if len(got) != 2 || got[0] != (LinkSpan{Row: 0, From: 2, To: 19}) || got[1] != (LinkSpan{Row: 1, From: 0, To: 17}) {
		t.Fatalf("got %+v", got)
	}
	if got := l.Visible(s, 1, 1); len(got) != 1 || got[0].Row != 1 {
		t.Fatalf("second row only: %+v", got)
	}
}

func TestVisibleWithoutLinksAllocatesNothing(t *testing.T) {
	s := screen.New(80, 24)
	for range 23 {
		printAll(s, strings.Repeat("plain text: no address ", 3))
		s.CarriageReturn()
		s.LineFeed()
	}
	var l Links
	l.Visible(s, 0, 24)
	if n := testing.AllocsPerRun(20, func() { l.Visible(s, 0, 24) }); n != 0 {
		t.Fatalf("%v allocs per frame", n)
	}
}

func BenchmarkVisibleLinks240x80(b *testing.B) {
	s := screen.New(240, 80)
	for range 79 {
		printAll(s, strings.Repeat("some log text https://example.com/x/y ", 6))
		s.CarriageReturn()
		s.LineFeed()
	}
	var l Links
	b.ReportAllocs()
	for b.Loop() {
		l.Visible(s, 0, 80)
	}
}

// A full row ended by a real newline is not a wrap: the next line is not part of the link.
func TestLinkDoesNotRunIntoTheNextLine(t *testing.T) {
	s := screen.New(20, 3)
	printAll(s, "https://example.com/")
	s.CarriageReturn()
	s.LineFeed()
	printAll(s, "Next log entry")
	var l Links
	if u, ok := l.At(s, Point{Col: 3, Row: 0}); !ok || u != "https://example.com/" {
		t.Fatalf("got %q %v", u, ok)
	}
	if _, ok := l.At(s, Point{Col: 1, Row: 1}); ok {
		t.Fatal("the next line became part of the link")
	}
}

func TestFindDocPaths(t *testing.T) {
	cases := []struct{ in, want string }{
		{"I've written it to `docs/superpowers/specs/x-design.md`.", "docs/superpowers/specs/x-design.md"},
		{"⏺ Wrote 120 lines to docs/plan.md", "docs/plan.md"},
		{"  ⎿  Read README.md (42 lines)", "README.md"},
		{"│ See ./notes/todo.md for details │", "./notes/todo.md"},
		{"• Edited docs/HANDOFF.md (+12 -3)", "docs/HANDOFF.md"},
		{"Updated README.md:42 and docs/a.md:12:3.", "README.md docs/a.md"},
		{"open ~/notes/today.md", "~/notes/today.md"},
		{"/Users/pat/kaze/docs/x.MD", "/Users/pat/kaze/docs/x.MD"},
		{`C:\Users\pat\kaze\docs\x.md`, `C:\Users\pat\kaze\docs\x.md`},
		{`.\docs\x.md and C:/a/b.md`, `.\docs\x.md C:/a/b.md`},
		{"**docs/bold.md** (see docs/paren.md)", "docs/bold.md docs/paren.md"},
		{"保存先docs/a.md", "docs/a.md"},
		{"main.go x.mdx x.md.bak README.markdown .md a/.md", ""},
		{"https://github.com/a/b/blob/main/README.md", "https://github.com/a/b/blob/main/README.md"},
		{"see docs/a.md, docs/b.md; done", "docs/a.md docs/b.md"},
	}
	for _, c := range cases {
		var got []string
		rs := []rune(c.in)
		for _, sp := range findLinks(rs) {
			got = append(got, string(rs[sp[0]:sp[1]]))
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsWebLink(t *testing.T) {
	for _, s := range []string{"https://a.io", "HTTP://A.IO/x"} {
		if !IsWebLink(s) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"docs/a.md", "file:///etc/passwd", "mailto:a@b.c", "javascript:x", "http:/x", ""} {
		if IsWebLink(s) {
			t.Fatal(s)
		}
	}
}
func TestLinkAtFindsADocPathWithoutItsLine(t *testing.T) {
	s := screen.New(40, 2)
	printAll(s, "edit docs/a.md:12 now")
	var l Links
	if u, ok := l.At(s, Point{Col: 7, Row: 0}); !ok || u != "docs/a.md" {
		t.Fatalf("%q %v", u, ok)
	}
	if _, ok := l.At(s, Point{Col: 15, Row: 0}); ok {
		t.Fatal("suffix linked")
	}
}
func TestLinkAtJoinsAWrappedWindowsPath(t *testing.T) {
	s := screen.New(12, 4)
	printAll(s, `x C:\code\kaze\docs\plan.md`)
	var l Links
	for _, p := range []Point{{Col: 5, Row: 0}, {Col: 2, Row: 1}} {
		if u, ok := l.At(s, p); !ok || u != `C:\code\kaze\docs\plan.md` {
			t.Fatalf("%q %v", u, ok)
		}
	}
}
func TestVisibleWithDocPathsAllocatesNothing(t *testing.T) {
	s := screen.New(60, 23)
	for range 22 {
		printAll(s, "wrote docs/plan.md and README.md:3\r\n")
	}
	var l Links
	l.Visible(s, 0, 23)
	if n := testing.AllocsPerRun(20, func() { l.Visible(s, 0, 23) }); n != 0 {
		t.Fatal(n)
	}
}

func TestLinksInGivesByteOffsets(t *testing.T) {
	s := "見て docs/a.md と https://a.io"
	ls := LinksIn(s)
	if len(ls) != 2 {
		t.Fatal(ls)
	}
	for _, l := range ls {
		if s[l.Start:l.End] != l.Text {
			t.Fatal(l)
		}
	}
}

func TestLinksInInvalidUTF8Prefix(t *testing.T) {
	s := "\xff docs/a.md"
	ls := LinksIn(s)
	if len(ls) != 1 || ls[0].Text != "docs/a.md" {
		t.Fatal(ls)
	}
}
