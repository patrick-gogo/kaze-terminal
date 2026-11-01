package ui

import (
	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/resources"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
	"image"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestResourcesDoNotPollOrOverlapAndPause(t *testing.T) {
	var scans, wakes atomic.Int32
	gate := make(chan struct{})
	w := newResourceWork(func(now time.Time, _ []resources.Root, _ bool) resources.Snapshot {
		scans.Add(1)
		<-gate
		return resources.Snapshot{At: now}
	}, func() { wakes.Add(1) })
	defer w.close()
	now := time.Unix(100, 0)
	w.tick(now, nil)
	deadline := time.Now().Add(time.Second)
	for scans.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	w.tick(now.Add(3*time.Second), nil)
	if scans.Load() != 1 {
		t.Fatal("overlapping scan")
	}
	w.pause(true)
	close(gate)
	deadline = time.Now().Add(time.Second)
	for len(w.in) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	w.tick(now.Add(4*time.Second), nil)
	if wakes.Load() != 0 || scans.Load() != 1 {
		t.Fatal("minimized activity")
	}
	w.pause(false)
	w.tick(now.Add(5*time.Second), nil)
	deadline = time.Now().Add(time.Second)
	for len(w.in) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	w.tick(now.Add(5*time.Second), nil)
	if scans.Load() != 2 {
		t.Fatal("resume did not sample")
	}
	time.Sleep(10 * time.Millisecond)
	if scans.Load() != 2 {
		t.Fatal("idle monitor polled")
	}
}
func TestResourceStatusFitsAndCompacts(t *testing.T) {
	m := busyModel()
	m.Resources = resources.Snapshot{At: time.Now(), Cores: 8, Total: resources.Value{CPU: 4, CPUValid: true, Bytes: 1800 << 20, RAMValid: true}}
	for _, width := range []int{1600, 900, 600, 320, 180} {
		l := statusAt(m, width)
		for g, r := range l.Groups {
			if r.Min.X < 0 || r.Max.X > width {
				t.Fatalf("group %d outside", g)
			}
			for other, q := range l.Groups {
				if other != g && r.Overlaps(q) {
					t.Fatalf("%d overlaps %d at %d", g, other, width)
				}
			}
		}
	}
	l := statusAt(m, 1600)
	if !shown(l, grpMem) || l.Groups[grpMem].Min.X >= l.Groups[grpUsage].Min.X {
		t.Fatal("resources must precede Claude usage")
	}
}

func TestResourceCardIsContainedAndScrollable(t *testing.T) {
	for _, size := range [][2]int{{1600, 700}, {600, 500}, {320, 260}} {
		win := busyWindow(t)
		for i := 0; i < 30; i++ {
			win.stat.m.Resources.Rows = append(win.stat.m.Resources.Rows, resources.Row{ID: i, Name: "A long session label that must fit", Value: resources.Value{CPU: 2, CPUValid: true, Bytes: 152 << 20, RAMValid: true}})
		}
		win.stat.y = size[1] - 32
		win.stat.l.Groups[grpMem] = image.Rect(size[0]-200, 0, size[0]-10, 32)
		win.stat.hover = grpMem
		win.drawResourceCard(pageContext(size[0], size[1]))
		cs := win.stat.card
		if cs.box.Empty() || !cs.box.In(image.Rect(0, 0, size[0], size[1]-32)) || cs.maxScroll == 0 || cs.viewport.Empty() {
			t.Fatalf("%v: card geometry %+v", size, cs)
		}
	}
}

func TestResourceStaleResultsCloseAndCadence(t *testing.T) {
	var calls atomic.Int32
	done := make(chan struct{})
	w := newResourceWork(func(now time.Time, _ []resources.Root, _ bool) resources.Snapshot {
		calls.Add(1)
		<-done
		return resources.Snapshot{At: now}
	}, nil)
	now := time.Unix(100, 0)
	roots := []resources.Root{{PID: 1, Owner: 1, Name: "old"}}
	w.tick(now, roots)
	close(done)
	until := time.Now().Add(time.Second)
	for len(w.in) == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	newRoots := []resources.Root{{PID: 2, Owner: 2, Name: "new"}}
	w.tick(now, newRoots)
	if !w.snapshot.At.IsZero() {
		t.Fatal("stale result applied")
	}
	w.tick(now, newRoots)
	until = time.Now().Add(time.Second)
	for len(w.in) == 0 && time.Now().Before(until) {
		time.Sleep(time.Millisecond)
	}
	w.tick(now, newRoots)
	w.tick(now.Add(time.Second), newRoots)
	if calls.Load() != 2 {
		t.Fatal("cadence bypass")
	}
	w.close()
	w.tick(now.Add(4*time.Second), newRoots)
	if calls.Load() != 2 {
		t.Fatal("closed monitor scanned")
	}
}
func TestResourceFormattingDoesNotInventZero(t *testing.T) {
	a, b := resourceValues(resources.Value{}, false)
	if a != "n/a" || b != "n/a" {
		t.Fatal("unknown reading shown as zero")
	}
	a, b = resourceValues(resources.Value{CPUValid: true, RAMValid: true, Bytes: 1536 << 20, MissingCPU: 1}, true)
	if a != "0%*" || b != "1.5 GB*" {
		t.Fatal(a, b)
	}
	if clickable(grpMem) {
		t.Fatal("resource hover should not promise a click action")
	}
}

func TestResourceHoverCardPointerLifecycle(t *testing.T) {
	win := busyWindow(t)
	win.stat.hover = grpMem
	var router input.Router
	draw := func() {
		gtx := pageContext(1600, 700)
		gtx.Source = router.Source()
		win.statusEvents(gtx)
		win.drawResourceCard(gtx)
		router.Frame(gtx.Ops)
	}
	draw()
	card := win.stat.card.box
	at := center(card)
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: at})
	draw()
	if !win.stat.card.hovered {
		t.Fatal("moving into card closed it")
	}
	router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: f32.Pt(2, 2)})
	draw()
	if win.stat.card.hovered || !win.stat.card.box.Empty() {
		t.Fatal("card stayed after leaving")
	}
}

type resourceRootPTY struct {
	ptyx.Session
	pid int
}

func (p *resourceRootPTY) RootPID() int { return p.pid }
func TestResourceRootsKeepScratchAndWorkspaceOwnership(t *testing.T) {
	st := &recStarter{}
	pid := 100
	win := &window{defTitle: "zsh", names: map[tabs.TabID]string{}}
	win.mgr = tabs.NewManager(func(env []string, dir string) (ptyx.Session, error) {
		p, err := st.start(env, dir)
		pid++
		return &resourceRootPTY{p, pid}, err
	}, 80, 24, nil)
	defer win.mgr.CloseAll()
	a, err := win.mgr.NewOpts(tabs.StartOpts{Title: "Claude"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := win.mgr.NewOpts(tabs.StartOpts{Title: "API", Group: &tabs.Group{Name: "Project", Run: 1}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = win.mgr.NewOpts(tabs.StartOpts{Parent: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	roots := win.resourceRoots()
	if len(roots) != 3 || roots[0].Owner != int(a.ID) || roots[1].Owner != int(b.ID) || roots[2].Owner != int(a.ID) || roots[1].Name != "Project / API" {
		t.Fatalf("roots %+v", roots)
	}
	w := newResourceWork(nil, nil)
	win.resources = w
	win.onConfig(app.ConfigEvent{Config: app.Config{Mode: app.Minimized}})
	if !w.paused.Load() {
		t.Fatal("minimized configuration did not pause")
	}
	win.onConfig(app.ConfigEvent{Config: app.Config{Mode: app.Windowed}})
	if w.paused.Load() || !w.reset {
		t.Fatal("resume did not reset")
	}
}

// Observe longer than the gate: no timer or redraw request can sustain sampling.
func TestResourceIdleAndMinimizedHaveNoRecurringWork(t *testing.T) {
	if os.Getenv("KAZE_RESOURCE_BENCH") != "1" {
		t.Skip("opt-in idle observation")
	}
	for _, minimized := range []bool{false, true} {
		var scans, wakes atomic.Int32
		w := newResourceWork(func(now time.Time, _ []resources.Root, _ bool) resources.Snapshot {
			scans.Add(1)
			return resources.Snapshot{At: now}
		}, func() { wakes.Add(1) })
		w.tick(time.Now(), nil)
		deadline := time.Now().Add(time.Second)
		for len(w.in) == 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		w.tick(time.Now(), nil)
		w.pause(minimized)
		before, notify := scans.Load(), wakes.Load()
		time.Sleep(2100 * time.Millisecond)
		if scans.Load() != before || wakes.Load() != notify {
			t.Fatal("idle monitor generated work")
		}
		if minimized {
			w.tick(time.Now(), nil)
			if scans.Load() != before {
				t.Fatal("minimized frame started work")
			}
		}
		w.close()
	}
}

func TestResourceCardTextContrast(t *testing.T) {
	for _, th := range theme.All() {
		c := newChrome()
		c.setTheme(th)
		if contrast(c.resourceMuted(), c.th.ToastBG) < 4.5 {
			t.Fatalf("%s: resource card detail contrast", th.Key)
		}
	}
}

func TestReviewSpinnerRenameDoesNotDiscardScan(t *testing.T) {
	var resets atomic.Int32
	w := newResourceWork(func(now time.Time, roots []resources.Root, reset bool) resources.Snapshot {
		if reset {
			resets.Add(1)
		}
		return resources.Snapshot{At: now, Rows: []resources.Row{{ID: roots[0].Owner, Name: roots[0].Name}}, Total: resources.Value{CPU: 12, CPUValid: true}}
	}, nil)
	defer w.close()
	now := time.Unix(100, 0)
	old := []resources.Root{{PID: 1, Owner: 10, Name: "thinking"}}
	changed := []resources.Root{{PID: 1, Owner: 10, Name: "thinking ⠹"}}
	w.tick(now, old)
	deadline := time.Now().Add(time.Second)
	for len(w.in) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	w.tick(now, changed)
	if !w.snapshot.Total.CPUValid || w.snapshot.Rows[0].Name != changed[0].Name {
		t.Fatalf("rename discarded CPU or kept stale label: %+v", w.snapshot)
	}
	if w.reset || resets.Load() != 1 {
		t.Fatal("rename reset baselines")
	}
}

func TestReviewTopologyChangeDoesNotResetUnchangedProcesses(t *testing.T) {
	var calls, resets atomic.Int32
	w := newResourceWork(func(now time.Time, _ []resources.Root, reset bool) resources.Snapshot {
		calls.Add(1)
		if reset {
			resets.Add(1)
		}
		return resources.Snapshot{At: now}
	}, nil)
	defer w.close()
	now := time.Unix(100, 0)
	roots := []resources.Root{{PID: 1, Owner: 10}}
	w.tick(now, roots)
	deadline := time.Now().Add(time.Second)
	for len(w.in) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	roots = append(roots, resources.Root{PID: 2, Owner: 20})
	w.tick(now, roots)
	deadline = time.Now().Add(time.Second)
	for len(w.in) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	w.tick(now, roots)
	if calls.Load() != 2 || resets.Load() != 1 {
		t.Fatalf("topology reset all baselines: calls=%d resets=%d", calls.Load(), resets.Load())
	}
}

func TestReviewUndrawableCardClearsHoverGeometry(t *testing.T) {
	for _, size := range [][2]int{{160, 500}, {800, 50}} {
		win := busyWindow(t)
		win.stat.hover = grpMem
		win.stat.card.hovered = true
		win.stat.card.box = image.Rect(20, 20, 200, 200)
		win.stat.card.bridge = image.Rect(20, 200, 200, 568)
		win.stat.card.viewport = win.stat.card.box
		win.stat.y = size[1] - 32
		win.drawResourceCard(pageContext(size[0], size[1]))
		cs := win.stat.card
		if cs.hovered || !cs.box.Empty() || !cs.bridge.Empty() || !cs.viewport.Empty() || win.stat.hover == grpMem {
			t.Fatalf("undrawable card kept hover: %+v", cs)
		}
	}
}

func TestReviewProcessIdentityIgnoresLabelAndOrder(t *testing.T) {
	a := []resources.Root{{PID: 1, Owner: 10, Name: "first"}, {PID: 2, Owner: 20, Name: "second"}}
	b := []resources.Root{{PID: 2, Owner: 20, Name: "changed"}, {PID: 1, Owner: 10, Name: "spinner"}}
	if !sameResourceProcesses(a, b) {
		t.Fatal("presentation changed process identity")
	}
	b[0].Owner = 30
	if sameResourceProcesses(a, b) {
		t.Fatal("owner change went unnoticed")
	}
	b[0].Owner = 20
	b[0].PID = 3
	if sameResourceProcesses(a, b) {
		t.Fatal("PID change went unnoticed")
	}
}
