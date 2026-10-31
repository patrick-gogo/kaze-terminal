// Package version reports the kaze version and the build it came from.
package version

import (
	"runtime/debug"
	"time"
)

// Number is the release version shown in the settings page and `kaze --version`.
const Number = "0.12"

// Build is what the Go toolchain recorded about the commit a binary was built from.
type Build struct {
	Commit   string // first 7 characters of the revision; empty when unknown
	Time     time.Time
	Modified bool
}

// Read extracts the VCS settings; a binary built without them gives the zero Build.
func Read(info *debug.BuildInfo, ok bool) Build {
	var b Build
	if !ok || info == nil {
		return b
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			b.Commit = s.Value
			if len(b.Commit) > 7 {
				b.Commit = b.Commit[:7]
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				b.Time = t
			}
		case "vcs.modified":
			b.Modified = s.Value == "true"
		}
	}
	return b
}

// Line is the one-line build description, with the time in the local zone.
func (b Build) Line() string { return b.line(time.Local) }

func (b Build) line(loc *time.Location) string {
	if b.Commit == "" {
		return "build unknown"
	}
	s := "build " + b.Commit
	if !b.Time.IsZero() {
		s += " · " + b.Time.In(loc).Format("Jan 2, 3:04 PM")
	}
	if b.Modified {
		s += " + local changes"
	}
	return s
}

// Text is the multi-line form printed by `kaze --version`.
func Text() string {
	info, ok := debug.ReadBuildInfo()
	return "kaze " + Number + "\n" + Read(info, ok).Line()
}
