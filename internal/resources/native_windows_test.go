package resources

import (
	"context"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsNativeSelf(t *testing.T) {
	p := Native()
	list, _, err := p.List()
	if err != nil {
		t.Fatal(err)
	}
	var self Process
	for _, v := range list {
		if v.PID == os.Getpid() {
			self = v
		}
	}
	self, err = p.(processInspector).Identify(self)
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Read(self)
	if err != nil || !r.CPUValid || !r.RAMValid || r.Bytes == 0 || self.Birth == 0 || p.Cores() < 1 {
		t.Fatalf("self reading: %+v %+v %v", self, r, err)
	}
	if unsafe.Sizeof(memoryCounters{}) != 80 {
		t.Fatal("unexpected 64-bit memory counter layout")
	}
	if filetime(windows.Filetime{HighDateTime: 1, LowDateTime: 2}) != 1<<32|2 {
		t.Fatal("FILETIME conversion")
	}
	c := New(os.Getpid(), p)
	now := time.Now()
	c.Sample(now, nil, false)
	s := c.Sample(now.Add(time.Second), nil, false)
	if !s.Total.CPUValid || !s.Total.RAMValid {
		t.Fatalf("deltas: %+v", s)
	}
}

// The helper owns no UI and blocks only on the pipe created by its test parent.
func TestWindowsResourceHelper(t *testing.T) {
	if os.Getenv("KAZE_RESOURCE_HELPER") != "1" {
		return
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}

type windowsProbe struct {
	nativeProvider
	inspections, reads int
}

func (p *windowsProbe) Identify(x Process) (Process, error) {
	p.inspections++
	return p.nativeProvider.Identify(x)
}
func (p *windowsProbe) Read(x Process) (Reading, error) { p.reads++; return p.nativeProvider.Read(x) }
func TestWindowsSamplingOverhead(t *testing.T) {
	if os.Getenv("KAZE_RESOURCE_BENCH") != "1" {
		t.Skip("opt-in native Windows sampling probe")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, count := range []int{1, 4, 16} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var commands []*exec.Cmd
			var pipes []io.WriteCloser
			defer func() {
				for _, p := range pipes {
					p.Close()
				}
				for _, cmd := range commands {
					cmd.Wait()
				}
			}()
			var roots []Root
			for i := 0; i < count; i++ {
				cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWindowsResourceHelper$")
				cmd.Env = append(os.Environ(), "KAZE_RESOURCE_HELPER=1")
				pipe, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				pipes = append(pipes, pipe)
				if err = cmd.Start(); err != nil {
					t.Fatal(err)
				}
				commands = append(commands, cmd)
				roots = append(roots, Root{PID: cmd.Process.Pid, Owner: i + 1, Name: "isolated helper"})
			}
			p := &windowsProbe{}
			c := New(os.Getpid(), p)
			list, _, err := p.List()
			if err != nil {
				t.Fatal(err)
			}
			self := Process{PID: os.Getpid()}
			for _, v := range list {
				if v.PID == self.PID {
					self = v
				}
			}
			self, err = p.Identify(self)
			if err != nil {
				t.Fatal(err)
			}
			before, err := p.Read(self)
			if err != nil {
				t.Fatal(err)
			}
			const scans = 50
			var elapsed []time.Duration
			initial := p.inspections
			for i := 0; i < scans; i++ {
				start := time.Now()
				s := c.Sample(start, roots, false)
				elapsed = append(elapsed, time.Since(start))
				if s.Partial {
					t.Fatalf("partial probe: %+v", s)
				}
			}
			after, err := p.Read(self)
			if err != nil {
				t.Fatal(err)
			}
			sort.Slice(elapsed, func(i, j int) bool { return elapsed[i] < elapsed[j] })
			var sum time.Duration
			for _, v := range elapsed {
				sum += v
			}
			inspected := p.inspections - initial
			if inspected > scans*(count+1) {
				t.Fatalf("inspected unrelated processes: %d", inspected)
			}
			t.Logf("roots=%d cores=%d system-processes=%d identity-opens/scan=%.1f mean=%s p95=%s max=%s", count, p.Cores(), len(list), float64(inspected)/scans, sum/scans, elapsed[47], elapsed[49])
			delta := after.CPU - before.CPU
			if delta > 0 {
				t.Logf("CPU/scan=%s one-core-at-2s=%.4f%%", delta/scans, float64(delta)/scans/float64(2*time.Second)*100)
			} else {
				t.Log("CPU delta below native accounting resolution")
			}
		})
	}
}
