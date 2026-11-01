package ui

import (
	"strings"
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

type blockedInputPTY struct {
	*recPTY
	release <-chan struct{}
}

func (p blockedInputPTY) Write(b []byte) (int, error) { <-p.release; return p.recPTY.Write(b) }

func TestInputOverloadIsVisibleAndDoesNotWaitForEcho(t *testing.T) {
	win := testWindow(t, &recStarter{})
	win.mgr.CloseAll()
	release := make(chan struct{})
	defer close(release)
	win.mgr = tabs.NewManager(func([]string, string) (ptyx.Session, error) { return blockedInputPTY{newRecPTY(), release}, nil }, 20, 4, nil)
	t.Cleanup(win.mgr.CloseAll)
	tab, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	full := false
	for range 2048 {
		if tab.Session.Submit([]byte("x")) == term.QueueFull {
			full = true
			break
		}
	}
	if !full {
		t.Fatal("queue never filled")
	}
	mark := tab.Session.EchoMark()
	win.route(layout.Context{}, []event.Event{key.EditEvent{Text: "rejected"}}, 8, 16)
	if !strings.Contains(win.notice, "Input not sent") {
		t.Fatalf("missing overload notice: %q", win.notice)
	}
	if tab.Session.EchoMark() != mark {
		t.Fatal("rejected input advanced echo tracking")
	}
	win.agents = map[tabs.TabID]agentState{tab.ID: {claude: true}}
	win.startPrompt("/review", false)
	if !strings.Contains(win.notice, "Input not sent") {
		t.Fatalf("palette replaced rejection with success: %q", win.notice)
	}
}
