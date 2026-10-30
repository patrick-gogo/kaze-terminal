package sound

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/assets/sounds"
)

func TestBundledChimeIsValidWav(t *testing.T) {
	b := sounds.Chime
	if len(b) < 44 || len(b) >= 64*1024 {
		t.Fatalf("size = %d", len(b))
	}
	if string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" || string(b[12:16]) != "fmt " {
		t.Fatal("bad RIFF/WAVE/fmt header")
	}
	if f := binary.LittleEndian.Uint16(b[20:]); f != 1 {
		t.Fatalf("format = %d, want PCM", f)
	}
	if ch := binary.LittleEndian.Uint16(b[22:]); ch != 1 {
		t.Fatalf("channels = %d", ch)
	}
	rate := binary.LittleEndian.Uint32(b[24:])
	if rate != 44100 {
		t.Fatalf("rate = %d", rate)
	}
	if bits := binary.LittleEndian.Uint16(b[34:]); bits != 16 {
		t.Fatalf("bits = %d", bits)
	}
	if string(b[36:40]) != "data" {
		t.Fatal("no data chunk")
	}
	n := binary.LittleEndian.Uint32(b[40:])
	if int(n) != len(b)-44 {
		t.Fatalf("data size %d, file has %d", n, len(b)-44)
	}
	dur := float64(n) / 2 / float64(rate)
	if dur < 0.6 || dur > 1.0 {
		t.Fatalf("duration = %.3fs, want a chime long enough to notice", dur)
	}
}

// The chime must be heard over music or a call: its loudest sample sits near
// full scale (-1 dBFS), not the old quiet -12 dBFS.
func TestBundledChimeIsLoudEnough(t *testing.T) {
	b := sounds.Chime
	peak := 0
	for i := 44; i+1 < len(b); i += 2 {
		v := int(int16(binary.LittleEndian.Uint16(b[i:])))
		peak = max(peak, v, -v)
	}
	if db := 20 * math.Log10(float64(peak)/32768); db < -2 || db > -0.5 {
		t.Fatalf("peak %.1f dBFS, want about -1 dBFS", db)
	}
}

func TestPlayCoalescesWithinOneSecond(t *testing.T) {
	var plays atomic.Int32
	p, _ := newWith("", func([]byte) { plays.Add(1) })
	t0 := time.Now()
	p.Play(t0)
	p.Play(t0.Add(100 * time.Millisecond))
	p.Play(t0.Add(200 * time.Millisecond))
	waitFor(t, &plays, 1)
	time.Sleep(50 * time.Millisecond)
	if n := plays.Load(); n != 1 {
		t.Fatalf("plays = %d, want 1", n)
	}
	p.Play(t0.Add(1300 * time.Millisecond))
	waitFor(t, &plays, 2)
}

func TestPlayNeverBlocks(t *testing.T) {
	p, _ := newWith("", func([]byte) { time.Sleep(5 * time.Second) })
	start := time.Now()
	p.Play(time.Now())
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("Play took %v", d)
	}
}

func TestMissingCustomFileFallsBackToBundled(t *testing.T) {
	var got []byte
	done := make(chan struct{})
	p, warn := newWith("Z:/definitely/not/here.wav", func(b []byte) { got = b; close(done) })
	if warn == "" {
		t.Fatal("want a warning for a missing file")
	}
	p.Play(time.Now())
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("no play")
	}
	if len(got) != len(sounds.Chime) {
		t.Fatalf("played %d bytes, want bundled %d", len(got), len(sounds.Chime))
	}
}

func waitFor(t *testing.T, c *atomic.Int32, n int32) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if c.Load() >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("plays = %d, want %d", c.Load(), n)
}

func writeFile(t *testing.T, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "c.wav")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBadChimeFilesFallBackWithWarning(t *testing.T) {
	wavHead := append(append([]byte{}, wavMagic()...), make([]byte, 100)...)
	cases := map[string][]byte{
		"empty":    {},
		"not wav":  []byte("this is definitely not a wav file at all"),
		"oversize": append(append([]byte{}, wavHead[:12]...), make([]byte, maxFileBytes+1)...),
		"short":    []byte("RIFF"),
	}
	for name, b := range cases {
		p, warn := newWith(writeFile(t, b), func([]byte) {})
		if warn == "" {
			t.Errorf("%s: no warning", name)
		}
		if len(p.data) != len(sounds.Chime) {
			t.Errorf("%s: did not fall back to bundled", name)
		}
	}
}

func TestValidCustomWavIsUsed(t *testing.T) {
	b := append(append([]byte{}, wavMagic()...), make([]byte, 100)...)
	p, warn := newWith(writeFile(t, b), func([]byte) {})
	if warn != "" || len(p.data) != len(b) {
		t.Fatalf("warn=%q len=%d", warn, len(p.data))
	}
}

func TestCachePathIsPerContent(t *testing.T) {
	a := cachePath("/c", []byte("one"))
	b := cachePath("/c", []byte("two"))
	if a == b || a != cachePath("/c", []byte("one")) {
		t.Fatalf("a=%s b=%s", a, b)
	}
	if !strings.HasSuffix(a, ".wav") || !strings.Contains(filepath.ToSlash(a), "kaze/chime-") {
		t.Fatalf("path = %s", a)
	}
}

func wavMagic() []byte {
	return []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'A', 'V', 'E'}
}
