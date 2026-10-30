# Plan 6b: agent awareness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each sidebar row knows whether a Claude session is running, needs the owner or is done, which slash command it last ran, its title by the spec's rule (or the owner's rename) and its lines added/removed; one key jumps to the next session that needs the owner.

**Architecture:** A new Claude Code UserPromptSubmit hook runs `kaze notify prompt`, which sends the leading slash command and its arguments over the existing notify channel as a new `prompt` kind. The window keeps a per-tab `agentState` (Claude seen, running, command, argument, the shell prompt count at the last hook) fed by prompt/done/needs/usage messages and cleared when the shell prompt comes back. The sidebar model reads it for the title, command, state and counts. Line counts come from `git diff --shortstat HEAD` run off the UI goroutine, at most one per tab, on events only.

**Tech Stack:** Go 1.27, Gio v0.10.3. Tests with `go test`.

**Spec:** `docs/superpowers/specs/2026-10-30-kaze-agent-sidebar-design.md` sections 5.2 (title rule, lines added/removed, rename), 6 (agent awareness), 11 (`next_waiting`), 15 item 2. Mockup: `docs/design/agent-6b-6d-6e-mockup.html` (the 6b column). Read the spec sections before any task.

**Not in 6b:** chips and timed checks (6c, parked); floating panes and the scratch shell (6d); the palette and Commands page (6e).

## Global Constraints

- Windows first; macOS must build and behave the same where it applies. `GOOS=windows go vet ./...` must pass; darwin vet of Gio packages can't run on Windows (the owner checks on the Mac).
- Go is at `%LOCALAPPDATA%\Programs\go\bin`; set `GOTMPDIR` under `C:\Users\john\Desktop\kaze-wt\`. If Smart App Control blocks a test binary, rerun with `-ldflags "-X main.r=<random>"` or `-B 0x<hex>`.
- Idle CPU stays at zero: no tickers. The running mark is drawn still. Git runs only on events (a shell prompt report or a hook message), never on a timer, at most one in flight per tab.
- The prompt hook must return fast, print nothing (Claude Code adds a prompt hook's output to the conversation) and always exit 0.
- Nothing writes `~/.claude/settings.json` except Settings, Claude, Fix with its show-the-change, confirm and back-up steps.
- Hook text is untrusted: strip control characters and cap lengths before it reaches the UI.
- Every color from theme tokens; all five themes, including Washi, must read well.
- Tests never read or write the owner's real `~/.claude/settings.json`, `config.toml` or `workspaces.toml`, never run real Docker, never open visible windows. Git tests use repos built in `t.TempDir()`; skip the test when `git` is not on PATH. RED tests must fail safely and be bounded.
- Known failures before this plan (Windows): `internal/ui` `TestPickFillsFolderTildeAndEmptyTitle`, `TestListMembersShowCommandsWithoutCutting`; `internal/workspaces` `TestExpandFolder`, `TestJoinedPOSIXFieldsKeepCurrentShellContext`.
- Commits: conventional, capitalized verb. No AI attribution lines. No em dashes in user-visible text, docs or comments; comments 1 to 2 lines, why not what.

## Review Focus

1. Claude exits (`/exit`, Ctrl+C): the next shell prompt report turns the row back into a plain shell, ends running and keeps a renamed title (Task 3).
2. The owner interrupts with Esc (no hook fires): the row stays running until the next prompt, Stop or needs message, and no timer is added (Task 3).
3. A hostile or huge prompt (1 MB, control characters, escape sequences, no leading `/`): the hook stays under its time budget, sends a bounded command/argument or none, and the row shows only clean text (Task 1).
4. `git` missing, slow or failing (not a repo, no HEAD yet): no console window flashes, no count shows, nothing blocks the UI, and a later event retries (Task 5).
5. Renaming: empty text goes back to the title rule; workspace members can't be renamed; a rename survives Claude exiting and coming back (Task 4).

---

### Task 1: `kaze notify prompt`

**Files:**
- Modify: `internal/notify/notify.go`, `cmd/kaze/notify.go`
- Test: `internal/notify/notify_test.go`, `cmd/kaze/notify_test.go`

**Interfaces:**
- Produces: `notify.KindPrompt Kind = "prompt"`; `notify.Msg` gains `Command string \`json:"command,omitempty"\`` and `Args string \`json:"args,omitempty"\``. The server accepts `prompt`, clears `Usage`, and blanks `Command` unless it matches `^/[A-Za-z0-9][A-Za-z0-9:_.-]{0,63}$`; `Args` has control characters removed and is cut to 120 runes.
- Produces: `func promptCommand(prompt string) (cmd, args string)` in `cmd/kaze/notify.go`: leading whitespace skipped; if the prompt starts with `/`, `cmd` is the first word and `args` the rest of the first line, trimmed; otherwise both empty.

- [ ] **Step 1: Write the failing tests**

```go
func TestPromptCommand(t *testing.T) // "/ship-ticket V3-3348" -> "/ship-ticket","V3-3348"; "  /v3-lap\nmore" -> "/v3-lap",""; "fix the bug" -> "",""; "/" -> "","" ; "/a b c" -> "/a","b c"
func TestNotifyPromptSendsCommand(t *testing.T)   // fake server; stdin {"prompt":"/review-mine --quality-only"} -> one Msg{Kind:"prompt", Command:"/review-mine", Args:"--quality-only"}, nothing printed, exit 0
func TestNotifyPromptPlainTextStillSends(t *testing.T) // "please continue" -> Msg{Kind:"prompt"} with empty Command (marks running)
func TestNotifyPromptHugeInputStaysBounded(t *testing.T) // 1 MB prompt with \x1b and \x07 -> returns within notifyTotal, Args <= 120 runes, no control chars
func TestServerCleansPrompt(t *testing.T)          // server gets Command "/x\x1b[31m" -> Command ""; Args with \n and \x1b -> stripped, 120-rune cap
```

- [ ] **Step 2: Run** `go test ./internal/notify/ ./cmd/kaze/ -run 'Prompt'` → FAIL (undefined).
- [ ] **Step 3: Implement** the kind, fields, server checks, `promptCommand` and the `prompt` branch of `runNotify` (hook JSON field `prompt`). Keep the existing always-exit-0 and time-budget behavior.
- [ ] **Step 4: Run** `go test ./internal/notify/ ./cmd/kaze/` → PASS.
- [ ] **Step 5: Commit** `feat(notify): Report the slash command a prompt starts with`

### Task 2: The prompt hook in Claude Code setup

**Files:**
- Modify: `internal/claudesetup/setup.go`, `internal/ui/settings_claude.go`
- Test: `internal/claudesetup/setup_test.go`, `internal/ui/settings_claude_test.go`

**Interfaces:**
- Produces: `claudesetup.Report.Prompt Entry`, found under hook event `UserPromptSubmit` with args `notify prompt` and no matcher. `Inspect` fills it (Unreadable for all four on a parse error); `Fix` adds a missing one or repoints a broken one with summary text `the prompt hook (shows which command each session runs)`.
- The settings page's setup row gains the line `Command each session runs: <entryText(r.Prompt)>`; `fixable(r.Prompt)` joins the Fix condition.

- [ ] **Step 1: Write the failing tests**
  - `TestInspectFindsThePromptHook`: settings with a `UserPromptSubmit` kaze entry → `Prompt.State == OK`; without → `Missing`; pointing at a missing exe → `Broken`.
  - `TestFixAddsThePromptHook`: Missing → output has `hooks.UserPromptSubmit[0].hooks[0].command == "\"<exe>\" notify prompt"`, no `matcher` key, other hooks untouched; summary includes the prompt line.
  - `TestClaudeSectionShowsThePromptHook`: the setup row text includes `Command each session runs:` and the Fix button shows when only the prompt hook is missing (fake settings path in `t.TempDir()`).
- [ ] **Step 2: Run** `go test ./internal/claudesetup/ ./internal/ui/ -run 'Prompt'` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/claudesetup/ ./internal/ui/` → PASS (baseline failures aside).
- [ ] **Step 5: Commit** `feat(claudesetup): Check and fix the prompt hook`

### Task 3: Agent state per tab

**Files:**
- Create: `internal/ui/agent.go`
- Modify: `internal/ui/alerts_ui.go` (`onNotify`, `deliver`), `internal/ui/window.go` (`frame`: prune and leave checks)
- Test: `internal/ui/agent_test.go`

**Interfaces:**
- Produces:

```go
type agentState struct {
	claude, running bool
	command, arg    string // last slash command and its first argument
	shellAt         uint64 // cwdSeq(t) when the last hook message arrived
}
func (win *window) agentOf(id tabs.TabID) agentState // zero value for an unknown tab
func (win *window) applyAgent(m notify.Msg, t *tabs.Tab) // UI goroutine
func (win *window) leaveAgents(list []*tabs.Tab)       // a newer cwdSeq than shellAt means Claude exited
```

Rules (spec 6.1):
- `prompt`: claude = true, running = true; when `Command != ""`, set command and arg (`arg` = first whitespace-separated word of `Args`, cut to 40 runes); clear the tab's alert mark and toast (`alerts.visited`); no chime, flash or toast. `onNotify` must not chime or flash for `prompt` (it passes straight to the inbox like usage does to its store).
- `done` / `needs`: claude = true, running = false (marks and toasts as today).
- `usage`: claude = true.
- Every hook message sets `shellAt = cwdSeq(t)`.
- `leaveAgents`, once per frame: a Claude tab whose `cwdSeq` is now greater than `shellAt` becomes claude = false, running = false, command = ""; `arg` is kept so the row keeps its title (spec 6.1).
- State for closed tabs is pruned with the other per-tab maps.

- [ ] **Step 1: Write the failing tests** (`testWindow`, `recStarter`, messages through `deliver`):

```go
func TestPromptMarksRunningWithCommand(t *testing.T)   // prompt "/ship-ticket" "V3-3348 now" -> claude, running, command "/ship-ticket", arg "V3-3348"
func TestPlainPromptKeepsTheCommand(t *testing.T)      // then prompt with empty Command -> running, command still "/ship-ticket"
func TestStopEndsRunning(t *testing.T)                 // done -> running false, claude true, mark done
func TestNeedsEndsRunning(t *testing.T)
func TestPromptClearsTheMark(t *testing.T)             // needs then prompt -> markOf == markNone, no toast for the tab
func TestPromptDoesNotChime(t *testing.T)              // onNotify(prompt) for a background tab -> chime fake not called, flash not called
func TestUsageMarksClaude(t *testing.T)
func TestShellPromptMeansClaudeLeft(t *testing.T)      // prompt, then the PTY writes an OSC 7 report -> after leaveAgents: claude false, running false, command "", arg kept
func TestInterruptStaysRunningWithoutATimer(t *testing.T) // prompt, then 3 frames with no messages -> still running; frame schedules no InvalidateCmd for it
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Prompt|Stop|Needs|Usage|ShellPrompt|Interrupt'` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline aside).
- [ ] **Step 5: Commit** `feat(ui): Track whether each tab's Claude is running`

### Task 4: Rows show it, and rename

**Files:**
- Modify: `internal/ui/sidebar_model.go`, `internal/ui/sidebar.go`, `internal/ui/sidebar_window.go`
- Test: `internal/ui/sidebar_model_test.go`, `internal/ui/sidebar_test.go`, `internal/ui/sidebar_rename_test.go`

**Interfaces:**
- Consumes: `agentOf` (Task 3).
- Produces: `sideRow` gains `Command string`, `Running, Claude bool`, `Add, Del int` (Task 5 fills the counts); `sideInput` gains `Agent func(tabs.TabID) agentState` and `Name func(tabs.TabID) string`; `win.names map[tabs.TabID]string`.

Title rule (spec 5.2): the owner's name wins; else the stored `arg` when non-empty (also after Claude left, so the row keeps its title); else the folder's base name when there is a folder report; else `tabLabel` as today. Plain shells use the folder's base name the same way. Workspace members keep their fixed titles and are never renamed.

Row drawing (spec 5.2): line 1 is the title, then ` · /command` in `th`'s magenta (ANSI magenta via `c.memberColor("magenta")`) while `Command != ""`; the state mark is an accent ring drawn still for running; the state word is `running` (accent) when running and the mark is none, else `needs you` / `done` as today. The filter also matches `Command`.

Rename: a double click (`pointer.Event.NumClicks == 2`) on a session row's title opens an inline single-line editor in place of the title, prefilled with the current title and fully selected. Enter saves (`names[id]`; blank text deletes the name), Esc cancels, a click elsewhere saves. While renaming the editor has the keyboard (add it to `focusTarget`); kaze's shortcuts still work, and anything that changes the main view ends the rename the way `dropFilter` ends filtering.

- [ ] **Step 1: Write the failing tests**

```go
func TestRowTitleRule(t *testing.T)            // name > arg > folder base > tabLabel, for Claude and plain tabs
func TestRowShowsCommandAndRunning(t *testing.T) // agent running "/ship-ticket" -> row.Command, Running, state word "running"
func TestNeedsBeatsRunningWord(t *testing.T)
func TestFilterMatchesCommand(t *testing.T)
func TestRunningMarkIsStill(t *testing.T)      // layoutSidebar twice with a running row -> no InvalidateCmd recorded
func TestDoubleClickRenames(t *testing.T)      // double click title, type "api work", Enter -> row title "api work"
func TestRenameBlankRestoresTheRule(t *testing.T)
func TestRenameEscCancels(t *testing.T)
func TestMembersAreNotRenamed(t *testing.T)
func TestRenameSurvivesClaudeLeaving(t *testing.T)
func TestShortcutWhileRenamingEndsIt(t *testing.T) // Ctrl+Shift+T while renaming -> new session, keyboard on the terminal, name saved
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'RowTitle|RowShows|NeedsBeats|FilterMatchesCommand|RunningMark|Rename|Members'` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline aside).
- [ ] **Step 5: Commit** `feat(ui): Show each session's command and state, and rename rows`

### Task 5: Lines added and removed

**Files:**
- Create: `internal/gitstat/gitstat.go`, `internal/gitstat/hide_windows.go`, `internal/gitstat/hide_other.go`, `internal/ui/linecount.go`
- Test: `internal/gitstat/gitstat_test.go`, `internal/ui/linecount_test.go`

**Interfaces:**
- Produces: `func gitstat.Shortstat(ctx context.Context, dir string) (add, del int, err error)` runs `git -C <dir> diff --shortstat HEAD` with no console window on Windows (same `HideWindow`/`CREATE_NO_WINDOW` as `internal/workspaces/probe_windows.go`), reads at most 4 KB of output, and parses `N insertion(s)(+)` / `M deletion(s)(-)`; empty output is `0, 0, nil`.
- Produces: `func (win *window) wantCount(t *tabs.Tab)` starts one count for a tab in a repo when none is in flight for it, with a 2 second timeout, and delivers the result on a channel the frame drains (then `Invalidate`). Called from `deliver` for prompt/done/needs messages and when a tab's `cwdSeq` changes (a shell prompt report). The model reads the latest result into `sideRow.Add/Del`.
- Rows draw `+N` (green, `th.Done`) and `-M` (red, `th.UsageHigh`) on line 3 when either is non-zero (spec 5.2).

- [ ] **Step 1: Write the failing tests**
  - `TestShortstatParses`: table test of the parser for `" 3 files changed, 212 insertions(+), 14 deletions(-)"`, insertions only, deletions only, singular forms, empty.
  - `TestShortstatInARealRepo` (skip without git): `git init`, commit a file, edit it → add/del as expected.
  - `TestShortstatNotARepo`: a plain temp dir → error, returns within the timeout.
  - `TestCountsRunOnEventsOnly`: with a fake `gitstat` func counting calls, 5 idle frames → 0 calls; a done message → 1; a second message while the first is in flight → still 1 in flight.
  - `TestCountShowsOnTheRow`: fake returns 212/14 → row Add 212, Del 14.
- [ ] **Step 2: Run** `go test ./internal/gitstat/ ./internal/ui/ -run 'Shortstat|Count'` → FAIL.
- [ ] **Step 3: Implement** (the window holds `countFn func(ctx, dir) (int, int, error)` so tests fake it).
- [ ] **Step 4: Run** `go test ./internal/gitstat/ ./internal/ui/` → PASS.
- [ ] **Step 5: Commit** `feat(ui): Show lines added and removed on each row`

### Task 6: Jump to what needs you

**Files:**
- Modify: `internal/config/config.go`, `internal/ui/logic.go`, `internal/ui/sidebar.go`, `internal/ui/sidebar_window.go`
- Test: `internal/config/config_test.go`, `internal/ui/logic_test.go`, `internal/ui/sidebar_window_test.go`

**Interfaces:**
- Produces: action `next_waiting` (`actNextWaiting`), defaults Ctrl+Shift+J (Windows) and Cmd+J (macOS); `func (win *window) nextWaiting()`; a "Next waiting" button at the bottom of the sidebar, above the workspaces section, showing the key (`hits.next`).

Behavior (spec 6.3): go to the next session after the shown one, in `sessionOrder`, whose mark is needs, wrapping; with none, set the status-bar notice `Nothing is waiting on you.` and change nothing else. Members are skipped.

- [ ] **Step 1: Write the failing tests**: defaults and `KnownActions`; `TestNextWaitingCycles` (two needs sessions, three presses wrap); `TestNextWaitingNoneShowsNotice`; `TestNextWaitingButton` (click the button hit).
- [ ] **Step 2: Run** → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/config/ ./internal/ui/` → PASS.
- [ ] **Step 5: Commit** `feat(ui): Jump to the next session that needs you`

### Task 7: Docs and version

**Files:**
- Modify: `README.md`, `docs/HANDOFF.md`, `internal/version/version.go`, `internal/version/version_test.go`

- [ ] **Step 1:** version test expects `0.8` → FAIL; set `Number = "0.8"` → PASS.
- [ ] **Step 2:** README: the prompt hook (what it does, that it prints nothing, Fix in Settings), rows (command, running, title rule, rename, line counts), `next_waiting` in the shortcuts and `[keys]` table. HANDOFF: what 6b shipped, that the owner should press Fix once to add the prompt hook, 6d and 6e next, 6c parked, go-winres rerun for 0.8.
- [ ] **Step 3:** `go test ./...` → only the known baseline failures.
- [ ] **Step 4: Commit** `docs: Describe agent awareness and bump kaze to 0.8`
