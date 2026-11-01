# kaze handoff

Last updated: 2026-11-01, during the main-audit follow-up on `perf/audit-followup`. Read this first when you pick up kaze on another machine.

## Where things stand

**Audit follow-up is implemented** on `perf/audit-followup`, with the owner's
authorization to commit/push and open a PR, explicitly without merging. Subagent
text wrapping shapes once without paint recording; pane caches track content and
layout; reopened helpers reset their turn's steps/report/time. Git discovery runs
on at most two background workers with stale-result checks and a bounded path
spelling cache. Input reserves chunks and bytes before copying, includes the active
write, and reports overload to the user. The queue budget is 64 MiB + 4 KiB so one
maximum paste fits. PR test/vet/build CI is added, and the concurrent notification
test uses race-safe callbacks. See `docs/audits/2026-11-01-main-followup.md` and the
matching audit-followup design/plan. No install, version bump or release.

**macOS session shortcuts:** a native hidden-window probe reproduced AppKit
consuming Ctrl+Tab and Ctrl+Shift+Tab before Gio received them. The Mac view's
existing attachment now owns a window-scoped local key monitor that forwards only
those combinations to Gio's normal key path, once, retaining configured actions.
It detaches with the view/window. The native regression uses hidden, unfocused
windows and no global input; router tests cover both directions, wraparound,
workspace/scratch exclusion, Settings/board and overrides. Native CI runs the probe.


**Start folder (issue #77) shipped in 0.19** (PR #81). `start_dir` is an optional config
string; empty means home, `~` expands, invalid paths/types warn and use home. Settings >
Workspaces > Startup shows the path, Choose… and Use home, reusing the native folder picker.
Only new plain tabs use it; workspace/restored folders and startup precedence are unchanged.
Plain tabs record their folder at creation, before OSC 7. Windows picker runtime remains
unverified. See `docs/audits/2026-11-01-start-folder.md`. The subagent pane rework (#78,
PR #79) shipped in the same release.


**Session restoration (issue #75) is implemented** on `feat/session-restore`
(approved design and plan: `docs/superpowers/{specs,plans}/2026-11-01-kaze-session-restore*.md`).
Default-on `restore_sessions` has a Startup toggle in Settings > Workspaces, even when
there are no workspaces or their file is broken. `internal/sessionstate` saves bounded,
versioned metadata beside config.toml, with atomic replacement, an OS lifetime lock,
coalesced event-driven writes and a final pre-teardown flush. Background cwd and
origin-terminal hooks update it while minimized; scratch hooks cannot replace the
main identity. Restore maps names/order/selection and workspace grouping to fresh tabs,
queues exact UUID resume commands after shell readiness, and never replays prompts or
workspace recipes. Explicit --workspace wins; otherwise restoration precedes the
startup workspace. Ordinary extra instances neither restore nor write the owner snapshot.
Updater relaunch waits for the old owner to release its lock after shell teardown;
failed relaunch keeps ownership and restores input. Bench and ordinary test windows
never touch the owner's state. Merged in PR #76; the required 0.18 release updates the version, changelog,
lander and download notes. Installed-app update and Windows runtime checks remain open. See
`docs/audits/2026-11-01-session-restore.md` for verification and remaining platform checks.


**Performance fixes are built for PR #60** (2026-11-01), alongside its sidebar
flash, finished-subagent and long-command fixes. Title/status geometry is cached;
width measurement avoids paint recording; full histories and dirty rows reuse
storage; forgotten pane IDs are pruned; scrolling rotates row references; the
subagent inbox compacts obsolete tools in batches while preserving lifecycle
states. The matched applied-log benchmarks improve from 33.9 to 72.8 MB/s at
120x40 and 8.23 to about 52 MB/s at 240x80 on an Apple M3. These are CPU
microbenchmarks, not native latency results. Full tests, vet, focused races and
macOS/Windows amd64/arm64 builds pass; Windows runtime behavior is not manually
verified. See `docs/audits/2026-11-01-performance.md` and the performance-fixes plan.
No version bump until the release workflow after merge.

## Implementation history

The entries below describe the state at each milestone. Their branch/version and
setup notes are historical; use the current status above and Next below for work.

**Plan 1 (terminal core with tabs) is done**, reviewed task by task plus a whole-branch review, and on `main`.

**Plan 3 (the window frame) is built**: Plan 3a (kaze's own title bar) and Plan 3b (alerts, chime, folder and branch), plans in `docs/superpowers/plans/`. What it added:

- **Own title bar on Windows:** tabs at the top edge, kaze-drawn minimize / maximize-restore / close (close turns red on hover). The empty bar space moves the window (native drag, double-click maximize, edge snap, Win+Z), and at least 48dp of it always stays free however many tabs are open; the 7dp strip above the tabs also moves the window. **No Windows 11 snap-layout flyout** on hover over maximize: Gio has no way to answer `HTMAXBUTTON`. **macOS stays decorated** (the native title bar): an undecorated Gio window loses the traffic lights, so the inset look waits for the Mac milestone.
- **Alerts from Claude Code hooks:** `kaze notify needs|done` sends the tab's id to kaze over **TCP on 127.0.0.1 with a per-run token** (`KAZE_NOTIFY`, `KAZE_NOTIFY_TOKEN`), not named pipes or sockets: no dependency, same code on both OSes. The tab gets a **static** dot (amber = needs you, green = done; no pulse, which would cost idle CPU) and a toast with "Go ›". `notify` always exits 0 within about 1s. It reads `notification_type` from the hook JSON: `idle_prompt` and `auth_success` are dropped, anything else (or no type) is "needs".
- **Chime:** bundled soft chime on "needs you" (`chime_needs`, on), optional on "done" (`chime_done`, off), custom WAV via `chime_file`. No chime when that tab is already active and focused; bursts give one chime. Plays while minimized too.
- **Taskbar flash on Windows only** (FlashWindowEx) when an alert arrives while kaze is minimized or unfocused. The macOS Dock bounce is deferred to the Mac milestone.
- **Folder and branch in the status bar** from OSC 7 (pwsh and zsh snippets in the README) and `.git/HEAD` (worktrees handled, no git process). The branch is re-read on a new prompt report, on tab switch and on window focus gain.
- **Installed on the owner's Windows laptop:** the Claude Code hooks in `~/.claude/settings.json` (Notification with matcher `permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input` -> `kaze.exe notify needs`, Stop -> `kaze.exe notify done`, timeout 3) and the OSC 7 snippet in the pwsh profile. After each rebuild of `kaze-app\kaze.exe`, run `kaze.exe notify done` once so Smart App Control vets the new exe before a hook's 3s timeout.

**Plan 4 (Claude usage) is built** on `feature/plan-4-usage`:

- `kaze statusline` is Claude Code's status line command. It prints `5h 23%  ·  wk 41%  ·  ctx 34%` for Claude's own status row and, inside a kaze tab, sends the numbers to kaze as a `usage` message on the alert listener. Usage messages never chime, flash or mark a tab, and travel apart from the alert inbox so they cannot replace a pending alert.
- The status bar shows them right-aligned before the memory text. Plan limits are account-wide (latest from any tab); context is per tab and hides once the tab's shell reports a new prompt (OSC 7), meaning Claude exited. Amber at 80%, red at 95%, with the reset time. A window past its reset disappears via one timed wake, no ticker.
- **Owner's scope (2026-10-29):** 5-hour, weekly and context only. No per-model (Fable) number: Claude Code's status line data has none, and the only other source is an undocumented endpoint behind the login token, which he declined. Cost and the process tree are left out.
- **Installed on the owner's laptop:** `statusLine` -> `kaze.exe statusline` in `~/.claude/settings.json`. The old orca status line and its 13 hooks were removed on 2026-10-29 (backups `settings.json.bak-20261029-*`).

**Plan 5a (settings page) is built** on `feature/settings-page` (spec `docs/superpowers/specs/2026-10-29-kaze-settings-page-design.md`, plan `docs/superpowers/plans/2026-10-29-kaze-plan-5a-settings.md`):

- **Settings page** opens from the gear (title bar on Windows, right end of the tab strip on macOS, where the system title bar stays) or Ctrl+, / Cmd+, (`open_settings` under `[keys]`), as a chip beside the tabs. Sections: Appearance (theme, font size, line spacing, cursor, blink) and Claude (chimes, taskbar flash, done-toast time, usage readout, Claude Code setup check). Workspaces is greyed out until Plan 5b.
- **Five themes** in `internal/theme`: Midnight (default), Sumi, Tide, Washi (light), Sakura. A theme sets the terminal palette and the chrome colors; `[colors]` still overrides the palette.
- **Config writes** (`internal/config/write.go`): one key is edited in place, other lines and comments kept. New keys: `theme`, `line_spacing`, `cursor_style`, `cursor_blink`, `taskbar_flash`, `done_toast`, `show_usage`, plus `open_settings` under `[keys]`.
- **Claude setup check** (`internal/claudesetup`): inspects the Stop hook, the Notification hook and the status line in `~/.claude/settings.json` (OK / missing / broken / taken). Fix and Replace only after a confirm dialog, with a timestamped `.bak` copy first; other entries are never touched.
- **`kaze --version`** (`internal/version`) prints the version and build commit without opening a window.
- Not done: the Windows app icon, the notification card redesign.

**The settings redesign is built** on `feat/settings-redesign` (mockup `docs/design/settings-redesign-mockup.html`, approved without its gradients; plan `docs/superpowers/plans/2026-10-31-kaze-settings-redesign.md`):

- **UI font:** Manrope Regular/Medium/SemiBold in `assets/fonts` (static instances cut from the OFL variable font, because Gio draws a variable font only at its default ExtraLight). `chrome.sans`, `page.textW`, `page.mono`; JetBrains Mono stays for keys, paths, commands and versions.
- **Frame** (`settings.go`, `settings_style.go`): menu with title, icons (`chrome.icon`), a "Kaze" group, badges and the update state in the footer; no "Later" line. Rows with `heading` start a card (`newCard` starts one without a label); rows can carry `icon`, `pill`/`tone` and `monoHint`; hints and lines wrap. Controls restyled; buttons can be `ghost`.
- **Pages:** Appearance (theme cards with previews, live preview, cursor chips), Workspaces (a card per workspace with a state pill), Claude (setup check first, with Connected/Missing/Broken/Taken pills), Shortcuts (was Commands: grouped key caps, search), Updates (new; the updater moved out of About), About (lockup, version pill, facts row, Report an issue).
- **Tests** click like a person: `pressAny` scrolls a control into view first, because the roomier pages scroll.

**Agents settings (issue #31) is built** on `feat/agents-settings` (design and plan
`docs/superpowers/{specs,plans}/2026-10-31-kaze-agents-settings*.md`):

- Settings **Agents** replaces Claude: shared alerts, Claude-only usage, then Claude Code and
  Codex setup cards. Existing config keys and installed Claude hook commands still work.
- `kaze notify <needs|done|prompt|interrupt> --agent codex` identifies Codex; no agent flag keeps
  Claude behavior. Codex Stop prints `{}` even outside Kaze. Other Codex commands are silent.
  Codex slash commands and `$skill` names are bounded and sanitized like Claude commands.
- Sidebar and palette sessions identify the agent. Interrupt ends running with no alert, chime
  or flash. The next shell prompt clears the agent. Claude's palette skill catalog is unchanged.
- `internal/codexsetup` checks hooks.json and read-only inline hooks. Fix confirms first,
  preserves other handlers, backs up existing files, checks for stale sources, and writes only
  hooks.json. Inline problems or duplicate Kaze handlers need manual repair. Codex's notify,
  config.toml and hook trust are never written. After Fix, the user must review `/hooks` in Codex;
  the dismissible reminder lasts until restart. The page says Configured, not Connected.
- The ordered JSON editor is shared with claudesetup; Claude's repair and Replace tests remain.
- Verified the prerequisite with the real Codex CLI 0.166.0 in a real Kaze PTY, isolated HOME,
  temporary input-blocked overlay and local Responses stub: env inheritance, loopback delivery,
  prompt delivery, Stop `{}`, one completed turn, and Interrupt on a probe-only SIGINT. No account credentials or external model calls were used.
- Baseline `go test ./...` and vet passed. Baseline UI race testing reproduces the already-known
  race in the fake chime/flash counters of TestOnNotifyNeverBlocksAndStaysBounded. Windows hooks
  remain unverified by hand. No global input was injected and no owner settings were changed.

**Downloads and updates are built** (spec `docs/superpowers/specs/2026-10-31-kaze-updates-design.md`, plan `docs/superpowers/plans/2026-10-31-kaze-updates.md`). Version 0.11:

- **Releases** (`.github/workflows/release.yml`, PR #35): pushing tag `v` + `version.Number` builds the universal Mac app (`KAZE_UNIVERSAL=1 ./tools/package-macos.sh`) and the Windows x64/ARM exes, and publishes them with `SHA256SUMS` and the first-launch notes in `.github/release-notes.md`. Unsigned. The lander's Download buttons point at `releases/latest/download/<zip>`.
- **Updater** (`internal/update`, `internal/ui/updates.go`): checks `releases/latest` 10 s after start and every 24 h (`check_updates`, on). Status bar "Update X" opens Settings, Updates (its own page since the settings redesign); it has Update and restart (asks first), What's new, Check now and the switch. The install downloads the zip and `SHA256SUMS` next to the installed app, verifies, unzips (no `..`, absolute paths or links), renames the old `Kaze.app`/`kaze.exe` aside (`Kaze.app.old`, `kaze-old.exe`, removed on the next start), moves the new one in (and back on failure), relaunches (`open -n` / the exe) and quits.
- **Source builds are never replaced:** a `.git` in the app's or exe's folder or its parent (a clone's root or `dist/`) means Download (release page) only.
- **Test windows never reach the network:** the updater is idle unless `initUpdates` (called from `Run`) set its URL; tests point it at `httptest` servers.
- **Not tested for real yet:** an actual update needs two releases with the updater (0.11, then 0.11.1 or 0.12). Check on the Mac and on Windows that the relaunch comes up and the old copy is removed.

**Plan 6d (floating panes and the scratch shell) is built** on `feat/floating-panes` (plan `docs/superpowers/plans/2026-10-30-kaze-plan-6d-floating-panes.md`, spec sections 7, 8, 11). Version 0.10:

- **Floating panes** (`internal/ui/panes*.go`): a workspace member opens over the shown session from its sidebar line, the server dots or the palette (`floatMember`). Drag the header, resize from edges and corners (6 dp bands, 14 dp corners, 280x140 dp minimum), click to raise and take the keyboard, Expand to make it the main view, × to hide it (the member keeps running). Up to 4 panes per session, the oldest member replaced first; one session at a time per member. Geometry is kept in dp relative to the terminal area and fitted to it every frame.
- **Own renderer and size per pane:** each visible pane has its own `render.Renderer` (a renderer keeps one tab's strips and dirty rows) and its own PTY size (`tabs.Manager.ResizeTab`, `Dock`; `ResizeAll` skips floated tabs). `SetVisible` makes a pane's output ask for frames. Hidden panes and other sessions' panes cost nothing.
- **Input through views** (`termView`): the session and every pane use the same key, paste, mouse and selection code. Esc goes to the program in a pane. Focus reports move with the keyboard.
- **Scratch shell** (`internal/ui/scratch.go`): `scratch_shell` (Ctrl+Shift+` / Ctrl+`), a title bar button on Windows, a sidebar and rail button on macOS. A tab with `Tab.Parent` set: hidden from the sidebar and next/prev, started in the session's folder, kept running while hidden, replaced if it exited, closed with its session. Its hook messages count on the parent row (`hookTab`); usage stays on the scratch tab.
- **To check on the Mac by hand:** the Ctrl+` key name and the scratch button in the rail. **To do by the owner:** the go-winres rerun so the exe details say 0.10.
- **Known failing before 6d** (also on `main` at 61a9229, macOS): the four `TestCount*` line-count tests, `TestRenameSurvivesClaudeLeaving`, and a data race reported by `go test -race` in `TestOnNotifyNeverBlocksAndStaysBounded`.

**Plan 6e (the palette) is built** on `feat/palette` (plan `docs/superpowers/plans/2026-10-30-kaze-plan-6e-palette.md`, spec sections 9.3 and 10), on top of 6b. Version 0.9:

- **Palette** (`internal/ui/palette*.go`): `palette` key (Ctrl+Shift+K / Cmd+K), the title bar search box on Windows, a magnifier next to the sidebar toggle (and in the rail) on macOS. Sections Waiting on you, Sessions, Skills, Kaze; contains-anywhere matching with name-prefix matches first; the list is rebuilt from live state each frame while open, the selection follows its item. No ticker: hover redraws only when the row under the pointer changes.
- **Skill catalog** (`internal/skills`): user and project `skills/*/SKILL.md` and `commands/**/*.md`, enabled plugins from `installed_plugins.json` (newest version, else the highest cache folder). Frontmatter read without a YAML library, 32 KB per file, 1000 entries, read only, on a goroutine when the palette opens, cached by modification time and size. A plugin's nested command keeps its folder in the name (`plugin:folder:name`), as Claude Code names it.
- **Starting a skill** (`internal/ui/prompt_start.go`, spec 9.3): typed into a Claude session in front (no Enter), else a new session in the same folder types `claude '<skill>'` on its first prompt marker; after 3s the workspace "Waiting for shell prompt" banner asks. `workspaces.QuoteArg` quotes for POSIX and PowerShell; other shells get a notice.
- **Settings, Commands:** every shortcut as bound on this machine, the mouse gestures, and the command line with Copy buttons. The nav's Later line is now "Terminal, Shell setup".
- **Not in 6e:** chips and `kaze status` (6c, parked); changing shortcuts from Settings.
- **To do by the owner:** rerun the go-winres command in the README so the exe details say 0.9. The Windows search box is untested by hand (built and tested on the Mac).
- **Known failing on macOS before 6e** (also on `main` at 7822b23): the four line-count tests (`TestCount*`) and `TestRenameSurvivesClaudeLeaving` in `internal/ui`.

**Workspaces UI round 2 is built** on `feat/workspaces-ui` (spec `docs/superpowers/specs/2026-10-30-kaze-workspaces-ui-design.md`, plan `docs/superpowers/plans/2026-10-30-kaze-workspaces-ui.md`): a two-pane workspace editor (tab list left, selected tab right, next unused color, red problem marks), a native folder picker (`internal/pick`: AppleScript `choose folder` on macOS, PowerShell `FolderBrowserDialog` on Windows; Windows not yet tried by hand), a list that shows each tab's color, folder and full command chain, a group dropdown that shows what each member runs and its status (Finished = a prompt report after the line was typed), and About / sidebar without the build line. Still owed: the Windows picker check on the laptop.

**Plan 6b (agent awareness) is built** on `feat/agent-awareness` (plan `docs/superpowers/plans/2026-10-30-kaze-plan-6b-agent-awareness.md`, spec `docs/superpowers/specs/2026-10-30-kaze-agent-sidebar-design.md`). Version 0.8:

- **Prompt hook:** `kaze notify prompt` runs from a Claude Code `UserPromptSubmit` hook. It sends the leading slash command and its first line of arguments, prints nothing, always exits 0, and cleans and caps the text (`internal/notify`, `cmd/kaze/notify.go`). Settings, Claude, Claude Code setup shows it as "Command each session runs"; Fix adds or repoints it with the usual show-the-change, confirm and back-up steps (`internal/claudesetup`).
- **Per-session state** (`internal/ui/agent.go`): running starts on a prompt and ends on done or needs. When the shell prompt comes back, Claude has left and the row is a plain shell. Esc interrupts fire no hook, so an interrupted row stays running until the next message (known limit).
- **Rows:** ` · /command` in magenta, a still `running` ring (no ticker, idle CPU stays zero; `needs you` and `done` win over it). Title rule: your name, then the command's first argument, then the folder name, then the shell title. Double-click a title to rename (Enter saves, Esc cancels, blank goes back to the rule; workspace members can't be renamed). The filter matches the command.
- **Line counts:** `+N -M` from `git diff --shortstat HEAD` (`internal/gitstat`, `internal/ui/linecount.go`), run only on events, one at a time per tab, no console window, 2s timeout, hidden outside a repo.
- **Next waiting:** `next_waiting` (Ctrl+Shift+J on Windows, Cmd+J on macOS, changeable under `[keys]`) and a "Next waiting" button at the bottom of the sidebar. With nothing waiting the status bar says "Nothing is waiting on you."
- **To do by the owner:** press **Fix** once in Settings, Claude, Claude Code setup to add the prompt hook (the installed settings.json predates it); rerun the go-winres command in the README so the exe details say 0.8 (the `.syso` files still carry older versions); then run `kaze.exe notify done` once after the rebuild for Smart App Control. The macOS look of the new rows and the Next waiting key are unchecked on a Mac.
- **What is next:** Plan 6d (floating panes and the scratch shell, `docs/superpowers/plans/2026-10-30-kaze-plan-6d-floating-panes.md`) and Plan 6e (the palette, `docs/superpowers/plans/2026-10-30-kaze-plan-6e-palette.md`). 6e is now built (above). **Plan 6c (`kaze status`, chips, timed checks) stays parked.**

**Plan 6a (the sidebar) is built** on `feat/agent-sidebar` (spec `docs/superpowers/specs/2026-10-30-kaze-agent-sidebar-design.md`). Version 0.7:

- **A left sidebar replaces the tab strip.** Sessions are grouped by git repo (worktrees count as the same repo), groups fold, rows show title, short folder, branch and `needs you` / `done`. Hover gives a ×, middle click closes, `+` opens a session. Settings is a pinned row at the top while open. A "Filter sessions…" field narrows by title, folder and branch (Esc clears, Enter shows the first match, folded groups open while filtering).
- **Workspaces section** at the bottom: a folding header per running workspace ("all up" / "N down"), one line per member (color bar, title, status, `stopped` in red). Clicking a member shows it as the main view. Running a workspace keeps the current session in front. The status bar has one dot per running member plus "servers"; clicking shows the down member.
- **Hiding:** `toggle_sidebar` (Ctrl+Shift+B / Cmd+B) saves `sidebar = true|false`; under 560 dp the sidebar hides without saving. The Windows title bar holds the toggle, K mark, gear and window buttons; macOS keeps the native title bar, with the toggle and gear at the top of the sidebar and a thin rail when hidden.
- **Removed:** the tab strip, the workspace group chip and dropdown, the Settings chip. Ctrl+Tab and Ctrl+Shift+Tab walk sessions in sidebar order and skip workspace members.
- **Plans 6b to 6e were next**, as in spec section 15 (6b is now built, see above): 6b agent awareness, 6c `kaze status`, chips and timed checks (**parked** by the owner, 2026-10-30), 6d floating panes and the scratch shell, 6e the palette, the skill catalog and the Commands settings section.
- **Still open from 6a:** the go-winres rerun is now for 0.8 (above). The macOS look of the sidebar (rail, toggle at the top of the sidebar) is still unchecked on a Mac.

**Plan 5b (Workspaces) is built** on `feature/workspaces` (decisions `docs/superpowers/specs/2026-10-29-kaze-workspaces-decisions.md`, mockups in `docs/design/`). Version 0.6:

- **Workspaces** are saved launch recipes (name plus ordered tabs: title, folder, commands, color, wait for nothing / Docker / a port) kept in `workspaces.toml` beside `config.toml` (`internal/workspaces`). kaze owns the layout; a hand-edited file is checked strictly and never overwritten when invalid. Edited in Settings > Workspaces with a Save / Cancel draft; Run, or Go to tabs when members are already open.
- **A run is one tab group** in the strip (chip with member dots, name and count, dropdown, alerts on the chip, Ctrl+Tab steps through members), not one tab per member. Real panes are a later plan and would replace groups.
- **Commands** are typed as one joined line (`; if ($?) { }` in PowerShell, `&&` in zsh/bash/sh) once the first prompt marker (OSC 7 or OSC 133 A) arrives; after 3s without one a banner offers Run commands / Use shell. Typing in a waiting tab cancels its commands. Docker (started once if needed) and port waits give up after 3 minutes.
- **Startup:** `startup_workspace` (a workspace ID) and `kaze --workspace "<name>"`, which wins.
- **To do by the owner:** the `.syso` files in `cmd/kaze` still carry version 0.5; regenerate with the go-winres command in the README. The macOS run of the shell-readiness and one-line tests is still owed. The `samurai` workspace is created only on his word.
- Known limits: no folder Browse button, commands can not be reordered, closing the window drops an unsaved draft.

What works (tested on Windows):

- Tabs (Ctrl+Shift+T / W, Ctrl+Tab), each with its own shell (pwsh on Windows, zsh on macOS) and `KAZE_TAB_ID`.
- Full xterm-style emulation for Claude Code, git, vim, less: colors (256 + truecolor), alt screen, scroll regions, bracketed paste, mouse modes, focus events (1004), synchronized output (2026).
- Fast renderer: CPU-drawn grid in 8 strips, only changed rows redrawn, nothing drawn when idle.
- Japanese: wide characters, bundled Noto Sans JP subset, system font fallback for rare kanji.
- Symbols Claude Code prints (`⏺ ⎿ ✻`, spinners, emoji in one color) via system symbol fonts. Box-drawing is drawn by kaze so borders join.
- Readable text on colored backgrounds (minimum-contrast nudge).
- Mouse selection (drag, double-click word, triple-click line, Shift+drag in mouse-mode apps), Ctrl+Shift+C / V, optional `copy_on_select`.
- Status bar: kaze's private memory and tab count.
- Config file with warnings for bad values.

Measured (Windows, `docs/benchmarks/2026-10-28-milestone-1.md`):

| Goal | Result | Met |
|---|---|---|
| Claude Code streaming frames < 16.7ms | p99 5.1ms | yes |
| Smooth scroll through 10,000 lines | no late frames | yes |
| Idle CPU ~0 | 0.0% with 4 tabs | yes |
| Memory doesn't grow when idle | ~147MB, flat; +0.5MB per idle tab, ~+30MB per tab with full history | yes |
| Keypress to glyph < 10ms | ~25ms typical; kaze's own part ~8ms p50, ~24ms p99 | **no** |
| Launch to prompt < 500ms | ~1.3s with pwsh (kaze's first frame ~0.45s) | **no** |

**Window chrome redesign (feat/window-chrome):** one Kaze-drawn 42dp title bar on macOS and Windows (spec `docs/superpowers/specs/2026-10-31-kaze-window-chrome-design.md`) and the direction-A status bar (32dp). macOS is now frameless: `internal/ui/maclights_darwin.go` shows the native traffic lights again and keeps them centred; its callbacks never call Gio inline (that deadlocked once). Native double-click on empty bar space zooms or minimizes per `AppleActionOnDoubleClick`. Owner checked the Mac behaviour by hand on 2026-10-31. The Windows build compiles (amd64, arm64) but has not been run since the redesign: try it on the Windows laptop. Every lander capture still shows the old chrome.

## Next

1. Review the audit follow-up branch. The owner's existing release hold in
   `CLAUDE.md` remains in force; do not publish from this task.
2. Verify installed-app update/relaunch and current window chrome, hooks and native
   folder pickers on Windows. A cross-build does not verify runtime behavior.
3. Follow up the existing combining-mark/NFD and Option-as-Alt gaps separately.
4. Re-run native keypress/first-frame benchmarks before claiming end-to-end gains.
   Incremental link scanning is lower priority than these measured fixes.
5. Subagent in-between messages require a transcript reader; hooks only provide
   the prompt, tool activity and final report. Other future ideas remain in specs.

## Mac milestone checklist

Native macOS builds, tests and owner chrome checks are complete. This original
setup/checklist is historical; unresolved platform checks are listed above.

1. Install Go (version in `go.mod`) and the Xcode Command Line Tools: `xcode-select --install`.
2. Clone and build: `git clone https://github.com/patrick-gogo/kaze-terminal && cd kaze-terminal && go build -o kaze ./cmd/kaze && ./kaze`.
3. Run the tests: `go test ./...` and `go vet ./...`. Native macOS tests now run locally; PR CI adds macOS and Windows checks.
4. Check by hand, in a kaze tab:
   - zsh starts; `echo $TERM $COLORTERM $TERM_PROGRAM` prints `xterm-256color truecolor kaze`.
   - `claude` renders with colors, its input box borders join, symbols show.
   - Japanese file names: macOS stores names in NFD, and kaze currently **drops combining marks**, so `ls` of Japanese names may lose dakuten (が → か). Known gap, fix it here.
   - Rare kanji draw (Hiragino fallback paths in `internal/render/fontpaths_darwin.go` are unverified).
   - Cmd+T / Cmd+W / Cmd+C / Cmd+V work.
   - **Option key:** kaze treats Option as Alt unconditionally. On layouts that type `@ { | [ ]` with Option this breaks; add a config toggle.
   - Trackpad/mouse wheel: scroll speed may be off (unit not verified on macOS).
   - macOS status memory now uses `proc_pid_rusage` physical footprint, including native allocations.
5. Plan 3 on macOS:
   - **Hooks setup:** add the two Claude Code hooks from the README ("Alerts") to `~/.claude/settings.json` on the Mac, with the Mac path to `kaze` (`"/path/to/kaze" notify needs` / `notify done`, timeout 3). Check that a permission prompt in a kaze tab marks it and chimes (`afplay`).
   - **zsh snippet:** add the OSC 7 snippet from the README to `~/.zshrc` and check folder and branch show, including a folder with spaces, `#`, `%` and Japanese. Unlike the pwsh one, the zsh snippet only escapes `%`, `#` and space and sends other characters as raw UTF-8 (kaze accepts that); it could not be run on Windows, so verify it here.
   - **Dock bounce:** alerts while kaze is in the background should bounce the Dock icon (the Windows taskbar flash has no macOS twin yet; `flashTaskbar` is a no-op off Windows).
   - **Title bar:** the native traffic lights are restored on the custom frame; owner checked the Mac behavior on 2026-10-31.
6. Benchmarks: `bench/run.sh` (it uses a history-free shell and kaze drops real input in bench mode, so it can't type into your sessions). Add a "macOS" column to the milestone doc.

## Decisions so far (why the code looks the way it does)

- **Go + Gio**, one codebase for Windows and macOS. Beating Warp is not a goal; "fast, not laggy, fits the workflow" is. Memory has no hard cap but must not grow when idle.
- **Renderer:** a CPU-drawn image of the grid, split into 8 horizontal strips; only strips with changed rows are re-uploaded. Chosen by two spikes (`docs/spike-results.md`): Gio's text path and per-glyph ops were far too slow.
- **Synchronized output:** frames are held while mode 2026 is on, up to 150ms, but never when nothing has been drawn yet (tab switch).
- **Parser** treats `:` like `;` but records which params were colon-separated, so SGR `4:3` (curly underline) and `38:2::r:g:b` work.
- **Live processes stop when Kaze closes.** Shells live in the Kaze process behind `ptyx.Session`. Issue #75 supersedes the earlier no-restoration decision: folders and exact supported agent conversations now reopen automatically from saved metadata; this does not keep PTYs alive.
- **Shell env:** each tab gets `KAZE_TAB_ID`, `TERM=xterm-256color`, `COLORTERM=truecolor`, `TERM_PROGRAM=kaze`; inherited `TERM_PROGRAM`/`WT_*` from the launching terminal are dropped.
- **Title bar:** custom chrome on both platforms, with restored native traffic lights on macOS (owner checked 2026-10-31).
- **Alert transport:** TCP 127.0.0.1 + random token (spec 6.4 note), not named pipes.
- **No pulsing dot, no idle reminders** (spec 6.8 note): a static dot keeps idle CPU at zero; `idle_prompt` and `auth_success` notifications are dropped.
- **Copy is Ctrl+Shift+C** (Ctrl+C must stay "interrupt"). Smart Ctrl+C is a later idea, not wanted yet.
- **Ctrl+Backspace** sends 0x08 (word delete in PSReadLine).
- **Claude Code features left unanswered** (kitty keyboard protocol, XTVERSION, some OSCs): Claude falls back fine. Side effect: Shift+Enter submits; use Alt+Enter for a newline.
- **Fonts:** JetBrains Mono + Noto Sans JP subset bundled (OFL); missing symbols and rare kanji come from system fonts, loaded lazily, memoized, malformed fonts skipped.
- **Low-contrast text** is nudged just enough to reach 4.5:1, not forced to white (keeps dim text dim).
- **Benchmarks** measure keypress latency inside kaze (`KAZE_BENCH`), never with global input.

## Known gaps (deferred, not blocking)

- Combining marks dropped (NFD on macOS, see checklist).
- macOS: Option-as-Alt toggle, wheel units and rare-kanji fallback verification. Memory reporting uses physical footprint.
- Scrollback uses packed 16-byte cells, trims default blank tails and reuses evicted lines; retained memory still depends on line widths and contents.
- Ctrl+Alt+letter is dropped on a US Windows layout.
- Status bar lacks CPU and the per-tab process tree (spec 6.5; left out of Plan 4 by the owner).
- `font_family` is accepted but not supported yet (kaze warns).
- Glyph cache is safe only with one window / one UI goroutine (add a mutex before multi-window).
- Closing a tab whose ConPTY close times out can keep that screen in memory until ConPTY lets go.
- Smaller items: the full later-fix backlog is below, and every "minor (deferred)" line is in `docs/history/plan-1-ledger.md`.

## Where kaze came from

Patrick (the owner) started kaze on 2026-10-27 after seeing **sir Sat's** terminal. sir Sat is Patrick's senior/lead at work; his own terminal is written in **Swift** (likely macOS-only), very lightweight, memory-lean and fitted to his workflow. His key idea, which kaze copies: **only redraw what changed** (dirty tracking). sir Sat's terminal also has a **browser button** that opens a separate Chromium window; that's kaze's "browser button" later feature. Swift was ruled out for kaze because Swift on Windows has no usable UI toolkit; Go + Gio was chosen instead.

Patrick's daily terminal before kaze was **Warp** (Windows and Mac). He uses it like a plain terminal; the one Warp feature he relies on is the pane that says a tab's Claude session is done or needs him (that's Plan 3's alerts). He runs 3-4 tabs at once, flips between them, and wants side-by-side only for related things (a lead agent and its subagents: Plan 2). He doesn't speak or type Japanese, but his work output is full of it, so Japanese must display correctly (no IME needed).

He also considered Zellij; the answer was: it runs inside kaze, but kaze's own tabs are needed for the per-tab Claude features.

## Working with the owner

- **Plain language, short.** TL;DR first. No jargon; explain terms the first time. Use tables for status.
- **Recommend, don't survey.** When there's a choice, give one recommendation and why. End decision questions with the recommendation.
- **One question at a time, with context first** ("here's what I found, so: X or Y?").
- **No em dashes** in anything written for him. 12-hour times. Translate any Japanese you quote.
- **"how far along are we" / "update?" = report only**: re-check live state (git log, status) before any summary, show a progress bar plus a table, never stop or restart work to answer.
- **Brainstorm before building** anything new; he approves the design, then the plan. Once approved, **don't stop for check-ins**: run to the end, stop only for blockers, irreversible actions, or real decisions.
- **Never commit or push unless asked**; never push to `main` (his git hook blocks it: branch + PR, he merges). **Never AI attribution** in commits or PRs.
- He was fine with parallel agents ("can some tasks run in parallel?") when tasks don't share files: use separate git worktrees and merge when the main checkout is idle.
- **Smart App Control stays ON** on the Windows laptop (turning it off is irreversible). Live with the occasional "Part of this app has been blocked" pop-up during builds; rebuild with a new build id when an exe is blocked.

## The Windows laptop setup (for reference)

- Repo: `C:\Users\john\Desktop\kaze`. Go 1.27 installed in `%LOCALAPPDATA%\Programs\go` (zip install; winget/go.dev downloads failed, `dl.google.com` worked).
- His daily build: `C:\Users\john\Desktop\kaze-app\kaze.exe` (built with `-H=windowsgui`, test-launched until Smart App Control allowed it).
- pwsh profile: `C:\Users\john\OneDrive\Documents\PowerShell\profile.ps1` (has the PSReadLine lines below).
- Test runs used `GOTMPDIR` under the Desktop because App Control blocks test binaries in the default temp dir.
- Backup of the pre-rewrite history: `C:\Users\john\Desktop\kaze-backup\kaze-before-rewrite-2026-10-28.bundle`.

## Incidents in Plan 1 (and the rules they produced)

1. **OOM crash.** A RED test for "clamp 65535x65535 to 4096" ran before the clamp existed and allocated billions of cells, crashing the machine and the session. Rule: RED tests must fail safely (bounded sizes, pure-function tests for limits).
2. **Stray keystrokes.** Two GUI-testing agents used global SendKeys/SendInput while kaze wasn't the foreground window; `cls` and `echo hello-kaze-selection second-word` landed in Patrick's live Claude Code window. Later, benchmark windows grabbed focus and ~11 of his real keystrokes went into a bench shell, and 3 `flood.exe` lines landed in his pwsh history (not yet removed; ask him). Rules: never inject global input; bench mode drops real input by default; bench shells don't save history; launch without activating.
3. **Smart App Control** blocks fresh unsigned binaries at random. Not an incident, a fact of life on that laptop.

## Later-fix backlog (from the final whole-branch review)

Remaining Mac checks: Option-as-Alt toggle; NFD combining marks; rare-kanji fallback paths; wheel units. Physical-footprint memory reporting is implemented.

Later:
- A few remaining screen/vt test gaps (combining-mark escape, wide char while wrap pending, CAN/SUB inside OSC/DCS, split ST, truncated colon groups, colon-mask reset).
- UTF-8 second-byte ranges (E0/ED/F0/F4) not validated (harmless: renders a replacement glyph).
- ptyx: 3s wait-test limit could flake on slow CI; small Resize-vs-Close race window; Read/Write after Close reach the closed pty.
- Multi-param `CSI ... T` treated as scroll-down.
- Cmd+nav/Enter still encode on macOS.
- Ctrl+Alt+letter dropped on US Windows layout.
- Hidden tabs resize when the grid size actually changes; the unchanged-size guard is implemented.
- Input overload is rejected whole with a status notice; the queue is bounded by chunks and bytes, including the active write.
- Glyph cache unguarded across UI goroutines (fine with one window); double-line box junction overlap. Shared font-size caches retain only the two most recent sizes.
- Selection uses retained dirty-row scratch. Ideographic space U+3000 is not a word stop (matters for Japanese double-click).
- Full-history memory varies with retained line widths; cells are packed to 16 bytes and blank tails are trimmed.
- Shift+Enter submits in Claude Code (needs the kitty keyboard protocol); Alt+Enter works.
- A ConPTY close timeout keeps that tab's screen alive until ConPTY releases.
- kaze's own keypress p99 (~24ms) and first frame (~0.45s).

## If this repo goes public

History was already scrubbed once (before the first push). Personal/work context was re-added afterwards in `docs/HANDOFF.md`, `docs/history/` and `CLAUDE.md`. Before going public again: remove or neutralize those (names, employer details, ticket keys like `V3-...`, local paths, the ledger), and check `git log --all -p` with a grep, because history keeps old versions. The scrub script used last time replaced terms with `git filter-repo` on text blobs only (fonts untouched) and moved authors to `283561876+patrick-gogo@users.noreply.github.com`.

## Optional shell setup

Autocomplete in pwsh (history suggestions + Tab menu) is shell-level, not kaze. In `$PROFILE`:

```powershell
if ($Host.Name -eq 'ConsoleHost' -and -not [Console]::IsOutputRedirected -and (Get-Module -ListAvailable PSReadLine)) {
    Set-PSReadLineOption -PredictionSource History -PredictionViewStyle ListView
    Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete
}
```

macOS equivalent: the `zsh-autosuggestions` plugin plus zsh's own `compinit` menu; the steps are in the README ("Optional shell setup", zsh).
