package ui

import (
	"cmp"
	"image/color"
	"slices"
	"sync"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

// usageBox hands usage reports from notify goroutines to the UI goroutine. It
// is apart from the alert inbox, which keeps one message per tab, so a report
// can never replace a pending alert. Only the latest report per tab matters.
type usageBox struct {
	mu    sync.Mutex
	seq   uint64
	byTab map[tabs.TabID]usageReport
}

type usageReport struct {
	tab    tabs.TabID
	u      notify.Usage
	cwdSeq uint64 // the tab's folder-report count when the report arrived
	shell  uint64 // shellSeq when the report arrived
	seq    uint64
}

// put keeps the tab's newest report, but limits from an earlier, undrained
// report survive one that has none.
func (b *usageBox) put(tab tabs.TabID, u notify.Usage, cwdSeq, shell uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.seq++
	if b.byTab == nil {
		b.byTab = map[tabs.TabID]usageReport{}
	}
	if old, ok := b.byTab[tab]; ok {
		u.FiveHour = usage.Merge(old.u.FiveHour, u.FiveHour)
		u.Week = usage.Merge(old.u.Week, u.Week)
	}
	b.byTab[tab] = usageReport{tab, u, cwdSeq, shell, b.seq}
}

// take empties the box, oldest report first.
func (b *usageBox) take() []usageReport {
	b.mu.Lock()
	all := b.byTab
	b.byTab = nil
	b.mu.Unlock()
	out := make([]usageReport, 0, len(all))
	for _, r := range all {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b usageReport) int { return cmp.Compare(a.seq, b.seq) })
	return out
}

// ctxEntry is a tab's context use and the folder-report count when it came;
// a later report means the shell prompt is back, so Claude has exited.
type ctxEntry struct {
	pct    float64
	cwdSeq uint64
}

// drainUsage applies queued reports; UI goroutine only. Plan limits are
// account-wide, so any tab's report counts, merged by usage.Merge.
func (win *window) drainUsage() {
	for _, r := range win.usageIn.take() {
		win.acct.FiveHour = usage.Merge(win.acct.FiveHour, r.u.FiveHour)
		win.acct.Week = usage.Merge(win.acct.Week, r.u.Week)
		if !win.mgr.Has(r.tab) {
			continue
		}
		win.noteAgent(notify.Msg{Kind: notify.KindUsage}, r.tab, r.shell)
		if r.u.Ctx == nil {
			delete(win.ctx, r.tab)
			continue
		}
		if win.ctx == nil {
			win.ctx = map[tabs.TabID]ctxEntry{}
		}
		win.ctx[r.tab] = ctxEntry{pct: *r.u.Ctx, cwdSeq: r.cwdSeq}
	}
}

// usageSegments is the account limits plus the active tab's context while Claude still runs there.
func (win *window) usageSegments(active *tabs.Tab, now time.Time) []usage.Segment {
	u := win.acct
	if active != nil {
		if e, ok := win.ctx[active.ID]; ok && e.cwdSeq == cwdSeq(active) {
			u.Ctx = &e.pct
		}
	}
	return usage.Segments(u, now)
}

func cwdSeq(t *tabs.Tab) uint64 {
	s := t.Session
	s.Lock()
	defer s.Unlock()
	return s.Applier().CwdSeq()
}

func (c *chrome) usageColor(l usage.Level) color.NRGBA {
	switch l {
	case usage.High:
		return c.th.UsageHigh
	case usage.Warn:
		return c.th.Needs
	}
	return c.th.Folder
}
