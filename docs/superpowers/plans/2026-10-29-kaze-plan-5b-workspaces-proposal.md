# Plan 5b: Workspaces implementation proposal

Date: 2026-10-29
Status: PROPOSED FOR REVIEW; no implementation in this branch.
Baseline: main 04d9b82 (Settings PR #11 merged).

**Outcome:** save a project launch recipe, start its tabs once, and return to them without repeating setup.

**Behavior authority for this proposal:** [Workspaces behavior spec](../specs/2026-10-29-kaze-workspaces-proposal.md). Its decision table lists departures from the earlier approved settings design. Obtain review of those decisions before implementation. Do not treat this plan as approval to revise the earlier specification.

## Architecture and existing integration points

- Add `internal/workspaces` for definitions, persistence, validation, immutable launch snapshots, and a deterministic startup state machine. Keep Gio and OS calls outside its pure model.
- Add `internal/workspaces/probe` (or files in the same package if a subpackage adds no value) for cancellable Docker/loopback probes and Docker Desktop launch adapters.
- Add `internal/shelllaunch` for supported-shell readiness/command adapters, if the spike justifies a shared package. Keep shell-specific quoting and script handling here.
- Extend `internal/tabs/manager.go`: its start callback currently accepts only environment and `NewWith` only accepts prefeed. Introduce per-tab launch options carrying folder and metadata while preserving `New()` and its normal home-tab behavior. Never mutate global `win.opt.Dir` temporarily to create workspace tabs.
- Extend `internal/term/session.go`: expose launch-input acceptance and write-failure reporting; current `Write` silently drops a full queue. Keep screen mutation locked and workers away from UI state.
- Add `internal/ui/settings_workspaces.go`, editor state, and tests. Integrate into the existing `settingsSection` contract in `settings.go`; preserve the Appearance and Claude sections, scrolling, keyboard focus, and dialogs.
- Add workspace launch state to `window`, driven on the UI goroutine. Workers send bounded result messages tagged with run generation/tab ID; they never touch Gio controls, tabs, or config structs directly.
- Reuse the local config directory, `config.Set`, theme colors, tab manager cleanup, `term` session notifications, and PTY abstraction. Keep recipe data separate from `config.toml` and Claude settings.

Proposed logical interfaces (names may change after the spike):

```go
// Definitions are persisted; LaunchSnapshot is immutable after preflight.
type Store interface {
    Load() (Document, Revision, error)
    Save(Document, Revision) (Revision, error) // rejects stale source revision
}
type ShellStarter interface {
    Start(TabLaunch) (SessionHandle, error)
}
type Probe interface {
    Wait(context.Context, Dependency) error
}
// Startup reducer accepts events and returns effects; it owns no goroutines.
func Advance(State, Event) (State, []Effect)
```

The concrete SessionHandle must support accepted/full/closed launch submission and asynchronous write errors. Do not add a broad framework to achieve these small interfaces.

## Required sequencing

### Task 0: Resolve the behavior decisions and prove shell delivery

- [ ] Review the spec's decision table, especially readiness fallback, same-shell execution, and failure semantics.
- [ ] Write bounded PTY integration fixtures for zsh on macOS and PowerShell 7 on Windows. Fixtures use temporary profiles and files; they never type into an OS window.
- [ ] Prove a profile that takes longer than three seconds cannot receive commands before its explicit readiness marker. Verify marker parsing and tab/generation association.
- [ ] Prove environment activation and a changed folder remain in the interactive shell after the recipe finishes.
- [ ] Prove two commands execute in order; a failing first command stops the second; a long-running final command receives no subsequent startup text as stdin.
- [ ] Prove Unicode and paths containing spaces, single quotes, double quotes, dollar signs, and backticks round-trip as data in generated launcher paths. Do not interpolate folder paths into commands when PTY cwd can set them directly.
- [ ] Prove Ctrl+C leaves a usable shell and cleanup does not delete a script before the shell reads it.
- [ ] Record protocol, adapter support, and results in `docs/spikes/workspace-shell-launch.md`. If the candidate fails, revise the design before building the full UI. Unsupported shells must fail preflight for command-bearing tabs.

**Gate:** Windows and macOS evidence, not just a mock, before automatic command launching is implemented. Do not use OSC 7 alone as a completion protocol or substitute screen scraping.

### Task 1: Definition model and persistence

Files: `internal/workspaces/model.go`, `store.go`, matching tests.

- [ ] Test and implement the documented v1 TOML schema, bounded load, strict fields, stable IDs, name uniqueness, command/color/wait validation, and folder normalization.
- [ ] Cover absent file, unsupported version, duplicate IDs, malformed nested tables, Unicode/space paths, invalid ports, limits, and meaningful field-level errors.
- [ ] Implement atomic writes, symlink and permission preservation, and stale revision rejection. A rejected or failed save leaves old bytes intact.
- [ ] Preserve definitions on read failure; show an error rather than returning an empty writable document.
- [ ] Keep relative paths and shell-variable expansion out of accepted launch folders. Folder checks use injected filesystem operations in unit tests.

**Gate:** temp-directory round trips on both platforms, stale-write and failed-rename tests, and no new dependency unless justified.

### Task 2: Per-tab launch options and input delivery

Files: `internal/tabs/manager.go`, `internal/ui/window.go`, `internal/term/session.go`, tests.

- [ ] Add per-tab folder, fixed title, workspace/run/recipe-tab metadata, and color while preserving ordinary `New()` behavior and terminal identity environment variables.
- [ ] Keep user-saved workspace titles stable when applications emit OSC title updates; retain dynamic titles for ordinary tabs.
- [ ] Test distinct folders for concurrently created tabs; no shared mutable start-directory race.
- [ ] Implement the spike's supported-shell launcher and private temporary artifact lifetime.
- [ ] Add launch enqueue acceptance and failure events. Test closed/full queues, short PTY writes, and no UI blocking. Never retry a launch after uncertain delivery.
- [ ] Test exactly-once submission per generation; retaining the interactive shell after success, failure, or interruption.

**Gate:** ordinary terminal regression tests still pass. No global keyboard/mouse injection, detached shell host, or changes to Claude's config.

### Task 3: Startup state machine and readiness probes

Files: `internal/workspaces/run.go`, probe/platform files, tests.

- [ ] Implement reducer states/events/effects from the behavior spec, with injected clock, shell starter, probe, and cancellation functions.
- [ ] Preflight everything, snapshot definitions, then create shells in order. Focus the first successful tab once.
- [ ] Test Run/Run double clicks, mixed successful and failed shell starts, missing folders, generation reuse attempts, recipe edits during a run, and stale worker results after close.
- [ ] Docker and port waits follow documented intervals/deadlines; share equal dependencies within one run. Start Docker Desktop at most once per run.
- [ ] Test Docker missing, launcher failure, hung CLI, IPv4/IPv6 loopback success, port timeout, tab cancellation, and app shutdown without live Docker or real network dependencies in unit tests.
- [ ] Add shell-ready events and the three-second explicit-confirmation state. User takeover cancels pending submission before forwarding input.
- [ ] Test successful dependency completion racing with typing, close, and shutdown; every terminal outcome releases workers and timers.

**Gate:** deterministic tests with fake time and bounded queues. No sleep-driven tests lasting three minutes. No residual pollers after a settled launch.

### Task 4: Workspace settings and editor

Files: `internal/ui/settings_workspaces.go`, editor helpers, settings integration/tests.

- [ ] Enable Workspaces sidebar entry; implement empty/error/list states and the explicit draft editor.
- [ ] Add tab/command ordering, named colors, folder picker abstraction with text fallback, wait selection, field errors, Save/Cancel, and delete confirmation.
- [ ] Support text input, caret/edit behavior, paste, Unicode, IME-safe composition, and escaping from a field. Reuse Gio text editor widgets with Kaze styling where practical rather than building a new text editing engine.
- [ ] Test keyboard navigation through fields, actions, sidebar, and confirmation dialogs; ensure terminal input cannot leak beneath settings.
- [ ] Dirty editor navigation prompts; stale source refuses overwrite; save never launches commands. Rename/delete do not kill existing tabs or change their snapshots.
- [ ] Verify draft retention through theme switches, errors, small-window scrolling, and large UI scale. Controls must remain reachable at 700x350.

**Gate:** native screenshot review on Windows and macOS for empty, populated, editor, validation error, and delete states in all five themes. Do not reset the owner's real config for fixtures.

### Task 5: Launch menu, status, and startup CLI

Files: `internal/ui/tabbar.go`, `window.go`, `status.go`, `internal/config/config.go`, `cmd/kaze/main.go`, tests.

- [ ] Implement plus-chevron menu with keyboard/click dismissal; normal plus/new-tab shortcuts remain unchanged.
- [ ] Bind live tabs to workspace IDs and show starting/open/partial state. Run again focuses first surviving tab, including a starting or exited tab, without rerunning commands.
- [ ] Draw static waiting/failed/confirmation information outside the PTY screen state, so output and cursor positions are not corrupted. Alert dots temporarily override workspace colors.
- [ ] Add optional `startup_workspace` ID and CLI `--workspace <name>` precedence. Unknown names or bad startup definitions follow documented fallback and show a useful warning.
- [ ] Guard window shutdown while first workspace tab is pending. All-starts-fail fallback cannot leave an empty immortal window.
- [ ] Test startup with no home tab for success, one home tab for fallback, no duplicate startup execution, normal tab close, and unchanged notify/statusline/version CLI dispatch.
- [ ] Startup selection is a separate config write. If clearing it on workspace deletion fails, surface that error; the stale-ID fallback must still be safe on restart.

**Gate:** end-to-end fixture starts real PTYs with harmless commands and fake readiness dependencies on each platform. No owner workspace is started automatically by tests.

### Task 6: Ship checks and documentation

- [ ] Run affected package tests, full `go test ./...`, `go vet ./...`, and targeted race tests; document any pre-existing failing tests separately rather than hiding them with a green summary.
- [ ] Build Windows and macOS executables; a cross-compile does not replace native shell integration verification.
- [ ] Measure idle CPU before/after; count workers and pending timers after success/cancel/timeouts; measure UI responsiveness while Docker CLI hangs. Workspace startup waits must not block frames or shutdown.
- [ ] Update README with schema, platform-specific command examples, readiness setup, default startup precedence, and failure/retry behavior. Explain how custom shells are handled.
- [ ] Update settings design with reviewed Plan 5b decisions and HANDOFF with actual results. Bump product version only when implementation ships, not in this docs PR.
- [ ] Only after explicit owner instruction, create their personal workspace through the UI. Do not commit their local filesystem layout or application commands as a default recipe.

## Reviewer checklist

1. Are the changed readiness and command-delivery decisions acceptable?
2. Does the shell adapter prove current-shell environment persistence and interruption on both systems?
3. Is partial startup useful and predictable without restarting successful servers?
4. Can user input or stale callbacks cause commands to be injected late or twice?
5. Can saves overwrite external edits, invalid files, or symlink targets incorrectly?
6. Does every wait have bounded execution and cleanup, with no background activity after launch?
7. Are Workspaces usable from keyboard and in small windows without regressing existing settings?

Suggested delivery: small commits per task on `feature/workspaces`, with behavior tests first and task review before continuing. The dependency order is 0 → 1 → 2 → 3 → 4 → 5 → 6; persistence and UI draft work may overlap only after interfaces are settled. No merge or owner-machine setup is implied by approving documentation.
