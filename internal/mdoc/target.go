package mdoc

import (
	"net/url"
	"strings"
	"unicode"
)

type TargetKind uint8

const (
	TargetNone TargetKind = iota
	TargetWeb
	TargetDoc
	TargetAnchor
)

type Target struct {
	Kind              TargetKind
	URL, Path, Anchor string
}

func Classify(dest string) Target {
	lower := strings.ToLower(dest)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return Target{Kind: TargetWeb, URL: dest}
	}
	if strings.HasPrefix(dest, "#") {
		return Target{Kind: TargetAnchor, Anchor: dest[1:]}
	}
	p, a, _ := strings.Cut(dest, "#")
	if i := strings.IndexByte(p, ':'); i >= 0 {
		if i != 1 || !unicode.IsLetter(rune(p[0])) || len(p) < 3 || p[2] != '/' && p[2] != '\\' {
			return Target{}
		}
	}
	decoded, e := url.PathUnescape(p)
	if e != nil || !IsMarkdown(decoded) {
		return Target{}
	}
	return Target{Kind: TargetDoc, Path: decoded, Anchor: a}
}
