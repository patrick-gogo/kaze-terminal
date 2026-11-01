package ui

import (
	"os"
	"slices"
	"sync/atomic"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/resources"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

type resourceResult struct {
	generation uint64
	roots      []resources.Root
	snapshot   resources.Snapshot
}
type resourceWork struct {
	sample          func(time.Time, []resources.Root, bool) resources.Snapshot
	wake            func()
	in              chan resourceResult
	busy            bool
	last            time.Time
	generation      uint64
	reset           bool
	paused, stopped atomic.Bool
	snapshot        resources.Snapshot
}

func newResourceWork(sample func(time.Time, []resources.Root, bool) resources.Snapshot, wake func()) *resourceWork {
	return &resourceWork{sample: sample, wake: wake, in: make(chan resourceResult, 1), reset: true}
}
func (w *resourceWork) close() {
	if w != nil {
		w.stopped.Store(true)
	}
}
func (w *resourceWork) pause(p bool) {
	if w == nil || w.paused.Swap(p) == p {
		return
	}
	w.generation++
	w.reset = true
	w.last = time.Time{}
}
func (w *resourceWork) tick(now time.Time, roots []resources.Root) {
	if w == nil || w.stopped.Load() {
		return
	}
	completed := false
	select {
	case r := <-w.in:
		w.busy = false
		if r.generation == w.generation && slices.Equal(r.roots, roots) && !w.paused.Load() {
			w.snapshot = r.snapshot
			completed = true
		} else {
			w.last = time.Time{}
			w.reset = true
		}
	default:
	}
	if w.paused.Load() || w.busy || completed || (!w.last.IsZero() && now.Sub(w.last) < memEvery) {
		return
	}
	w.busy = true
	w.last = now
	roots = append([]resources.Root(nil), roots...)
	generation, reset := w.generation, w.reset
	w.reset = false
	go func() {
		s := w.sample(now, roots, reset)
		if w.stopped.Load() {
			return
		}
		w.in <- resourceResult{generation, roots, s}
		if !w.paused.Load() && !w.stopped.Load() && w.wake != nil {
			w.wake()
		}
	}()
}

// needRoots avoids walking/labeling every terminal on every streaming frame.
func (w *resourceWork) needRoots(now time.Time) bool {
	return w != nil && !w.stopped.Load() && !w.paused.Load() && (len(w.in) > 0 || (!w.busy && (w.last.IsZero() || now.Sub(w.last) >= memEvery)))
}
func (win *window) resourceRoots() []resources.Root {
	if win.mgr == nil {
		return nil
	}
	list := win.mgr.Tabs()
	var roots []resources.Root
	names := map[tabs.TabID]string{}
	for _, t := range list {
		name := win.tabResourceName(t)
		names[t.ID] = name
	}
	for _, t := range list {
		pid := t.Session.RootPID()
		if pid == 0 {
			continue
		}
		owner := t.ID
		if t.Parent != 0 {
			owner = t.Parent
		}
		roots = append(roots, resources.Root{PID: pid, Owner: int(owner), Name: names[owner]})
	}
	return roots
}
func (win *window) tabResourceName(t *tabs.Tab) string {
	if t.Group != nil {
		return cleanText(t.Group.Name + " / " + tabLabel(t, win.defTitle))
	}
	if name := win.names[t.ID]; name != "" {
		return cleanText(name)
	}
	return tabLabel(t, win.defTitle)
}
func (win *window) initResources() {
	if win.bench != nil {
		return
	}
	c := resources.New(os.Getpid(), resources.Native())
	win.resources = newResourceWork(c.Sample, win.w.Invalidate)
}
