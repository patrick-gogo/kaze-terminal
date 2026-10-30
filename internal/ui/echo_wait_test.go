package ui

import (
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// echoPTY echoes every write back, like a shell at its prompt.
type echoPTY struct{ *recPTY }

func (p echoPTY) Write(b []byte) (int, error) {
	n, err := p.recPTY.Write(b)
	echo := append([]byte(nil), b...)
	go p.w.Write(echo) // the reader applies it; a closed pipe just drops it
	return n, err
}

func TestTypedKeyEchoShowsInTheFrameThatSentIt(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.mgr.CloseAll()
	win.mgr = tabs.NewManager(func([]string, string) (ptyx.Session, error) {
		return echoPTY{newRecPTY()}, nil
	}, 80, 24, nil)
	t.Cleanup(win.mgr.CloseAll)
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	tab := win.mgr.Active()
	win.routeAndWait(layout.Context{}, tab, []event.Event{key.EditEvent{Text: "q"}}, 8, 16)
	tab.Session.Lock()
	got := tab.Session.Screen().Cell(0, 0).Rune
	tab.Session.Unlock()
	if got != 'q' {
		t.Fatalf("cell after the frame's input = %q, want the echoed q already there", got)
	}
}
