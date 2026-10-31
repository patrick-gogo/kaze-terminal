package tabs_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func TestParentTabIsNotMadeActive(t *testing.T) {
	m, _ := newManager(t)
	a := mustNew(t, m)
	s, err := m.NewOpts(tabs.StartOpts{Parent: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Active(); got == nil || got.ID != a.ID {
		t.Fatalf("active %v, want the session %d", got, a.ID)
	}
	if s.Parent != a.ID || a.Parent != 0 {
		t.Fatalf("parents %d %d", s.Parent, a.Parent)
	}
}

func TestTabKeepsItsStartFolder(t *testing.T) {
	m, _ := newManager(t)
	d, err := m.NewOpts(tabs.StartOpts{Dir: `C:\x`})
	if err != nil {
		t.Fatal(err)
	}
	if d.Dir != `C:\x` || mustNew(t, m).Dir != "" {
		t.Fatalf("dir %q", d.Dir)
	}
}

func visibleFixture(t *testing.T) (*tabs.Manager, *starter, *tabs.Tab, *atomic.Int64) {
	st := &starter{}
	var frames atomic.Int64
	m := tabs.NewManager(st.start, 80, 24, func() { frames.Add(1) })
	t.Cleanup(st.closeAll)
	bg := mustNew(t, m)
	mustNew(t, m) // active
	return m, st, bg, &frames
}

func TestVisibleBackgroundTabRequestsFrames(t *testing.T) {
	m, st, bg, frames := visibleFixture(t)
	m.SetVisible(bg.ID, true)
	st.fakes[0].outW.Write([]byte("x"))
	waitFor(t, "a frame for the visible tab", func() bool { return frames.Load() > 0 })
	m.SetVisible(bg.ID, false)
	time.Sleep(20 * time.Millisecond)
	before := frames.Load()
	st.fakes[0].outW.Write([]byte("more output\r\n"))
	time.Sleep(200 * time.Millisecond)
	if got := frames.Load() - before; got != 0 {
		t.Fatalf("hidden tab asked for %d frames", got)
	}
	m.SetVisible(9999, true) // unknown tabs are ignored
}

func TestWatchOffKeepsVisible(t *testing.T) {
	m, st, bg, frames := visibleFixture(t)
	m.SetVisible(bg.ID, true)
	m.Watch(bg.ID, true)
	m.Watch(bg.ID, false)
	st.fakes[0].outW.Write([]byte("x"))
	waitFor(t, "a frame for the visible tab", func() bool { return frames.Load() > 0 })
}

func TestResizeTabIsSkippedByResizeAll(t *testing.T) {
	m, st := newManager(t)
	mustNew(t, m)
	b := mustNew(t, m)
	m.ResizeTab(b.ID, 40, 10)
	m.ResizeAll(100, 30)
	if got := st.fakes[1].lastSize(); got != [2]int{40, 10} {
		t.Fatalf("pane pty %v, want [40 10]", got)
	}
	if got := st.fakes[0].lastSize(); got != [2]int{100, 30} {
		t.Fatalf("session pty %v, want [100 30]", got)
	}
	n := sizeCount(st.fakes[1])
	m.ResizeTab(b.ID, 40, 10) // unchanged: nothing sent
	if sizeCount(st.fakes[1]) != n {
		t.Fatal("an unchanged ResizeTab resized the pty")
	}
}

func TestDockReturnsToTheSharedSize(t *testing.T) {
	m, st := newManager(t)
	a := mustNew(t, m)
	b := mustNew(t, m)
	m.ResizeTab(b.ID, 40, 10)
	m.ResizeAll(100, 30)
	m.Dock(b.ID)
	if got := st.fakes[1].lastSize(); got != [2]int{100, 30} {
		t.Fatalf("docked pty %v, want [100 30]", got)
	}
	n := sizeCount(st.fakes[0])
	m.Dock(a.ID) // never sized on its own
	m.Dock(b.ID) // already docked
	if sizeCount(st.fakes[0]) != n || st.fakes[1].lastSize() != [2]int{100, 30} {
		t.Fatal("docking an unsized tab resized it")
	}
	m.ResizeAll(90, 20)
	if got := st.fakes[1].lastSize(); got != [2]int{90, 20} {
		t.Fatalf("docked tab missed ResizeAll: %v", got)
	}
}

func TestResizeTabUnknownIsIgnored(t *testing.T) {
	m, _ := newManager(t)
	m.ResizeTab(999, 1, 1)
	m.Dock(999)
}

func sizeCount(f *fakePTY) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sizes)
}
