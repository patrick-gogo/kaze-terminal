# kaze Plan 5b: Workspaces with tab groups

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Save a project launch recipe, start all its tabs in one click as a single tab group, and return to it without starting anything twice.

**Architecture:** `internal/workspaces` holds the pure parts (recipe model and store, line joining, readiness probes, the per-tab launch state machine). The tab manager learns per-tab start options (folder, fixed title, group membership, color). The UI drives runs on its goroutine: probes report back over a bounded channel, prompt readiness is read from the terminal's OSC 7 / OSC 133 counters, and the joined command line is submitted once. The tab strip draws a run as one group chip with a dropdown. Settings gains the Workspaces section with a draft editor.

**Tech stack:** Go 1.27, Gio v0.10.3 (`widget.Editor` for text fields), BurntSushi/toml.

**Spec (authority, in this order):** `docs/superpowers/specs/2026-10-29-kaze-workspaces-decisions.md` (wins on conflict), then `docs/superpowers/specs/2026-10-29-kaze-workspaces-proposal.md`, then `docs/superpowers/specs/2026-10-29-kaze-settings-page-design.md` section 6.2. Mockups: `docs/design/tab-group-mockup.html` (variant A), `docs/design/workspaces-mockup.html`.

## Global constraints

- Idle CPU stays zero: no tickers. Probes and the 3-second readiness timer exist only while a launch is pending; everything stops once every tab is settled.
- Never type into a shell the owner has taken over: any key, paste or terminal mouse input in a waiting tab cancels its launch before the input is forwarded. Close or app exit cancels too; late results are ignored by generation.
- A launch line is submitted at most once per tab per run and is never retried.
- `workspaces.toml` lives next to `config.toml`; kaze owns its format (header says so). Strict load: invalid file → readable error, nothing launches from it, its bytes are never replaced. Writes: temp file in the same dir + sync + rename, symlinks followed, stale-revision writes refused.
- Readiness: first OSC 7 or OSC 133 `A` after the shell starts; none within 3s → "Waiting for shell prompt" with **Run commands** / **Use shell**. Never send on a timer alone.
- Joined line: PowerShell `a; if ($?) { b }`; zsh/bash/sh `a && b`; one Enter. Other shells: command-bearing tabs fail preflight.
- Probes: Docker `docker info` immediately then every 2s, 2s per-probe deadline, start Docker Desktop at most once per run; port 127.0.0.1 then ::1, 500ms per probe, every 1s; overall 3 minutes. Probes for the same dependency in one run are shared.
- Bounds: name/title 80 runes, 20 tabs/workspace, 20 commands/tab, 8 KiB/command, 1 MiB/file; titles/names non-empty, no control chars; workspace names unique ignoring case; commands reject CR, LF, NUL and other control chars; folder: leading `~` expanded, must be absolute after expansion, no other expansion.
- Colors: cyan, magenta, yellow, green, blue, red mapped through the current theme's ANSI colors.
- Never touch the owner's real `config.toml`, `workspaces.toml` or Claude settings in tests; never start the owner's workspace automatically.
- Plain language in UI text; no em dashes.

## Review focus

1. A dependency becomes ready in the same frame the owner types in that tab: the typed key wins and the line is never sent.
2. Run clicked twice quickly, or Run plus startup in one process: exactly one run, one set of tabs.
3. A hand-edited `workspaces.toml` changes while the editor is open: Save refuses, the draft survives.
4. The owner closes the last member while others are still waiting: their probes stop, no stray toast or wake afterwards.
5. A theme switch while the group dropdown or the editor is open: both repaint in the new theme, the draft is kept.

---

### Task 1: Recipe model and store

**Files:** create `internal/workspaces/model.go`, `store.go`, `model_test.go`, `store_test.go`.

**Interfaces (produces):**
- `type Wait string` (`WaitNone = "none"`, `WaitDocker = "docker"`, `WaitPort = "port"`); `type Tab struct { ID, Title, Folder string; Commands []string; Color string; Wait Wait; Port int }`; `type Workspace struct { ID, Name string; Tabs []Tab }`; `type Document struct { Workspaces []Workspace }`.
- `func (d Document) Validate(home string) []FieldError` with `type FieldError struct { Workspace, Tab, Field, Msg string }`; `func (d Document) ByID(id string) (Workspace, bool)`; `func (d Document) ByName(name string) (Workspace, bool)` (case-insensitive, unique).
- `func NewID() string` (random, `ws_` + 8 hex; tabs `t_` + 8 hex).
- `func ExpandFolder(folder, home string) (string, error)`.
- `type Revision [32]byte` (sha256 of the bytes read; zero for a missing file); `func Load(path string) (Document, Revision, error)` (missing → empty doc); `func Save(path string, d Document, prev Revision) (Revision, error)` (refuses with `ErrStale` when the file's current revision differs from `prev`; writes header comment `# Written by kaze's settings page. Edits here are fine, but comments are not kept.`, `schema_version = 1`, `[[workspaces]]`, `[[workspaces.tabs]]`).
- `func Path(configPath string) string` (sibling `workspaces.toml`).

- [ ] Tests: round trip of the samurai example; strict errors for unknown fields, `schema_version = 2`, duplicate IDs, duplicate names (case-insensitive), bad color, port 0 / 65536 with wait port, port set with wait none, CR in a command, empty title, 21 tabs, relative folder; `~` expansion; missing file = empty, revision zero; `ErrStale` after an external edit leaves the file byte-identical; invalid file returns an error and `Save` with its zero/old revision refuses; CRLF input loads.
- [ ] Commit `feat(workspaces): Add the workspace recipe model and store`.

### Task 2: Shell kinds and the joined line

**Files:** create `internal/workspaces/line.go`, `line_test.go`.

**Interfaces:** `type ShellKind int` (`ShellUnknown`, `ShellPowerShell`, `ShellPOSIX`); `func KindOf(argv []string) ShellKind` (base name without extension, case-insensitive: `pwsh`, `powershell` → PowerShell; `zsh`, `bash`, `sh` → POSIX); `func JoinLine(k ShellKind, cmds []string) (string, error)` (empty slice → "", nil; unknown kind with commands → error naming the shell; commands copied verbatim, blank ones skipped).

- [ ] Tests: one command unchanged; three commands for each kind (`a; if ($?) { b }; if ($?) { c }`, `a && b && c`); quotes, `$`, backticks and Unicode passed through verbatim; unknown shell errors; `C:\Program Files\PowerShell\7\pwsh.exe` and `/bin/zsh` detected.
- [ ] Commit `feat(workspaces): Join a tab's commands into one shell line`.

### Task 3: Per-tab start options, prompt readiness and submit

**Files:** modify `internal/tabs/manager.go`, `internal/term/session.go`, `internal/term/apply.go`, `internal/term/cwd.go` (or a new `prompt.go`), `internal/ui/window.go`; tests in `internal/tabs`, `internal/term`.

**Interfaces:**
- `tabs.StartOpts struct { Dir, Title, Color string; Group *Group }`; `tabs.Group struct { Workspace, Name string; Run uint64; Member int }`; `Tab` gains `Group *Group`, `Color string`, `FixedTitle string` (when set, `Title` stays it despite OSC titles).
- `func (m *Manager) NewOpts(o StartOpts) (*Tab, error)`; `New()` unchanged. The manager's start callback becomes `func(env []string, dir string)`; the window passes `o.Dir` or `win.opt.Dir`.
- `func (a *Applier) PromptSeq() uint64`: counts OSC `133;A` marks (OSC 133 is otherwise ignored).
- `type SubmitResult int` (`Accepted`, `QueueFull`, `Closed`); `func (s *Session) Submit(p []byte) SubmitResult`: like Write but reports the outcome. `Write` keeps today's behaviour.

- [ ] Tests: two tabs created with different dirs reach `start` with their own dir; ordinary `New` uses the default; fixed title survives an OSC 0 title; `PromptSeq` counts `ESC ] 133 ; A BEL` and ignores `133;B`/`133;C`; Submit returns Closed after exit and QueueFull on a full queue (bounded test queue).
- [ ] Commit `feat(tabs): Start tabs with their own folder, title and group, and report submits`.

### Task 4: Readiness probes

**Files:** create `internal/workspaces/probe.go`, `probe_windows.go`, `probe_other.go`, `probe_test.go`.

**Interfaces:** `type Probes struct { Exec func(ctx context.Context, name string, args ...string) error; Dial func(ctx context.Context, addr string) error; StartDocker func() error; Now func() time.Time }` (defaults use os/exec, net.Dialer, the platform Docker Desktop launcher: Windows `%LOCALAPPDATA%\Programs\DockerDesktop\Docker Desktop.exe` or `C:\Program Files\Docker\Docker\Docker Desktop.exe`; macOS `open -a Docker`); `func (p Probes) WaitDocker(ctx context.Context) error`; `func (p Probes) WaitPort(ctx context.Context, port int) error`. Errors: `ErrDockerMissing` (no CLI and no Desktop), `context.DeadlineExceeded` on the 3-minute limit.

- [ ] Tests with fake Exec/Dial/clock (no real Docker, no sleeps over 1s): ready on first probe; ready on third probe; Docker Desktop started exactly once; missing Docker → `ErrDockerMissing`; port ready on ::1 only; deadline returns promptly; cancel stops the loop and a hung Exec is abandoned at its 2s per-probe deadline.
- [ ] Commit `feat(workspaces): Add Docker and port readiness probes`.

### Task 5: Running a workspace

**Files:** create `internal/workspaces/run.go`, `run_test.go` (pure per-tab state machine), `internal/ui/workspace_run.go`, `workspace_run_test.go`; modify `internal/ui/window.go` (input cancel hook, frame drain, `Empty` guard), `internal/ui/alerts_ui.go` (summary toast reuse).

**Interfaces:**
- `workspaces.TabState` (`StartingShell`, `Waiting`, `NeedsConfirmation`, `Launched`, `Failed`, `Cancelled`); `type TabRun struct { State TabState; Line string; Wait Wait; Port int; DepReady bool; ShellReady bool; Err string; Started time.Time }`; `func (r *TabRun) Event(e Event) (submit bool)` for `DepReadyEvent`, `DepFailedEvent{Err}`, `ShellReadyEvent`, `ConfirmTimeoutEvent`, `UserInputEvent`, `RunCommandsEvent`, `ClosedEvent`. `submit` is true exactly once, when both readiness facts hold (or on RunCommands from NeedsConfirmation), and never after `Cancelled`, `Failed` or `Launched`.
- UI: `func (win *window) runWorkspace(id string)`: reload the store, preflight (validate, expand folders, check they exist, `KindOf(win.opt.Shell)`, `JoinLine` per tab), refuse if a run of that workspace still has open members (focus its first open member instead), create tabs in recipe order with `NewOpts` (group = workspace id/name, a new run number, member index), focus the first, start one probe goroutine per distinct dependency with a run-scoped context, results onto a bounded channel drained on the UI goroutine each frame.
- Readiness per tab: `ShellReady` when `CwdSeq` or `PromptSeq` moved past the value at creation; a tab with commands and no readiness after 3s gets `ConfirmTimeoutEvent` via one `op.InvalidateCmd{At}`.
- Submit: `Session.Submit(line + "\r")`; Accepted → Launched; anything else → Failed with a message.
- Waiting banner: drawn by kaze over the top of the terminal area (not into the PTY): "Waiting for Docker…" / "Waiting for port 8000…" / "Waiting for shell prompt", the line to run, and buttons **Use shell** (cancel) and, when NeedsConfirmation, **Run commands**. Failed shows the reason. The banner goes away when the tab launches or is cancelled.
- Input cancel: in `termEvents`, any key/edit/paste/mouse report to a member in Waiting or NeedsConfirmation sends `UserInputEvent` first, then forwards the input.
- Summary toast once all members settle: "samurai: started 4 of 4 tabs" or "started 3 of 4 tabs. api logs: port 8000 never answered."
- The window does not close on `mgr.Empty()` while a run is creating its first tab.

- [ ] Tests (pure): every transition in the state diagram; submit exactly once; UserInput then DepReady → no submit; Closed cancels.
- [ ] Tests (UI, fake probes and recording PTYs): run creates 4 tabs in order with their dirs and group; Docker-ready + prompt → each tab receives exactly its joined line + `\r`; typing first → nothing sent (Review focus 1); Run twice → one set (Review focus 2); port failure → summary "3 of 4"; closing the last member cancels the probe context (Review focus 4); no wake scheduled once all settled.
- [ ] Commit `feat(ui): Run a workspace into its own tabs`.

### Task 6: The tab group in the tab strip (variant A)

**Files:** modify `internal/ui/tabbar.go`, `internal/ui/window.go`, `internal/ui/logic.go` (next/prev through members); create `internal/ui/tabgroup.go`, `tabgroup_test.go`.

**Interfaces:** members of one run are drawn as one chip at the position of the first member: dot stack in member colors (alert ring overrides), name, `› member title` when a member is active, a count badge (amber when any member needs you, green when done), `▾`. `func (win *window) openGroupMenu(run uint64)`; the dropdown lists members (number, color dot, title, short folder, "needs you"/"done", ×), header "samurai · 4 tabs" and the run state; pick by click, `1`–`9`, `↑`/`↓`/`Enter`; `Esc` or click outside closes; theme tokens only. `next_tab`/`prev_tab` step through members in order. The chip has no ×; the dropdown row × closes that member. The group disappears with its last member.

- [ ] Tests: 2 ordinary tabs + 4 members → 3 strip items; chip label with and without an active member; dropdown pick by click and by `3`; Ctrl+Tab from the tab before the group enters member 1, then 2; alert on a member colors the badge; closing members via the row ×; last member closed → chip gone; theme switch with the menu open (Review focus 5).
- [ ] Commit `feat(ui): Show a running workspace as one tab group`.

### Task 7: Text fields for the settings kit

**Files:** modify `internal/ui/controls.go`, `internal/ui/settings.go`; create `internal/ui/textfield_test.go`.

**Interfaces:** a `ctlText` control kind backed by `gioui.org/widget.Editor` (single line, no IME requirement), styled with theme tokens (field bg, 1px border, accent border when focused), with a per-field error line in red under it. Focusing a field takes keyboard focus from the page's Tab order; `Esc` leaves the field; Tab / Shift+Tab move to the next/previous control; clipboard paste works. Value read/written through the row builder (`id` → string).

- [ ] Tests: typing into a focused field updates its value; Tab leaves it and focuses the next control; Esc returns focus to the page; paste inserts text; long text scrolls inside the field; terminal never receives keys while a field has focus.
- [ ] Commit `feat(ui): Add text fields to the settings controls`.

### Task 8: Settings > Workspaces and startup

**Files:** create `internal/ui/settings_workspaces.go`, `settings_workspaces_test.go`; modify `internal/ui/settings.go` (enable the entry), `internal/config/config.go` (`startup_workspace` string key, default ""), `cmd/kaze/main.go` (`--workspace <name>`), `internal/ui/window.go` (startup path).

**Behaviour:** list (name, state badge: Closed / Starting / Open / "2 of 4 tabs open" / Needs attention, tab count, member lines, buttons Run or Go to tabs, Edit, Delete with confirm dialog "Tabs that are running keep running."); "+ New workspace"; empty state "No workspaces yet". Draft editor per the mockup: name, tab cards (title, folder, commands with "+ Add command" and per-line remove, color swatches, Wait None / Docker / Port with port field), move up/down, remove, "+ Add tab", the joined line preview, Save (disabled while invalid; field errors shown), Cancel; leaving with unsaved changes asks Save / Discard / Keep editing; stale file → "The file changed on disk. Reload?" and the draft is kept. Startup row: "Run a workspace when kaze starts" None / names, saved as the ID with `config.Set`. Deleting the startup workspace clears the key. Startup: `--workspace` name, else `startup_workspace` ID, else home tab; a valid workspace replaces the home tab; unknown name / missing ID / invalid file → home tab plus a warning; `--workspace` without a name prints an error and exits 2.

- [ ] Tests: list states; Edit → Cancel leaves the file untouched; Save writes and Run uses the saved recipe; invalid field blocks Save; stale write refused with draft kept (Review focus 3); delete clears startup; startup precedence table; `--workspace` missing name exits 2 before any window.
- [ ] Commit `feat(ui): Add the Workspaces settings and startup workspace`.

### Task 9: Docs and finish

README "Workspaces" section (what a workspace is, the tab group, readiness and the shell snippet, the joined line, the file, startup and `--workspace`), HANDOFF and settings spec note, version 0.6 (`version.Number`, winres.json, Info.plist; regenerating the .syso needs the owner, so note it for the controller). Commit `docs: Document workspaces and tab groups`.
