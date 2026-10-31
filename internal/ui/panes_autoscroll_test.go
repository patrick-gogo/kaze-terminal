package ui

import (
	"image"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

func historyOffset(tab *tabs.Tab) int {
	tab.Session.Lock()
	defer tab.Session.Unlock()
	return tab.Session.Screen().ViewOffset()
}

func floatFrameAt(t *testing.T, win *window, r *input.Router, now time.Time) {
	t.Helper()
	gtx := pageContext(1000, 600)
	gtx.Source, gtx.Now = r.Source(), now
	if err := win.frame(gtx); err != nil {
		t.Fatal(err)
	}
	r.Frame(gtx.Ops)
}

func TestPaneSelectionAutoScrollsOnlyItsOwnHistory(t *testing.T) {
	for _, below := range []bool{false, true} {
		t.Run(map[bool]string{false: "above", true: "below"}[below], func(t *testing.T) {
			win, st, r, a, _, m1, m2 := paneWindow(t)
			win.fl.open(a.ID, m2.ID, paneMember)
			p, _ := win.fl.open(a.ID, m1.ID, paneMember)
			win.focusPane(m1.ID)
			floatFrames(t, win, r)
			for i, tab := range []*tabs.Tab{a, m1, m2} {
				ptyIndex := []int{0, 2, 3}[i]
				emitTo(t, st, ptyIndex, tab, strings.Repeat("line\r\n", 100), func() bool {
					tab.Session.Lock()
					defer tab.Session.Unlock()
					return tab.Session.Screen().ScrollbackLen() > 20
				})
			}
			if below {
				m1.Session.Lock()
				m1.Session.Screen().SetViewOffset(5)
				m1.Session.Unlock()
			}
			floatFrames(t, win, r)
			var v termView
			for _, pv := range win.fl.views {
				if pv.p == p {
					v = pv.v
				}
			}
			cw, ch := win.rend.CellSize()
			from := v.grid.Min.Add(image.Pt(cw/2, ch/2))
			to := image.Pt(from.X, v.grid.Min.Y-ch)
			if below {
				to.Y = v.grid.Min.Y + v.rows*ch + ch
			}
			now := time.Unix(1000, 0)
			r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at(from)})
			floatFrameAt(t, win, r, now)
			r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at(to)})
			floatFrameAt(t, win, r, now)
			before := historyOffset(m1)
			floatFrameAt(t, win, r, now.Add(autoScrollEvery))
			got := historyOffset(m1)
			if (!below && got <= before) || (below && got >= before) {
				t.Fatalf("offset %d after %d: selection did not auto-scroll", got, before)
			}
			if historyOffset(a) != 0 || historyOffset(m2) != 0 {
				t.Fatal("selection scrolled another terminal")
			}
			if wake, ok := r.WakeupTime(); !ok || wake.IsZero() {
				t.Fatalf("held outside selection has no timed wake: %v %v", wake, ok)
			}
			r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at(from)})
			floatFrameAt(t, win, r, now.Add(2*autoScrollEvery))
			stopped := historyOffset(m1)
			floatFrameAt(t, win, r, now.Add(3*autoScrollEvery))
			if historyOffset(m1) != stopped {
				t.Fatal("selection kept scrolling after returning inside")
			}
			if _, ok := r.WakeupTime(); ok {
				t.Fatal("selection inside kept scheduling frames")
			}
		})
	}
}
