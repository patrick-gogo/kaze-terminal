package resources

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"syscall"
	"testing"
	"time"
)

func TestNativeSelfAndChild(t *testing.T) {
	cmd := exec.Command("/bin/sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cmd.Process.Kill(); cmd.Wait() }()
	c := New(os.Getpid(), Native())
	s := c.Sample(time.Now(), []Root{{PID: cmd.Process.Pid, Owner: 1, Name: "test session"}}, false)
	if s.Cores < 1 || len(s.Rows) != 2 || !s.Rows[0].RAMValid || !s.Rows[1].RAMValid {
		t.Fatalf("native self/child: %+v", s)
	}
	s = c.Sample(time.Now().Add(time.Millisecond), []Root{{PID: cmd.Process.Pid, Owner: 1, Name: "test session"}}, false)
	if !s.Total.CPUValid {
		t.Fatalf("CPU delta unavailable: %+v", s)
	}
}

// Explicit opt-in for measured collector overhead; never runs a load by default.
func TestNativeSamplingOverhead(t *testing.T) {
	if os.Getenv("KAZE_RESOURCE_BENCH") != "1" {
		t.Skip("opt-in overhead measurement")
	}
	p := Native()
	ps, _, err := p.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{1, 4, 16} {
		func() {
			var cmds []*exec.Cmd
			var roots []Root
			defer func() {
				for _, cmd := range cmds {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			for i := 0; i < count; i++ {
				cmd := exec.Command("/bin/sleep", "60")
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				cmds = append(cmds, cmd)
				roots = append(roots, Root{PID: cmd.Process.Pid, Owner: i + 1, Name: "test"})
			}
			c := New(os.Getpid(), p)
			self := Process{}
			for _, x := range ps {
				if x.PID == os.Getpid() {
					self = x
				}
			}
			before, _ := p.Read(self)
			var memBefore, memAfter runtime.MemStats
			runtime.ReadMemStats(&memBefore)
			var durations []time.Duration
			start := time.Now()
			cold := c.Sample(time.Now(), roots, false)
			coldTime := time.Since(start)
			if cold.Partial {
				t.Fatal("partial baseline")
			}
			const scans = 50
			for i := 0; i < scans; i++ {
				start = time.Now()
				c.Sample(time.Now(), roots, false)
				durations = append(durations, time.Since(start))
			}
			runtime.ReadMemStats(&memAfter)
			after, _ := p.Read(self)
			sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
			var sum time.Duration
			for _, d := range durations {
				sum += d
			}
			cpuPerScan := float64(after.CPU-before.CPU) / float64(scans+1)
			fmt.Printf("roots=%d cores=%d processes=%d cold=%s mean=%s p95=%s max=%s allocated/scan=%d CPU/scan=%.0fns one-core-at-2s=%.4f%%\n", count, p.Cores(), len(ps), coldTime, sum/scans, durations[47], durations[49], (memAfter.TotalAlloc-memBefore.TotalAlloc)/(scans+1), cpuPerScan, cpuPerScan/float64(2*time.Second)*100)
		}()
	}
}

func TestNativeCPUUnitsMatchGetrusage(t *testing.T) {
	p := Native()
	ps, _, err := p.List()
	if err != nil {
		t.Fatal(err)
	}
	self := Process{}
	for _, x := range ps {
		if x.PID == os.Getpid() {
			self = x
		}
	}
	before, _ := p.Read(self)
	var a, b syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &a); err != nil {
		t.Fatal(err)
	}
	// Bounded CPU work exposes incorrect Mach tick vs nanosecond assumptions.
	deadline := time.Now().Add(30 * time.Millisecond)
	for time.Now().Before(deadline) {
	}
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &b); err != nil {
		t.Fatal(err)
	}
	after, _ := p.Read(self)
	expected := time.Duration(b.Utime.Nano() + b.Stime.Nano() - a.Utime.Nano() - a.Stime.Nano())
	got := after.CPU - before.CPU
	if got < expected/2 || got > expected*2 {
		t.Fatalf("CPU counter units: got %s, getrusage %s", got, expected)
	}
}
