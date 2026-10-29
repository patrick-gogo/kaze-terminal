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
	firstTable, found := len(lines), false
	var scan tomlScan
	for i, l := range lines {
		if !scan.topLevel() {
			scan.line(l)
			continue
		}
		if tableHeader.MatchString(l) {
			firstTable = i
			break
		}
		if assignsKey(l, key) {
			lines[i] = key + " = " + lit + commentOf(l) + eol
			found = true
			break
		}
		scan.line(l)
	}
	if !found {
		line := key + " = " + lit + eol
		if firstTable == len(lines) {
			if n := len(lines); n > 0 && !strings.HasSuffix(lines[n-1], "\n") {
				lines[n-1] += eol
			}
			lines = append(lines, line)
		} else {
			// Above the table's own comment block and the blank line before it.
			at := firstTable
			for at > 0 && strings.HasPrefix(strings.TrimSpace(lines[at-1]), "#") {
				at--
			}
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

	// A symlinked config (a dotfiles repo) is updated in place, not replaced by a plain file.
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return replace(path, []byte(out))
}

// replace writes data to a fresh temp file beside path and renames it over
// path, so a crash or a second writer never leaves a half-written config.
func replace(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	err = tmp.Chmod(0o644) // CreateTemp makes 0600; the config was always world-readable
	if err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		os.Remove(name)
	}
	return err
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

// tomlScan tracks value boundaries while locating top-level assignments. TOML
// has already been validated, so this only needs to skip strings and containers.
// In particular, shell scripts in multiline strings may look like config keys.
type tomlScan struct {
	quote byte
	multi bool
	depth int
}

func (s *tomlScan) topLevel() bool { return s.quote == 0 && s.depth == 0 }

func (s *tomlScan) line(line string) {
	for i := 0; i < len(line); i++ {
		c := line[i]
		if s.quote != 0 {
			if s.quote == '"' && c == '\\' {
				i++
				continue
			}
			if c != s.quote {
				continue
			}
			if s.multi {
				n := 1
				for i+n < len(line) && line[i+n] == s.quote {
					n++
				}
				if n < 3 {
					i += n - 1
					continue
				}
				i += n - 1
			}
			s.quote, s.multi = 0, false
			continue
		}
		switch c {
		case '#':
			return
		case '"', '\'':
			s.quote = c
			if i+2 < len(line) && line[i+1] == c && line[i+2] == c {
				s.multi = true
				i += 2
			}
		case '[', '{':
			s.depth++
		case ']', '}':
			s.depth--
		}
	}
}

// assignsKey decodes just the assignment's key, including TOML quoted and
// escaped key syntax, without interpreting any of its possibly multiline value.
func assignsKey(line, key string) bool {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if quote == '"' && c == '\\' {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '#':
			return false
		case '"', '\'':
			quote = c
		case '=':
			var probe map[string]any
			if _, err := toml.Decode(line[:i]+" = true", &probe); err != nil {
				return false
			}
			v, ok := probe[key].(bool)
			return len(probe) == 1 && ok && v
		}
	}
	return false
}
