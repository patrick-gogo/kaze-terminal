package version

import (
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func info(kv ...string) *debug.BuildInfo {
	bi := &debug.BuildInfo{}
	for i := 0; i+1 < len(kv); i += 2 {
		bi.Settings = append(bi.Settings, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
	}
	return bi
}

func TestReadVCSSettings(t *testing.T) {
	b := Read(info(
		"vcs.revision", "6fa6324abcdef0123456789",
		"vcs.time", "2026-10-29T01:19:00Z",
		"vcs.modified", "true",
	), true)
	if b.Commit != "6fa6324" {
		t.Errorf("Commit = %q, want 6fa6324", b.Commit)
	}
	if !b.Time.Equal(time.Date(2026, 10, 29, 1, 19, 0, 0, time.UTC)) {
		t.Errorf("Time = %v", b.Time)
	}
	if !b.Modified {
		t.Error("Modified = false, want true")
	}
}

func TestReadShortRevisionKept(t *testing.T) {
	if got := Read(info("vcs.revision", "abc"), true).Commit; got != "abc" {
		t.Errorf("Commit = %q, want abc", got)
	}
}

func TestReadUnknown(t *testing.T) {
	for name, b := range map[string]Build{
		"no settings": Read(info(), true),
		"not ok":      Read(nil, false),
		"ok but nil":  Read(nil, true),
	} {
		if got := b.Line(); got != "build unknown" {
			t.Errorf("%s: Line() = %q, want build unknown", name, got)
		}
	}
}

func TestLineFormat(t *testing.T) {
	loc := time.FixedZone("T", 8*3600)
	b := Read(info("vcs.revision", "6fa6324abcdef", "vcs.time", "2026-10-29T01:19:00Z"), true)
	if got, want := b.line(loc), "build 6fa6324 · Oct 29, 9:19 AM"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	b.Modified = true
	if got, want := b.line(loc), "build 6fa6324 · Oct 29, 9:19 AM + local changes"; got != want {
		t.Errorf("modified line = %q, want %q", got, want)
	}
}

func TestLineUsesLocalZone(t *testing.T) {
	b := Read(info("vcs.revision", "6fa6324", "vcs.time", "2026-10-29T01:19:00Z"), true)
	want := "build 6fa6324 · " + b.Time.In(time.Local).Format("Jan 2, 3:04 PM")
	if got := b.Line(); got != want {
		t.Errorf("Line() = %q, want %q", got, want)
	}
}

func TestText(t *testing.T) {
	got := Text()
	if !strings.HasPrefix(got, "kaze "+Number+"\nbuild ") {
		t.Errorf("Text() = %q", got)
	}
}

func TestNumber(t *testing.T) {
	if Number != "0.10" {
		t.Errorf("Number = %q, want 0.10", Number)
	}
}
