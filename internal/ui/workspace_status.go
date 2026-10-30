package ui

import (
	"fmt"

	"github.com/patrick-gogo/kaze-terminal/internal/tabs"
	"github.com/patrick-gogo/kaze-terminal/internal/workspaces"
)

// memberStatus is how far a run member got, in words for the group menu, and
// whether that needs the owner or failed. Finished means the shell reported a
// prompt after its line was typed; kaze cannot tell whether the commands worked.
func (win *window) memberStatus(m *wsMember, t *tabs.Tab) (text string, needs, failed bool) {
	r := &m.run
	switch {
	case r.State == workspaces.Failed:
		return "Failed", false, true
	case r.State == workspaces.Cancelled:
		return "Cancelled", false, false
	case r.State == workspaces.NeedsConfirmation:
		return "Confirm to run", true, false
	case r.Line == "":
		return "No commands", false, false
	case r.State == workspaces.Launched:
		if t != nil && promptCount(t.Session) > m.promptAt {
			return "Finished", false, false
		}
		return "Running", false, false
	case !r.ShellReady || r.DepReady:
		return "Starting", false, false
	case r.Wait == workspaces.WaitDocker:
		return "Waiting for Docker", false, false
	case r.Wait == workspaces.WaitPort:
		return fmt.Sprintf("Waiting for port %d", r.Port), false, false
	}
	return "Starting", false, false
}
