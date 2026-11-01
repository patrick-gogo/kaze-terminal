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
	CPU                    float64
	Bytes                  uint64
	CPUValid, RAMValid     bool
	MissingCPU, MissingRAM int
}
type Row struct {
	ID     int
	Name   string
	Closed bool
	Value
}
type Snapshot struct {
	At      time.Time
	Cores   int
	Rows    []Row
	Total   Value
	Partial bool
	Error   string
	Level   int
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
	elapsed := now.Sub(c.last)
	s := Snapshot{At: now, Cores: c.provider.Cores()}
	if s.Cores < 1 {
		s.Cores = 0
		s.Partial = true
		s.Error = "CPU core count unavailable"
	}
	if reset || (!c.last.IsZero() && (now.Sub(c.last) > 10*time.Second || !now.After(c.last))) {
		c.previous = nil
		c.history = nil
	}
	c.last = now
	ps, partial, err := c.provider.List()
	if err != nil {
		s.Error = err.Error()
		s.Partial = true
		c.previous = nil
		c.history = nil
		return s
	}
	s.Partial = s.Partial || partial
	if len(ps) > MaxProcesses {
		ps = ps[:MaxProcesses]
		s.Partial = true
	}
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
	if len(roots) > MaxProcesses {
		roots = roots[:MaxProcesses]
		s.Partial = true
	}
	for _, r := range roots {
		requested[r.PID] = r.Owner
		liveNames[r.Owner] = r.Name
		c.names[r.Owner] = r.Name
		p, ok := byPID[r.PID]
		if !ok {
			continue
		}
		key := [2]int{r.PID, r.Owner}
		birth, seen := c.roots[key]
		if !seen {
			birth = p.Birth
		}
		nextRoots[key] = birth
		if p.Birth != 0 && birth == p.Birth {
			direct[r.PID] = r.Owner
		} else {
			s.Partial = true
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
	nextOwned := map[identity]int{}
	nextPrevious := map[identity]baseline{}
	rows := map[int]*Row{0: {ID: 0, Name: "Kaze itself"}}
	for _, p := range ps {
		if p.Birth == 0 {
			owner, known := requested[p.PID]
			for key, o := range c.owned {
				if key.pid == p.PID {
					owner, known = o, true
					nextOwned[key] = o
					break
				}
			}
			if known {
				row := rows[owner]
				if row == nil {
					name, live := liveNames[owner]
					if !live {
						name = c.names[owner]
					}
					row = &Row{ID: owner, Name: name, Closed: !live}
					rows[owner] = row
				}
				row.MissingCPU++
				row.MissingRAM++
				s.Partial = true
			}
			continue
		}
		owner, ok := find(p.PID)
		if !ok {
			continue
		}
		key := identity{p.PID, p.Birth}
		nextOwned[key] = owner
		row := rows[owner]
		if row == nil {
			name, live := liveNames[owner]
			if !live {
				name = c.names[owner]
			}
			row = &Row{ID: owner, Name: name, Closed: !live}
			rows[owner] = row
		}
		r, err := c.provider.Read(p)
		if errors.Is(err, ErrGone) {
			delete(nextOwned, key)
			continue
		}
		if err != nil {
			r = Reading{}
			s.Partial = true
		}
		if r.RAMValid {
			row.Bytes += r.Bytes
			row.RAMValid = true
		} else {
			row.MissingRAM++
			s.Partial = true
		}
		prev, has := c.previous[key]
		dt := now.Sub(prev.at)
		if r.CPUValid {
			nextPrevious[key] = baseline{r.CPU, now}
		}
		if !r.CPUValid {
			s.Partial = true
		}
		if s.Cores > 0 && r.CPUValid && has && dt > 0 && r.CPU >= prev.cpu {
			row.CPU += float64(r.CPU-prev.cpu) / float64(dt) / float64(s.Cores) * 100
			row.CPUValid = true
		} else {
			row.MissingCPU++
		}
	}
	c.owned, c.previous = nextOwned, nextPrevious
	// Forget labels and root identities when no live observed process uses them.
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
	}
	s.Total.CPU = clamp(s.Total.CPU)
	if s.Total.CPUValid && s.Total.MissingCPU > 0 {
		s.Partial = true
	}
	return c.smooth(s, elapsed)
}
func (c *Collector) smooth(s Snapshot, dt time.Duration) Snapshot {
	// All process readings in one scan share one timestamp. Keep interval weights
	// independently of per-process baselines, which are replaced after every scan.
	if s.Total.CPUValid {
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
