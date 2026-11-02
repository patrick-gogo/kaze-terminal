// Package resources measures this Kaze process and its observable descendants.
// It never inspects command lines, launches tools, or polls on its own.
package resources

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

const MaxProcesses = 16384

var ErrGone = errors.New("process exited or identity changed")

type Process struct {
	PID, Parent int
	Birth       uint64
}
type Reading struct {
	// At is captured next to the native CPU counter read, with a monotonic clock.
	At                 time.Time
	CPU                time.Duration
	Bytes              uint64
	CPUValid, RAMValid bool
}
type Provider interface {
	List() ([]Process, bool, error)
	Read(Process) (Reading, error)
	Cores() int
}
type Root struct {
	PID, Owner int
	Name       string
}
type Value struct {
	CPU                                float64
	Bytes                              uint64
	CPUValid, RAMValid                 bool
	MissingCPU, MissingRAM, WarmingCPU int
}
type Row struct {
	ID     int
	Name   string
	Closed bool
	Value
}
type Snapshot struct {
	At       time.Time
	Interval time.Duration
	Cores    int
	Rows     []Row
	Total    Value
	Partial  bool
	Error    string
	Level    int
}
type identity struct {
	pid   int
	birth uint64
}
type baseline struct {
	cpu time.Duration
	at  time.Time
}
type interval struct {
	pct float64
	dt  time.Duration
}
type Collector struct {
	self     int
	provider Provider
	owned    map[identity]int
	previous map[identity]baseline
	roots    map[[2]int]uint64
	names    map[int]string
	history  []interval
	last     time.Time
}

func New(self int, p Provider) *Collector { return &Collector{self: self, provider: p} }
func Level(p float64) int {
	if p >= 80 {
		return 2
	}
	if p >= 50 {
		return 1
	}
	return 0
}
func Size(n uint64) string {
	if n >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}
func clamp(v float64) float64 { return math.Max(0, math.Min(v, 100)) }
func (c *Collector) Sample(now time.Time, roots []Root, reset bool) Snapshot {
	s := Snapshot{At: now, Cores: c.provider.Cores()}
	if s.Cores < 1 {
		s.Cores = 0
		s.Partial = true
		s.Error = "CPU core count unavailable"
	}
	// Idle intervals still have useful cumulative counters. Only an explicit
	// suspend/resume reset invalidates every baseline; per-process identity and
	// counter discontinuities are handled independently below.
	if reset {
		c.previous = nil
		c.history = nil
		c.last = time.Time{}
	}
	ps, partial, err := c.provider.List()
	if err != nil {
		s.Error = err.Error()
		s.Partial = true
		return s
	}
	s.Partial = s.Partial || partial
	if len(ps) > MaxProcesses {
		ps = ps[:MaxProcesses]
		s.Partial = true
	}
	if len(roots) > MaxProcesses {
		roots = roots[:MaxProcesses]
		s.Partial = true
	}
	ps = c.inspectCandidates(ps, roots)
	byPID := make(map[int]Process, len(ps))
	for _, p := range ps {
		byPID[p.PID] = p
	}
	if c.roots == nil {
		c.roots = map[[2]int]uint64{}
	}
	if c.names == nil {
		c.names = map[int]string{}
	}
	liveNames := map[int]string{0: "Kaze itself", -1: "Other Kaze processes"}
	direct := map[int]int{c.self: 0}
	requested := map[int]int{c.self: 0}
	nextRoots := map[[2]int]uint64{}
	for _, r := range roots {
		liveNames[r.Owner] = r.Name
		c.names[r.Owner] = r.Name
		p, ok := byPID[r.PID]
		if !ok {
			continue
		}
		key := [2]int{r.PID, r.Owner}
		birth, seen := c.roots[key]
		if !seen || birth == 0 {
			birth = p.Birth
		}
		nextRoots[key] = birth
		if p.Birth != 0 && birth == p.Birth {
			direct[r.PID] = r.Owner
			requested[r.PID] = r.Owner
		} else if p.Birth == 0 {
			requested[r.PID] = r.Owner
		}
	}
	c.roots = nextRoots
	owners := map[int]int{}
	visiting := map[int]bool{}
	var find func(int) (int, bool)
	find = func(pid int) (int, bool) {
		if o, ok := owners[pid]; ok {
			return o, true
		}
		p, ok := byPID[pid]
		if !ok || p.Birth == 0 || visiting[pid] {
			return 0, false
		}
		visiting[pid] = true
		if o, ok := direct[pid]; ok {
			owners[pid] = o
			return o, true
		}
		if o, ok := c.owned[identity{pid, p.Birth}]; ok {
			owners[pid] = o
			return o, true
		}
		parent, ok := byPID[p.Parent]
		if !ok || parent.Birth == 0 || parent.Birth > p.Birth {
			return 0, false
		}
		o, ok := find(parent.PID)
		if ok {
			if parent.PID == c.self {
				o = -1
			}
			owners[pid] = o
		}
		return o, ok
	}
	// A denied identity can only be reported under a current root/tree. Matching
	// an old PID alone must not retain a closed row or contaminate its totals.
	unknownOwner := func(p Process) (int, bool) {
		seen := map[int]bool{}
		for p.PID != 0 && !seen[p.PID] {
			seen[p.PID] = true
			if o, ok := requested[p.PID]; ok {
				return o, true
			}
			parent, ok := byPID[p.Parent]
			if !ok || parent.Birth == 0 {
				break
			}
			if parent.PID == c.self {
				return -1, true
			}
			p = parent
		}
		return 0, false
	}
	nextOwned := map[identity]int{}
	nextPrevious := map[identity]baseline{}
	rows := map[int]*Row{0: {ID: 0, Name: "Kaze itself"}}
	rowFor := func(owner int) *Row {
		if r := rows[owner]; r != nil {
			return r
		}
		name, live := liveNames[owner]
		if !live {
			name = c.names[owner]
		}
		r := &Row{ID: owner, Name: name, Closed: !live}
		rows[owner] = r
		return r
	}
	for _, p := range ps {
		if p.Birth == 0 {
			if owner, known := unknownOwner(p); known {
				r := rowFor(owner)
				r.MissingCPU++
				r.MissingRAM++
				s.Partial = true
			}
			continue
		}
		owner, ok := find(p.PID)
		if !ok {
			continue
		}
		key := identity{p.PID, p.Birth}
		r, err := c.provider.Read(p)
		if errors.Is(err, ErrGone) {
			continue
		}
		nextOwned[key] = owner
		row := rowFor(owner)
		if err != nil {
			r = Reading{}
			s.Partial = true
		}
		if r.At.IsZero() && r.CPUValid {
			r.At = time.Now()
		} // timestamp at this read, never the frame request
		if r.At.After(s.At) {
			s.At = r.At
		}
		if r.RAMValid {
			row.Bytes += r.Bytes
			row.RAMValid = true
		} else {
			row.MissingRAM++
			s.Partial = true
		}
		prev, has := c.previous[key]
		if previousOwner, ok := c.owned[key]; ok && previousOwner != owner {
			has = false
		}
		dt := r.At.Sub(prev.at)
		if r.CPUValid {
			nextPrevious[key] = baseline{r.CPU, r.At}
		}
		switch {
		case !r.CPUValid || s.Cores == 0:
			row.MissingCPU++
			s.Partial = true
		case has && dt > 0 && r.CPU >= prev.cpu:
			row.CPU += float64(r.CPU-prev.cpu) / float64(dt) / float64(s.Cores) * 100
			row.CPUValid = true
		default:
			row.WarmingCPU++
		}
	}
	c.owned, c.previous = nextOwned, nextPrevious
	nextNames := map[int]string{}
	for id, row := range rows {
		if id > 0 {
			nextNames[id] = row.Name
		}
	}
	c.names = nextNames
	ids := make([]int, 0, len(rows))
	for id := range rows {
		if id != 0 {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	ids = append([]int{0}, ids...)
	for _, id := range ids {
		row := rows[id]
		row.CPU = clamp(row.CPU)
		s.Rows = append(s.Rows, *row)
		s.Total.CPU += row.CPU
		s.Total.Bytes += row.Bytes
		s.Total.CPUValid = s.Total.CPUValid || row.CPUValid
		s.Total.RAMValid = s.Total.RAMValid || row.RAMValid
		s.Total.MissingCPU += row.MissingCPU
		s.Total.MissingRAM += row.MissingRAM
		s.Total.WarmingCPU += row.WarmingCPU
	}
	s.Total.CPU = clamp(s.Total.CPU)
	if !c.last.IsZero() && s.At.After(c.last) {
		s.Interval = s.At.Sub(c.last)
	}
	c.last = s.At
	return c.smooth(s, s.Interval)
}
func (c *Collector) smooth(s Snapshot, dt time.Duration) Snapshot {
	if s.Total.CPUValid && dt > 0 {
		c.history = append(c.history, interval{s.Total.CPU, dt})
		if len(c.history) > 3 {
			c.history = c.history[len(c.history)-3:]
		}
		var total, weight float64
		for _, h := range c.history {
			total += h.pct * float64(h.dt)
			weight += float64(h.dt)
		}
		if weight > 0 {
			s.Level = Level(total / weight)
		}
	}
	return s
}
