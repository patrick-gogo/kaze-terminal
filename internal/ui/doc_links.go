package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/gitinfo"
	"github.com/patrick-gogo/kaze-terminal/internal/mdoc"
	"path/filepath"
)

func (win *window) followDocLink(d *docView, dest string) {
	t := mdoc.Classify(dest)
	switch t.Kind {
	case mdoc.TargetWeb:
		win.open(t.URL)
	case mdoc.TargetAnchor:
		win.docAnchor(d, t.Anchor)
	case mdoc.TargetDoc:
		top := gitinfo.Top
		if win.topFn != nil {
			top = win.topFn
		}
		path, e := mdoc.Resolve(t.Path, mdoc.Roots{Dir: filepath.Dir(d.path), Home: win.home, Top: top})
		if e != nil {
			win.note("Can't find " + t.Path)
			return
		}
		nd := win.openDoc(d.session, path)
		if nd != nil && t.Anchor != "" {
			win.docAnchor(nd, t.Anchor)
		}
	}
}
func (win *window) docAnchor(d *docView, slug string) { d.anchor = slug; win.w.Invalidate() }
