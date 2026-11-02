package term_test

import (
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/term"
)

// fakePTY feeds output through a pipe and records what is written to it and every Resize.
type fakePTY struct {
	outR *io.PipeReader
	outW *io.PipeWriter

	mu      sync.Mutex
	written bytes.Buffer
	sizes   [][2]int
	block   chan struct{} // when non-nil, Write blocks until it is closed

	exited   chan struct{}
	code     int
	exitOnce sync.Once
	closed   atomic.Bool
}

func newFake() *fakePTY {
	r, w := io.Pipe()
	return &fakePTY{outR: r, outW: w, exited: make(chan struct{})}
}

func (f *fakePTY) Read(b []byte) (int, error) { return f.outR.Read(b) }

func (f *fakePTY) Write(b []byte) (int, error) {
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.Write(b)
}

func (f *fakePTY) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, [2]int{cols, rows})
	return nil
}

func (f *fakePTY) Close() error {
	if f.closed.CompareAndSwap(false, true) {
		f.exit(-1)
	}
	return nil
}

func (f *fakePTY) Wait() (int, error) {
	<-f.exited
	return f.code, nil
}

// exit ends the process: output EOF, then Wait returns code.
func (f *fakePTY) exit(code int) {
	f.exitOnce.Do(func() {
		f.code = code
		f.outW.Close()
		close(f.exited)
	})
}

func (f *fakePTY) writtenString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.written.String()
}

func (f *fakePTY) lastSize() ([2]int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sizes) == 0 {
		return [2]int{}, 0
	}
	return f.sizes[len(f.sizes)-1], len(f.sizes)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func screenRow(s *term.Session, r int) string {
	s.Lock()
	defer s.Unlock()
	return row(s.Screen(), r)
}

func TestOutputReachesScreenAndCallsOnChange(t *testing.T) {
	f := newFake()
	var changes atomic.Int32
	s := term.NewSession(f, 20, 5, func() { changes.Add(1) })
	defer s.Close()
	f.outW.Write([]byte("hi"))
	waitFor(t, "onChange", func() bool { return changes.Load() > 0 })
	if got := screenRow(s, 0); got != "hi" {
		t.Fatalf("row 0 = %q", got)
	}
}

func TestOnChangeMayLockTheSession(t *testing.T) {
	f := newFake()
	var s *term.Session
	var seen atomic.Bool
	ready := make(chan struct{})
	s = term.NewSession(f, 20, 5, func() {
		<-ready
		s.Lock() // the UI reads the screen from onChange; that must not deadlock
		seen.Store(true)
		s.Unlock()
	})
	close(ready)
	defer s.Close()
	f.outW.Write([]byte("x"))
	waitFor(t, "onChange to lock", seen.Load)
}

func TestRepliesAreWrittenBackToPTY(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 5, func() {})
	defer s.Close()
	f.outW.Write([]byte("\x1b[6n"))
	waitFor(t, "cursor report", func() bool { return f.writtenString() == "\x1b[1;1R" })
}

func TestWriteReachesPTY(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 5, func() {})
	defer s.Close()
	buf := []byte("ls\r")
	s.Write(buf)
	buf[0] = 'X' // the caller may reuse its buffer straight away
	waitFor(t, "input", func() bool { return f.writtenString() == "ls\r" })
}

func TestWriteDoesNotBlockOnStuckPTY(t *testing.T) {
	f := newFake()
	f.block = make(chan struct{})
	defer close(f.block)
	s := term.NewSession(f, 20, 5, func() {})
	defer s.Close()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 10000; i++ {
			s.Write([]byte("a"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Write blocked on a stuck PTY")
	}
}

func TestResizeDuringStreamingDoesNotPanic(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 80, 24, func() {})
	defer s.Close()

	wrote := make(chan struct{})
	go func() {
		defer close(wrote)
		for i := 0; i < 10000; i++ {
			fmt.Fprintf(f.outW, "line %d 日本語のテキスト mixed ASCII\r\n", i)
		}
	}()
	rng := rand.New(rand.NewSource(1))
	var cols, rows int
	for i := 0; i < 200; i++ {
		cols, rows = 1+rng.Intn(300), 1+rng.Intn(100)
		s.Resize(cols, rows)
	}
	<-wrote

	f.outW.Write([]byte("end"))
	waitFor(t, "final output", func() bool {
		s.Lock()
		defer s.Unlock()
		col, _ := s.Screen().Cursor()
		return col > 0 || cols < 4
	})
	s.Lock()
	gc, gr := s.Screen().Size()
	col, r := s.Screen().Cursor()
	s.Unlock()
	if gc != cols || gr != rows {
		t.Fatalf("screen %dx%d, want %dx%d", gc, gr, cols, rows)
	}
	if col < 0 || col >= cols || r < 0 || r >= rows {
		t.Fatalf("cursor %d,%d outside %dx%d", col, r, cols, rows)
	}
	if last, _ := f.lastSize(); last != [2]int{cols, rows} {
		t.Fatalf("pty last resize %v, want %dx%d", last, cols, rows)
	}
}

func TestResizeClampsToAtLeastOneByOne(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 5, func() {})
	defer s.Close()
	s.Resize(0, -1)
	s.Lock()
	c, r := s.Screen().Size()
	s.Unlock()
	if c != 1 || r != 1 {
		t.Fatalf("screen %dx%d", c, r)
	}
	if last, _ := f.lastSize(); last != [2]int{1, 1} {
		t.Fatalf("pty resize %v", last)
	}
}

func TestProcessExitShowsMarkerAndKeepsSessionReadable(t *testing.T) {
	f := newFake()
	var changes atomic.Int32
	s := term.NewSession(f, 40, 5, func() { changes.Add(1) })
	defer s.Close()
	if done, _ := s.Exited(); done {
		t.Fatal("exited before the process did")
	}
	f.outW.Write([]byte("bye"))
	f.exit(0)
	waitFor(t, "exit", func() bool { done, _ := s.Exited(); return done })
	if _, code := s.Exited(); code != 0 {
		t.Fatalf("code = %d", code)
	}
	waitFor(t, "marker", func() bool { return screenRow(s, 1) == "[process exited 0]" })
	if got := screenRow(s, 0); got != "bye" {
		t.Fatalf("row 0 = %q", got)
	}
	if changes.Load() == 0 {
		t.Fatal("onChange not called")
	}
	s.Write([]byte("ignored"))
	time.Sleep(20 * time.Millisecond)
	if got := f.writtenString(); got != "" {
		t.Fatalf("write after exit reached the pty: %q", got)
	}
}

func TestExitCodeIsReported(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 40, 5, func() {})
	defer s.Close()
	f.exit(3)
	waitFor(t, "marker", func() bool { return strings.Contains(screenRow(s, 1)+screenRow(s, 0), "[process exited 3]") })
	if done, code := s.Exited(); !done || code != 3 {
		t.Fatalf("Exited = %v, %d", done, code)
	}
}

func TestOtherSessionsUnaffectedByOneExiting(t *testing.T) {
	f1, f2 := newFake(), newFake()
	s1 := term.NewSession(f1, 40, 5, func() {})
	s2 := term.NewSession(f2, 40, 5, func() {})
	defer s1.Close()
	defer s2.Close()
	f1.exit(0)
	waitFor(t, "first exit", func() bool { done, _ := s1.Exited(); return done })
	f2.outW.Write([]byte("still here"))
	waitFor(t, "second output", func() bool { return screenRow(s2, 0) == "still here" })
	if done, _ := s2.Exited(); done {
		t.Fatal("second session marked exited")
	}
}

func TestCloseIsIdempotentAndStopsWrites(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 20, 5, func() {})
	s.Close()
	s.Close()
	if !f.closed.Load() {
		t.Fatal("pty not closed")
	}
	s.Write([]byte("x")) // must not panic
	s.Resize(10, 10)     // must not panic
}

func TestOnChangeNeverRunsConcurrentlyAcrossExit(t *testing.T) {
	f := newFake()
	var inflight, maxInflight, calls atomic.Int32
	s := term.NewSession(f, 20, 5, func() {
		n := inflight.Add(1)
		for {
			m := maxInflight.Load()
			if n <= m || maxInflight.CompareAndSwap(m, n) {
				break
			}
		}
		if calls.Add(1) == 1 {
			time.Sleep(700 * time.Millisecond) // outlasts the exit drain, so the exit notify would overlap
		}
		inflight.Add(-1)
	})
	defer s.Close()
	f.outW.Write([]byte("hi"))
	f.exit(0)
	waitFor(t, "exit marker", func() bool { e, _ := s.Exited(); return e })
	waitFor(t, "second onChange", func() bool { return calls.Load() >= 2 })
	waitFor(t, "onChange idle", func() bool { return inflight.Load() == 0 })
	if m := maxInflight.Load(); m != 1 {
		t.Fatalf("max concurrent onChange = %d, want 1", m)
	}
}

// Only bounded sizes here: a broken clamp must not be able to allocate a huge grid.
func TestSessionClampsOversizedDimensions(t *testing.T) {
	for _, sz := range [][2]int{{4097, 1}, {1, 4097}} {
		f := newFake()
		s := term.NewSession(f, sz[0], sz[1], func() {})
		s.Lock()
		cols, rows := s.Screen().Size()
		s.Unlock()
		if cols > 4096 || rows > 4096 {
			t.Fatalf("NewSession(%v) size %dx%d, want <= 4096", sz, cols, rows)
		}
		s.Resize(2, 2)
		s.Resize(sz[0], sz[1])
		s.Lock()
		cols, rows = s.Screen().Size()
		s.Unlock()
		if cols > 4096 || rows > 4096 {
			t.Fatalf("Resize(%v) size %dx%d, want <= 4096", sz, cols, rows)
		}
		if last, _ := f.lastSize(); last[0] > 4096 || last[1] > 4096 {
			t.Fatalf("PTY resized to %v", last)
		}
		s.Close()
	}
}

func TestNewSessionSizesThePTY(t *testing.T) {
	f := newFake()
	s := term.NewSession(f, 4097, 1, func() {})
	defer s.Close()
	if last, n := f.lastSize(); n != 1 || last != [2]int{4096, 1} {
		t.Fatalf("PTY sizes = %v (%d calls), want one call with 4096x1", last, n)
	}
}

type rootedPTY struct{ *fakePTY }

func (*rootedPTY) RootPID() int { return 123 }
func TestSessionRootPIDOptionalAndClosed(t *testing.T) {
	for _, rooted := range []bool{false, true} {
		f := newFake()
		var s *term.Session
		if rooted {
			s = term.NewSession(&rootedPTY{f}, 10, 2, nil)
		} else {
			s = term.NewSession(f, 10, 2, nil)
		}
		want := 0
		if rooted {
			want = 123
		}
		if s.RootPID() != want {
			t.Fatal("optional root interface")
		}
		s.Close()
		if s.RootPID() != 0 {
			t.Fatal("closed session root")
		}
	}
}
