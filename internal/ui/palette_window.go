package ui

import (
	"bytes"
	"image"
	"io"
	"path/filepath"
	"runtime"

	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"

	"github.com/patrick-gogo/kaze-terminal/internal/gitinfo"
	kinput "github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/skills"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// paletteUI is the palette's state; UI goroutine only, except loaded, which
// the skill scan sends on.
type paletteUI struct {
	open     bool
	ed       widget.Editor // the query
	sel      string        // the selected item's Key; "" or a gone key is the first item
	scroll   int
	reveal   bool // scroll the selection into view next frame
	hover    int  // the item index under the pointer, -1 for none
	hits     palHits
	items    []palItem // as last built, the ones hits refer to
	tag      int       // pointer events anywhere in the window while open
	filters  []event.Filter
	edInput  input.Router // feeds the native editor one ordered event at a time
	edOps    op.Ops       // the editor's keyboard handler for the private router
	edReady  bool
	skills   []skills.Skill // the last list loaded, kept across opens
	scanning bool
	loaded   chan []skills.Skill // buffered 1; the scan sends, the frame drains
	recent   recentPrompts
	spaces   []workspaces.Workspace // read on open
	redraws  int                    // frames the palette's pointer input asked for; tests count them
}

// selIndex is the selected item's index: its Key's, else the first; -1 for none.
func (p *paletteUI) selIndex() int {
	if len(p.items) == 0 {
		return -1
	}
	for i, it := range p.items {
		if it.Key == p.sel {
			return i
		}
	}
	return 0
}

func (win *window) togglePalette() {
	if win.palette.open {
		win.closePalette()
	} else {
		win.openPalette()
	}
}

// openPalette shows the palette with an empty query and the field focused. It
// shows the skills loaded last time at once and starts one scan for changes.
func (win *window) openPalette() {
	p := &win.palette
	if p.open {
		return
	}
	win.dropSideEdit()
	p.open, p.sel, p.scroll, p.hover, p.reveal = true, "", 0, -1, false
	p.ed.SetText("")
	p.edInput, p.edReady = input.Router{}, false
	p.spaces = nil
	if win.cfgPath != "" {
		if doc, _, err := workspaces.Load(workspaces.Path(win.cfgPath)); err == nil {
			p.spaces = doc.Workspaces
		}
	}
	win.startSkillScan()
	win.wantFocus = true
	win.w.Invalidate()
}

func (win *window) closePalette() {
	p := &win.palette
	if !p.open {
		return
	}
	p.open, p.hits, p.items, p.hover = false, palHits{}, nil, -1
	p.ed.SetText("")
	win.wantFocus = true
	win.w.Invalidate()
}

// startSkillScan reads the skill files on a goroutine, at most one at a time,
// so the UI never waits on the disk.
func (win *window) startSkillScan() {
	p := &win.palette
	if p.scanning {
		return
	}
	if p.loaded == nil {
		p.loaded = make(chan []skills.Skill, 1)
	}
	src, cwd := win.skillSources()
	fn := win.skillsFn
	if fn == nil {
		if win.skillCache == nil {
			win.skillCache = skills.NewCache()
		}
		fn = win.skillCache.Load
	}
	p.scanning = true
	loaded, w := p.loaded, win.w
	go func() {
		if cwd != "" {
			src.Project = gitinfo.Top(cwd)
		}
		loaded <- fn(src)
		if w != nil {
			w.Invalidate()
		}
	}()
}

// skillSources is where skills are read from: ~/.claude, and the current
// session's checkout (its folder is returned; the scan finds the top).
func (win *window) skillSources() (skills.Sources, string) {
	var src skills.Sources
	if win.claudePath != "" {
		src.ClaudeDir = filepath.Dir(win.claudePath)
	}
	cwd := ""
	if t := win.shownTab(); t != nil {
		cwd = win.locOf(t).cwd
	}
	return src, cwd
}

// palInput is the palette's live input: states change while it is open.
func (win *window) palInput() palInput {
	in := palInput{
		Query:    win.palette.ed.Text(),
		Sessions: sessionRows(buildSide(win.sideIn())),
		Skills:   win.palette.skills,
		Recent:   win.palette.recent.list,
	}
	for _, r := range win.curRuns() {
		in.Members = append(in.Members, r.Members...)
	}
	for _, c := range kazeCommands {
		if c.Act == actPalette {
			continue // it is open already
		}
		a := palAction{Name: c.Label, Config: c.Config, Act: c.Act}
		if sc, ok := win.binds[c.Config]; ok {
			a.Key = formatShortcut(runtime.GOOS, sc)
		}
		in.Actions = append(in.Actions, a)
	}
	in.Actions = append(in.Actions, workspaceActions(win.palette.spaces)...)
	return in
}

func (win *window) refreshPalette() {
	win.palette.items = buildPalette(win.palInput())
}

// paletteEvents takes the skill scan's answer, and while open, the field's
// keys and every pointer event in the window.
func (win *window) paletteEvents(gtx layout.Context) {
	p := &win.palette
	select {
	case list := <-p.loaded:
		p.skills, p.scanning = list, false
	default:
	}
	if !p.open {
		return
	}
	win.initPaletteEditor(gtx)
	// One stream preserves ordering between text, native editing keys and
	// palette actions. The private router lets the native Editor handle each
	// edit without reaching ahead past Enter or a palette navigation key.
	for p.open {
		ev, ok := gtx.Event(p.filters...)
		if !ok {
			break
		}
		if win.bench.IgnoreInput() {
			continue
		}
		if e, ok := ev.(key.Event); ok && win.isPaletteKey(e) {
			if e.State == key.Press {
				win.refreshPalette()
				win.paletteKey(gtx, e)
			}
			continue
		}
		p.edInput.Queue(ev)
		win.updatePaletteEditor(gtx)
	}
	kinds := pointer.Press | pointer.Move | pointer.Scroll
	for p.open {
		ev, ok := gtx.Event(pointer.Filter{Target: &p.tag, Kinds: kinds, ScrollY: pointer.ScrollRange{Min: -1 << 20, Max: 1 << 20}})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok && !win.bench.IgnoreInput() {
			win.palettePointer(gtx, e)
		}
	}
}

// initPaletteEditor supplies the native widget with its own keyboard source.
// Its public EditorState and clipboard requests are mirrored to the real window.
func (win *window) initPaletteEditor(gtx layout.Context) {
	p := &win.palette
	if p.filters == nil {
		p.filters = []event.Filter{
			key.FocusFilter{Target: &p.ed},
			key.Filter{Focus: &p.ed, Optional: allMods},
			key.Filter{Focus: &p.ed, Name: key.NameTab, Optional: allMods},
			transfer.TargetFilter{Target: &p.ed, Type: "application/text"},
		}
	}
	p.ed.SingleLine, p.ed.Submit = true, false
	if p.edReady {
		return
	}
	p.edReady = true
	p.edOps.Reset()
	event.Op(&p.edOps, &p.ed)
	win.updatePaletteEditor(gtx)
	p.edInput.Source().Execute(key.FocusCmd{Tag: &p.ed})
	win.updatePaletteEditor(gtx)
}

func (win *window) updatePaletteEditor(gtx layout.Context) {
	p := &win.palette
	eg := gtx
	eg.Source = p.edInput.Source()
	for {
		ev, ok := p.ed.Update(eg)
		if !ok {
			break
		}
		if _, ok := ev.(widget.ChangeEvent); ok {
			win.ui.fit(p.ed.Text())
			p.sel, p.scroll = "", 0
			win.w.Invalidate()
		}
	}
	p.edInput.Frame(&p.edOps)
	win.syncPaletteEditor(gtx)
}

func (win *window) syncPaletteEditor(gtx layout.Context) {
	p := &win.palette
	s := p.edInput.EditorState()
	gtx.Execute(key.SelectionCmd{Tag: &p.ed, Range: s.Selection.Range, Caret: s.Selection.Caret, CompositionBounds: s.Selection.CompositionBounds})
	gtx.Execute(key.SnippetCmd{Tag: &p.ed, Snippet: s.Snippet})
	if mime, data, ok := p.edInput.WriteClipboard(); ok {
		gtx.Execute(clipboard.WriteCmd{Type: mime, Data: io.NopCloser(bytes.NewReader(data))})
	}
	if p.edInput.ClipboardRequested() {
		gtx.Execute(clipboard.ReadCmd{Tag: &p.ed})
	}
	switch p.edInput.TextInputState() {
	case input.TextInputOpen:
		gtx.Execute(key.SoftKeyboardCmd{Show: true})
	case input.TextInputClose:
		gtx.Execute(key.SoftKeyboardCmd{Show: false})
	}
	if at, ok := p.edInput.WakeupTime(); ok {
		gtx.Execute(op.InvalidateCmd{At: at})
	}
}

func (win *window) isPaletteKey(e key.Event) bool {
	switch e.Name {
	case key.NameUpArrow, key.NameDownArrow, key.NameReturn, key.NameEnter, key.NameEscape:
		return true
	}
	_, ok := win.km[kinput.Shortcut{Name: e.Name, Mods: e.Modifiers}]
	return ok
}

func (win *window) paletteKey(gtx layout.Context, e key.Event) {
	p := &win.palette
	switch e.Name {
	case key.NameUpArrow, key.NameDownArrow:
		i := p.selIndex()
		if i < 0 {
			return
		}
		if e.Name == key.NameUpArrow {
			i = max(i-1, 0)
		} else {
			i = min(i+1, len(p.items)-1)
		}
		p.sel, p.reveal = p.items[i].Key, true
		win.w.Invalidate()
		return
	case key.NameReturn, key.NameEnter:
		win.refreshPalette() // the query may have changed since the last draw
		if i := p.selIndex(); i >= 0 {
			win.palAct(gtx, p.items[i], e.Modifiers.Contain(key.ModShift))
		}
		return
	case key.NameEscape:
		win.closePalette()
		return
	}
	a, ok := win.km[kinput.Shortcut{Name: e.Name, Mods: e.Modifiers}]
	if !ok {
		return
	}
	switch a {
	case actPaste:
		p.edInput.Source().Execute(clipboard.ReadCmd{Tag: &p.ed})
		win.syncPaletteEditor(gtx)
	case actCopy:
		win.writeClipboard(gtx, p.ed.SelectedText())
	case actPalette:
		win.closePalette()
	default:
		win.closePalette()
		win.runShortcut(gtx, a)
	}
}

// palettePointer: a click on a row acts on it (Shift: a new session), a click
// outside the box closes the palette and does nothing else, hovering moves
// the selection and the wheel scrolls the list.
func (win *window) palettePointer(gtx layout.Context, e pointer.Event) {
	p := &win.palette
	at := e.Position.Round()
	switch e.Kind {
	case pointer.Press:
		if i, ok := p.hits.rowAt(at); ok && i < len(p.items) {
			win.palAct(gtx, p.items[i], e.Modifiers.Contain(key.ModShift))
			return
		}
		if !at.In(p.hits.box) {
			win.closePalette()
			return
		}
		win.wantFocus = true // a click on the field or a header keeps the keyboard there
	case pointer.Move:
		i, ok := p.hits.rowAt(at)
		if !ok {
			p.hover = -1
			return
		}
		if i == p.hover || i >= len(p.items) {
			return
		}
		p.hover, p.sel = i, p.items[i].Key
		win.paletteRedraw()
	case pointer.Scroll:
		if !at.In(p.hits.list) {
			return
		}
		p.scroll = min(max(p.hits.scroll+int(e.Scroll.Y), 0), p.hits.scrollMax)
		p.hover = -1 // the rows move under the pointer
		win.paletteRedraw()
	}
}

func (win *window) paletteRedraw() {
	win.palette.redraws++
	win.w.Invalidate()
}

// palAct closes the palette and does what the item is for.
func (win *window) palAct(gtx layout.Context, it palItem, shift bool) {
	win.closePalette()
	switch it.Section {
	case palWaiting, palSessions:
		if win.mgr.Lookup(it.Tab) == nil {
			return
		}
		if it.Member {
			win.floatMember(it.Tab)
		} else {
			win.selectSession(it.Tab)
		}
	case palSkills:
		win.startPrompt(it.Prompt, shift)
	case palKaze:
		if it.Workspace != "" {
			win.runWorkspace(it.Workspace)
		} else {
			win.runShortcut(gtx, it.Act)
		}
	}
}

// drawPalette draws the open palette over main and takes every pointer event
// in the window, so nothing under it is clicked.
func (win *window) drawPalette(gtx layout.Context, main image.Rectangle) {
	p := &win.palette
	if !p.open {
		return
	}
	win.refreshPalette()
	sel := p.selIndex()
	if sel >= 0 {
		p.sel = p.items[sel].Key
	}
	// Register and finish handling the window's IME target before its first
	// focus command. Gio then defers that focus change and replays any input
	// after the opening shortcut next frame. Subsequent input is consumed
	// in paletteEvents, not during Layout.
	if !p.edReady {
		win.initPaletteEditor(gtx)
		for {
			if _, ok := gtx.Event(p.filters...); !ok {
				break
			}
		}
	}
	eg := gtx
	eg.Source = p.edInput.Source()
	p.hits = win.ui.layoutPalette(eg, palState{items: p.items, sel: sel, scroll: p.scroll, hover: p.hover, reveal: p.reveal, ed: &p.ed}, main)
	win.syncPaletteEditor(gtx)
	p.scroll, p.reveal = p.hits.scroll, false
	area := clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops)
	event.Op(gtx.Ops, &p.tag)
	area.Pop()
}
