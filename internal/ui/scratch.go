package ui

import (
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/notify"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// toggleScratch shows or hides the shown session's scratch shell (spec 8.3),
// starting one in the session's folder the first time or after its shell exited.
func (win *window) toggleScratch() {
	owner := win.paneOwner()
	if owner == 0 {
		return // a member or Settings in front: no session to float over
	}
	s := win.mgr.Lookup(owner)
	set := win.fl.set(owner)
	if p := set.scratch(); p != nil {
		t := win.mgr.Lookup(p.id)
		if t != nil {
			if exited, _ := t.Session.Exited(); !exited {
				if !p.hidden {
					p.hidden = true
					if win.fl.keys == p.id {
						win.focusSession()
					}
				} else {
					win.fl.open(owner, p.id, paneScratch)
					win.focusPane(p.id)
				}
				win.w.Invalidate()
				return
			}
			win.mgr.Close(t.ID) // its shell exited: start a fresh one
		}
		win.fl.remove(p.id)
	}
	t, err := win.mgr.NewOpts(tabs.StartOpts{Dir: win.scratchDir(s), Parent: owner})
	if err != nil {
		win.notice = "scratch shell failed: " + err.Error()
		win.noticeUntil = time.Now().Add(noticeFor)
		win.w.Invalidate()
		return
	}
	if _, dropped := win.fl.open(owner, t.ID, paneScratch); dropped != 0 {
		win.mgr.Dock(dropped)
	}
	win.focusPane(t.ID)
}

// closeScratch closes session's scratch shell and forgets its panes.
func (win *window) closeScratch(session tabs.TabID) {
	for _, t := range win.mgr.Tabs() {
		if t.Parent == session {
			win.fl.remove(t.ID)
			win.mgr.Close(t.ID)
		}
	}
	if set := win.fl.sets[session]; set != nil {
		for _, p := range set.panes {
			win.mgr.Dock(p.id)
		}
		delete(win.fl.sets, session)
	}
}

// hookTab is where a hook message from tab id counts: a scratch shell's
// alerts and prompts belong to its session's row; usage stays with the shell
// that sent it. Safe off the UI goroutine (Parent never changes).
func hookTab(m *tabs.Manager, id tabs.TabID, kind notify.Kind) tabs.TabID {
	if kind == notify.KindUsage {
		return id
	}
	if t := m.Lookup(id); t != nil && t.Parent != 0 && m.Has(t.Parent) {
		return t.Parent
	}
	return id
}
