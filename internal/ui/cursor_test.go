package ui

import (
	"testing"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/render"
)

func TestRendererOptionsFollowConfig(t *testing.T) {
	for _, tc := range []struct {
		spacing, style string
		want           render.Options
	}{
		{"tight", "bar", render.Options{LineScale: 0.92, Cursor: render.CursorBar}},
		{"normal", "block", render.Options{LineScale: 1, Cursor: render.CursorBlock}},
		{"roomy", "underline", render.Options{LineScale: 1.2, Cursor: render.CursorUnderline}},
	} {
		if got := rendOptions(config.Config{LineSpacing: tc.spacing, CursorStyle: tc.style}); got != tc.want {
			t.Fatalf("%s/%s: %+v, want %+v", tc.spacing, tc.style, got, tc.want)
		}
	}
}

func TestBlinkSchedulesWakeOnlyWhenFocused(t *testing.T) {
	win := testWindow(t, &recStarter{})
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	win.rend = render.NewRenderer(14, win.pal, render.Options{})
	t0 := time.Unix(1000, 0)

	if _, ok := win.blink(t0); ok {
		t.Fatal("blink off scheduled a wake")
	}
	win.opt.Config.CursorBlink = true
	win.focused = false
	if _, ok := win.blink(t0); ok {
		t.Fatal("an unfocused window scheduled a blink wake")
	}

	win.focused = true
	next, ok := win.blink(t0)
	if !ok || !next.Equal(t0.Add(blinkEvery)) || win.cursorHidden {
		t.Fatalf("first blink frame: wake %v %v hidden %v, want shown and a wake at +%v", next, ok, win.cursorHidden, blinkEvery)
	}
	next, ok = win.blink(t0.Add(blinkEvery))
	if !ok || !next.Equal(t0.Add(2*blinkEvery)) || !win.cursorHidden {
		t.Fatalf("second phase: wake %v %v hidden %v, want hidden and a wake at +2 phases", next, ok, win.cursorHidden)
	}

	// A key shows the cursor and restarts the phase.
	win.route(layout.Context{}, []event.Event{key.Event{Name: "A", State: key.Press}}, 8, 16)
	at := t0.Add(blinkEvery + 100*time.Millisecond)
	next, ok = win.blink(at)
	if !ok || !next.Equal(at.Add(blinkEvery)) || win.cursorHidden {
		t.Fatalf("after a key: wake %v %v hidden %v, want shown and a fresh phase", next, ok, win.cursorHidden)
	}

	// Losing focus mid-phase leaves the cursor steady and shown.
	win.blink(at.Add(blinkEvery))
	win.focused = false
	if _, ok := win.blink(at.Add(blinkEvery)); ok || win.cursorHidden {
		t.Fatal("unfocused: the cursor kept blinking or stayed hidden")
	}
}
