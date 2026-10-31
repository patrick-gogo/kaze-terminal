package subagents

import (
	"strconv"
	"testing"
)

func TestToolHistoryReusesStorage(t *testing.T) {
	var tr Tracker
	e := tool("a1", "Read")
	for i := 0; i < maxSteps; i++ {
		tr.Apply(e)
	}
	if n := testing.AllocsPerRun(100, func() { tr.Apply(e) }); n != 0 {
		t.Fatalf("full history allocates %.0f times per step", n)
	}
	for i := 0; i < maxSteps+3; i++ {
		e.Summary = strconv.Itoa(i)
		tr.Apply(e)
	}
	a := tr.Get(Key{"s1", "a1"})
	if len(a.Steps) != maxSteps || a.Steps[0].Summary != "3" || a.Steps[maxSteps-1].Summary != strconv.Itoa(maxSteps+2) {
		t.Fatal("history no longer contains the newest ordered steps")
	}
}

func BenchmarkToolHistoryFull(b *testing.B) {
	var tr Tracker
	e := tool("a1", "Read")
	for i := 0; i < maxSteps; i++ {
		tr.Apply(e)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tr.Apply(e)
	}
}
