package ui

import (
	"image"
	"sync"
)

// lightsNative is the macOS side of the traffic lights. Every method runs on
// the native main thread (through the bridge's runner), never on Kaze's UI
// goroutine.
type lightsNative interface {
	// attach shows the window's standard buttons, places them and starts the
	// resize, full screen and close observers for gen.
	attach(view uintptr, gen uint64, barH, left float64)
	// place puts the buttons back, after something (Gio's Configure) hid or moved them.
	place(view uintptr, gen uint64, barH, left float64)
	// detach removes gen's observers.
	detach(view uintptr, gen uint64)
}

// macBridge keeps the native traffic lights shown and centred in Kaze's own
// title bar on a frameless Mac window (spec 7). It owns one attachment at a
// time, named by a generation, so work queued for an old view or a closed
// window does nothing. The UI reads only the published inset.
type macBridge struct {
	nat        lightsNative
	run        func(func()) // runs on the native thread: Window.Run
	invalidate func()       // asks for a frame after the inset changes
	barH, left float64      // the bar's height and the first button's x, in points

	mu     sync.Mutex
	gen    uint64
	view   uintptr // the attached NSView; 0 when none
	inset  float64 // points the lights take from the left; 0 when hidden
	closed bool
	drag   []image.Rectangle // the bar's empty space, in points from the top-left, as last drawn
}

// setDrag publishes the bar's empty space for the native double-click check.
// UI goroutine; rects in px, pxPerPt the window's scale.
func (b *macBridge) setDrag(rects []image.Rectangle, pxPerPt float32) {
	if b == nil || pxPerPt <= 0 {
		return
	}
	pt := make([]image.Rectangle, 0, len(rects))
	for _, r := range rects {
		pt = append(pt, image.Rect(int(float32(r.Min.X)/pxPerPt), int(float32(r.Min.Y)/pxPerPt),
			int(float32(r.Max.X)/pxPerPt+0.999), int(float32(r.Max.Y)/pxPerPt+0.999)))
	}
	b.mu.Lock()
	b.drag = pt
	b.mu.Unlock()
}

// dragAt reports whether a point, in points from the window's top-left, is
// empty bar space. Native thread.
func (b *macBridge) dragAt(x, y float64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := image.Pt(int(x), int(y))
	for _, r := range b.drag {
		if p.In(r) {
			return true
		}
	}
	return false
}

// viewEvent attaches to an AppKit view and detaches from a replaced one; view
// 0 (an invalid AppKitViewEvent) detaches. UI goroutine.
func (b *macBridge) viewEvent(view uintptr) {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed || view != 0 && view == b.view {
		b.mu.Unlock()
		return
	}
	old, oldGen := b.view, b.gen
	b.gen++
	b.view, b.inset = view, 0
	gen := b.gen
	b.mu.Unlock()
	if old != 0 {
		b.run(func() { b.nat.detach(old, oldGen) })
	}
	if view != 0 {
		b.run(func() {
			if b.current(gen) {
				b.nat.attach(view, gen, b.barH, b.left)
			}
		})
	}
}

// reapply places the buttons again; call it after a window option, which
// makes Gio hide them. UI goroutine.
func (b *macBridge) reapply() {
	if b == nil {
		return
	}
	b.mu.Lock()
	gen, view := b.gen, b.view
	b.mu.Unlock()
	if view == 0 {
		return
	}
	b.run(func() {
		if b.current(gen) {
			b.nat.place(view, gen, b.barH, b.left)
		}
	})
}

// publish records the inset a native callback measured for gen and asks for
// a frame. Any goroutine; a stale generation is ignored.
func (b *macBridge) publish(gen uint64, inset float64) {
	b.mu.Lock()
	ok := !b.closed && gen == b.gen && b.view != 0
	if ok {
		b.inset = inset
	}
	b.mu.Unlock()
	if ok && b.invalidate != nil {
		b.invalidate()
	}
}

// close ends the attachment. The native side removes its observers when the
// window closes, so this works even after the runner stops running work.
func (b *macBridge) close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.gen++
	b.view, b.inset = 0, 0
}

// Inset is the points the traffic lights take from the bar's left edge.
func (b *macBridge) Inset() float64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.inset
}

func (b *macBridge) current(gen uint64) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.closed && gen == b.gen
}

func (b *macBridge) generation() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.gen
}

func (b *macBridge) attachedView() uintptr {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.view
}

// nativeRunner runs native work on the main thread in order, from one worker
// goroutine, so Kaze's event loop never waits on the main thread.
func (win *window) nativeRunner() func(func()) {
	q := make(chan func(), 64)
	go func() {
		for f := range q {
			win.w.Run(f)
		}
	}()
	return func(f func()) { q <- f }
}
