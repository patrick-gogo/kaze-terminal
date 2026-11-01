package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/sessionstate"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/term"
	"github.com/patrick-gogo/kaze-terminal/internal/update"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// restoreWaitDefault bounds how long an updated Kaze waits for the old one to close.
const restoreWaitDefault = 8 * time.Second

// restorePlan is prepared before entering the rendering loop.
type restorePlan struct {
	entry     sessionstate.Entry
	dir, line string
	warnings  []string
}

// initRestore runs before the first shell. Only Run (and isolated tests) calls it.
func (win *window) initRestore() {
	if win.bench != nil {
		return
	}
	if win.restorePath == "" {
		if win.cfgPath == "" {
			return
		}
		win.restorePath = filepath.Join(filepath.Dir(win.cfgPath), "sessions.json")
	}
	var err error
	wait := win.restoreWaitFor
	if wait <= 0 {
		wait = restoreWaitDefault
	}
	deadline := time.Now().Add(wait)
	for {
		win.restore, err = sessionstate.Open(win.restorePath)
		if err == nil || !errors.Is(err, sessionstate.ErrOwned) || !win.restoreWait || !time.Now().Before(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		// Only a stuck previous owner stops an updated Kaze: opening anyway could
		// resume conversations it still runs. Other errors open without restoring.
		if win.restoreWait && errors.Is(err, sessionstate.ErrOwned) {
			win.restoreFatal = errors.New("Kaze updated, but the previous Kaze window is still closing, so its sessions were not reopened. " +
				"They are saved: close the other Kaze window, then open Kaze again")
		}
		win.opt.Warnings = append(win.opt.Warnings, "Session restoration unavailable: "+err.Error())
		return
	}
	win.restore.Freeze(true)
	if !win.opt.Config.RestoreSessions {
		win.restore.SetEnabled(false)
		win.restore.Flush()
		return
	}
	win.restoreSaved, err = sessionstate.Load(win.restorePath)
	if err != nil {
		win.restoreSaved = sessionstate.Snapshot{}
	}
	if err != nil && !os.IsNotExist(err) {
		msg := "Could not read saved sessions: " + err.Error()
		// Keep the unreadable file (a newer Kaze's, or damaged): the first save would replace it.
		if bak := win.restorePath + ".bak"; os.Rename(win.restorePath, bak) == nil {
			msg += "; kept a copy at " + bak
		}
		win.opt.Warnings = append(win.opt.Warnings, msg)
	}
	if strings.TrimSpace(win.opt.Workspace) == "" {
		for _, e := range win.restoreSaved.Entries {
			p := restorePlan{entry: e, dir: e.Cwd}
			if st, err := os.Stat(p.dir); p.dir == "" || err != nil || !st.IsDir() {
				p.warnings = append(p.warnings, "folder unavailable: "+p.dir+"; opened a shell in the default folder")
				p.dir = win.opt.Dir
			} else if e.Agent != "" || e.Session != "" {
				var err error
				p.line, err = win.resumeLine(e)
				if err != nil {
					p.warnings = append(p.warnings, err.Error())
				}
			}
			win.restorePlans = append(win.restorePlans, p)
		}
	}

}
func (win *window) trackRestore() {
	if win.restore == nil || win.mgr == nil {
		return
	}
	win.mgr.SetStateObserver(win.captureRestore)
	win.restore.Freeze(false)
	win.captureRestore()
	for id, name := range win.names {
		win.restore.Rename(int(id), name)
	}
}

// captureRestore is safe on notifier goroutines, including while minimized.
func (win *window) captureRestore() {
	win.restoreCaptureMu.Lock()
	defer win.restoreCaptureMu.Unlock()
	win.captureRestoreLocked()
}
func (win *window) captureRestoreLocked() {
	if win.restore == nil || win.mgr == nil {
		return
	}
	list, active := win.mgr.State()
	live := make([]sessionstate.Live, 0, len(list))
	for _, t := range list {
		dir := t.Dir
		if dir == "" {
			dir = win.opt.Dir
		}
		l := sessionstate.Live{ID: int(t.ID), Cwd: dir, FixedTitle: t.FixedTitle, Color: t.Color, Seq: t.Seq, Exited: t.Exited}
		if g := t.Group; g != nil {
			l.Workspace, l.WorkspaceName, l.Run, l.Member = g.Workspace, g.Name, g.Run, g.Member
		}
		live = append(live, l)
	}
	win.restore.SetTabs(live, int(active))
}

// freezeRestore orders the final capture against in-flight origin hook events.
func (win *window) freezeRestore() {
	win.restoreCaptureMu.Lock()
	defer win.restoreCaptureMu.Unlock()
	if win.restore != nil {
		win.captureRestoreLocked()
		win.restore.Freeze(true)
	}
}
func (win *window) closeRestore() { win.finishRestore(true) }

// finishRestore saves and stops restoration. When shells did not close in time
// it keeps ownership until the process exits, so an updated Kaze waiting on it
// cannot resume a conversation the old shells still run.
func (win *window) finishRestore(released bool) {
	if win.restore == nil {
		return
	}
	if win.mgr != nil {
		win.captureRestore()
		win.mgr.SetStateObserver(nil)
	}
	win.restore.Freeze(true)
	closeStore := win.restore.Close
	if !released {
		closeStore = win.restore.CloseKeepingOwnership
	}
	if err := closeStore(); err != nil {
		fmt.Fprintln(os.Stderr, "kaze: could not save sessions:", err)
	}
}
func (win *window) originIdentity(m notify.Msg, t *tabs.Tab) {
	if win.restore == nil || t == nil || t.Parent != 0 || subKind(m.Kind) || m.Kind == notify.KindUsage {
		return
	}
	win.restoreCaptureMu.Lock()
	defer win.restoreCaptureMu.Unlock()
	win.captureRestoreLocked()
	agent := m.Agent
	if agent == "" {
		agent = "claude"
	}
	win.restore.Note(int(t.ID), agent, m.Session, m.Transcript, m.Cwd, shellSeq(t), m.Kind == notify.KindSession)
}

// restoreAgentQuit forgets a tab's conversation once its agent left the front.
func (win *window) restoreAgentQuit(id tabs.TabID) {
	if win.restore != nil {
		win.restore.Cancel(int(id))
	}
}
func (win *window) restoreWarning(s string) {
	win.opt.Warnings = append(win.opt.Warnings, "Session restore: "+s)
}
func (win *window) resumeLine(e sessionstate.Entry) (string, error) {
	if !sessionstate.ValidID(e.Session) {
		return "", errors.New("conversation ID is unavailable; opened a shell")
	}
	if e.Agent != "claude" && e.Agent != "codex" {
		return "", errors.New("unsupported agent; opened a shell")
	}
	if e.Transcript != "" {
		if st, err := os.Stat(e.Transcript); err != nil || !st.Mode().IsRegular() {
			return "", errors.New("saved conversation file is unavailable; opened a shell")
		}
	}
	kind := workspaces.KindOf(win.opt.Shell)
	if kind == workspaces.ShellUnknown {
		return "", errors.New("this shell cannot launch resumed agents automatically")
	}
	// The ready interactive shell owns command resolution. Finder and updater
	// launches can have a minimal PATH that omits agents installed by the user.
	// If the command is missing, the shell displays its error in this tab.
	// Only the two allowlisted names above reach this point. Keep the command
	// word bare so interactive POSIX shells expand local-installer aliases.
	q := e.Agent

	id, _ := workspaces.QuoteArg(kind, e.Session)
	if e.Agent == "claude" {
		return q + " --resume " + id, nil
	}
	return q + " resume " + id, nil
}
func (win *window) restoreStartup() bool {
	if win.restore == nil || !win.opt.Config.RestoreSessions || strings.TrimSpace(win.opt.Workspace) != "" || len(win.restoreSaved.Entries) == 0 {
		return false
	}
	var selected tabs.TabID
	groups := map[uint64]*wsRun{}
	// Startup warnings go to the first tab, as on the workspace path; each tab
	// also shows its own restore warnings, never another tab's.
	startWarns := append([]string(nil), win.opt.Warnings...)
	first := true
	for i, p := range win.restorePlans {
		e, dir := p.entry, p.dir
		var warns []string
		if first {
			warns = startWarns
		}
		for _, warn := range p.warnings {
			win.restoreWarning(warn)
			warns = append(warns, win.opt.Warnings[len(win.opt.Warnings)-1])
		}
		var group *tabs.Group
		if e.Workspace != "" {
			r := groups[e.Run]
			if r == nil {
				win.wsr.next++
				ctx, cancel := context.WithCancel(context.Background())
				r = &wsRun{id: win.wsr.next, workspace: e.Workspace, name: e.WorkspaceName, ctx: ctx, cancel: cancel}
				groups[e.Run] = r
				if win.runs == nil {
					win.runs = map[uint64]*wsRun{}
				}
				win.runs[r.id] = r
			}
			group = &tabs.Group{Workspace: e.Workspace, Name: e.WorkspaceName, Run: r.id, Member: len(r.members)}
		}
		resume := p.line
		t, err := win.mgr.NewOpts(tabs.StartOpts{Dir: dir, Title: e.FixedTitle, Color: e.Color, Group: group, Prefeed: func(s *term.Session) { injectWarnings(s, warns) }})
		if err != nil {
			win.restoreWarning("could not open " + dir + ": " + err.Error())
			continue
		}
		first = false
		if e.Title != "" {
			if win.names == nil {
				win.names = map[tabs.TabID]string{}
			}
			win.names[t.ID] = e.Title
		}
		if i == win.restoreSaved.Active || selected == 0 {
			selected = t.ID
		}
		if group != nil {
			r := win.runs[group.Run]
			m := &wsMember{tab: t.ID, title: e.FixedTitle, dir: dir}
			m.run.Event(workspaces.StartedEvent{})
			r.members = append(r.members, m)
		}
		if resume != "" {
			l := &wsMember{tab: t.ID, title: "Resume " + e.Agent, run: workspaces.TabRun{Line: resume, Started: win.wsNow()}}
			l.run.Event(workspaces.StartedEvent{})
			if win.launches == nil {
				win.launches = map[tabs.TabID]*wsMember{}
			}
			win.launches[t.ID] = l
			win.restore.Pending(int(t.ID), e, shellSeq(t))
			win.mgr.Watch(t.ID, true)
		}
	}
	win.mgr.SetActive(selected)
	if len(win.opt.Warnings) > 0 {
		win.note(win.opt.Warnings[len(win.opt.Warnings)-1])
	}
	return !win.mgr.Empty()
}

// restartForUpdate captures on the UI goroutine, flushes off it, then relaunches.
// The child waits on the lifetime lock, which shutdown releases after PTYs close.
func (win *window) restartForUpdate(target update.Target, relaunch func(update.Target) error) {
	win.restartBusy = true
	win.freezeRestore()
	go func() {
		var err error
		if win.restore != nil {
			err = win.restore.Flush()
		}
		if err == nil {
			err = relaunch(target)
		}
		win.post(func(win *window) {
			if err != nil {
				if win.restore != nil {
					win.restore.Freeze(false)
				}
				win.restartBusy = false
				win.upd.busy = ""
				win.upd.err = "Restart failed: " + err.Error()
				return
			}
			win.upd.busy = "Restarting…"
			win.upd.quit()
		})
	}()
}
