package ui

import (
	"fmt"
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"testing"
)

func TestForgottenAgentPaneIDsArePruned(t *testing.T) {
	win := &window{}
	keep := subagents.Key{Session: "keep", ID: "a"}
	win.subs.Apply(subagents.Event{Kind: subagents.EvStart, Session: keep.Session, AgentID: keep.ID, Tab: 2, Src: 2})
	stable := win.fl.agentID(keep)
	for i := 0; i < 128; i++ {
		k := subagents.Key{Session: "old", ID: fmt.Sprint(i)}
		e := subagents.Event{Kind: subagents.EvStart, Session: k.Session, AgentID: k.ID, Tab: 1, Src: 1}
		win.subs.Apply(e)
		win.fl.agentID(k)
		e.Kind = subagents.EvStop
		win.subs.Apply(e)
	}
	// Done eviction must remove identities before the parent closes too.
	win.dropGoneAgents()
	if len(win.fl.agentIDs) > 51 {
		t.Fatalf("forgotten identities retained: %d", len(win.fl.agentIDs))
	}
	win.subs.CloseTab(1)
	win.set.shown = true
	win.dropGoneAgents()
	if len(win.fl.agentIDs) != 1 || win.fl.agentID(keep) != stable {
		t.Fatal("parent cleanup lost a live identity or retained dead ones")
	}
}
