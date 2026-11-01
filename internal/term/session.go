package term

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/screen"
	"github.com/patrick-gogo/kaze-terminal/internal/vt"
)

const (
	readBufSize = 64 * 1024
	// writeQueueLen bounds queued input chunks; past it, input is dropped rather than blocking the UI.
	writeQueueLen = 1024
	// Include the active write. One maximum clipboard paste and its markers fit.
	writeQueueBytes = (64 << 20) + 4096
	// maxSize caps each dimension so a bad UI measurement can't allocate billions of cells.
	maxSize = 4096
	// maxCells caps the whole grid; 4096x4096 alone would be 16M cells.
	maxCells = 1 << 22
)

// drainGrace is how long to wait for remaining output after the process exits.
// ConPTY keeps the output pipe open until it is closed, so the reader may never see EOF on its own.
var drainGrace = 250 * time.Millisecond

// clampSize limits each dimension to 1..maxSize, then shrinks rows until the grid fits maxCells.
func clampSize(cols, rows int) (int, int) {
	cols, rows = min(max(cols, 1), maxSize), min(max(rows, 1), maxSize)
	if cols*rows > maxCells {
		rows = maxCells / cols
	}
	return cols, rows
}

// Session is one tab: a shell in a PTY whose output a reader goroutine applies to a screen.
type Session struct {
	mu     sync.Mutex // guards scr, app and parser
	scr    *screen.Screen
	app    *Applier
	parser *vt.Parser

	pty      ptyx.Session
	onChange func()
	notifyCh chan struct{} // wakes the one goroutine allowed to call onChange

	writes                  chan []byte
	writeMu                 sync.Mutex // guards reservations; copying and PTY I/O happen outside it
	writeBytes, writeChunks int
	done                    chan struct{} // closed by Close
	closeOnce               sync.Once
	closed                  atomic.Bool

	resizeMu sync.Mutex // keeps the PTY's last size equal to the screen's

	exited   atomic.Bool
	exitCode atomic.Int64

	// For WaitEcho: writes made through Write, output chunks applied, and a
	// wake for the one UI goroutine that waits.
	inSeq, outSeq atomic.Uint64
	outSig        chan struct{}
}

// NewSession starts the reader, writer and notifier goroutines for p. onChange is called
// only from the notifier goroutine, without the lock held, after output is applied or the
// process exits; bursts may coalesce into one call.
func NewSession(p ptyx.Session, cols, rows int, onChange func()) *Session {
	return NewSessionOpts(p, cols, rows, Opts{}, onChange)
}

// Opts are a new session's optional settings.
type Opts struct {
	Scrollback int // history cap in lines; 0 keeps the screen's default
	// Prefeed runs before the PTY is read, so what it writes always lands before the shell's output.
	Prefeed func(*Session)
}

// NewSessionOpts is NewSession with options.
func NewSessionOpts(p ptyx.Session, cols, rows int, o Opts, onChange func()) *Session {
	if onChange == nil {
		onChange = func() {}
	}
	cols, rows = clampSize(cols, rows)
	scr := screen.New(cols, rows)
	if o.Scrollback > 0 {
		scr = screen.NewWithScrollback(cols, rows, o.Scrollback)
	}
	s := &Session{
		scr:      scr,
		pty:      p,
		onChange: onChange,
		notifyCh: make(chan struct{}, 1),
		outSig:   make(chan struct{}, 1),
		writes:   make(chan []byte, writeQueueLen),
		done:     make(chan struct{}),
	}
	// Replies are produced under the lock; queueing them keeps a slow PTY from stalling the reader.
	s.app = NewApplier(s.scr, replyWriter{s})
	s.parser = vt.NewParser(s.app)

	if o.Prefeed != nil {
		o.Prefeed(s) // nothing reads the PTY yet, so the shell's output lands after this
	}
	readDone := make(chan struct{})
	_ = p.Resize(cols, rows) // a fresh PTY may start at some other size; ignore a dead one
	go s.readLoop(readDone)
	go s.writeLoop()
	go s.notifyLoop()
	go s.waitLoop(readDone)
	return s
}

func (s *Session) Lock()   { s.mu.Lock() }
func (s *Session) Unlock() { s.mu.Unlock() }

// Screen is only safe to use while the session is locked.
func (s *Session) Screen() *screen.Screen { return s.scr }

// Applier is only safe to use while the session is locked.
func (s *Session) Applier() *Applier { return s.app }

// Write queues a whole input message without waiting for room and reports rejection.
// Accepted writes advance echo tracking; closed sessions retain no new input.
func (s *Session) Write(p []byte) SubmitResult { return s.Submit(p) }

// EchoMark is where a session's input and output stood, taken before a frame's input.
type EchoMark struct{ in, out uint64 }

// EchoMark records the session's input and output so far, for WaitEcho.
func (s *Session) EchoMark() EchoMark {
	return EchoMark{in: s.inSeq.Load(), out: s.outSeq.Load()}
}

// WaitEcho waits up to max for output after mark, but only when something was
// written since mark. The frame that sends a key calls it, so the shell's echo,
// which usually lands well under a millisecond later, shows in that same frame
// rather than a whole screen refresh later. It reports whether output came.
func (s *Session) WaitEcho(mark EchoMark, max time.Duration) bool {
	if s.inSeq.Load() == mark.in {
		return false
	}
	var timer *time.Timer
	for s.outSeq.Load() == mark.out {
		if timer == nil {
			timer = time.NewTimer(max)
			defer timer.Stop()
		}
		select {
		case <-s.outSig:
		case <-timer.C:
			return s.outSeq.Load() != mark.out
		case <-s.done:
			return false
		}
	}
	return true
}

// SubmitResult says what became of a Submit.
type SubmitResult int

const (
	Accepted  SubmitResult = iota // queued for the shell
	QueueFull                     // dropped: the shell is not reading its input
	Closed                        // dropped: the process exited or the session is closed
)

func (r SubmitResult) String() string {
	switch r {
	case Accepted:
		return "accepted"
	case QueueFull:
		return "queue full"
	case Closed:
		return "closed"
	}
	return "SubmitResult(" + strconv.Itoa(int(r)) + ")"
}

// Submit is Write that reports whether the input was queued, so a caller
// sending a command once can tell a dropped line from a sent one.
func (s *Session) Submit(p []byte) SubmitResult {
	if s.exited.Load() || s.closed.Load() {
		return Closed
	}
	if len(p) == 0 {
		return Accepted
	}
	if s.enqueue(p) {
		s.inSeq.Add(1)
		return Accepted
	}
	if s.exited.Load() || s.closed.Load() {
		return Closed
	}
	return QueueFull
}

func (s *Session) reserveWrite(n int) bool {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if s.closed.Load() || s.exited.Load() || s.writeChunks >= writeQueueLen || n > writeQueueBytes-s.writeBytes {
		return false
	}
	s.writeChunks++
	s.writeBytes += n
	return true
}

func (s *Session) releaseWrite(n int) {
	s.writeMu.Lock()
	s.writeBytes -= n
	s.writeChunks--
	s.writeMu.Unlock()
}

func (s *Session) enqueue(p []byte) bool {
	if !s.reserveWrite(len(p)) {
		return false
	}
	b := append([]byte(nil), p...) // callers reuse their buffers
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if !s.closed.Load() && !s.exited.Load() {
		select {
		case s.writes <- b:
			return true
		default:
		}
	}
	s.writeBytes -= len(b)
	s.writeChunks--
	return false
}

// discardWrites releases queued payloads even if the active PTY write is stuck.
func (s *Session) discardWrites() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	for {
		select {
		case b := <-s.writes:
			s.writeBytes -= len(b)
			s.writeChunks--
		default:
			return
		}
	}
}

type replyWriter struct{ s *Session }

func (w replyWriter) Write(p []byte) (int, error) {
	w.s.enqueue(p)
	return len(p), nil
}

// Resize clamps with clampSize, resizes the screen, then the PTY.
func (s *Session) Resize(cols, rows int) {
	cols, rows = clampSize(cols, rows)
	s.resizeMu.Lock()
	defer s.resizeMu.Unlock()
	s.mu.Lock()
	s.scr.Resize(cols, rows)
	s.mu.Unlock()
	_ = s.pty.Resize(cols, rows) // fails only once the PTY is gone, when the size no longer matters
}

// Exited reports whether the process has exited and its exit code. It does not take the lock.
func (s *Session) Exited() (bool, int) {
	if !s.exited.Load() {
		return false, 0
	}
	return true, int(s.exitCode.Load())
}

// Close kills the process and releases the PTY. It is safe to call more than once.
func (s *Session) Close() {
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		close(s.done)
		s.discardWrites()
		_ = s.pty.Close()
	})
}

func (s *Session) readLoop(readDone chan struct{}) {
	defer close(readDone)
	buf := make([]byte, readBufSize)
	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.parser.Feed(buf[:n])
			s.mu.Unlock()
			s.outSeq.Add(1)
			select {
			case s.outSig <- struct{}{}:
			default:
			}
			s.notify()
		}
		if err != nil {
			return
		}
	}
}

func (s *Session) writeLoop() {
	for {
		select {
		case <-s.done:
			return
		case b := <-s.writes:
			if !s.closed.Load() && !s.exited.Load() {
				_, _ = s.pty.Write(b) // a dead PTY is reported through Wait, not here
			}
			s.releaseWrite(len(b))
		}
	}
}

// waitLoop shows the exit marker once the process is gone and its output has drained.
func (s *Session) waitLoop(readDone chan struct{}) {
	code, err := s.pty.Wait()
	if err != nil {
		code = -1
	}
	select {
	case <-readDone:
	case <-time.After(drainGrace):
		_ = s.pty.Close() // unblocks a reader that will never see EOF
		select {
		case <-readDone:
		case <-time.After(drainGrace):
		}
	}
	s.mu.Lock()
	s.exitCode.Store(int64(code))
	s.exited.Store(true)
	s.parser.Feed([]byte(fmt.Sprintf("\x1b[0m\r\n[process exited %d]", code))) // default pen, whatever the shell left set
	s.mu.Unlock()
	s.discardWrites()
	s.notify()
}

// notify asks the notifier to call onChange; a pending request already covers this one.
func (s *Session) notify() {
	select {
	case s.notifyCh <- struct{}{}:
	default:
	}
}

// notifyLoop is the only caller of onChange, so it never overlaps itself.
// It skips onChange after Close, so a closed tab does not ask for frames.
func (s *Session) notifyLoop() {
	for {
		select {
		case <-s.done:
			return
		case <-s.notifyCh:
			if !s.closed.Load() {
				s.onChange()
			}
		}
	}
}

// Foreground is the process group in front of the tab's pty (its leader's pid),
// or 0 when the platform can't tell.
func (s *Session) Foreground() int {
	if f, ok := s.pty.(ptyx.Foregrounder); ok {
		return f.Foreground()
	}
	return 0
}

// OutputSeq counts the reads of program output; it moves whenever the tab printed.
func (s *Session) OutputSeq() uint64 { return s.outSeq.Load() }
