package main

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

const summaryRunes = 120

// toolSummary is one line saying what a tool call does (spec 5.3), cleaned
// and cut; "" for tools it has nothing to say about.
func toolSummary(tool string, input json.RawMessage) string {
	var in struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		Pattern  string `json:"pattern"`
		Path     string `json:"path"`
		URL      string `json:"url"`
		Query    string `json:"query"`
	}
	if json.Unmarshal(input, &in) != nil {
		return ""
	}
	s := ""
	switch tool {
	case "Bash":
		s, _, _ = strings.Cut(in.Command, "\n")
	case "Read", "Edit", "Write", "MultiEdit", "NotebookEdit":
		s = lastParts(in.FilePath, 3)
	case "Grep", "Glob":
		s = in.Pattern
		if in.Path != "" {
			s += " in " + in.Path
		}
	case "WebFetch":
		if u, err := url.Parse(in.URL); err == nil {
			s = u.Host + u.Path
		}
	case "WebSearch":
		s = in.Query
	}
	return notify.CleanLine(strings.TrimSpace(s), summaryRunes)
}

// lastParts is the last n parts of a slash or backslash path.
func lastParts(p string, n int) string {
	parts := strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) > n {
		parts = parts[len(parts)-n:]
	}
	return strings.Join(parts, "/")
}
