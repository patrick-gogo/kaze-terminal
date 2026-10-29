# kaze: design

Date: 2026-10-28
Owner: Patrick
Status: approved by Patrick 2026-10-28

## 1. What this is and why

kaze is a personal terminal emulator for Windows and macOS. Patrick uses it like a normal terminal, but it is built around how he works with Claude Code: several sessions open at once, lead agents spawning subagents, and needing to know when a session in another tab has finished or needs him.

It is inspired by a teammate's own terminal, which is small, fast and fitted to one person's workflow. kaze follows the same idea: it only carries what Patrick uses, so it can stay lighter and faster than a general-purpose terminal.

Today Patrick uses **Warp** on both Windows and macOS. He uses it like a plain terminal. The one Warp feature he values is the alert when a Claude Code session in a tab is done or needs him.

## 2. Goals

1. Feel instant. Never laggy. This beats every other goal.
2. Run Claude Code all day without visual glitches.
3. Show what agents are doing: subagents visible and readable live, attached to the lead that spawned them.
4. Tell Patrick when a tab needs him (done, question, permission prompt).
5. Show usage: Claude plan usage plus kaze's own resource use.
6. Run on Windows (primary today) and macOS (Patrick's MacBook) from one codebase.

## 3. Non-goals (first version)

- Typing Japanese through an IME. Patrick does not type Japanese. Japanese must **display** correctly (from output and paste) but IME composition is out.
- General split panes. The only split is the "related view" for a subagent beside its lead (section 6.3).
- A settings screen. Configuration is a file. *Reversed 2026-10-29 by Patrick: see `2026-10-29-kaze-settings-page-design.md`; the file stays the source of truth.*
- Built-in AI chat, blocks, cloud sync, collaboration, themes marketplace: everything Warp has that Patrick does not use.
- Linux support (not ruled out later, not designed for now).

## 4. Anti-goals (failure even if it works)

- **Laggy.** If typing or Claude Code's streaming output feels sluggish, kaze has failed. Fitting Patrick's workflow comes first; beating Warp is not a goal (Patrick, 2026-10-28, after the first Gio spike).
- Background CPU use when nothing is happening.
- Features that cost performance when they are not visible (a hidden panel must cost nothing).

## 5. Performance goals

Revised 2026-10-28 after the first Gio spike showed an empty Gio window alone uses ~160MB on Patrick's laptop (GPU driver baseline). Patrick's ruling: keep Go and Gio; the bar is "fast and not laggy", not beating Warp.

| Measure | Goal |
|---|---|
| Keypress to glyph on screen | under 10ms |
| Claude Code streaming | frames under 16.7ms (60fps), no flicker, no tearing |
| Launch to ready prompt | under 500ms |
| Scrolling 10,000 lines of scrollback | smooth, no visible stutter |
| Idle CPU | effectively zero |
| Memory | no hard cap; measured and reported, kept from growing with idle tabs |

A small benchmark harness that measures these is part of the work. Warp is measured alongside for reference only.

## 6. First version: what it does

### 6.1 Terminal core

- Correct VT/xterm behaviour sufficient for Claude Code, git, pwsh, zsh, vim-level full-screen apps: colors (256 and truecolor), cursor movement, alternate screen, bracketed paste, mouse reporting, scroll regions.
- Synchronized output (begin/end update markers) supported so full-screen apps can update without flicker. Which markers Claude Code actually sends must be verified early.
- Scrollback with a configurable hard cap, stored compactly.
- Copy and paste (mouse selection, keyboard shortcuts per platform conventions).
- Japanese and other wide characters display at the correct width (two columns); font fallback so CJK glyphs render even when the main font lacks them.
- Default shell: pwsh on Windows, zsh on macOS. Configurable.

### 6.2 Rendering rules (decided, from a colleague's terminal)

- **Dirty tracking:** only cells/lines that changed since the last frame are redrawn. Typing one character redraws one cell.
- **Glyph cache:** each glyph is rasterized once and reused.
- **No frame when nothing changed.** Idle means no drawing.
- **Coalescing:** bursts of output are drawn at most once per display refresh.

### 6.3 Tabs and subagent child tabs

- Normal tabs for Patrick's 3 to 4 concurrent sessions. Patrick flips between them; he does not normally need them side by side.
- When a Claude Code session in a tab spawns subagents, each subagent appears as a **child tab** attached right after its lead's tab (mockup option B, chosen by Patrick).
- Clicking a child tab opens that subagent's transcript **split beside its lead**, updating live.
- The transcript view is read-only and readable: messages and tool calls (for example "Bash git show ...", "Read file.py"), not raw JSON.
- Status per child tab: running, or finished (✓).
- Overflow: when a lead has many subagents, only the first few get their own child tab; the rest collapse into a `+N` chip that expands on click. *(Claude's call; the exact count is left to the builder.)*
- Finished child tabs fade and close themselves after a while. *(Claude's call; Patrick can override.)*
- Nested subagents (spawn depth > 1) are shown under their own parent. *(Claude's call; presentation left to the builder.)*

**Data source (verified on Patrick's machine 2026-10-28):** Claude Code writes transcripts to disk; kaze only watches files and never hooks into Claude Code for this.

```
~/.claude/projects/<project>/<session-id>.jsonl                            lead / main session
~/.claude/projects/<project>/<session-id>/subagents/agent-<id>.jsonl       each subagent
~/.claude/projects/<project>/<session-id>/subagents/agent-<id>.meta.json   agentType, description, model, spawnDepth, finished, ...
```

Each tab must know which Claude session is running in it, so kaze can find the right folder. How to map tab to session is left to the builder; one option is the session-start hook in 6.4 reporting its session id with the tab id.

### 6.4 Attention alerts

When Claude Code in any tab finishes its turn or needs Patrick (a question, a permission prompt, a ruling), that tab is marked and a small pane/toast says which tab and what kind of attention it needs. This is the Warp feature Patrick wants kept.

- Signal comes from **Claude Code hooks**, not screen scraping: the hook for "turn finished" and the hook for "needs input/notification".
- Each tab sets an environment variable with its tab id (for example `KAZE_TAB_ID`) so the hook can say which tab it is.
- The hook sends a small message to kaze over a local channel (named pipe on Windows, Unix socket on macOS, or equivalent: builder's choice).
- kaze ships the hook command and documents how to add it to Claude Code settings.
- Alert clears when Patrick switches to that tab.
- *Note 2026-10-28 (Plan 3):* the local channel is TCP on `127.0.0.1` (a random port) with a random 32-byte token per kaze run, passed to each tab as `KAZE_NOTIFY` and `KAZE_NOTIFY_TOKEN`, instead of a named pipe or Unix socket. No extra dependency, and the same code on Windows and macOS. The token keeps other local programs from faking alerts.

### 6.5 Status bar

- **Left: Claude usage**, 5-hour and weekly plan usage. Source still to verify: the Claude Code status-line input may carry this; if so, a tiny status-line script writes it to a file kaze reads. Token and cost per session can also be summed from the transcript `.jsonl` files.
- **Right: kaze's own memory and CPU**, plus the active tab's process tree (shell, Claude, node dev servers) so runaway processes are visible.
- Refresh rate is low (about once per second or slower) and stops when the window is hidden.
- *Note 2026-10-29 (Plan 4):* the source is Claude Code's status line input (`rate_limits.five_hour` / `seven_day`, `context_window.used_percentage`), taken by `kaze statusline`, which also prints the line for Claude's own status row. The owner chose 5-hour, weekly and the tab's context only: no per-model (Fable) usage (not in the status line data), no cost, and the right side keeps kaze's memory and tab count (CPU and the process tree are not planned). The numbers sit at the right of the bar, since the left now holds the folder and branch.

### 6.6 Config

A single config file in a standard per-user location (font, font size, colors, default shell, scrollback cap, keybindings). Reloaded on change if cheap; otherwise on restart.

> Note (2026-10-29): a settings page now edits this file, one key at a time with comments kept. See `2026-10-29-kaze-settings-page-design.md`.

### 6.7 Look: "Midnight" (Patrick's pick, 2026-10-28)

Soft dark chrome, rounded tabs, violet accent; calm and modern, close to Warp and current editors. Flat colors and rounded corners only (no window blur: Gio can't do it and it costs GPU).

| Token | Value | Used for |
|---|---|---|
| chrome | `#0b0d12` | tab bar and status bar background |
| bg | `#0f1117` | terminal background, active tab |
| panel | `#12151d` | subagent side pane |
| border | `#1d2230` | separators |
| child | `#161a24` | child-tab background |
| fg | `#d7dae3` | text |
| muted | `#8a90a2` | inactive tab text |
| dim | `#5d6273` | finished child tabs, hints |
| accent | `#8b7bff` | active tab dot, meters |
| accent-soft | `#b9a6ff` | child-tab text, cursor |
| attention | `#f2b84b` | "needs you" dot and alert |
| done | `#62d39a` | "done" marker |
| toast | `#1a1f2c` on `#2a3142` border | alert pane |
| selection | `#2e2a52` | selected text background |

ANSI 0-15: `#1d2230 #f07178 #62d39a #f2b84b #8ab4ff #b9a6ff #6fd3e0 #d7dae3` / `#5d6273 #ff8b92 #8be3b4 #ffd27a #a9c7ff #cdbfff #95e6f0 #ffffff`.

Shapes: tabs have 9px top corners; child tabs are smaller (11px text) and sit right after their lead; alerts are a rounded (10px) pane bottom-right above the status bar; status bar meters are 46x5px rounded bars. Mockup: `docs/design/look-midnight.html`.

### 6.8 Plan 3 decisions (Patrick, 2026-10-28; Plan 1 shipped first)

Plan 3 is the next plan. It covers the frame of the window: alerts, sound, title bar and the status bar's location info. It supersedes Plan 1's "keep the native title bar" call.

- **Attention alerts:** as in 6.4. The tab gets an amber pulsing dot; a toast bottom-right says "<tab> needs you · <reason>" with a "Go ›" that switches to it.
- **Chime:** a short, soft sound bundled with kaze plays on **"needs you"** (permission prompt, question). **"Done" is silent.** Both are configurable (on/off per kind, sound file). No chime when that tab is already active and focused; several alerts at once give one chime. Windows plays it through winmm `PlaySound` (no cgo); macOS through `afplay`.
- **kaze's own title bar:** no OS title bar. Tabs sit at the top edge.
  - Windows: kaze draws minimize, maximize/restore and close on the right (close turns red on hover). Empty tab-bar space drags the window; double-click toggles maximize; all edges and corners resize. Check whether Gio allows the Windows 11 snap-layout hover menu on the maximize button; Win+Z and edge snapping must keep working.
  - macOS: keep the native red/yellow/green buttons, inset into kaze's dark tab bar (like Safari and VS Code).
  - Mockup: `docs/design/titlebar-and-status.html`.
- **Status bar shows the current folder and git branch** of the active tab, left side, before the Claude usage: `~\Desktop\project  ⎇ feature/x`. Outside a repo the branch is hidden.
  - Folder: the shell reports it with OSC 7 from its prompt. kaze documents a small snippet for pwsh (`$PROFILE`) and zsh (`~/.zshrc`); adding it to the owner's profile needs the owner's yes.
  - Branch: kaze reads `.git/HEAD` (walking up from the folder; handles worktrees' `gitdir:` files) without running git; refresh on folder change and when HEAD changes.
  - Each tab has its own folder and branch.
- *Note 2026-10-28 (Plan 3 as built):* the "needs you" dot is a **static** amber dot, not a pulsing one: a pulse would redraw the window all the time and break the idle-CPU goal. Claude Code's `idle_prompt` reminders ("waiting for your input" after about a minute) and `auth_success` are **dropped**, so the types that ask something of you (`permission_prompt`, `elicitation_dialog`, `elicitation_url_dialog`, `agent_needs_input`), any unknown type, or an older Claude Code that sends no type raise "needs you"; the hook matcher limits which types reach kaze at all. The branch is also re-read on tab switch and when the window regains focus, because Claude Code can switch branches without a new prompt.

Order after Plan 3: Plan 2 (subagent child tabs, 6.3), Plan 4 (Claude usage in the status bar, 6.5), the Mac milestone, then section 7.

## 7. Later features (in order, not in the first version)

1. **Browser button.** A button that opens a **separate** Chromium window (not embedded), like a colleague's terminal. Launched with a DevTools debugging port so Playwright-driven agents can drive the window Patrick is watching. Windows: Edge or Chrome; macOS: Chrome. Whether agents can attach cleanly must be proven with a spike first.
2. **Brain graph.** A graph of notes from a notes vault (Patrick's knowledge base), shown as clickable nodes; clicking a node opens the note for reading inside kaze. Shows only related notes, not the whole vault: start from the current ticket (from the git branch, e.g. `feature/TICKET-123` → TICKET-123), add notes the agents opened this session, plus their direct links (one hop). A toggle shows the full graph. Kept for after the terminal itself is solid.

## 8. How it gets made (decided)

- **Language: Go.** Chosen over Swift (a teammate's choice, but Swift on Windows has no usable UI toolkit) and Rust (good fit, but slower to iterate). Claude's recommendation, accepted by Patrick.
- **UI: Gio** (cross-platform Go UI with its own text shaping). Chosen because it runs on Windows and macOS from one codebase, and the browser opens as a separate window so nothing needs embedding.
- **Shell process:** ConPTY on Windows, Unix pty on macOS, behind one interface (a library such as `go-pty` covers both).
- **Sessions do not survive kaze closing, crashing or restarting** (Patrick's decision, same as Warp). Shells run inside the kaze process; no background PTY host. The shell layer stays behind a clean interface so a survive-restart host could be added later without rewriting the UI.
- **Repo:** `<repo>` (e.g. `%USERPROFILE%\Desktop\kaze`), its own git repo, separate from any work repo.

## 9. Left to the builder

- VT parser: an existing Go library or a custom one, as long as section 5 targets are met.
- Internal architecture, package layout, the alert channel mechanism, tab-to-session mapping.
- Exact keybindings (follow each platform's normal conventions).
- Overflow count for child tabs and how long finished ones linger.
- Default font (must have a CJK fallback).

## 10. Calls made after the builder review (Claude's calls, Patrick can override)

1. **If the Gio spike misses the targets:** *(superseded 2026-10-28 by Patrick: Go and Gio stay regardless; spikes pick the fastest drawing method, they are not go/no-go gates.)*
2. **Alerts when kaze isn't focused:** first version is in-app (tab mark + pane) plus a taskbar flash on Windows and a Dock bounce/badge on macOS. These need no signing or packaging. Real OS toast notifications come later.
3. **Benchmarks:** the section 5 goals are measured on Windows at each milestone (and on the MacBook when available). Keypress latency uses software screen-capture timing. Warp numbers are recorded for reference, not as a pass/fail bar.
4. **Attention categories:** two, straight from the hooks: **done** (turn finished) and **needs you** (permission prompt or waiting for input). No guessing from transcript text.
5. **Usage source:** if no reliable 5-hour/weekly source exists, the first version ships with per-session tokens and cost from the transcripts, and plan usage follows when a source is found. If Patrick already has a Claude Code status line, kaze's script wraps it instead of replacing it.
6. **Which sessions count:** only Claude Code started in a kaze tab on the local machine. Jobs on a remote agent runner, SSH, WSL and Docker sessions are out of scope for subagent views and alerts (a remote runner has its own dashboard).

## 11. Open items to verify early

1. Which synchronized-output markers Claude Code emits, and that Gio can present a frame on demand without tearing.
2. Where Claude 5-hour and weekly usage can be read reliably.
3. That Gio meets the keypress-latency and memory targets on both platforms (a small spike before building features on top, testing both Gio's normal text path and the glyph-atlas fallback from section 10).
