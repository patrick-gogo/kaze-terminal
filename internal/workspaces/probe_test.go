package workspaces

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// fakeClock drives Now and After with virtual time so cadence tests never sleep.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	waited []time.Duration
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	c.waited = append(c.waited, d)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

func dockerProbes(c *fakeClock, exec func(call int) error) (Probes, *int, *int) {
	var execs, starts int
	p := Probes{
		Exec: func(ctx context.Context, name string, args ...string) error {
			execs++
			return exec(execs)
		},
		StartDocker: func() error { starts++; return nil },
		Now:         c.Now,
		After:       c.After,
	}
	return p, &execs, &starts
}

func TestWaitDockerReadyOnFirstProbe(t *testing.T) {
	c := newFakeClock()
	p, execs, starts := dockerProbes(c, func(int) error { return nil })
	if err := p.WaitDocker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *execs != 1 || *starts != 0 || len(c.waited) != 0 {
		t.Fatalf("execs=%d starts=%d waited=%v", *execs, *starts, c.waited)
	}
}

func TestWaitDockerReadyOnThirdProbe(t *testing.T) {
	c := newFakeClock()
	p, execs, starts := dockerProbes(c, func(n int) error {
		if n < 3 {
			return errors.New("daemon not running")
		}
		return nil
	})
	if err := p.WaitDocker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *execs != 3 || *starts != 1 {
		t.Fatalf("execs=%d starts=%d", *execs, *starts)
	}
	for _, d := range c.waited {
		if d != 2*time.Second {
			t.Fatalf("cadence %v, want 2s", c.waited)
		}
	}
}

func TestWaitDockerStartsDesktopOnce(t *testing.T) {
	c := newFakeClock()
	p, _, starts := dockerProbes(c, func(n int) error {
		if n < 10 {
			return errors.New("down")
		}
		return nil
	})
	if err := p.WaitDocker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *starts != 1 {
		t.Fatalf("starts=%d, want 1", *starts)
	}
}

func TestWaitDockerMissing(t *testing.T) {
	c := newFakeClock()
	p, execs, _ := dockerProbes(c, func(int) error { return exec.ErrNotFound })
	p.StartDocker = func() error { return ErrDockerMissing }
	err := p.WaitDocker(context.Background())
	if !errors.Is(err, ErrDockerMissing) {
		t.Fatalf("err=%v, want ErrDockerMissing", err)
	}
	if *execs != 1 || len(c.waited) != 0 {
		t.Fatalf("should fail promptly: execs=%d waited=%v", *execs, c.waited)
	}
}

func TestWaitDockerCLIPresentDesktopAbsentKeepsWaiting(t *testing.T) {
	c := newFakeClock()
	p, execs, _ := dockerProbes(c, func(n int) error {
		if n < 3 {
			return errors.New("daemon down")
		}
		return nil
	})
	p.StartDocker = func() error { return ErrDockerMissing }
	if err := p.WaitDocker(context.Background()); err != nil {
		t.Fatal(err)
	}
	if *execs != 3 {
		t.Fatalf("execs=%d", *execs)
	}
}

func TestWaitDockerDeadline(t *testing.T) {
	c := newFakeClock()
	p, execs, _ := dockerProbes(c, func(int) error { return errors.New("down") })
	err := p.WaitDocker(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if got := c.Now().Sub(time.Unix(1_000_000, 0)); got < 3*time.Minute || got > 3*time.Minute+5*time.Second {
		t.Fatalf("virtual elapsed %v", got)
	}
	if *execs < 80 || *execs > 100 {
		t.Fatalf("execs=%d", *execs)
	}
}

func TestWaitDockerCancelStopsLoop(t *testing.T) {
	c := newFakeClock()
	ctx, cancel := context.WithCancel(context.Background())
	p, execs, _ := dockerProbes(c, func(n int) error {
		if n == 2 {
			cancel()
		}
		return errors.New("down")
	})
	err := p.WaitDocker(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if *execs > 3 {
		t.Fatalf("loop kept going: %d", *execs)
	}
}

func TestWaitDockerHungExecAbandonedAtPerProbeDeadline(t *testing.T) {
	c := newFakeClock()
	release := make(chan struct{})
	defer close(release)
	fired := make(chan time.Time, 1)
	fired <- time.Time{}
	entered := make(chan struct{})
	var mu sync.Mutex
	calls := 0
	p := Probes{
		Exec: func(ctx context.Context, name string, args ...string) error {
			mu.Lock()
			calls++
			n := calls
			mu.Unlock()
			if n == 1 {
				close(entered)
				<-release // ignores ctx on purpose
			}
			return nil
		},
		StartDocker: func() error { return nil },
		Now:         c.Now,
		After:       c.After,
		Timeout: func(time.Duration) <-chan time.Time {
			// Fire only once the first Exec is actually hung.
			select {
			case <-entered:
				return fired
			case <-time.After(500 * time.Millisecond):
				return nil
			}
		},
	}
	done := make(chan error, 1)
	go func() { done <- p.WaitDocker(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("hung Exec was not abandoned")
	}
	if len(c.waited) == 0 || c.waited[0] != 2*time.Second {
		t.Fatalf("waited=%v", c.waited)
	}
}

func portProbes(c *fakeClock, ready func(call int, addr string) error) (Probes, *[]string) {
	var addrs []string
	return Probes{
		Dial: func(ctx context.Context, addr string) error {
			addrs = append(addrs, addr)
			return ready(len(addrs), addr)
		},
		Now:   c.Now,
		After: c.After,
	}, &addrs
}

func TestWaitPortReadyOnLoopbackV4(t *testing.T) {
	c := newFakeClock()
	p, addrs := portProbes(c, func(int, string) error { return nil })
	if err := p.WaitPort(context.Background(), 5432); err != nil {
		t.Fatal(err)
	}
	if len(*addrs) != 1 || (*addrs)[0] != "127.0.0.1:5432" {
		t.Fatalf("addrs=%v", *addrs)
	}
}

func TestWaitPortReadyOnV6Only(t *testing.T) {
	c := newFakeClock()
	p, addrs := portProbes(c, func(_ int, addr string) error {
		if addr == "[::1]:3000" {
			return nil
		}
		return errors.New("refused")
	})
	if err := p.WaitPort(context.Background(), 3000); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(*addrs) != "[127.0.0.1:3000 [::1]:3000]" || len(c.waited) != 0 {
		t.Fatalf("addrs=%v waited=%v", *addrs, c.waited)
	}
}

func TestWaitPortCadenceAndDeadline(t *testing.T) {
	c := newFakeClock()
	p, _ := portProbes(c, func(int, string) error { return errors.New("refused") })
	err := p.WaitPort(context.Background(), 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	for _, d := range c.waited {
		if d != time.Second {
			t.Fatalf("cadence %v, want 1s", d)
		}
	}
}

func TestWaitPortCancel(t *testing.T) {
	c := newFakeClock()
	ctx, cancel := context.WithCancel(context.Background())
	p, addrs := portProbes(c, func(n int, _ string) error {
		if n == 3 {
			cancel()
		}
		return errors.New("refused")
	})
	if err := p.WaitPort(ctx, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if len(*addrs) > 6 {
		t.Fatalf("loop kept going: %d", len(*addrs))
	}
}

// Exercises the default Dial against a listener the test owns.
func TestDefaultDialReachesLocalListener(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no loopback listener")
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if err := (Probes{}).WaitPort(context.Background(), port); err != nil {
		t.Fatal(err)
	}
}
