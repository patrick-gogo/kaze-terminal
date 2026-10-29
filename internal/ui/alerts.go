package ui

import (
	"image/color"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// mark is a tab's attention dot.
type mark int

const (
	markNone mark = iota
	markDone
	markNeeds
)

const (
	maxToasts = 3
	maxReason = 120
)

// toast is one alert card. Text is the cleaned reason; the tab title is read
// when it is drawn so a renamed tab shows its current name.
type toast struct {
	Tab   tabs.TabID
	Kind  notify.Kind
	Text  string
	Until time.Time // zero = stays until visited or dismissed

	// Run is set on a workspace run's summary, which Title names; it is not
	// cleared by visiting its tab, only by its close button or its timer.
	Run   uint64
	Title string
}

// alerts holds the tab marks and toasts. Used only on the UI goroutine.
type alerts struct {
	marks     map[tabs.TabID]mark
	toasts    []toast // oldest first, at most maxToasts
	doneToast string  // the done_toast setting: how long a "done" card stays
}

// doneToastFor is how long a "done" card stays for a done_toast value; 0 means
// until seen. Anything unknown is the default, 6s.
func doneToastFor(v string) time.Duration {
	switch v {
	case "3s":
		return 3 * time.Second
	case "10s":
		return 10 * time.Second
	case "until_seen":
		return 0
	}
	return 6 * time.Second
}

// apply records one alert. It does nothing for a tab that is gone or for the
// tab the owner is looking at right now.
func (a *alerts) apply(m notify.Msg, active tabs.TabID, focused bool, exists func(tabs.TabID) bool, now time.Time) (chime notify.Kind, redraw bool) {
	id := tabs.TabID(m.Tab)
	if id <= 0 || !exists(id) || (id == active && focused) {
		return "", false
	}
	if m.Kind != notify.KindNeeds && m.Kind != notify.KindDone {
		return "", false
	}
	if a.marks == nil {
		a.marks = map[tabs.TabID]mark{}
	}
	// "needs" stays until the tab is seen, even if Claude finishes meanwhile.
	if m.Kind == notify.KindNeeds {
		a.marks[id] = markNeeds
	} else if a.marks[id] != markNeeds {
		a.marks[id] = markDone
	}
	t := toast{Tab: id, Kind: m.Kind, Text: cleanReason(m.Reason)}
	if d := doneToastFor(a.doneToast); m.Kind == notify.KindDone && d > 0 {
		t.Until = now.Add(d)
	}
	a.dropToast(id) // one card per tab: the latest news replaces the old
	a.toasts = append(a.toasts, t)
	if n := len(a.toasts); n > maxToasts {
		a.toasts = slices.Delete(a.toasts, 0, n-maxToasts)
	}
	return m.Kind, true
}

func (a *alerts) dropToast(id tabs.TabID) bool {
	n := len(a.toasts)
	a.toasts = slices.DeleteFunc(a.toasts, func(t toast) bool { return t.Tab == id && t.Run == 0 })
	return len(a.toasts) != n
}

// visited clears the tab's mark and its toast.
func (a *alerts) visited(id tabs.TabID) bool {
	_, had := a.marks[id]
	delete(a.marks, id)
	return a.dropToast(id) || had
}

// dismiss closes one toast but keeps the tab's mark: it was not seen yet.
func (a *alerts) dismiss(id tabs.TabID, run uint64) {
	a.toasts = slices.DeleteFunc(a.toasts, func(t toast) bool { return t.Tab == id && t.Run == run })
}

// addRun shows a run's summary, one per run, with the alert toasts.
func (a *alerts) addRun(t toast) {
	a.toasts = slices.DeleteFunc(a.toasts, func(o toast) bool { return o.Run == t.Run })
	a.toasts = append(a.toasts, t)
	if n := len(a.toasts); n > maxToasts {
		a.toasts = slices.Delete(a.toasts, 0, n-maxToasts)
	}
}

// expire drops timed-out toasts and returns when the next one is due.
func (a *alerts) expire(now time.Time) (next time.Time, changed bool) {
	n := len(a.toasts)
	a.toasts = slices.DeleteFunc(a.toasts, func(t toast) bool { return !t.Until.IsZero() && !now.Before(t.Until) })
	for _, t := range a.toasts {
		if !t.Until.IsZero() && (next.IsZero() || t.Until.Before(next)) {
			next = t.Until
		}
	}
	return next, len(a.toasts) != n
}

// prune forgets tabs that were closed.
func (a *alerts) prune(exists func(tabs.TabID) bool) {
	for id := range a.marks {
		if !exists(id) {
			delete(a.marks, id)
		}
	}
	a.toasts = slices.DeleteFunc(a.toasts, func(t toast) bool { return !exists(t.Tab) })
}

func (a *alerts) markOf(id tabs.TabID) mark { return a.marks[id] }

// list is the toasts newest first.
func (a *alerts) list() []toast {
	out := slices.Clone(a.toasts)
	slices.Reverse(out)
	return out
}

// cleanReason makes the sender's text safe to draw on one line: no control,
// format (bidi) or line-separator runes, at most maxReason runes.
func cleanReason(s string) string {
	var b strings.Builder
	n := 0
	for _, r := range s {
		if n == maxReason {
			break
		}
		if r == utf8.RuneError || unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			continue
		}
		b.WriteRune(r)
		n++
	}
	return strings.TrimSpace(b.String())
}

// dotColor is the dot a tab shows: an alert mark wins over the active accent.
func (c *chrome) dotColor(active bool, m mark) (color.NRGBA, bool) {
	switch {
	case m == markNeeds:
		return c.th.Needs, true
	case m == markDone:
		return c.th.Done, true
	case active:
		return c.th.Accent, true
	}
	return color.NRGBA{}, false
}
