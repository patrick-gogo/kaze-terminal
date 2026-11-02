package mdoc

import (
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"html"
	"strconv"
	"strings"
)

type Style uint8

const (
	Bold Style = 1 << iota
	Italic
	Code
	Strike
)

type Span struct {
	Text  string
	Style Style
	Link  string
}
type Kind uint8

const (
	HeadingBlock Kind = iota + 1
	Para
	CodeBlock
	Item
	TableBlock
	Rule
)

// HeadingBlock names the block kind; Heading is the outline entry type.
type Block struct {
	Kind                 Kind
	Level, Indent, Quote int
	Marker               string
	Task                 int8
	Spans                []Span
	Code                 string
	Table                *Table
	Slug                 string
}

const MaxDepth = 8

type Align uint8

const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

type Table struct {
	Align []Align
	Rows  [][][]Span
}
type Heading struct {
	Level      int
	Text, Slug string
	Block      int
}
type Doc struct {
	Blocks  []Block
	Outline []Heading
}

func Parse(src []byte) *Doc {
	root := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(src))
	d := &Doc{}
	idx := -1
	quote := 0
	type listState struct {
		ordered bool
		number  int
	}
	var lists []listState
	var first []bool
	style := Style(0)
	link := ""
	type inlineState struct {
		style Style
		link  string
	}
	var styles []inlineState
	var table *Table
	row, cell := -1, -1
	used := map[string]bool{}
	counts := map[string]int{}
	add := func(s string) {
		if s == "" {
			return
		}
		sp := Span{Text: s, Style: style, Link: link}
		if table != nil && row >= 0 && cell >= 0 {
			table.Rows[row][cell] = append(table.Rows[row][cell], sp)
		} else if idx >= 0 {
			d.Blocks[idx].Spans = append(d.Blocks[idx].Spans, sp)
		}
	}
	block := func(kind Kind) int {
		d.Blocks = append(d.Blocks, Block{Kind: kind, Indent: min(max(len(lists)-1, 0), MaxDepth), Quote: min(quote, MaxDepth)})
		idx = len(d.Blocks) - 1
		return idx
	}
	lines := func(n ast.Node) string {
		var b strings.Builder
		for i := 0; i < n.Lines().Len(); i++ {
			seg := n.Lines().At(i)
			b.Write(seg.Value(src))
		}
		return strings.TrimSuffix(b.String(), "\n")
	}
	ast.Walk(root, func(n ast.Node, enter bool) (ast.WalkStatus, error) {
		if !enter {
			switch v := n.(type) {
			case *ast.Blockquote:
				quote--
			case *ast.List:
				lists = lists[:len(lists)-1]
			case *ast.ListItem:
				first = first[:len(first)-1]
			case *ast.Emphasis, *ast.CodeSpan, *east.Strikethrough, *ast.Link, *ast.Image:
				old := styles[len(styles)-1]
				styles = styles[:len(styles)-1]
				style, link = old.style, old.link
			case *east.Table:
				table = nil
				row, cell = -1, -1
			case *ast.Heading:
				b := &d.Blocks[idx]
				var text strings.Builder
				for _, s := range b.Spans {
					text.WriteString(s.Text)
				}
				name := text.String()
				base := Slug(name)
				slug := base
				for used[slug] {
					counts[base]++
					slug = base + "-" + strconv.Itoa(counts[base])
				}
				used[slug] = true
				b.Slug = slug
				d.Outline = append(d.Outline, Heading{v.Level, name, slug, idx})
			}
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Blockquote:
			quote++
		case *ast.List:
			lists = append(lists, listState{v.IsOrdered(), v.Start})
		case *ast.ListItem:
			first = append(first, true)
		case *ast.Heading:
			block(HeadingBlock)
			d.Blocks[idx].Level = v.Level
		case *ast.Paragraph, *ast.TextBlock:
			block(Para)
			if len(first) > 0 && first[len(first)-1] {
				first[len(first)-1] = false
				d.Blocks[idx].Kind = Item
				l := &lists[len(lists)-1]
				if l.ordered {
					d.Blocks[idx].Marker = strconv.Itoa(l.number) + "."
					l.number++
				} else {
					d.Blocks[idx].Marker = "•"
				}
			}
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			block(CodeBlock)
			d.Blocks[idx].Code = lines(n)
			return ast.WalkSkipChildren, nil
		case *ast.ThematicBreak:
			block(Rule)
		case *ast.HTMLBlock:
			block(Para)
			raw := lines(n)
			if v.HasClosure() {
				raw += "\n" + string(v.ClosureLine.Value(src))
			}
			add(raw)
			return ast.WalkSkipChildren, nil
		case *east.Table:
			block(TableBlock)
			table = &Table{}
			d.Blocks[idx].Table = table
			for _, a := range v.Alignments {
				align := AlignNone
				switch a {
				case east.AlignLeft:
					align = AlignLeft
				case east.AlignCenter:
					align = AlignCenter
				case east.AlignRight:
					align = AlignRight
				}
				table.Align = append(table.Align, align)
			}
		case *east.TableHeader, *east.TableRow:
			table.Rows = append(table.Rows, [][]Span{})
			row++
			cell = -1
		case *east.TableCell:
			table.Rows[row] = append(table.Rows[row], []Span{})
			cell++
		case *east.TaskCheckBox:
			if idx >= 0 {
				d.Blocks[idx].Task = 1
				if v.IsChecked {
					d.Blocks[idx].Task = 2
				}
				d.Blocks[idx].Marker = ""
			}
		case *ast.Emphasis:
			styles = append(styles, inlineState{style, link})
			if v.Level == 2 {
				style |= Bold
			} else {
				style |= Italic
			}
		case *ast.CodeSpan:
			styles = append(styles, inlineState{style, link})
			style |= Code
		case *east.Strikethrough:
			styles = append(styles, inlineState{style, link})
			style |= Strike
		case *ast.Link:
			styles = append(styles, inlineState{style, link})
			link = string(v.Destination)
		case *ast.Image:
			styles = append(styles, inlineState{style, link})
			style |= Italic
			if n.FirstChild() == nil {
				add("image")
			}
		case *ast.AutoLink:
			u := string(v.URL(src))
			if strings.HasPrefix(u, "www.") {
				u = "http://" + u
			}
			old := link
			link = u
			add(string(v.Label(src)))
			link = old
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			s := string(v.Segment.Value(src))
			if style&Code == 0 {
				s = html.UnescapeString(s)
			}
			add(s)
			if v.HardLineBreak() {
				add("\n")
			} else if v.SoftLineBreak() {
				if style&Code != 0 {
					add(" ")
				} else {
					add(" ")
				}
			}
		case *ast.String:
			add(string(v.Value))
		case *ast.RawHTML:
			var b strings.Builder
			for i := 0; i < v.Segments.Len(); i++ {
				seg := v.Segments.At(i)
				b.Write(seg.Value(src))
			}
			add(b.String())
		}
		return ast.WalkContinue, nil
	})
	return d
}
