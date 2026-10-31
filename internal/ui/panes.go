package ui

import (
	"image"
	"slices"
	"sort"

	"github.com/patrick-gogo/kaze-terminal/internal/render"
	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
)

// Floating pane sizes in dp at 100% scale (spec 8.1, mockup .fp).
const (
	maxPanes     = 4 // per session, the scratch shell included even while hidden
	paneHeaderDp = 30
	paneEdgeDp   = 6
	paneCornerDp = 14
	paneRadiusDp = 8
)

var (
	paneMinDp     = image.Pt(280, 140)
	memberSizeDp  = image.Pt(560, 260)
	scratchRectDp = image.Rect(150, 80, 670, 320)
)

type paneKind int

const (
	paneMember paneKind = iota
	paneScratch
)

// pane is one floating terminal over a session. r is kept in dp relative to
// the terminal area's top-left, so the sidebar or a monitor change keeps it put.
type pane struct {
	id     tabs.TabID
	kind   paneKind
	r      image.Rectangle
	fit    image.Rectangle // r as fitted last frame, dp; a drag starts from it
	z      uint64          // draw order: higher is in front
	opened uint64          // open order: the oldest member is replaced first
	hidden bool            // the scratch shell only: toggled off, still running
	body   int             // event tag: keys, paste and the terminal's pointer input
	frame  int             // event tag: header and edges
	hits   paneHits
	grab   paneGrab
}

// paneHits is where the last frame drew a pane, in window px.
type paneHits struct {
	outer, header, body image.Rectangle
	expand, close       image.Rectangle // empty when the pane has no such button
}

// paneGrab is a header drag or an edge resize in progress.
type paneGrab struct {
	on    bool
	e     edge            // 0: moving
	from  image.Point     // press position, window px
	start image.Rectangle // fitted rect at the press, dp
}

// paneSet is one session's panes in open order.
type paneSet struct{ panes []*pane }

// floats is the window's floating panes; UI goroutine only.
type floats struct {
	sets    map[tabs.TabID]*paneSet // by owning session
	seq     uint64                  // z and open-order stamps
	keys    tabs.TabID              // the pane with the keyboard; 0 = the session
	rends   map[tabs.TabID]*render.Renderer
	shown   []tabs.TabID // tabs marked visible last frame
	views   []paneView   // drawn last frame, front last; input uses them
	redraws int          // frames pane gestures asked for; tests count them
	refocus bool         // the pane with the keyboard was just drawn: focus it once its filters are up
}

func (f *floats) set(session tabs.TabID) *paneSet {
	if f.sets == nil {
		f.sets = map[tabs.TabID]*paneSet{}
	}
	s := f.sets[session]
	if s == nil {
		s = &paneSet{}
		f.sets[session] = s
	}
	return s
}

func (f *floats) stamp() uint64 {
	f.seq++
	return f.seq
}

// open shows tab id as a pane over session: an existing pane is unhidden and
// raised; a member floating over another session moves here. When the set
// grows past maxPanes the oldest member pane leaves it and comes back as dropped.
func (f *floats) open(session, id tabs.TabID, kind paneKind) (p *pane, dropped tabs.TabID) {
	if o := f.owner(id); o != 0 && o != session {
		f.remove(id)
	}
	s := f.set(session)
	if p := s.find(id); p != nil {
		p.hidden, p.z = false, f.stamp()
		return p, 0
	}
	p = &pane{id: id, kind: kind}
	if kind == paneScratch {
		p.r = scratchRectDp
	} else {
		n := len(s.visible())
		at := image.Pt(80+30*n, 40+30*n)
		p.r = image.Rectangle{Min: at, Max: at.Add(memberSizeDp)}
	}
	p.opened = f.stamp()
	p.z = f.stamp()
	s.panes = append(s.panes, p)
	if len(s.panes) > maxPanes {
		var old *pane
		for _, q := range s.panes {
			if q.kind == paneMember && (old == nil || q.opened < old.opened) {
				old = q
			}
		}
		if old != nil {
			f.remove(old.id)
			dropped = old.id
		}
	}
	return p, dropped
}

// raise brings the pane to the front of its set.
func (f *floats) raise(session, id tabs.TabID) {
	if p := f.set(session).find(id); p != nil {
		p.z = f.stamp()
	}
}

// remove takes id out of whichever set holds it.
func (f *floats) remove(id tabs.TabID) (session tabs.TabID, ok bool) {
	for sid, s := range f.sets {
		if i := slices.IndexFunc(s.panes, func(p *pane) bool { return p.id == id }); i >= 0 {
			s.panes = slices.Delete(s.panes, i, i+1)
			if f.keys == id {
				f.keys = 0
			}
			return sid, true
		}
	}
	return 0, false
}

// owner is the session whose set holds id, 0 if none.
func (f *floats) owner(id tabs.TabID) tabs.TabID {
	for sid, s := range f.sets {
		if s.find(id) != nil {
			return sid
		}
	}
	return 0
}

// prune drops panes whose tab is gone and sets whose session is gone. It
// returns the scratch shells of dropped sets, which the caller closes, and
// their member tabs, which the caller docks.
func (f *floats) prune(exists func(tabs.TabID) bool) (orphans, released []tabs.TabID) {
	ids := make([]tabs.TabID, 0, len(f.sets))
	for sid := range f.sets {
		ids = append(ids, sid)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, sid := range ids {
		s := f.sets[sid]
		gone := !exists(sid)
		kept := s.panes[:0]
		for _, p := range s.panes {
			switch {
			case !exists(p.id):
			case gone && p.kind == paneScratch:
				orphans = append(orphans, p.id)
			case gone:
				released = append(released, p.id)
			default:
				kept = append(kept, p)
				continue
			}
			if f.keys == p.id {
				f.keys = 0
			}
		}
		s.panes = kept
		if gone {
			delete(f.sets, sid)
		}
	}
	return orphans, released
}

func (s *paneSet) find(id tabs.TabID) *pane {
	for _, p := range s.panes {
		if p.id == id {
			return p
		}
	}
	return nil
}

func (s *paneSet) scratch() *pane {
	for _, p := range s.panes {
		if p.kind == paneScratch {
			return p
		}
	}
	return nil
}

// visible is the set's shown panes back to front.
func (s *paneSet) visible() []*pane {
	var out []*pane
	for _, p := range s.panes {
		if !p.hidden {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].z < out[j].z })
	return out
}

// front is the highest visible pane, nil if none.
func (s *paneSet) front() *pane {
	v := s.visible()
	if len(v) == 0 {
		return nil
	}
	return v[len(v)-1]
}

// edge names the sides a resize moves, as a bit set; 0 means a move.
type edge uint8

const (
	edgeN edge = 1 << iota
	edgeS
	edgeW
	edgeE
)

// fitPane is r as drawn in area: shrunk to at most area's size, then moved
// inside it. The stored r is never changed, so a pane comes back to its size
// when the window grows again.
func fitPane(r, area image.Rectangle, least image.Point) image.Rectangle {
	w := max(r.Dx(), least.X)
	h := max(r.Dy(), least.Y)
	w, h = max(min(w, area.Dx()), 0), max(min(h, area.Dy()), 0)
	x := min(max(r.Min.X, area.Min.X), area.Max.X-w)
	y := min(max(r.Min.Y, area.Min.Y), area.Max.Y-h)
	return image.Rect(x, y, x+w, y+h)
}

// edgeAt is the resize zone of r under p: a corner within corner px of one,
// else a side within band px; e == 0 inside. ok is false outside r.
func edgeAt(r image.Rectangle, p image.Point, band, corner int) (e edge, ok bool) {
	if !p.In(r) {
		return 0, false
	}
	near := func(d, n int) bool { return d >= 0 && d < n }
	n, s := near(p.Y-r.Min.Y, corner), near(r.Max.Y-1-p.Y, corner)
	w, east := near(p.X-r.Min.X, corner), near(r.Max.X-1-p.X, corner)
	switch {
	case n && w:
		return edgeN | edgeW, true
	case n && east:
		return edgeN | edgeE, true
	case s && w:
		return edgeS | edgeW, true
	case s && east:
		return edgeS | edgeE, true
	}
	if near(p.Y-r.Min.Y, band) {
		e |= edgeN
	} else if near(r.Max.Y-1-p.Y, band) {
		e |= edgeS
	}
	if near(p.X-r.Min.X, band) {
		e |= edgeW
	} else if near(r.Max.X-1-p.X, band) {
		e |= edgeE
	}
	return e, true
}

// dragRect is start moved by delta (e == 0) or with the sides in e moved,
// never smaller than least: a side stops there and the opposite one stays put.
func dragRect(start image.Rectangle, e edge, delta, least image.Point) image.Rectangle {
	if e == 0 {
		return start.Add(delta)
	}
	r := start
	if e&edgeW != 0 {
		r.Min.X = min(start.Min.X+delta.X, start.Max.X-least.X)
	}
	if e&edgeE != 0 {
		r.Max.X = max(start.Max.X+delta.X, start.Min.X+least.X)
	}
	if e&edgeN != 0 {
		r.Min.Y = min(start.Min.Y+delta.Y, start.Max.Y-least.Y)
	}
	if e&edgeS != 0 {
		r.Max.Y = max(start.Max.Y+delta.Y, start.Min.Y+least.Y)
	}
	return r
}
