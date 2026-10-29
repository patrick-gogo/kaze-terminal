package ui

import (
	"io"
	"strings"
	"testing"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/transfer"

	"github.com/patrick-gogo/kaze-terminal/internal/theme"
)

// fieldDraft is what a fieldSection's rows show and its fields write back.
type fieldDraft struct {
	name, dir string
	dirErr    string
	hideDir   bool // drops the dir field from the rows
}

// fieldSection is a toggle, two text fields and a button, in that focus order.
func fieldSection(d *fieldDraft) settingsSection {
	return settingsSection{
		title: "Fields",
		rows: func(*window) []settingRow {
			rows := []settingRow{
				{label: "Flag", ctrls: []ctrl{{kind: ctlToggle, id: "flag"}}},
				{label: "Name", ctrls: []ctrl{{kind: ctlText, id: "name", val: d.name, placeholder: "my-app", width: 200,
					set: func(s string) { d.name = s }}}},
			}
			if !d.hideDir {
				rows = append(rows, settingRow{label: "Folder", ctrls: []ctrl{{kind: ctlText, id: "dir", val: d.dir, err: d.dirErr, width: 160,
					set: func(s string) { d.dir = s }}}})
			}
			return append(rows, settingRow{label: "Go", ctrls: []ctrl{{kind: ctlButton, id: "go", text: "Save"}}})
		},
		act: func(*window, string, int) {},
	}
}

type fieldRun struct {
	t   *testing.T
	win *window
	r   *input.Router
	at  time.Time
}

// fieldWindow is a window whose settings page shows d, laid out and focused.
func fieldWindow(t *testing.T, win *window, d *fieldDraft) *fieldRun {
	t.Helper()
	win.set.sections = []settingsSection{fieldSection(d)}
	f := &fieldRun{t: t, win: win, r: new(input.Router), at: time.Unix(1000, 0)}
	win.openSettings()
	f.frame()
	f.frame()
	return f
}

func (f *fieldRun) frame() {
	f.t.Helper()
	frameWith(f.t, f.win, f.r, f.at)
}

// send queues evs and runs two frames: one to take them, one to settle focus.
func (f *fieldRun) send(evs ...event.Event) {
	f.t.Helper()
	f.r.Queue(evs...)
	f.frame()
	f.frame()
}

func modPress(name key.Name, mods key.Modifiers) key.Event {
	return key.Event{Name: name, Modifiers: mods, State: key.Press}
}

func (f *fieldRun) typeText(s string) {
	f.t.Helper()
	ed := &f.win.set.fields[f.win.set.editing].ed
	at := ed.Len()
	f.send(key.EditEvent{Range: key.Range{Start: at, End: at}, Text: s})
}

func (f *fieldRun) clickField(id string) {
	f.t.Helper()
	cs := controlsOf(f.win, id)
	if len(cs) != 1 {
		f.t.Fatalf("%d hit areas for %s, want 1", len(cs), id)
	}
	p := center(cs[0].rect)
	f.send(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p},
	)
}

func (f *fieldRun) focusedOn(tag any) bool {
	return f.r.Source().Focused(tag)
}

func (f *fieldRun) wantEditing(id string) {
	f.t.Helper()
	s := &f.win.set
	if s.editing != id {
		f.t.Fatalf("editing %q, want %q", s.editing, id)
	}
	if id == "" {
		if !f.focusedOn(&s.tag) {
			f.t.Fatal("the page does not have the keyboard")
		}
		return
	}
	if !f.focusedOn(&s.fields[id].ed) {
		f.t.Fatalf("field %s does not have the keyboard", id)
	}
	if s.focus < 0 || s.focus >= len(s.controls) || s.controls[s.focus].id != id {
		f.t.Fatalf("page focus %d is not on %s", s.focus, id)
	}
}

func fieldWin(t *testing.T) (*fieldRun, *fieldDraft) {
	win, _ := alertWindow(t, 1)
	d := &fieldDraft{dir: "~/code"}
	return fieldWindow(t, win, d), d
}

func TestTextFieldShowsValueAndWidth(t *testing.T) {
	f, d := fieldWin(t)
	if got := f.win.fieldText("dir"); got != "~/code" {
		t.Fatalf("dir field holds %q, want the row's value", got)
	}
	if !pageHas(f.win, "my-app") {
		t.Fatal("the empty name field shows no placeholder")
	}
	if cs := controlsOf(f.win, "dir"); len(cs) != 1 || cs[0].rect.Dx() != 160 {
		t.Fatalf("dir hit areas %+v, want one 160 wide", cs)
	}
	d.dirErr = "No such folder"
	f.frame()
	if !pageHas(f.win, "No such folder") {
		t.Fatal("the error line is not drawn")
	}
	// A value the row changes replaces the text; one it keeps does not.
	d.dir = "~/other"
	f.frame()
	if got := f.win.fieldText("dir"); got != "~/other" {
		t.Fatalf("dir %q after the row changed it", got)
	}
}

func TestTextFieldClickFocusesAndTypingUpdatesValue(t *testing.T) {
	f, d := fieldWin(t)
	f.clickField("name")
	f.wantEditing("name")
	f.typeText("web")
	if d.name != "web" || f.win.fieldText("name") != "web" {
		t.Fatalf("draft %q field %q after typing web", d.name, f.win.fieldText("name"))
	}
	// The section keeps the typed value, so the next frame must not reset the caret.
	f.typeText("app")
	if d.name != "webapp" {
		t.Fatalf("draft %q after typing more, want webapp", d.name)
	}
}

func TestTextFieldTabMovesThroughFields(t *testing.T) {
	f, _ := fieldWin(t)
	// Tab from the page: the toggle first, then the name field.
	f.send(modPress(key.NameTab, 0))
	f.wantEditing("")
	f.send(modPress(key.NameTab, 0))
	f.wantEditing("name")
	f.send(modPress(key.NameTab, 0))
	f.wantEditing("dir")
	f.send(modPress(key.NameTab, 0))
	f.wantEditing("")
	if c := f.win.set.controls[f.win.set.focus]; c.id != "go" {
		t.Fatalf("Tab out of the last field focused %q, want go", c.id)
	}
	f.send(modPress(key.NameTab, key.ModShift))
	f.wantEditing("dir")
	f.send(modPress(key.NameTab, key.ModShift))
	f.wantEditing("name")
}

func TestTextFieldEscAndEnterReturnToPage(t *testing.T) {
	f, d := fieldWin(t)
	f.clickField("name")
	f.send(modPress(key.NameEscape, 0))
	f.wantEditing("")
	if c := f.win.set.controls[f.win.set.focus]; c.id != "name" {
		t.Fatalf("Esc moved the page focus to %q", c.id)
	}
	f.send(key.EditEvent{Text: "z"})
	if d.name != "" {
		t.Fatalf("typing after Esc changed the field to %q", d.name)
	}
	// Enter on the focused field goes back in; Enter inside it leaves.
	f.send(modPress(key.NameReturn, 0))
	f.wantEditing("name")
	f.send(modPress(key.NameReturn, 0))
	f.wantEditing("")
	// A click elsewhere on the page also takes the keyboard back.
	f.clickField("dir")
	f.wantEditing("dir")
	f.win.settingsPointer(pressAt(center(controlsOf(f.win, "go")[0].rect)))
	f.frame()
	f.wantEditing("")
}

func TestTextFieldPasteAndCopy(t *testing.T) {
	f, d := fieldWin(t)
	f.clickField("name")
	paste := func(mods key.Modifiers, text string) {
		t.Helper()
		f.send(modPress("V", mods))
		if !f.r.ClipboardRequested() {
			t.Fatalf("mods %v: no clipboard read", mods)
		}
		f.send(transfer.DataEvent{Type: "application/text", Open: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(text))
		}})
	}
	paste(key.ModCtrl|key.ModShift, "api")
	paste(key.ModCtrl, "-dev")
	if d.name != "api-dev" {
		t.Fatalf("draft %q after two pastes, want api-dev", d.name)
	}
	f.send(modPress("A", key.ModCtrl))
	f.send(modPress("C", key.ModCtrl|key.ModShift))
	if _, b, ok := f.r.WriteClipboard(); !ok || string(b) != "api-dev" {
		t.Fatalf("Ctrl+Shift+C copied %q %v", b, ok)
	}
	f.wantEditing("name")
}

func TestTextFieldLongTextScrollsInside(t *testing.T) {
	f, d := fieldWin(t)
	d.dir = strings.Repeat("very/long/folder/", 20)
	f.frame()
	f.clickField("dir")
	f.send(modPress(key.NameEnd, 0))
	r := controlsOf(f.win, "dir")[0].rect
	if r.Dx() != 160 {
		t.Fatalf("the field grew to %d px with long text", r.Dx())
	}
	x := f.win.set.fields["dir"].ed.CaretCoords().X
	if x <= 0 || x > float32(r.Dx()) {
		t.Fatalf("caret at x=%v after End, want inside the %d px field", x, r.Dx())
	}
}

func TestTextFieldKeysNeverReachTheTerminal(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	win.opt.Config.FontSize = 14
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	d := &fieldDraft{}
	f := fieldWindow(t, win, d)
	f.clickField("name")
	f.send(modPress("A", 0), key.EditEvent{Text: "a"}, modPress(key.NameLeftArrow, 0), modPress(key.NameEnd, 0),
		modPress("C", key.ModCtrl), modPress(key.NameF5, 0), modPress(key.NameUpArrow, 0))
	f.typeText("x")
	time.Sleep(50 * time.Millisecond)
	if got := st.ptys[0].sent(); got != "" {
		t.Fatalf("the shell got %q while a field had the keyboard", got)
	}
	if d.name != "ax" {
		t.Fatalf("field %q, want ax", d.name)
	}
	// kaze's own Ctrl shortcuts still work from inside a field.
	f.send(modPress("T", key.ModCtrl|key.ModShift))
	if n := len(win.mgr.Tabs()); n != 2 || win.set.shown {
		t.Fatalf("Ctrl+Shift+T in a field: %d tabs, settings shown %v", n, win.set.shown)
	}
}

func TestTextFieldBlinkWakesOnlyWhileFocused(t *testing.T) {
	f, _ := fieldWin(t)
	f.r.WakeupTime()
	f.frame()
	if at, ok := f.r.WakeupTime(); ok {
		t.Fatalf("the page wakes at %v with no field focused", at)
	}
	f.clickField("name")
	f.r.WakeupTime()
	f.frame()
	if _, ok := f.r.WakeupTime(); !ok {
		t.Fatal("a focused field does not blink its caret")
	}
	f.send(modPress(key.NameEscape, 0))
	f.r.WakeupTime()
	f.frame()
	if at, ok := f.r.WakeupTime(); ok {
		t.Fatalf("the page still wakes at %v after the field let go", at)
	}
}

func TestTextFieldKeepsTextAcrossThemesAndDropsWhenGone(t *testing.T) {
	f, d := fieldWin(t)
	f.clickField("dir")
	f.typeText("/x")
	f.win.ui.setTheme(theme.All()[2])
	f.frame()
	if got := f.win.fieldText("dir"); got != "~/code/x" {
		t.Fatalf("dir %q after a theme switch", got)
	}
	d.hideDir = true
	f.frame()
	if _, ok := f.win.set.fields["dir"]; ok {
		t.Fatal("the dir editor outlived its row")
	}
	if f.win.set.editing != "" {
		t.Fatalf("still editing %q after its row went away", f.win.set.editing)
	}
	f.frame()
	f.wantEditing("")
}

func TestTextFieldLetsGoUnderADialog(t *testing.T) {
	f, d := fieldWin(t)
	open := false
	f.win.set.sections[0].dialog = func(*window) *dialogBox {
		if !open {
			return nil
		}
		return &dialogBox{title: "Sure?", buttons: []ctrl{{kind: ctlButton, id: "go", text: "OK"}}}
	}
	f.clickField("name")
	r := controlsOf(f.win, "name")[0].rect
	open = true
	f.frame()
	f.frame()
	f.wantEditing("")
	// The field is under the dimmed page now; a click there must not reach it.
	p := center(r)
	f.r.Queue(
		pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: p},
		pointer.Event{Kind: pointer.Release, Source: pointer.Mouse, Position: p},
	)
	f.frame()
	if f.focusedOn(&f.win.set.fields["name"].ed) {
		t.Fatal("a click under the dialog gave the field the keyboard")
	}
	f.frame()
	f.send(key.EditEvent{Text: "q"})
	if f.win.set.editing != "" || d.name != "" {
		t.Fatalf("a click under the dialog reached the field: editing %q, value %q", f.win.set.editing, d.name)
	}
}

// A kaze shortcut in the same batch as typing must not cost the typed text.
func TestTextFieldEditSurvivesAShortcutInTheSameBatch(t *testing.T) {
	f, d := fieldWin(t)
	f.clickField("name")
	f.send(key.EditEvent{Text: "x"}, modPress(",", key.ModCtrl))
	if !f.win.set.shown {
		t.Fatal("Ctrl+, hid the settings page")
	}
	if d.name != "x" {
		t.Fatalf("draft %q after typing x then Ctrl+, in one batch, want x", d.name)
	}
}

func TestTextFieldCtrlCCopiesTheSelection(t *testing.T) {
	f, _ := fieldWin(t)
	f.clickField("dir")
	f.send(modPress("A", key.ModCtrl))
	f.send(modPress("C", key.ModCtrl))
	if _, b, ok := f.r.WriteClipboard(); !ok || string(b) != "~/code" {
		t.Fatalf("Ctrl+C copied %q %v, want the field's selection", b, ok)
	}
}
