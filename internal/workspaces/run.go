package workspaces

import (
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// TabState is where one workspace tab is on its way to running its commands.
type TabState int

const (
	StartingShell TabState = iota
	Waiting
	NeedsConfirmation
	Launched
	Failed
	Cancelled
)

func (s TabState) String() string {
	switch s {
	case StartingShell:
		return "StartingShell"
	case Waiting:
		return "Waiting"
	case NeedsConfirmation:
		return "NeedsConfirmation"
	case Launched:
		return "Launched"
	case Failed:
		return "Failed"
	case Cancelled:
		return "Cancelled"
	}
	return fmt.Sprintf("TabState(%d)", int(s))
}

// Event is something that happened to a workspace tab while it starts.
type Event interface{ isEvent() }

type (
	StartedEvent        struct{}             // the shell is running
	StartFailedEvent    struct{ Err string } // the shell could not start
	ExitedEvent         struct{}             // the shell ended before the commands were sent
	DepReadyEvent       struct{}             // Docker or the port answered
	DepFailedEvent      struct{ Err string }
	ShellReadyEvent     struct{} // the shell drew its first prompt
	ConfirmTimeoutEvent struct{} // no prompt marker within the wait
	UserInputEvent      struct{} // the owner typed, pasted, clicked or pressed Use shell
	RunCommandsEvent    struct{} // the owner confirmed the shell is at its prompt
	ClosedEvent         struct{}
	SubmitFailedEvent   struct{ Err string } // the submitted line could not be queued
)

func (StartedEvent) isEvent()        {}
func (StartFailedEvent) isEvent()    {}
func (ExitedEvent) isEvent()         {}
func (DepReadyEvent) isEvent()       {}
func (DepFailedEvent) isEvent()      {}
func (ShellReadyEvent) isEvent()     {}
func (ConfirmTimeoutEvent) isEvent() {}
func (UserInputEvent) isEvent()      {}
func (RunCommandsEvent) isEvent()    {}
func (ClosedEvent) isEvent()         {}
func (SubmitFailedEvent) isEvent()   {}

// TabRun is one workspace tab's startup. Line is the joined command line, empty
// for a tab without commands.
type TabRun struct {
	State      TabState
	Line       string
	Wait       Wait
	Port       int
	DepReady   bool
	ShellReady bool
	TimedOut   bool // the prompt wait passed; ask the owner once the dependency is ready
	Err        string
	Started    time.Time
}

// Event applies e and reports whether the line must be submitted now. That is
// true exactly once, and never after the tab was cancelled, failed or launched.
func (r *TabRun) Event(e Event) (submit bool) {
	if r.Settled() {
		// The only way out of Launched: the line it submitted never reached the queue.
		if f, ok := e.(SubmitFailedEvent); ok && r.State == Launched {
			r.State, r.Err = Failed, f.Err
		}
		return false
	}
	switch e := e.(type) {
	case StartedEvent:
		if r.State != StartingShell {
			return false
		}
		if r.Wait == "" || r.Wait == WaitNone {
			r.DepReady = true
		}
		if r.Line == "" {
			r.State = Launched // nothing to send, so nothing to wait for
			return false
		}
		r.State = Waiting
	case StartFailedEvent:
		r.State, r.Err = Failed, e.Err
	case ExitedEvent:
		r.State, r.Err = Failed, "The shell exited before the commands were sent."
	case DepReadyEvent:
		r.DepReady = true
	case DepFailedEvent:
		r.State, r.Err = Failed, e.Err
	case ShellReadyEvent:
		r.ShellReady = true
	case ConfirmTimeoutEvent:
		r.TimedOut = true
	case UserInputEvent, ClosedEvent:
		r.State = Cancelled
	case RunCommandsEvent:
		if r.State == NeedsConfirmation {
			r.ShellReady = true // the owner sees the prompt
		}
	}
	return r.advance()
}

func (r *TabRun) advance() bool {
	if r.State != Waiting && r.State != NeedsConfirmation {
		return false
	}
	switch {
	case r.DepReady && r.ShellReady:
		r.State = Launched
		return true
	case r.DepReady && r.TimedOut:
		r.State = NeedsConfirmation
	}
	return false
}

// Settled reports that the tab will not change on its own any more.
func (r *TabRun) Settled() bool {
	return r.State == Launched || r.State == Failed || r.State == Cancelled
}

// Summary is the one-line result of a settled run, and whether no tab failed; a
// tab the owner took over is not a failure.
func Summary(name string, runs []TabRun, titles []string) (string, bool) {
	started := 0
	var fails []string
	for i, r := range runs {
		switch r.State {
		case Launched:
			started++
		case Failed:
			fails = append(fails, titles[i]+": "+lowerFirst(r.Err))
		}
	}
	s := fmt.Sprintf("%s: started %d of %d tabs", name, started, len(runs))
	if len(fails) > 0 {
		s += ". " + strings.Join(fails, " ")
	}
	return s, len(fails) == 0
}

func lowerFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToLower(r)) + s[n:]
}

// RunLabel is a run's state for its tab group: Starting, Running, Needs
// attention, or "3 of 4" once it settled short. needs is true while a tab
// waits for the owner or failed.
func RunLabel(runs []TabRun) (label string, needs bool) {
	started, settled := 0, true
	for _, r := range runs {
		switch r.State {
		case Launched:
			started++
		case Failed, NeedsConfirmation:
			needs = true
		}
		if !r.Settled() {
			settled = false
		}
	}
	switch {
	case !settled && needs:
		return "Needs attention", true
	case !settled:
		return "Starting", false
	case started == len(runs):
		return "Running", false
	}
	return fmt.Sprintf("%d of %d", started, len(runs)), needs
}
