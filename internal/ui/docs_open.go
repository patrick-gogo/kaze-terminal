package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/gitinfo"
	"github.com/patrick-gogo/kaze-terminal/internal/mdoc"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"path/filepath"
)

func (win *window) termFolder(t *tabs.Tab) string {
	if t != nil {
		t.Session.Lock()
		cwd := t.Session.Applier().Cwd()
		t.Session.Unlock()
		if cwd != "" {
			return cwd
		}
		if t.Dir != "" {
			return t.Dir
		}
	}
	if win.opt.Dir != "" {
		return win.opt.Dir
	}
	return win.home
}
func (win *window) docParent(t *tabs.Tab, inPane bool) tabs.TabID {
	if t == nil {
		return 0
	}
	if t.Parent != 0 {
		return t.Parent
	}
	if _, member := win.isMember(t); member {
		if inPane {
			return win.paneOwner()
		}
		if ids := win.sessionIDs(); len(ids) > 0 {
			return ids[0]
		}
		return 0
	}
	return t.ID
}
func (win *window) openDocRef(t *tabs.Tab, inPane bool, ref string) {
	id := win.docParent(t, inPane)
	if id == 0 {
		win.note("Open a session to read " + filepath.Base(ref))
		return
	}
	top := gitinfo.Top
	if win.topFn != nil {
		top = win.topFn
	}
	p, e := mdoc.Resolve(ref, mdoc.Roots{Dir: win.termFolder(t), Home: win.home, Top: top})
	if e != nil {
		win.note("Can't find " + ref)
		return
	}
	win.openDoc(id, p)
}
