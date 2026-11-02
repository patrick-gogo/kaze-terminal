package ui

import (
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"github.com/patrick-gogo/kaze-terminal/internal/gitinfo"
	"github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/mdoc"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"image"
	"path/filepath"
	"runtime"
	"time"
)

type docID uint64
type docView struct {
	id                            docID
	session                       tabs.TabID
	path, key, name, folder, repo string
	file                          mdoc.File
	loadedAt                      time.Time
	removed                       bool
	scroll                        int
	anchor                        string
}
type docsState struct {
	list        []*docView
	shown, last docID
	tag         int
}

func (win *window) openDoc(session tabs.TabID, path string) *docView {
	if win.mgr.Lookup(session) == nil {
		return nil
	}
	k := mdoc.PathKey(path, runtime.GOOS)
	for _, d := range win.docs.list {
		if d.session == session && d.key == k {
			win.showDoc(d.id)
			return d
		}
	}
	f, e := mdoc.Load(path)
	if e != nil {
		win.note("Can't open " + filepath.Base(path) + ": " + e.Error())
		return nil
	}
	dir := filepath.Dir(path)
	top := gitinfo.Top
	if win.topFn != nil {
		top = win.topFn
	}
	folder := tildeHome(dir, win.home)
	if root := top(dir); root != "" {
		if rel, e := filepath.Rel(root, dir); e == nil {
			folder = rel
			if folder == "." {
				folder = ""
			}
		}
	}
	repoFn := gitinfo.Repo
	if win.repoFn != nil {
		repoFn = win.repoFn
	}
	_, repo := repoFn(dir)
	if repo == "" {
		repo = filepath.Base(dir)
	}
	win.docs.last++
	d := &docView{id: win.docs.last, session: session, path: path, key: k, name: filepath.Base(path), folder: folder, repo: repo, file: f, loadedAt: time.Now()}
	win.docs.list = append(win.docs.list, d)
	win.showDoc(d.id)
	return d
}
func (win *window) showDoc(id docID) {
	d := win.docByID(id)
	if d == nil {
		return
	}
	win.dropSideEdit()
	win.hideSettings()
	win.closeBoard()
	win.mgr.SetActive(d.session)
	win.docs.shown = id
	win.endGesture()
	win.wantFocus = true
	win.side.reveal = true
	win.w.Invalidate()
}
func (win *window) hideDoc() {
	if win.docs.shown == 0 {
		return
	}
	win.docs.shown = 0
	win.wantFocus = true
	win.w.Invalidate()
}
func (win *window) closeDoc(id docID) {
	for i, d := range win.docs.list {
		if d.id == id {
			copy(win.docs.list[i:], win.docs.list[i+1:])
			win.docs.list[len(win.docs.list)-1] = nil
			win.docs.list = win.docs.list[:len(win.docs.list)-1]
			break
		}
	}
	if win.docs.shown == id {
		win.hideDoc()
	}
	win.w.Invalidate()
}
func (win *window) closeSessionDocs(session tabs.TabID) {
	for i := len(win.docs.list) - 1; i >= 0; i-- {
		if win.docs.list[i].session == session {
			win.closeDoc(win.docs.list[i].id)
		}
	}
}
func (win *window) pruneDocs(list []*tabs.Tab) {
	for i := len(win.docs.list) - 1; i >= 0; i-- {
		d := win.docs.list[i]
		found := false
		for _, t := range list {
			if t.ID == d.session {
				found = true
				break
			}
		}
		if !found {
			win.closeDoc(d.id)
		}
	}
}
func (win *window) docByID(id docID) *docView {
	for _, d := range win.docs.list {
		if d.id == id {
			return d
		}
	}
	return nil
}
func (win *window) shownDoc() *docView { return win.docByID(win.docs.shown) }
func (win *window) sessionDocs(id tabs.TabID) []*docView {
	var out []*docView
	for _, d := range win.docs.list {
		if d.session == id {
			out = append(out, d)
		}
	}
	return out
}
func (win *window) drawDoc(gtx layout.Context, d *docView, r image.Rectangle) {
	paint.FillShape(gtx.Ops, win.pal.BG, clip.Rect(r).Op())
	a := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &win.docs.tag)
	a.Pop()
}
func (win *window) docEvents(gtx layout.Context) {
	tag := &win.docs.tag
	for {
		ev, ok := gtx.Event(key.FocusFilter{Target: tag}, key.Filter{Focus: tag, Optional: allMods}, pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Scroll, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			return
		}
		if win.bench.IgnoreInput() {
			continue
		}
		switch e := ev.(type) {
		case key.Event:
			if e.State == key.Press {
				if a, ok := win.km[input.Shortcut{Name: e.Name, Mods: e.Modifiers}]; ok {
					win.runShortcut(gtx, a)
				}
			}
		case key.EditEvent:
			gtx.Execute(key.SnippetCmd{Tag: tag})
			gtx.Execute(key.SelectionCmd{Tag: tag})
		}
		if win.docs.shown == 0 {
			return
		}
	}
}
