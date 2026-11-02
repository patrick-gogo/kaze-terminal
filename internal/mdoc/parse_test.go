package mdoc

import (
	"strings"
	"testing"
	"time"
)

func TestParseHeadingsAndOutline(t *testing.T) {
	d := Parse([]byte("# A\n## B\n### B\n"))
	if len(d.Outline) != 3 || d.Outline[2].Slug != "b-1" || d.Outline[1].Block != 1 {
		t.Fatalf("%+v", d)
	}
}
func TestParseInlineStyles(t *testing.T) {
	d := Parse([]byte("a **b** *c* `d` ~~e~~ [f](x.md) <https://g.io>"))
	var flags Style
	var links []string
	for _, s := range d.Blocks[0].Spans {
		flags |= s.Style
		if s.Link != "" {
			links = append(links, s.Link)
		}
	}
	if flags != Bold|Italic|Code|Strike || len(links) != 2 {
		t.Fatal(d)
	}
}
func TestParseStructures(t *testing.T) {
	src := "3. one\n4. two\n   - deep\n\n- [ ] a\n- [x] b\n\n> quote\n\n```go\nx := 1\n```\n\n| a | b |\n|:--|--:|\n| 1 | 2 |\n\n---\n\n![alt](p.png)\n\n<div>x</div>\n"
	d := Parse([]byte(src))
	var items, tasks, quotes, codes, tables, rules int
	for _, b := range d.Blocks {
		if b.Kind == Item {
			items++
			if b.Task > 0 {
				tasks++
			}
		}
		if b.Quote > 0 {
			quotes++
		}
		if b.Kind == CodeBlock {
			codes++
			if b.Code != "x := 1" {
				t.Fatal(b)
			}
		}
		if b.Kind == TableBlock {
			tables++
			if len(b.Table.Rows) != 2 || b.Table.Align[0] != AlignLeft || b.Table.Align[1] != AlignRight {
				t.Fatal(b.Table)
			}
		}
		if b.Kind == Rule {
			rules++
		}
	}
	if items != 5 || tasks != 2 || quotes != 1 || codes != 1 || tables != 1 || rules != 1 {
		t.Fatalf("%+v", d.Blocks)
	}
}
func TestParseDeepNestingStaysBounded(t *testing.T) {
	done := make(chan *Doc, 1)
	go func() {
		done <- Parse([]byte(strings.Repeat("> ", 5000) + "hello\n" + strings.Repeat("- ", 2000) + "end"))
	}()
	select {
	case d := <-done:
		for _, b := range d.Blocks {
			if b.Quote > MaxDepth || b.Indent > MaxDepth {
				t.Fatal(b)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("parse deadline")
	}
}
func TestSlugGitHubStyle(t *testing.T) {
	for in, want := range map[string]string{"Hello, World!": "hello-world", "Plan 1: Core": "plan-1-core", "日本語": "日本語"} {
		if Slug(in) != want {
			t.Fatal(in, Slug(in))
		}
	}
}
func TestClassifyTargets(t *testing.T) {
	for in, want := range map[string]TargetKind{"https://a.io": TargetWeb, "HTTP://A.IO": TargetWeb, "#intro": TargetAnchor, "../x.md": TargetDoc, "other.md#plan-1": TargetDoc, "my%20doc.md": TargetDoc, `C:\a\b.md`: TargetDoc, "mailto:a@b.c": TargetNone, "file:///x.md": TargetNone, "javascript:x": TargetNone, "main.go": TargetNone, "": TargetNone} {
		if got := Classify(in); got.Kind != want {
			t.Fatal(in, got)
		}
	}
	if Classify("my%20doc.md").Path != "my doc.md" {
		t.Fatal("decode")
	}
}
