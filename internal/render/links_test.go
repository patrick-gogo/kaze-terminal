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
		for _, sp := range findURLs(rs) {
			got = append(got, string(rs[sp[0]:sp[1]]))
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFindURLsTwoOnALine(t *testing.T) {
	rs := []rune("a https://a.io b http://b.io/c")
	if sp := findURLs(rs); len(sp) != 2 || string(rs[sp[1][0]:sp[1][1]]) != "http://b.io/c" {
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
