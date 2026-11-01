package term

import (
	"sync"
	"testing"
	"time"
)

func TestRejectedInputDoesNotAllocate(t *testing.T) {
	s := &Session{writes: make(chan []byte, writeQueueLen)}
	for range writeQueueLen {
		if !s.enqueue([]byte("x")) {
			t.Fatal("early rejection")
		}
	}
	p := make([]byte, 64<<10)
	if n := testing.AllocsPerRun(5, func() {
		if s.enqueue(p) {
			t.Fatal("full queue accepted input")
		}
	}); n != 0 {
		t.Fatalf("rejected input allocated %v times", n)
	}
}

func TestQueueByteReservationAndConcurrentProducers(t *testing.T) {
	s := &Session{writes: make(chan []byte, writeQueueLen)}
	// Simulate a large blocked write without allocating its payload.
	s.writeBytes = writeQueueBytes - 8
	if s.enqueue(make([]byte, 9)) {
		t.Fatal("byte budget exceeded")
	}
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() { defer wg.Done(); s.enqueue([]byte("xx")) }()
	}
	wg.Wait()
	if len(s.writes) != 4 || s.writeBytes != writeQueueBytes {
		t.Fatalf("queued %d bytes %d", len(s.writes), s.writeBytes)
	}
}

func TestMaximumPasteFitsByteReservation(t *testing.T) {
	s := &Session{}
	if !s.reserveWrite((64 << 20) + 12) {
		t.Fatal("maximum bracketed paste cannot fit")
	}
	if s.reserveWrite(4096) {
		t.Fatal("aggregate byte cap exceeded")
	}
}

type budgetPTY struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *budgetPTY) Read([]byte) (int, error) { return 0, nil }
func (p *budgetPTY) Write(b []byte) (int, error) {
	p.entered <- struct{}{}
	<-p.release
	return len(b), nil
}
func (p *budgetPTY) Resize(int, int) error { return nil }
func (p *budgetPTY) Wait() (int, error)    { return 0, nil }
func (p *budgetPTY) Close() error          { p.once.Do(func() { close(p.release) }); return nil }

func TestQueueCountsActiveWriteAndClearsOnClose(t *testing.T) {
	p := &budgetPTY{entered: make(chan struct{}, 1), release: make(chan struct{})}
	s := &Session{pty: p, writes: make(chan []byte, writeQueueLen), done: make(chan struct{})}
	done := make(chan struct{})
	go func() { s.writeLoop(); close(done) }()
	defer s.Close()
	s.Write([]byte("one"))
	select {
	case <-p.entered:
	case <-time.After(time.Second):
		t.Fatal("write never started")
	}
	s.Write([]byte("two"))
	s.writeMu.Lock()
	bytes := s.writeBytes
	s.writeMu.Unlock()
	if bytes != 6 {
		t.Fatalf("active input not counted: %d", bytes)
	}
	s.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("writer did not stop")
	}
	if len(s.writes) != 0 || s.writeBytes != 0 || s.writeChunks != 0 {
		t.Fatal("closed queue retained input")
	}
}

func BenchmarkRejectedInput(b *testing.B) {
	s := &Session{writes: make(chan []byte, writeQueueLen)}
	for range writeQueueLen {
		s.enqueue([]byte("x"))
	}
	p := make([]byte, 64<<10)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.enqueue(p)
	}
}
