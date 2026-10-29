package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

var tableHeader = regexp.MustCompile(`^\s*\[`)

// Set writes one top-level key, keeping every other line and comment as is.
// value is a string, bool, int or float64. A file that does not parse is
// never written.
func Set(path, key string, value any) error {
	lit, err := tomlLiteral(value)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	src := string(data)
	var probe map[string]any
	if _, err := toml.Decode(src, &probe); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}

	eol := "\n"
	if strings.Contains(src, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.SplitAfter(src, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	keyLine := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=`)
	firstTable, found := len(lines), false
	for i, l := range lines {
		if tableHeader.MatchString(l) {
			firstTable = i
			break
		}
		if keyLine.MatchString(l) {
			lines[i] = key + " = " + lit + commentOf(l) + eol
			found = true
			break
		}
	}
	if !found {
		line := key + " = " + lit + eol
		if firstTable == len(lines) {
			if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
				lines[n-1] += eol
			}
			lines = append(lines, line)
		} else {
			// Above the table, but keep the blank line that separates them.
			at := firstTable
			for at > 0 && strings.TrimSpace(lines[at-1]) == "" {
				at--
			}
			lines = append(lines[:at], append([]string{line}, lines[at:]...)...)
		}
	}
	out := strings.Join(lines, "")
	if _, err := toml.Decode(out, &probe); err != nil {
		return fmt.Errorf("config %s: edit would not parse: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// commentOf returns the whitespace and "# comment" after the value on a
// key line, or "" when there is none. Quotes are respected so a # inside a
// string does not count.
func commentOf(line string) string {
	line = strings.TrimRight(line, "\r\n")
	i := strings.Index(line, "=") + 1
	var quote byte
	for j := i; j < len(line); j++ {
		c := line[j]
		switch {
		case quote == '"' && c == '\\':
			j++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			k := j
			for k > i && (line[k-1] == ' ' || line[k-1] == '\t') {
				k--
			}
			return line[k:]
		}
	}
	return ""
}

func tomlLiteral(v any) (string, error) {
	switch x := v.(type) {
	case string:
		var b strings.Builder
		b.WriteByte('"')
		for _, r := range x {
			switch {
			case r == '"' || r == '\\':
				b.WriteByte('\\')
				b.WriteRune(r)
			case r == '\n':
				b.WriteString(`\n`)
			case r == '\r':
				b.WriteString(`\r`)
			case r == '\t':
				b.WriteString(`\t`)
			case r < 0x20 || r == 0x7f:
				fmt.Fprintf(&b, `\u%04x`, r)
			default:
				b.WriteRune(r)
			}
		}
		b.WriteByte('"')
		return b.String(), nil
	case bool:
		return strconv.FormatBool(x), nil
	case int:
		return strconv.Itoa(x), nil
	case float64:
		s := strconv.FormatFloat(x, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eEn") {
			s += ".0"
		}
		return s, nil
	}
	return "", fmt.Errorf("config: cannot write a %T value", v)
}
