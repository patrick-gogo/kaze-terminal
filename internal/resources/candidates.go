package resources

import "errors"

// A lightweight parent snapshot need not open handles to unrelated processes.
// Platforms with such snapshots resolve creation identity only for candidates.
type processInspector interface {
	Identify(Process) (Process, error)
}

func (c *Collector) inspectCandidates(ps []Process, roots []Root) []Process {
	inspector, ok := c.provider.(processInspector)
	if !ok {
		return ps
	}
	byPID := make(map[int]Process, len(ps))
	children := make(map[int][]int, len(ps))
	for _, p := range ps {
		byPID[p.PID] = p
		children[p.Parent] = append(children[p.Parent], p.PID)
	}
	type inspected struct {
		p   Process
		err error
	}
	cache := map[int]inspected{}
	read := func(pid int) inspected {
		if r, ok := cache[pid]; ok {
			return r
		}
		p, ok := byPID[pid]
		if !ok {
			return inspected{}
		}
		p, err := inspector.Identify(p)
		r := inspected{p, err}
		cache[pid] = r
		return r
	}
	type seed struct {
		pid      int
		birth    uint64
		retained bool
	}
	seeds := []seed{{pid: c.self}}
	for _, r := range roots {
		seeds = append(seeds, seed{pid: r.PID, birth: c.roots[[2]int{r.PID, r.Owner}]})
	}
	for key := range c.owned {
		seeds = append(seeds, seed{pid: key.pid, birth: key.birth, retained: true})
	}
	selected := map[int]bool{}
	var out []Process
	for _, root := range seeds {
		r := read(root.pid)
		// A historical PID is only a lookup hint, never evidence of current ownership.
		// Do not follow a replacement's children when identity is denied or changed.
		if root.retained && (r.err != nil || r.p.Birth == 0 || r.p.Birth != root.birth) {
			continue
		}
		if root.birth != 0 && r.p.Birth != 0 && r.p.Birth != root.birth {
			continue
		}
		queue := []int{root.pid}
		for len(queue) > 0 {
			pid := queue[len(queue)-1]
			queue = queue[:len(queue)-1]
			if selected[pid] {
				continue
			}
			selected[pid] = true
			r := read(pid)
			if r.p.PID == 0 || errors.Is(r.err, ErrGone) {
				continue
			}
			// Keep a current tree's denied record for an honest unavailable count, but
			// never keep historical ownership or expand through an unverified identity.
			if r.err != nil {
				r.p.Birth = 0
			}
			out = append(out, r.p)
			if r.p.Birth != 0 {
				queue = append(queue, children[pid]...)
			}
		}
	}
	return out
}
