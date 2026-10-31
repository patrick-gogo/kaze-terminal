package ui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestCountClearsWhenTheFolderLeavesTheRepo(t *testing.T) {
	win, _ := sideWindow(t, false)
	tab := openIn(t, win, osPath("/code/app"))
	win.lc.fn = func(context.Context, string) (int, int, error) { return 5, 6, nil }
	win.checkCounts(win.mgr.Tabs())
	waitCounts(t, win, func() bool { return win.lc.counts[tab.ID] != lineCount{} })
	report(t, tab, osPath("/elsewhere"))
	win.checkCounts(win.mgr.Tabs())
	if a, d := win.linesOf(tab.ID); a != 0 || d != 0 {
		t.Fatalf("kept %d, %d outside a repo", a, d)
	}
}

func TestEventDuringACountRunsExactlyOneMore(t *testing.T) {
	win, _ := sideWindow(t, false)
	tab := openIn(t, win, osPath("/code/app"))
	var calls atomic.Int32
	release := make(chan struct{}, 4)
	win.lc.fn = func(ctx context.Context, dir string) (int, int, error) {
		calls.Add(1)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return 1, 1, nil
	}
	win.wantCount(tab)
	win.wantCount(tab)
	win.wantCount(tab)
	release <- struct{}{}
	waitCounts(t, win, func() bool { return calls.Load() == 2 })
	release <- struct{}{}
	waitCounts(t, win, func() bool { return len(win.lc.inflight) == 0 })
	time.Sleep(20 * time.Millisecond)
	win.drainCounts()
	if got := calls.Load(); got != 2 {
		t.Fatalf("%d counts, want 2", got)
	}
}
