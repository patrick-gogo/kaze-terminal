package ui

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"

	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// recPTY records what the shell is sent; Close ends Read and Wait.
type recPTY struct {
	r    *io.PipeReader
	w    *io.PipeWriter
	mu   sync.Mutex
	got  strings.Builder
	size [2]int // the last Resize, cols and rows
	done chan struct{}
	once sync.Once
}

func newRecPTY() *recPTY {
	r, w := io.Pipe()
	return &recPTY{r: r, w: w, done: make(chan struct{})}
}
func (p *recPTY) Read(b []byte) (int, error) { return p.r.Read(b) }
func (p *recPTY) Write(b []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got.Write(b)
	return len(b), nil
}
func (p *recPTY) Resize(cols, rows int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.size = [2]int{cols, rows}
	return nil
}
func (p *recPTY) lastSize() [2]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.size
}
func (p *recPTY) Close() error {
	p.once.Do(func() { p.w.Close(); close(p.done) })
	return nil
}
func (p *recPTY) Wait() (int, error) { <-p.done; return 0, nil }
func (p *recPTY) sent() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.got.String()
}

type recStarter struct {
	mu   sync.Mutex
	ptys []*recPTY
	fail error // returned instead of starting while set
}

func (s *recStarter) start([]string, string) (ptyx.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return nil, s.fail
	}
	p := newRecPTY()
	s.ptys = append(s.ptys, p)
	return p, nil
}

func testWindow(t *testing.T, st *recStarter) *window {
	t.Helper()
	km, _ := buildKeymap("windows", nil)
	win := &window{w: new(app.Window), km: km, sels: map[tabs.TabID]*tabSel{}}
	win.mgr = tabs.NewManager(st.start, 80, 24, nil)
	t.Cleanup(win.mgr.CloseAll)
	return win
}

func waitSent(t *testing.T, p *recPTY, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for p.sent() != want {
		if time.Now().After(deadline) {
			t.Fatalf("shell got %q, want %q", p.sent(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestInputAfterATabSwitchReachesTheNewTab(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	newTab := key.Event{Name: "T", Modifiers: key.ModCtrl | key.ModShift, State: key.Press}
	batch := []event.Event{newTab, key.EditEvent{Text: "a"}, newTab, key.EditEvent{Text: "b"}}
	win.route(layout.Context{}, batch, 8, 16)

	if n := len(win.mgr.Tabs()); n != 3 {
		t.Fatalf("%d tabs after two new-tab keys in one frame, want 3", n)
	}
	st.mu.Lock()
	ptys := append([]*recPTY(nil), st.ptys...)
	st.mu.Unlock()
	waitSent(t, ptys[1], "a")
	waitSent(t, ptys[2], "b")
	if got := ptys[0].sent(); got != "" {
		t.Fatalf("first tab got %q, want nothing", got)
	}
}

func TestBadShellFallsBackToTheDefault(t *testing.T) {
	st := &recStarter{}
	shell := []string{"no-such-shell"}
	def := []string{"pwsh"}
	m := tabs.NewManager(func(env []string, dir string) (ptyx.Session, error) {
		if shell[0] == "no-such-shell" {
			return nil, errors.New("not found")
		}
		return st.start(env, dir)
	}, 80, 24, nil)
	t.Cleanup(m.CloseAll)

	tab, warn, err := openFirst(m, &shell, def, []string{"bad font"})
	if err != nil || tab == nil {
		t.Fatalf("openFirst: tab %v, err %v", tab, err)
	}
	if shell[0] != "pwsh" {
		t.Fatalf("shell = %v after the fallback, want the default", shell)
	}
	if !strings.Contains(warn, `"no-such-shell"`) || !strings.Contains(warn, "not found") {
		t.Fatalf("warning %q does not name the shell and the error", warn)
	}
	// Both the config warning and the fallback one are in the new tab.
	tab.Session.Lock()
	scr := tab.Session.Screen()
	var lines []string
	for i := 0; i < scr.ScrollbackLen(); i++ {
		var b strings.Builder
		for _, c := range scr.ScrollbackLine(i) {
			if c.Rune > 0 {
				b.WriteRune(c.Rune)
			}
		}
		lines = append(lines, strings.TrimSpace(b.String()))
	}
	tab.Session.Unlock()
	if len(lines) != 2 || lines[0] != "kaze: bad font" || !strings.HasPrefix(lines[1], "kaze: shell") {
		t.Fatalf("first tab shows %q, want the config warning then the fallback warning", lines)
	}
}

func TestDefaultShellFailureIsAnError(t *testing.T) {
	m := tabs.NewManager(func([]string, string) (ptyx.Session, error) { return nil, errors.New("boom") }, 80, 24, nil)
	shell := []string{"pwsh"}
	if _, _, err := openFirst(m, &shell, []string{"pwsh"}, nil); err == nil {
		t.Fatal("the default shell failing was not reported")
	}
}

func TestNewTabFailureShowsInTheStatusBar(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	st.mu.Lock()
	st.fail = errors.New("out of ptys")
	st.mu.Unlock()
	win.newTab()
	if got := win.statusText(1); !strings.Contains(got, "out of ptys") {
		t.Fatalf("status bar %q does not show the failure", got)
	}
}

func TestBothShellsFailingNamesBoth(t *testing.T) {
	m := tabs.NewManager(func([]string, string) (ptyx.Session, error) { return nil, errors.New("boom") }, 80, 24, nil)
	shell := []string{"no-such-shell"}
	_, _, err := openFirst(m, &shell, []string{"pwsh"}, nil)
	if err == nil || !strings.Contains(err.Error(), `"no-such-shell"`) || strings.Count(err.Error(), "boom") != 2 {
		t.Fatalf("err = %v; want the configured shell's failure and the default's", err)
	}
}

func TestNoticeExpires(t *testing.T) {
	win := &window{}
	now := time.Now()
	win.notice, win.noticeUntil = "new tab failed: x", now.Add(noticeFor)
	if at, ok := win.noticeWake(now); !ok || !at.Equal(now.Add(noticeFor)) {
		t.Fatalf("noticeWake = %v, %v; want the expiry so an idle window redraws then", at, ok)
	}
	if _, ok := win.noticeWake(now.Add(noticeFor + time.Second)); ok {
		t.Fatal("an expired notice still asks for a frame")
	}
	if win.notice != "" {
		t.Fatalf("expired notice kept: %q", win.notice)
	}
}
