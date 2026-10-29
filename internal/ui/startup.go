package ui

import (
	"fmt"
	"strings"

	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// openStartup opens the window's first tabs: the --workspace one, else the
// startup_workspace one, else the usual tab. A workspace that can't run leaves
// the usual tab with a warning saying why, and nothing typed into any shell.
func (win *window) openStartup(def []string) error {
	ws, warn, ok := win.startupWorkspace()
	if ok {
		plan, err := win.preflight(ws)
		if err != nil {
			warn = fmt.Sprintf("workspace %q not started: %v", ws.Name, err)
		} else {
			run := win.launch(ws, plan, win.opt.Warnings)
			if !win.mgr.Empty() {
				return nil
			}
			why := "no shell could start"
			if len(run.members) > 0 && run.members[0].run.Err != "" {
				why = run.members[0].run.Err
			}
			warn = fmt.Sprintf("workspace %q not started: %s", ws.Name, why)
		}
	}
	if warn != "" {
		win.opt.Warnings = append(win.opt.Warnings, warn)
	}
	_, _, err := openFirst(win.mgr, &win.opt.Shell, def, win.opt.Warnings)
	return err
}

// startupWorkspace is the workspace to start with, if one is chosen and found;
// otherwise warn says why the usual tab opens instead ("" when none was chosen).
// A name from --workspace never falls back to the startup_workspace setting.
func (win *window) startupWorkspace() (ws workspaces.Workspace, warn string, ok bool) {
	name, id := strings.TrimSpace(win.opt.Workspace), win.opt.Config.StartupWorkspace
	if name == "" && id == "" {
		return ws, "", false
	}
	what := "the startup workspace"
	if name != "" {
		what = fmt.Sprintf("workspace %q", name)
	}
	if win.cfgPath == "" {
		return ws, what + " not started: kaze has no settings folder", false
	}
	doc, _, err := workspaces.Load(workspaces.Path(win.cfgPath))
	if err != nil {
		return ws, what + " not started: " + err.Error(), false
	}
	if name != "" {
		if ws, ok = doc.ByName(name); !ok {
			return ws, fmt.Sprintf("no workspace is named %q; opened a normal tab", name), false
		}
		return ws, "", true
	}
	if ws, ok = doc.ByID(id); !ok {
		return ws, "the startup workspace no longer exists; opened a normal tab (choose one in Settings > Workspaces)", false
	}
	return ws, "", true
}
