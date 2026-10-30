package term_test

import (
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

// A key's echo usually lands a fraction of a millisecond after the key is
// written. WaitEcho lets the frame that sent the key wait for it, so the echo
// shows in that frame instead of one screen refresh later.
func TestWaitEchoReturnsOnceOutputFollowsAWrite(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 4, nil)
	defer s.Close()
	mark := s.EchoMark()
	s.Write([]byte("x"))
	go func() {
		time.Sleep(time.Millisecond)
		f.outW.Write([]byte("x"))
	}()
	start := time.Now()
	if !s.WaitEcho(mark, 2*time.Second) {
		t.Fatal("WaitEcho gave up although the echo arrived")
	}
	if d := time.Since(start); d > 500*time.Millisecond {
		t.Fatalf("WaitEcho took %v; it should return as soon as output arrives", d)
	}
	s.Lock()
	got := s.Screen().Cell(0, 0).Rune
	s.Unlock()
	if got != 'x' {
		t.Fatalf("echo not applied when WaitEcho returned: %q", got)
	}
}

func TestWaitEchoDoesNotWaitWithoutAWrite(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 4, nil)
	defer s.Close()
	start := time.Now()
	if s.WaitEcho(s.EchoMark(), 2*time.Second) {
		t.Fatal("WaitEcho reported an echo with nothing written")
	}
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Fatalf("WaitEcho waited %v with nothing written; it must return at once", d)
	}
}

func TestWaitEchoGivesUpAfterItsLimit(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 4, nil)
	defer s.Close()
	mark := s.EchoMark()
	s.Write([]byte("secret")) // e.g. a password prompt: nothing comes back
	start := time.Now()
	if s.WaitEcho(mark, 20*time.Millisecond) {
		t.Fatal("WaitEcho reported an echo that never came")
	}
	if d := time.Since(start); d < 15*time.Millisecond || d > time.Second {
		t.Fatalf("WaitEcho waited %v; want about its 20ms limit", d)
	}
}
