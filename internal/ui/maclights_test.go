package ui

import (
	"image"
	"sync"
	"testing"
)

// fakeLights records the native calls and the generation each one ran for.
type fakeLights struct {
	mu    sync.Mutex
	calls []string
}

func (f *fakeLights) add(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fakeLights) attach(view uintptr, gen uint64, barH, left float64) {
	f.add("attach")
}
func (f *fakeLights) place(view uintptr, gen uint64, barH, left float64) { f.add("place") }
func (f *fakeLights) detach(view uintptr, gen uint64)                    { f.add("detach") }
func (f *fakeLights) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// nativeQueue runs native work later, as Window.Run would on the main thread.
type nativeQueue struct{ fs []func() }

func (q *nativeQueue) run(f func()) { q.fs = append(q.fs, f) }
func (q *nativeQueue) drain() {
	for len(q.fs) > 0 {
		f := q.fs[0]
		q.fs = q.fs[1:]
		f()
	}
}

func newBridge(nat lightsNative, run func(func())) *macBridge {
	return &macBridge{nat: nat, run: run, barH: 42, left: 14, invalidate: func() {}}
}

func TestBridgeAttachDispatchesThroughRunner(t *testing.T) {
	f, q := &fakeLights{}, &nativeQueue{}
	b := newBridge(f, q.run)
	b.viewEvent(7)
	if len(f.got()) != 0 {
		t.Fatal("native work ran outside the runner")
	}
	q.drain()
	if got := f.got(); len(got) != 1 || got[0] != "attach" {
		t.Fatalf("calls %v", got)
	}
}

func TestBridgeDuplicateAttachIsANoop(t *testing.T) {
	f, q := &fakeLights{}, &nativeQueue{}
	b := newBridge(f, q.run)
	e := uintptr(7)
	b.viewEvent(e)
	b.viewEvent(e)
	q.drain()
	if got := f.got(); len(got) != 1 {
		t.Fatalf("a repeated event attached twice: %v", got)
	}
}

func TestBridgeInvalidAndReplacedViewsDetach(t *testing.T) {
	f, q := &fakeLights{}, &nativeQueue{}
	b := newBridge(f, q.run)
	for _, e := range []uintptr{7, 9, 0} {
		b.viewEvent(e)
		q.drain() // the main thread runs each event's work before the next
	}
	want := []string{"attach", "detach", "attach", "detach"}
	if got := f.got(); len(got) != len(want) {
		t.Fatalf("calls %v want %v", got, want)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("calls %v want %v", got, want)
			}
		}
	}
	if b.Inset() != 0 {
		t.Fatal("inset kept after detach")
	}
}

func TestBridgeStaleQueuedWorkIsIgnored(t *testing.T) {
	f, q := &fakeLights{}, &nativeQueue{}
	b := newBridge(f, q.run)
	b.viewEvent(7)
	b.reapply() // queued for generation 1
	b.viewEvent(0)
	q.drain()
	for _, c := range f.got() {
		if c == "place" {
			t.Fatal("a reapply queued for a detached view still ran")
		}
	}
}

func TestBridgePublishesOnlyCurrentGeneration(t *testing.T) {
	q := &nativeQueue{}
	b := newBridge(&fakeLights{}, q.run)
	b.viewEvent(7)
	q.drain()
	gen := b.generation()
	b.publish(gen, 76)
	if b.Inset() != 76 {
		t.Fatalf("inset %v", b.Inset())
	}
	b.publish(gen-1, 10)
	if b.Inset() != 76 {
		t.Fatal("a stale generation changed the inset")
	}
}

func TestBridgeCloseDetachesEvenWithoutRunner(t *testing.T) {
	f := &fakeLights{}
	b := newBridge(f, func(func()) {}) // the runner no longer runs anything
	b.viewEvent(7)
	b.close()
	if b.generation() == 0 || b.attachedView() != 0 {
		t.Fatal("close left an attachment")
	}
	b.publish(b.generation(), 50)
	if b.Inset() != 0 {
		t.Fatal("a callback after close published")
	}
}

func TestBridgeInsetRace(t *testing.T) {
	b := newBridge(&fakeLights{}, func(f func()) { f() })
	b.viewEvent(7)
	gen := b.generation()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				b.publish(gen, float64(j))
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = b.Inset()
			}
		}()
	}
	wg.Wait()
}

func TestNoBridgeIsSafe(t *testing.T) {
	var b *macBridge
	b.viewEvent(7)
	b.reapply()
	b.close()
	if b.Inset() != 0 {
		t.Fatal("nil bridge inset")
	}
}

func TestBridgeDragAtUsesPoints(t *testing.T) {
	b := newBridge(&fakeLights{}, func(f func()) { f() })
	b.setDrag([]image.Rectangle{image.Rect(200, 0, 600, 84)}, 2) // px at 2x
	if !b.dragAt(150, 20) || b.dragAt(90, 20) || b.dragAt(150, 50) {
		t.Fatal("drag hit in points")
	}
}
