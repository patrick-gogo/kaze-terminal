package subagents

import "testing"

func TestReopenedHelperClearsPreviousTurn(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "h", "default"))
	for i := 0; i < maxSteps+2; i++ {
		tr.Apply(tool("h", "Bash"))
	}
	stop := ev(EvStop, "h", "default")
	stop.Report = "old report"
	tr.Apply(stop)
	a := tr.Get(Key{"s1", "h"})
	capacity := cap(a.Steps)
	start := ev(EvStart, "h", "default")
	start.At, start.Prompt = at(600), "new prompt"
	tr.Apply(start)
	if len(a.Steps) != 0 || a.Dropped != 0 || a.Report != "" || a.Activity != (Activity{}) || a.Started != start.At {
		t.Fatalf("previous turn retained: %+v", a)
	}
	if cap(a.Steps) != capacity {
		t.Fatal("step capacity was discarded")
	}
	for _, s := range a.Steps[:cap(a.Steps)] {
		if s.Tool != "" || s.Summary != "" {
			t.Fatal("old strings retained")
		}
	}
}
