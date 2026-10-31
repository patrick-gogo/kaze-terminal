package subagents

import "testing"

func TestLateStopCompletesSynthesizedDone(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	tr.EndTab(1, at(10))
	stop := ev(EvStop, "a1", "Explore")
	stop.At, stop.Report = at(11), "Final result"
	if !tr.Apply(stop) {
		t.Fatal("late report ignored")
	}
	a := tr.Get(Key{"s1", "a1"})
	if a.Report != stop.Report || a.State != Done || !a.Ended.Equal(stop.At) {
		t.Fatalf("completion %+v", a)
	}
	_, _, done := tr.Board()
	if len(done) != 1 {
		t.Fatalf("duplicate completion: %d", len(done))
	}
	stop.Report = "duplicate"
	if tr.Apply(stop) || a.Report != "Final result" {
		t.Fatal("duplicate stop overwrote report")
	}
}

func TestRealStopIsFinalEvenWithEmptyReport(t *testing.T) {
	var tr Tracker
	tr.Apply(ev(EvStart, "a1", "Explore"))
	stop := ev(EvStop, "a1", "Explore")
	tr.Apply(stop)
	stop.Report = "duplicate"
	if tr.Apply(stop) || tr.Get(Key{"s1", "a1"}).Report != "" {
		t.Fatal("replaced a real stop")
	}
}
