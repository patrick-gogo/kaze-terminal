package ui

import (
	"io"
	"strings"
	"testing"

	"gioui.org/io/key"
	"gioui.org/io/transfer"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/skills"
)

func TestPaletteQueryAndEnterSameFrame(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	openPal(t, win, r)
	r.Queue(key.EditEvent{Range: key.Range{}, Text: "ship"}, key.Event{Name: key.NameReturn, State: key.Press})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	waitSent(t, st.ptys[0], "/ship-ticket ")
}

func TestPaletteOpeningAndTypingSameFrame(t *testing.T) {
	win, st, r, _ := palWindow(t)
	sc := shortcutFor(t, win, actPalette)
	r.Queue(key.Event{Name: sc.Name, Modifiers: sc.Modifiers, State: key.Press}, key.Event{Name: sc.Name, Modifiers: sc.Modifiers, State: key.Release}, key.EditEvent{Range: key.Range{}, Text: "ship"})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	if got := st.ptys[0].sent(); got != "" {
		t.Fatalf("palette input leaked into terminal: %q", got)
	}
	if got := win.palette.ed.Text(); got != "ship" {
		t.Fatalf("query %q, want ship", got)
	}
}

func TestPaletteOpeningQueryAndEnterSameFrame(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	// Avoid the asynchronous catalog load deciding whether the fast search matches.
	win.palette.skills = []skills.Skill{{Name: "ship-ticket"}}
	sc := shortcutFor(t, win, actPalette)
	r.Queue(key.Event{Name: sc.Name, Modifiers: sc.Modifiers, State: key.Press},
		key.Event{Name: sc.Name, Modifiers: sc.Modifiers, State: key.Release},
		key.EditEvent{Range: key.Range{}, Text: "ship"}, key.Event{Name: key.NameReturn, State: key.Press})
	for range 4 {
		sideFrame(t, win, r)
	}
	waitSent(t, st.ptys[0], "/ship-ticket ")
	if win.palette.open {
		t.Fatal("palette still open")
	}
}

func TestPaletteQueryArrowAndEnterSameFrame(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	openPal(t, win, r)
	win.palette.skills = []skills.Skill{{Name: "ship-one"}, {Name: "ship-two"}}
	r.Queue(key.EditEvent{Range: key.Range{}, Text: "ship"}, key.Event{Name: key.NameDownArrow, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Press})
	sideFrame(t, win, r)
	sideFrame(t, win, r)
	waitSent(t, st.ptys[0], "/ship-two ")
}

// Enter commits the query before later text is replayed to the terminal.
func TestPaletteEnterPrecedesLaterEdit(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	openPal(t, win, r)
	r.Queue(key.EditEvent{Range: key.Range{}, Text: "ship"},
		key.Event{Name: key.NameReturn, State: key.Press},
		key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: "review"})
	for range 4 {
		sideFrame(t, win, r)
	}
	waitSent(t, st.ptys[0], "/ship-ticket review")
}

func TestPaletteEnterPrecedesLaterArrow(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	openPal(t, win, r)
	win.palette.skills = []skills.Skill{{Name: "ship-one"}, {Name: "ship-two"}}
	r.Queue(key.EditEvent{Range: key.Range{}, Text: "ship"}, key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameDownArrow, State: key.Press})
	for range 4 {
		sideFrame(t, win, r)
	}
	waitSent(t, st.ptys[0], "/ship-one \x1b[B")
}

func TestPaletteShiftEnterPrecedesLaterEdit(t *testing.T) {
	win, st, r, tb := palWindow(t)
	win.noteAgent(notify.Msg{Tab: int(tb[0].ID), Kind: notify.KindDone}, tb[0].ID, shellSeq(tb[0]))
	openPal(t, win, r)
	r.Queue(key.EditEvent{Range: key.Range{}, Text: "ship"}, key.Event{Name: key.NameReturn, State: key.Press, Modifiers: key.ModShift}, key.EditEvent{Range: key.Range{Start: 0, End: 4}, Text: "review"})
	for range 4 {
		sideFrame(t, win, r)
	}
	if !strings.Contains(win.notice, "Opened a new session for /ship-ticket.") {
		t.Fatalf("notice %q", win.notice)
	}
	// Follow-up typing takes over the new shell before its prompt, cancelling
	// the pending automatic launch rather than overwriting the user's text.
	if len(win.launches) != 0 {
		t.Fatal("follow-up typing did not take over the new shell")
	}
	if len(st.ptys) != 4 {
		t.Fatalf("ptys %d", len(st.ptys))
	}
	waitSent(t, st.ptys[3], "review")
}

func TestPaletteNativeEditingOrder(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	r.Queue(key.EditEvent{Range: key.Range{}, Text: "abc"}, key.SelectionEvent{Start: 3, End: 3}, key.Event{Name: key.NameLeftArrow, State: key.Press}, key.Event{Name: key.NameDeleteBackward, State: key.Press})
	sideFrame(t, win, r)
	if got := win.palette.ed.Text(); got != "ac" {
		t.Fatalf("after native Left and Backspace %q", got)
	}
	r.Queue(key.SelectionEvent{Start: 0, End: 1}, key.EditEvent{Range: key.Range{Start: 0, End: 1}, Text: "日"})
	sideFrame(t, win, r)
	if got := win.palette.ed.Text(); got != "日c" {
		t.Fatalf("after selection replacement %q", got)
	}
}

func TestPaletteNativeComposition(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	r.Queue(key.CompositionEvent{Start: 0, End: 1}, key.EditEvent{Range: key.Range{}, Text: "に"}, key.SelectionEvent{Start: 1, End: 1})
	sideFrame(t, win, r)
	r.Queue(key.EditEvent{Range: key.Range{Start: 0, End: 1}, Text: "日"}, key.CompositionEvent{Start: -1, End: -1}, key.SnippetEvent{Start: 0, End: 1})
	sideFrame(t, win, r)
	if got := win.palette.ed.Text(); got != "日" {
		t.Fatalf("composed text %q", got)
	}
	if got := r.EditorState().Snippet.Text; got != "日" {
		t.Fatalf("window IME snippet %q", got)
	}
}

func TestPalettePasteRoundTrip(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured", true: "native"}[native], func(t *testing.T) {
			win, _, r, _ := palWindow(t)
			openPal(t, win, r)
			sc := shortcutFor(t, win, actPaste)
			if native {
				sc.Name = "V"
				sc.Modifiers = key.ModShortcut
			}
			r.Queue(key.Event{Name: sc.Name, Modifiers: sc.Modifiers, State: key.Press})
			sideFrame(t, win, r)
			if !r.ClipboardRequested() {
				t.Fatal("window did not request clipboard")
			}
			r.Queue(transfer.DataEvent{Type: "application/text", Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("ship")) }})
			sideFrame(t, win, r)
			if got := win.palette.ed.Text(); got != "ship" {
				t.Fatalf("pasted query %q", got)
			}
		})
	}
}

func TestPaletteNativeCutAndUndo(t *testing.T) {
	win, _, r, _ := palWindow(t)
	openPal(t, win, r)
	r.Queue(key.EditEvent{Range: key.Range{}, Text: "abc"}, key.SelectionEvent{Start: 0, End: 2}, key.Event{Name: "X", Modifiers: key.ModShortcut, State: key.Press})
	sideFrame(t, win, r)
	if got := win.palette.ed.Text(); got != "c" {
		t.Fatalf("after cut %q", got)
	}
	mime, data, ok := r.WriteClipboard()
	if !ok || mime != "application/text" || string(data) != "ab" {
		t.Fatalf("clipboard %q %q %v", mime, data, ok)
	}
	r.Queue(key.Event{Name: "Z", Modifiers: key.ModShortcut, State: key.Press})
	sideFrame(t, win, r)
	if got := win.palette.ed.Text(); got != "abc" {
		t.Fatalf("after undo %q", got)
	}
}
