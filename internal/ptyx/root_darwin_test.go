package ptyx

import (
	"os"
	"testing"
)

func TestLiveRootPID(t *testing.T) {
	s, err := Start([]string{"/bin/sleep", "10"}, 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r, ok := s.(Rooter)
	if !ok || r.RootPID() <= 0 {
		t.Fatal("live root unavailable")
	}
	pid := r.RootPID()
	if pid != r.RootPID() {
		t.Fatal("root changed")
	}
	s.Close()
	if r.RootPID() != 0 {
		t.Fatal("closed root retained")
	}
}
