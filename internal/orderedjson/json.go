// Package orderedjson edits JSON while preserving object order and number spelling.
package orderedjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// Node is a JSON value that keeps its object keys in file order, so a fix
// rewrites the owner's settings with only the changed lines different.
type Node struct {
	Kind byte // '{', '[', or 0 for a scalar
	Keys []string
	Vals []*Node // object values (by keys) or array items
	Raw  string  // a scalar as JSON text
}

const maxDepth = 200

func Parse(b []byte) (*Node, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	n, err := parseValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, errors.New("extra data after the JSON value")
	}
	return n, nil
}

func parseValue(d *json.Decoder, depth int) (*Node, error) {
	if depth > maxDepth {
		return nil, errors.New("JSON nested too deep")
	}
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		n := &Node{Kind: byte(t)}
		for d.More() {
			if n.Kind == '{' {
				k, err := d.Token()
				if err != nil {
					return nil, err
				}
				n.Keys = append(n.Keys, k.(string))
			}
			v, err := parseValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			n.Vals = append(n.Vals, v)
		}
		if _, err := d.Token(); err != nil { // the closing delimiter
			return nil, err
		}
		return n, nil
	case json.Number:
		return &Node{Raw: t.String()}, nil
	default:
		return Scalar(t), nil
	}
}

// Scalar is a string, bool or null Node.
func Scalar(v any) *Node {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return &Node{Raw: strings.TrimSuffix(b.String(), "\n")}
}

func Object() *Node { return &Node{Kind: '{'} }

func (n *Node) Get(key string) *Node {
	if n == nil || n.Kind != '{' {
		return nil
	}
	for i, k := range n.Keys {
		if k == key {
			return n.Vals[i]
		}
	}
	return nil
}

func (n *Node) Set(key string, v *Node) {
	for i, k := range n.Keys {
		if k == key {
			n.Vals[i] = v
			return
		}
	}
	n.Keys = append(n.Keys, key)
	n.Vals = append(n.Vals, v)
}

// Str is the Node's string value; ok is false for anything but a string.
func (n *Node) Str() (string, bool) {
	if n == nil || n.Kind != 0 || !strings.HasPrefix(n.Raw, `"`) {
		return "", false
	}
	var s string
	return s, json.Unmarshal([]byte(n.Raw), &s) == nil
}

// Encode writes n the way Claude Code does: two-space indent, no HTML escaping.
func (n *Node) Encode() []byte {
	var b bytes.Buffer
	n.write(&b, 0)
	b.WriteByte('\n')
	return b.Bytes()
}

func (n *Node) write(b *bytes.Buffer, depth int) {
	if n.Kind == 0 {
		b.WriteString(n.Raw)
		return
	}
	open, end := "{", "}"
	if n.Kind == '[' {
		open, end = "[", "]"
	}
	if len(n.Vals) == 0 {
		b.WriteString(open + end)
		return
	}
	b.WriteString(open + "\n")
	pad := strings.Repeat("  ", depth+1)
	for i, v := range n.Vals {
		b.WriteString(pad)
		if n.Kind == '{' {
			b.WriteString(Scalar(n.Keys[i]).Raw + ": ")
		}
		v.write(b, depth+1)
		if i < len(n.Vals)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat("  ", depth) + end)
}

func (n *Node) ListVals() []*Node {
	if n == nil || n.Kind != '[' {
		return nil
	}
	return n.Vals
}
