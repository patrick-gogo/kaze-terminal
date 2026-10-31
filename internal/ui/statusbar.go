package ui

import (
	"image"
	"strconv"
	"strings"
)

// statusGroup is one part of the status bar (spec 6.1).
type statusGroup int

const (
	grpFolder statusGroup = iota
	grpBranch
	grpUpdate
	grpAgents
	grpServers
	grpUsage
	grpMem
	grpTabs
	grpNotice
)

// statusDims are the status bar's sizes in px.
type statusDims struct {
	H, Pad, ItemPad, Sep, Icon, Gap, Meter, PillPad, Dot int
}

// statusLayout is where the status bar's visible groups go, and what was
// compacted to fit (spec 6.4).
type statusLayout struct {
	Groups  map[statusGroup]image.Rectangle // only the shown ones
	Compact struct {
		ServersLabel, UsageLabels, UsageMeters bool
		Agents, Update, AggregateServers       bool
		BranchCounts                           bool // the +N -M were dropped
	}
	FolderText, BranchText, NoticeText string
	UsageKeys                          []string // the usage entries shown, in order
	Seps                               []int    // x of each hairline between right-hand groups
	Tight                              bool     // padding reduced and separators dropped (step 7)
}

// agentsPill is the agents group's text.
func agentsPill(m statusModel, compact bool) string {
	if compact {
		return strconv.Itoa(m.Agents)
	}
	s := strconv.Itoa(m.Agents) + " agents"
	if m.Agents == 1 {
		s = "1 agent"
	}
	if m.Waiting > 0 {
		s += " · " + strconv.Itoa(m.Waiting) + " waiting"
	}
	return s
}

// serversCount is "up/total".
func serversCount(ds []serverDot) string {
	up := 0
	for _, d := range ds {
		if d.Up {
			up++
		}
	}
	return strconv.Itoa(up) + "/" + strconv.Itoa(len(ds))
}

// statusFit is one compaction state of the layout.
type statusFit struct {
	noTabs, noMem, serversLabel, usageLabels           bool
	folderBudget, branchBudget                         int // -1: as is; 0: omitted
	noCounts, compactPills, noMeters, tight, aggregate bool
	noServers, noBranch, noUpdate, noAgents            bool
	usageDrop                                          int // usage entries dropped from the end (ctx, wk, 5h)
}

// layoutStatusBar fits the model into width px, taking the spec's steps in
// order and stopping as soon as everything fits.
func layoutStatusBar(m statusModel, width int, measure func(s string, mono bool) int, d statusDims) statusLayout {
	f := statusFit{folderBudget: -1, branchBudget: -1}
	steps := []func(){
		func() { f.noTabs = true },
		func() { f.noMem = true },
		func() { f.serversLabel = true },
		func() { f.usageLabels = true },
		func() { f.folderBudget = -2 }, // shorten to whatever is left
		func() { f.folderBudget = 0 },
		func() { f.branchBudget = -2 },
		func() { f.noCounts = true },
		func() { f.compactPills = true },
		func() { f.noMeters, f.usageLabels, f.tight, f.aggregate = true, false, true, true },
		func() { f.noServers = true },
		func() { f.noBranch = true },
		func() { f.usageDrop = 1 },
		func() { f.usageDrop = 2 },
		func() { f.usageDrop = 3 },
		func() { f.noUpdate = true },
		func() { f.noAgents = true },
	}
	l := statusPlace(m, width, measure, d, f)
	for _, step := range steps {
		if l.fits {
			break
		}
		step()
		l = statusPlace(m, width, measure, d, f)
	}
	return l.statusLayout
}

type placed struct {
	statusLayout
	fits bool
}

// cutLeft keeps the end of s within w px, with a leading ellipsis.
func cutLeft(s string, w int, measure func(string, bool) int) string {
	r := []rune(s)
	for i := 0; i < len(r); i++ {
		t := "…" + string(r[i:])
		if i == 0 {
			t = s
		}
		if measure(t, true) <= w {
			return t
		}
	}
	return ""
}

// cutRight keeps the start of s within w px, with a trailing ellipsis.
func cutRight(s string, w int, measure func(string, bool) int) string {
	if measure(s, true) <= w {
		return s
	}
	r := []rune(s)
	for n := len(r) - 1; n > 0; n-- {
		if t := string(r[:n]) + "…"; measure(t, true) <= w {
			return t
		}
	}
	return ""
}

// statusPlace lays out one compaction state and reports whether it fits.
func statusPlace(m statusModel, width int, measure func(s string, mono bool) int, d statusDims, f statusFit) placed {
	var p placed
	p.Groups = map[statusGroup]image.Rectangle{}
	ip, gap := d.ItemPad, d.Gap
	if f.tight {
		ip, gap = d.ItemPad/2, d.Gap/2
	}
	p.Tight = f.tight
	p.Compact.ServersLabel, p.Compact.UsageLabels, p.Compact.UsageMeters = f.serversLabel || f.aggregate, f.usageLabels, f.noMeters
	p.Compact.Agents, p.Compact.Update, p.Compact.AggregateServers = f.compactPills, f.compactPills, f.aggregate
	p.Compact.BranchCounts = f.noCounts

	// Right-hand groups, widths only.
	type item struct {
		g   statusGroup
		w   int
		sep bool // a hairline before it
	}
	var right []item
	if m.Update != "" && !f.noUpdate {
		w := 2*d.PillPad + d.Icon
		if !f.compactPills {
			w += gap/2 + measure("Update "+m.Update, false)
		}
		right = append(right, item{grpUpdate, 2*ip + w, false})
	}
	if m.Agents > 0 && !f.noAgents {
		right = append(right, item{grpAgents, 2*ip + 2*d.PillPad + d.Dot + gap/2 + measure(agentsPill(m, f.compactPills), false), true})
	}
	if len(m.Servers) > 0 && !f.noServers {
		n := len(m.Servers)
		if f.aggregate {
			n = 1
		}
		w := n*d.Dot + (n-1)*(d.Dot*2/3)
		if !f.serversLabel && !f.aggregate {
			w += gap + measure(serversCount(m.Servers), true) + gap + measure("SERVERS", false)
		}
		right = append(right, item{grpServers, 2*ip + w, true})
	}
	keys := []string{}
	if n := len(m.Usage) - f.usageDrop; n > 0 {
		w := 0
		for i, u := range m.Usage[:n] {
			keys = append(keys, u.Key)
			if i > 0 {
				w += gap
			}
			if !f.usageLabels {
				w += measure(strings.ToUpper(u.Key), false) + gap/2
			}
			if !f.noMeters {
				w += d.Meter + gap/2
			}
			w += measure(strconv.Itoa(u.Pct)+"%", true)
		}
		right = append(right, item{grpUsage, 2*ip + w, true})
	}
	p.UsageKeys = keys
	if !f.noMem {
		right = append(right, item{grpMem, 2*ip + d.Icon + gap + measure(strconv.Itoa(m.MemMB)+" MB", true), true})
	}
	if !f.noTabs {
		right = append(right, item{grpTabs, 2*ip + d.Icon + gap + measure(strconv.Itoa(m.Tabs)+" tabs", true), f.noMem})
	}
	// Separators only between visible neighbours, and none when tight.
	if len(right) > 0 {
		right[0].sep = false
	}
	rightW := 0
	for _, it := range right {
		rightW += it.w
		if it.sep && !f.tight {
			rightW += d.Sep
		}
	}
	// The notice is reserved before the readouts (spec 6.4 step 1).
	noticeW := 0
	if m.Notice != "" {
		budget := width / 3
		p.NoticeText = cutRight(m.Notice, budget-2*ip, measure)
		if p.NoticeText != "" {
			noticeW = 2*ip + measure(p.NoticeText, true)
		}
	}

	// Left-hand groups.
	branchCore := func(text string) int { return 2*ip + d.Icon + gap + measure(text, true) }
	countsW := 0
	if m.Branch != "" && !f.noCounts && (m.Add > 0 || m.Del > 0) {
		if m.Add > 0 {
			countsW += gap + measure("+"+strconv.Itoa(m.Add), true)
		}
		if m.Del > 0 {
			countsW += gap/2 + measure("−"+strconv.Itoa(m.Del), true)
		}
	}
	avail := width - 2*d.Pad - rightW - noticeW
	folder, branch := m.Folder, m.Branch
	if f.noBranch {
		branch = ""
	}
	folderW := func(s string) int {
		if s == "" {
			return 0
		}
		return 2*ip + d.Icon + gap + measure(s, true)
	}
	branchW := 0
	if branch != "" {
		branchW = branchCore(branch) + countsW
	}
	switch f.folderBudget {
	case 0:
		folder = ""
	case -2:
		room := avail - branchW - 2*ip - d.Icon - gap
		folder = cutLeft(folder, room, measure)
	}
	if branch != "" && f.branchBudget == -2 {
		room := avail - folderW(folder) - 2*ip - d.Icon - gap - countsW
		branch = cutRight(branch, room, measure)
		if branch == "" {
			branchW = 0
		} else {
			branchW = branchCore(branch) + countsW
		}
	}
	if f.noCounts && branch != "" {
		branchW = branchCore(branch)
	}
	p.FolderText, p.BranchText = folder, branch
	leftW := folderW(folder) + branchW
	p.fits = leftW+rightW+noticeW+2*d.Pad <= width

	// Place: folder and branch from the left, the rest from the right.
	x := d.Pad
	if folder != "" {
		p.Groups[grpFolder] = image.Rect(x, 0, x+folderW(folder), d.H)
		x += folderW(folder)
	}
	if branch != "" {
		p.Groups[grpBranch] = image.Rect(x, 0, x+branchW, d.H)
		x += branchW
	}
	rx := width - d.Pad
	for i := len(right) - 1; i >= 0; i-- {
		it := right[i]
		p.Groups[it.g] = image.Rect(rx-it.w, 0, rx, d.H)
		rx -= it.w
		if it.sep && !f.tight {
			p.Seps = append([]int{rx - d.Sep/2}, p.Seps...)
			rx -= d.Sep
		}
	}
	if p.NoticeText != "" {
		p.Groups[grpNotice] = image.Rect(rx-noticeW, 0, rx, d.H)
	}
	if !p.fits {
		// Never draw or hit outside the bar, even mid-search.
		for g, r := range p.Groups {
			r = r.Intersect(image.Rect(0, 0, width, d.H))
			if r.Empty() {
				delete(p.Groups, g)
			} else {
				p.Groups[g] = r
			}
		}
	}
	return p
}
