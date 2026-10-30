package screen

import (
	"testing"
	"unsafe"
)

// Every history row keeps a Cell per column, so padding costs memory per line.
func TestCellIsSixteenBytes(t *testing.T) {
	if n := unsafe.Sizeof(Cell{}); n > 16 {
		t.Fatalf("Cell is %d bytes, want at most 16", n)
	}
}
