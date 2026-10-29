package ui

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/claudesetup"
)

var doneToasts = []string{"3s", "6s", "10s", "until_seen"}

// maxDiffLines keeps the confirm dialog inside the default window; the pane does not scroll.
const maxDiffLines = 14

// claudeState is the Claude Code setup check, read when the section is shown
// and after a write, never per frame.
type claudeState struct {
	data    []byte // the settings as read; nil when the file does not exist
	report  claudesetup.Report
	readErr string // why the file could not be read
	dialog  *claudeDialog
}

// claudeDialog is a pending write the owner has to confirm.
type claudeDialog struct {
	title     string
	summary   []string
	diff      []string
	from, out []byte // the file it was computed from, and what Write puts there
}

func claudeSection() settingsSection {
	return settingsSection{
		title: "Claude", hint: "Alerts come from Claude Code hooks; usage comes from its status line.",
		rows: claudeRows, act: claudeAct, show: (*window).loadClaude, dialog: claudeDialogBox,
	}
}

func claudeRows(win *window) []settingRow {
	c := win.opt.Config
	rows := []settingRow{
		{label: "Chime when Claude needs you", ctrls: []ctrl{
			{kind: ctlButton, id: "chime_test", text: "Test"},
			{kind: ctlToggle, id: "chime_needs", on: c.ChimeNeeds},
		}},
		{label: "Chime when Claude is done", ctrls: []ctrl{{kind: ctlToggle, id: "chime_done", on: c.ChimeDone}}},
		{label: "Flash the taskbar", hint: "When kaze is in the background", ctrls: []ctrl{{kind: ctlToggle, id: "taskbar_flash", on: c.TaskbarFlash}}},
		{label: `"Done" toast stays for`, ctrls: []ctrl{{
			kind: ctlSegmented, id: "done_toast", options: []string{"3s", "6s", "10s", "Until seen"},
			selected: choiceIndex(doneToasts, c.DoneToast, 1),
		}}},
		{label: "Show usage in the status bar", ctrls: []ctrl{{kind: ctlToggle, id: "show_usage", on: c.ShowUsage}}},
	}
	return append(rows, win.setupRow())
}

// setupRow is the Claude Code setup check: one line per kaze entry, this kaze's
// path, and Fix / Replace when they have something to do.
func (win *window) setupRow() settingRow {
	row := settingRow{label: "Claude Code setup"}
	if win.exePath != "" {
		row.hint = "This kaze: " + truncateLeft(win.exePath, 80)
	}
	cs := &win.claude
	r := cs.report
	switch {
	case win.claudePath == "":
		row.lines = []string{"Claude Code settings not found"}
		return row
	case cs.readErr != "":
		row.lines = []string{"Cannot read " + tildeHome(win.claudePath, win.home) + ": " + truncateRight(cs.readErr, 60)}
		return row
	case r.Stop.State == claudesetup.Unreadable:
		row.lines = []string{tildeHome(win.claudePath, win.home) + " is not valid settings JSON"}
		return row
	}
	row.lines = []string{
		"Alerts when Claude needs you: " + entryText(r.Needs),
		"Alerts when Claude is done: " + entryText(r.Stop),
		"Usage in the status bar: " + entryText(r.StatusLine),
	}
	if win.exePath == "" {
		return row
	}
	fixable := func(e claudesetup.Entry) bool {
		return e.State == claudesetup.Missing || e.State == claudesetup.Broken
	}
	if fixable(r.Stop) || fixable(r.Needs) || fixable(r.StatusLine) {
		row.ctrls = append(row.ctrls, ctrl{kind: ctlButton, id: "claude_fix", text: "Fix", strong: true})
	}
	if r.StatusLine.State == claudesetup.Taken {
		row.ctrls = append(row.ctrls, ctrl{kind: ctlButton, id: "claude_replace", text: "Replace"})
	}
	return row
}

// entryText is an entry's state in plain words, with the end of its exe path.
func entryText(e claudesetup.Entry) string {
	switch e.State {
	case claudesetup.OK:
		return "OK · " + shortExe(e.Exe)
	case claudesetup.Broken:
		return "broken, file gone · " + shortExe(e.Exe)
	case claudesetup.Taken:
		return "taken by " + truncateRight(cleanText(e.Command), 40)
	}
	return "missing"
}

// shortExe is the exe's folder and name, enough to tell builds apart.
func shortExe(p string) string {
	p = cleanText(p)
	i := strings.LastIndexAny(p, `/\`)
	if i <= 0 {
		return p
	}
	if j := strings.LastIndexAny(p[:i], `/\`); j >= 0 {
		return p[j+1:]
	}
	return p
}

// exeExists is whether a kaze entry's exe is there: a path is checked as a
// file, a bare name is looked up on PATH as Claude Code's shell would.
func exeExists(p string) bool {
	if !strings.ContainsAny(p, `/\`) {
		_, err := exec.LookPath(p)
		return err == nil
	}
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

// readClaude reads the settings file; a missing file reads as empty settings,
// so Fix can create it.
func (win *window) readClaude() (data []byte, r claudesetup.Report, err error) {
	data, err = os.ReadFile(win.claudePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, claudesetup.Inspect([]byte("{}"), exeExists), nil
	}
	if err != nil {
		return nil, r, err
	}
	return data, claudesetup.Inspect(data, exeExists), nil
}

// loadClaude re-reads the settings and drops any dialog computed from older ones.
func (win *window) loadClaude() {
	cs := &win.claude
	*cs = claudeState{}
	if win.claudePath == "" {
		return
	}
	data, r, err := win.readClaude()
	if err != nil {
		cs.readErr = err.Error()
		return
	}
	cs.data, cs.report = data, r
}

func claudeAct(win *window, id string, arg int) {
	on := arg == 1
	switch id {
	case "chime_test":
		win.testChime()
	case "chime_needs", "chime_done":
		if on {
			if warn := win.ensureChime(); warn != "" {
				win.note(warn)
			}
		}
		key := id
		win.alertMu.Lock()
		if id == "chime_needs" {
			win.opt.Config.ChimeNeeds = on
		} else {
			win.opt.Config.ChimeDone = on
		}
		win.alertMu.Unlock()
		win.saveSetting(key, on)
	case "taskbar_flash":
		win.alertMu.Lock()
		win.opt.Config.TaskbarFlash = on
		win.alertMu.Unlock()
		win.saveSetting(id, on)
	case "done_toast":
		if arg >= 0 && arg < len(doneToasts) {
			win.opt.Config.DoneToast = doneToasts[arg]
			win.saveSetting(id, doneToasts[arg])
		}
	case "show_usage":
		win.opt.Config.ShowUsage = on
		win.saveSetting(id, on)
	case "claude_fix":
		win.openFix()
	case "claude_replace":
		win.openReplace()
	case "dialog_cancel":
		win.claude.dialog = nil
	case "dialog_write":
		win.writeClaude()
	}
}

// testChime plays the chime now, building the player if no chime was on yet.
func (win *window) testChime() {
	if warn := win.ensureChime(); warn != "" {
		win.note(warn)
	}
	win.alertMu.Lock()
	chime := win.chime
	win.alertMu.Unlock()
	if chime != nil {
		chime(time.Now())
	}
}

// note shows msg in the status bar for a while.
func (win *window) note(msg string) {
	win.notice = msg
	win.noticeUntil = time.Now().Add(noticeFor)
}

// fresh re-reads the settings for a dialog, so it shows the file as it is now.
func (win *window) fresh() ([]byte, claudesetup.Report, bool) {
	data, r, err := win.readClaude()
	if err != nil {
		win.note("Claude settings: " + err.Error())
		win.loadClaude()
		return nil, r, false
	}
	return data, r, true
}

func (win *window) openFix() {
	data, r, ok := win.fresh()
	if !ok {
		return
	}
	src := data
	if src == nil {
		src = []byte("{}")
	}
	out, summary, err := claudesetup.Fix(src, r, win.exePath)
	if err != nil {
		win.note("Claude settings: " + err.Error())
		win.loadClaude()
		return
	}
	if len(summary) == 0 {
		win.loadClaude() // fixed meanwhile
		return
	}
	win.claude.dialog = &claudeDialog{
		title: "Fix the Claude Code setup", summary: summary,
		diff: claudesetup.Diff(src, out, maxDiffLines), from: data, out: out,
	}
}

func (win *window) openReplace() {
	data, r, ok := win.fresh()
	if !ok || r.StatusLine.State != claudesetup.Taken {
		win.loadClaude()
		return
	}
	out, removed := claudesetup.Replace(data, win.exePath)
	if out == nil {
		win.loadClaude()
		return
	}
	win.claude.dialog = &claudeDialog{
		title: "Replace the status line",
		summary: []string{
			"Removes: " + cleanText(removed),
			`Adds: "` + win.exePath + `" statusline`,
		},
		diff: claudesetup.Diff(data, out, maxDiffLines), from: data, out: out,
	}
}

// writeClaude writes the confirmed change, unless the file changed since the
// dialog was opened; either way the check is read again.
func (win *window) writeClaude() {
	d := win.claude.dialog
	if d == nil {
		return
	}
	defer win.loadClaude()
	cur, err := os.ReadFile(win.claudePath)
	if errors.Is(err, os.ErrNotExist) {
		cur, err = nil, nil
	}
	switch {
	case err != nil:
		win.note("Claude settings: " + err.Error())
	case !bytes.Equal(cur, d.from) || (cur == nil) != (d.from == nil):
		win.note("Claude settings changed meanwhile; nothing written, check again")
	default:
		if err := claudesetup.Write(win.claudePath, d.out, time.Now()); err != nil {
			win.note("Claude settings not written: " + err.Error())
		} else {
			win.note("Claude settings updated; the old file is kept as a .bak copy")
		}
	}
}

// claudeDialogBox is the open confirm dialog, if any.
func claudeDialogBox(win *window) *dialogBox {
	d := win.claude.dialog
	if d == nil {
		return nil
	}
	return &dialogBox{
		title: d.title, lines: d.summary, code: d.diff,
		buttons: []ctrl{
			{kind: ctlButton, id: "dialog_cancel", text: "Cancel"},
			{kind: ctlButton, id: "dialog_write", text: "Write", strong: true},
		},
	}
}
