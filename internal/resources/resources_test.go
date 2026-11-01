package resources

import (
	"errors"
	"testing"
	"time"
)

type fakeProvider struct {
	ps      []Process
	samples map[int]Reading
	err     error
}

func (f *fakeProvider) List() ([]Process, bool, error) { return f.ps, false, f.err }
func (f *fakeProvider) Read(p Process) (Reading, error) {
	r, ok := f.samples[p.PID]
	if !ok {
		return Reading{}, errors.New("denied")
	}
	return r, nil
}
func (*fakeProvider) Cores() int { return 4 }
func fixture() (*Collector, *fakeProvider) {
	f := &fakeProvider{ps: []Process{{PID: 1, Birth: 1}, {PID: 2, Parent: 1, Birth: 2}, {PID: 3, Parent: 2, Birth: 3}, {PID: 4, Parent: 3, Birth: 4}, {PID: 5, Parent: 1, Birth: 5}, {PID: 6, Parent: 5, Birth: 6}, {PID: 99, Birth: 99}}, samples: map[int]Reading{}}
	for _, p := range f.ps {
		f.samples[p.PID] = Reading{CPU: time.Second, Bytes: 100, CPUValid: true, RAMValid: true}
	}
	return New(1, f), f
}
func TestOwnershipDeltasAndReuse(t *testing.T) {
	c, f := fixture()
	now := time.Unix(100, 0)
	roots := []Root{{PID: 2, Owner: 10, Name: "Claude"}, {PID: 5, Owner: 10, Name: "Claude"}, {PID: 4, Owner: 20, Name: "Workspace / API"}}
	s := c.Sample(now, roots, false)
	if s.Total.Bytes != 600 || s.Total.CPUValid || len(s.Rows) != 3 {
		t.Fatalf("first: %+v", s)
	}
	if s.Rows[1].Bytes != 400 || s.Rows[2].Bytes != 100 {
		t.Fatalf("rows: %+v", s.Rows)
	}
	for id, r := range f.samples {
		r.CPU += 2 * time.Second
		f.samples[id] = r
	}
	s = c.Sample(now.Add(2*time.Second), roots, false)
	if !s.Total.CPUValid || s.Total.CPU != 100 {
		t.Fatalf("normalized/clamped: %+v", s)
	}
	// Parent and root gone; observed child remains assigned. PID 3 is reused.
	f.ps = []Process{{PID: 1, Birth: 1}, {PID: 3, Birth: 300}, {PID: 4, Birth: 4}, {PID: 6, Birth: 6}}
	s = c.Sample(now.Add(4*time.Second), nil, false)
	if len(s.Rows) != 3 || !s.Rows[1].Closed || s.Total.Bytes != 300 {
		t.Fatalf("reparent/reuse: %+v", s)
	}
	if s.Rows[1].Bytes != 100 || s.Rows[2].Bytes != 100 {
		t.Fatalf("retained ownership %+v", s.Rows)
	}
}
func TestMissingPartialResetAndBounds(t *testing.T) {
	c, f := fixture()
	now := time.Unix(100, 0)
	roots := []Root{{PID: 2, Owner: 10, Name: "session"}}
	delete(f.samples, 3)
	s := c.Sample(now, roots, false)
	if !s.Partial || s.Total.MissingRAM != 1 || s.Total.Bytes != 500 {
		t.Fatalf("partial %+v", s)
	}
	s = c.Sample(now.Add(time.Second), roots, true)
	if s.Total.CPUValid {
		t.Fatal("reset retained baseline")
	}
	f.err = errors.New("unavailable")
	s = c.Sample(now.Add(2*time.Second), roots, false)
	if s.Error == "" || s.Total.RAMValid || s.Total.CPUValid {
		t.Fatalf("failed scan %+v", s)
	}
}
func TestFormatsAndThresholds(t *testing.T) {
	if Size(152<<20) != "152 MB" || Size(1536<<20) != "1.5 GB" {
		t.Fatal("units")
	}
	for _, tc := range []struct {
		p    float64
		want int
	}{{49.9, 0}, {50, 1}, {79.9, 1}, {80, 2}} {
		if Level(tc.p) != tc.want {
			t.Fatal(tc)
		}
	}
}

func TestSmoothingUsesActualIntervalAndResets(t *testing.T) {
	c, f := fixture()
	f.ps = f.ps[:1]
	now := time.Unix(100, 0)
	c.Sample(now, nil, false)
	r := f.samples[1]
	r.CPU += 4 * time.Second
	f.samples[1] = r
	s := c.Sample(now.Add(time.Second), nil, false)
	if s.Level != 2 || s.Total.CPU != 100 {
		t.Fatalf("first interval %+v", s)
	}
	// Three seconds with no CPU: the weighted mean is 25%, not 50%.
	s = c.Sample(now.Add(4*time.Second), nil, false)
	if s.Level != 0 {
		t.Fatal("smoothing did not use elapsed time")
	}
	s = c.Sample(now.Add(20*time.Second), nil, false)
	if s.Total.CPUValid {
		t.Fatal("idle gap reused baseline")
	}
}
func TestCyclesAndReusedParentCannotClaimAnUnrelatedChild(t *testing.T) {
	c, f := fixture()
	f.ps = append(f.ps, Process{PID: 8, Parent: 9, Birth: 8}, Process{PID: 9, Parent: 8, Birth: 9}, Process{PID: 10, Parent: 2, Birth: 1})
	s := c.Sample(time.Unix(100, 0), []Root{{PID: 2, Owner: 10, Name: "shell"}}, false)
	if s.Total.Bytes != 600 {
		t.Fatalf("unrelated descendants counted: %+v", s)
	}
}

func TestUnavailableIdentityKeepsTotalsHonest(t *testing.T) {
	c, f := fixture()
	roots := []Root{{PID: 2, Owner: 10, Name: "shell"}}
	c.Sample(time.Unix(100, 0), roots, false)
	// Listed, but the OS now denies metadata for the owned child.
	f.ps[2] = Process{PID: 3}
	s := c.Sample(time.Unix(102, 0), roots, false)
	if !s.Partial || s.Total.MissingRAM == 0 {
		t.Fatalf("denied identity silently disappeared: %+v", s)
	}
}
