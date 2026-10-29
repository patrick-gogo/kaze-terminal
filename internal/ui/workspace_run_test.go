package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// dirStarter is recStarter that also records each shell's folder.
type dirStarter struct {
	recStarter
	dirs  []string
	probe func() // runs inside each start, before the tab exists
}

func (s *dirStarter) start(env []string, dir string) (ptyx.Session, error) {
	if s.probe != nil {
		s.probe()
	}
	s.mu.Lock()
	s.dirs = append(s.dirs, dir)
	s.mu.Unlock()
	return s.recStarter.start(env, dir)
}

func (s *dirStarter) pty(i int) *recPTY {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ptys[i]
}

// fakeDeps stands in for Docker and the port: nothing real is run or dialed.
type fakeDeps struct {
	dockerUp chan struct{} // closed when Docker answers
	portOK   bool
	mu       sync.Mutex
	ctxs     []context.Context // every probe's run context
	now      time.Time         // the probes' clock; jumps past the 3-minute limit on a failed dial
}

func (f *fakeDeps) probes() workspaces.Probes {
	never := func(time.Duration) <-chan time.Time { return nil }
	return workspaces.Probes{
		Exec: func(ctx context.Context, name string, args ...string) error {
			f.mu.Lock()
			f.ctxs = append(f.ctxs, ctx)
			f.mu.Unlock()
			select {
			case <-f.dockerUp:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
		Dial: func(ctx context.Context, addr string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.ctxs = append(f.ctxs, ctx)
			if f.portOK {
				return nil
			}
			f.now = f.now.Add(4 * time.Minute)
			return errors.New("refused")
		},
		StartDocker: func() error { return nil },
		Now: func() time.Time {
			f.mu.Lock()
			defer f.mu.Unlock()
			return f.now
		},
		After:   func(time.Duration) <-chan time.Time { c := make(chan time.Time, 1); c <- time.Time{}; return c },
		Timeout: never,
	}
}

type runFixture struct {
	t    *testing.T
	win  *window
	st   *dirStarter
	deps *fakeDeps
	dirs []string
	now  time.Time
}

var samuraiTabs = []struct {
	title string
	cmds  []string
	wait  workspaces.Wait
	port  int
	line  string
}{
	{"api", []string{"docker compose up -d", `.\venv\Scripts\Activate.ps1`}, workspaces.WaitDocker, 0, `docker compose up -d; if ($?) { .\venv\Scripts\Activate.ps1 }`},
	{"admin", []string{"npm run dev"}, workspaces.WaitDocker, 0, "npm run dev"},
	{"storefront", []string{"npm run dev -- --port 3001"}, workspaces.WaitDocker, 0, "npm run dev -- --port 3001"},
	{"api logs", []string{"docker compose logs -f --tail=100 api"}, workspaces.WaitPort, 8000, "docker compose logs -f --tail=100 api"},
}

// newRunFixture saves the samurai workspace in a temp config folder and
// returns a window whose probes and clock are fakes.
func newRunFixture(t *testing.T) *runFixture {
	t.Helper()
	tmp := t.TempDir()
	st := &dirStarter{}
	km, _ := buildKeymap("windows", nil)
	win := &window{w: new(app.Window), km: km, sels: map[tabs.TabID]*tabSel{}, ui: newChrome(), focused: true}
	win.mgr = tabs.NewManager(st.start, 80, 24, nil)
	t.Cleanup(win.mgr.CloseAll)
	t.Cleanup(win.cancelRuns)
	win.opt.Shell = []string{`C:\Program Files\PowerShell\7\pwsh.exe`}
	win.home = tmp
	win.cfgPath = filepath.Join(tmp, "kaze", "config.toml")
	f := &runFixture{t: t, win: win, st: st, deps: &fakeDeps{dockerUp: make(chan struct{}), now: time.Unix(5000, 0)}, now: time.Unix(1000, 0)}
	win.wsr.probes = f.deps.probes()
	win.wsr.now = func() time.Time { return f.now }

	ws := workspaces.Workspace{ID: "ws_samurai", Name: "samurai"}
	for _, s := range samuraiTabs {
		dir := filepath.Join(tmp, "src", strings.ReplaceAll(s.title, " ", "_"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		f.dirs = append(f.dirs, dir)
		ws.Tabs = append(ws.Tabs, workspaces.Tab{ID: "t_" + strings.ReplaceAll(s.title, " ", "_"), Title: s.title, Folder: dir, Commands: s.cmds, Wait: s.wait, Port: s.port, Color: "cyan"})
	}
	f.save(ws)
	return f
}

func (f *runFixture) save(ws ...workspaces.Workspace) {
	f.t.Helper()
	path := workspaces.Path(f.win.cfgPath)
	_, rev, _ := workspaces.Load(path)
	if _, err := workspaces.Save(path, workspaces.Document{Workspaces: ws}, rev); err != nil {
		f.t.Fatal(err)
	}
}

// prompt makes member i's shell draw its first prompt and waits until it is applied.
func (f *runFixture) prompt(i int) {
	f.t.Helper()
	go f.st.pty(i).w.Write([]byte("\x1b]133;A\x07PS> "))
	tab := f.win.mgr.Tabs()[i]
	f.until("prompt applied", func() bool {
		tab.Session.Lock()
		defer tab.Session.Unlock()
		return tab.Session.Applier().PromptSeq() > 0
	})
}

// until ticks the run, as frames would, until cond holds; bounded.
func (f *runFixture) until(what string, cond func() bool) {
	f.t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		f.win.wsTick(f.now)
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			f.t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func (f *runFixture) theRun() *wsRun {
	f.t.Helper()
	if len(f.win.runs) != 1 {
		f.t.Fatalf("%d runs, want 1", len(f.win.runs))
	}
	for _, r := range f.win.runs {
		return r
	}
	return nil
}

func (f *runFixture) state(i int) workspaces.TabState { return f.theRun().members[i].run.State }

func (f *runFixture) allSettled() bool {
	r := f.win.runs
	for _, run := range r {
		for _, m := range run.members {
			if !m.run.Settled() {
				return false
			}
		}
	}
	return true
}

func (f *runFixture) runToast() toast {
	f.t.Helper()
	for _, t := range f.win.alerts.toasts {
		if t.Run != 0 {
			return t
		}
	}
	f.t.Fatalf("no run summary toast in %+v", f.win.alerts.toasts)
	return toast{}
}

func TestRunCreatesTabsInOrderWithDirsAndGroup(t *testing.T) {
	f := newRunFixture(t)
	f.win.runWorkspace("ws_samurai")
	list := f.win.mgr.Tabs()
	if len(list) != 4 {
		t.Fatalf("%d tabs, want 4", len(list))
	}
	run := list[0].Group.Run
	for i, tab := range list {
		if tab.Title != samuraiTabs[i].title || tab.Color != "cyan" {
			t.Errorf("tab %d: title %q color %q", i, tab.Title, tab.Color)
		}
		g := tab.Group
		if g == nil || g.Workspace != "ws_samurai" || g.Name != "samurai" || g.Run != run || g.Member != i || run == 0 {
			t.Errorf("tab %d group %+v", i, g)
		}
		if f.st.dirs[i] != f.dirs[i] {
			t.Errorf("tab %d dir %q, want %q", i, f.st.dirs[i], f.dirs[i])
		}
	}
	if a := f.win.mgr.Active(); a.ID != list[0].ID {
		t.Fatalf("active is %q, want the first member", a.Title)
	}
	if label, _ := f.win.runState(run); label != "Starting" {
		t.Fatalf("run state %q, want Starting", label)
	}
}

func TestReadyTabsGetExactlyTheirLineOnce(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	f.win.runWorkspace("ws_samurai")
	for i := range samuraiTabs {
		f.prompt(i)
	}
	f.win.wsTick(f.now)
	for i := range 3 {
		if got := f.st.pty(i).sent(); got != "" {
			t.Fatalf("tab %d got %q before Docker was up", i, got)
		}
	}
	close(f.deps.dockerUp)
	f.until("every tab launched", f.allSettled)
	for i := 0; i < 5; i++ {
		f.win.wsTick(f.now) // later frames send nothing more
	}
	for i, s := range samuraiTabs {
		waitSent(t, f.st.pty(i), s.line+"\r")
	}
	time.Sleep(10 * time.Millisecond)
	for i, s := range samuraiTabs {
		if got := f.st.pty(i).sent(); got != s.line+"\r" {
			t.Fatalf("tab %d got %q, want its line once", i, got)
		}
	}
	tst := f.runToast()
	if got := tst.Title + tst.Text; got != "samurai: started 4 of 4 tabs" {
		t.Fatalf("summary %q", got)
	}
	run := f.theRun()
	if label, needs := f.win.runState(run.id); label != "Running" || needs {
		t.Fatalf("run state %q needs %v", label, needs)
	}
	if run.ctx.Err() == nil {
		t.Fatal("a settled run kept its probe context")
	}
}

func TestTypingFirstSendsNoCommands(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	f.win.runWorkspace("ws_samurai")
	f.win.route(layout.Context{}, []event.Event{key.EditEvent{Text: "x"}}, testCW, testCH)
	if f.state(0) != workspaces.Cancelled {
		t.Fatalf("api is %v after typing, want Cancelled", f.state(0))
	}
	for i := range samuraiTabs {
		f.prompt(i)
	}
	close(f.deps.dockerUp)
	f.until("every tab settled", f.allSettled)
	waitSent(t, f.st.pty(0), "x")
	for i := 1; i < 4; i++ {
		waitSent(t, f.st.pty(i), samuraiTabs[i].line+"\r")
	}
	time.Sleep(10 * time.Millisecond)
	if got := f.st.pty(0).sent(); got != "x" {
		t.Fatalf("api got %q, want only the typed x", got)
	}
}

func TestSelectingTextDoesNotTakeOver(t *testing.T) {
	f := newRunFixture(t)
	f.win.runWorkspace("ws_samurai")
	tab := f.win.mgr.Active()
	r := &pointerRun{t: t, win: f.win, tab: tab}
	r.send(pointer.Press, pointer.ButtonPrimary, 0, 0, 0)
	r.send(pointer.Drag, pointer.ButtonPrimary, 0, 3, 0)
	r.send(pointer.Release, 0, 0, 3, 0)
	f.win.onPointer(layout.Context{}, tab, pointer.Event{Kind: pointer.Scroll, Scroll: f32.Pt(0, -120)}, modes(tab.Session), testCW, testCH)
	if f.state(0) != workspaces.Waiting {
		t.Fatalf("api is %v after selecting and scrolling, want Waiting", f.state(0))
	}
}

func TestKeysPasteAndAppMouseTakeOver(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	f.win.runWorkspace("ws_samurai")
	tabsNow := f.win.mgr.Tabs()
	// A mouse-mode app sees the click, so the owner has taken the tab.
	second := tabsNow[1]
	go f.st.pty(1).w.Write([]byte("\x1b[?1000h"))
	f.until("mouse mode on", func() bool { return modes(second.Session).MouseButton })
	f.win.onPointer(layout.Context{}, second, pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: cellPos(1, 1)}, modes(second.Session), testCW, testCH)
	if f.state(1) != workspaces.Cancelled {
		t.Fatalf("admin is %v after a reported click, want Cancelled", f.state(1))
	}
	// A key that types (Enter) takes over; a tab switch does not.
	f.win.mgr.SetActive(tabsNow[2].ID)
	f.win.route(layout.Context{}, []event.Event{key.Event{Name: key.NameTab, Modifiers: key.ModCtrl, State: key.Press}}, testCW, testCH)
	if f.state(2) != workspaces.Waiting {
		t.Fatalf("storefront is %v after switching away, want Waiting", f.state(2))
	}
	f.win.mgr.SetActive(tabsNow[2].ID)
	f.win.route(layout.Context{}, []event.Event{key.Event{Name: key.NameReturn, State: key.Press}}, testCW, testCH)
	if f.state(2) != workspaces.Cancelled {
		t.Fatalf("storefront is %v after Enter, want Cancelled", f.state(2))
	}
	f.win.mgr.SetActive(tabsNow[3].ID)
	paste := transfer.DataEvent{Type: "application/text", Open: func() io.ReadCloser { return io.NopCloser(strings.NewReader("ls")) }}
	f.win.route(layout.Context{}, []event.Event{paste}, testCW, testCH)
	if f.state(3) != workspaces.Cancelled {
		t.Fatalf("api logs is %v after a paste, want Cancelled", f.state(3))
	}
}

func TestRunTwiceOpensOneSet(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	f.win.runWorkspace("ws_samurai")
	f.win.mgr.SetActive(f.win.mgr.Tabs()[2].ID)
	f.win.runWorkspace("ws_samurai")
	list := f.win.mgr.Tabs()
	if len(list) != 4 || len(f.win.runs) != 1 {
		t.Fatalf("%d tabs and %d runs after running twice, want 4 and 1", len(list), len(f.win.runs))
	}
	if f.win.mgr.Active().ID != list[0].ID {
		t.Fatal("running again did not focus the first member")
	}
	for i := range samuraiTabs {
		f.prompt(i)
	}
	close(f.deps.dockerUp)
	f.until("every tab launched", f.allSettled)
	f.win.runWorkspace("ws_samurai") // settled but still open: still one set
	if len(f.win.mgr.Tabs()) != 4 {
		t.Fatalf("%d tabs after a third run, want 4", len(f.win.mgr.Tabs()))
	}
	for i, s := range samuraiTabs {
		waitSent(t, f.st.pty(i), s.line+"\r")
	}
}

func TestPortFailureSummarizesThreeOfFour(t *testing.T) {
	f := newRunFixture(t)
	f.win.runWorkspace("ws_samurai")
	for i := range samuraiTabs {
		f.prompt(i)
	}
	close(f.deps.dockerUp)
	f.until("every tab settled", f.allSettled)
	if f.state(3) != workspaces.Failed {
		t.Fatalf("api logs is %v, want Failed", f.state(3))
	}
	tst := f.runToast()
	if got, want := tst.Title+tst.Text, "samurai: started 3 of 4 tabs. api logs: port 8000 never answered."; got != want {
		t.Fatalf("summary %q, want %q", got, want)
	}
	if got := f.st.pty(3).sent(); got != "" {
		t.Fatalf("api logs got %q though its port never answered", got)
	}
	if label, needs := f.win.runState(f.theRun().id); label != "3 of 4" || !needs {
		t.Fatalf("run state %q needs %v", label, needs)
	}
	if len(f.win.alerts.toasts) != 1 {
		t.Fatalf("%d toasts, want one summary per run", len(f.win.alerts.toasts))
	}
	f.win.visit(f.win.mgr.Active())
	if len(f.win.alerts.toasts) != 1 {
		t.Fatal("looking at the first member dropped the run summary")
	}
}

func TestClosingTheLastMemberCancelsTheProbes(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true // api logs keeps waiting for its prompt
	f.win.runWorkspace("ws_samurai")
	f.until("the Docker probe started", func() bool {
		f.deps.mu.Lock()
		defer f.deps.mu.Unlock()
		return len(f.deps.ctxs) > 0
	})
	run := f.theRun()
	for _, tab := range f.win.mgr.Tabs()[:3] {
		f.win.mgr.Close(tab.ID)
	}
	f.win.wsTick(f.now)
	if run.ctx.Err() != nil {
		t.Fatal("the probes stopped while a member was still waiting")
	}
	f.win.mgr.Close(f.win.mgr.Tabs()[0].ID)
	f.win.wsTick(f.now)
	f.deps.mu.Lock()
	ctxs := append([]context.Context(nil), f.deps.ctxs...)
	f.deps.mu.Unlock()
	for _, c := range ctxs {
		select {
		case <-c.Done():
		case <-time.After(time.Second):
			t.Fatal("a probe kept running after the last member closed")
		}
	}
	if len(f.win.runs) != 0 {
		t.Fatal("the run is still tracked after its last member closed")
	}
	if !f.win.shouldClose() {
		t.Fatal("an empty window with no run being created stays open")
	}
}

func TestPromptTimeoutAsksAndRunCommandsSends(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	close(f.deps.dockerUp)
	f.win.runWorkspace("ws_samurai")
	at, ok := f.win.wsTick(f.now)
	if !ok || !at.Equal(f.now.Add(3*time.Second)) {
		t.Fatalf("wake %v %v, want one at +3s", at, ok)
	}
	f.until("dependencies ready", func() bool { return f.theRun().members[0].run.DepReady })
	f.now = f.now.Add(3 * time.Second)
	f.win.wsTick(f.now)
	if f.state(0) != workspaces.NeedsConfirmation {
		t.Fatalf("api is %v after 3s without a prompt, want NeedsConfirmation", f.state(0))
	}
	if got := f.st.pty(0).sent(); got != "" {
		t.Fatalf("api got %q on a timer alone", got)
	}
	f.win.drawWsBanner(pageContext(800, 600), f.win.mgr.Active(), image.Rect(0, 38, 800, 576))
	var run image.Rectangle
	for _, h := range f.win.wsr.hits {
		if h.kind == bannerRun {
			run = h.rect
		}
	}
	if run.Empty() {
		t.Fatalf("no Run commands button in %+v", f.win.wsr.hits)
	}
	f.win.bannerPress(pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(run.Min.X+2), float32(run.Min.Y+2))})
	waitSent(t, f.st.pty(0), samuraiTabs[0].line+"\r")
	if f.state(0) != workspaces.Launched {
		t.Fatalf("api is %v after Run commands", f.state(0))
	}
}

func TestUseShellCancelsAndTheBannerGoes(t *testing.T) {
	f := newRunFixture(t)
	f.win.runWorkspace("ws_samurai")
	f.win.drawWsBanner(pageContext(800, 600), f.win.mgr.Active(), image.Rect(0, 38, 800, 576))
	var use image.Rectangle
	for _, h := range f.win.wsr.hits {
		if h.kind == bannerRun {
			t.Fatal("Run commands shows before the prompt wait passed")
		}
		if h.kind == bannerShell {
			use = h.rect
		}
	}
	if use.Empty() {
		t.Fatalf("no Use shell button in %+v", f.win.wsr.hits)
	}
	f.win.bannerPress(pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(use.Min.X+2), float32(use.Min.Y+2))})
	if f.state(0) != workspaces.Cancelled {
		t.Fatalf("api is %v after Use shell", f.state(0))
	}
	f.win.drawWsBanner(pageContext(800, 600), f.win.mgr.Active(), image.Rect(0, 38, 800, 576))
	if len(f.win.wsr.hits) != 0 {
		t.Fatal("the banner stayed after Use shell")
	}
}

func TestBannerSaysWhatItWaitsFor(t *testing.T) {
	f := newRunFixture(t)
	f.win.runWorkspace("ws_samurai")
	cases := map[int]string{0: "Waiting for Docker…", 3: "Waiting for port 8000…"}
	for i, want := range cases {
		m := f.theRun().members[i]
		if got := bannerText(&m.run); got != want {
			t.Errorf("member %d banner %q, want %q", i, got, want)
		}
	}
	m := f.theRun().members[1]
	m.run.DepReady = true
	if got := bannerText(&m.run); got != "Waiting for shell prompt" {
		t.Errorf("banner %q once Docker is up", got)
	}
}

func TestNoWakeOnceEverythingSettled(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	close(f.deps.dockerUp)
	f.win.runWorkspace("ws_samurai")
	for i := range samuraiTabs {
		f.prompt(i)
	}
	f.until("every tab launched", f.allSettled)
	if at, ok := f.win.wsTick(f.now); ok {
		t.Fatalf("a settled run still asks for a wake at %v", at)
	}
}

func TestExitedShellFails(t *testing.T) {
	f := newRunFixture(t)
	f.win.runWorkspace("ws_samurai")
	f.st.pty(1).Close()
	f.until("admin failed", func() bool { return f.state(1) == workspaces.Failed })
}

func TestPreflightFailuresOpenNoTabs(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(f *runFixture, ws *workspaces.Workspace)
		shell []string
		want  string
	}{
		{"missing folder", func(f *runFixture, ws *workspaces.Workspace) {
			ws.Tabs[2].Folder = filepath.Join(f.win.home, "gone")
		}, nil, "storefront"},
		{"unknown shell", nil, []string{`C:\Windows\System32\cmd.exe`}, "cmd"},
		{"trailing comment", func(f *runFixture, ws *workspaces.Workspace) {
			ws.Tabs[0].Commands = []string{"docker compose up -d # start", "b"}
		}, nil, "comment"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newRunFixture(t)
			first, _ := f.win.mgr.New() // the owner's own tab
			doc, _, _ := workspaces.Load(workspaces.Path(f.win.cfgPath))
			ws := doc.Workspaces[0]
			if c.edit != nil {
				c.edit(f, &ws)
				f.save(ws)
			}
			if c.shell != nil {
				f.win.opt.Shell = c.shell
			}
			f.win.runWorkspace("ws_samurai")
			if n := len(f.win.mgr.Tabs()); n != 1 || len(f.win.runs) != 0 {
				t.Fatalf("%d tabs and %d runs after a failed preflight", n, len(f.win.runs))
			}
			if f.win.mgr.Active().ID != first.ID {
				t.Fatal("a failed preflight changed the active tab")
			}
			if !strings.Contains(f.win.notice, c.want) || !strings.Contains(f.win.notice, "samurai") {
				t.Fatalf("notice %q, want it to name %q and the workspace", f.win.notice, c.want)
			}
		})
	}
}

// The loop asks shouldClose only after a frame; by then a startup run has opened its tabs.
func TestWindowStaysOpenAfterAStartupWorkspace(t *testing.T) {
	f := newRunFixture(t)
	f.win.opt.Config.StartupWorkspace = "ws_samurai"
	if err := f.win.openStartup(nil); err != nil {
		t.Fatal(err)
	}
	if n := len(f.win.mgr.Tabs()); n != len(samuraiTabs) || f.win.shouldClose() {
		t.Fatalf("%d tabs, close %v after the startup workspace", n, f.win.shouldClose())
	}
}

func TestFrameDrawsTheBannerOverTheTerminal(t *testing.T) {
	f := newRunFixture(t)
	f.win.opt.Config = config.Default()
	f.win.runWorkspace("ws_samurai")
	var r input.Router
	sizedFrame(t, f.win, &r, 800, 600)
	if len(f.win.wsr.hits) == 0 {
		t.Fatal("the frame drew no banner for a waiting tab")
	}
	if b := f.win.wsr.hits[0].rect; b.Min.Y < 38 || b.Max.Y > 576 {
		t.Fatalf("banner %v is outside the terminal area", b)
	}
	f.win.openSettings()
	sizedFrame(t, f.win, &r, 800, 600)
	if len(f.win.wsr.hits) != 0 {
		t.Fatal("the banner kept its buttons under the settings page")
	}
}

// A key typed in the same frame that the dependency and the prompt become
// ready wins: the owner's input is forwarded and the line is never typed.
func TestKeyInTheSameFrameAsReadinessWins(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	f.win.opt.Config = config.Default()
	f.win.runWorkspace("ws_samurai")
	var r input.Router
	sizedFrame(t, f.win, &r, 800, 600) // focuses the terminal on the first member

	close(f.deps.dockerUp)
	deadline := time.Now().Add(3 * time.Second)
	for len(f.win.wsr.results) == 0 { // queued, not yet drained: no tick runs here
		if time.Now().After(deadline) {
			t.Fatal("the Docker result never arrived")
		}
		time.Sleep(time.Millisecond)
	}
	api := f.win.mgr.Tabs()[0]
	go f.st.pty(0).w.Write([]byte("\x1b]133;A\x07PS> "))
	for !promptSeen(api.Session) {
		if time.Now().After(deadline) {
			t.Fatal("the prompt marker never arrived")
		}
		time.Sleep(time.Millisecond)
	}
	if f.state(0) != workspaces.Waiting {
		t.Fatalf("api is %v before the frame, want Waiting", f.state(0))
	}

	r.Queue(key.Event{Name: key.NameReturn, State: key.Press})
	sizedFrame(t, f.win, &r, 800, 600)
	if f.state(0) != workspaces.Cancelled {
		t.Fatalf("api is %v after the frame, want Cancelled", f.state(0))
	}
	waitSent(t, f.st.pty(0), "\r")
	time.Sleep(10 * time.Millisecond)
	if got := f.st.pty(0).sent(); got != "\r" {
		t.Fatalf("api got %q, want only the typed Enter", got)
	}
}

func TestAppWheelTakesOverButHoverDoesNot(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	f.win.runWorkspace("ws_samurai")
	tab := f.win.mgr.Tabs()[0]
	go f.st.pty(0).w.Write([]byte("\x1b[?1003h")) // any-motion mouse mode
	f.until("mouse mode on", func() bool { return modes(tab.Session).MouseAny })
	f.win.onPointer(layout.Context{}, tab, pointer.Event{Kind: pointer.Move, Position: cellPos(2, 2)}, modes(tab.Session), testCW, testCH)
	if f.state(0) != workspaces.Waiting {
		t.Fatalf("api is %v after a hover report, want Waiting", f.state(0))
	}
	f.win.onPointer(layout.Context{}, tab, pointer.Event{Kind: pointer.Scroll, Scroll: f32.Pt(0, -120), Position: cellPos(2, 2)}, modes(tab.Session), testCW, testCH)
	if f.state(0) != workspaces.Cancelled {
		t.Fatalf("api is %v after a wheel report to the app, want Cancelled", f.state(0))
	}
}

// A run whose only missing tab is one the owner took over is not a failure.
func TestTakeoverOnlyRunGetsADoneCard(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	close(f.deps.dockerUp)
	f.win.runWorkspace("ws_samurai")
	f.win.route(layout.Context{}, []event.Event{key.EditEvent{Text: "x"}}, testCW, testCH)
	for i := 1; i < 4; i++ {
		f.prompt(i)
	}
	f.until("every tab settled", f.allSettled)
	tst := f.runToast()
	if tst.Kind != notify.KindDone || tst.Until.IsZero() {
		t.Fatalf("toast kind %q until %v, want a timed done card", tst.Kind, tst.Until)
	}
	if got := tst.Title + tst.Text; got != "samurai: started 3 of 4 tabs" {
		t.Fatalf("summary %q", got)
	}
	if label, needs := f.win.runState(f.theRun().id); label != "3 of 4" || needs {
		t.Fatalf("run state %q needs %v, want 3 of 4 without needs", label, needs)
	}
}

func TestFailedBannerOffersDismiss(t *testing.T) {
	f := newRunFixture(t)
	f.win.runWorkspace("ws_samurai")
	f.until("api logs failed", func() bool { return f.state(3) == workspaces.Failed })
	if got := bannerButtons(&f.theRun().members[3].run); len(got) != 1 || got[0].text != "Dismiss" {
		t.Fatalf("failed banner buttons %+v, want only Dismiss", got)
	}
	if got := bannerButtons(&f.theRun().members[0].run); len(got) != 1 || got[0].text != "Use shell" {
		t.Fatalf("waiting banner buttons %+v, want only Use shell", got)
	}
}

// Run from the page says so when not one shell started; nothing else would.
func TestRunThatOpensNothingPostsANotice(t *testing.T) {
	f := newRunFixture(t)
	f.st.mu.Lock()
	f.st.fail = errors.New("out of ptys")
	f.st.mu.Unlock()
	f.win.runWorkspace("ws_samurai")
	if len(f.win.mgr.Tabs()) != 0 {
		t.Fatalf("%d tabs opened", len(f.win.mgr.Tabs()))
	}
	if !strings.Contains(f.win.notice, "samurai") || !strings.Contains(f.win.notice, "out of ptys") {
		t.Fatalf("notice %q, want the workspace and why", f.win.notice)
	}
}

// Dragging in a mouse-reporting app is the owner using it; hovering is not.
func TestAppDragTakesOverButHoverDoesNot(t *testing.T) {
	f := newRunFixture(t)
	f.deps.portOK = true
	f.win.runWorkspace("ws_samurai")
	tab := f.win.mgr.Tabs()[0]
	go f.st.pty(0).w.Write([]byte("[?1003h")) // any-motion mouse mode
	f.until("mouse mode on", func() bool { return modes(tab.Session).MouseAny })
	f.win.onPointer(layout.Context{}, tab, pointer.Event{Kind: pointer.Move, Position: cellPos(2, 2)}, modes(tab.Session), testCW, testCH)
	if f.state(0) != workspaces.Waiting {
		t.Fatalf("api is %v after a hover report, want Waiting", f.state(0))
	}
	f.win.onPointer(layout.Context{}, tab, pointer.Event{Kind: pointer.Move, Buttons: pointer.ButtonPrimary, Position: cellPos(3, 2)}, modes(tab.Session), testCW, testCH)
	if f.state(0) != workspaces.Cancelled {
		t.Fatalf("api is %v after a drag report to the app, want Cancelled", f.state(0))
	}
}

// Once no tab needs Docker, a sibling waiting for its prompt must not keep
// Docker polling. Both closing and taking over release the last consumer.
func TestUnusedWorkspaceDependencyIsCancelled(t *testing.T) {
	for _, closeTab := range []bool{false, true} {
		t.Run(fmt.Sprint("close=", closeTab), func(t *testing.T) {
			f := newRunFixture(t)
			doc, _, err := workspaces.Load(workspaces.Path(f.win.cfgPath))
			if err != nil {
				t.Fatal(err)
			}
			ws := doc.Workspaces[0]
			ws.Tabs = ws.Tabs[:2]
			ws.Tabs[1].Wait = workspaces.WaitNone
			f.save(ws)
			f.win.runWorkspace(ws.ID)
			f.until("Docker probe started", func() bool {
				f.deps.mu.Lock()
				defer f.deps.mu.Unlock()
				return len(f.deps.ctxs) > 0
			})
			f.deps.mu.Lock()
			ctx := f.deps.ctxs[0]
			f.deps.mu.Unlock()
			tab := f.win.mgr.Tabs()[0]
			if closeTab {
				f.win.mgr.Close(tab.ID)
			} else {
				f.win.takeOver(tab)
			}
			f.now = f.now.Add(4 * time.Second)
			f.win.wsTick(f.now)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("unused Docker dependency kept polling")
			}
			if f.theRun().ctx.Err() != nil {
				t.Fatal("whole run cancelled while sibling awaits prompt")
			}
			if f.state(1) != workspaces.NeedsConfirmation {
				t.Fatalf("sibling state %v", f.state(1))
			}
		})
	}
}

func TestSharedWorkspaceDependencyRetainsRemainingConsumers(t *testing.T) {
	f := newRunFixture(t)
	doc, _, err := workspaces.Load(workspaces.Path(f.win.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	ws := doc.Workspaces[0]
	ws.Tabs = ws.Tabs[:2]
	f.save(ws)
	f.win.runWorkspace(ws.ID)
	f.until("Docker probe started", func() bool {
		f.deps.mu.Lock()
		defer f.deps.mu.Unlock()
		return len(f.deps.ctxs) > 0
	})
	f.deps.mu.Lock()
	ctx := f.deps.ctxs[0]
	f.deps.mu.Unlock()
	f.win.takeOver(f.win.mgr.Tabs()[0])
	f.win.wsTick(f.now)
	if ctx.Err() != nil {
		t.Fatal("shared dependency cancelled with a consumer still waiting")
	}
	f.prompt(1)
	close(f.deps.dockerUp)
	f.until("remaining tab launched", func() bool { return f.state(1) == workspaces.Launched })
	if f.state(0) != workspaces.Cancelled {
		t.Fatal("cancelled member was revived by shared result")
	}
	waitSent(t, f.st.pty(1), ws.Tabs[1].Commands[0]+"\r")
	if f.st.pty(0).sent() != "" {
		t.Fatal("cancelled member received commands")
	}
}
