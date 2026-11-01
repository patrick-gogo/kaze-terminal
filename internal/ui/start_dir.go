package ui

import (
	"fmt"
	"path/filepath"
	"slices"

	"github.com/patrick-gogo/kaze-terminal/internal/config"
)

// plainStartDir resolves only when opening a tab, never during layout. Explicit
// workspace/restored directories bypass this fallback. Existing tabs retain Dir.
func (win *window) plainStartDir() string {
	home := win.opt.Dir
	if home == "" {
		home = win.home
	}
	if win.bench != nil {
		return home
	}
	dir, err := config.ResolveStartDir(win.opt.Config.StartDir, home)
	if err == nil {
		return dir
	}
	warning := fmt.Sprintf("start_dir: %v; using the home folder", err)
	if !slices.Contains(win.opt.Warnings, warning) {
		win.opt.Warnings = append(win.opt.Warnings, warning)
	}
	win.note(warning)
	return home
}

func (win *window) startDirRow() settingRow {
	path := win.opt.Config.StartDir
	if path == "" {
		path = win.home
	}
	return settingRow{label: "Start folder", hint: tildeHome(path, win.home), monoHint: true,
		ctrls: []ctrl{{kind: ctlButton, id: "ws.start.pick", text: "Choose…", dead: win.wsp.picking},
			{kind: ctlButton, id: "ws.start.home", text: "Use home", ghost: true, dead: win.opt.Config.StartDir == "" || win.wsp.picking}}}
}

func (win *window) setStartDir(value string) {
	if _, err := config.ResolveStartDir(value, win.home); err != nil {
		win.note("Start folder: " + err.Error() + ". Choose an existing folder.")
		return
	}
	win.opt.Config.StartDir = value
	win.saveSetting("start_dir", value)
}

// pickStartDir shares the workspace picker's single-flight queue. A tagged
// result applies to this setting even if a workspace draft has since opened.
func (win *window) pickStartDir() {
	p := &win.wsp
	if p.picking {
		return
	}
	start := win.plainStartDir()
	if p.picked == nil {
		p.picked = make(chan wsPicked, 1)
	}
	p.picking = true
	ch, w := p.picked, win.w
	go func() {
		path, ok, err := pickFolder(start)
		// Native pickers normally return absolute paths. Normalize only valid answers.
		if ok && path == "" && err == nil {
			err = fmt.Errorf("the folder picker returned an empty path")
		}
		if ok && err == nil {
			path, err = filepath.Abs(path)
		}
		ch <- wsPicked{startDir: true, path: path, ok: ok, err: err}
		if w != nil {
			w.Invalidate()
		}
	}()
}
