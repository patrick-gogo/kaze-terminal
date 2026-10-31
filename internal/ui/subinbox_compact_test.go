package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/subagents"
	"strconv"
	"testing"
)

func TestSubInboxOverflowMakesBatchRoom(t *testing.T) {
	var q subInbox
	for i := 0; i < maxSubEvents+1; i++ {
		q.put(subagents.Event{Kind: subagents.EvSession, Src: 1, Session: "s"}, subagents.Event{Kind: subagents.EvTool, Src: 1, Session: "s", AgentID: "a", Summary: strconv.Itoa(i)})
	}
	if len(q.evs) > maxSubEvents*3/4 {
		t.Fatalf("overflow left %d entries; next event must repeat compaction", len(q.evs))
	}
	events := q.take()
	if events[len(events)-1].Summary != strconv.Itoa(maxSubEvents) {
		t.Fatal("lost newest action")
	}
	if len(q.sessions) != 0 {
		t.Fatal("drained inbox retained source identities")
	}
}

func TestSubInboxSessionTransitionsAcrossSources(t *testing.T) {
	var q subInbox
	for _, e := range []subagents.Event{
		{Kind: subagents.EvSession, Src: 1, Session: "a"}, {Kind: subagents.EvSession, Src: 2, Session: "a"},
		{Kind: subagents.EvSession, Src: 1, Session: "b"}, {Kind: subagents.EvSession, Src: 2, Session: "b"},
		{Kind: subagents.EvSession, Src: 1, Session: "a"}, {Kind: subagents.EvSession, Src: 1, Session: "a"},
	} {
		q.put(e)
	}
	if es := q.take(); len(es) != 5 || es[4].Src != 1 || es[4].Session != "a" {
		t.Fatalf("lost ordered transitions: %v", es)
	}
}

func BenchmarkSubInboxFlood(b *testing.B) {
	var q subInbox
	e := subagents.Event{Kind: subagents.EvTool, Src: 1, Session: "s", AgentID: "a"}
	for i := 0; i < maxSubEvents; i++ {
		q.put(e)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.put(subagents.Event{Kind: subagents.EvSession, Src: 1, Session: "s"}, e)
	}
}
