package ui

import (
	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"image"
	"testing"
)

func TestAgentPaneShortcutClickOpensADoc(t *testing.T) {
	win, _, r, a, _, _, _ := paneWindow(t)
	win.liveMgr.Store(win.mgr)
	dir := t.TempDir()
	want := writeDoc(t, dir, "docs/plan.md", "# Plan")
	report(t, a, dir)
	win.onNotify(notify.Msg{Tab: int(a.ID), Kind: notify.KindAgentStart, Session: "s", AgentID: "x"})
	win.onNotify(notify.Msg{Tab: int(a.ID), Kind: notify.KindAgentStop, Session: "s", AgentID: "x", Report: "Wrote docs/plan.md for review"})
	floatFrames(t, win, r)
	k := subagents.Key{Session: "s", ID: "x"}
	win.openAgentPane(a.ID, k)
	floatFrames(t, win, r)
	p := win.fl.set(a.ID).find(win.fl.agentID(k))
	gtx := pageContext(1000, 600)
	p.scroll = 0
	inner := agentInner(gtx, p.hits.body)
	var at image.Point
	found := false
	for i, l := range p.wrapped {
		if l.text == "Wrote docs/plan.md for review" {
			at = image.Pt(inner.Min.X+win.ui.textWidth(gtx, "Wrote ", 12, font.Font{Typeface: sansFace})+5, inner.Min.Y+i*gtx.Dp(18)+5)
			found = true
		}
	}
	if !found {
		t.Fatal(p.wrapped)
	}
	ref, ok := win.agentPaneLink(gtx, p, at)
	if !ok || ref != "docs/plan.md" {
		t.Fatal(ref, ok)
	}
	e := pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary}
	e.Position.X = float32(at.X)
	e.Position.Y = float32(at.Y)
	win.agentPanePointer(gtx, p, e)
	if win.shownDoc() != nil {
		t.Fatal("plain click")
	}
	e.Modifiers = key.ModShortcut
	win.agentPanePointer(gtx, p, e)
	if d := win.shownDoc(); d == nil || d.path != want || d.session != a.ID {
		t.Fatal(d)
	}
}
