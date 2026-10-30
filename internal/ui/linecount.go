package ui

import (
	"context"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/gitstat"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const countTimeout = 2 * time.Second

type lineCount struct{ add, del int }

type countResult struct {
	tab      tabs.TabID
	add, del int
	err      error
}

// lineCounter holds the git line counts. A count starts on an event only (a
// hook message or a shell prompt report), at most one in flight per tab; UI
// goroutine only, except the goroutines that send on results.
type lineCounter struct {
	fn       func(ctx context.Context, dir string) (int, int, error) // nil counts with git
	results  chan countResult
	inflight map[tabs.TabID]bool
	again    map[tabs.TabID]bool // an event arrived while a count ran
	counts   map[tabs.TabID]lineCount
	seen     map[tabs.TabID]uint64 // cwdSeq already counted
}

// wantCount counts t's repo unless a count for t is already running, in
// which case it runs once more when that one lands.
func (win *window) wantCount(t *tabs.Tab) {
	if t == nil {
		return
	}
	lc := &win.lc
	if lc.inflight[t.ID] {
		if lc.again == nil {
			lc.again = map[tabs.TabID]bool{}
		}
		lc.again[t.ID] = true
		return
	}
	loc := win.locOf(t)
	if loc.cwd == "" || loc.repo == "" {
		return
	}
	if lc.results == nil {
		lc.results = make(chan countResult, 64)
	}
	if lc.inflight == nil {
		lc.inflight = map[tabs.TabID]bool{}
	}
	lc.inflight[t.ID] = true
	fn, results, w, id, dir := lc.fn, lc.results, win.w, t.ID, loc.cwd
	if fn == nil {
		fn = gitstat.Shortstat
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), countTimeout)
		defer cancel()
		add, del, err := fn(ctx, dir)
		results <- countResult{id, add, del, err} // one per tab in flight, so never full
		if w != nil {
			w.Invalidate()
		}
	}()
}

// checkCounts counts tabs whose shell reported a folder since the last look.
func (win *window) checkCounts(list []*tabs.Tab) {
	for _, t := range list {
		seq := cwdSeq(t)
		if seq == 0 || win.lc.seen[t.ID] == seq {
			continue
		}
		if win.lc.seen == nil {
			win.lc.seen = map[tabs.TabID]uint64{}
		}
		win.lc.seen[t.ID] = seq
		win.wantCount(t)
	}
}

// drainCounts applies the counts that landed since the last frame.
func (win *window) drainCounts() {
	lc := &win.lc
	for {
		select {
		case r := <-lc.results:
			delete(lc.inflight, r.tab)
			t := win.mgr.Lookup(r.tab)
			if t == nil {
				delete(lc.again, r.tab)
				continue
			}
			if lc.counts == nil {
				lc.counts = map[tabs.TabID]lineCount{}
			}
			if r.err != nil {
				delete(lc.counts, r.tab) // not a repo, or git failed: show nothing
			} else {
				lc.counts[r.tab] = lineCount{r.add, r.del}
			}
			if lc.again[r.tab] {
				delete(lc.again, r.tab)
				win.wantCount(t)
			}
		default:
			return
		}
	}
}

func (win *window) linesOf(id tabs.TabID) (add, del int) {
	c := win.lc.counts[id]
	return c.add, c.del
}

// pruneCounts forgets closed tabs.
func (win *window) pruneCounts(list []*tabs.Tab) {
	for id := range win.lc.counts {
		if !tabIn(list, id) {
			delete(win.lc.counts, id)
		}
	}
	for id := range win.lc.seen {
		if !tabIn(list, id) {
			delete(win.lc.seen, id)
		}
	}
}
