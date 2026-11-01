package ui

import (
	"image"
	"image/color"
	"math"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/resources"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

var statusTestDims = statusDims{H: 32, Pad: 6, ItemPad: 9, Sep: 9, Icon: 13, Gap: 6, Meter: 38, PillPad: 7, Dot: 6}

func seven(s string, mono bool) int { return 7 * utf8.RuneCountInString(s) }

// busyModel is the mockup's busy state.
func busyModel() statusModel {
	return statusModel{
		Folder: "~/projects/todo-api", Branch: "feature/auth", Add: 12, Del: 3, Update: "0.15",
		Agents: 4, Waiting: 1, Servers: []serverDot{{Up: true}, {Up: true}, {Down: true}},
		Usage:     []usage.Detail{{Key: "5h", Pct: 84, Level: usage.Warn}, {Key: "wk", Pct: 93, Level: usage.Warn}, {Key: "ctx", Pct: 71}},
		Resources: resources.Snapshot{Cores: 8, Total: resources.Value{CPU: 4, CPUValid: true, Bytes: 152 << 20, RAMValid: true}}, Tabs: 7,
	}
}

func statusAt(m statusModel, w int) statusLayout { return layoutStatusBar(m, w, seven, statusTestDims) }

func shown(l statusLayout, g statusGroup) bool { _, ok := l.Groups[g]; return ok }

func TestStatusOrderAndSeparators(t *testing.T) {
	l := statusAt(busyModel(), 1600)
	order := []statusGroup{grpFolder, grpBranch, grpUpdate, grpAgents, grpServers, grpMem, grpUsage, grpTabs}
	prev := -1
	for _, g := range order {
		r, ok := l.Groups[g]
		if !ok {
			t.Fatalf("group %d missing at 1600px", g)
		}
		if r.Min.X < prev {
			t.Fatalf("group %d at %v is out of order", g, r)
		}
		prev = r.Max.X
	}
	for _, x := range l.Seps {
		for _, r := range l.Groups {
			if x > r.Min.X && x < r.Max.X {
				t.Fatalf("separator at %d inside %v", x, r)
			}
		}
	}
	if len(l.Seps) != 4 { // update, agents, servers, usage, then memory and tabs together
		t.Fatalf("%d separators", len(l.Seps))
	}
}

func TestStatusDropOrder(t *testing.T) {
	m := busyModel()
	var stages []string
	note := func(s string) {
		if len(stages) == 0 || stages[len(stages)-1] != s {
			stages = append(stages, s)
		}
	}
	for w := 1400; w >= 600; w -= 5 {
		l := statusAt(m, w)
		switch {
		case shown(l, grpTabs):
			note("full")
		case !l.Compact.ResourcesLabel:
			note("no tabs")
		case !l.Compact.ResourcesMeter:
			note("no resource label")
		case !l.Compact.ServersLabel:
			note("no resource meter")
		case !l.Compact.UsageLabels:
			note("no servers label")
		default:
			note("no usage labels")
		}
	}
	want := []string{"full", "no tabs", "no resource label", "no resource meter", "no servers label", "no usage labels"}
	for i, s := range want {
		if i >= len(stages) || stages[i] != s {
			t.Fatalf("stages %v, want them to start %v", stages, want)
		}
	}
	l := statusAt(m, 600)
	if !shown(l, grpBranch) || !shown(l, grpAgents) || !shown(l, grpUpdate) || len(l.UsageKeys) != 3 {
		t.Fatalf("600px keeps branch, agents, update and all usage: %+v", l)
	}
}

func TestStatusLongTextFits(t *testing.T) {
	m := busyModel()
	m.Branch = strings.Repeat("b", 120)
	m.Folder = "~/" + strings.Repeat("f", 90)
	m.Update = "10.123.4567"
	m.Agents, m.Waiting, m.Add, m.Del = 9999, 999, 123456, 98765
	for _, w := range []int{600, 320, 900, 1200} {
		l := statusAt(m, w)
		var rs []image.Rectangle
		for _, r := range l.Groups {
			if r.Min.X < 0 || r.Max.X > w {
				t.Fatalf("%dpx: %v outside the bar", w, r)
			}
			for _, o := range rs {
				if r.Overlaps(o) {
					t.Fatalf("%dpx: %v overlaps %v", w, r, o)
				}
			}
			rs = append(rs, r)
		}
	}
}

func TestStatusNoticeSurvivesCompaction(t *testing.T) {
	m := busyModel()
	m.Notice = "new tab failed: " + strings.Repeat("x", 80)
	l := statusAt(m, 700)
	if !shown(l, grpNotice) {
		t.Fatalf("notice must survive compaction: %+v", l)
	}
	if l.NoticeText == "" || utf8.RuneCountInString(l.NoticeText) >= utf8.RuneCountInString(m.Notice) {
		t.Fatalf("notice not shortened: %q", l.NoticeText)
	}
}

func TestStatusHiddenGroupsHaveNoHits(t *testing.T) {
	l := statusAt(busyModel(), 320)
	for g, r := range l.Groups {
		if r.Empty() {
			t.Fatalf("group %d listed with an empty rect", g)
		}
	}
	if len(l.Seps) > max(len(l.Groups)-1, 0) {
		t.Fatalf("%d separators for %d groups", len(l.Seps), len(l.Groups))
	}
}

func TestStatusHidesEmptyGroups(t *testing.T) {
	m := statusModel{Folder: "~/x", Branch: "main", Tabs: 1}
	l := statusAt(m, 1200)
	var got []statusGroup
	for g := range l.Groups {
		got = append(got, g)
	}
	slices.Sort(got)
	if !slices.Equal(got, []statusGroup{grpFolder, grpBranch, grpMem, grpTabs}) {
		t.Fatalf("groups %v", got)
	}
}

func TestStatusAgentsPill(t *testing.T) {
	m := busyModel()
	if s := agentsPill(m, false); s != "4 agents · 1 waiting" {
		t.Fatalf("%q", s)
	}
	m.Waiting = 0
	if s := agentsPill(m, false); s != "4 agents" {
		t.Fatalf("%q", s)
	}
	m.Agents = 1
	if s := agentsPill(m, false); s != "1 agent" {
		t.Fatalf("%q", s)
	}
	if s := agentsPill(m, true); s != "1" {
		t.Fatalf("compact %q", s)
	}
}

// busyWindow draws a frame with an update out, agents and usage.
func busyWindow(t *testing.T) *window {
	t.Helper()
	win, _ := sideWindow(t, true)
	openIn(t, win, osPath("/code/alpha/a"))
	win.upd.newer, win.upd.rel.Version, win.opt.Config.CheckUpdates = true, "0.15", true
	pct := 40.0
	win.acct.FiveHour = &notify.Window{Pct: pct}
	win.liveMgr.Store(win.mgr)
	win.onNotify(notify.Msg{Tab: int(win.mgr.Tabs()[0].ID), Kind: notify.KindAgentStart, Session: "s", AgentID: "a", AgentType: "X"})
	win.drainSubs(t0)
	var r input.Router
	sizedFrame(t, win, &r, 1600, 600)
	return win
}

func TestStatusClicks(t *testing.T) {
	win := busyWindow(t)
	press := func(g statusGroup) {
		r := win.statusRect(g)
		if r.Empty() {
			t.Fatalf("group %d not drawn", g)
		}
		win.statusPointer(pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2))})
	}
	press(grpAgents)
	if !win.board.open {
		t.Fatal("agents did not open the board")
	}
	win.closeBoard()
	press(grpUsage)
	if !win.set.shown || win.sections()[win.set.section].title != "Agents" {
		t.Fatal("usage did not open Settings, Agents")
	}
	win.closeSettings()
	press(grpFolder)
	if win.set.shown || win.board.open {
		t.Fatal("the folder is not clickable")
	}
	press(grpUpdate)
	if !win.set.shown {
		t.Fatal("update did not open Settings")
	}
}

func TestUsageCardOnHoverOnly(t *testing.T) {
	win := busyWindow(t)
	r := win.statusRect(grpUsage)
	at := f32.Pt(float32(r.Min.X+3), float32(r.Min.Y+r.Dy()/2))
	win.statusPointer(pointer.Event{Kind: pointer.Move, Position: at})
	if win.stat.hover != grpUsage {
		t.Fatalf("hover %d", win.stat.hover)
	}
	gtx := pageContext(1600, 600)
	win.drawUsageCard(gtx) // draws; a frame with the card must not panic
	win.statusPointer(pointer.Event{Kind: pointer.Leave, Position: at})
	if win.stat.hover != grpNone {
		t.Fatal("the card stayed after the pointer left")
	}
}

func TestStatusInEveryTheme(t *testing.T) {
	for _, th := range theme.All() {
		win := busyWindow(t)
		win.applyTheme(th)
		var r input.Router
		sizedFrame(t, win, &r, 900, 600)
		if win.statusRect(grpAgents).Empty() {
			t.Fatalf("%s: agents not drawn", th.Key)
		}
	}
}

// contrast is the WCAG contrast ratio of two colours.
func contrast(a, b color.NRGBA) float64 {
	lum := func(c color.NRGBA) float64 {
		ch := func(v uint8) float64 {
			x := float64(v) / 255
			if x <= 0.03928 {
				return x / 12.92
			}
			return math.Pow((x+0.055)/1.055, 2.4)
		}
		return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
	}
	l1, l2 := lum(a)+0.05, lum(b)+0.05
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return l1 / l2
}

func TestStatusTextReadableInEveryTheme(t *testing.T) {
	for _, th := range theme.All() {
		c := newChrome()
		c.setTheme(th)
		if r := contrast(c.statusLabelColor(), th.UI.Chrome); r < 3.5 {
			t.Errorf("%s: labels %.2f:1 on the bar", th.Key, r)
		}
		if r := contrast(c.statusValueColor(), th.UI.Chrome); r < 4.5 {
			t.Errorf("%s: values %.2f:1 on the bar", th.Key, r)
		}
	}
}
