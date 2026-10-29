package workspaces

import (
	"strings"
	"testing"
)

// waiting is a started tab with commands that waits for w.
func waiting(t *testing.T, w Wait) *TabRun {
	t.Helper()
	r := &TabRun{Line: "npm run dev", Wait: w}
	if r.Event(StartedEvent{}) {
		t.Fatal("starting the shell submitted")
	}
	if r.State != Waiting {
		t.Fatalf("state %v after start, want Waiting", r.State)
	}
	return r
}

func TestStartedWithoutCommandsIsLaunchedWithoutSubmit(t *testing.T) {
	r := &TabRun{Wait: WaitDocker}
	if r.Event(StartedEvent{}) || r.State != Launched {
		t.Fatalf("state %v, want Launched without a submit", r.State)
	}
}

func TestStartFailedFails(t *testing.T) {
	r := &TabRun{Line: "a"}
	if r.Event(StartFailedEvent{Err: "no shell"}) || r.State != Failed || r.Err != "no shell" {
		t.Fatalf("state %v err %q", r.State, r.Err)
	}
}

func TestNoWaitSubmitsOnShellReady(t *testing.T) {
	r := waiting(t, WaitNone)
	if !r.DepReady {
		t.Fatal("a tab that waits for nothing is not dependency-ready")
	}
	if !r.Event(ShellReadyEvent{}) || r.State != Launched {
		t.Fatalf("state %v, want a submit and Launched", r.State)
	}
}

func TestBothFactsInEitherOrderSubmitOnce(t *testing.T) {
	for _, order := range [][]Event{
		{DepReadyEvent{}, ShellReadyEvent{}},
		{ShellReadyEvent{}, DepReadyEvent{}},
	} {
		r := waiting(t, WaitDocker)
		if r.Event(order[0]) {
			t.Fatalf("%T alone submitted", order[0])
		}
		if !r.Event(order[1]) {
			t.Fatalf("%T after %T did not submit", order[1], order[0])
		}
		// Nothing afterwards submits again.
		for _, e := range []Event{DepReadyEvent{}, ShellReadyEvent{}, RunCommandsEvent{}, ConfirmTimeoutEvent{}} {
			if r.Event(e) {
				t.Fatalf("%T submitted a second time", e)
			}
		}
		if r.State != Launched {
			t.Fatalf("state %v, want Launched", r.State)
		}
	}
}

func TestDepFailedFailsAndNeverSubmits(t *testing.T) {
	r := waiting(t, WaitPort)
	r.Event(DepFailedEvent{Err: "Port 8000 never answered."})
	if r.State != Failed || r.Err != "Port 8000 never answered." {
		t.Fatalf("state %v err %q", r.State, r.Err)
	}
	if r.Event(ShellReadyEvent{}) || r.Event(DepReadyEvent{}) || r.Event(RunCommandsEvent{}) {
		t.Fatal("a failed tab submitted")
	}
}

func TestExitedWhileWaitingFails(t *testing.T) {
	r := waiting(t, WaitDocker)
	r.Event(ExitedEvent{})
	if r.State != Failed || r.Err == "" {
		t.Fatalf("state %v err %q", r.State, r.Err)
	}
}

func TestTimeoutNeedsConfirmationThenRunCommandsSubmits(t *testing.T) {
	r := waiting(t, WaitNone)
	if r.Event(RunCommandsEvent{}) {
		t.Fatal("Run commands while plainly waiting submitted")
	}
	if r.Event(ConfirmTimeoutEvent{}) || r.State != NeedsConfirmation {
		t.Fatalf("state %v after the timeout, want NeedsConfirmation", r.State)
	}
	if !r.Event(RunCommandsEvent{}) || r.State != Launched {
		t.Fatalf("state %v after Run commands, want a submit and Launched", r.State)
	}
	if r.Event(RunCommandsEvent{}) || r.Event(ShellReadyEvent{}) {
		t.Fatal("submitted twice")
	}
}

func TestLatePromptAfterTimeoutStillSubmits(t *testing.T) {
	r := waiting(t, WaitNone)
	r.Event(ConfirmTimeoutEvent{})
	if !r.Event(ShellReadyEvent{}) || r.State != Launched {
		t.Fatalf("state %v, want the late prompt to submit", r.State)
	}
}

// The owner is only asked about the prompt once the dependency is ready.
func TestTimeoutBeforeDependencyAsksOnlyOnceItIsReady(t *testing.T) {
	r := waiting(t, WaitDocker)
	r.Event(ConfirmTimeoutEvent{})
	if r.State != Waiting {
		t.Fatalf("state %v while Docker is still starting, want Waiting", r.State)
	}
	if r.Event(DepReadyEvent{}) || r.State != NeedsConfirmation {
		t.Fatalf("state %v once Docker is up with no prompt seen, want NeedsConfirmation", r.State)
	}
}

func TestUserInputCancelsAndLaterReadinessSendsNothing(t *testing.T) {
	for _, s := range []TabState{Waiting, NeedsConfirmation} {
		r := waiting(t, WaitDocker)
		if s == NeedsConfirmation {
			r.Event(DepReadyEvent{})
			r.Event(ConfirmTimeoutEvent{})
		}
		if r.State != s {
			t.Fatalf("setup state %v, want %v", r.State, s)
		}
		if r.Event(UserInputEvent{}) || r.State != Cancelled {
			t.Fatalf("state %v after input, want Cancelled", r.State)
		}
		for _, e := range []Event{DepReadyEvent{}, ShellReadyEvent{}, RunCommandsEvent{}} {
			if r.Event(e) {
				t.Fatalf("%T after the owner took over submitted", e)
			}
		}
	}
}

func TestClosedCancels(t *testing.T) {
	r := waiting(t, WaitDocker)
	if r.Event(ClosedEvent{}) || r.State != Cancelled {
		t.Fatalf("state %v after close, want Cancelled", r.State)
	}
	if r.Event(DepReadyEvent{}) || r.Event(ShellReadyEvent{}) {
		t.Fatal("a closed tab submitted")
	}
}

func TestSettledStatesStay(t *testing.T) {
	r := waiting(t, WaitNone)
	r.Event(ShellReadyEvent{})
	r.Event(ClosedEvent{})
	r.Event(UserInputEvent{})
	if r.State != Launched {
		t.Fatalf("state %v, a launched tab must stay launched", r.State)
	}
	if !r.Settled() {
		t.Fatal("Launched is not settled")
	}
	f := waiting(t, WaitPort)
	f.Event(DepFailedEvent{Err: "x"})
	f.Event(ClosedEvent{})
	if f.State != Failed {
		t.Fatalf("state %v, a failed tab must stay failed", f.State)
	}
	if waiting(t, WaitNone).Settled() {
		t.Fatal("Waiting counts as settled")
	}
}

func TestSubmitFailedTurnsLaunchedIntoFailed(t *testing.T) {
	r := waiting(t, WaitNone)
	r.Event(ShellReadyEvent{})
	if r.Event(SubmitFailedEvent{Err: "Could not send the commands to this tab."}) || r.State != Failed {
		t.Fatalf("state %v, want Failed", r.State)
	}
	if r.Event(RunCommandsEvent{}) || r.Event(ShellReadyEvent{}) {
		t.Fatal("a failed submit was retried")
	}
	w := waiting(t, WaitNone)
	w.Event(SubmitFailedEvent{Err: "x"})
	if w.State != Waiting {
		t.Fatalf("state %v, a submit failure before any submit must change nothing", w.State)
	}
}

func TestSummary(t *testing.T) {
	ok := TabRun{State: Launched}
	cases := []struct {
		name   string
		runs   []TabRun
		titles []string
		want   string
		full   bool
	}{
		{"all", []TabRun{ok, ok, ok, ok}, []string{"api", "admin", "storefront", "api logs"}, "samurai: started 4 of 4 tabs", true},
		{"port", []TabRun{ok, ok, ok, {State: Failed, Err: "Port 8000 never answered."}}, []string{"api", "admin", "storefront", "api logs"},
			"samurai: started 3 of 4 tabs. api logs: port 8000 never answered.", false},
		{"two failures and a takeover", []TabRun{{State: Failed, Err: "Docker is not installed."}, {State: Cancelled}, ok, {State: Failed, Err: "Port 8000 never answered."}}, []string{"api", "admin", "storefront", "api logs"},
			"samurai: started 1 of 4 tabs. api: docker is not installed. api logs: port 8000 never answered.", false},
	}
	for _, c := range cases {
		got, full := Summary("samurai", c.runs, c.titles)
		if got != c.want || full != c.full {
			t.Errorf("%s: Summary = %q, %v; want %q, %v", c.name, got, full, c.want, c.full)
		}
	}
}

func TestRunLabel(t *testing.T) {
	ok, wait, fail := TabRun{State: Launched}, TabRun{State: Waiting}, TabRun{State: Failed}
	confirm := TabRun{State: NeedsConfirmation}
	cases := []struct {
		runs  []TabRun
		label string
		needs bool
	}{
		{[]TabRun{wait, ok}, "Starting", false},
		{[]TabRun{confirm, ok}, "Needs attention", true},
		{[]TabRun{fail, wait}, "Needs attention", true},
		{[]TabRun{ok, ok}, "Running", false},
		{[]TabRun{ok, ok, ok, fail}, "3 of 4", true},
		{[]TabRun{ok, {State: Cancelled}}, "1 of 2", false},
	}
	for _, c := range cases {
		label, needs := RunLabel(c.runs)
		if label != c.label || needs != c.needs {
			var s []string
			for _, r := range c.runs {
				s = append(s, r.State.String())
			}
			t.Errorf("RunLabel(%s) = %q, %v; want %q, %v", strings.Join(s, ","), label, needs, c.label, c.needs)
		}
	}
}
