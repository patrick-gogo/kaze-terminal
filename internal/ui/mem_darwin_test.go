//go:build darwin

package ui

import (
	"syscall"
	"testing"
)

// Memory kaze touches outside Go's heap (GPU driver, fonts, native windowing) is
// part of its cost. The status bar must count it, as Activity Monitor does.
func TestProcessMBCountsNativeMemory(t *testing.T) {
	before, ok := processMB()
	if !ok {
		t.Fatal("processMB has no reading on macOS")
	}
	const size = 128 << 20
	b, err := syscall.Mmap(-1, 0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANON|syscall.MAP_PRIVATE)
	if err != nil {
		t.Skipf("mmap: %v", err)
	}
	defer syscall.Munmap(b)
	for i := 0; i < size; i += 4096 {
		b[i] = 1 // touch every page so it is resident
	}
	after, _ := processMB()
	if after-before < 100 {
		t.Fatalf("touching 128 MB outside Go's heap moved the reading from %.0f to %.0f MB", before, after)
	}
}
