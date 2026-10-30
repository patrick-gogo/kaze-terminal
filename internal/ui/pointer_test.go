package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

const testCW, testCH = 8, 16

// shellTab opens a tab whose shell has printed out, and returns it with its PTY.
func shellTab(t *testing.T, win *window, st *recStarter, out string, until func() bool) (*tabs.Tab, *recPTY) {
	t.Helper()
	tab, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	p := st.ptys[len(st.ptys)-1]
	st.mu.Unlock()
	go p.w.Write([]byte(out))
	deadline := time.Now().Add(3 * time.Second)
	for !until() {
		if time.Now().After(deadline) {
			t.Fatal("shell output never arrived")
		}
		time.Sleep(time.Millisecond)
	}
	return tab, p
}

// cellPos is the pixel centre of a cell, relative to the grid.
func cellPos(col, row int) f32.Point {
	return f32.Pt(float32(col*testCW+testCW/2), float32(row*testCH+testCH/2))
}

type pointerRun struct {
	t   *testing.T
	win *window
	tab *tabs.Tab
	at  time.Duration
}

func (r *pointerRun) send(kind pointer.Kind, b pointer.Buttons, mods key.Modifiers, col, row int) {
	r.at += 150 * time.Millisecond
	e := pointer.Event{Kind: kind, Buttons: b, Modifiers: mods, Position: cellPos(col, row), Time: r.at}
	r.win.onPointer(layout.Context{}, r.tab, e, modes(r.tab.Session), testCW, testCH)
}

func (r *pointerRun) selected() string {
	ts := r.win.sels[r.tab.ID]
	if ts == nil {
		return ""
	}
	r.tab.Session.Lock()
	defer r.tab.Session.Unlock()
	return render.SelectedText(r.tab.Session.Screen(), ts.sel)
}

func textShown(tab *tabs.Tab, col int, r rune) func() bool {
	return func() bool {
		tab.Session.Lock()
		defer tab.Session.Unlock()
		return tab.Session.Screen().Cell(col, 0).Rune == r
	}
}

func TestPointerSelectsAndCopies(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	var clip []string
	win.toClipboard = func(s string) { clip = append(clip, s) }
	var tab *tabs.Tab
	tab, _ = shellTab(t, win, st, "hello-kaze-selection second-word", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 31, 'd')()
	})
	r := &pointerRun{t: t, win: win, tab: tab}
	P := pointer.ButtonPrimary

	r.send(pointer.Press, P, 0, 0, 0)
	r.send(pointer.Drag, P, 0, 4, 0)
	r.send(pointer.Drag, P, 0, 9, 0)
	r.send(pointer.Release, 0, 0, 9, 0)
	if got := r.selected(); got != "hello-kaze" {
		t.Fatalf("drag selected %q, want hello-kaze", got)
	}
	if len(clip) != 0 {
		t.Fatalf("copied %q with copy_on_select off", clip)
	}
	// The copy shortcut writes the selection to the clipboard.
	win.route(layout.Context{}, []event.Event{key.Event{Name: "C", Modifiers: key.ModCtrl | key.ModShift, State: key.Press}}, testCW, testCH)
	if len(clip) != 1 || clip[0] != "hello-kaze" {
		t.Fatalf("Ctrl+Shift+C copied %q, want [hello-kaze]", clip)
	}

	win.opt.Config.CopyOnSelect = true
	r.at += time.Second
	r.send(pointer.Press, P, 0, 25, 0)
	r.send(pointer.Release, 0, 0, 25, 0)
	r.at -= 100 * time.Millisecond // keep the second click inside the double-click window
	r.send(pointer.Press, P, 0, 25, 0)
	r.send(pointer.Release, 0, 0, 25, 0)
	if got := r.selected(); got != "second-word" {
		t.Fatalf("double-click selected %q, want second-word", got)
	}
	if clip[len(clip)-1] != "second-word" {
		t.Fatalf("copy_on_select copied %q, want second-word", clip[len(clip)-1])
	}

	r.at += time.Second
	r.send(pointer.Press, P, 0, 3, 5)
	r.send(pointer.Release, 0, 0, 3, 5)
	if r.win.sels[tab.ID].sel.Active {
		t.Fatal("a single click left the selection")
	}
}

func TestMouseModeDragGoesToTheAppAndShiftDragSelects(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	// Turn on button-event tracking with SGR reports, then print a word.
	var tab *tabs.Tab
	tab, p := shellTab(t, win, st, "\x1b[?1002h\x1b[?1006hword", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 3, 'd')()
	})
	r := &pointerRun{t: t, win: win, tab: tab}
	P, S := pointer.ButtonPrimary, pointer.ButtonSecondary

	r.send(pointer.Press, P, 0, 0, 0)
	r.send(pointer.Drag, P, 0, 2, 0)
	r.send(pointer.Release, 0, 0, 2, 0)
	if r.selected() != "" {
		t.Fatal("a plain drag selected text while the app has the mouse")
	}
	waitSent(t, p, "\x1b[<0;1;1M\x1b[<32;3;1M\x1b[<0;3;1m")

	// The right button's release is reported as the right button.
	r.send(pointer.Press, S, 0, 1, 0)
	r.send(pointer.Release, 0, 0, 1, 0)
	waitSent(t, p, "\x1b[<0;1;1M\x1b[<32;3;1M\x1b[<0;3;1m\x1b[<2;2;1M\x1b[<2;2;1m")

	r.at += time.Second
	r.send(pointer.Press, P, key.ModShift, 0, 0)
	r.send(pointer.Drag, P, key.ModShift, 3, 0)
	r.send(pointer.Release, 0, key.ModShift, 3, 0)
	if got := r.selected(); got != "word" {
		t.Fatalf("Shift+drag selected %q, want word", got)
	}
	time.Sleep(50 * time.Millisecond)
	if got := p.sent(); got != "\x1b[<0;1;1M\x1b[<32;3;1M\x1b[<0;3;1m\x1b[<2;2;1M\x1b[<2;2;1m" {
		t.Fatalf("Shift+drag was reported to the app: %q", got)
	}
}

// A tab switch while the button is held must end the drag: otherwise autoScroll
// keeps waking the window every 40ms once the old tab is shown again.
func TestTabSwitchMidDragEndsTheDrag(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	tab1, _ := shellTab(t, win, st, "hello", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 4, 'o')()
	})
	win.switchedTo(tab1)
	r := &pointerRun{t: t, win: win, tab: tab1}
	P := pointer.ButtonPrimary
	r.send(pointer.Press, P, 0, 0, 0)
	r.send(pointer.Drag, P, 0, 2, 30) // below the 24-row grid
	if !win.autoScroll(layout.Context{}, tab1, testCW, testCH) {
		t.Fatal("test setup: a drag below the grid did not auto-scroll")
	}

	newTab := key.Event{Name: "T", Modifiers: key.ModCtrl | key.ModShift, State: key.Press}
	win.route(layout.Context{}, []event.Event{newTab}, testCW, testCH)
	tab2 := win.mgr.Active()
	win.switchedTo(tab2)
	r.tab = tab2
	r.send(pointer.Release, 0, 0, 2, 30) // the release arrives while tab 2 is shown

	win.mgr.SetActive(tab1.ID)
	win.switchedTo(tab1)
	if win.sels[tab1.ID].dragging {
		t.Fatal("the first tab is still dragging after the button was released on another tab")
	}
	if win.autoScroll(layout.Context{}, tab1, testCW, testCH) {
		t.Fatal("auto-scroll still asks for frames after the drag ended")
	}
	if win.held != 0 || win.toApp {
		t.Fatalf("gesture state survived the switch: held %v toApp %v", win.held, win.toApp)
	}
}

// An app that saw the press gets its release, even when the tab changed in between.
func TestTabSwitchMidAppDragReleasesInTheOwnerTab(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	tab1, p1 := shellTab(t, win, st, "\x1b[?1002h\x1b[?1006hx", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 0, 'x')()
	})
	win.switchedTo(tab1)
	r := &pointerRun{t: t, win: win, tab: tab1}
	r.send(pointer.Press, pointer.ButtonPrimary, 0, 1, 0)
	waitSent(t, p1, "\x1b[<0;2;1M")

	newTab := key.Event{Name: "T", Modifiers: key.ModCtrl | key.ModShift, State: key.Press}
	win.route(layout.Context{}, []event.Event{newTab}, testCW, testCH)
	tab2 := win.mgr.Active()
	win.switchedTo(tab2)
	r.tab = tab2
	r.send(pointer.Release, 0, 0, 5, 0)

	waitSent(t, p1, "\x1b[<0;2;1M\x1b[<0;2;1m")
	st.mu.Lock()
	p2 := st.ptys[1]
	st.mu.Unlock()
	time.Sleep(50 * time.Millisecond)
	if got := p2.sent(); got != "" {
		t.Fatalf("the new tab got mouse reports %q for a gesture it never saw", got)
	}
}

// A floating pane's press is mapped through its own grid and size, not the main one.
func TestPointerUsesTheViewsGrid(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	tab, p := shellTab(t, win, st, "\x1b[?1000h\x1b[?1006hx", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 0, 'x')()
	})
	v := termView{tab: tab, tag: new(int), grid: image.Rect(300, 200, 300+10*testCW, 200+5*testCH), cols: 10, rows: 5}
	press := func(x, y int) {
		at := f32.Pt(float32(x), float32(y))
		win.pointerView(layout.Context{}, v, pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: at}, modes(tab.Session), testCW, testCH)
		win.pointerView(layout.Context{}, v, pointer.Event{Kind: pointer.Release, Position: at}, modes(tab.Session), testCW, testCH)
	}
	press(300+3*testCW+1, 200+2*testCH+1)
	waitSent(t, p, "\x1b[<0;4;3M\x1b[<0;4;3m")
	press(300+40*testCW, 200+1)
	waitSent(t, p, "\x1b[<0;4;3M\x1b[<0;4;3m\x1b[<0;10;1M\x1b[<0;10;1m")
}
