package ui

import (
	"image"
	"runtime/debug"
	"slices"
	"strings"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
	kinput "github.com/patrick-gogo/kaze-terminal/internal/input"
	"github.com/patrick-gogo/kaze-terminal/internal/theme"
	"github.com/patrick-gogo/kaze-terminal/internal/version"
)

// pageContext is a 1x-scale context for a whole window.
func pageContext(w, h int) layout.Context {
	return layout.Context{
		Ops:         new(op.Ops),
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Exact(image.Pt(w, h)),
	}
}

// pageRect is a page box like the one frame gives an 800x600 window with the sidebar off.
var pageRect = image.Rect(0, 38, 800, 576)

type activation struct {
	id  string
	arg int
}

// kitSection is one row of every control kind, recording what it is asked to do.
func kitSection(got *[]activation, on *bool) settingsSection {
	return settingsSection{
		title: "Kit",
		rows: func(*window) []settingRow {
			return []settingRow{
				{label: "Theme", block: true, ctrls: []ctrl{
					{kind: ctlSwatch, id: "theme", theme: theme.All()[0], current: true, arg: 0},
					{kind: ctlSwatch, id: "theme", theme: theme.All()[2], arg: 2},
				}},
				{label: "Flag", hint: "a dim hint", ctrls: []ctrl{{kind: ctlToggle, id: "flag", on: *on}}},
				{label: "Pick", ctrls: []ctrl{{kind: ctlSegmented, id: "pick", options: []string{"A", "B", "C"}, selected: 1}}},
				{label: "Size", ctrls: []ctrl{{kind: ctlStepper, id: "size", min: 8, max: 32, value: 32}}},
				{label: "Go", ctrls: []ctrl{{kind: ctlButton, id: "go", text: "Test"}, {kind: ctlButton, id: "fix", text: "Fix", strong: true}}},
			}
		},
		act: func(_ *window, id string, arg int) {
			*got = append(*got, activation{id, arg})
			if id == "flag" {
				*on = arg == 1
			}
		},
	}
}

// kitWindow is a window with settings open on the kit section, laid out once.
func kitWindow(t *testing.T) (*window, *[]activation, *bool) {
	t.Helper()
	win, _ := alertWindow(t, 2)
	got, on := &[]activation{}, new(bool)
	win.set.sections = []settingsSection{kitSection(got, on)}
	win.openSettings()
	win.drawSettings(pageContext(800, 600), pageRect)
	return win, got, on
}

func controlsOf(win *window, id string) []control {
	var out []control
	for _, c := range win.set.controls {
		if c.id == id {
			out = append(out, c)
		}
	}
	return out
}

func center(r image.Rectangle) f32.Point {
	return f32.Pt(float32(r.Min.X+r.Dx()/2), float32(r.Min.Y+r.Dy()/2))
}

func pressAt(p f32.Point) pointer.Event {
	return pointer.Event{Kind: pointer.Press, Buttons: pointer.ButtonPrimary, Position: p}
}

func TestCtrlCommaOpensSettings(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	win.route(layout.Context{}, []event.Event{key.Event{Name: ",", Modifiers: key.ModCtrl, State: key.Press}}, 8, 16)
	if !win.set.open || !win.set.shown {
		t.Fatalf("Ctrl+, left settings open=%v shown=%v", win.set.open, win.set.shown)
	}
	if got := st.ptys[0].sent(); got != "" {
		t.Fatalf("the shell got %q for Ctrl+,", got)
	}
	mac, _ := buildKeymap("darwin", nil)
	if mac[kinput.Shortcut{Name: ",", Mods: key.ModCommand}] != actOpenSettings {
		t.Fatal("Cmd+, does not open settings on macOS")
	}
	km, warns := buildKeymap("windows", map[string]string{"open_settings": "ctrl+shift+p"})
	if len(warns) != 0 || km[kinput.Shortcut{Name: "P", Mods: key.ModCtrl | key.ModShift}] != actOpenSettings {
		t.Fatalf("open_settings is not overridable in [keys]: %v", warns)
	}
}

func TestGearOpensSettings(t *testing.T) {
	win, got := captionWindow(t, 1000)
	if win.bar.Gear.Empty() {
		t.Fatal("no gear in the custom title bar")
	}
	win.captionEvent(pointer.Event{Kind: pointer.Move, Position: center(win.bar.Gear)})
	if win.caption.hover != capGear {
		t.Fatalf("hover %v over the gear", win.caption.hover)
	}
	click(win, win.bar.Gear)
	if !win.set.shown || len(*got) != 0 {
		t.Fatalf("gear click: shown %v, window actions %v", win.set.shown, *got)
	}
}

func TestClickTogglesControl(t *testing.T) {
	win, _, on := kitWindow(t)
	for want := range []bool{true, false} {
		tg := controlsOf(win, "flag")
		if len(tg) != 1 {
			t.Fatalf("%d toggle hit areas, want 1", len(tg))
		}
		win.settingsPointer(pressAt(center(tg[0].rect)))
		if *on != (want == 0) {
			t.Fatalf("click %d: toggle %v", want, *on)
		}
		win.drawSettings(pageContext(800, 600), pageRect)
	}
}

func TestEveryControlKindActivates(t *testing.T) {
	win, got, _ := kitWindow(t)
	var want []activation
	for _, c := range win.set.controls {
		if !c.rect.In(pageRect) || c.rect.Empty() {
			t.Fatalf("%s at %v is outside the page", c.id, c.rect)
		}
		win.settingsPointer(pressAt(center(c.rect)))
		if !c.dead {
			want = append(want, activation{c.id, c.arg})
		}
	}
	wantIDs := []activation{
		{"theme", 0}, {"theme", 2}, {"flag", 1}, {"pick", 0}, {"pick", 1}, {"pick", 2},
		{"size", 31}, {"go", 0}, {"fix", 0},
	}
	if !slices.Equal(*got, wantIDs) || !slices.Equal(want, wantIDs) {
		t.Fatalf("activations %v, want %v (the stepper's + at its max does nothing)", *got, wantIDs)
	}
}

func TestTabMovesFocusAndEnterActivates(t *testing.T) {
	win, got, on := kitWindow(t)
	n := len(win.set.controls)
	tab := key.Event{Name: key.NameTab, State: key.Press}
	back := key.Event{Name: key.NameTab, Modifiers: key.ModShift, State: key.Press}
	win.settingsKey(layout.Context{}, tab)
	win.settingsKey(layout.Context{}, tab)
	win.settingsKey(layout.Context{}, tab)
	if win.set.focus != 2 {
		t.Fatalf("focus %d after three Tabs, want 2", win.set.focus)
	}
	win.settingsKey(layout.Context{}, key.Event{Name: key.NameSpace, State: key.Press})
	if !*on || len(*got) != 1 || (*got)[0].id != "flag" {
		t.Fatalf("Space on the toggle: %v", *got)
	}
	win.settingsKey(layout.Context{}, back)
	win.settingsKey(layout.Context{}, back)
	win.settingsKey(layout.Context{}, back)
	// The sidebar now participates in the focus order.
	for range win.set.nav {
		win.settingsKey(layout.Context{}, back)
	}
	if win.set.focus != n-1 {
		t.Fatalf("focus %d after Shift+Tab past the first, want the last (%d)", win.set.focus, n-1)
	}
	win.settingsKey(layout.Context{}, key.Event{Name: key.NameReturn, State: key.Press})
	if last := (*got)[len(*got)-1]; last.id != "fix" {
		t.Fatalf("Enter activated %v, want fix", last)
	}
}

// frameWith runs one headless frame fed by r.
func frameWith(t *testing.T, win *window, r *input.Router, at time.Time) {
	t.Helper()
	gtx := pageContext(800, 600)
	gtx.Source = r.Source()
	gtx.Now = at
	if err := win.frame(gtx); err != nil {
		t.Fatal(err)
	}
	r.Frame(gtx.Ops)
}

func TestTerminalGetsNoKeysWhileSettingsShown(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	win.focused, win.wantFocus = true, true
	win.opt.Config.FontSize = 14
	if _, err := win.mgr.New(); err != nil {
		t.Fatal(err)
	}
	var r input.Router
	at := time.Unix(1000, 0)
	typeA := func() {
		frameWith(t, win, &r, at)
		r.Queue(key.Event{Name: "A", State: key.Press}, key.EditEvent{Text: "a"}, key.Event{Name: "A", State: key.Release})
		frameWith(t, win, &r, at)
	}

	win.openSettings()
	typeA()
	if win.winTitle != "kaze · Settings" {
		t.Fatalf("title %q while settings shown", win.winTitle)
	}
	time.Sleep(50 * time.Millisecond)
	if got := st.ptys[0].sent(); got != "" {
		t.Fatalf("the shell got %q while settings was shown", got)
	}

	// The same keys reach the shell once the terminal is back, so the harness delivers keys.
	win.closeSettings()
	typeA()
	waitSent(t, st.ptys[0], "a")
}

func TestBlinkStopsWhileSettingsShown(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.opt.Config.CursorBlink = true
	t0 := time.Unix(1000, 0)
	if _, ok := win.blink(t0); !ok {
		t.Fatal("no blink with the terminal shown")
	}
	win.openSettings()
	if _, ok := win.blink(t0); ok {
		t.Fatal("the cursor blink kept waking the window under settings")
	}
}

// The sidebar footer shows the version only; the build is in Copy build info.
func TestVersionBlockShowsVersionWithoutBuild(t *testing.T) {
	old := readBuild
	t.Cleanup(func() { readBuild = old })
	readBuild = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567"},
			{Key: "vcs.time", Value: "2026-10-29T01:19:00Z"},
		}}, true
	}
	win, _ := alertWindow(t, 1)
	win.openSettings()
	win.drawSettings(pageContext(800, 600), pageRect)
	texts := win.set.texts
	if !slices.Contains(texts, "kaze "+version.Number) {
		t.Fatalf("texts %q lack the version", texts)
	}
	all := strings.Join(texts, " ")
	if strings.Contains(all, "abcdef1") {
		t.Fatalf("texts %q still show the build line", texts)
	}
	for _, want := range []string{"Appearance", "Workspaces", "Agents", "Shortcuts", "Updates", "About"} {
		if !strings.Contains(all, want) {
			t.Fatalf("nav lacks %q: %q", want, texts)
		}
	}
	if len(win.set.nav) != 6 {
		t.Fatalf("%d selectable sections, want 6", len(win.set.nav))
	}
	win.settingsPointer(pressAt(center(win.set.nav[2].rect)))
	if win.set.section != 2 {
		t.Fatalf("section %d after clicking Claude, want 2", win.set.section)
	}
}

func TestKeyboardCanReachClaudeSection(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.opt.Config = config.Default()
	win.openSettings()
	win.drawSettings(pageContext(800, 600), pageRect)
	// Tab past the Appearance controls, then Appearance, Workspaces and Claude in the sidebar.
	for i := 0; i < len(win.set.controls)+3; i++ {
		win.settingsKey(layout.Context{}, key.Event{Name: key.NameTab, State: key.Press})
	}
	win.settingsKey(layout.Context{}, key.Event{Name: key.NameReturn, State: key.Press})
	if win.sections()[win.set.section].title != "Agents" {
		t.Fatalf("keyboard left section at %q", win.sections()[win.set.section].title)
	}
	// Shift+Tab twice, past Workspaces, reaches Appearance.
	win.drawSettings(pageContext(800, 600), pageRect)
	for range 2 {
		win.settingsKey(layout.Context{}, key.Event{Name: key.NameTab, Modifiers: key.ModShift, State: key.Press})
	}
	win.settingsKey(layout.Context{}, key.Event{Name: key.NameSpace, State: key.Press})
	if win.sections()[win.set.section].title != "Appearance" {
		t.Fatal("could not return to Appearance with keyboard")
	}
}
