package term_test

import (
	"testing"

	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

func TestSubmitAcceptedReachesTheShell(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 5, nil)
	defer s.Close()
	if r := s.Submit([]byte("npm run dev\r")); r != term.Accepted {
		t.Fatalf("submit = %v, want Accepted", r)
	}
	waitFor(t, "submitted line", func() bool { return f.writtenString() == "npm run dev\r" })
}

func TestSubmitReportsClosedAfterExitAndClose(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 5, nil)
	defer s.Close()
	f.exit(0)
	waitFor(t, "exit", func() bool { ok, _ := s.Exited(); return ok })
	if r := s.Submit([]byte("x\r")); r != term.Closed {
		t.Fatalf("submit after exit = %v, want Closed", r)
	}

	g := newFake()
	s2 := term.NewSession(g, 20, 5, nil)
	s2.Close()
	if r := s2.Submit([]byte("x\r")); r != term.Closed {
		t.Fatalf("submit after Close = %v, want Closed", r)
	}
}

func TestSubmitReportsQueueFullWithoutBlocking(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{}) // the shell stops reading input, so the queue fills
	s := term.NewSession(f, 20, 5, nil)
	defer s.Close()
	defer close(f.block)
	// Bounded: the queue holds far fewer chunks than this.
	for i := 0; i < 1<<14; i++ {
		switch r := s.Submit([]byte("x")); r {
		case term.Accepted:
			continue
		case term.QueueFull:
			return
		default:
			t.Fatalf("submit %d = %v, want Accepted or QueueFull", i, r)
		}
	}
	t.Fatal("queue never reported full")
}
