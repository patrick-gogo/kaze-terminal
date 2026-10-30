package skills

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func TestParseFrontmatter(t *testing.T) {
	cases := []struct {
		name, in, fallback string
		wantName, wantDesc string
		ok                 bool
	}{
		{"plain", "---\nname: ship-ticket\ndescription: Open the PR\n---\nbody", "f", "ship-ticket", "Open the PR", true},
		{"double quotes", "---\nname: x\ndescription: \"quoted \\\"x\\\"\"\n---\n", "f", "x", `quoted "x"`, true},
		{"single quotes", "---\nname: x\ndescription: 'it''s'\n---\n", "f", "x", "it's", true},
		{"folded", "---\nname: x\ndescription: >-\n  folded\n  text\n---\n", "f", "x", "folded text", true},
		{"literal", "---\nname: x\ndescription: |\n  a\n  b\n---\n", "f", "x", "a b", true},
		{"continuation", "---\nname: x\ndescription: one\n  two\n---\n", "f", "x", "one two", true},
		{"bom crlf", "\xef\xbb\xbf---\r\nname: x\r\n---\r\n", "f", "x", "", true},
		{"dots close", "---\nname: x\ndescription: d\n...\nbody", "f", "x", "d", true},
		{"no frontmatter", "\n\n# Heading here\nmore", "fb", "fb", "Heading here", true},
		{"never closed", "---\nname: x\n", "fb", "fb", "name: x", true},
		{"bad name", "---\nname: bad name!\n---\n", "fb", "fb", "", true},
		{"bad name and fallback", "---\nname: bad name!\n---\n", "", "", "", false},
		{"bad fallback only", "hello", "has space", "", "", false},
		{"nested key ignored", "---\nname: x\nmeta:\n  description: no\n---\nBody line", "f", "x", "Body line", true},
	}
	for _, c := range cases {
		name, desc, ok := parseFile([]byte(c.in), c.fallback)
		if ok != c.ok || (ok && (name != c.wantName || desc != c.wantDesc)) {
			t.Errorf("%s: got (%q, %q, %v), want (%q, %q, %v)", c.name, name, desc, ok, c.wantName, c.wantDesc, c.ok)
		}
	}
}

func TestParseDescriptionIsCleanAndShort(t *testing.T) {
	long := strings.Repeat("ab\x1b\tc ", 250)
	_, desc, ok := parseFile([]byte("---\nname: x\ndescription: "+long+"\n---\n"), "f")
	if !ok {
		t.Fatal("not ok")
	}
	if n := utf8.RuneCountInString(desc); n > 200 || n == 0 {
		t.Fatalf("desc has %d runes", n)
	}
	for _, r := range desc {
		if unicode.IsControl(r) {
			t.Fatalf("control rune %q in %q", r, desc)
		}
	}
	if strings.Contains(desc, "  ") {
		t.Fatalf("whitespace not collapsed: %q", desc)
	}
}
