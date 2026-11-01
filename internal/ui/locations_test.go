package ui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/gitinfo"
)

func waitLocations(t *testing.T, win *window, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("location result did not arrive")
		}
		win.drainLocations()
		time.Sleep(time.Millisecond)
	}
}

func TestLocationsDoNotBlockAndRejectStaleResults(t *testing.T) {
	win := testWindow(t, &recStarter{})
	tab, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan string, 4)
	release := make(chan struct{}, 8)
	defer close(release)
	win.locations.lookup = func(dir string) gitinfo.Info {
		started <- dir
		<-release
		return gitinfo.Info{Branch: dir, Repo: dir, Name: dir}
	}
	report(t, tab, osPath("/old"))
	if e := win.locOf(tab); e.cwd != osPath("/old") || e.branch != "" {
		t.Fatalf("initial %+v", e)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no lookup")
	}
	report(t, tab, osPath("/new"))
	win.locOf(tab)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("new folder not queued")
	}
	// Release both generations together; the old result cannot win.
	release <- struct{}{}
	release <- struct{}{}
	waitLocations(t, win, func() bool { return win.locs[tab.ID].branch == osPath("/new") })
	if win.locs[tab.ID].repo != osPath("/new") {
		t.Fatal("stale repo applied")
	}
	win.mgr.Close(tab.ID)
	win.pruneLocs(nil)
	win.drainLocations()
	if len(win.locs) != 0 {
		t.Fatal("closed tab retained")
	}
}

func TestLocationsBoundWorkersAndRefreshOnInvalidation(t *testing.T) {
	win := testWindow(t, &recStarter{})
	var active, maxActive, calls atomic.Int32
	release := make(chan struct{}, 8)
	defer close(release)
	win.locations.lookup = func(dir string) gitinfo.Info {
		n := active.Add(1)
		for old := maxActive.Load(); n > old && !maxActive.CompareAndSwap(old, n); old = maxActive.Load() {
		}
		calls.Add(1)
		<-release
		active.Add(-1)
		return gitinfo.Info{Branch: "main", Repo: "repo", Name: "repo"}
	}
	for range 5 {
		tab, err := win.mgr.New()
		if err != nil {
			t.Fatal(err)
		}
		report(t, tab, osPath("/repo"))
		win.locOf(tab)
	}
	if len(win.locations.inflight) > 2 {
		t.Fatal("unbounded workers")
	}
	for range 5 {
		release <- struct{}{}
		win.drainLocations()
	}
	waitLocations(t, win, func() bool { win.drainLocations(); return calls.Load() == 5 && len(win.locations.inflight) == 0 })
	if maxActive.Load() > 2 {
		t.Fatalf("%d workers", maxActive.Load())
	}
	tab := win.mgr.Active()
	for range 3 {
		win.locOf(tab)
	}
	if calls.Load() != 5 {
		t.Fatal("unchanged frames repeated lookup")
	}
	win.staleBranch(tab)
	if got := win.locOf(tab); got.repo != "repo" {
		t.Fatal("refresh hid known repository")
	}
	release <- struct{}{}
	waitLocations(t, win, func() bool { return calls.Load() == 6 && len(win.locations.inflight) == 0 })
}

func TestLocationsRejectInvalidatedAndClosedRequests(t *testing.T) {
	win := testWindow(t, &recStarter{})
	tab, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan int, 4)
	release := make(chan struct{}, 4)
	var calls atomic.Int32
	win.locations.lookup = func(string) gitinfo.Info {
		n := int(calls.Add(1))
		started <- n
		<-release
		if n == 1 {
			return gitinfo.Info{Branch: "old"}
		}
		return gitinfo.Info{Branch: "new"}
	}
	report(t, tab, osPath("/repo"))
	win.locOf(tab)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no lookup")
	}
	win.staleBranch(tab)
	win.locOf(tab)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no refresh")
	}
	release <- struct{}{}
	release <- struct{}{}
	waitLocations(t, win, func() bool { return len(win.locations.inflight) == 0 })
	if win.locs[tab.ID].branch != "new" {
		t.Fatal("older refresh won")
	}
	win.staleBranch(tab)
	win.locOf(tab)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("no closing lookup")
	}
	win.mgr.Close(tab.ID)
	win.pruneLocs(nil)
	release <- struct{}{}
	waitLocations(t, win, func() bool { return len(win.locations.inflight) == 0 })
	if len(win.locs) != 0 {
		t.Fatal("late result restored closed tab")
	}
}

func TestLocationDiscoveryRecountsAfterPendingFolderReport(t *testing.T) {
	win := testWindow(t, &recStarter{})
	tab, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	win.locations.lookup = func(string) gitinfo.Info { return gitinfo.Info{Branch: "main", Repo: "repo", Name: "repo"} }
	win.lc.fn = func(context.Context, string) (int, int, error) { return 7, 3, nil }
	report(t, tab, osPath("/repo"))
	win.checkCounts(win.mgr.Tabs())
	waitLocations(t, win, func() bool { return win.locs[tab.ID].repo == "repo" })
	win.checkCounts(win.mgr.Tabs())
	waitCounts(t, win, func() bool { return win.lc.counts[tab.ID] == (lineCount{7, 3}) })
}
