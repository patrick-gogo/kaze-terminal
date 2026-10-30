package ui

import (
	"fmt"
	"image"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// wsPageRect is a tall page, so every editor row is on screen without scrolling.
var wsPageRect = image.Rect(0, 38, 900, 4000)

func wsIndex(t *testing.T) int {
	t.Helper()
	i := slices.IndexFunc(defaultSections(), func(s settingsSection) bool { return s.title == "Workspaces" })
	if i < 0 {
		t.Fatal("no Workspaces section")
	}
	return i
}

// newWSPage is the samurai run fixture with the Workspaces section shown.
func newWSPage(t *testing.T) *runFixture {
	t.Helper()
	f := newRunFixture(t)
	f.win.opt.Config = config.Default()
	f.win.openSettings()
	f.win.selectSection(wsIndex(t))
	f.draw()
	return f
}

func (f *runFixture) draw() {
	f.win.drawSettings(pageContext(wsPageRect.Dx(), wsPageRect.Max.Y+24), wsPageRect)
}

// press clicks the control with id and arg, then lays the page out again.
func (f *runFixture) press(id string, arg int) {
	f.t.Helper()
	c, ok := f.ctl(id, arg)
	if !ok {
		f.t.Fatalf("no %s control with arg %d; page %q", id, arg, f.win.set.texts)
	}
	f.win.settingsPointer(pressAt(center(c.rect)))
	f.draw()
}

func (f *runFixture) ctl(id string, arg int) (control, bool) {
	for _, c := range f.win.set.controls {
		if c.id == id && c.arg == arg {
			return c, true
		}
	}
	return control{}, false
}

// typeInto replaces field id's text, as an edit in its editor would.
func (f *runFixture) typeInto(id, s string) {
	f.t.Helper()
	fld := f.win.set.fields[id]
	if fld == nil || fld.set == nil {
		f.t.Fatalf("no field %s; fields %v", id, slices.Collect(maps.Keys(f.win.set.fields)))
	}
	fld.set(s)
	f.draw()
}

func (f *runFixture) file() string {
	f.t.Helper()
	b, err := os.ReadFile(workspaces.Path(f.win.cfgPath))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func (f *runFixture) has(s string) bool { return pageHas(f.win, s) }

func (f *runFixture) wantText(s string) {
	f.t.Helper()
	if !f.has(s) {
		f.t.Fatalf("page lacks %q: %q", s, f.win.set.texts)
	}
}

// tabKey is the draft key of tab i in the editor.
func (f *runFixture) tabKey(i int) int {
	f.t.Helper()
	d := f.win.wsp.draft
	if d == nil || i >= len(d.tabs) {
		f.t.Fatalf("no draft tab %d", i)
	}
	return d.tabs[i].key
}

func tabID(k int, what string) string { return fmt.Sprintf("ws.tab.%d.%s", k, what) }

// nameID is the open draft's name field.
func (f *runFixture) nameID() string {
	f.t.Helper()
	if f.win.wsp.draft == nil {
		f.t.Fatal("no draft open")
	}
	return f.win.wsp.draft.nameID
}

func TestWorkspacesIsASection(t *testing.T) {
	f := newWSPage(t)
	if defaultSections()[wsIndex(t)].later {
		t.Fatal("Workspaces is still greyed")
	}
	for _, s := range []string{"samurai", "Closed", "4 tabs", "Run", "Edit", "Delete", "+ New workspace", "Run a workspace when kaze starts"} {
		f.wantText(s)
	}
	if f.has("coming next") {
		t.Fatal("the nav still says coming next")
	}
	f.wantText("docker compose up -d → " + `.\venv\Scripts\Activate.ps1`)
	f.wantText("after port 8000 answers")
}

func TestWorkspaceListStates(t *testing.T) {
	f := newWSPage(t)
	f.win.runWorkspace("ws_samurai")
	f.draw()
	f.wantText("Starting")
	f.wantText("Go to tabs")
	run := f.theRun()
	for _, m := range run.members {
		m.run.State = workspaces.Launched
	}
	f.draw()
	f.wantText("Open")
	f.win.mgr.Close(run.members[3].tab)
	f.draw()
	f.wantText("3 of 4 tabs open")
	run.members[0].run.State = workspaces.Failed
	f.draw()
	f.wantText("Needs attention")
	for _, m := range run.members {
		f.win.mgr.Close(m.tab)
	}
	f.win.wsTick(f.now)
	f.draw()
	f.wantText("Closed")
	if f.has("Go to tabs") {
		t.Fatal("a closed workspace still offers Go to tabs")
	}
}

func TestRunAndGoToTabs(t *testing.T) {
	f := newWSPage(t)
	own, _ := f.win.mgr.New()
	f.press("ws.run", 0)
	if n := len(f.win.mgr.Tabs()); n != 5 || f.win.set.shown {
		t.Fatalf("%d tabs, settings shown %v after Run", n, f.win.set.shown)
	}
	first := f.theRun().members[0].tab
	f.win.selectTab(own.ID)
	f.win.openSettings()
	f.draw()
	f.press("ws.run", 0) // now Go to tabs
	if n := len(f.win.mgr.Tabs()); n != 5 || f.win.mgr.Active().ID != first || f.win.set.shown {
		t.Fatalf("Go to tabs: %d tabs, active %d want %d", n, f.win.mgr.Active().ID, first)
	}
}

func TestRunErrorShowsInTheNotice(t *testing.T) {
	f := newWSPage(t)
	f.win.opt.Shell = []string{`C:\Windows\System32\cmd.exe`}
	f.press("ws.run", 0)
	if len(f.win.mgr.Tabs()) != 0 || !strings.Contains(f.win.notice, "samurai") {
		t.Fatalf("tabs %d notice %q", len(f.win.mgr.Tabs()), f.win.notice)
	}
}

func TestEditThenCancelLeavesTheFileUntouched(t *testing.T) {
	f := newWSPage(t)
	before := f.file()
	f.press("ws.edit", 0)
	f.wantText("Edit workspace")
	f.typeInto(f.nameID(), "renamed")
	f.typeInto(tabID(f.tabKey(0), "title"), "backend")
	f.press("ws.cancel", 0)
	if f.win.wsp.draft != nil || f.file() != before {
		t.Fatal("Cancel kept the draft or wrote the file")
	}
	f.wantText("samurai")
}

func TestSaveWritesAndRunUsesTheSavedRecipe(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	k := f.tabKey(0)
	f.typeInto(tabID(k, "title"), "backend")
	cmd := f.win.wsp.draft.tabs[0].cmds[1].key
	f.press(fmt.Sprintf("ws.cmd.%d.rm", cmd), 0)
	f.typeInto(fmt.Sprintf("ws.cmd.%d", f.win.wsp.draft.tabs[0].cmds[0].key), "echo hi")
	f.press(tabID(k, "color"), 5) // red
	f.press("ws.save", 0)
	if f.win.wsp.draft != nil {
		t.Fatalf("draft kept after Save; notice %q", f.win.notice)
	}
	doc, _, err := workspaces.Load(workspaces.Path(f.win.cfgPath))
	if err != nil {
		t.Fatal(err)
	}
	got := doc.Workspaces[0].Tabs[0]
	if got.Title != "backend" || !slices.Equal(got.Commands, []string{"echo hi"}) || got.Color != "red" || got.ID != "t_api" {
		t.Fatalf("saved tab %+v", got)
	}
	f.wantText("backend")
	f.press("ws.run", 0)
	run := f.theRun()
	if run.members[0].title != "backend" || run.members[0].run.Line != "echo hi" {
		t.Fatalf("run used %q / %q", run.members[0].title, run.members[0].run.Line)
	}
}

func TestInvalidFieldBlocksSave(t *testing.T) {
	f := newWSPage(t)
	before := f.file()
	f.press("ws.edit", 0)
	f.typeInto(f.nameID(), "  ")
	f.wantText("Workspace name cannot be empty")
	if c, ok := f.ctl("ws.save", 0); !ok || !c.dead {
		t.Fatalf("Save is live while the name is empty: %+v", c)
	}
	f.press("ws.save", 0)
	if f.file() != before || f.win.wsp.draft == nil {
		t.Fatal("an invalid draft was saved")
	}
	f.typeInto(f.nameID(), "samurai")
	k := f.tabKey(1)
	f.sel(1)
	f.typeInto(tabID(k, "folder"), "relative")
	f.wantText("Folder must be a full path")
	f.typeInto(tabID(k, "folder"), f.dirs[1])
	f.press(tabID(k, "wait"), 2) // Port
	f.typeInto(tabID(k, "port"), "abc")
	f.wantText("Port must be a number from 1 to 65535")
	if c, _ := f.ctl("ws.save", 0); !c.dead {
		t.Fatal("Save is live with a bad port")
	}
	f.typeInto(tabID(k, "port"), "3000")
	if c, _ := f.ctl("ws.save", 0); c.dead {
		t.Fatalf("Save stays dead once valid: %q", f.win.set.texts)
	}
}

func TestPortFieldOnlyForPortWait(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	k0, k3 := f.tabKey(0), f.tabKey(3)
	if _, ok := f.win.set.fields[tabID(k0, "port")]; ok {
		t.Fatal("a Docker tab shows a port field")
	}
	f.sel(3)
	if f.win.fieldText(tabID(k3, "port")) != "8000" {
		t.Fatalf("port field %q", f.win.fieldText(tabID(k3, "port")))
	}
	f.press(tabID(k3, "wait"), 0) // None
	if _, ok := f.win.set.fields[tabID(k3, "port")]; ok {
		t.Fatal("the port field stayed after choosing None")
	}
	f.press("ws.save", 0)
	doc, _, _ := workspaces.Load(workspaces.Path(f.win.cfgPath))
	if tb := doc.Workspaces[0].Tabs[3]; tb.Wait != workspaces.WaitNone || tb.Port != 0 {
		t.Fatalf("saved %+v", tb)
	}
}

// Review focus 3: a hand edit while the editor is open is never overwritten.
func TestStaleSaveIsRefusedAndTheDraftKept(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.typeInto(f.nameID(), "mine")
	doc, _, _ := workspaces.Load(workspaces.Path(f.win.cfgPath))
	ws := doc.Workspaces[0]
	ws.Name = "theirs"
	f.save(ws)
	onDisk := f.file()
	f.press("ws.save", 0)
	f.wantText("The file changed on disk. Reload?")
	if f.file() != onDisk || f.win.wsp.draft == nil || f.win.wsp.draft.name != "mine" {
		t.Fatal("a stale save wrote the file or lost the draft")
	}
	f.press("ws.stale.keep", 0)
	if f.has("The file changed on disk. Reload?") || f.win.wsp.draft.name != "mine" {
		t.Fatal("Keep editing did not keep the draft")
	}
	f.press("ws.save", 0)
	f.press("ws.stale.reload", 0)
	if d := f.win.wsp.draft; d == nil || d.name != "theirs" {
		t.Fatalf("Reload did not load the file into the draft: %+v", d)
	}
	f.typeInto(f.nameID(), "mine again")
	f.press("ws.save", 0)
	if !strings.Contains(f.file(), "mine again") {
		t.Fatal("a save after Reload did not write")
	}
}

func TestSaveOverAnInvalidFileKeepsTheDraft(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.typeInto(f.nameID(), "mine")
	path := workspaces.Path(f.win.cfgPath)
	if err := os.WriteFile(path, []byte("not = [valid"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.press("ws.save", 0)
	if f.file() != "not = [valid" || f.win.wsp.draft == nil || f.win.wsp.draft.name != "mine" {
		t.Fatal("saving over an invalid file replaced it or lost the draft")
	}
	if !strings.Contains(f.win.notice, "problem") {
		t.Fatalf("notice %q", f.win.notice)
	}
}

func TestInvalidFileShowsItsError(t *testing.T) {
	f := newWSPage(t)
	if err := os.WriteFile(workspaces.Path(f.win.cfgPath), []byte("schema_version = 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.win.hideSettings()
	f.win.openSettings() // the list is read again each time it shows
	f.draw()
	f.wantText("schema_version must be 1")
	if _, ok := f.ctl("ws.run", 0); ok {
		t.Fatal("a broken file still offers Run")
	}
}

func TestDeleteAsksAndClearsTheStartupWorkspace(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.startup", 1)
	if f.win.opt.Config.StartupWorkspace != "ws_samurai" || !strings.Contains(readFile(t, f.win.cfgPath), `startup_workspace = "ws_samurai"`) {
		t.Fatalf("startup %q", f.win.opt.Config.StartupWorkspace)
	}
	f.press("ws.delete", 0)
	f.wantText("Tabs that are running keep running.")
	f.press("ws.del.cancel", 0)
	if !strings.Contains(f.file(), "samurai") {
		t.Fatal("Cancel deleted the workspace")
	}
	f.press("ws.delete", 0)
	f.press("ws.del.ok", 0)
	if strings.Contains(f.file(), "samurai") {
		t.Fatal("Delete kept the workspace")
	}
	if f.win.opt.Config.StartupWorkspace != "" || !strings.Contains(readFile(t, f.win.cfgPath), `startup_workspace = ""`) {
		t.Fatalf("startup not cleared: %q", readFile(t, f.win.cfgPath))
	}
	f.wantText("No workspaces yet")
}

func TestStartupRowNone(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.startup", 1)
	f.press("ws.startup", 0)
	if f.win.opt.Config.StartupWorkspace != "" || !strings.Contains(readFile(t, f.win.cfgPath), `startup_workspace = ""`) {
		t.Fatal("None did not clear the startup workspace")
	}
	if len(f.win.mgr.Tabs()) != 0 {
		t.Fatal("choosing a startup workspace ran it")
	}
}

func TestNewWorkspace(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.delete", 0)
	f.press("ws.del.ok", 0)
	f.press("ws.new", 0)
	f.wantText("New workspace")
	k := f.tabKey(0)
	if f.win.set.editing != f.nameID() {
		t.Fatalf("editing %q, want the name field", f.win.set.editing)
	}
	f.typeInto(f.nameID(), "notes")
	f.typeInto(tabID(k, "title"), "shell")
	f.typeInto(tabID(k, "folder"), f.dirs[0])
	f.press("ws.save", 0)
	doc, _, err := workspaces.Load(workspaces.Path(f.win.cfgPath))
	if err != nil || len(doc.Workspaces) != 1 {
		t.Fatalf("doc %+v err %v; notice %q", doc, err, f.win.notice)
	}
	w := doc.Workspaces[0]
	if w.Name != "notes" || !strings.HasPrefix(w.ID, "ws_") || len(w.Tabs) != 1 || len(w.Tabs[0].Commands) != 0 || !strings.HasPrefix(w.Tabs[0].ID, "t_") {
		t.Fatalf("saved %+v", w)
	}
}

func TestDuplicateNameBlocksSave(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.new", 0)
	f.typeInto(f.nameID(), "SAMURAI")
	f.wantText(`Another workspace is already named "SAMURAI"`)
	if c, _ := f.ctl("ws.save", 0); !c.dead {
		t.Fatal("Save is live with a duplicate name")
	}
}

func TestEditorTabAndCommandOps(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	d := f.win.wsp.draft
	titles := func() string {
		var s []string
		for _, tb := range d.tabs {
			s = append(s, tb.title)
		}
		return strings.Join(s, ",")
	}
	f.press("ws.addtab", 0)
	if len(d.tabs) != 5 || f.win.set.editing != tabID(f.tabKey(4), "title") {
		t.Fatalf("%d tabs, editing %q after Add tab", len(d.tabs), f.win.set.editing)
	}
	f.typeInto(tabID(f.tabKey(4), "title"), "extra")
	f.press(tabID(f.tabKey(4), "up"), 0)
	if titles() != "api,admin,storefront,extra,api logs" {
		t.Fatalf("after up: %s", titles())
	}
	f.sel(0)
	f.press(tabID(f.tabKey(0), "down"), 0)
	if titles() != "admin,api,storefront,extra,api logs" {
		t.Fatalf("after down: %s", titles())
	}
	f.sel(0)
	if c, _ := f.ctl(tabID(f.tabKey(0), "up"), 0); !c.dead {
		t.Fatal("the first tab can move up")
	}
	f.sel(3)
	f.press(tabID(f.tabKey(3), "rm"), 0)
	if titles() != "admin,api,storefront,api logs" {
		t.Fatalf("after remove: %s", titles())
	}
	k := f.tabKey(0)
	f.sel(0)
	f.press(tabID(k, "addcmd"), 0)
	cmds := d.tabs[0].cmds
	if len(cmds) != 2 || f.win.set.editing != fmt.Sprintf("ws.cmd.%d", cmds[1].key) {
		t.Fatalf("%d commands, editing %q after Add command", len(cmds), f.win.set.editing)
	}
	// The field ids follow the items, so the text stays with its command.
	f.typeInto(fmt.Sprintf("ws.cmd.%d", cmds[1].key), "npm test")
	f.press(fmt.Sprintf("ws.cmd.%d.rm", cmds[0].key), 0)
	if got := d.tabs[0].cmds; len(got) != 1 || f.win.fieldText(fmt.Sprintf("ws.cmd.%d", got[0].key)) != "npm test" {
		t.Fatalf("after removing the first command: %+v", got)
	}
}

func TestJoinedLinePreview(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.wantText(`docker compose up -d; if ($?) { .\venv\Scripts\Activate.ps1 }`)
	f.win.opt.Shell = []string{"/bin/zsh"}
	f.draw()
	f.wantText(`{ docker compose up -d; } && { .\venv\Scripts\Activate.ps1; }`)
	f.win.opt.Shell = []string{`C:\Windows\System32\cmd.exe`}
	f.draw()
	f.wantText("cmd: commands can't run in this shell")
}

func TestLeavingAnUnsavedDraftAsks(t *testing.T) {
	f := newWSPage(t)
	before := f.file()
	ws := wsIndex(t)

	// A clean draft just goes.
	f.press("ws.edit", 0)
	f.win.selectSection(0)
	if f.win.set.section != 0 || f.win.wsp.draft != nil {
		t.Fatal("leaving a clean draft asked or kept it")
	}

	f.win.selectSection(ws)
	f.draw()
	f.press("ws.edit", 0)
	f.typeInto(f.nameID(), "changed")
	f.win.selectSection(0)
	f.draw()
	if f.win.set.section != ws {
		t.Fatal("left the section with unsaved changes")
	}
	f.wantText("Save")
	f.wantText("Discard")
	f.press("ws.leave.keep", 0)
	if f.win.set.section != ws || f.win.wsp.draft == nil || f.win.wsp.draft.name != "changed" {
		t.Fatal("Keep editing lost the draft")
	}

	f.win.selectSection(0)
	f.draw()
	f.press("ws.leave.discard", 0)
	if f.win.set.section != 0 || f.win.wsp.draft != nil || f.file() != before {
		t.Fatal("Discard did not leave and drop the draft")
	}

	// Closing settings: Save writes, then closes.
	f.win.selectSection(ws)
	f.draw()
	f.press("ws.edit", 0)
	f.typeInto(f.nameID(), "saved on close")
	f.win.closeSettings()
	if !f.win.set.open {
		t.Fatal("settings closed over unsaved changes")
	}
	f.draw()
	f.press("ws.leave.save", 0)
	if f.win.set.open || !strings.Contains(f.file(), "saved on close") {
		t.Fatal("Save on leaving did not save and close")
	}
}

func TestOpeningAnotherWorkspaceAsks(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.typeInto(f.nameID(), "changed")
	f.win.activate("ws.new", 0)
	f.draw()
	if d := f.win.wsp.draft; d == nil || d.name != "changed" {
		t.Fatal("opening another workspace dropped unsaved changes")
	}
	f.press("ws.leave.discard", 0)
	if d := f.win.wsp.draft; d == nil || d.name != "" || !d.isNew {
		t.Fatalf("Discard did not open the new draft: %+v", d)
	}
}

// Review focus 5: a theme switch keeps the draft.
func TestThemeSwitchKeepsTheDraft(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.typeInto(f.nameID(), "mine")
	f.win.applyTheme(mustTheme(t, "washi"))
	f.draw()
	if f.win.wsp.draft == nil || f.win.fieldText(f.nameID()) != "mine" {
		t.Fatal("a theme switch lost the draft")
	}
}

// Each folder is looked up on disk when its text changes, not on every frame.
func TestFoldersAreCheckedOnlyWhenTheyChange(t *testing.T) {
	f := newWSPage(t)
	n := 0
	real := statFolder
	statFolder = func(p string) (os.FileInfo, error) { n++; return real(p) }
	t.Cleanup(func() { statFolder = real })
	f.press("ws.edit", 0)
	f.draw()
	if n != len(samuraiTabs) {
		t.Fatalf("%d folder checks for an opened draft, want one per tab (%d)", n, len(samuraiTabs))
	}
	k := f.tabKey(0)
	f.typeInto(f.nameID(), "renamed")
	f.typeInto(tabID(k, "title"), "backend")
	f.typeInto(fmt.Sprintf("ws.cmd.%d", f.win.wsp.draft.tabs[0].cmds[0].key), "echo hi")
	f.draw()
	if n != len(samuraiTabs) {
		t.Fatalf("%d folder checks after typing in other fields, want %d", n, len(samuraiTabs))
	}
	f.typeInto(tabID(k, "folder"), f.dirs[0]+"-gone")
	f.draw()
	if n != len(samuraiTabs)+1 {
		t.Fatalf("%d folder checks after changing one folder, want %d", n, len(samuraiTabs)+1)
	}
	f.wantText("This folder does not exist yet; Run stops here until it does.")
	// The unsaved-changes dialog lays the draft's checks out every frame too.
	f.win.selectSection(0)
	f.draw()
	f.draw()
	if n != len(samuraiTabs)+1 {
		t.Fatalf("%d folder checks under the leave dialog, want %d", n, len(samuraiTabs)+1)
	}
	f.press("ws.leave.keep", 0)
	if err := os.MkdirAll(f.dirs[0]+"-gone", 0o755); err != nil {
		t.Fatal(err)
	}
	before := n
	f.press("ws.save", 0) // Save looks again, once per folder
	if n != before+len(samuraiTabs) || f.win.wsp.draft != nil {
		t.Fatalf("%d folder checks on Save, want %d; draft kept %v", n-before, len(samuraiTabs), f.win.wsp.draft != nil)
	}
}

// Undo in a newly opened draft never brings back text from the draft before it.
func TestUndoInANewDraftKeepsItsOwnText(t *testing.T) {
	f := newWSPage(t)
	if _, err := f.win.mgr.New(); err != nil { // the frame draws nothing without a tab
		t.Fatal(err)
	}
	f.win.openSettings()
	var r input.Router
	frame := func() { sizedFrame(t, f.win, &r, 900, 1600) }
	frame()
	f.win.activate("ws.edit", 0)
	frame()
	a := f.nameID()
	f.win.editField(a)
	frame()
	frame()
	ed := &f.win.set.fields[a].ed
	if !r.Source().Focused(ed) {
		t.Fatal("draft A's name field does not have the keyboard")
	}
	r.Queue(key.EditEvent{Range: key.Range{Start: ed.Len(), End: ed.Len()}, Text: "XYZ"})
	frame()
	frame()
	if got := f.win.wsp.draft.name; got != "samuraiXYZ" {
		t.Fatalf("draft A name %q after typing", got)
	}
	f.win.activate("ws.new", 0) // asks about A's changes
	frame()
	f.win.activate("ws.leave.discard", 0) // drops A and opens B in the same call
	frame()
	frame()
	d := f.win.wsp.draft
	if d == nil || !d.isNew {
		t.Fatalf("draft B did not open: %+v", d)
	}
	b := f.nameID()
	if f.win.set.editing != b || !r.Source().Focused(&f.win.set.fields[b].ed) {
		t.Fatalf("editing %q, want draft B's name field", f.win.set.editing)
	}
	r.Queue(key.Event{Name: "Z", Modifiers: key.ModShortcut, State: key.Press})
	frame()
	frame()
	if d.name != "" || f.win.fieldText(b) != "" {
		t.Fatalf("Ctrl+Z in draft B brought back %q (field %q)", d.name, f.win.fieldText(b))
	}
}

// sel selects draft tab i, as a click on its row in the tab list would.
func (f *runFixture) sel(i int) {
	f.t.Helper()
	wsAct(f.win, "ws.sel", f.tabKey(i))
	f.draw()
}

func TestEditorSelectsFirstTabAndSwitches(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	d := f.win.wsp.draft
	if d.sel != f.tabKey(0) {
		t.Fatalf("selected %d after Edit, want the first tab %d", d.sel, f.tabKey(0))
	}
	f.sel(2)
	if d.sel != f.tabKey(2) {
		t.Fatalf("selected %d, want tab 2's %d", d.sel, f.tabKey(2))
	}
	for i := range d.tabs {
		_, has := f.win.set.fields[tabID(f.tabKey(i), "title")]
		if has != (i == 2) {
			t.Fatalf("tab %d title field shown: %v; only the selected tab's fields show", i, has)
		}
	}
}

func TestAddTabTakesNextUnusedColorAndIsSelected(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	d := f.win.wsp.draft
	for i, c := range []string{"cyan", "magenta", "yellow", "green"} {
		d.tabs[i].color = c
	}
	f.press("ws.addtab", 0)
	nt := d.tabs[len(d.tabs)-1]
	if nt.color != "blue" || d.sel != nt.key || f.win.set.editing != tabID(nt.key, "title") {
		t.Fatalf("new tab color %q, selected %v, editing %q", nt.color, d.sel == nt.key, f.win.set.editing)
	}
}

func TestNextColorCyclesWhenAllAreUsed(t *testing.T) {
	var tabs []*wsDraftTab
	for _, c := range wsColors {
		tabs = append(tabs, &wsDraftTab{color: c})
	}
	if got := nextColor(tabs); got != wsColors[0] {
		t.Fatalf("nextColor with all six used = %q, want %q", got, wsColors[0])
	}
}

func TestSwitchingTabsKeepsTypedText(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	id := tabID(f.tabKey(0), "title")
	f.win.editField(id)
	f.typeInto(id, "api-2")
	f.sel(1)
	if f.win.set.editing == id {
		t.Fatal("tab 0's title still has the keyboard after selecting tab 1")
	}
	f.sel(0)
	if got := f.win.wsp.draft.tabs[0].title; got != "api-2" || f.win.fieldText(id) != "api-2" {
		t.Fatalf("title %q / field %q after switching away and back", got, f.win.fieldText(id))
	}
}

func TestRemovingSelectedTabSelectsNeighbor(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	d := f.win.wsp.draft
	f.sel(3)
	f.press(tabID(f.tabKey(3), "rm"), 0)
	if len(d.tabs) != 3 || d.sel != f.tabKey(2) {
		t.Fatalf("%d tabs, selected %d, want the new last tab %d", len(d.tabs), d.sel, f.tabKey(2))
	}
	f.sel(1)
	f.press(tabID(f.tabKey(1), "rm"), 0)
	if d.sel != f.tabKey(1) {
		t.Fatalf("after removing the middle tab, selected %d, want the tab now at its place %d", d.sel, f.tabKey(1))
	}
}

// fakePicker stands in for the system folder dialog: each call reports its
// start folder and waits for the test to answer.
type fakePicker struct {
	starts chan string
	answer chan string // "" means cancel
}

func usePicker(t *testing.T) *fakePicker {
	t.Helper()
	fp := &fakePicker{starts: make(chan string, 4), answer: make(chan string, 4)}
	old := pickFolder
	pickFolder = func(start string) (string, bool, error) {
		fp.starts <- start
		select {
		case p := <-fp.answer:
			return p, p != "", nil
		case <-time.After(2 * time.Second):
			return "", false, nil
		}
	}
	t.Cleanup(func() { pickFolder = old })
	return fp
}

// settle draws until the picker's answer has been applied, for at most 2s.
func (f *runFixture) settlePick() {
	f.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for f.win.wsp.picking {
		if time.Now().After(deadline) {
			f.t.Fatal("the picker's answer never arrived")
		}
		time.Sleep(5 * time.Millisecond)
		f.draw()
	}
}

func TestPickFillsFolderTildeAndEmptyTitle(t *testing.T) {
	fp := usePicker(t)
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.press("ws.addtab", 0)
	d := f.win.wsp.draft
	nt := d.tabs[len(d.tabs)-1]
	f.press(tabID(nt.key, "pick"), 0)
	if start := <-fp.starts; start != f.win.home {
		t.Fatalf("picker started in %q, want the home folder %q for an empty field", start, f.win.home)
	}
	fp.answer <- filepath.Join(f.win.home, "work", "frontend")
	f.settlePick()
	want := "~" + string(filepath.Separator) + filepath.Join("work", "frontend")
	if nt.folder != want || f.win.fieldText(tabID(nt.key, "folder")) != want || nt.title != "frontend" {
		t.Fatalf("folder %q field %q title %q, want %q and title frontend", nt.folder, f.win.fieldText(tabID(nt.key, "folder")), nt.title, want)
	}
	// A title the owner typed stays.
	f.sel(0)
	f.press(tabID(f.tabKey(0), "pick"), 0)
	if start := <-fp.starts; start != f.dirs[0] {
		t.Fatalf("picker started in %q, want the tab's folder %q", start, f.dirs[0])
	}
	fp.answer <- filepath.Join(f.win.home, "other")
	f.settlePick()
	if d.tabs[0].title != "api" {
		t.Fatalf("picking a folder changed the typed title to %q", d.tabs[0].title)
	}
}

func TestPickCancelChangesNothing(t *testing.T) {
	fp := usePicker(t)
	f := newWSPage(t)
	f.press("ws.edit", 0)
	before := f.win.wsp.draft.tabs[0].folder
	f.press(tabID(f.tabKey(0), "pick"), 0)
	<-fp.starts
	fp.answer <- ""
	f.settlePick()
	if got := f.win.wsp.draft.tabs[0].folder; got != before {
		t.Fatalf("cancel changed the folder to %q", got)
	}
}

func TestPickResultForRemovedTabIsDropped(t *testing.T) {
	fp := usePicker(t)
	f := newWSPage(t)
	f.press("ws.edit", 0)
	d := f.win.wsp.draft
	f.sel(2)
	k := f.tabKey(2)
	f.press(tabID(k, "pick"), 0)
	<-fp.starts
	f.press(tabID(k, "rm"), 0)
	var before []string
	for _, tb := range d.tabs {
		before = append(before, tb.folder)
	}
	fp.answer <- filepath.Join(f.win.home, "elsewhere")
	f.settlePick()
	for i, tb := range d.tabs {
		if tb.folder != before[i] {
			t.Fatalf("tab %d folder became %q after its neighbour's picker answered", i, tb.folder)
		}
	}
}

func TestPickResultAfterTheDraftClosedIsDropped(t *testing.T) {
	fp := usePicker(t)
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.press(tabID(f.tabKey(0), "pick"), 0)
	<-fp.starts
	f.press("ws.cancel", 0)
	f.press("ws.edit", 0)
	reopened := f.win.wsp.draft.tabs[0].folder
	fp.answer <- filepath.Join(f.win.home, "elsewhere")
	f.settlePick()
	if got := f.win.wsp.draft.tabs[0].folder; got != reopened {
		t.Fatalf("an answer for the closed draft landed in the new one: %q", got)
	}
}

func TestSecondPickWhileOpenDoesNothing(t *testing.T) {
	fp := usePicker(t)
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.press(tabID(f.tabKey(0), "pick"), 0)
	f.press(tabID(f.tabKey(0), "pick"), 0)
	<-fp.starts
	select {
	case <-fp.starts:
		t.Fatal("a second picker opened while the first was up")
	case <-time.After(50 * time.Millisecond):
	}
	fp.answer <- ""
	f.settlePick()
}

// ctlsOf is every control with id this frame.
func (f *runFixture) ctlsOf(id string) []control {
	var out []control
	for _, c := range f.win.set.controls {
		if c.id == id {
			out = append(out, c)
		}
	}
	return out
}

func TestEditorListShowsEveryTabWithCommandCount(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	for _, s := range []string{"api", "admin", "storefront", "api logs", "2 commands", "1 command"} {
		f.wantText(s)
	}
	items := f.ctlsOf("ws.sel")
	if len(items) != 4 {
		t.Fatalf("%d tab list items, want 4", len(items))
	}
	title, ok := f.ctl(tabID(f.tabKey(0), "title"), 0)
	if !ok {
		t.Fatal("no title field for the selected tab")
	}
	for i, it := range items {
		if it.arg != f.tabKey(i) || it.rect.Max.X > title.rect.Min.X {
			t.Fatalf("item %d (arg %d) at %v is not left of the title field %v", i, it.arg, it.rect, title.rect)
		}
	}
}

func TestEditorMarksTabsWithProblems(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	f.sel(2)
	f.typeInto(tabID(f.tabKey(2), "title"), "")
	f.sel(0)
	if !slices.Equal(f.win.wsp.flagged, []int{f.tabKey(2)}) {
		t.Fatalf("flagged tabs %v, want only tab 2's key %d", f.win.wsp.flagged, f.tabKey(2))
	}
}

func TestEditorShowsChooseAndNumberedCommands(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	if c, ok := f.ctl(tabID(f.tabKey(0), "pick"), 0); !ok || c.kind != ctlButton {
		t.Fatal("no Choose… button for the folder")
	}
	f.wantText("Choose…")
	f.wantText("Command 1")
	f.wantText("Command 2")
	f.wantText("×")
	if f.has("Tab 1") {
		t.Fatal(`the old "Tab 1" heading is still drawn`)
	}
}

func TestEditorStacksWhenNarrow(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	narrow := image.Rect(0, 38, 700, 4000)
	f.win.drawSettings(pageContext(narrow.Dx(), narrow.Max.Y+24), narrow)
	title, ok := f.ctl(tabID(f.tabKey(0), "title"), 0)
	if !ok {
		t.Fatal("no title field")
	}
	for _, it := range f.ctlsOf("ws.sel") {
		if it.rect.Max.Y > title.rect.Min.Y {
			t.Fatalf("list item %v is not above the title field %v in a narrow page", it.rect, title.rect)
		}
	}
}

// Opening or closing the editor replaces every control, so the clicked button's
// focus must not land on whatever control now has its place.
func TestOpeningAndClosingTheEditorDropsFocus(t *testing.T) {
	f := newWSPage(t)
	f.press("ws.edit", 0)
	if f.win.set.focus != -1 {
		t.Fatalf("focus %d after opening the editor, want none", f.win.set.focus)
	}
	f.press("ws.cancel", 0)
	if f.win.set.focus != -1 {
		t.Fatalf("focus %d after closing the editor, want none", f.win.set.focus)
	}
}
