package ui

import (
	"github.com/patrick-gogo/kaze-terminal/internal/mdoc"
	"os"
	"time"
)

const docPoll = time.Second

func (win *window) refreshDoc(d *docView, now time.Time) {
	stamp, e := mdoc.StatFile(d.path)
	if e != nil {
		if os.IsNotExist(e) {
			d.removed = true
		}
		return
	}
	if stamp == d.file.Stamp && !d.removed {
		return
	}
	f, e := mdoc.Load(d.path)
	if e != nil {
		return
	}
	parsed := mdoc.Parse(f.Data)
	win.fitDoc(parsed)
	d.file, d.doc, d.loadedAt, d.removed = f, parsed, now, false
}
func (win *window) docTick(now time.Time) (time.Time, bool) {
	d := win.shownDoc()
	if d == nil {
		return time.Time{}, false
	}
	if !now.Before(win.docs.checked.Add(docPoll)) {
		win.refreshDoc(d, now)
		win.docs.checked = now
	}
	return win.docs.checked.Add(docPoll), true
}
