package ui

import (
	"strings"
	"time"
	"unicode"

	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

const maxPrompt = 512 // runes typed for one prompt

// cleanPrompt drops control characters, so nothing typed can press Enter,
// and cuts the prompt to maxPrompt runes.
func cleanPrompt(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == unicode.ReplacementChar {
			return -1
		}
		return r
	}, s)
	if r := []rune(s); len(r) > maxPrompt {
		s = string(r[:maxPrompt])
	}
	return s
}

// typeHere reports whether a prompt is typed into cur (true) or a new session
// opens for it (false): typing is only for a Claude session in front that is
// not waiting on a permission, and Shift always asks for a new session.
func typeHere(cur *tabs.Tab, isSession, settingsFront bool, a agentState, m mark, shift bool) bool {
	return cur != nil && isSession && !settingsFront && !shift && a.claude && m != markNeeds
}

// startPrompt starts prompt (a "/skill") the spec 9.3 way, decided now, not
// when the palette opened: typed into the current Claude session without
// Enter, or `claude '<prompt>'` in a new session once its shell is ready.
func (win *window) startPrompt(prompt string, shift bool) {
	p := cleanPrompt(prompt)
	if p == "" {
		return
	}
	cur := win.mgr.Active()
	isSession := false
	if cur != nil {
		_, member := win.isMember(cur)
		isSession = !member
	}
	var a agentState
	var m mark
	if cur != nil {
		a, m = win.agentOf(cur.ID), win.alerts.markOf(cur.ID)
	}
	if typeHere(cur, isSession, win.set.shown || win.docs.shown != 0, a, m, shift) {
		if !win.typeInto(cur, p+" ") {
			return
		}
		win.palette.recent.add(p)
		win.note("Typed " + p + " into " + win.titleOf(cur) + ". Press Enter to send.")
		win.wantFocus = true
		return
	}
	kind := workspaces.KindOf(win.opt.Shell)
	q, err := workspaces.QuoteArg(kind, p)
	if err != nil || kind == workspaces.ShellUnknown {
		win.note("Can't start " + p + " in this shell (" + progName(win.opt.Shell) + "): only PowerShell, zsh, bash and sh are supported.")
		return
	}
	dir := ""
	if cur != nil {
		dir = win.locOf(cur).cwd
	}
	t, err := win.mgr.NewOpts(tabs.StartOpts{Dir: dir})
	if err != nil {
		win.note("new tab failed: " + err.Error())
		return
	}
	win.dropSideEdit()
	win.hideSettings()
	win.selectSession(t.ID)
	l := &wsMember{tab: t.ID, title: p, run: workspaces.TabRun{Line: "claude " + q, Started: win.wsNow()}}
	l.run.Event(workspaces.StartedEvent{})
	if win.launches == nil {
		win.launches = map[tabs.TabID]*wsMember{}
	}
	win.launches[t.ID] = l
	win.mgr.Watch(t.ID, true) // so its first prompt wakes the window
	win.palette.recent.add(p)
	win.note("Opened a new session for " + p + ".")
	win.wantFocus = true
}

// typeInto writes text the way typed keys reach the shell; no Enter.
func (win *window) typeInto(t *tabs.Tab, text string) bool {
	win.takeOver(t)
	if !win.writeInput(t, input.EncodeText(text)) {
		return false
	}
	win.snapToBottom(t.Session)
	return true
}

// titleOf is t's sidebar title.
func (win *window) titleOf(t *tabs.Tab) string {
	for _, r := range sessionRows(buildSide(win.sideIn())) {
		if r.ID == t.ID {
			return r.Title
		}
	}
	return tabLabelWith(t, win.defTitle, nil)
}

// progName is the shell's program name without folder or extension, for
// either path separator.
func progName(argv []string) string {
	if len(argv) == 0 {
		return "shell"
	}
	name := argv[0]
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	return name
}

// launchOf is the pending `claude` line of the new session t, or nil.
func (win *window) launchOf(t *tabs.Tab) *wsMember {
	if t == nil {
		return nil
	}
	return win.launches[t.ID]
}

// bannerMember is what t's waiting banner shows: a workspace member or a
// session waiting to start claude.
func (win *window) bannerMember(t *tabs.Tab) *wsMember {
	if l := win.launchOf(t); l != nil {
		return l
	}
	if m := win.memberOf(t); m != nil {
		return m
	}
	return win.launchOf(t)
}

// launchTick types each new session's `claude` line on its first prompt
// marker, asks the owner after confirmAfter (never sends on the timer), and
// drops launches whose tab closed, exited or was taken over. It returns the
// one deadline still to wake for, if any.
func (win *window) launchTick(now time.Time) (wake time.Time, ok bool) {
	for id, l := range win.launches {
		t := win.mgr.Lookup(id)
		if t == nil {
			delete(win.launches, id)
			continue
		}
		if exited, _ := t.Session.Exited(); exited {
			l.run.Event(workspaces.ClosedEvent{})
		}
		if !l.run.Settled() && !l.run.ShellReady && promptSeen(t.Session) {
			win.wsEvent(l, t, workspaces.ShellReadyEvent{})
		}
		due := l.run.Started.Add(confirmAfter)
		if !l.run.Settled() && !l.run.TimedOut && !now.Before(due) {
			win.wsEvent(l, t, workspaces.ConfirmTimeoutEvent{})
		}
		if l.run.Settled() {
			if win.restore != nil && l.run.State != workspaces.Launched {
				win.restore.Cancel(int(id))
			}
			win.mgr.Watch(id, false)
			delete(win.launches, id)
			continue
		}
		if !l.run.TimedOut && (!ok || due.Before(wake)) {
			wake, ok = due, true
		}
	}
	return wake, ok
}
