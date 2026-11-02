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
	now     time.Time
}

func (f *fakeProvider) List() ([]Process, bool, error) { return f.ps, false, f.err }
func (f *fakeProvider) Read(p Process) (Reading, error) {
	r, ok := f.samples[p.PID]
	if !ok {
		return Reading{}, errors.New("denied")
	}
	if r.At.IsZero() {
		r.At = f.now
	}
	return r, nil
}
func (*fakeProvider) Cores() int { return 4 }
func sampleFake(c *Collector, f *fakeProvider, now time.Time, roots []Root, reset bool) Snapshot {
	f.now = now
	return c.Sample(now, roots, reset)
}
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
	s := sampleFake(c, f, now, roots, false)
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
	s = sampleFake(c, f, now.Add(2*time.Second), roots, false)
	if !s.Total.CPUValid || s.Total.CPU != 100 {
		t.Fatalf("normalized/clamped: %+v", s)
	}
	// Parent and root gone; observed child remains assigned. PID 3 is reused.
	f.ps = []Process{{PID: 1, Birth: 1}, {PID: 3, Birth: 300}, {PID: 4, Birth: 4}, {PID: 6, Birth: 6}}
	s = sampleFake(c, f, now.Add(4*time.Second), nil, false)
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
	s := sampleFake(c, f, now, roots, false)
	if !s.Partial || s.Total.MissingRAM != 1 || s.Total.Bytes != 500 {
		t.Fatalf("partial %+v", s)
	}
	s = sampleFake(c, f, now.Add(time.Second), roots, true)
	if s.Total.CPUValid {
		t.Fatal("reset retained baseline")
	}
	f.err = errors.New("unavailable")
	s = sampleFake(c, f, now.Add(2*time.Second), roots, false)
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

func TestSmoothingUsesActualIntervalAcrossIdle(t *testing.T) {
	c, f := fixture()
	f.ps = f.ps[:1]
	now := time.Unix(100, 0)
	sampleFake(c, f, now, nil, false)
	r := f.samples[1]
	r.CPU += 4 * time.Second
	f.samples[1] = r
	s := sampleFake(c, f, now.Add(time.Second), nil, false)
	if s.Level != 2 || s.Total.CPU != 100 {
		t.Fatalf("first interval %+v", s)
	}
	// Three seconds with no CPU: the weighted mean is 25%, not 50%.
	s = sampleFake(c, f, now.Add(4*time.Second), nil, false)
	if s.Level != 0 {
		t.Fatal("smoothing did not use elapsed time")
	}
	s = sampleFake(c, f, now.Add(20*time.Second), nil, false)
	if !s.Total.CPUValid {
		t.Fatal("idle gap lost a valid baseline")
	}
}
func TestCyclesAndReusedParentCannotClaimAnUnrelatedChild(t *testing.T) {
	c, f := fixture()
	f.ps = append(f.ps, Process{PID: 8, Parent: 9, Birth: 8}, Process{PID: 9, Parent: 8, Birth: 9}, Process{PID: 10, Parent: 2, Birth: 1})
	s := sampleFake(c, f, time.Unix(100, 0), []Root{{PID: 2, Owner: 10, Name: "shell"}}, false)
	if s.Total.Bytes != 600 {
		t.Fatalf("unrelated descendants counted: %+v", s)
	}
}

func TestUnavailableIdentityKeepsTotalsHonest(t *testing.T) {
	c, f := fixture()
	roots := []Root{{PID: 2, Owner: 10, Name: "shell"}}
	sampleFake(c, f, time.Unix(100, 0), roots, false)
	// Listed, but the OS now denies metadata for the owned child.
	f.ps[2] = Process{PID: 3, Parent: 2}
	s := sampleFake(c, f, time.Unix(102, 0), roots, false)
	if !s.Partial || s.Total.MissingRAM == 0 {
		t.Fatalf("denied identity silently disappeared: %+v", s)
	}
}

func TestReviewIdleAndNewProcessesKeepCPUAvailable(t *testing.T) {
	c, f := fixture()
	now := time.Unix(100, 0)
	roots := []Root{{PID: 2, Owner: 10, Name: "Claude"}}
	sampleFake(c, f, now, roots, false)
	r := f.samples[1]
	r.CPU += time.Second
	f.samples[1] = r
	s := sampleFake(c, f, now.Add(30*time.Second), roots, false)
	if !s.Total.CPUValid || s.Total.MissingCPU != 0 {
		t.Fatalf("idle invalidated a live baseline: %+v", s)
	}
	f.ps = append(f.ps, Process{PID: 100, Parent: 2, Birth: 100})
	f.samples[100] = Reading{CPU: time.Second, Bytes: 42, CPUValid: true, RAMValid: true}
	s = sampleFake(c, f, now.Add(32*time.Second), roots, false)
	if !s.Total.CPUValid || s.Partial || s.Total.MissingCPU != 0 {
		t.Fatalf("new process made totals partial: %+v", s)
	}
}

func TestReviewClosedPIDWithoutIdentityCannotRetainOwner(t *testing.T) {
	c, f := fixture()
	now := time.Unix(100, 0)
	sampleFake(c, f, now, []Root{{PID: 2, Owner: 10, Name: "old session"}}, false)
	// After the session closes, a protected unrelated process takes its PID.
	f.ps = []Process{{PID: 1, Birth: 1}, {PID: 2, Parent: 99}}
	s := sampleFake(c, f, now.Add(2*time.Second), nil, false)
	if s.Partial || len(s.Rows) != 1 || len(c.owned) != 1 {
		t.Fatalf("unverified PID retained old ownership: %+v", s)
	}
}

func TestReviewCPUUsesCounterTimesNotFrameTimes(t *testing.T) {
	c, f := fixture()
	f.ps = f.ps[:1]
	frame := time.Unix(100, 0)
	f.samples[1] = Reading{At: frame.Add(time.Second), CPU: time.Second, CPUValid: true, RAMValid: true}
	sampleFake(c, f, frame, nil, false)
	f.samples[1] = Reading{At: frame.Add(5 * time.Second), CPU: 5 * time.Second, CPUValid: true, RAMValid: true}
	s := sampleFake(c, f, frame.Add(2*time.Second), nil, false)
	// Four CPU seconds across four real seconds on four cores is 25%, not 50%.
	if s.Total.CPU != 25 || s.Interval != 4*time.Second {
		t.Fatalf("used frame delay rather than counter interval: %+v", s)
	}
}

type inspectingFake struct {
	*fakeProvider
	identities map[int]Process
	inspected  []int
}

func (f *inspectingFake) Identify(p Process) (Process, error) {
	f.inspected = append(f.inspected, p.PID)
	if id, ok := f.identities[p.PID]; ok {
		return id, nil
	}
	return p, errors.New("denied")
}
func TestReviewInspectOnlyOwnedCandidates(t *testing.T) {
	f := &inspectingFake{fakeProvider: &fakeProvider{samples: map[int]Reading{}}, identities: map[int]Process{}}
	for i := 1; i <= 500; i++ {
		p := Process{PID: i, Parent: 999}
		if i == 1 {
			p.Parent = 0
		}
		if i == 2 || i == 3 {
			p.Parent = i - 1
		}
		f.ps = append(f.ps, p)
		p.Birth = uint64(i)
		f.identities[i] = p
		f.samples[i] = Reading{At: time.Unix(100, 0), CPU: time.Second, CPUValid: true, RAMValid: true}
	}
	c := New(1, f)
	s := c.Sample(time.Unix(100, 0), []Root{{PID: 2, Owner: 10, Name: "session"}}, false)
	if len(f.inspected) != 3 || s.Partial {
		t.Fatalf("opened unrelated processes: %d inspections, %+v", len(f.inspected), s)
	}
	// A reused protected PID must not trigger inspection of its replacement's tree.
	f.ps = []Process{{PID: 1}, {PID: 2, Parent: 999}, {PID: 4, Parent: 2}}
	delete(f.identities, 2)
	f.inspected = nil
	s = c.Sample(time.Unix(102, 0), nil, false)
	if len(f.inspected) != 2 || len(s.Rows) != 1 || s.Partial {
		t.Fatalf("followed unverified old PID: inspected=%v snapshot=%+v", f.inspected, s)
	}
}

func TestReviewNewRootsAndReadFailuresAreIndependent(t *testing.T) {
	c, f := fixture()
	now := time.Unix(100, 0)
	roots := []Root{{PID: 2, Owner: 10, Name: "before"}}
	sampleFake(c, f, now, roots, false)
	f.ps = append(f.ps, Process{PID: 100, Parent: 1, Birth: 100})
	f.samples[100] = Reading{CPU: time.Second, Bytes: 100, CPUValid: true, RAMValid: true}
	roots = append(roots, Root{PID: 100, Owner: 20, Name: "new tab"})
	roots[0].Name = "spinner"
	s := sampleFake(c, f, now.Add(2*time.Second), roots, false)
	if !s.Total.CPUValid || s.Partial || s.Total.WarmingCPU != 1 || s.Total.MissingCPU != 0 {
		t.Fatalf("new root disrupted existing readings: %+v", s)
	}
	// A real counter read failure is still missing and partial, never warming.
	r := f.samples[100]
	r.CPUValid = false
	f.samples[100] = r
	s = sampleFake(c, f, now.Add(4*time.Second), roots, false)
	if !s.Partial || s.Total.MissingCPU != 1 || s.Total.WarmingCPU != 0 {
		t.Fatalf("read failure hidden as warmup: %+v", s)
	}
	delete(f.samples, 100) // also deny memory
	s = sampleFake(c, f, now.Add(6*time.Second), roots, false)
	if !s.Partial || s.Total.MissingRAM != 1 {
		t.Fatalf("memory denial hidden: %+v", s)
	}
}

func TestReviewVerifiedSurvivorRetainsOwnershipAfterClose(t *testing.T) {
	f := &inspectingFake{fakeProvider: &fakeProvider{samples: map[int]Reading{}, now: time.Unix(100, 0)}, identities: map[int]Process{}}
	for _, p := range []Process{{PID: 1, Birth: 1}, {PID: 2, Parent: 1, Birth: 2}, {PID: 3, Parent: 2, Birth: 3}} {
		f.identities[p.PID] = p
		f.ps = append(f.ps, Process{PID: p.PID, Parent: p.Parent})
		f.samples[p.PID] = Reading{CPU: time.Second, CPUValid: true, RAMValid: true, Bytes: 100}
	}
	c := New(1, f)
	c.Sample(f.now, []Root{{PID: 2, Owner: 10, Name: "closed later"}}, false)
	f.ps = []Process{{PID: 1}, {PID: 3, Parent: 999}}
	f.identities[3] = Process{PID: 3, Parent: 999, Birth: 3}
	f.now = f.now.Add(2 * time.Second)
	s := c.Sample(f.now, nil, false)
	if len(s.Rows) != 2 || s.Rows[1].ID != 10 || !s.Rows[1].Closed || !s.Rows[1].CPUValid || s.Partial {
		t.Fatalf("verified surviving child lost ownership: %+v", s)
	}
}
