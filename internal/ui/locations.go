package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/gitinfo"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const maxLocationWorkers = 2

// locationWork has at most two background lookups and one pending request per
// live tab. Only the UI goroutine touches its maps or applies results.
type locationWork struct {
	cache    gitinfo.Cache
	lookup   func(string) gitinfo.Info
	results  chan locationResult
	inflight map[uint64]bool
	next     uint64
}
type locationResult struct {
	id     tabs.TabID
	ticket uint64
	info   gitinfo.Info
}

func (win *window) requestLocation(t *tabs.Tab, cwd string, seq uint64) locEntry {
	e := win.locs[t.ID]
	if e.cwd != cwd {
		e = locEntry{}
	}
	win.locations.next++
	e.cwd, e.seq, e.ticket, e.pending, e.stale = cwd, seq, win.locations.next, true, false
	if win.locs == nil {
		win.locs = map[tabs.TabID]locEntry{}
	}
	win.locs[t.ID] = e
	win.startLocations()
	return e
}

func (win *window) startLocations() {
	l := &win.locations
	if l.results == nil {
		l.results = make(chan locationResult, maxLocationWorkers)
		l.inflight = make(map[uint64]bool)
	}
	for id, e := range win.locs {
		if len(l.inflight) >= maxLocationWorkers {
			return
		}
		if !e.pending || e.stale || l.inflight[e.ticket] {
			continue
		}
		l.inflight[e.ticket] = true
		lookup := l.lookup
		if lookup == nil {
			lookup = l.cache.Lookup
		}
		results, w := l.results, win.w
		go func(id tabs.TabID, e locEntry) {
			info := lookup(e.cwd)
			results <- locationResult{id, e.ticket, info}
			if w != nil {
				w.Invalidate()
			}
		}(id, e)
	}
}

func (win *window) drainLocations() {
	l := &win.locations
	for {
		select {
		case r := <-l.results:
			delete(l.inflight, r.ticket)
			e, ok := win.locs[r.id]
			if !ok || e.ticket != r.ticket || e.stale {
				continue
			}
			t := win.mgr.Lookup(r.id)
			if t == nil {
				delete(win.locs, r.id)
				continue
			}
			t.Session.Lock()
			cwd, seq := t.Session.Applier().Cwd(), t.Session.Applier().CwdSeq()
			t.Session.Unlock()
			if cwd != e.cwd || seq != e.seq {
				if cwd == "" {
					delete(win.locs, r.id)
				} else {
					win.requestLocation(t, cwd, seq)
				}
				continue
			}
			e.branch, e.repo, e.repoName, e.pending = r.info.Branch, r.info.Repo, r.info.Name, false
			win.locs[r.id] = e
			// The first folder report may have arrived before repo discovery.
			delete(win.lc.seen, r.id)
		default:
			win.startLocations()
			return
		}
	}
}
