package skills

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const maxDesc = 200 // runes; the palette shows one line

var validName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// parseFile reads a skill or command file's name and one-line description.
// fallback (the folder or file name) is used when the frontmatter has no
// usable name; ok is false when neither is a valid name.
func parseFile(data []byte, fallback string) (name, desc string, ok bool) {
	if len(data) > maxRead {
		data = data[:maxRead]
	}
	fields, body, _ := frontmatter(data)
	name = fields["name"]
	if !validName.MatchString(name) {
		name = fallback
	}
	if !validName.MatchString(name) {
		return "", "", false
	}
	desc = cleanDesc(fields["description"])
	if desc == "" {
		desc = cleanDesc(firstLine(body))
	}
	return name, desc, true
}

// frontmatter splits a "---" block from the body and reads its top-level
// name and description. Without a closing line the whole file is body.
func frontmatter(data []byte) (fields map[string]string, body []byte, ok bool) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	fields = map[string]string{}
	if len(lines) == 0 || lines[0] != "---" {
		return fields, []byte(text), false
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" || lines[i] == "..." {
			end = i
			break
		}
	}
	if end < 0 {
		return fields, []byte(text), false
	}
	block := lines[1:end]
	for i := 0; i < len(block); i++ {
		l := block[i]
		if l == "" || l[0] == ' ' || l[0] == '\t' || l[0] == '#' {
			continue
		}
		key, val, found := strings.Cut(l, ":")
		if !found {
			continue
		}
		key, val = strings.TrimSpace(key), strings.TrimSpace(val)
		// Indented lines that follow belong to this key: a block scalar's
		// text or a plain value's continuation, joined with spaces.
		var more []string
		for i+1 < len(block) && (block[i+1] == "" || block[i+1][0] == ' ' || block[i+1][0] == '\t') {
			i++
			if s := strings.TrimSpace(block[i]); s != "" {
				more = append(more, s)
			}
		}
		if key != "name" && key != "description" {
			continue
		}
		if isBlockScalar(val) {
			val = strings.Join(more, " ")
		} else {
			val = unquote(strings.Join(append([]string{val}, more...), " "))
		}
		fields[key] = val
	}
	return fields, []byte(strings.Join(lines[end+1:], "\n")), true
}

func isBlockScalar(v string) bool {
	switch v {
	case "|", ">", "|-", ">-", "|+", ">+":
		return true
	}
	return false
}

func unquote(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		r := strings.NewReplacer(`\"`, `"`, `\\`, `\`)
		return r.Replace(v[1 : len(v)-1])
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	return v
}

// firstLine is the body's first non-empty line that is not "---", without a
// Markdown heading's leading "#".
func firstLine(body []byte) string {
	for _, l := range strings.Split(string(body), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l == "---" {
			continue
		}
		return strings.TrimSpace(strings.TrimLeft(l, "# "))
	}
	return ""
}

// cleanDesc drops control characters and invalid bytes so a crafted file
// can't inject escapes into the UI, collapses whitespace and cuts to maxDesc.
func cleanDesc(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == utf8.RuneError {
			return -1
		}
		if unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > maxDesc {
		s = strings.TrimSpace(string([]rune(s)[:maxDesc]))
	}
	return s
}
