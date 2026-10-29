package ui

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// wsColors are the recipe colors in swatch order.
var wsColors = []string{"cyan", "magenta", "yellow", "green", "blue", "red"}

// statFolder looks a draft folder up on disk; tests count the lookups.
var statFolder = os.Stat

var wsWaits = []workspaces.Wait{workspaces.WaitNone, workspaces.WaitDocker, workspaces.WaitPort}

// wsPage is the Workspaces section: the list as read when it showed, and the
// draft being edited. UI goroutine only.
type wsPage struct {
	doc     workspaces.Document
	loadErr string // why the file could not be read; the list is hidden
	draft   *wsDraft
	dialog  wsDialog
	del     string // the workspace the delete dialog is about
	then    func() // what leaving was going to do, once the unsaved-changes question is answered
}

type wsDialog int

const (
	wsNoDialog wsDialog = iota
	wsDeleteDialog
	wsStaleDialog
	wsLeaveDialog
)

// wsDraft is a copy of one workspace being edited. Each tab and command has a
// key from wsKeys, so field ids follow the item when others move or go.
type wsDraft struct {
	id, name string
	nameID   string // the name field's id, unique to this draft
	isNew    bool
	tabs     []*wsDraftTab
	base     workspaces.Workspace // the draft as opened, to tell unsaved changes
	doc      workspaces.Document  // the file the draft was opened from, which Save replaces
	rev      workspaces.Revision
}

type wsDraftTab struct {
	key           int
	id            string
	title, folder string
	cmds          []*wsDraftCmd
	color         string
	wait          workspaces.Wait
	port          string // as typed; checked on Save
	// The folder last looked up on disk and whether it was missing; the page
	// is drawn every frame, so the disk is asked only when the folder changes.
	checkedDir       string
	checked, missing bool
}

type wsDraftCmd struct {
	key  int
	text string
}

func workspacesSection() settingsSection {
	return settingsSection{
		title: "Workspaces", hint: "A workspace opens a set of tabs in one click.",
		rows: wsRows, act: wsAct, show: (*window).loadWorkspaces, dialog: wsDialogBox, leave: wsLeave,
	}
}

// loadWorkspaces reads the list again; an open draft keeps the file it was opened from.
func (win *window) loadWorkspaces() {
	p := &win.wsp
	p.doc, p.loadErr = workspaces.Document{}, ""
	if win.cfgPath == "" {
		p.loadErr = "Workspaces are unavailable: kaze has no settings folder."
		return
	}
	path := workspaces.Path(win.cfgPath)
	doc, _, err := workspaces.Load(path)
	if err != nil {
		p.loadErr = strings.TrimPrefix(err.Error(), path+": ") // the hint names the file
		return
	}
	p.doc = doc
}

func wsRows(win *window) []settingRow {
	if d := win.wsp.draft; d != nil {
		return win.editorRows(d)
	}
	return win.listRows()
}

// listRows is one card per workspace, then New and the startup choice.
func (win *window) listRows() []settingRow {
	p := &win.wsp
	if p.loadErr != "" {
		return []settingRow{{label: "The workspaces file has a problem", lines: []string{truncateRight(cleanText(p.loadErr), 120)},
			hint: "Fix or delete " + tildeHome(workspaces.Path(win.cfgPath), win.home) + "; nothing runs from it until then.", ctrls: []ctrl{{kind: ctlButton, id: "ws.reload", text: "Reload"}}}}
	}
	newBtn := ctrl{kind: ctlButton, id: "ws.new", text: "+ New workspace"}
	if len(p.doc.Workspaces) == 0 {
		return []settingRow{{label: "No workspaces yet", hint: "Save a set of tabs, folders and commands to open together.", ctrls: []ctrl{newBtn}}}
	}
	var rows []settingRow
	for i, w := range p.doc.Workspaces {
		state := win.wsBadge(w.ID)
		run := "Run"
		if state != "Closed" {
			run = "Go to tabs"
		}
		row := settingRow{label: w.Name, lines: []string{state + " · " + plural(len(w.Tabs), "tab")}, ctrls: []ctrl{
			{kind: ctlButton, id: "ws.run", text: run, arg: i, strong: true},
			{kind: ctlButton, id: "ws.edit", text: "Edit", arg: i},
			{kind: ctlButton, id: "ws.delete", text: "Delete", arg: i},
		}}
		for _, t := range w.Tabs {
			row.lines = append(row.lines, truncateMiddle(win.memberLine(t), 100))
		}
		rows = append(rows, row)
	}
	rows = append(rows, settingRow{ctrls: []ctrl{newBtn}})
	return append(rows, win.startupRow())
}

// memberLine is one recipe tab in the list: title, folder, commands and wait.
func (win *window) memberLine(t workspaces.Tab) string {
	parts := []string{t.Title, tildeHome(t.Folder, win.home)}
	if len(t.Commands) > 0 {
		parts = append(parts, strings.Join(t.Commands, " → "))
	}
	switch t.Wait {
	case workspaces.WaitDocker:
		parts = append(parts, "after Docker is up")
	case workspaces.WaitPort:
		parts = append(parts, fmt.Sprintf("after port %d answers", t.Port))
	}
	return cleanText(strings.Join(parts, "   "))
}

func plural(n int, what string) string {
	if n == 1 {
		return "1 " + what
	}
	return strconv.Itoa(n) + " " + what + "s"
}

// wsBadge is the list's state for workspace id, in words.
func (win *window) wsBadge(id string) string {
	r := win.openRun(id)
	if r == nil {
		return "Closed"
	}
	open, starting, needs := 0, false, false
	for _, m := range r.members {
		if m.tab == 0 || !win.mgr.Has(m.tab) {
			continue
		}
		open++
		switch m.run.State {
		case workspaces.Failed, workspaces.NeedsConfirmation:
			needs = true
		case workspaces.Launched, workspaces.Cancelled:
		default:
			starting = true
		}
	}
	switch {
	case needs:
		return "Needs attention"
	case starting:
		return "Starting"
	case open < len(r.members):
		return fmt.Sprintf("%d of %d tabs open", open, len(r.members))
	}
	return "Open"
}

// startupRow picks the workspace kaze runs when it starts; saving it runs nothing.
func (win *window) startupRow() settingRow {
	opts := []string{"None"}
	sel, id := 0, win.opt.Config.StartupWorkspace
	for i, w := range win.wsp.doc.Workspaces {
		opts = append(opts, w.Name)
		if w.ID == id {
			sel = i + 1
		}
	}
	row := settingRow{label: "Run a workspace when kaze starts", hint: "Instead of the usual first tab", ctrls: []ctrl{{kind: ctlSegmented, id: "ws.startup", options: opts, selected: sel}}}
	if id != "" && sel == 0 {
		row.ctrls[0].selected = -1
		row.hint = "The chosen workspace no longer exists; kaze opens the usual tab"
	}
	return row
}

// wsErrors are the draft's problems by field, from the same checks Save runs.
type wsErrors struct {
	name, tabs string
	tab        map[int]map[string]string // by tab key, then field
	missing    map[int]bool              // folders not found; Run stops there, Save does not
	any        bool
}

func (e *wsErrors) of(key int, field string) string { return e.tab[key][field] }

func (win *window) draftErrors(d *wsDraft) wsErrors {
	e := wsErrors{tab: map[int]map[string]string{}, missing: map[int]bool{}}
	ws := d.workspace()
	label := strings.TrimSpace(ws.Name)
	if label == "" {
		label = ws.ID
	}
	// The draft goes last, so a name clash is reported on it, not on the saved one.
	all := workspaces.Document{Workspaces: append(slices.DeleteFunc(slices.Clone(d.doc.Workspaces), func(w workspaces.Workspace) bool { return w.ID == d.id }), ws)}
	for _, fe := range all.Validate(win.home) {
		if fe.Workspace != label || fe.Tab != "" {
			continue
		}
		e.any = true
		if fe.Field == "name" {
			e.name = fe.Msg
		} else if e.tabs == "" {
			e.tabs = fe.Msg
		}
	}
	kind := workspaces.KindOf(win.opt.Shell)
	for i, t := range d.tabs {
		set := func(field, msg string) {
			e.any = true
			if e.tab[t.key] == nil {
				e.tab[t.key] = map[string]string{}
			}
			if e.tab[t.key][field] == "" {
				e.tab[t.key][field] = msg
			}
		}
		one := workspaces.Document{Workspaces: []workspaces.Workspace{{ID: ws.ID, Name: "x", Tabs: []workspaces.Tab{ws.Tabs[i]}}}}
		for _, fe := range one.Validate(win.home) {
			if fe.Tab != "" {
				set(fe.Field, fe.Msg)
			}
		}
		if err := workspaces.CheckCommands(kind, ws.Tabs[i].Commands); err != nil {
			set("commands", err.Error())
		}
		if e.of(t.key, "folder") == "" {
			if dir, err := workspaces.ExpandFolder(t.folder, win.home); err == nil {
				if !t.checked || t.checkedDir != dir {
					st, err := statFolder(dir)
					t.checked, t.checkedDir, t.missing = true, dir, err != nil || !st.IsDir()
				}
				if t.missing {
					e.missing[t.key] = true
				}
			}
		}
	}
	return e
}

// workspace is the draft as Save would store it, before Save trims it.
func (d *wsDraft) workspace() workspaces.Workspace {
	ws := workspaces.Workspace{ID: d.id, Name: d.name}
	for _, t := range d.tabs {
		tab := workspaces.Tab{ID: t.id, Title: t.title, Folder: t.folder, Color: t.color, Wait: t.wait, Commands: []string{}}
		for _, c := range t.cmds {
			tab.Commands = append(tab.Commands, c.text)
		}
		if t.wait == workspaces.WaitPort {
			tab.Port = -1 // reported as out of range unless it parses
			if n, err := strconv.Atoi(strings.TrimSpace(t.port)); err == nil {
				tab.Port = n
			}
		}
		ws.Tabs = append(ws.Tabs, tab)
	}
	return ws
}

func (d *wsDraft) dirty() bool { return !reflect.DeepEqual(d.workspace(), d.base) }

// wsKeys numbers draft items across every draft, so no field id names an
// item of an earlier draft and its editor's undo history.
var wsKeys atomic.Int64

func (d *wsDraft) key() int { return int(wsKeys.Add(1)) }

func (d *wsDraft) addTab(t workspaces.Tab) *wsDraftTab {
	dt := &wsDraftTab{key: d.key(), id: t.ID, title: t.Title, folder: t.Folder, color: t.Color, wait: t.Wait}
	if dt.wait == "" {
		dt.wait = workspaces.WaitNone
	}
	if t.Port != 0 {
		dt.port = strconv.Itoa(t.Port)
	}
	for _, c := range t.Commands {
		dt.cmds = append(dt.cmds, &wsDraftCmd{key: d.key(), text: c})
	}
	if len(dt.cmds) == 0 {
		dt.cmds = []*wsDraftCmd{{key: d.key()}} // one empty field to type into
	}
	d.tabs = append(d.tabs, dt)
	return dt
}

func (d *wsDraft) tabByKey(k int) (int, *wsDraftTab) {
	for i, t := range d.tabs {
		if t.key == k {
			return i, t
		}
	}
	return -1, nil
}

func (d *wsDraft) cmdByKey(k int) (*wsDraftTab, int) {
	for _, t := range d.tabs {
		for j, c := range t.cmds {
			if c.key == k {
				return t, j
			}
		}
	}
	return nil, -1
}

// newDraft opens ws (or a new workspace when ws is nil) from doc, read at rev.
func newDraft(ws *workspaces.Workspace, doc workspaces.Document, rev workspaces.Revision) *wsDraft {
	d := &wsDraft{doc: doc, rev: rev}
	d.nameID = fmt.Sprintf("ws.name.%d", d.key())
	if ws == nil {
		d.id, d.isNew = workspaces.NewID(), true
		d.addTab(workspaces.Tab{ID: workspaces.NewTabID(), Color: wsColors[0]})
	} else {
		d.id, d.name = ws.ID, ws.Name
		for _, t := range ws.Tabs {
			d.addTab(t)
		}
	}
	d.base = d.workspace()
	return d
}

func (win *window) editorRows(d *wsDraft) []settingRow {
	errs := win.draftErrors(d)
	title := "Edit workspace"
	if d.isNew {
		title = "New workspace"
	}
	rows := []settingRow{
		{label: title, hint: "Changes are a draft until you press Save. Saving never runs anything."},
		{label: "Name", ctrls: []ctrl{{kind: ctlText, id: d.nameID, val: d.name, placeholder: "my-project", err: errs.name, width: 260,
			set: func(s string) { d.name = s }}}},
	}
	kind := workspaces.KindOf(win.opt.Shell)
	for i, t := range d.tabs {
		k := t.key
		tid := func(what string) string { return fmt.Sprintf("ws.tab.%d.%s", k, what) }
		// Edits look the tab up by key: it may have moved or gone since this frame.
		tab := func(f func(*wsDraftTab)) {
			if _, t := d.tabByKey(k); t != nil {
				f(t)
			}
		}
		head := settingRow{label: fmt.Sprintf("Tab %d", i+1), ctrls: []ctrl{
			{kind: ctlButton, id: tid("up"), text: "↑", dead: i == 0},
			{kind: ctlButton, id: tid("down"), text: "↓", dead: i == len(d.tabs)-1},
			{kind: ctlButton, id: tid("rm"), text: "Remove", dead: len(d.tabs) == 1},
		}}
		rows = append(rows, head,
			settingRow{label: "Title", ctrls: []ctrl{{kind: ctlText, id: tid("title"), val: t.title, placeholder: "api", err: errs.of(k, "title"), width: 260,
				set: func(s string) { tab(func(t *wsDraftTab) { t.title = s }) }}}},
		)
		folderErr := errs.of(k, "folder")
		if folderErr == "" && errs.missing[k] {
			folderErr = "This folder does not exist yet; Run stops here until it does."
		}
		rows = append(rows, settingRow{label: "Folder", ctrls: []ctrl{{kind: ctlText, id: tid("folder"), val: t.folder, placeholder: folderHint(), err: folderErr, width: 360,
			set: func(s string) { tab(func(t *wsDraftTab) { t.folder = s }) }}}})
		for j, c := range t.cmds {
			ck := c.key
			label := ""
			if j == 0 {
				label = "Commands"
			}
			var err string
			if j == 0 {
				err = errs.of(k, "commands")
			}
			rows = append(rows, settingRow{label: label, ctrls: []ctrl{
				{kind: ctlText, id: fmt.Sprintf("ws.cmd.%d", ck), val: c.text, placeholder: "npm run dev", err: err, width: 300,
					set: func(s string) {
						if t, j := d.cmdByKey(ck); t != nil {
							t.cmds[j].text = s
						}
					}},
				{kind: ctlButton, id: fmt.Sprintf("ws.cmd.%d.rm", ck), text: "Remove"},
			}})
		}
		rows = append(rows, previewRow(kind, win.opt.Shell, t, len(t.cmds) < workspaces.MaxCommands, tid("addcmd")))
		colors := make([]ctrl, len(wsColors))
		for ci, name := range wsColors {
			colors[ci] = ctrl{kind: ctlColor, id: tid("color"), text: name, arg: ci, on: t.color == name}
		}
		rows = append(rows,
			settingRow{label: "Color", ctrls: colors},
			settingRow{label: "Wait for", hint: "Before its commands are typed", ctrls: []ctrl{{kind: ctlSegmented, id: tid("wait"),
				options: []string{"None", "Docker", "Port"}, selected: slices.Index(wsWaits, t.wait)}}},
		)
		if t.wait == workspaces.WaitPort {
			rows = append(rows, settingRow{label: "Port", hint: "On this computer (localhost)", ctrls: []ctrl{{kind: ctlText, id: tid("port"), val: t.port, placeholder: "8000",
				err: errs.of(k, "port"), width: 100, set: func(s string) { tab(func(t *wsDraftTab) { t.port = s }) }}}})
		}
	}
	add := settingRow{ctrls: []ctrl{{kind: ctlButton, id: "ws.addtab", text: "+ Add tab", dead: len(d.tabs) >= workspaces.MaxTabs}}}
	if errs.tabs != "" {
		add.lines = []string{errs.tabs}
	}
	return append(rows, add, settingRow{ctrls: []ctrl{
		{kind: ctlButton, id: "ws.cancel", text: "Cancel"},
		{kind: ctlButton, id: "ws.save", text: "Save", strong: true, dead: errs.any},
	}})
}

// previewRow shows the one line typed into the tab once its prompt is ready.
func previewRow(kind workspaces.ShellKind, shell []string, t *wsDraftTab, canAdd bool, addID string) settingRow {
	var cmds []string
	for _, c := range t.cmds {
		cmds = append(cmds, c.text)
	}
	row := settingRow{label: "Typed as one line once the prompt is ready:", ctrls: []ctrl{{kind: ctlButton, id: addID, text: "+ Add command", dead: !canAdd}}}
	line, err := workspaces.JoinLine(kind, cmds)
	switch {
	case err != nil:
		row.lines = []string{shellName(shell) + ": " + err.Error()}
	case line == "":
		row.label, row.lines = "No commands: the tab opens at its prompt.", nil
	default:
		row.lines = []string{truncateMiddle(cleanText(line), 110)}
	}
	return row
}

func folderHint() string {
	if runtime.GOOS == "windows" {
		return `C:\Users\you\project`
	}
	return "~/project"
}

func wsAct(win *window, id string, arg int) {
	p := &win.wsp
	if d := p.draft; d != nil {
		if win.draftAct(d, id, arg) {
			return
		}
	}
	at := func() (workspaces.Workspace, bool) {
		if arg < 0 || arg >= len(p.doc.Workspaces) {
			return workspaces.Workspace{}, false
		}
		return p.doc.Workspaces[arg], true
	}
	switch id {
	case "ws.reload":
		win.loadWorkspaces()
	case "ws.run":
		if w, ok := at(); ok {
			win.goOrRun(w.ID)
		}
	case "ws.edit":
		if w, ok := at(); ok {
			win.openDraft(&w)
		}
	case "ws.new":
		win.openDraft(nil)
	case "ws.delete":
		if w, ok := at(); ok {
			p.dialog, p.del = wsDeleteDialog, w.ID
		}
	case "ws.del.cancel":
		p.dialog = wsNoDialog
	case "ws.del.ok":
		p.dialog = wsNoDialog
		win.deleteWorkspace(p.del)
	case "ws.startup":
		v := ""
		if arg > 0 && arg <= len(p.doc.Workspaces) {
			v = p.doc.Workspaces[arg-1].ID
		}
		win.opt.Config.StartupWorkspace = v
		win.saveSetting("startup_workspace", v)
	case "ws.stale.keep":
		p.dialog = wsNoDialog
	case "ws.stale.reload":
		p.dialog = wsNoDialog
		win.reloadDraft()
	case "ws.leave.keep":
		p.dialog, p.then = wsNoDialog, nil
	case "ws.leave.discard", "ws.leave.save":
		then := p.then
		p.dialog, p.then = wsNoDialog, nil
		if id == "ws.leave.save" && !win.saveDraft() {
			return // the save failed or went stale; its own message or dialog is up
		}
		p.draft = nil
		if then != nil {
			then()
		}
	}
}

// goOrRun focuses an open run's first tab, or starts the workspace.
func (win *window) goOrRun(id string) {
	if r := win.openRun(id); r != nil {
		for _, m := range r.members {
			if m.tab != 0 && win.mgr.Has(m.tab) {
				win.selectTab(m.tab)
				return
			}
		}
	}
	win.runWorkspace(id)
}

// openDraft opens ws, or a new workspace when nil, asking first about unsaved changes.
func (win *window) openDraft(ws *workspaces.Workspace) {
	if !wsLeave(win, func() { win.openDraft(ws) }) {
		return
	}
	if win.cfgPath == "" {
		return
	}
	// Read the file now, so Save can tell if it changes while the draft is open.
	doc, rev, err := workspaces.Load(workspaces.Path(win.cfgPath))
	if err != nil {
		win.note("workspaces: " + err.Error())
		win.loadWorkspaces()
		return
	}
	if ws != nil {
		cur, ok := doc.ByID(ws.ID)
		if !ok {
			win.note("that workspace no longer exists")
			win.loadWorkspaces()
			return
		}
		ws = &cur
	}
	d := newDraft(ws, doc, rev)
	win.setDraft(d)
	win.set.scroll = 0
	if d.isNew {
		win.editField(d.nameID)
	}
}

// setDraft opens d and drops the editors of the draft before it, so nothing
// typed there can come back through undo.
func (win *window) setDraft(d *wsDraft) {
	for id := range win.set.fields {
		if strings.HasPrefix(id, "ws.") {
			delete(win.set.fields, id)
		}
	}
	if strings.HasPrefix(win.set.editing, "ws.") {
		win.editField("")
	}
	win.wsp.draft = d
}

// draftAct runs the editor's own controls; false means id is not one of them.
func (win *window) draftAct(d *wsDraft, id string, arg int) bool {
	switch id {
	case "ws.cancel":
		win.wsp.draft = nil
		win.loadWorkspaces()
		return true
	case "ws.save":
		win.saveDraft()
		return true
	case "ws.addtab":
		if len(d.tabs) < workspaces.MaxTabs {
			t := d.addTab(workspaces.Tab{ID: workspaces.NewTabID(), Color: wsColors[len(d.tabs)%len(wsColors)]})
			win.editField(fmt.Sprintf("ws.tab.%d.title", t.key))
		}
		return true
	}
	var k int
	var what string
	if n, _ := fmt.Sscanf(id, "ws.cmd.%d.%s", &k, &what); n == 2 && what == "rm" {
		if t, j := d.cmdByKey(k); t != nil {
			t.cmds = slices.Delete(t.cmds, j, j+1)
		}
		return true
	}
	if n, _ := fmt.Sscanf(id, "ws.tab.%d.%s", &k, &what); n != 2 {
		return false
	}
	i, t := d.tabByKey(k)
	if t == nil {
		return true
	}
	switch what {
	case "up":
		if i > 0 {
			d.tabs[i-1], d.tabs[i] = d.tabs[i], d.tabs[i-1]
		}
	case "down":
		if i < len(d.tabs)-1 {
			d.tabs[i+1], d.tabs[i] = d.tabs[i], d.tabs[i+1]
		}
	case "rm":
		if len(d.tabs) > 1 {
			d.tabs = slices.Delete(d.tabs, i, i+1)
		}
	case "addcmd":
		if len(t.cmds) < workspaces.MaxCommands {
			c := &wsDraftCmd{key: d.key()}
			t.cmds = append(t.cmds, c)
			win.editField(fmt.Sprintf("ws.cmd.%d", c.key))
		}
	case "color":
		if arg >= 0 && arg < len(wsColors) {
			t.color = wsColors[arg]
		}
	case "wait":
		if arg >= 0 && arg < len(wsWaits) {
			t.wait = wsWaits[arg]
		}
	}
	return true
}

// saveDraft writes the draft into the file it was opened from. A file changed
// on disk is never overwritten: the draft stays and the page offers Reload.
func (win *window) saveDraft() bool {
	p := &win.wsp
	d := p.draft
	if d == nil {
		return false
	}
	for _, t := range d.tabs {
		t.checked = false // a folder made or removed since it was typed shows now
	}
	if win.draftErrors(d).any {
		return false
	}
	ws := d.workspace()
	doc := workspaces.Document{Workspaces: slices.Clone(d.doc.Workspaces)}
	if i := slices.IndexFunc(doc.Workspaces, func(w workspaces.Workspace) bool { return w.ID == d.id }); i >= 0 {
		doc.Workspaces[i] = ws
	} else {
		doc.Workspaces = append(doc.Workspaces, ws)
	}
	_, err := workspaces.Save(workspaces.Path(win.cfgPath), doc, d.rev)
	switch {
	case errors.Is(err, workspaces.ErrStale):
		p.dialog = wsStaleDialog
		return false
	case err != nil:
		win.note("Workspace not saved: " + err.Error())
		return false
	}
	p.draft = nil
	win.loadWorkspaces()
	win.note("Saved " + strings.TrimSpace(ws.Name) + ". Tabs already running keep the old recipe; the next Run uses this one.")
	return true
}

// reloadDraft reads the file again and puts its copy of the workspace in the
// draft; a workspace the file does not have keeps the draft as typed.
func (win *window) reloadDraft() {
	d := win.wsp.draft
	if d == nil {
		return
	}
	doc, rev, err := workspaces.Load(workspaces.Path(win.cfgPath))
	if err != nil {
		win.note("workspaces: " + err.Error())
		return
	}
	win.loadWorkspaces()
	if ws, ok := doc.ByID(d.id); ok {
		win.setDraft(newDraft(&ws, doc, rev))
		return
	}
	d.doc, d.rev = doc, rev
}

// deleteWorkspace removes id from the file and stops it being the startup workspace.
func (win *window) deleteWorkspace(id string) {
	path := workspaces.Path(win.cfgPath)
	doc, rev, err := workspaces.Load(path)
	if err == nil {
		doc.Workspaces = slices.DeleteFunc(doc.Workspaces, func(w workspaces.Workspace) bool { return w.ID == id })
		_, err = workspaces.Save(path, doc, rev)
	}
	if err != nil {
		win.note("Workspace not deleted: " + err.Error())
	} else if win.opt.Config.StartupWorkspace == id {
		win.opt.Config.StartupWorkspace = ""
		win.saveSetting("startup_workspace", "")
	}
	win.loadWorkspaces()
}

// wsLeave lets the page leave the section: a clean draft is dropped, unsaved
// changes ask Save / Discard / Keep editing first.
func wsLeave(win *window, then func()) bool {
	p := &win.wsp
	if p.draft == nil {
		return true
	}
	if !p.draft.dirty() {
		p.draft = nil
		return true
	}
	p.dialog, p.then = wsLeaveDialog, then
	return false
}

func wsDialogBox(win *window) *dialogBox {
	p := &win.wsp
	switch p.dialog {
	case wsDeleteDialog:
		name := p.del
		for _, w := range p.doc.Workspaces {
			if w.ID == p.del {
				name = w.Name
			}
		}
		return &dialogBox{
			title: "Delete the " + cleanText(name) + " workspace?",
			lines: []string{"Tabs that are running keep running."},
			buttons: []ctrl{
				{kind: ctlButton, id: "ws.del.cancel", text: "Cancel"},
				{kind: ctlButton, id: "ws.del.ok", text: "Delete", strong: true},
			},
		}
	case wsStaleDialog:
		return &dialogBox{
			title: "The file changed on disk. Reload?",
			lines: []string{"Reload replaces your changes with the file as it is now. Keep editing leaves your draft as it is; it can't be saved over the newer file."},
			buttons: []ctrl{
				{kind: ctlButton, id: "ws.stale.keep", text: "Keep editing"},
				{kind: ctlButton, id: "ws.stale.reload", text: "Reload", strong: true},
			},
		}
	case wsLeaveDialog:
		valid := p.draft != nil && !win.draftErrors(p.draft).any
		lines := []string{"This workspace has changes that are not saved."}
		if !valid {
			lines = append(lines, "Fix the marked fields to save it.")
		}
		return &dialogBox{
			title: "Save your changes?", lines: lines,
			buttons: []ctrl{
				{kind: ctlButton, id: "ws.leave.keep", text: "Keep editing"},
				{kind: ctlButton, id: "ws.leave.discard", text: "Discard"},
				{kind: ctlButton, id: "ws.leave.save", text: "Save", strong: true, dead: !valid},
			},
		}
	}
	return nil
}
