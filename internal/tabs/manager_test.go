package tabs_test

import (
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// fakePTY feeds output through a pipe and records env, writes and resizes.
type fakePTY struct {
	outR *io.PipeReader
	outW *io.PipeWriter

	mu     sync.Mutex
	sizes  [][2]int
	closed atomic.Bool
	exited chan struct{}
	once   sync.Once
}

func newFake() *fakePTY {
	r, w := io.Pipe()
	return &fakePTY{outR: r, outW: w, exited: make(chan struct{})}
}

func (f *fakePTY) Read(b []byte) (int, error)  { return f.outR.Read(b) }
func (f *fakePTY) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakePTY) Resize(cols, rows int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sizes = append(f.sizes, [2]int{cols, rows})
	return nil
}
func (f *fakePTY) Close() error {
	f.closed.Store(true)
	f.exit()
	return nil
}
func (f *fakePTY) Wait() (int, error) { <-f.exited; return 0, nil }
func (f *fakePTY) exit() {
	f.once.Do(func() { f.outW.Close(); close(f.exited) })
}
func (f *fakePTY) lastSize() [2]int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sizes) == 0 {
		return [2]int{}
	}
	return f.sizes[len(f.sizes)-1]
}

// starter records every env it is given and every fake it hands out.
type starter struct {
	mu    sync.Mutex
	envs  [][]string
	dirs  []string
	fakes []*fakePTY
}

func (s *starter) start(env []string, dir string) (ptyx.Session, error) {
	f := newFake()
	s.mu.Lock()
	s.envs = append(s.envs, env)
	s.dirs = append(s.dirs, dir)
	s.fakes = append(s.fakes, f)
	s.mu.Unlock()
	return f, nil
}

func (s *starter) closeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.fakes {
		f.exit()
	}
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

func newManager(t *testing.T) (*tabs.Manager, *starter) {
	st := &starter{}
	m := tabs.NewManager(st.start, 80, 24, func() {})
	t.Cleanup(st.closeAll)
	return m, st
}

func mustNew(t *testing.T, m *tabs.Manager) *tabs.Tab {
	t.Helper()
	tab, err := m.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return tab
}

func ids(m *tabs.Manager) []tabs.TabID {
	var out []tabs.TabID
	for _, t := range m.Tabs() {
		out = append(out, t.ID)
	}
	return out
}

func countPrefix(env []string, prefix string) (n int, last string) {
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			n++
			last = e
		}
	}
	return n, last
}

func TestNewTabSetsTabIDEnv(t *testing.T) {
	t.Setenv("KAZE_TAB_ID", "99") // a kaze started from inside a kaze tab must not pass this on
	m, st := newManager(t)
	mustNew(t, m)
	mustNew(t, m)
	for i, want := range []string{"KAZE_TAB_ID=1", "KAZE_TAB_ID=2"} {
		n, got := countPrefix(st.envs[i], "KAZE_TAB_ID=")
		if n != 1 || got != want {
			t.Errorf("tab %d env: %d KAZE_TAB_ID entries, last %q; want exactly %q", i+1, n, got, want)
		}
		if n, _ := countPrefix(st.envs[i], "PATH="); n == 0 {
			if n2, _ := countPrefix(st.envs[i], "Path="); n2 == 0 {
				t.Errorf("tab %d env lost the parent environment (no PATH)", i+1)
			}
		}
	}
}

func TestClosingActiveTabActivatesNeighbor(t *testing.T) {
	m, _ := newManager(t)
	mustNew(t, m)
	two := mustNew(t, m)
	mustNew(t, m)
	m.SetActive(two.ID)
	if a := m.Active(); a == nil || a.ID != 2 {
		t.Fatalf("active = %v, want 2", a)
	}
	m.Close(2)
	if a := m.Active(); a == nil || a.ID != 3 {
		t.Fatalf("after Close(2) active = %v, want 3", a)
	}
	m.Close(3)
	if a := m.Active(); a == nil || a.ID != 1 {
		t.Fatalf("after Close(3) active = %v, want 1", a)
	}
	if got := ids(m); len(got) != 1 || got[0] != 1 {
		t.Fatalf("tabs = %v, want [1]", got)
	}
}

func TestClosingInactiveTabKeepsActive(t *testing.T) {
	m, _ := newManager(t)
	mustNew(t, m)
	mustNew(t, m)
	mustNew(t, m) // New activates the new tab
	m.Close(1)
	if a := m.Active(); a == nil || a.ID != 3 {
		t.Fatalf("active = %v, want 3", a)
	}
}

func TestNextPrevWrap(t *testing.T) {
	m, _ := newManager(t)
	mustNew(t, m)
	mustNew(t, m)
	mustNew(t, m)
	m.Next()
	if m.Active().ID != 1 {
		t.Fatalf("Next from last: active %d, want 1", m.Active().ID)
	}
	m.Prev()
	if m.Active().ID != 3 {
		t.Fatalf("Prev from first: active %d, want 3", m.Active().ID)
	}
}

func TestTabIDsAreNeverReused(t *testing.T) {
	m, _ := newManager(t)
	one := mustNew(t, m)
	m.Close(one.ID)
	two := mustNew(t, m)
	if two.ID != 2 {
		t.Fatalf("second ID = %d, want 2", two.ID)
	}
}

func TestTitleFollowsOSC(t *testing.T) {
	m, st := newManager(t)
	mustNew(t, m)
	go st.fakes[0].outW.Write([]byte("\x1b]0;claude\x07"))
	waitFor(t, "title", func() bool { return m.Tabs()[0].Title == "claude" })
}

func TestExitedFollowsProcess(t *testing.T) {
	m, st := newManager(t)
	mustNew(t, m)
	st.fakes[0].exit()
	waitFor(t, "exited", func() bool { return m.Tabs()[0].Exited })
	if len(m.Tabs()) != 1 {
		t.Fatal("an exited tab must stay open until closed")
	}
}

func TestResizeAllResizesEveryPTY(t *testing.T) {
	m, st := newManager(t)
	mustNew(t, m)
	mustNew(t, m)
	m.ResizeAll(100, 30)
	for i, f := range st.fakes {
		if got := f.lastSize(); got != [2]int{100, 30} {
			t.Errorf("pty %d size %v, want [100 30]", i, got)
		}
	}
	if c, r := m.Size(); c != 100 || r != 30 {
		t.Errorf("Size() = %d,%d, want 100,30", c, r)
	}
	// A tab opened later starts at the new size.
	mustNew(t, m)
	waitFor(t, "new tab size", func() bool { return st.fakes[2].lastSize() == [2]int{100, 30} })
}

func TestClosingLastTabClosesWindow(t *testing.T) {
	m, st := newManager(t)
	one := mustNew(t, m)
	if m.Empty() {
		t.Fatal("Empty with one tab")
	}
	m.Close(one.ID)
	if !m.Empty() || m.Active() != nil || len(m.Tabs()) != 0 {
		t.Fatalf("after closing the last tab: Empty=%v Active=%v Tabs=%d", m.Empty(), m.Active(), len(m.Tabs()))
	}
	waitFor(t, "pty closed", func() bool { return st.fakes[0].closed.Load() })
}

// Only the active tab asks for frames; a background tab asks only when its title or exit state changes.
func TestBackgroundTabOutputRequestsNoFrame(t *testing.T) {
	st := &starter{}
	var frames atomic.Int64
	m := tabs.NewManager(st.start, 80, 24, func() { frames.Add(1) })
	t.Cleanup(st.closeAll)
	mustNew(t, m)
	mustNew(t, m) // active
	bg := st.fakes[0]

	// Output in the active tab asks for a frame.
	st.fakes[1].outW.Write([]byte("x"))
	waitFor(t, "active frame", func() bool { return frames.Load() > 0 })

	before := frames.Load()
	for i := 0; i < 20; i++ {
		bg.outW.Write([]byte("hello\r\n"))
	}
	bg.outW.Write([]byte("\x1b]0;bgtitle\x07"))
	waitFor(t, "background title", func() bool { return m.Tabs()[0].Title == "bgtitle" })
	time.Sleep(20 * time.Millisecond) // let any stray notifications land
	got := frames.Load() - before
	if got > 1 {
		t.Fatalf("background output requested %d frames, want at most 1 (the title change)", got)
	}
	if got != 1 {
		t.Fatalf("background title change requested %d frames, want 1", got)
	}
}

func TestStartErrorAddsNoTab(t *testing.T) {
	m := tabs.NewManager(func([]string, string) (ptyx.Session, error) { return nil, io.ErrUnexpectedEOF }, 80, 24, nil)
	if _, err := m.New(); err == nil {
		t.Fatal("New returned no error")
	}
	if !m.Empty() {
		t.Fatal("a failed start left a tab")
	}
	two, err := m.New()
	if err == nil || two != nil {
		t.Fatal("expected error again")
	}
}

func TestSetScrollbackCapsNewTabsHistory(t *testing.T) {
	m, st := newManager(t)
	m.SetScrollback(3)
	tab := mustNew(t, m)
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("line\r\n")
	}
	b.WriteString("end")
	go st.fakes[0].outW.Write([]byte(b.String()))
	s := tab.Session
	waitFor(t, "output applied", func() bool {
		s.Lock()
		defer s.Unlock()
		col, _ := s.Screen().Cursor()
		return col == 3
	})
	s.Lock()
	n := s.Screen().ScrollbackLen()
	s.Unlock()
	if n != 3 {
		t.Fatalf("history holds %d lines, want the configured 3", n)
	}
}

// slowClose takes a while to release its PTY, as ConPTY can.
type slowClose struct {
	*fakePTY
	released atomic.Bool
}

func (s *slowClose) Close() error {
	time.Sleep(300 * time.Millisecond)
	s.released.Store(true)
	return s.fakePTY.Close()
}

func TestCloseAllWaitsForACloseInProgress(t *testing.T) {
	var slow *slowClose
	m := tabs.NewManager(func([]string, string) (ptyx.Session, error) {
		slow = &slowClose{fakePTY: newFake()}
		return slow, nil
	}, 80, 24, nil)
	tab := mustNew(t, m)
	m.Close(tab.ID) // closes in the background
	start := time.Now()
	m.CloseAll()
	if !slow.released.Load() {
		t.Fatal("CloseAll returned while a tab's shell was still being closed")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("CloseAll took %v; its wait must be bounded", d)
	}
}

func countPrefixFold(env []string, prefix string) (n int, last string) {
	for _, e := range env {
		if len(e) >= len(prefix) && strings.EqualFold(e[:len(prefix)], prefix) {
			n++
			last = e
		}
	}
	return n, last
}

// Shells and TUIs pick features from these, so a kaze started from Windows
// Terminal must not claim to be Windows Terminal.
func TestNewTabSetsKazeTerminalIdentity(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "vscode")
	t.Setenv("TERM_PROGRAM_VERSION", "1.2.3")
	t.Setenv("WT_SESSION", "abc")
	t.Setenv("WT_PROFILE_ID", "{xyz}")
	t.Setenv("TERM", "dumb")
	t.Setenv("COLORTERM", "")
	m, st := newManager(t)
	mustNew(t, m)
	env := st.envs[0]
	for _, gone := range []string{"TERM_PROGRAM_VERSION=", "WT_SESSION=", "WT_PROFILE_ID="} {
		if n, got := countPrefixFold(env, gone); n != 0 {
			t.Errorf("inherited %s passed on: %q", gone, got)
		}
	}
	for _, want := range []string{"TERM=xterm-256color", "COLORTERM=truecolor", "TERM_PROGRAM=kaze"} {
		name := want[:strings.IndexByte(want, '=')+1]
		if n, got := countPrefixFold(env, name); n != 1 || got != want {
			t.Errorf("%d %s entries, last %q; want exactly %q", n, name, got, want)
		}
	}
}

func TestNewTabEnvHasNotifyVars(t *testing.T) {
	t.Setenv("KAZE_NOTIFY", "stale")
	t.Setenv("kaze_notify_token", "stale") // inherited copies must go, in any case
	m, st := newManager(t)
	m.SetNotify("127.0.0.1:5555", "abc123")
	mustNew(t, m)
	for _, want := range []string{"KAZE_NOTIFY=127.0.0.1:5555", "KAZE_NOTIFY_TOKEN=abc123"} {
		name := want[:strings.Index(want, "=")+1]
		n, got := countPrefix(st.envs[0], name)
		if n != 1 || got != want {
			t.Errorf("%d %s entries, last %q; want exactly %q", n, name, got, want)
		}
	}
	for _, e := range st.envs[0] {
		if strings.Contains(e, "stale") {
			t.Errorf("inherited %q leaked into the tab env", e)
		}
	}
}

func TestHasReportsOpenTabsOnly(t *testing.T) {
	m, _ := newManager(t)
	a := mustNew(t, m)
	b := mustNew(t, m)
	m.Close(a.ID)
	if m.Has(a.ID) || !m.Has(b.ID) || m.Has(0) || m.Has(99) {
		t.Fatalf("Has: closed %v open %v zero %v unknown %v", m.Has(a.ID), m.Has(b.ID), m.Has(0), m.Has(99))
	}
}

func TestLookupFindsOpenTabsOnly(t *testing.T) {
	m, _ := newManager(t)
	one := mustNew(t, m)
	if got := m.Lookup(one.ID); got != one {
		t.Fatalf("Lookup(%d) = %v", one.ID, got)
	}
	m.Close(one.ID)
	if got := m.Lookup(one.ID); got != nil {
		t.Fatalf("closed tab found: %v", got)
	}
}

// hungClose never releases its PTY until the test ends, as a stuck ConPTY can.
type hungClose struct {
	*fakePTY
	release chan struct{}
}

func (h *hungClose) Close() error { <-h.release; return h.fakePTY.Close() }

func TestCloseAllReportsWhetherEveryShellWasReleased(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	hang := true
	m := tabs.NewManager(func([]string, string) (ptyx.Session, error) {
		if hang {
			return &hungClose{fakePTY: newFake(), release: release}, nil
		}
		return newFake(), nil
	}, 80, 24, nil)
	mustNew(t, m)
	if m.CloseAllReleased() {
		t.Fatal("a hung shell was reported released")
	}
	hang = false
	mustNew(t, m)
	if !m.CloseAllReleased() {
		t.Fatal("a closed shell was reported hung")
	}
}
