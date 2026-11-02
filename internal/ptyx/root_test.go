package ptyx

import "testing"

func TestRootPIDIsStableAndClearedOnClose(t *testing.T) {
	s := &session{}
	if s.RootPID() != 0 {
		t.Fatal("empty session has root")
	}
	s.closed.Store(true)
	if s.RootPID() != 0 {
		t.Fatal("closed session has root")
	}
}
