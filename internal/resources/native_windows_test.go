package resources

import (
	"golang.org/x/sys/windows"
	"os"
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
