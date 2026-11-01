package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/patrick-gogo/kaze-terminal/internal/claudesetup"
	"github.com/patrick-gogo/kaze-terminal/internal/codexsetup"
)

type codexState struct {
	data, config     []byte
	report           codexsetup.Report
	readErr          string
	installed, trust bool
	dialog           *codexDialog
}
type codexDialog struct {
	claudeDialog
	config []byte
}

func (win *window) loadAgents() { win.loadClaude(); win.loadCodex() }
func readCodexFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const limit = 1 << 20
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > limit {
		return nil, fmt.Errorf("%s is larger than 1 MiB", filepath.Base(path))
	}
	return b, nil
}
func (win *window) codexSources() ([]byte, []byte, error) {
	a, err := readCodexFile(filepath.Join(win.codexDir, "hooks.json"))
	if err != nil {
		return nil, nil, err
	}
	b, err := readCodexFile(filepath.Join(win.codexDir, "config.toml"))
	return a, b, err
}
func (win *window) loadCodex() {
	trust := win.codex.trust
	win.codex = codexState{trust: trust}
	if win.codexDir == "" {
		return
	}
	st, err := os.Stat(win.codexDir)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		win.codex.readErr = err.Error()
		return
	}
	if !st.IsDir() {
		win.codex.readErr = "Codex settings path is not a directory"
		return
	}
	win.codex.installed = true
	a, b, err := win.codexSources()
	if err != nil {
		win.codex.readErr = err.Error()
		return
	}
	r, err := codexsetup.Inspect(a, b, exeExists)
	if err != nil {
		win.codex.readErr = err.Error()
		return
	}
	win.codex.data, win.codex.config, win.codex.report = a, b, r
}
func (win *window) codexRows() []settingRow {
	cs := &win.codex
	if cs.readErr != "" {
		return []settingRow{{icon: "alert", label: "Can't read Codex hooks", hint: cleanText(cs.readErr)}}
	}
	if !cs.installed {
		return []settingRow{{icon: "alert", label: "Codex not found", hint: "Install Codex, then open this page again."}}
	}
	labels := []string{"Alerts when Codex needs you", "Alerts when Codex is done", "Command each session runs", "Interrupts",
		"Helpers start", "Helper reports", "Helper steps"}
	hints := []string{"PermissionRequest hook", "Stop hook", "UserPromptSubmit hook", "Interrupt hook",
		"SubagentStart hook", "SubagentStop hook", "PreToolUse hook"}
	icons := []string{"alert", "check", "prompt", "prompt", "claude", "check", "prompt"}
	var rows []settingRow
	offered := false
	for i, e := range cs.report.Entries() {
		state, tone := "Missing", "warn"
		switch e.State {
		case codexsetup.OK:
			state, tone = "Configured", "ok"
		case codexsetup.Broken:
			state, tone = "Broken", "err"
		case codexsetup.Taken:
			state = "Other hooks"
		}
		hint := hints[i] + " · " + e.Source
		if e.State == codexsetup.OK {
			hint += " · " + shortExe(e.Exe)
		}
		if e.Detail != "" {
			hint += " · " + e.Detail
		}
		row := settingRow{icon: icons[i], label: labels[i], hint: hint, pill: state, tone: tone, monoHint: true}
		if !offered && win.exePath != "" && e.Fixable && e.State != codexsetup.OK {
			row.ctrls = []ctrl{{kind: ctlButton, id: "codex_fix", text: "Fix", strong: true}}
			offered = true
		}
		rows = append(rows, row)
	}
	if cs.trust {
		rows = append(rows, settingRow{icon: "alert", label: "Trust Kaze's hooks in Codex", hint: "Now open Codex and run /hooks to review and trust Kaze's hooks.", ctrls: []ctrl{{kind: ctlButton, id: "codex_trust_dismiss", text: "Dismiss", ghost: true}}})
	} else {
		rows = append(rows, settingRow{hint: "Codex runs these hooks after you review and trust them with /hooks."})
	}
	return rows
}
func agentsAct(win *window, id string, arg int) {
	switch id {
	case "codex_fix":
		win.openCodexFix()
		return
	case "codex_trust_dismiss":
		win.codex.trust = false
		return
	case "dialog_cancel":
		if win.codex.dialog != nil {
			win.codex.dialog = nil
			return
		}
	case "dialog_write":
		if win.codex.dialog != nil {
			win.writeCodex()
			return
		}
	}
	claudeAct(win, id, arg)
}
func agentsDialogBox(win *window) *dialogBox {
	if d := win.codex.dialog; d != nil {
		return &dialogBox{title: d.title, lines: d.summary, code: d.diff, buttons: []ctrl{{kind: ctlButton, id: "dialog_cancel", text: "Cancel"}, {kind: ctlButton, id: "dialog_write", text: "Write", strong: true}}}
	}
	return claudeDialogBox(win)
}
func (win *window) openCodexFix() {
	if win.exePath == "" {
		return
	}
	win.loadCodex()
	if !win.codex.installed || win.codex.readErr != "" {
		return
	}
	a, b := win.codex.data, win.codex.config
	out, summary, err := codexsetup.Fix(a, b, win.exePath, exeExists)
	if err != nil {
		win.note("Codex hooks: " + err.Error())
		return
	}
	if len(summary) == 0 {
		return
	}
	win.claude.dialog = nil
	before := a
	if before == nil {
		before = []byte("{}")
	} // preview a new file as an empty object
	summary = append(summary, "Only hooks.json is written; an existing file is backed up first.", "Then run /hooks in Codex to review and trust these hooks.")
	win.codex.dialog = &codexDialog{claudeDialog: claudeDialog{title: "Fix the Codex setup", summary: withRelayout(summary, before), diff: claudesetup.Diff(before, out, maxDiffLines), from: a, out: out}, config: b}
}
func sameCodexSource(a, b []byte) bool { return (a == nil) == (b == nil) && bytes.Equal(a, b) }
func (win *window) writeCodex() {
	d := win.codex.dialog
	if d == nil {
		return
	}
	defer win.loadCodex()
	a, b, err := win.codexSources()
	switch {
	case err != nil:
		win.note("Codex hooks: " + err.Error())
	case !sameCodexSource(a, d.from) || !sameCodexSource(b, d.config):
		win.note("Codex hooks changed meanwhile; nothing written, check again")
	default:
		if err := claudesetup.Write(filepath.Join(win.codexDir, "hooks.json"), d.out, time.Now()); err != nil {
			win.note("Codex hooks not written: " + err.Error())
		} else {
			win.codex.trust = true
			win.note("Codex hooks updated. Run /hooks in Codex to trust them.")
		}
	}
}

// agentsRows keeps the alert configuration shared while setup is per agent.
func agentsRows(win *window) []settingRow {
	rows := sharedAgentRows(win)
	rows = append(rows, settingRow{heading: "Claude Code setup"})
	rows = append(rows, win.setupRows()...)
	rows = append(rows, settingRow{heading: "Codex setup"})
	return append(rows, win.codexRows()...)
}
