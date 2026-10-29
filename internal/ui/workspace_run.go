package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// confirmAfter is how long a workspace tab waits for its first prompt marker
// before asking the owner whether the shell is ready.
const confirmAfter = 3 * time.Second

// wsRun is one run of a workspace: its tabs in recipe order and the context
// its probes run under. UI goroutine only.
type wsRun struct {
	id         uint64
	workspace  string
	name       string
	members    []*wsMember
	ctx        context.Context
	cancel     context.CancelFunc
	probes     map[string]context.CancelFunc // one context per dependency with waiting consumers
	summarized bool
}

type wsMember struct {
	tab        tabs.TabID // 0 when the shell could not start
	title      string
	dir        string // where its shell started, for the group menu
	run        workspaces.TabRun
	hideBanner bool // the owner dismissed a failure banner
}

// wsResult is one probe's answer for every member of a run waiting on dep.
type wsResult struct {
	run uint64
	dep string
	err error
}

// wsRunner is the window's workspace-run plumbing; UI goroutine only, except
// results, which probe goroutines send on.
type wsRunner struct {
	next    uint64
	results chan wsResult
	probes  workspaces.Probes // zero value probes the real system; tests use fakes
	now     func() time.Time
	tag     int
	hits    []bannerHit
	hitTab  tabs.TabID // the tab the banner hits belong to
}

func (win *window) wsNow() time.Time {
	if win.wsr.now != nil {
		return win.wsr.now()
	}
	return time.Now()
}

// shouldClose is true once the last tab closed. The loop asks only after a
// frame, and a run opens all its tabs within one, so it never sees a run half open.
func (win *window) shouldClose() bool {
	return win.mgr != nil && win.mgr.Empty()
}

func (win *window) wsNotice(msg string) {
	win.notice = msg
	win.noticeUntil = time.Now().Add(noticeFor)
	if win.w != nil {
		win.w.Invalidate()
	}
}

// wsPlan is one recipe tab after preflight.
type wsPlan struct {
	tab  workspaces.Tab
	dir  string
	line string
}

// preflight checks the whole recipe before any tab opens and names the tab at fault.
func (win *window) preflight(ws workspaces.Workspace) ([]wsPlan, error) {
	if errs := (workspaces.Document{Workspaces: []workspaces.Workspace{ws}}).Validate(win.home); len(errs) > 0 {
		return nil, errs[0]
	}
	kind := workspaces.KindOf(win.opt.Shell)
	var plan []wsPlan
	for _, t := range ws.Tabs {
		dir, err := workspaces.ExpandFolder(t.Folder, win.home)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.Title, err)
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("%s: the folder %s does not exist", t.Title, dir)
		}
		line, err := workspaces.JoinLine(kind, t.Commands)
		if err != nil {
			return nil, fmt.Errorf("%s: %s: %w", t.Title, shellName(win.opt.Shell), err)
		}
		if err := workspaces.CheckCommands(kind, t.Commands); err != nil {
			return nil, fmt.Errorf("%s: %w", t.Title, err)
		}
		plan = append(plan, wsPlan{tab: t, dir: dir, line: line})
	}
	return plan, nil
}

// openRun is the run of workspace id that still has a tab open, or nil.
func (win *window) openRun(id string) *wsRun {
	for _, r := range win.runs {
		if r.workspace != id {
			continue
		}
		for _, m := range r.members {
			if m.tab != 0 && win.mgr.Has(m.tab) {
				return r
			}
		}
	}
	return nil
}

// runWorkspace opens workspace id's tabs and starts waiting for each tab's
// dependency and prompt. A run that still has open tabs is focused instead.
func (win *window) runWorkspace(id string) {
	if win.cfgPath == "" {
		win.wsNotice("workspaces are unavailable: kaze has no settings folder")
		return
	}
	doc, _, err := workspaces.Load(workspaces.Path(win.cfgPath))
	if err != nil {
		win.wsNotice("workspaces: " + err.Error())
		return
	}
	ws, ok := doc.ByID(id)
	if !ok {
		win.wsNotice("that workspace no longer exists")
		return
	}
	if r := win.openRun(id); r != nil {
		for _, m := range r.members {
			if m.tab != 0 && win.mgr.Has(m.tab) {
				win.selectTab(m.tab)
				break
			}
		}
		return
	}
	plan, err := win.preflight(ws)
	if err != nil {
		win.wsNotice(ws.Name + ": " + err.Error())
		return
	}
	run := win.launch(ws, plan, nil)
	if win.openRun(ws.ID) == nil {
		// No tab means no banner or summary card either; say it here.
		why := "no shell could start"
		if len(run.members) > 0 && run.members[0].run.Err != "" {
			why = run.members[0].run.Err
		}
		win.wsNotice(ws.Name + ": " + why)
	}
}

// launch opens the tabs of a preflighted workspace and starts its probes; the
// first tab that starts gets warns written in, as the usual first tab does.
// It returns the run.
func (win *window) launch(ws workspaces.Workspace, plan []wsPlan, warns []string) *wsRun {
	win.wsr.next++
	run := &wsRun{id: win.wsr.next, workspace: ws.ID, name: ws.Name}
	run.ctx, run.cancel = context.WithCancel(context.Background())
	if win.runs == nil {
		win.runs = map[uint64]*wsRun{}
	}
	win.runs[run.id] = run
	now := win.wsNow()
	var first tabs.TabID
	for i, p := range plan {
		m := &wsMember{title: p.tab.Title, dir: p.dir, run: workspaces.TabRun{Line: p.line, Wait: p.tab.Wait, Port: p.tab.Port, Started: now}}
		run.members = append(run.members, m)
		opts := tabs.StartOpts{
			Dir: p.dir, Title: p.tab.Title, Color: p.tab.Color,
			Group: &tabs.Group{Workspace: ws.ID, Name: ws.Name, Run: run.id, Member: i},
		}
		if first == 0 && len(warns) > 0 {
			opts.Prefeed = func(s *term.Session) { injectWarnings(s, warns) }
		}
		t, err := win.mgr.NewOpts(opts)
		if err != nil {
			m.run.Event(workspaces.StartFailedEvent{Err: "The shell could not start: " + err.Error()})
			continue
		}
		m.tab = t.ID
		m.run.Event(workspaces.StartedEvent{})
		if !m.run.Settled() {
			win.mgr.Watch(t.ID, true) // so its first prompt wakes the window
		}
		if first == 0 {
			first = t.ID
		}
	}
	if first != 0 {
		win.selectTab(first) // later results never steal focus
	}
	win.startProbes(run)
	if win.w != nil {
		win.w.Invalidate()
	}
	return run
}

func depKey(r *workspaces.TabRun) string {
	if r.Wait == workspaces.WaitPort {
		return "port:" + strconv.Itoa(r.Port)
	}
	return string(r.Wait)
}

// startProbes runs one probe per distinct dependency; members waiting on the
// same one share its answer.
func (win *window) startProbes(run *wsRun) {
	if win.wsr.results == nil {
		win.wsr.results = make(chan wsResult, 64)
	}
	started := map[string]bool{}
	for _, m := range run.members {
		r := &m.run
		if r.Settled() || r.DepReady {
			continue
		}
		key := depKey(r)
		if started[key] {
			continue
		}
		started[key] = true
		wait, port := r.Wait, r.Port
		ctx, cancel := context.WithCancel(run.ctx)
		if run.probes == nil {
			run.probes = map[string]context.CancelFunc{}
		}
		run.probes[key] = cancel
		probes, results, w := win.wsr.probes, win.wsr.results, win.w
		go func() {
			var err error
			if wait == workspaces.WaitPort {
				err = probes.WaitPort(ctx, port)
			} else {
				err = probes.WaitDocker(ctx)
			}
			select {
			case results <- wsResult{run: run.id, dep: key, err: err}:
			case <-ctx.Done():
				return
			}
			if w != nil {
				w.Invalidate()
			}
		}()
	}
}

func depFailure(r *workspaces.TabRun, err error) string {
	switch {
	case r.Wait == workspaces.WaitPort:
		return fmt.Sprintf("Port %d never answered.", r.Port)
	case errors.Is(err, workspaces.ErrDockerMissing):
		return "Docker is not installed."
	}
	return "Docker never became ready."
}

// wsDrain applies the probe answers that arrived since the last frame.
func (win *window) wsDrain() {
	for {
		select {
		case res := <-win.wsr.results:
			run := win.runs[res.run]
			if run == nil || errors.Is(res.err, context.Canceled) {
				continue
			}
			if cancel := run.probes[res.dep]; cancel != nil {
				cancel()
				delete(run.probes, res.dep)
			}
			for _, m := range run.members {
				if m.run.Settled() || depKey(&m.run) != res.dep {
					continue
				}
				var e workspaces.Event = workspaces.DepReadyEvent{}
				if res.err != nil {
					e = workspaces.DepFailedEvent{Err: depFailure(&m.run, res.err)}
				}
				win.wsEvent(m, win.mgr.Lookup(m.tab), e)
			}
		default:
			return
		}
	}
}

// wsEvent feeds e to m and types its line, once, when the machine says so.
func (win *window) wsEvent(m *wsMember, t *tabs.Tab, e workspaces.Event) {
	if m.run.Event(e) {
		res := term.Closed
		if t != nil {
			res = t.Session.Submit([]byte(m.run.Line + "\r"))
		}
		if res != term.Accepted {
			m.run.Event(workspaces.SubmitFailedEvent{Err: "Could not send the commands to this tab."})
		}
	}
	if m.run.Settled() && t != nil {
		win.mgr.Watch(t.ID, false)
	}
	if win.w != nil {
		win.w.Invalidate()
	}
}

// promptSeen reports the shell's first prompt marker: an OSC 7 folder report
// or an OSC 133;A mark. A fresh session starts both counts at 0.
func promptSeen(s *term.Session) bool {
	s.Lock()
	defer s.Unlock()
	return s.Applier().CwdSeq() > 0 || s.Applier().PromptSeq() > 0
}

// wsTick advances every run: probe answers, prompts, exits, closed tabs and
// the prompt timeout. It returns the next time it needs a frame, if any.
func (win *window) wsTick(now time.Time) (wake time.Time, ok bool) {
	if len(win.runs) == 0 {
		return time.Time{}, false
	}
	win.wsDrain()
	for id, run := range win.runs {
		open := 0
		for _, m := range run.members {
			if m.tab == 0 {
				continue
			}
			t := win.mgr.Lookup(m.tab)
			if t == nil {
				if !m.run.Settled() {
					win.wsEvent(m, nil, workspaces.ClosedEvent{})
				}
				continue
			}
			open++
			if m.run.Settled() {
				continue
			}
			if exited, _ := t.Session.Exited(); exited {
				win.wsEvent(m, t, workspaces.ExitedEvent{})
				continue
			}
			if !m.run.ShellReady && promptSeen(t.Session) {
				win.wsEvent(m, t, workspaces.ShellReadyEvent{})
			}
			due := m.run.Started.Add(confirmAfter)
			if !m.run.Settled() && !m.run.TimedOut && !now.Before(due) {
				win.wsEvent(m, t, workspaces.ConfirmTimeoutEvent{})
			}
			if !m.run.Settled() && !m.run.TimedOut && (!ok || due.Before(wake)) {
				wake, ok = due, true
			}
		}
		// A run can still be waiting for a different tab's prompt after every
		// consumer of a dependency closes or takes over. Stop that dependency
		// immediately rather than polling until the entire run settles.
		for dep, cancel := range run.probes {
			needed := false
			for _, m := range run.members {
				if !m.run.Settled() && !m.run.DepReady && depKey(&m.run) == dep {
					needed = true
					break
				}
			}
			if !needed {
				cancel()
				delete(run.probes, dep)
			}
		}
		if !run.summarized && run.settled() {
			run.summarized = true
			run.cancel()
			win.summarize(run, now)
		}
		if open == 0 {
			run.cancel()
			delete(win.runs, id)
		}
	}
	return wake, ok
}

func (r *wsRun) settled() bool {
	for _, m := range r.members {
		if !m.run.Settled() {
			return false
		}
	}
	return true
}

func (r *wsRun) states() []workspaces.TabRun {
	out := make([]workspaces.TabRun, len(r.members))
	for i, m := range r.members {
		out[i] = m.run
	}
	return out
}

// summarize shows the run's one summary toast, on its first open tab.
func (win *window) summarize(run *wsRun, now time.Time) {
	var at tabs.TabID
	titles := make([]string, len(run.members))
	for i, m := range run.members {
		titles[i] = m.title
		if at == 0 && m.tab != 0 && win.mgr.Has(m.tab) {
			at = m.tab
		}
	}
	if at == 0 {
		return // nothing left to show it on
	}
	text, ok := workspaces.Summary(run.name, run.states(), titles)
	t := toast{Tab: at, Run: run.id, Title: run.name, Text: text[len(run.name):], Kind: notify.KindNeeds}
	if ok {
		t.Kind = notify.KindDone
		d := doneToastFor(win.opt.Config.DoneToast)
		if d == 0 {
			d = doneToastFor("") // the summary of a good run never waits to be seen
		}
		t.Until = now.Add(d)
	}
	win.alerts.addRun(t)
}

// runState is the run's label for its tab group and whether it needs the owner.
func (win *window) runState(run uint64) (label string, needs bool) {
	r := win.runs[run]
	if r == nil {
		return "", false
	}
	return workspaces.RunLabel(r.states())
}

// memberOf is the waiting-run member behind tab t, or nil.
func (win *window) memberOf(t *tabs.Tab) *wsMember {
	if t == nil || t.Group == nil {
		return nil
	}
	r := win.runs[t.Group.Run]
	if r == nil || t.Group.Member < 0 || t.Group.Member >= len(r.members) {
		return nil
	}
	if m := r.members[t.Group.Member]; m.tab == t.ID {
		return m
	}
	return nil
}

// takeOver cancels t's automatic launch because the owner is typing into it.
// Call it before the input is forwarded.
func (win *window) takeOver(t *tabs.Tab) {
	if m := win.memberOf(t); m != nil && !m.run.Settled() {
		win.wsEvent(m, t, workspaces.UserInputEvent{})
	}
}

// cancelRuns stops every run's probes; for shutdown.
func (win *window) cancelRuns() {
	for _, r := range win.runs {
		r.cancel()
	}
}
