package ui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

// waitCounts drains until cond holds, giving up after a bounded wait.
func waitCounts(t *testing.T, win *window, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("count result never arrived")
		}
		win.drainCounts()
		time.Sleep(time.Millisecond)
	}
}

func TestCountsRunOnEventsOnly(t *testing.T) {
	win, _ := sideWindow(t, false)
	tab := openIn(t, win, `C:\code\app`)
	var calls atomic.Int32
	release := make(chan struct{})
	win.lc.fn = func(ctx context.Context, dir string) (int, int, error) {
		calls.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return 1, 2, nil
	}
	list := win.mgr.Tabs()
	win.checkCounts(list) // the shell's first report is itself an event
	win.drainCounts()
	// Settle that first count so idle frames start from nothing in flight.
	close(release)
	waitCounts(t, win, func() bool { return len(win.lc.inflight) == 0 })
	base := calls.Load()
	for range 5 {
		win.checkCounts(list)
		win.drainCounts()
	}
	if got := calls.Load(); got != base {
		t.Fatalf("idle frames ran %d counts", got-base)
	}
	hold := make(chan struct{})
	defer close(hold)
	win.lc.fn = func(ctx context.Context, dir string) (int, int, error) {
		calls.Add(1)
		select {
		case <-hold:
		case <-ctx.Done():
		}
		return 1, 2, nil
	}
	win.deliver(notify.Msg{Tab: int(tab.ID), Kind: notify.KindDone}, t0)
	deadline := time.Now().Add(3 * time.Second)
	for calls.Load() != base+1 {
		if time.Now().After(deadline) {
			t.Fatal("done did not start a count")
		}
		time.Sleep(time.Millisecond)
	}
	win.deliver(notify.Msg{Tab: int(tab.ID), Kind: notify.KindDone}, t0)
	if got := calls.Load(); got != base+1 || len(win.lc.inflight) != 1 {
		t.Fatalf("calls %d inflight %d, want %d and 1", got, len(win.lc.inflight), base+1)
	}
}

func TestCountSkipsTabsWithoutARepo(t *testing.T) {
	win, _ := sideWindow(t, false)
	tab := openIn(t, win, `C:\elsewhere`)
	var calls atomic.Int32
	win.lc.fn = func(context.Context, string) (int, int, error) { calls.Add(1); return 1, 1, nil }
	win.wantCount(tab)
	if calls.Load() != 0 || len(win.lc.inflight) != 0 {
		t.Fatal("counted a folder outside any repo")
	}
}

func TestCountShowsOnTheRow(t *testing.T) {
	win, _ := sideWindow(t, false)
	tab := openIn(t, win, `C:\code\app`)
	win.lc.fn = func(context.Context, string) (int, int, error) { return 212, 14, nil }
	win.wantCount(tab)
	waitCounts(t, win, func() bool { return win.lc.counts[tab.ID] != lineCount{} })
	m := buildSide(win.sideIn())
	if len(m.Groups) != 1 || len(m.Groups[0].Rows) != 1 {
		t.Fatalf("model %+v", m)
	}
	if r := m.Groups[0].Rows[0]; r.Add != 212 || r.Del != 14 {
		t.Fatalf("row %+v", r)
	}
}

func TestRowWithCountsDraws(t *testing.T) {
	m := sideModel{Loose: []sideRow{{ID: 1, Title: "t", Folder: "~/x", Branch: "main", Add: 212, Del: 14, Mark: markDone}}}
	if h, _ := sideDraw(t, m, sideState{}, 200); len(h.rows) != 1 {
		t.Fatalf("%d rows drawn", len(h.rows))
	}
}
