package ui

import (
	"image"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/pointer"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

type promptFixture struct {
	t   *testing.T
	win *window
	st  *dirStarter
	now time.Time
	cur *tabs.Tab
}

// newPromptFixture opens one session in folder dir; the shell is zsh unless set.
func newPromptFixture(t *testing.T) *promptFixture {
	t.Helper()
	st := &dirStarter{}
	win := testWindow(t, &st.recStarter)
	win.mgr = tabs.NewManager(st.start, 80, 24, nil)
	t.Cleanup(win.mgr.CloseAll)
	win.ui = newChrome()
	win.focused = true
	win.opt.Shell = []string{"/bin/zsh"}
	f := &promptFixture{t: t, win: win, st: st, now: time.Unix(1000, 0)}
	win.wsr.now = func() time.Time { return f.now }
	cur, err := win.mgr.New()
	if err != nil {
		t.Fatal(err)
	}
	f.cur = cur
	return f
}

func (f *promptFixture) claude(id tabs.TabID) {
	f.win.noteAgent(notify.Msg{Tab: int(id), Kind: notify.KindPrompt}, id, 0)
}

func (f *promptFixture) needs(id tabs.TabID) {
	f.win.alerts.apply(notify.Msg{Tab: int(id), Kind: notify.KindNeeds}, 0, false, func(tabs.TabID) bool { return true }, f.now)
}

// cwd makes the shell of pty i report folder dir and waits for it to apply.
func (f *promptFixture) cwd(i int, t *tabs.Tab, dir string) {
	f.t.Helper()
	u := url.URL{Path: "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")}
	go f.st.pty(i).w.Write([]byte("\x1b]7;file://host" + u.EscapedPath() + "\x07"))
	f.until("the folder report", func() bool {
		t.Session.Lock()
		defer t.Session.Unlock()
		return t.Session.Applier().Cwd() != ""
	})
}

func (f *promptFixture) until(what string, cond func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		f.win.launchTick(f.now)
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// settle gives a stray write time to arrive before checking that none did.
func settle() { time.Sleep(30 * time.Millisecond) }

func TestTypeHereRule(t *testing.T) {
	cur := &tabs.Tab{ID: 1}
	idle := agentState{claude: true}
	run := agentState{claude: true, running: true}
	cases := []struct {
		name                    string
		cur                     *tabs.Tab
		session, settings, shft bool
		a                       agentState
		m                       mark
		want                    bool
	}{
		{"claude idle", cur, true, false, false, idle, markNone, true},
		{"claude done", cur, true, false, false, idle, markDone, true},
		{"claude running", cur, true, false, false, run, markNone, true},
		{"needs", cur, true, false, false, idle, markNeeds, false},
		{"plain shell", cur, true, false, false, agentState{}, markNone, false},
		{"shift", cur, true, false, true, idle, markNone, false},
		{"member", cur, false, false, false, idle, markNone, false},
		{"settings in front", cur, true, true, false, idle, markNone, false},
		{"no session", nil, true, false, false, idle, markNone, false},
	}
	for _, c := range cases {
		if got := typeHere(c.cur, c.session, c.settings, c.a, c.m, c.shft); got != c.want {
			t.Errorf("%s: got %v", c.name, got)
		}
	}
}

func TestTypingIntoClaudeHasNoEnter(t *testing.T) {
	f := newPromptFixture(t)
	f.claude(f.cur.ID)
	f.win.startPrompt("/ship-ticket", false)
	waitSent(t, f.st.pty(0), "/ship-ticket ")
	if len(f.win.mgr.Tabs()) != 1 {
		t.Fatal("a new session opened")
	}
	if !strings.Contains(f.win.notice, "Typed /ship-ticket into") || !strings.Contains(f.win.notice, "Press Enter to send.") {
		t.Fatalf("notice %q", f.win.notice)
	}
	if f.win.palette.recent.list[0] != "/ship-ticket" {
		t.Fatalf("recent %v", f.win.palette.recent.list)
	}
}

func TestTypedPromptIsCleaned(t *testing.T) {
	f := newPromptFixture(t)
	f.claude(f.cur.ID)
	f.win.startPrompt("/x\r\n\x1b[2J\t"+strings.Repeat("y", 600), false)
	want := "/x[2J" + strings.Repeat("y", 512-5) + " "
	waitSent(t, f.st.pty(0), want)
}

func TestNeedsOpensANewSession(t *testing.T) {
	f := newPromptFixture(t)
	dir := t.TempDir()
	f.cwd(0, f.cur, dir)
	f.claude(f.cur.ID)
	f.needs(f.cur.ID)
	f.win.startPrompt("/v3-lap", false)
	if n := len(f.win.mgr.Tabs()); n != 2 {
		t.Fatalf("%d tabs, want 2", n)
	}
	if got := f.st.dirs[1]; got != dir {
		t.Fatalf("new session in %q, want %q", got, dir)
	}
	if a := f.win.mgr.Active(); a == nil || a.ID == f.cur.ID {
		t.Fatal("the new session is not shown")
	}
	if f.win.notice != "Opened a new session for /v3-lap." {
		t.Fatalf("notice %q", f.win.notice)
	}
	settle()
	if got := f.st.pty(0).sent(); got != "" {
		t.Fatalf("old tab got %q", got)
	}
}

func TestNewSessionTypesClaudeWhenReady(t *testing.T) {
	f := newPromptFixture(t)
	f.win.startPrompt("/v3-lap", false) // a plain shell: a new session
	nt := f.win.mgr.Active()
	f.win.launchTick(f.now)
	settle()
	if got := f.st.pty(1).sent(); got != "" {
		t.Fatalf("wrote %q before the prompt", got)
	}
	f.cwd(1, nt, "/tmp")
	f.until("the line", func() bool { return f.st.pty(1).sent() != "" })
	waitSent(t, f.st.pty(1), "claude '/v3-lap'\r")
	if len(f.win.launches) != 0 {
		t.Fatal("the launch was kept after it ran")
	}
}

func TestNewSessionQuotesForPowerShell(t *testing.T) {
	f := newPromptFixture(t)
	f.win.opt.Shell = []string{`C:\Program Files\PowerShell\7\pwsh.exe`}
	f.win.startPrompt("/it's", false)
	f.cwd(1, f.win.mgr.Active(), "/tmp")
	f.until("the line", func() bool { return f.st.pty(1).sent() != "" })
	waitSent(t, f.st.pty(1), "claude '/it''s'\r")
}

func TestNewSessionAsksAfterTheDeadline(t *testing.T) {
	for _, run := range []bool{true, false} {
		f := newPromptFixture(t)
		f.win.startPrompt("/v3-lap", false)
		nt := f.win.mgr.Active()
		wake, ok := f.win.launchTick(f.now)
		if !ok || !wake.Equal(f.now.Add(confirmAfter)) {
			t.Fatalf("wake %v %v, want one at the deadline", wake, ok)
		}
		f.now = f.now.Add(confirmAfter)
		if _, ok := f.win.launchTick(f.now); ok {
			t.Fatal("asked for another wake after the deadline")
		}
		settle()
		if got := f.st.pty(1).sent(); got != "" {
			t.Fatalf("wrote %q on the timer", got)
		}
		m := f.win.bannerMember(nt)
		if !bannerShown(m) || bannerText(&m.run) != "Waiting for shell prompt" {
			t.Fatalf("banner not shown: %+v", m)
		}
		f.win.drawWsBanner(pageContext(800, 600), nt, image.Rect(0, 0, 800, 600))
		want := bannerShell
		if run {
			want = bannerRun
		}
		pressed := false
		for _, h := range f.win.wsr.hits {
			if h.kind == want {
				c := h.rect.Min.Add(image.Pt(2, 2))
				f.win.bannerPress(pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(c.X), float32(c.Y))})
				pressed = true
			}
		}
		if !pressed {
			t.Fatalf("no button %v", want)
		}
		f.win.launchTick(f.now)
		if run {
			waitSent(t, f.st.pty(1), "claude '/v3-lap'\r")
		} else {
			settle()
			if got := f.st.pty(1).sent(); got != "" {
				t.Fatalf("Use shell still wrote %q", got)
			}
		}
		if len(f.win.launches) != 0 {
			t.Fatalf("run=%v: the launch was kept", run)
		}
	}
}

func TestPendingLaunchDroppedOnClose(t *testing.T) {
	f := newPromptFixture(t)
	f.win.startPrompt("/v3-lap", false)
	nt := f.win.mgr.Active()
	f.win.closeTab(nt.ID)
	f.win.launchTick(f.now)
	if len(f.win.launches) != 0 {
		t.Fatal("the launch outlived its tab")
	}
	f.now = f.now.Add(time.Minute)
	f.win.launchTick(f.now)
	settle()
	if got := f.st.pty(0).sent(); got != "" {
		t.Fatalf("another tab got %q", got)
	}
}

func TestPendingLaunchDroppedOnExit(t *testing.T) {
	f := newPromptFixture(t)
	f.win.startPrompt("/v3-lap", false)
	f.st.pty(1).Close()
	f.until("the launch dropped", func() bool { return len(f.win.launches) == 0 })
}

func TestOwnerTypingCancelsLaunch(t *testing.T) {
	f := newPromptFixture(t)
	f.win.startPrompt("/v3-lap", false)
	nt := f.win.mgr.Active()
	f.win.takeOver(nt)
	if len(f.win.launches) != 0 {
		t.Fatal("typing did not cancel the launch")
	}
	f.cwd(1, nt, "/tmp")
	f.win.launchTick(f.now)
	settle()
	if got := f.st.pty(1).sent(); got != "" {
		t.Fatalf("wrote %q after the owner took over", got)
	}
}

func TestUnknownShellOpensNothing(t *testing.T) {
	f := newPromptFixture(t)
	f.win.opt.Shell = []string{`C:\Windows\System32\cmd.exe`}
	f.win.startPrompt("/v3-lap", false)
	if n := len(f.win.mgr.Tabs()); n != 1 {
		t.Fatalf("%d tabs", n)
	}
	if !strings.HasPrefix(f.win.notice, "Can't start /v3-lap in this shell (cmd):") {
		t.Fatalf("notice %q", f.win.notice)
	}
}

func TestRuleDecidedWhenActing(t *testing.T) {
	f := newPromptFixture(t)
	f.claude(f.cur.ID) // idle Claude: typing here would be fine...
	f.needs(f.cur.ID)  // ...until it asks for permission before Enter
	f.win.startPrompt("/x", false)
	if n := len(f.win.mgr.Tabs()); n != 2 {
		t.Fatalf("%d tabs, want a new session", n)
	}
	settle()
	if got := f.st.pty(0).sent(); got != "" {
		t.Fatalf("typed %q into a permission prompt", got)
	}
}

func TestLaunchIsNotAWorkspaceRun(t *testing.T) {
	f := newPromptFixture(t)
	f.win.startPrompt("/x", false)
	if len(f.win.runs) != 0 || len(f.win.sessionIDs()) != 2 {
		t.Fatalf("runs %d, sessions %v", len(f.win.runs), f.win.sessionIDs())
	}
	if f.win.memberOf(f.win.mgr.Active()) != nil {
		t.Fatal("the new session counts as a workspace member")
	}
	_ = workspaces.Waiting
}
