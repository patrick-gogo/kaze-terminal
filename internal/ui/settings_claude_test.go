package ui

import (
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/claudesetup"
	"github.com/patrick-gogo/kaze-terminal/internal/config"
	"github.com/patrick-gogo/kaze-terminal/internal/notify"
)

// claudeFixture is a temp Claude settings file with every kaze entry pointing
// at a real kaze-app\kaze.exe, minus the ones named in drop.
type claudeFixture struct {
	dir, settings, exe, cfg string
}

func newClaudeFixture(t *testing.T, statusCmd string, dropStop bool) claudeFixture {
	t.Helper()
	dir := t.TempDir()
	app := filepath.Join(dir, "kaze-app")
	if err := os.Mkdir(app, 0o755); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(app, "kaze.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	q := `"` + exe + `" `
	node := map[string]any{"type": "command", "command": `"C:\Program Files\nodejs\node.exe" "C:\x\hook.js"`}
	stop := []any{map[string]any{"hooks": []any{node}}}
	if !dropStop {
		stop = append(stop, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": q + "notify done", "timeout": 3}}})
	}
	m := map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash(git status:*)"}},
		"hooks": map[string]any{
			"SessionStart": []any{map[string]any{"hooks": []any{node}}},
			"Stop":         stop,
			"Notification": []any{map[string]any{"matcher": claudesetup.NeedsMatcher, "hooks": []any{map[string]any{"type": "command", "command": q + "notify needs", "timeout": 3}}}},
		},
		"statusLine": map[string]any{"type": "command", "command": statusCmd},
	}
	if statusCmd == "" {
		m["statusLine"].(map[string]any)["command"] = q + "statusline"
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f := claudeFixture{dir: dir, settings: filepath.Join(dir, "settings.json"), exe: exe, cfg: filepath.Join(dir, "config.toml")}
	if err := os.WriteFile(f.settings, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// claudeWindow is a window with default settings and the Claude section shown,
// reading f's settings file and saving to f's config.
func claudeWindow(t *testing.T, f claudeFixture, w, h int) *window {
	t.Helper()
	win, _ := alertWindow(t, 2)
	win.opt.Config = config.Default()
	win.cfgPath, win.claudePath, win.exePath = f.cfg, f.settings, f.exe
	win.openSettings()
	win.selectSection(claudeIndex(t))
	win.drawSettings(pageContext(w, h), image.Rect(0, 38, w, h-24))
	return win
}

func claudeIndex(t *testing.T) int {
	t.Helper()
	i := slices.IndexFunc(defaultSections(), func(s settingsSection) bool { return s.title == "Claude" })
	if i < 0 {
		t.Fatal("no Claude section")
	}
	return i
}

// pressAny clicks the first control with id, then lays out the page again.
func pressAny(t *testing.T, win *window, id string, arg int) {
	t.Helper()
	for _, c := range controlsOf(win, id) {
		if c.arg == arg {
			win.settingsPointer(pressAt(center(c.rect)))
			win.drawSettings(pageContext(800, 600), pageRect)
			return
		}
	}
	t.Fatalf("no %s control with arg %d", id, arg)
}

func pageHas(win *window, s string) bool {
	return slices.ContainsFunc(win.set.texts, func(x string) bool { return strings.Contains(x, s) })
}

func TestDoneToastSetting(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	other := win.mgr.Tabs()[1].ID
	pressAny(t, win, "done_toast", 0)
	if win.opt.Config.DoneToast != "3s" || !strings.Contains(readFile(t, f.cfg), `done_toast = "3s"`) {
		t.Fatalf("done_toast %q, file %q", win.opt.Config.DoneToast, readFile(t, f.cfg))
	}
	win.deliver(notify.Msg{Tab: int(other), Kind: notify.KindDone}, t0)
	if got := win.alerts.list()[0].Until; !got.Equal(t0.Add(3 * time.Second)) {
		t.Fatalf("3s: until %v", got)
	}
	pressAny(t, win, "done_toast", 3)
	if win.opt.Config.DoneToast != "until_seen" {
		t.Fatalf("done_toast %q", win.opt.Config.DoneToast)
	}
	win.deliver(notify.Msg{Tab: int(other), Kind: notify.KindDone}, t0)
	if got := win.alerts.list()[0].Until; !got.IsZero() {
		t.Fatalf("until_seen: until %v, want zero", got)
	}
}

func TestFlashOffSuppressesFlash(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	played := []time.Time{}
	win.chime = func(at time.Time) { played = append(played, at) }
	flashes := 0
	win.flash = func() { flashes++ }
	win.liveMgr.Store(win.mgr)
	win.publish(win.mgr.Active(), false)
	pressAny(t, win, "taskbar_flash", 0)
	if win.opt.Config.TaskbarFlash || !strings.Contains(readFile(t, f.cfg), "taskbar_flash = false") {
		t.Fatal("taskbar_flash not turned off and saved")
	}
	win.onNotify(notify.Msg{Tab: int(win.mgr.Tabs()[1].ID), Kind: notify.KindNeeds})
	if flashes != 0 || len(played) != 1 {
		t.Fatalf("flashes %d chimes %d, want 0 and 1", flashes, len(played))
	}
}

func TestShowUsageOff(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	win.liveMgr.Store(win.mgr)
	win.onNotify(usageMsg(int(win.mgr.Tabs()[0].ID), 10, 30))
	win.drainUsage()
	if len(win.statusParts().Usage) == 0 {
		t.Fatal("no usage with show_usage on")
	}
	pressAny(t, win, "show_usage", 0)
	if u := win.statusParts().Usage; len(u) != 0 || !strings.Contains(readFile(t, f.cfg), "show_usage = false") {
		t.Fatalf("usage %v with show_usage off", u)
	}
}

func TestChimeTestButtonPlays(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	win.opt.Config.ChimeNeeds, win.opt.Config.ChimeDone = false, false
	win.chime = nil // both chimes were off at startup
	played, built := 0, 0
	old := newChime
	newChime = func(string) (func(time.Time), string) {
		built++
		return func(time.Time) { played++ }, ""
	}
	t.Cleanup(func() { newChime = old })
	pressAny(t, win, "chime_test", 0)
	pressAny(t, win, "chime_test", 0)
	if built != 1 || played != 2 {
		t.Fatalf("built %d played %d, want 1 and 2", built, played)
	}
}

func TestChimeTurnedOnBuildsThePlayer(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	win.opt.Config.ChimeNeeds = false
	win.chime = nil
	old := newChime
	played := 0
	newChime = func(string) (func(time.Time), string) { return func(time.Time) { played++ }, "" }
	t.Cleanup(func() { newChime = old })
	win.drawSettings(pageContext(800, 600), pageRect)
	pressAny(t, win, "chime_needs", 1)
	if !win.opt.Config.ChimeNeeds || win.chime == nil || !strings.Contains(readFile(t, f.cfg), "chime_needs = true") {
		t.Fatal("chime_needs on did not build the player and save")
	}
	win.liveMgr.Store(win.mgr)
	win.publish(win.mgr.Active(), true)
	win.onNotify(notify.Msg{Tab: int(win.mgr.Tabs()[1].ID), Kind: notify.KindNeeds})
	if played != 1 {
		t.Fatalf("played %d after turning the chime on", played)
	}
}

func TestSetupLinesShowState(t *testing.T) {
	f := newClaudeFixture(t, "bash x.sh", true)
	win := claudeWindow(t, f, 800, 600)
	short := filepath.Join("kaze-app", "kaze.exe")
	for _, want := range []string{
		"Alerts when Claude needs you: OK · " + short,
		"Alerts when Claude is done: missing",
		"Usage in the status bar: taken by bash x.sh",
		"This kaze: ",
	} {
		if !pageHas(win, want) {
			t.Errorf("no %q in %q", want, win.set.texts)
		}
	}
	if len(controlsOf(win, "claude_fix")) != 1 || len(controlsOf(win, "claude_replace")) != 1 {
		t.Fatal("want Fix and Replace buttons")
	}
}

func TestAllOKHasNoButtons(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	if len(controlsOf(win, "claude_fix"))+len(controlsOf(win, "claude_replace")) != 0 {
		t.Fatal("Fix or Replace shown with everything OK")
	}
}

func TestFixDialogNeedsConfirm(t *testing.T) {
	f := newClaudeFixture(t, "", true)
	before := readFile(t, f.settings)
	win := claudeWindow(t, f, 800, 600)
	pressAny(t, win, "claude_fix", 0)
	if win.claude.dialog == nil || len(controlsOf(win, "dialog_write")) != 1 {
		t.Fatal("Fix did not open the dialog")
	}
	if len(controlsOf(win, "claude_fix")) != 0 || len(win.set.nav) != 0 {
		t.Fatal("the page under the dialog is still clickable")
	}
	if !pageHas(win, "notify done") {
		t.Fatalf("dialog does not show the change: %q", win.set.texts)
	}
	if readFile(t, f.settings) != before {
		t.Fatal("Fix wrote before Write was pressed")
	}
	pressAny(t, win, "dialog_write", 0)
	if win.claude.dialog != nil {
		t.Fatal("dialog still open after Write")
	}
	if readFile(t, f.settings) == before {
		t.Fatal("Write did not change the file")
	}
	baks, _ := filepath.Glob(f.settings + ".bak-*")
	if len(baks) != 1 || readFile(t, baks[0]) != before {
		t.Fatalf("backups %v", baks)
	}
	if win.claude.report.Stop.State != claudesetup.OK {
		t.Fatalf("report not re-read after the write: %+v", win.claude.report.Stop)
	}
}

func TestDialogCancelWritesNothing(t *testing.T) {
	f := newClaudeFixture(t, "", true)
	before := readFile(t, f.settings)
	win := claudeWindow(t, f, 800, 600)
	pressAny(t, win, "claude_fix", 0)
	pressAny(t, win, "dialog_cancel", 0)
	if win.claude.dialog != nil || readFile(t, f.settings) != before {
		t.Fatal("Cancel left the dialog or wrote")
	}
	if baks, _ := filepath.Glob(f.settings + ".bak-*"); len(baks) != 0 {
		t.Fatalf("backups %v", baks)
	}
}

func TestWriteRefusesAChangedFile(t *testing.T) {
	f := newClaudeFixture(t, "", true)
	win := claudeWindow(t, f, 800, 600)
	pressAny(t, win, "claude_fix", 0)
	if err := os.WriteFile(f.settings, []byte(`{"edited": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	pressAny(t, win, "dialog_write", 0)
	if readFile(t, f.settings) != `{"edited": true}` || win.notice == "" {
		t.Fatalf("wrote over a file changed after the dialog opened; notice %q", win.notice)
	}
}

func TestReplaceDialogNamesTheRemovedCommand(t *testing.T) {
	f := newClaudeFixture(t, "bash x.sh", false)
	win := claudeWindow(t, f, 800, 600)
	pressAny(t, win, "claude_replace", 0)
	if win.claude.dialog == nil || !pageHas(win, "bash x.sh") {
		t.Fatalf("Replace dialog does not name the removed command: %q", win.set.texts)
	}
	pressAny(t, win, "dialog_write", 0)
	if win.claude.report.StatusLine.State != claudesetup.OK {
		t.Fatalf("status line %+v after Replace", win.claude.report.StatusLine)
	}
}

func TestSettingsFileReadOnShowNotPerFrame(t *testing.T) {
	f := newClaudeFixture(t, "", false)
	win := claudeWindow(t, f, 800, 600)
	if err := os.WriteFile(f.settings, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	win.drawSettings(pageContext(800, 600), pageRect)
	if win.claude.report.Stop.State != claudesetup.OK {
		t.Fatal("the file was read again on a frame")
	}
	win.hideSettings()
	win.openSettings()
	if win.claude.report.Stop.State != claudesetup.Missing {
		t.Fatal("the file was not read again when the section was shown")
	}
}

func TestNoClaudePathReadsNothing(t *testing.T) {
	win, _ := alertWindow(t, 1)
	win.opt.Config = config.Default()
	win.openSettings()
	win.selectSection(claudeIndex(t))
	win.drawSettings(pageContext(800, 600), pageRect)
	if len(controlsOf(win, "claude_fix")) != 0 || !pageHas(win, "not found") {
		t.Fatalf("texts %q", win.set.texts)
	}
}

// The pane does not scroll: at the default window size every row, and the
// dialog with a long change, must fit above the status bar.
func TestClaudeSectionFitsDefaultWindow(t *testing.T) {
	f := newClaudeFixture(t, "bash x.sh", true)
	if err := os.WriteFile(f.settings, []byte(`{"hooks": {}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	const w, h = 1100, 720
	win := claudeWindow(t, f, w, h)
	bottom := h - 24
	for _, c := range win.set.controls {
		if c.rect.Max.Y > bottom || c.rect.Max.X > w {
			t.Fatalf("control %s at %v is off the page", c.id, c.rect)
		}
	}
	if len(controlsOf(win, "claude_fix")) != 1 {
		t.Fatal("no Fix button")
	}
	for _, c := range controlsOf(win, "claude_fix") {
		win.settingsPointer(pressAt(center(c.rect)))
	}
	win.drawSettings(pageContext(w, h), image.Rect(0, 38, w, bottom))
	if win.claude.dialog == nil {
		t.Fatal("no dialog")
	}
	for _, c := range win.set.controls {
		if c.rect.Max.Y > bottom || c.rect.Min.Y < 38 {
			t.Fatalf("dialog button %s at %v is off the page", c.id, c.rect)
		}
	}
}

func unreadableWindow(t *testing.T, write func(path string)) (*window, string) {
	t.Helper()
	f := newClaudeFixture(t, "", true)
	if err := os.Remove(f.settings); err != nil {
		t.Fatal(err)
	}
	if write != nil {
		write(f.settings)
	}
	return claudeWindow(t, f, 800, 600), f.settings
}

func TestMissingSettingsFileCannotBeRead(t *testing.T) {
	win, path := unreadableWindow(t, nil)
	if !pageHas(win, "Can't read ") || !pageHas(win, "settings.json (not found)") {
		t.Fatalf("texts %q", win.set.texts)
	}
	if len(controlsOf(win, "claude_fix"))+len(controlsOf(win, "claude_replace")) != 0 {
		t.Fatal("Fix or Replace offered for a missing file")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("settings file created: %v", err)
	}
}

func TestEmptyAndInvalidSettingsCannotBeRead(t *testing.T) {
	for body, want := range map[string]string{"": "(empty)", " \t ": "(empty)", "{nope": "(not valid JSON)"} {
		win, _ := unreadableWindow(t, func(p string) {
			if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		})
		if !pageHas(win, "Can't read ") || !pageHas(win, want) {
			t.Errorf("%q: texts %q", body, win.set.texts)
		}
		if len(controlsOf(win, "claude_fix"))+len(controlsOf(win, "claude_replace")) != 0 {
			t.Errorf("%q: Fix or Replace offered", body)
		}
	}
}

const relayoutLine = "The file's layout will also be rewritten (values unchanged)."

func TestFixDialogFlagsALayoutRewrite(t *testing.T) {
	f := newClaudeFixture(t, "", true)
	var v any
	if err := json.Unmarshal([]byte(readFile(t, f.settings)), &v); err != nil {
		t.Fatal(err)
	}
	four, _ := json.MarshalIndent(v, "", "    ")
	if err := os.WriteFile(f.settings, four, 0o644); err != nil {
		t.Fatal(err)
	}
	win := claudeWindow(t, f, 800, 600)
	pressAny(t, win, "claude_fix", 0)
	if win.claude.dialog == nil || !slices.Contains(win.claude.dialog.summary, relayoutLine) {
		t.Fatalf("4-space file: dialog %+v", win.claude.dialog)
	}
}

func TestFixDialogQuietForClaudeLayout(t *testing.T) {
	f := newClaudeFixture(t, "", true) // json.MarshalIndent with 2 spaces: Claude Code's layout
	win := claudeWindow(t, f, 800, 600)
	pressAny(t, win, "claude_fix", 0)
	if win.claude.dialog == nil || slices.Contains(win.claude.dialog.summary, relayoutLine) {
		t.Fatalf("2-space file: dialog %+v", win.claude.dialog)
	}
}
