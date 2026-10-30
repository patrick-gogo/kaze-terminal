package ptyx_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
)

// readUntil accumulates output until want appears or the timeout hits.
// ConPTY wraps and decorates output, so callers match with Contains.
func readUntil(t *testing.T, s ptyx.Session, want string, d time.Duration) string {
	t.Helper()
	var mu sync.Mutex
	var sb strings.Builder
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := s.Read(buf)
			if n > 0 {
				mu.Lock()
				sb.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := sb.String()
		mu.Unlock()
		if strings.Contains(got, want) {
			return got
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("timed out waiting for %q; got %q", want, sb.String())
	return ""
}

func TestEchoThroughPTY(t *testing.T) {
	argv := []string{"sh", "-c", "echo kaze-ok"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/c", "echo kaze-ok"}
	}
	s, err := ptyx.Start(argv, 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	out := readUntil(t, s, "kaze-ok", 5*time.Second)
	if !strings.Contains(out, "kaze-ok") {
		t.Fatalf("%q", out)
	}
}

func TestEnvIsPassedThrough(t *testing.T) {
	argv := []string{"sh", "-c", "echo tab=$KAZE_TAB_ID"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/c", "echo tab=%KAZE_TAB_ID%"}
	}
	s, err := ptyx.Start(argv, 80, 24, append(os.Environ(), "KAZE_TAB_ID=7"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	readUntil(t, s, "tab=7", 5*time.Second)
}

func TestWaitReturnsAfterExit(t *testing.T) {
	argv := []string{"sh", "-c", "exit 3"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/c", "exit 3"}
	}
	s, err := ptyx.Start(argv, 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	type res struct {
		code int
		err  error
	}
	ch := make(chan res, 1)
	go func() {
		c, err := s.Wait()
		ch <- res{c, err}
	}()
	select {
	case r := <-ch:
		if r.code != 3 {
			t.Fatalf("exit code = %d, err = %v; want 3", r.code, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Wait did not return within 5s")
	}
}

func TestCloseDoesNotHangWhileChildRuns(t *testing.T) {
	argv := []string{"sh", "-c", "sleep 30"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/k"}
	}
	s, err := ptyx.Start(argv, 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		s.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return within 2s")
	}
}

func TestResize(t *testing.T) {
	argv := []string{"sh", "-c", "sleep 30"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/k"}
	}
	s, err := ptyx.Start(argv, 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Resize(100, 30); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultShellIsNotEmpty(t *testing.T) {
	if len(ptyx.DefaultShell()) == 0 {
		t.Fatal("empty")
	}
}

func sleeper() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd.exe", "/k"}
	}
	return []string{"sh", "-c", "sleep 30"}
}

func TestInvalidSizeRejected(t *testing.T) {
	for _, sz := range [][2]int{{0, 24}, {80, 0}, {-1, 24}, {80, -5}, {65536, 24}, {80, 65536}} {
		if s, err := ptyx.Start(sleeper(), sz[0], sz[1], os.Environ(), ""); err == nil {
			s.Close()
			t.Fatalf("Start(%v) accepted an invalid size", sz)
		}
	}
	s, err := ptyx.Start(sleeper(), 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, sz := range [][2]int{{0, 24}, {80, 0}, {-1, 24}, {80, 65536}, {65536, 24}} {
		if err := s.Resize(sz[0], sz[1]); !errors.Is(err, ptyx.ErrInvalidSize) {
			t.Fatalf("Resize(%v) = %v, want ErrInvalidSize", sz, err)
		}
	}
	if err := s.Resize(65535, 65535); err != nil && errors.Is(err, ptyx.ErrInvalidSize) {
		t.Fatalf("max size rejected: %v", err)
	}
}

func TestResizeAfterCloseFailsCleanly(t *testing.T) {
	s, err := ptyx.Start(sleeper(), 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := s.Resize(100, 30); !errors.Is(err, ptyx.ErrClosed) {
		t.Fatalf("Resize after Close = %v, want ErrClosed", err)
	}
}

func TestWaitAfterCloseReturnsPromptly(t *testing.T) {
	s, err := ptyx.Start(sleeper(), 80, 24, os.Environ(), "")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Wait hung after Close")
	}
}

// A bare program name is found on PATH even when the working directory is set;
// go-pty would otherwise look for it inside dir.
func TestBareProgramNameWithWorkingDir(t *testing.T) {
	argv := []string{"sh", "-c", "echo kaze-dir"}
	if runtime.GOOS == "windows" {
		argv = []string{"cmd.exe", "/c", "echo kaze-dir"}
	}
	s, err := ptyx.Start(argv, 80, 24, os.Environ(), t.TempDir())
	if err != nil {
		t.Fatalf("Start with a working dir: %v", err)
	}
	defer s.Close()
	readUntil(t, s, "kaze-dir", 5*time.Second)
}

func TestResolveFindsTheProgramOnPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	dir, base := filepath.Split(exe)
	t.Setenv("PATH", dir)
	name := base // ptyx.test here; on Windows ptyx.test.exe, found as ptyx.test
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	got := ptyx.Resolve([]string{name, "-x"})
	if !strings.EqualFold(filepath.Clean(got[0]), filepath.Clean(exe)) || got[1] != "-x" {
		t.Fatalf("Resolve(%q) = %v, want %s", name, got, exe)
	}
	if got := ptyx.Resolve([]string{"kaze-no-such-program"}); got[0] != "kaze-no-such-program" {
		t.Fatalf("a missing program changed to %v; Start must report it", got)
	}
	if got := ptyx.Resolve(nil); len(got) != 0 {
		t.Fatalf("Resolve(nil) = %v", got)
	}
}
