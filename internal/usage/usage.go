// Package usage turns Claude's reported plan usage into short status texts,
// shared by `kaze statusline` and the status bar.
package usage

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

type Level int

const (
	Normal Level = iota
	Warn         // 80% or more
	High         // 95% or more
)

type Segment struct {
	Text  string
	Level Level
}

// Segments is 5h, wk and ctx, leaving out what is absent or already reset.
func Segments(u notify.Usage, now time.Time) []Segment {
	out := []Segment{}
	for _, w := range []struct {
		label string
		win   *notify.Window
	}{{"5h", u.FiveHour}, {"wk", u.Week}} {
		if w.win == nil || expired(w.win, now) {
			continue
		}
		p := clamp(w.win.Pct)
		s := Segment{Text: fmt.Sprintf("%s %d%%", w.label, int(p)), Level: level(p)}
		if s.Level != Normal && w.win.Resets > 0 {
			s.Text += " (resets " + resetText(time.Unix(w.win.Resets, 0), now) + ")"
		}
		out = append(out, s)
	}
	if u.Ctx != nil {
		p := clamp(*u.Ctx)
		out = append(out, Segment{Text: fmt.Sprintf("ctx %d%%", int(p)), Level: level(p)})
	}
	return out
}

// Line is the plain one-line form for Claude Code's own status row.
func Line(u notify.Usage, now time.Time) string {
	var parts []string
	for _, s := range Segments(u, now) {
		parts = append(parts, s.Text)
	}
	return strings.Join(parts, "  ·  ")
}

// NextReset is the earliest reset still ahead, zero when none.
func NextReset(u notify.Usage, now time.Time) time.Time {
	var next time.Time
	for _, w := range []*notify.Window{u.FiveHour, u.Week} {
		if w == nil || w.Resets <= 0 || expired(w, now) {
			continue
		}
		if t := time.Unix(w.Resets, 0); next.IsZero() || t.Before(next) {
			next = t
		}
	}
	return next
}

// Merge picks what to keep for one window. Every Claude session reports the
// limits from its own last reply, so an idle tab can report older, lower
// numbers: a later reset wins, and within one window use only goes up.
func Merge(held, next *notify.Window) *notify.Window {
	switch {
	case next == nil:
		return held
	case held == nil, held.Resets == 0 || next.Resets == 0, next.Resets > held.Resets:
		return next
	case next.Resets < held.Resets:
		return held
	case next.Pct < held.Pct:
		return held
	}
	return next
}

func expired(w *notify.Window, now time.Time) bool {
	return w.Resets > 0 && now.Unix() >= w.Resets
}

func clamp(p float64) float64 {
	if math.IsNaN(p) || p < 0 {
		return 0
	}
	return math.Min(p, 100)
}

func level(p float64) Level {
	switch {
	case p >= 95:
		return High
	case p >= 80:
		return Warn
	}
	return Normal
}

func resetText(t, now time.Time) string {
	t = t.In(now.Location())
	if t.Sub(now) >= 24*time.Hour {
		return t.Format("Mon 3:04 PM")
	}
	return t.Format("3:04 PM")
}

// Detail is one limit for the status bar's usage card.
type Detail struct {
	Key   string // "5h", "wk", "ctx"
	Name  string // "5-hour", "Week", "Context"
	Pct   int
	Level Level
	Reset string // "resets in 2h 14m", "resets Mon 9:00 AM", "this session"; "" when unknown
}

// Details is Segments for the hover card: the same limits and levels, each
// with when it resets.
func Details(u notify.Usage, now time.Time) []Detail {
	var out []Detail
	for _, w := range []struct {
		key, name string
		win       *notify.Window
	}{{"5h", "5-hour", u.FiveHour}, {"wk", "Week", u.Week}} {
		if w.win == nil || expired(w.win, now) {
			continue
		}
		p := clamp(w.win.Pct)
		d := Detail{Key: w.key, Name: w.name, Pct: int(p), Level: level(p)}
		if w.win.Resets > 0 {
			d.Reset = resetIn(time.Unix(w.win.Resets, 0), now)
		}
		out = append(out, d)
	}
	if u.Ctx != nil {
		p := clamp(*u.Ctx)
		out = append(out, Detail{Key: "ctx", Name: "Context", Pct: int(p), Level: level(p), Reset: "this session"})
	}
	return out
}

// resetIn is "resets in 2h 14m" within a day, else "resets Mon 9:00 AM".
func resetIn(t, now time.Time) string {
	d := t.Sub(now).Round(time.Minute)
	switch {
	case d >= 24*time.Hour:
		return "resets " + resetText(t, now)
	case d >= time.Hour:
		return fmt.Sprintf("resets in %dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
	return fmt.Sprintf("resets in %dm", int(d.Minutes()))
}
