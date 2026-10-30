// Package ui is kaze's main window: the title bar, the sidebar, the active tab's
// terminal and the status bar, plus keyboard, mouse, clipboard and focus handling.
package ui

import (
	"fmt"
	"image"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/patrick-gogo/kaze-terminal/internal/bench"
	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/ptyx"
	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/screen"
	"github.com/patrick-gogo/kaze-terminal/internal/sound"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
	"github.com/patrick-gogo/kaze-terminal/internal/usage"
)

// Options are what main resolved at startup.
type Options struct {
	Config   config.Config
	Warnings []string        // shown in the first tab
	Shell    []string        // argv, already resolved to the platform default when the config has none
	Dir      string          // working directory for new shells
	Bench    *bench.Recorder // nil unless KAZE_BENCH is set
	// Workspace is the name from --workspace; it wins over the startup_workspace setting.
	Workspace string
}

const (
	allMods = key.ModCtrl | key.ModShift | key.ModAlt | key.ModSuper | key.ModCommand
	// maxPaste bounds a clipboard read so a giant clipboard can't exhaust memory.
	maxPaste = 64 << 20
	// memEvery limits how often the status bar reads memory stats (ReadMemStats stops the world).
	memEvery = 2 * time.Second
	// noticeFor is how long an error stays in the status bar.
	noticeFor = 15 * time.Second
	// shutdownWait bounds quitting, as tabs.CloseAll does for the shells.
	shutdownWait = 2 * time.Second
	// blinkEvery is one blink phase, shown or hidden.
	blinkEvery = 530 * time.Millisecond
)

type window struct {
	w   *app.Window
	opt Options
	pal render.Palette
	km  keymap

	mgr       *tabs.Manager
	rend      *render.Renderer
	rendPx    float32
	rendStale bool // a setting changed: rebuild before the next layout, never mid-frame
	ui        *chrome
	termTag   *int
	defTitle  string
	winTitle  string
	lastTab   *tabs.Tab
	focused   bool
	wantFocus bool

	blinkFrom    time.Time // start of the shown phase; zero restarts it
	cursorHidden bool

	wheel     wheel
	gate      textGate
	composing bool
	gridMin   image.Point // top-left of the grid in window pixels

	home     string                              // for "~" in the status bar
	branchFn func(dir string) string             // gitinfo.Branch unless a test replaces it
	repoFn   func(dir string) (key, name string) // gitinfo.Repo unless a test replaces it
	locs     map[tabs.TabID]locEntry

	memMB   float64
	memRead time.Time

	notice      string // a recent error for the status bar
	noticeUntil time.Time

	sels       map[tabs.TabID]*tabSel
	clicks     clicks
	held       pointer.Buttons // buttons down, to tell which one a press or release changed
	toApp      bool            // the current mouse gesture is reported to the app, not selecting
	gestureTab *tabs.Tab       // the tab the held gesture started in
	gestureCol int             // last cell reported to the app, for a release sent by endGesture
	gestureRow int
	dragPos    image.Point // last drag position relative to the grid, for auto-scroll
	scrolledAt time.Time

	toClipboard func(string) // replaces the Gio clipboard write in tests

	bench *bench.Recorder // nil unless KAZE_BENCH is set; every hook checks it first
	bf    benchFrame

	// Title bar: kaze draws its own caption buttons where custom is set (Windows).
	custom     bool
	bar        barLayout // where the last frame put the toggle, drag area and buttons
	caption    captionState
	capTag     *int
	dotsTag    *int
	serverDots image.Rectangle     // the status bar's dots in window pixels; empty when none
	maximized  bool                // from the last ConfigEvent, so the button shows the real state
	performFn  func(system.Action) // replaces win.w.Perform in tests

	// Alerts from Claude Code hooks (kaze notify).
	srv         *notify.Server  // nil when the listener could not start
	inbox       inbox           // notify goroutines -> UI goroutine
	alerts      alerts          // UI goroutine only
	alertMu     sync.Mutex      // guards chime and the alert settings onNotify reads
	chime       func(time.Time) // sound.Player.Play; nil until a chime is on or tested
	flash       func()          // flashes the taskbar button
	toastTag    *int
	liveMgr     atomic.Pointer[tabs.Manager] // win.mgr, for onNotify
	snapActive  atomic.Int64                 // active TabID, mirrored for onNotify
	snapFocused atomic.Bool
	hwnd        atomic.Uintptr // native window handle, 0 until known

	// Claude usage from `kaze statusline`.
	usageIn usageBox                  // notify goroutines -> UI goroutine
	acct    notify.Usage              // plan limits (no Ctx); UI goroutine only
	ctx     map[tabs.TabID]ctxEntry   // per-tab context use; UI goroutine only
	agents  map[tabs.TabID]agentState // per-tab Claude state; UI goroutine only
	names   map[tabs.TabID]string     // row names the owner gave; UI goroutine only
	lc      lineCounter               // git lines added and removed per tab; UI goroutine only

	set     settingsState // the Settings page; UI goroutine only
	cfgPath string        // where the page saves; "" applies changes without saving

	claudePath string      // Claude Code's settings.json; "" checks nothing
	exePath    string      // this kaze, which a Claude setup fix points the entries at
	claude     claudeState // the Claude section's setup check and dialog

	runs map[uint64]*wsRun // workspace runs with a tab still open; UI goroutine only
	wsr  wsRunner
	side sideUI // the sidebar; UI goroutine only
	wsp  wsPage // the Workspaces section: its list and the open draft; UI goroutine only
}

// Run drives the window until it closes. It returns after every shell is closed.
func Run(w *app.Window, opt Options) error {
	km, warns := buildKeymap(runtime.GOOS, opt.Config.Keys)
	input.SetReserved(runtime.GOOS, km.shortcuts())
	opt.Warnings = append(opt.Warnings, warns...)
	win := &window{
		w: w, opt: opt, km: km,
		ui:       newChrome(),
		termTag:  new(int),
		defTitle: shellName(opt.Shell),
		focused:  true, wantFocus: true,
		home:   userHome(),
		sels:   map[tabs.TabID]*tabSel{},
		bench:  opt.Bench,
		custom: customTitleBar,
		capTag: new(int),

		toastTag: new(int),
		cfgPath:  config.Path(),
	}
	win.applyTheme(startTheme(opt.Config))
	win.snapFocused.Store(true)
	win.flash = func() { flashTaskbar(win.hwnd.Load()) }
	if home := win.home; home != "" {
		win.claudePath = filepath.Join(home, ".claude", "settings.json")
	}
	if exe, err := os.Executable(); err == nil {
		win.exePath = exe
	}
	if c := opt.Config; c.ChimeNeeds || c.ChimeDone {
		if warn := win.ensureChime(); warn != "" {
			win.opt.Warnings = append(win.opt.Warnings, warn)
		}
	}
	// Last: onNotify may run as soon as this returns.
	if srv, err := notify.Listen(win.onNotify); err != nil {
		win.opt.Warnings = append(win.opt.Warnings, "alerts are off: "+err.Error())
	} else {
		win.srv = srv
	}
	return win.loop()
}

// newChime builds the chime player; tests replace it.
var newChime = func(file string) (func(time.Time), string) {
	p, warn := sound.New(file)
	return p.Play, warn
}

// ensureChime builds the player the first time a chime is needed, so turning a
// chime on (or pressing Test) works when both were off at startup.
// The player is built outside alertMu: reading a WAV file must not hold up onNotify.
func (win *window) ensureChime() (warn string) {
	win.alertMu.Lock()
	have := win.chime != nil
	win.alertMu.Unlock()
	if have {
		return ""
	}
	play, warn := newChime(win.opt.Config.ChimeFile) // ChimeFile only changes on the UI goroutine
	win.alertMu.Lock()
	if win.chime == nil {
		win.chime = play
	}
	win.alertMu.Unlock()
	return warn
}

func shellName(argv []string) string {
	if len(argv) == 0 {
		return "shell"
	}
	b := filepath.Base(argv[0])
	return strings.TrimSuffix(b, filepath.Ext(b))
}

func (win *window) loop() error {
	var ops op.Ops
	for {
		switch e := win.w.Event().(type) {
		case app.DestroyEvent:
			if win.bench != nil {
				win.benchScreen()
				win.bench.Close(privateMB())
			}
			win.shutdown()
			return e.Err
		case app.ConfigEvent:
			win.onConfig(e)
		case app.FrameEvent:
			start := time.Now()
			gtx := app.NewContext(&ops, e)
			if err := win.frameOrShutdown(gtx); err != nil {
				return err
			}
			e.Frame(gtx.Ops)
			if win.bench != nil {
				win.bench.FrameDone(start, time.Now(), win.bf.frame)
				win.bf = benchFrame{}
				if win.bench.ShouldQuit() {
					win.w.Perform(system.ActionClose)
				}
			}
			if win.shouldClose() {
				win.w.Perform(system.ActionClose)
			}
		default:
			if h, ok := viewHandle(e); ok {
				win.hwnd.Store(h)
			}
		}
	}
}

// frameOrShutdown is frame, closing the listener and the shells when it fails.
func (win *window) frameOrShutdown(gtx layout.Context) error {
	err := win.frame(gtx)
	if err != nil {
		win.shutdown()
	}
	return err
}

// shutdown closes the alert listener and every shell, together within shutdownWait.
func (win *window) shutdown() {
	start := time.Now()
	win.cancelRuns()
	var done chan struct{}
	if win.srv != nil {
		done = make(chan struct{})
		go func() { win.srv.Close(); close(done) }()
	}
	if win.mgr != nil {
		win.mgr.CloseAll()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(max(shutdownWait-time.Since(start), 0)):
		}
	}
}

func (win *window) onConfig(e app.ConfigEvent) {
	win.setFocused(e.Config.Focused)
	if m := e.Config.Mode == app.Maximized; m != win.maximized {
		win.maximized = m
		win.w.Invalidate() // the maximize button swaps to restore
	}
}

// perform asks the window for a system action (minimize, maximize, close...).
func (win *window) perform(a system.Action) {
	if win.performFn != nil {
		win.performFn(a)
		return
	}
	win.w.Perform(a)
}

func (win *window) setFocused(f bool) {
	if f == win.focused {
		return
	}
	win.focused = f
	win.snapFocused.Store(f)
	if !f {
		// Windows may never send Leave once another window takes the pointer.
		win.caption.hover = capNone
		win.side.st.hover = 0
	}
	if win.rend != nil {
		win.rend.SetFocused(f)
	}
	if win.mgr != nil {
		if t := win.mgr.Active(); t != nil {
			sendFocus(t, f)
			if f {
				win.staleBranch(t)
			}
		}
	}
	win.w.Invalidate()
}

func sendFocus(t *tabs.Tab, focused bool) {
	if b := input.EncodeFocus(focused, modes(t.Session)); b != nil {
		t.Session.Write(b)
	}
}

func modes(s *term.Session) term.Modes {
	s.Lock()
	defer s.Unlock()
	return s.Applier().Modes()
}

// start launches a tab's shell in dir, or the window's folder when dir is "".
func (win *window) start(env []string, dir string) (ptyx.Session, error) {
	if dir == "" {
		dir = win.opt.Dir
	}
	cols, rows := win.mgr.Size()
	return ptyx.Start(win.opt.Shell, cols, rows, env, dir)
}

func (win *window) newTab() {
	win.dropSideEdit()
	win.hideSettings() // the new tab is shown
	if _, err := win.mgr.New(); err != nil {
		log.Printf("kaze: new tab: %v", err)
		// A windowsgui build has no console, so the status bar is where this can be seen.
		win.notice = "new tab failed: " + err.Error()
		win.noticeUntil = time.Now().Add(noticeFor)
		win.w.Invalidate()
	}
}

// startTheme is the theme named in the config; Load already warned about unknown names.
func startTheme(c config.Config) theme.Theme {
	if t, ok := theme.ByKey(c.Theme); ok {
		return t
	}
	return theme.Default()
}

// applyTheme repaints the terminal and chrome; [colors] overrides still win over the theme.
func (win *window) applyTheme(t theme.Theme) {
	win.pal = applyColors(t.Term, win.opt.Config.Colors)
	win.ui.setTheme(t)
	// Group members match the terminal's colors, [colors] overrides included.
	win.ui.ansi = win.pal.ANSI
	win.rendStale = true // rebuilt next frame with the new palette
	win.w.Invalidate()
}

// rendOptions maps the look settings to the renderer; Load already validated them.
func rendOptions(c config.Config) render.Options {
	o := render.Options{LineScale: 1, Cursor: render.CursorBlock}
	switch c.LineSpacing {
	case "tight":
		o.LineScale = 0.92
	case "roomy":
		o.LineScale = 1.2
	}
	switch c.CursorStyle {
	case "bar":
		o.Cursor = render.CursorBar
	case "underline":
		o.Cursor = render.CursorUnderline
	}
	return o
}

// blinks reports whether the cursor blinks: only while the terminal is in front of the user.
func (win *window) blinks() bool {
	return win.opt.Config.CursorBlink && win.focused && !win.set.shown
}

// blink sets the cursor's phase for now and returns when the next phase starts;
// ok is false when the cursor is steady, so an idle window never wakes for it.
func (win *window) blink(now time.Time) (next time.Time, ok bool) {
	if !win.blinks() {
		win.blinkFrom = time.Time{}
		win.setCursorHidden(false)
		return time.Time{}, false
	}
	if win.blinkFrom.IsZero() || now.Before(win.blinkFrom) {
		win.blinkFrom = now
	}
	n := now.Sub(win.blinkFrom) / blinkEvery
	win.setCursorHidden(n%2 == 1)
	return win.blinkFrom.Add((n + 1) * blinkEvery), true
}

func (win *window) setCursorHidden(h bool) {
	win.cursorHidden = h
	if win.rend != nil {
		win.rend.SetCursorShown(!h)
	}
}

func (win *window) frame(gtx layout.Context) error {
	barH, statusH, pad := gtx.Dp(38), gtx.Dp(24), gtx.Dp(8)
	if !win.custom {
		barH = 0 // the native title bar; the toggle and gear are in the sidebar
	}
	px := gtx.Metric.PxPerSp * win.opt.Config.FontSize
	if win.rend == nil || win.rendStale || px != win.rendPx {
		// A DPI change (another monitor) needs glyphs at the new size.
		win.rend = render.NewRenderer(px, win.pal, rendOptions(win.opt.Config))
		win.rend.SetFocused(win.focused)
		win.rendPx, win.rendStale = px, false
	}
	cw, ch := win.rend.CellSize()
	size := gtx.Constraints.Max
	bottom := max(size.Y-statusH, barH)
	sideRect := image.Rect(0, barH, win.sideWidth(gtx, size.X), bottom)
	termRect := image.Rect(sideRect.Max.X, barH, size.X, bottom)
	grid := image.Rect(termRect.Min.X+pad, termRect.Min.Y+pad/2, termRect.Max.X-pad, termRect.Max.Y-pad/2)
	win.gridMin = grid.Min
	cols, rows := gridSize(grid.Dx(), grid.Dy(), cw, ch)

	if win.mgr == nil {
		// The first shell starts at the real window size, not a guess.
		notify := win.w.Invalidate
		if win.bench != nil {
			notify = func() { win.bench.Output(time.Now()); win.w.Invalidate() }
		}
		win.mgr = tabs.NewManager(win.start, cols, rows, notify)
		win.liveMgr.Store(win.mgr)
		win.mgr.SetScrollback(win.opt.Config.ScrollbackLines)
		if win.srv != nil {
			win.mgr.SetNotify(win.srv.Addr(), win.srv.Token()) // before any tab, so every shell gets it
		}
		var def []string
		if len(win.opt.Config.Shell) > 0 {
			def = ptyx.Resolve(ptyx.DefaultShell())
		}
		if err := win.openStartup(def); err != nil {
			return fmt.Errorf("start %q: %w", win.opt.Shell, err)
		}
		if win.bench != nil {
			for range win.bench.ExtraTabs() {
				win.newTab()
			}
			win.bench.Start(win.w.Invalidate, privateMB)
		}
	}
	win.mgr.ResizeAll(cols, rows)

	win.barEvents(gtx)
	win.dotsEvents(gtx)
	win.sideEvents(gtx)
	win.filterEvents(gtx)
	win.renameEvents(gtx)
	win.toastEvents(gtx)
	win.bannerEvents(gtx)
	active := win.mgr.Active()
	if win.set.shown {
		win.settingsEvents(gtx) // the terminal gets no input; its shell keeps running
	} else if active != nil {
		win.termEvents(gtx, active, cw, ch)
	}
	if win.mgr.Empty() {
		return nil
	}
	active = win.mgr.Active()
	win.switchedTo(active)
	list := win.mgr.Tabs()
	win.pruneSels(list)
	win.pruneLocs(list)
	win.pruneAgents(list)
	win.leaveAgents(list)
	win.pruneCounts(list)
	win.checkCounts(list)
	win.drainCounts()
	win.alerts.prune(func(id tabs.TabID) bool {
		return slices.ContainsFunc(list, func(t *tabs.Tab) bool { return t.ID == id })
	})
	win.publish(win.shownTab(), win.focused)
	win.drainAlerts(gtx.Now)
	win.drainUsage()
	win.visit(win.shownTab())
	win.autoScroll(gtx, active, cw, ch)
	if at, ok := win.wsTick(win.wsNow()); ok {
		gtx.Execute(op.InvalidateCmd{At: at}) // one wake for the next prompt timeout
	}
	sel := win.checkSel(active)
	// After input, so a key typed this frame shows the cursor in it.
	if next, ok := win.blink(gtx.Now); ok {
		gtx.Execute(op.InvalidateCmd{At: next}) // one wake per phase, only while blinking
	}

	ops := gtx.Ops

	// Title bar (kaze's own only): toggle, mark, the empty space that moves the window, gear and caption buttons.
	bar := gtx
	bar.Constraints = layout.Exact(image.Pt(size.X, barH))
	win.drawBar(bar, barH)
	if r := win.bar.Drag; !r.Empty() {
		// Windows answers hit tests here with HTCAPTION: drag, double-click and snap are native.
		area := clip.Rect(r).Push(ops)
		system.ActionInputOp(system.ActionMove).Add(ops)
		area.Pop()
	}
	for _, r := range []image.Rectangle{win.bar.Toggle, win.bar.Gear.Union(win.bar.Buttons[0]).Union(win.bar.Buttons[2])} {
		if r.Empty() {
			continue
		}
		area := clip.Rect(r).Push(ops)
		event.Op(ops, win.capTag)
		area.Pop()
	}

	win.drawSide(gtx, sideRect)

	if win.set.shown {
		win.drawSettings(gtx, termRect) // the renderer is not laid out
		win.wsr.hits = win.wsr.hits[:0]
	} else {
		win.drawTerminal(gtx, active, termRect, grid, &sel)
		win.drawWsBanner(gtx, active, termRect) // after the terminal, so it takes the clicks it covers
	}

	// Status bar.
	st := op.Offset(image.Pt(0, size.Y-statusH)).Push(ops)
	sg := gtx
	sg.Constraints = layout.Exact(image.Pt(size.X, statusH))
	dots := win.ui.layoutStatus(sg, win.statusParts(), statusH)
	win.serverDots = dots.Add(image.Pt(0, size.Y-statusH))
	if at, ok := win.usageWake(gtx.Now); ok {
		gtx.Execute(op.InvalidateCmd{At: at}) // so a reset window leaves an idle bar
	}
	if at, ok := win.noticeWake(time.Now()); ok {
		gtx.Execute(op.InvalidateCmd{At: at}) // so the notice goes away on an idle window
	}
	st.Pop()

	if !win.serverDots.Empty() {
		area := clip.Rect(win.serverDots).Push(ops)
		event.Op(ops, win.dotsTag)
		area.Pop()
	}

	// Toasts float over the terminal, above the status bar; drawn last so they take clicks first.
	if next, _ := win.alerts.expire(gtx.Now); !next.IsZero() {
		gtx.Execute(op.InvalidateCmd{At: next}) // one wake for the next done toast, no ticker
	}
	if win.dialogShown() {
		win.ui.toastHits = win.ui.toastHits[:0] // the confirm dialog answers every click; toasts wait
	} else {
		win.drawToasts(gtx, termRect)
	}
	for _, h := range win.ui.toastHits {
		area := clip.Rect(h.rect).Push(ops)
		event.Op(ops, win.toastTag)
		area.Pop()
	}

	if win.wantFocus {
		gtx.Execute(key.FocusCmd{Tag: win.focusTarget()})
		win.wantFocus = false
	}
	return nil
}

// usageWake is when the status bar's usage next changes on its own.
func (win *window) usageWake(now time.Time) (time.Time, bool) {
	if !win.opt.Config.ShowUsage {
		return time.Time{}, false // nothing shown, nothing to clear at a reset
	}
	at := usage.NextReset(win.acct, now)
	return at, !at.IsZero()
}

// drawTerminal draws the active tab, the only one drawn, and takes the terminal's input.
func (win *window) drawTerminal(gtx layout.Context, active *tabs.Tab, termRect, grid image.Rectangle, sel *render.Selection) {
	ops := gtx.Ops
	paint.FillShape(ops, win.pal.BG, clip.Rect(termRect).Op())
	area := clip.Rect(termRect).Push(ops)
	event.Op(ops, win.termTag)
	area.Pop()
	g := clip.Rect(grid).Push(ops)
	off := op.Offset(grid.Min).Push(ops)
	tg := gtx
	tg.Constraints = layout.Exact(grid.Size())
	if win.bench != nil {
		win.benchBeforeDraw(active)
	}
	if win.rend != nil { // frame builds it first; a nil here must not take the window down
		win.rend.Layout(tg, active.Session, sel)
	}
	if win.bench != nil {
		win.benchAfterDraw(active)
	}
	off.Pop()
	g.Pop()
}

// shownTab is the terminal tab in front of the user, or nil while settings is shown.
func (win *window) shownTab() *tabs.Tab {
	if win.set.shown {
		return nil
	}
	return win.mgr.Active()
}

// switchedTo redraws a newly shown tab in full and moves focus reports to it.
func (win *window) switchedTo(t *tabs.Tab) {
	if t == win.lastTab {
		win.updateTitle(t)
		return
	}
	if win.lastTab != nil && win.focused {
		sendFocus(win.lastTab, false) // a no-op once that tab is closed
	}
	if win.focused {
		sendFocus(t, true)
	}
	win.lastTab = t
	win.staleBranch(t)
	win.endGesture()
	// One renderer serves every tab: dropping its strips frees the hidden tab's
	// buffers and makes the shown tab draw in full.
	if win.rend != nil {
		win.rend.Release()
	}
	win.updateTitle(t)
}

func (win *window) updateTitle(t *tabs.Tab) {
	title := "kaze · " + tabLabel(t, win.defTitle)
	if win.set.shown {
		title = "kaze · Settings"
	}
	if title != win.winTitle {
		win.winTitle = title
		win.w.Option(app.Title(title))
	}
}

// privateMB is the process's private memory, or the Go runtime's own count where the OS number is unavailable.
func privateMB() float64 {
	if mb, ok := processMB(); ok {
		return mb
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return float64(ms.Sys-ms.HeapReleased) / (1 << 20)
}

func (win *window) statusText(n int) string {
	if now := time.Now(); now.Sub(win.memRead) > memEvery {
		win.memMB = privateMB()
		win.memRead = now
	}
	unit := "tabs"
	if n == 1 {
		unit = "tab"
	}
	s := fmt.Sprintf("kaze %.0fMB · %d %s", win.memMB, n, unit)
	if _, ok := win.noticeWake(time.Now()); ok {
		s += " · " + win.notice
	}
	return s
}

// drawBar draws kaze's own top bar: the sidebar toggle, the K mark, the move area,
// the gear and the caption buttons. With the native title bar there is none.
func (win *window) drawBar(gtx layout.Context, barH int) {
	if !win.custom {
		win.bar = barLayout{}
		return
	}
	ops, th := gtx.Ops, win.ui.th
	size := image.Pt(gtx.Constraints.Max.X, barH)
	d := barDims{buttonW: gtx.Dp(46), gearW: gtx.Dp(46), toggleW: gtx.Dp(46), markW: gtx.Dp(30)}
	win.bar = layoutBar(size, d, true)
	paint.FillShape(ops, th.Chrome, clip.Rect{Max: size}.Op())
	paint.FillShape(ops, th.Border, clip.Rect(image.Rect(0, size.Y-1, size.X, size.Y)).Op())
	if m := win.bar.Mark; !m.Empty() {
		px := gtx.Dp(18)
		if mop, img := markImage(px); img != nil {
			cl := clip.Rect(m).Push(ops)
			t := op.Offset(image.Pt(m.Min.X+(m.Dx()-px)/2, m.Min.Y+(m.Dy()-px)/2)).Push(ops)
			mop.Add(ops)
			paint.PaintOp{}.Add(ops)
			t.Pop()
			cl.Pop()
		}
	}
	win.ui.layoutCaptions(gtx, win.bar, win.caption, win.maximized) // empty buttons are skipped
}

// captionEvent feeds one pointer event on the caption buttons and performs a completed click.
func (win *window) captionEvent(e pointer.Event) {
	if win.bench.IgnoreInput() {
		return
	}
	// Only the primary button clicks a caption button.
	if e.Kind == pointer.Press && !e.Buttons.Contain(pointer.ButtonPrimary) ||
		e.Kind == pointer.Release && e.Buttons.Contain(pointer.ButtonPrimary) {
		return
	}
	before := win.caption
	switch c := win.caption.pointer(e.Kind, e.Position.Round(), win.bar); c {
	case capNone:
	case capGear:
		win.openSettings()
	case capToggle:
		win.toggleSidebar()
	default:
		win.perform(actionFor(c, win.maximized))
	}
	if win.caption != before {
		win.w.Invalidate() // one frame for the hover or press color, never a loop
	}
}

func (win *window) barEvents(gtx layout.Context) {
	kinds := pointer.Press | pointer.Release | pointer.Move | pointer.Drag | pointer.Enter | pointer.Leave | pointer.Cancel
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: win.capTag, Kinds: kinds})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok {
			win.captionEvent(e)
		}
	}
}

// focusTarget is who gets the keyboard: the sidebar's rename or filter field, a settings
// text field, the settings page or the terminal.
func (win *window) focusTarget() event.Tag {
	if win.side.renaming != 0 && !win.side.hidden {
		return &win.side.ren
	}
	if win.side.filtering && !win.side.hidden {
		return &win.side.ed
	}
	if win.set.shown {
		if f := win.set.fields[win.set.editing]; f != nil && win.set.editing != "" {
			return &f.ed
		}
		return &win.set.tag
	}
	return win.termTag
}

// dotsEvents shows a run member when the status bar's server dots are clicked.
func (win *window) dotsEvents(gtx layout.Context) {
	if win.dotsTag == nil {
		win.dotsTag = new(int) // before the first draw, so its filter is up when the first click comes
	}
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: win.dotsTag, Kinds: pointer.Press})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok && e.Buttons.Contain(pointer.ButtonPrimary) && !win.bench.IgnoreInput() {
			win.showServer()
		}
	}
}

func (win *window) toastEvents(gtx layout.Context) {
	for {
		ev, ok := gtx.Event(pointer.Filter{Target: win.toastTag, Kinds: pointer.Press})
		if !ok {
			return
		}
		if e, ok := ev.(pointer.Event); ok {
			win.toastEvent(e)
		}
	}
}

func (win *window) termEvents(gtx layout.Context, t *tabs.Tab, cw, ch int) {
	m := modes(t.Session)
	kinds := pointer.Press | pointer.Release | pointer.Scroll | pointer.Drag | pointer.Cancel
	if m.MouseAny {
		kinds |= pointer.Move // only then, so hovering never wakes the idle window
	}
	var batch []event.Event
	for {
		ev, ok := gtx.Event(
			key.FocusFilter{Target: win.termTag},
			key.Filter{Focus: win.termTag, Optional: allMods},
			// Tab is a focus key in Gio and is only delivered to filters that name it.
			key.Filter{Focus: win.termTag, Name: key.NameTab, Optional: allMods},
			transfer.TargetFilter{Target: win.termTag, Type: "application/text"},
			pointer.Filter{Target: win.termTag, Kinds: kinds, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}},
		)
		if !ok {
			break
		}
		batch = append(batch, ev)
	}
	if win.bench != nil {
		batch = win.benchBatch(gtx, t, batch, ch)
	}
	win.route(gtx, batch, cw, ch)
}

// route delivers one frame's terminal input to the active tab.
func (win *window) route(gtx layout.Context, batch []event.Event, cw, ch int) {
	t := win.mgr.Active()
	if t == nil {
		return
	}
	m := modes(t.Session)
	edited := false
	for i, ev := range batch {
		switch e := ev.(type) {
		case key.Event:
			if e.State == key.Press {
				win.blinkFrom = time.Time{} // typing shows the cursor and restarts the blink
			}
			win.onKey(gtx, t, e, m)
			if win.set.shown {
				return // the key showed settings; the rest of the frame's input is not the terminal's
			}
			if a := win.mgr.Active(); a != t {
				// The key opened, closed or switched tabs; the rest of the frame's input goes to the new one.
				win.endGesture()
				if a == nil {
					return
				}
				t, m = a, modes(a.Session)
			}
		case key.CompositionEvent:
			win.composing = compositionActive(e)
		case key.EditEvent:
			edited = true
			win.blinkFrom = time.Time{}
			if win.gate.allow() && editCommits(batch, i, win.composing) && e.Text != "" {
				win.takeOver(t)
				win.benchKey(t)
				t.Session.Write(input.EncodeText(e.Text))
				win.snapToBottom(t.Session)
			}
		case transfer.DataEvent:
			win.paste(t, e, m)
		case pointer.Event:
			win.onPointer(gtx, t, e, m, cw, ch)
		}
	}
	if edited && !win.composing {
		// Gio keeps typed text as editor state; clear it so it doesn't grow forever.
		gtx.Execute(key.SnippetCmd{Tag: win.termTag})
		gtx.Execute(key.SelectionCmd{Tag: win.termTag})
	}
}

func (win *window) onKey(gtx layout.Context, t *tabs.Tab, e key.Event, m term.Modes) {
	if e.State != key.Press {
		win.gate.onKey(false, keyOut{})
		return
	}
	out := routeKey(runtime.GOOS, win.km, e.Name, e.Modifiers, m)
	win.gate.onKey(true, out)
	switch {
	case out.act != actNone:
		win.runShortcut(gtx, out.act)
	case out.data != nil:
		win.takeOver(t)
		win.benchKey(t)
		t.Session.Write(out.data)
		win.snapToBottom(t.Session)
	}
}

// runShortcut does one of kaze's own actions. The terminal and the settings page
// both use it, so a shortcut does the same wherever the keyboard is.
func (win *window) runShortcut(gtx layout.Context, a action) {
	switch a {
	case actNewTab:
		win.newTab()
	case actCloseTab:
		if win.set.shown {
			win.closeSettings()
		} else if t := win.mgr.Active(); t != nil {
			win.closeTab(t.ID)
		}
	case actNextTab:
		win.cycleTab(1)
	case actPrevTab:
		win.cycleTab(-1)
	case actOpenSettings:
		win.openSettings()
	case actToggleSidebar:
		win.toggleSidebar()
	case actNextWaiting:
		win.nextWaiting()
	case actPaste:
		if win.shownTab() != nil {
			gtx.Execute(clipboard.ReadCmd{Tag: win.termTag})
		}
	case actCopy:
		if t := win.shownTab(); t != nil {
			win.copySel(gtx, t)
		}
	}
}

func (win *window) paste(t *tabs.Tab, e transfer.DataEvent, m term.Modes) {
	r := e.Open()
	b, err := io.ReadAll(io.LimitReader(r, maxPaste))
	r.Close()
	if err != nil || len(b) == 0 {
		return
	}
	win.takeOver(t)
	t.Session.Write(input.EncodePaste(string(b), m))
	win.snapToBottom(t.Session)
}

func mouseOn(m term.Modes) bool { return m.MouseX10 || m.MouseButton || m.MouseAny }

func (win *window) onPointer(gtx layout.Context, t *tabs.Tab, e pointer.Event, m term.Modes, cw, ch int) {
	p := e.Position.Round().Sub(win.gridMin)
	cols, rows := win.mgr.Size()
	col, row := gridCell(p, cw, ch, cols, rows)
	switch e.Kind {
	case pointer.Press:
		win.wantFocus = true
		win.dropSideEdit() // the filter keeps its text, a rename is saved
		win.w.Invalidate()
		btn := pressedButton(win.held, e.Buttons)
		first := win.held == 0
		win.held = e.Buttons
		if first {
			// Shift lets the user select even when the app has the mouse.
			win.toApp = mouseOn(m) && e.Modifiers&key.ModShift == 0
			win.gestureTab = t
		}
		win.gestureCol, win.gestureRow = col, row
		if win.toApp {
			win.takeOver(t) // the app sees the click: the owner is using this shell
			t.Session.Write(input.EncodeMouse(btn, true, e.Modifiers, col, row, false, m))
			return
		}
		if btn == 0 {
			win.selectPress(t, p, e.Time, cw, ch)
		}
	case pointer.Release:
		btn := releasedButton(win.held, e.Buttons)
		win.held = e.Buttons
		if win.held == 0 {
			win.gestureTab = nil
		}
		if win.toApp {
			if mouseOn(m) {
				win.takeOver(t)
				t.Session.Write(input.EncodeMouse(btn, false, e.Modifiers, col, row, false, m))
			}
			return
		}
		if btn == 0 {
			win.selectRelease(gtx, t)
		}
	case pointer.Cancel:
		win.held, win.toApp, win.gestureTab = 0, false, nil
		if ts := win.sels[t.ID]; ts != nil {
			ts.end()
		}
	case pointer.Drag, pointer.Move:
		if win.toApp || e.Kind == pointer.Move {
			win.gestureCol, win.gestureRow = col, row
			if b := input.EncodeMouse(xtermButton(e.Buttons), true, e.Modifiers, col, row, true, m); b != nil {
				if e.Buttons != 0 {
					win.takeOver(t) // a drag the app sees is the owner using it; a hover is not
				}
				t.Session.Write(b)
			}
			return
		}
		win.selectDrag(t, p, cw, ch)
	case pointer.Scroll:
		unit, perNotch := float32(120), 3 // Windows reports 120 per wheel notch
		if runtime.GOOS == "darwin" {
			unit, perNotch = float32(ch), 1 // macOS reports pixels
		}
		n := win.wheel.notches(e.Scroll.Y, unit)
		if n == 0 {
			return
		}
		if b := input.EncodeWheel(n, col, row, m); b != nil {
			win.takeOver(t) // the app is reading the wheel, so the owner is using it
			t.Session.Write(b)
			return
		}
		if m.AltScreen {
			return // full-screen apps have no history to scroll
		}
		s := t.Session
		s.Lock()
		s.Screen().SetViewOffset(s.Screen().ViewOffset() + n*perNotch)
		s.Unlock()
		win.w.Invalidate()
	}
}

// snapToBottom returns a scrolled-back view to the live screen when the user types.
func (win *window) snapToBottom(s *term.Session) {
	s.Lock()
	moved := s.Screen().ViewOffset() > 0
	if moved {
		s.Screen().SetViewOffset(0)
	}
	s.Unlock()
	if moved {
		win.w.Invalidate()
	}
}

func (win *window) selOf(id tabs.TabID) *tabSel {
	ts := win.sels[id]
	if ts == nil {
		ts = &tabSel{}
		win.sels[id] = ts
	}
	return ts
}

// pruneSels forgets the selections of closed tabs.
func (win *window) pruneSels(list []*tabs.Tab) {
	if len(win.sels) <= len(list) {
		return
	}
	keep := make(map[tabs.TabID]*tabSel, len(list))
	for _, t := range list {
		if ts := win.sels[t.ID]; ts != nil {
			keep[t.ID] = ts
		}
	}
	win.sels = keep
}

// pointAt maps a grid pixel to an absolute point; call with the session locked.
func (win *window) pointAt(scr *screen.Screen, p image.Point, cw, ch int) render.Point {
	cols, rows := scr.Size()
	return absPoint(p, cw, ch, cols, rows, scr.ScrollbackLen()-scr.ViewOffset())
}

func (win *window) selectPress(t *tabs.Tab, p image.Point, at time.Duration, cw, ch int) {
	s := t.Session
	s.Lock()
	scr := s.Screen()
	ap := win.pointAt(scr, p, cw, ch)
	win.selOf(t.ID).begin(scr, ap, win.clicks.press(at, ap))
	s.Unlock()
	win.dragPos = p
	win.w.Invalidate()
}

func (win *window) selectDrag(t *tabs.Tab, p image.Point, cw, ch int) {
	ts := win.sels[t.ID]
	if ts == nil || !ts.dragging {
		return
	}
	s := t.Session
	s.Lock()
	ts.extend(s.Screen(), win.pointAt(s.Screen(), p, cw, ch))
	s.Unlock()
	win.dragPos = p
	win.w.Invalidate()
}

func (win *window) selectRelease(gtx layout.Context, t *tabs.Tab) {
	ts := win.sels[t.ID]
	if ts == nil || !ts.dragging {
		return
	}
	ts.end()
	if win.opt.Config.CopyOnSelect && ts.sel.Active {
		win.copySel(gtx, t)
	}
}

// autoScroll scrolls the view while a selection drag is held above or below the
// grid. It reports whether it asked for another frame.
func (win *window) autoScroll(gtx layout.Context, t *tabs.Tab, cw, ch int) bool {
	ts := win.sels[t.ID]
	if ts == nil || !ts.dragging || win.toApp || ch <= 0 {
		return false
	}
	_, rows := win.mgr.Size()
	dir := edgeDir(win.dragPos.Y, rows*ch)
	if dir == 0 {
		return false
	}
	if gtx.Now.Sub(win.scrolledAt) >= autoScrollEvery {
		win.scrolledAt = gtx.Now
		s := t.Session
		s.Lock()
		scr := s.Screen()
		if !scr.OnAltScreen() { // full-screen apps have no history to scroll
			scr.SetViewOffset(scr.ViewOffset() - dir)
			ts.extend(scr, win.pointAt(scr, win.dragPos, cw, ch))
		}
		s.Unlock()
	}
	gtx.Execute(op.InvalidateCmd{At: win.scrolledAt.Add(autoScrollEvery)})
	return true
}

// checkSel drops or shifts the active tab's selection when the screen moved
// under it, and returns what to draw.
func (win *window) checkSel(t *tabs.Tab) render.Selection {
	ts := win.sels[t.ID]
	if ts == nil {
		return render.Selection{}
	}
	t.Session.Lock()
	ts.check(t.Session.Screen())
	t.Session.Unlock()
	return ts.sel
}

func (win *window) copySel(gtx layout.Context, t *tabs.Tab) {
	ts := win.sels[t.ID]
	if ts == nil || !ts.sel.Active {
		return
	}
	t.Session.Lock()
	text := render.SelectedText(t.Session.Screen(), ts.sel)
	t.Session.Unlock()
	win.writeClipboard(gtx, text)
}

// writeClipboard copies text, unless it is empty.
func (win *window) writeClipboard(gtx layout.Context, text string) {
	if text == "" {
		return
	}
	if win.toClipboard != nil {
		win.toClipboard(text)
		return
	}
	gtx.Execute(clipboard.WriteCmd{Type: "application/text", Data: io.NopCloser(strings.NewReader(text))})
}

// openFirst opens the first tab with the warnings written in before the shell
// starts. When the configured shell fails and def is set, it retries with def
// and adds a warning that says so.
func openFirst(m *tabs.Manager, shell *[]string, def, warns []string) (*tabs.Tab, string, error) {
	prefeed := func(w []string) func(*term.Session) {
		return func(s *term.Session) { injectWarnings(s, w) }
	}
	t, err := m.NewWith(prefeed(warns))
	if err == nil || len(def) == 0 || slices.Equal(*shell, def) {
		return t, "", err
	}
	warn := fmt.Sprintf("shell %q failed: %v; using the default shell", strings.Join(*shell, " "), err)
	*shell = def
	t, err = m.NewWith(prefeed(append(slices.Clip(warns), warn)))
	if err != nil {
		err = fmt.Errorf("%s; the default shell failed too: %w", warn, err)
	}
	return t, warn, err
}

// noticeWake is when the status-bar notice expires, if one is showing; an expired one is dropped.
func (win *window) noticeWake(now time.Time) (time.Time, bool) {
	if win.notice != "" && !now.Before(win.noticeUntil) {
		win.notice = ""
	}
	return win.noticeUntil, win.notice != ""
}

// endGesture ends a held mouse gesture when the tab changes under it: an app
// that saw the press gets its release, and a selection drag stops, so the
// release landing on another tab can't leave either one stuck.
func (win *window) endGesture() {
	if g := win.gestureTab; g != nil {
		if win.toApp {
			m := modes(g.Session)
			for _, b := range []pointer.Buttons{pointer.ButtonPrimary, pointer.ButtonTertiary, pointer.ButtonSecondary} {
				if win.held.Contain(b) {
					g.Session.Write(input.EncodeMouse(xtermButton(b), false, 0, win.gestureCol, win.gestureRow, false, m))
				}
			}
		}
		if ts := win.sels[g.ID]; ts != nil {
			ts.end()
		}
	}
	win.held, win.toApp, win.gestureTab = 0, false, nil
}
