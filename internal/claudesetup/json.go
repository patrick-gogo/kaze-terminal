package claudesetup

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// node is a JSON value that keeps its object keys in file order, so a fix
// rewrites the owner's settings with only the changed lines different.
type node struct {
	kind byte // '{', '[', or 0 for a scalar
	keys []string
	vals []*node // object values (by keys) or array items
	raw  string  // a scalar as JSON text
}

const maxDepth = 200

func parse(b []byte) (*node, error) {
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

func parseValue(d *json.Decoder, depth int) (*node, error) {
	if depth > maxDepth {
		return nil, errors.New("JSON nested too deep")
	}
	tok, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		n := &node{kind: byte(t)}
		for d.More() {
			if n.kind == '{' {
				k, err := d.Token()
				if err != nil {
					return nil, err
				}
				n.keys = append(n.keys, k.(string))
			}
			v, err := parseValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			n.vals = append(n.vals, v)
		}
		if _, err := d.Token(); err != nil { // the closing delimiter
			return nil, err
		}
		return n, nil
	case json.Number:
		return &node{raw: t.String()}, nil
	default:
		return scalar(t), nil
	}
}

// scalar is a string, bool or null node.
func scalar(v any) *node {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return &node{raw: strings.TrimSuffix(b.String(), "\n")}
}

func object() *node { return &node{kind: '{'} }

func (n *node) get(key string) *node {
	if n == nil || n.kind != '{' {
		return nil
	}
	for i, k := range n.keys {
		if k == key {
			return n.vals[i]
		}
	}
	return nil
}

func (n *node) set(key string, v *node) {
	for i, k := range n.keys {
		if k == key {
			n.vals[i] = v
			return
		}
	}
	n.keys = append(n.keys, key)
	n.vals = append(n.vals, v)
}

// str is the node's string value; ok is false for anything but a string.
func (n *node) str() (string, bool) {
	if n == nil || n.kind != 0 || !strings.HasPrefix(n.raw, `"`) {
		return "", false
	}
	var s string
	return s, json.Unmarshal([]byte(n.raw), &s) == nil
}

// encode writes n the way Claude Code does: two-space indent, no HTML escaping.
func (n *node) encode() []byte {
	var b bytes.Buffer
	n.write(&b, 0)
	b.WriteByte('\n')
	return b.Bytes()
}

func (n *node) write(b *bytes.Buffer, depth int) {
	if n.kind == 0 {
		b.WriteString(n.raw)
		return
	}
	open, end := "{", "}"
	if n.kind == '[' {
		open, end = "[", "]"
	}
	if len(n.vals) == 0 {
		b.WriteString(open + end)
		return
	}
	b.WriteString(open + "\n")
	pad := strings.Repeat("  ", depth+1)
	for i, v := range n.vals {
		b.WriteString(pad)
		if n.kind == '{' {
			b.WriteString(scalar(n.keys[i]).raw + ": ")
		}
		v.write(b, depth+1)
		if i < len(n.vals)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat("  ", depth) + end)
}
