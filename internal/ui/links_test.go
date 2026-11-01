package ui

import (
	"image"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// Issue #67: Cmd+click (Ctrl+click on Windows) opens a link; a plain click still selects.
func TestShortcutClickOpensALink(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	var opened []string
	win.openLink = func(u string) error { opened = append(opened, u); return nil }
	var tab *tabs.Tab
	tab, _ = shellTab(t, win, st, "see https://example.com/pr/68 ok", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 31, 'k')()
	})
	r := &pointerRun{t: t, win: win, tab: tab}
	P := pointer.ButtonPrimary

	r.send(pointer.Press, P, 0, 10, 0)
	r.send(pointer.Release, 0, 0, 10, 0)
	if len(opened) != 0 {
		t.Fatalf("a plain click opened %v", opened)
	}
	r.at += time.Second
	r.send(pointer.Press, P, key.ModShortcut, 10, 0)
	r.send(pointer.Drag, P, key.ModShortcut, 14, 0)
	r.send(pointer.Release, 0, key.ModShortcut, 14, 0)
	if len(opened) != 1 || opened[0] != "https://example.com/pr/68" {
		t.Fatalf("opened %v", opened)
	}
	if ts := win.sels[tab.ID]; ts != nil && ts.sel.Active {
		t.Fatal("the link click also selected text")
	}
	r.at += time.Second
	r.send(pointer.Press, P, key.ModShortcut, 1, 0)
	r.send(pointer.Release, 0, key.ModShortcut, 1, 0)
	if len(opened) != 1 {
		t.Fatalf("a shortcut click off the link opened %v", opened)
	}
}

// The link click works while the app (Claude Code, vim) has the mouse, and the app never sees it.
func TestShortcutClickOpensALinkInMouseMode(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	var opened []string
	win.openLink = func(u string) error { opened = append(opened, u); return nil }
	var tab *tabs.Tab
	tab, p := shellTab(t, win, st, "\x1b[?1002h\x1b[?1006hhttps://a.io/x", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 13, 'x')()
	})
	r := &pointerRun{t: t, win: win, tab: tab}
	r.send(pointer.Press, pointer.ButtonPrimary, key.ModShortcut, 3, 0)
	r.send(pointer.Release, 0, key.ModShortcut, 3, 0)
	if len(opened) != 1 || opened[0] != "https://a.io/x" {
		t.Fatalf("opened %v", opened)
	}
	time.Sleep(50 * time.Millisecond)
	p.mu.Lock()
	got := p.got.String()
	p.mu.Unlock()
	if got != "" {
		t.Fatalf("the app saw the link click: %q", got)
	}
}

func TestLinkRectsCoverTheLinkCells(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	var tab *tabs.Tab
	tab, _ = shellTab(t, win, st, "go https://a.io/x", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 16, 'x')()
	})
	grid := image.Rect(100, 50, 100+80*testCW, 50+24*testCH)
	rects := win.linkRects(tab.Session, grid, testCW, testCH)
	want := image.Rect(100+3*testCW, 50, 100+17*testCW, 50+testCH)
	if len(rects) != 1 || rects[0] != want {
		t.Fatalf("got %v, want [%v]", rects, want)
	}
}

// The hand-cursor areas sit inside the terminal's input area, so a press on a
// link still reaches the terminal, and the router shows the hand there.
func TestLinkCursorAreasPassPressesToTheTerminal(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	tab, _ := shellTab(t, win, st, "go https://a.io/x", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 16, 'x')()
	})
	grid := image.Rect(0, 0, 80*testCW, 24*testCH)
	tag := new(int)
	ops := new(op.Ops)
	area := clip.Rect(grid).Push(ops)
	event.Op(ops, tag)
	win.linkCursors(ops, tab.Session, grid, testCW, testCH)
	area.Pop()
	var r input.Router
	filter := pointer.Filter{Target: tag, Kinds: pointer.Press}
	r.Event(filter) // register interest before the frame
	r.Frame(ops)
	at := f32.Pt(float32(5*testCW+2), float32(testCH/2))
	r.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: at})
	if c := r.Cursor(); c != pointer.CursorPointer {
		t.Fatalf("cursor over the link is %v", c)
	}
	r.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: at})
	got := false
	for {
		ev, ok := r.Event(filter)
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && e.Kind == pointer.Press {
			got = true
		}
	}
	if !got {
		t.Fatal("the terminal never got the press on the link")
	}
}

// A screenshot on the clipboard reaches kaze as an empty text paste. On macOS,
// Cmd+V then sends Ctrl+V, the key Claude Code and Codex use to paste an image,
// but only to an agent and only when the clipboard really holds an image: in a
// plain zsh, Ctrl+V would make the next key literal.
func TestEmptyPasteSendsCtrlVOnlyForAnAgentImage(t *testing.T) {
	if got := emptyPasteKey("darwin"); got != "\x16" {
		t.Fatalf("darwin: %q", got)
	}
	if got := emptyPasteKey("windows"); got != "" {
		t.Fatalf("windows: %q", got)
	}
	if runtime.GOOS != "darwin" {
		t.Skip("the window path uses the running OS")
	}
	empty := transfer.DataEvent{Type: "application/text", Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("")) }}
	for _, c := range []struct {
		name         string
		agent, image bool
		want         string
	}{
		{"shell with an image", false, true, ""},
		{"agent with an empty clipboard", true, false, ""},
		{"agent with an image", true, true, "\x16"},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := &recStarter{}
			win := testWindow(t, st)
			win.clipImage = func() bool { return c.image }
			tab, p := shellTab(t, win, st, "x", func() bool {
				a := win.mgr.Active()
				return a != nil && textShown(a, 0, 'x')()
			})
			if c.agent {
				win.agents = map[tabs.TabID]agentState{tab.ID: {agent: "claude", claude: true, running: true}}
			}
			win.paste(tab, empty, modes(tab.Session))
			if c.want != "" {
				waitSent(t, p, c.want)
				return
			}
			time.Sleep(50 * time.Millisecond)
			if got := p.sent(); got != "" {
				t.Fatalf("sent %q", got)
			}
		})
	}
}
