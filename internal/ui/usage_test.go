package ui

import (
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

func usageMsg(tab int, five, ctx float64) notify.Msg {
	u := &notify.Usage{FiveHour: &notify.Window{Pct: five}}
	if ctx >= 0 {
		u.Ctx = &ctx
	}
	return notify.Msg{Tab: tab, Kind: notify.KindUsage, Usage: u}
}

func segTexts(s []usage.Segment) []string {
	out := []string{}
	for _, x := range s {
		out = append(out, x.Text)
	}
	return out
}

func sameTexts(t *testing.T, got []usage.Segment, want ...string) {
	t.Helper()
	g := segTexts(got)
	if len(g) != len(want) {
		t.Fatalf("got %q, want %q", g, want)
	}
	for i := range g {
		if g[i] != want[i] {
			t.Fatalf("got %q, want %q", g, want)
		}
	}
}

func TestUsageAfterDoneKeepsTheAlert(t *testing.T) {
	win, played := alertWindow(t, 2)
	win.liveMgr.Store(win.mgr)
	other := int(win.mgr.Tabs()[1].ID)
	win.opt.Config.ChimeDone = true
	win.onNotify(notify.Msg{Tab: other, Kind: notify.KindDone})
	win.onNotify(usageMsg(other, 10, 5))
	if len(*played) != 1 {
		t.Fatalf("chimes %d, want 1 (usage must not chime)", len(*played))
	}
	win.drainAlerts(t0)
	win.drainUsage()
	if win.alerts.markOf(win.mgr.Tabs()[1].ID) != markDone || len(win.alerts.toasts) != 1 {
		t.Fatalf("done alert lost: marks %v toasts %d", win.alerts.marks, len(win.alerts.toasts))
	}
}

func TestUsageAccountFromAnyTabContextOnlyForActive(t *testing.T) {
	win, _ := alertWindow(t, 2)
	win.liveMgr.Store(win.mgr)
	a, b := int(win.mgr.Tabs()[0].ID), int(win.mgr.Tabs()[1].ID)
	win.onNotify(usageMsg(a, 10, 30))
	win.onNotify(usageMsg(b, 12, 60)) // newer account numbers, other tab's context
	win.drainUsage()
	sameTexts(t, statusUsage(win), "5h 12%", "ctx 30%")
	win.mgr.SetActive(win.mgr.Tabs()[1].ID)
	sameTexts(t, statusUsage(win), "5h 12%", "ctx 60%")
}

func TestUsageWithoutLimitsKeepsAccountNumbers(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.liveMgr.Store(win.mgr)
	a := int(win.mgr.Tabs()[0].ID)
	win.onNotify(usageMsg(a, 40, 20))
	win.drainUsage()
	ctx := 25.0
	win.onNotify(notify.Msg{Tab: a, Kind: notify.KindUsage, Usage: &notify.Usage{Ctx: &ctx}})
	win.drainUsage()
	sameTexts(t, statusUsage(win), "5h 40%", "ctx 25%")
}

func TestUsageWithoutContextClearsIt(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.liveMgr.Store(win.mgr)
	a := int(win.mgr.Tabs()[0].ID)
	win.onNotify(usageMsg(a, 40, 20))
	win.drainUsage()
	win.onNotify(usageMsg(a, 41, -1))
	win.drainUsage()
	sameTexts(t, statusUsage(win), "5h 41%")
}

func TestPromptAfterUsageHidesContext(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.liveMgr.Store(win.mgr)
	win.branchFn = func(string) string { return "" }
	tab := win.mgr.Tabs()[0]
	report(t, tab, tmp(t)) // the shell prompt that started Claude
	win.onNotify(usageMsg(int(tab.ID), 40, 20))
	win.drainUsage()
	sameTexts(t, statusUsage(win), "5h 40%", "ctx 20%")
	report(t, tab, tmp(t)) // Claude exited: the prompt is back
	sameTexts(t, statusUsage(win), "5h 40%")
}

func TestExpiredAccountWindowHidden(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.liveMgr.Store(win.mgr)
	a := int(win.mgr.Tabs()[0].ID)
	past := time.Now().Add(-time.Minute).Unix()
	win.onNotify(notify.Msg{Tab: a, Kind: notify.KindUsage, Usage: &notify.Usage{FiveHour: &notify.Window{Pct: 50, Resets: past}}})
	win.drainUsage()
	sameTexts(t, statusUsage(win))
}

func TestUsageForClosedTabIsDropped(t *testing.T) {
	win, _ := alertWindow(t, 2)
	win.liveMgr.Store(win.mgr)
	gone := win.mgr.Tabs()[1].ID
	win.onNotify(usageMsg(int(gone), 10, 70))
	win.drainUsage()
	win.mgr.Close(gone)
	win.pruneLocs(win.mgr.Tabs())
	if _, ok := win.ctx[gone]; ok {
		t.Fatal("closed tab's context kept")
	}
}

func TestIdleTabCannotLowerPlanUsage(t *testing.T) {
	win, _ := alertWindow(t, 2)
	win.liveMgr.Store(win.mgr)
	a, b := int(win.mgr.Tabs()[0].ID), int(win.mgr.Tabs()[1].ID)
	reset := time.Now().Add(time.Hour).Unix()
	report := func(tab int, p float64) notify.Msg {
		return notify.Msg{Tab: tab, Kind: notify.KindUsage, Usage: &notify.Usage{FiveHour: &notify.Window{Pct: p, Resets: reset}}}
	}
	win.onNotify(report(a, 62))
	win.drainUsage()
	win.onNotify(report(b, 48)) // b's last reply was long ago
	win.drainUsage()
	sameTexts(t, statusUsage(win), "5h 62%")
}

func TestLimitsSurviveAReportWithoutThemBeforeTheFrame(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.liveMgr.Store(win.mgr)
	a := int(win.mgr.Tabs()[0].ID)
	win.onNotify(usageMsg(a, 40, 20))
	ctx := 21.0
	win.onNotify(notify.Msg{Tab: a, Kind: notify.KindUsage, Usage: &notify.Usage{Ctx: &ctx}})
	win.drainUsage()
	sameTexts(t, statusUsage(win), "5h 40%", "ctx 21%")
}

func TestContextReportedBeforeThePromptStaysHidden(t *testing.T) {
	// A report that arrived while Claude ran but was drained after the prompt came back.
	win, _ := alertWindow(t, 1)
	win.liveMgr.Store(win.mgr)
	win.branchFn = func(string) string { return "" }
	tab := win.mgr.Tabs()[0]
	report(t, tab, tmp(t))
	win.onNotify(usageMsg(int(tab.ID), 40, 20))
	report(t, tab, tmp(t)) // Claude exited before the next frame
	win.drainUsage()
	sameTexts(t, statusUsage(win), "5h 40%")
}
