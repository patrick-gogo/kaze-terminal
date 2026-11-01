package term_test

import (
	"github.com/patrick-gogo/kaze-terminal/internal/term"
	"testing"
	"time"
)

// fgPTY is a fakePTY that reports a foreground process group.
type fgPTY struct {
	*fakePTY
	pg int
}

func (f fgPTY) Foreground() int { return f.pg }

func TestForegroundAndOutputSeq(t *testing.T) {
	f := newFake()
	changed := make(chan struct{}, 8)
	s := term.NewSession(fgPTY{f, 42}, 20, 4, func() { changed <- struct{}{} })
	t.Cleanup(s.Close)
	if s.Foreground() != 42 {
		t.Fatalf("foreground %d", s.Foreground())
	}
	before := s.OutputSeq()
	f.outW.Write([]byte("hi"))
	select {
	case <-changed:
	case <-time.After(2 * time.Second):
		t.Fatal("output not applied")
	}
	if s.OutputSeq() == before {
		t.Fatal("output did not advance the sequence")
	}
	plain := term.NewSession(newFake(), 20, 4, func() {})
	t.Cleanup(plain.Close)
	if plain.Foreground() != 0 {
		t.Fatal("a pty without a foreground group must report 0")
	}
}
