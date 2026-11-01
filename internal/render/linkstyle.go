package render

import "github.com/patrick-gogo/kaze-terminal/internal/screen"

// linkFG is the colour of a web link drawn in the default colour: the theme's blue.
var linkFG = screen.Indexed(4)

// queueLinkRows finds the visible links when anything changed and queues every
// row that has a link now or had one, so a link's style follows edits on any of
// its rows (a wrapped link spans several). Call with the session locked.
func (r *Renderer) queueLinkRows(scr *screen.Screen, base int) []LinkSpan {
	if len(r.snapRows) == 0 {
		return nil // nothing changed, so no link did
	}
	spans := r.links.Visible(scr, base, r.rows)
	clear(r.linkNow)
	for _, sp := range spans {
		if row := sp.Row - base; row >= 0 && row < r.rows {
			r.linkNow[row] = true
		}
	}
	clear(r.linkQueued)
	for _, row := range r.snapRows {
		r.linkQueued[row] = true
	}
	for row := range r.rows {
		if (r.linkNow[row] || r.linkWas[row]) && !r.linkQueued[row] {
			r.linkQueued[row] = true
			r.snapRows = append(r.snapRows, row)
		}
	}
	copy(r.linkWas, r.linkNow)
	return spans
}

// styleLinks underlines the copied link cells and draws those in the default
// colour in linkFG; a colour the program chose stays.
func (r *Renderer) styleLinks(spans []LinkSpan, base int) {
	for _, sp := range spans {
		row := sp.Row - base
		if row < 0 || row >= r.rows {
			continue
		}
		line := r.snap[row*r.cols : (row+1)*r.cols]
		for c := max(sp.From, 0); c <= min(sp.To, r.cols-1); c++ {
			line[c].Attrs |= screen.AttrUnderline
			if line[c].FG.IsDefault() {
				line[c].FG = linkFG
			}
		}
	}
}
