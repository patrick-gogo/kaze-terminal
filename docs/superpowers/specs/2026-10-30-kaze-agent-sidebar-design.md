# kaze: agent sidebar design

Date: 2026-10-30. Owner: Patrick. Mockup: `docs/design/agent-sidebar-mockup.html` (clickable; the owner approved the sidebar, the servers section, floating panes and the scratch shell in it on 2026-10-30). Where this document and the mockup disagree, this document wins (see 11).

Builds on the [main design](2026-10-28-kaze-terminal-design.md), the [settings page](2026-10-29-kaze-settings-page-design.md), the [workspaces decisions](2026-10-29-kaze-workspaces-decisions.md) and [workspaces UI round 2](2026-10-30-kaze-workspaces-ui-design.md) (PR 21). Everything there stands unless this document changes it.

## 1. Intent and why

kaze already rings when Claude is done or needs you. The owner wants it to be **agent-first**: the place that knows what each Claude session is doing, what is waiting on them, and what they can start next, and that fits how they work.

But kaze must stay **generic**. It learns about the owner's work from their own Claude Code setup (skills, commands, hooks) and from scripts they write, never from code that knows about their tickets, Jira, the devbox or the gauntlet. Another person with other skills gets the same features, filled with their own things.

Two real moments drove this:

- **The Monday miss.** The owner queues gauntlet work that runs on a remote machine over the weekend (Friday night, or Sunday once usage resets). On Monday they forget it exists. When they open kaze, they want it to say that the run finished and needs to be brought home.
- **Missing a waiting session.** With up to about 5 Claude sessions, sometimes one has been waiting on the owner and they didn't notice. Not a deal breaker, but it costs time.

Tab hunting is not a problem today. The owner raised one worry while designing this: more agent features must not mean more tabs to click through.

## 2. Goals

1. **A sidebar instead of the tab strip** (Warp-style): sessions listed down the left, grouped by repo, each row saying what it is doing.
2. **Agent awareness:** each Claude session's row shows the slash command the owner last ran in it and whether it is running, needs them, or is done.
3. **A workspaces section** at the bottom of the sidebar for the server-type tabs workspaces start, separate from the sessions the owner talks to.
4. **Floating panes:** a workspace tab can be opened floating over the current session, and each session has a **scratch shell** that toggles floating.
5. **Chips:** short "waiting on you" items in the status bar that any script can post with `kaze status`, each able to start a Claude prompt when clicked.
6. **Timed checks:** kaze runs the owner's own check scripts on a timer, so chips appear even when no Claude session is running (the Monday case).
7. **A palette** (one key) that lists what is waiting, every session, and every skill and command in the owner's Claude setup, and acts on the chosen one.
8. **Jump to what needs you:** one key goes to the next session waiting on the owner.

## 3. Non-goals

- Anything specific to the owner's tickets, Jira, GitHub, the devbox or the gauntlet inside kaze. Those live in the owner's skills and check scripts (see 12).
- Running Zellij or another multiplexer inside kaze, and the terminal features that would need (OSC 52 clipboard, the kitty keyboard protocol). Only Zellij's floating-pane idea is borrowed.
- Real tiled panes (side-by-side splits). Floating panes only.
- Guessing which skill the owner wants from their branch or folder.
- Showing skills Claude starts by itself during a run (a skill that calls another skill). The row shows what the owner typed.
- Claude Code's built-in commands (`/clear`, `/compact` and the like) in the palette: they have no files to read.
- Saving floating panes or chips across a Kaze restart. Session metadata restoration was subsequently approved under issue #75; see `2026-11-01-kaze-session-restore-design.md`. Live PTYs still stop at exit.
- Changing shortcuts from the settings page (read-only list for now).
- A notification center or history of past chips.

## 4. Anti-goals (failure even if it works)

- **Idle CPU above zero.** No polling except the owner's timed checks at their own interval. Git and skill-file reads happen on events (a prompt report, a hook message, opening the palette), never on a ticker.
- **A hook that slows or changes Claude.** The new prompt hook must return fast, print nothing (Claude Code adds a prompt hook's output to the conversation), and never block or alter the prompt.
- **kaze sending text into Claude on its own.** Into an existing session kaze only types; the owner presses Enter. The one exception is a session kaze opens for the purpose (see 9.3).
- **Writing Claude Code's settings without showing the change first** (same rule as the settings page: show the exact change, confirm, back up).
- **Keys that steal from Claude Code or the shell.** Claude Code uses Ctrl+B (run in background), shells use Ctrl+K (kill line) and Ctrl+J (newline). kaze's new keys must not take those.
- **More clicking than today.** Nothing in this design opens a session or pane the owner did not ask for.
- A layout that works in one theme only. Everything uses theme tokens and reads well in all five, including the light Washi.

## 5. The sidebar

### 5.1 Placement

- The sidebar replaces the tab strip on Windows and macOS. Everything the tab strip did (switch, new, close, alert dots, the workspace group chip) moves into it.
- **Top:** a filter field ("Filter sessions…") and a **+** button for a new session.
- **Middle:** the sessions (5.2, 5.3).
- **Bottom:** the workspaces section (section 7), then a "Next waiting" button showing its key.
- **Hide/show** with the toggle-sidebar key or a button in the title bar. While hidden, the main view takes the full width and the title bar's search box (which opens the palette) still works.
- **Windows title bar:** sidebar toggle, the K mark, a search box in the middle that opens the palette, the scratch-shell button, the gear, the caption buttons. **macOS** (native title bar): the same buttons sit at the top of the sidebar; when the sidebar is hidden, a slim rail (about 36px) stays on the left with the sidebar toggle, palette, scratch and gear buttons (Claude's call).
- The width is fixed by default (about 260px at 100% scale).

### 5.2 Rows

Each session row has up to three lines:

1. **Title**, then ` · /command` in the theme's magenta when a slash command was seen (6.2).
2. **Folder**, shortened (`~\Desktop\samurai_cart_v3`).
3. **Branch** (without a `feature/` style prefix), **lines added and removed** in green and red (`+212 -14`, hidden when both are zero), and the **state word**: `running`, `needs you`, `done`, or nothing when idle.

A state mark leads the row: an accent ring for running (drawn still, not animated, so a session stuck in running never costs CPU), an amber dot with a soft halo for needs you, a green dot for done, a muted dot for idle or a plain shell. The selected row is highlighted. Hovering a row shows a × to close it.

**Title rule** (Claude's call): the name the owner gave the row (double-click to rename) wins. Otherwise, for a Claude session, the first argument of the last slash command if there was one (`/ship-ticket V3-3348` gives `V3-3348`), else the folder's name. For a plain shell, the folder's name.

**Lines added and removed:** `git diff --shortstat HEAD` in the session's folder, run on a prompt report or a Stop/Notification message for that session, never on a timer. Skipped outside a git repo. The builder bounds it (a time limit, at most one run in flight per session).

### 5.3 Repo groups

- Sessions whose folder is inside the same git repo sit under one **group header**. Worktrees and the main checkout of one repo count as the same repo (they share its git folder, `git rev-parse --git-common-dir`), and the group is named after the main checkout's folder (Claude's call): a stack of small dots in the members' state colors, the repo's name, and the member count. When a member needs the owner, the count becomes amber "1 waiting".
- Groups are **open by default and never fold by themselves** (owner's decision: with 2 or 3 sessions in one repo, they want to click straight to each). Clicking the header folds or unfolds it. A folded group still shows the amber count, and `› <title>` when the current session is inside it. kaze remembers a fold until the owner unfolds it, for as long as kaze runs.
- Sessions outside any git repo are listed loose, below the groups.
- A group exists as soon as one session is in the repo (Claude's call: the header always shows the repo name, even with one member).
- A session moves between groups when its folder changes (OSC 7) to another repo.

### 5.4 Filter

Typing in the filter field narrows the rows to titles, commands, folders and branches that contain the text (case-insensitive). Folded groups open while a filter is active and fold back when it is cleared. The workspaces section is not filtered.

## 6. Agent awareness

### 6.1 States

| State | Starts when | Ends when |
|---|---|---|
| running | the prompt hook reports a prompt for the session | a Stop or needs message |
| needs you | a Notification hook with the needs matcher (today's "needs" alert) | the owner visits the session, or a new prompt |
| done | the Stop hook (today's "done" alert) | the owner visits the session (back to idle) |

Done and needs keep today's chime, taskbar flash and toast behavior. Running is new and makes no sound.

A row counts as a **Claude session** once any hook message (done, needs, prompt) or a usage report arrives from it. Until then it is a plain shell.

**Leaving Claude:** a shell prompt report from the session (the first OSC 7 or OSC 133 A after Claude was seen, the same signal Plan 5b uses for readiness) means Claude exited. The row goes back to a plain shell, ends running, and keeps its title. Claude's call.

**Interrupts:** Claude Code fires no hook when the owner interrupts a turn with Esc, so the row stays "running" until the next prompt, Stop or needs message. That is accepted (no timeout, so no extra timer); the running mark is drawn still, so it costs nothing.

### 6.2 The prompt hook

- A new Claude Code **UserPromptSubmit** hook runs `kaze notify prompt`. It reads the hook's JSON from stdin, sends the prompt's leading slash command and its arguments (for example `/ship-ticket` and `V3-3348`; nothing more of the prompt) to kaze through the existing notify channel, prints nothing, and exits 0 even if kaze is not running.
- A prompt that does not start with `/` marks the session running and leaves the shown command as it was.
- The settings page's **Claude Code setup** check (settings design 6.3) gains this hook: it shows OK / missing / broken for it like the Stop and Notification hooks, and **Fix** adds it with the same show-the-change, confirm and back-up steps. Nothing is written to `~/.claude/settings.json` any other way.

### 6.3 Jump to what needs you

The next-waiting key (and the sidebar's "Next waiting" button) goes to the next session in the needs-you state, in sidebar order, wrapping around. With none waiting it shows a short toast "Nothing is waiting on you." and does nothing else.

## 7. The workspaces section

- Every **running workspace** (a Plan 5b run) shows in a section at the bottom of the sidebar instead of as a group chip in the tab strip. The group chip and its dropdown go away; their information moves here.
- **Header:** `WORKSPACES`, then the workspace's name and its overall state: "all up", or "1 down" in red. With more than one workspace running, one header per workspace. The header folds and unfolds the section.
- **One short line per member:** a bar in the member's color, its title, and on the right its status from PR 21 (Starting, Waiting for Docker / port N, Confirm to run, Running, No commands…). A member whose commands **finished** (a prompt report after the line was typed: the server stopped) shows red `stopped`, and counts as down in the header. Claude's call: for a server, finishing means it died, so it is shown as a problem.
- **Clicking a member** opens it as a floating pane over the current session (section 8). Clicking a member that is already the main view does nothing.
- Members do not appear among the sessions and are not reached by next/prev tab. They are reached by clicking, the palette, or the status bar's server dots.
- Running a workspace no longer moves the main view: the current session stays in front and the members appear in the section.
- **Status bar:** one small dot per member, green (running), red (stopped), muted (anything else), plus the word "servers". Clicking opens the first stopped member floating, or the first member when none stopped.

## 8. Floating panes

### 8.1 Behavior

- A floating pane is a terminal drawn over the current session's view, with a header (state mark, title, status, **Expand**, **×**) and its own border. Drag it by the header; resize it from its edges or the bottom-right corner. It stays inside the window; when the window shrinks, panes move and shrink to fit.
- **Clicking a pane** brings it to the front and gives it the keyboard. The front pane gets the accent border. Clicking the session behind it gives the keyboard back to the session.
- **Esc goes to the program**, not to kaze (Claude and editors need it). A pane closes with its ×, or for the scratch shell with its toggle key. This changes the mockup, where Esc closed the front pane.
- **Panes belong to the session they were opened in.** Switching sessions hides them; coming back shows them where and how big they were. Positions live in memory only.
- At most **4 panes per session**; opening a fifth replaces the oldest workspace pane.
- Resizing a pane resizes its terminal (the program gets the new size), as with a window resize.

### 8.2 Workspace members as panes

- Opening a member floating shows that member's terminal; it keeps running as before. A member floats in one session at a time: opening it from another session moves it there.
- **Expand** makes the member the main view (its row in the workspaces section is highlighted). Picking any session goes back.
- **×** only hides the pane. It never stops the member or its server.

### 8.3 The scratch shell

- **Hook messages from floating terminals** (Claude's call): scratch shells and workspace members are terminals with their own tab IDs inside kaze, hidden from the session list and from next/prev. A scratch shell's done / needs / prompt messages count toward its **parent session's row** (the scratch shell is part of that session); if both run Claude, the latest message wins. A workspace member's messages show on its line in the workspaces section and on its pane header. Chimes and the taskbar flash work as for any session.
- Each session has at most one scratch shell: the default shell, started the first time the owner toggles it, in that session's current folder (its last OSC 7 report, else its start folder).
- The toggle key (or the title bar's scratch button) shows or hides it; hiding keeps it running and keeps what is on screen. If its shell exited, the next toggle starts a new one.
- It is not listed in the sidebar. Closing the session closes its scratch shell.

## 9. Chips and timed checks

### 9.1 `kaze status`

```
kaze status set <id> <text> [--run <prompt>] [--level info|warn|error]
kaze status clear <id>
```

- Sends through the existing notify channel, so it works from any process kaze started (a session, a workspace member, a scratch shell): those have the channel's address and token in their environment. From anywhere else it fails.
- **Windows caveat:** the daily `kaze.exe` is a GUI-subsystem program, so a calling shell may not wait for it and may not see its output or exit code. Like `kaze notify`, `kaze status` is fire-and-forget: it never prints on success, and errors are best effort. Callers must not rely on its exit code or on two quick calls arriving in order. No second console exe is added (Claude's call). Timed checks do not need it at all (9.4).
- `<id>`: a short name the poster picks (letters, digits, `-`, `_`; at most 32). Setting an existing id replaces it.
- `<text>`: what the chip says, at most 48 characters (longer is cut with an ellipsis).
- `--run`: the prompt to start when the chip is clicked, usually a slash command with arguments.
- `--level`: info (accent color), warn (amber, the default), error (red).
- At most **6** chips; a seventh replaces the oldest.

### 9.2 Chips in the status bar

- Chips sit at the left of the status bar, before the usage numbers, in the order they were first set.
- **Clicking** a chip with `--run` acts like choosing a skill in the palette (9.3) with that prompt, then hides the chip. A chip without `--run` just hides.
- A hidden chip stays hidden while later `set` calls with the same id carry the same text, so a timed check that keeps reporting the same lap doesn't bring it back. New text brings it back.
- Chips also head the palette's "Waiting on you" list.

### 9.3 Starting a prompt (shared by chips and the palette)

- **The current session is a Claude session that is idle, done or running:** kaze types the prompt (plus a trailing space) into it and does not press Enter. The owner adds arguments or presses Enter. A toast says what was typed and where.
- **The current session needs the owner, is not a Claude session, or the owner held Shift:** kaze opens a new session in the current session's folder and, once its shell is ready (the Plan 5b readiness rule), types `claude "<prompt>"` and Enter. The prompt is sent at once in this case, because the session was opened for it.
- Why needs-you opens a new session (Claude's call): a session that needs the owner is usually showing a permission or choice menu, and typed characters (digits especially) would answer it. kaze never types into it.
- Typing while Claude is running is safe: Claude Code holds the text in its input box until the turn ends.
- The prompt is quoted for the session's shell the same way Plan 5b joins command lines. A shell kaze can't quote for gets an error toast and nothing is typed.

### 9.4 Timed checks

- The config gains a list of checks:

  ```toml
  [[checks]]
  name = "lap"
  command = ["pwsh", "-NoProfile", "-File", "C:\\Users\\john\\scripts\\lap-check.ps1"]
  every = "15m"
  ```

- kaze runs each check **once when it starts** and then every `every` (at least `1m`; default `15m`). It runs hidden (no console window on Windows).
- **A check reports by printing**, one line per chip on its standard output, which kaze reads when the check ends (at most 16 KB; extra is ignored):

  ```
  set <id> <text> [--run <prompt>] [--level info|warn|error]
  clear <id>
  ```

  Same meaning and limits as `kaze status` (9.1); other lines are ignored. This way a check script never has to call kaze's GUI exe (Claude's call, see the Windows caveat in 9.1).
- A run that takes longer than **60 seconds** is stopped. A check whose last run is still going is skipped. A check that fails five times in a row shows one warning toast and keeps its schedule.
- Checks are edited in `config.toml` by hand for now. Settings shows them read-only in the Claude section: name, interval, last run time and result.
- This is the only timer this design adds, at the owner's chosen interval.

## 10. The palette

### 10.1 Opening and sections

- Opened with the palette key or the title bar's search box. A centered box over the main view with a query field.
- Three sections, in this order, each filtered by the query (case-insensitive, matching anywhere in name or description; a leading `/` is ignored):
  1. **Waiting on you:** chips, then sessions in the needs-you state.
  2. **Sessions:** every session and every workspace member, with its state.
  3. **Skills:** every skill and slash command from the owner's Claude setup (10.2), recently used first, then by name, each with its description cut to one line.
- ↑ ↓ move, **Enter** acts, **Shift+Enter** acts in a new session (skills and chips), **Esc** or a click outside closes.
- **Acting:** a chip or skill starts its prompt (9.3). A session goes to it. A workspace member opens floating.

### 10.2 Where skills come from

Read when the palette opens, cached, and re-read only when a source file's modification time changed:

- `~/.claude/skills/*/SKILL.md` and `~/.claude/commands/**/*.md` (user).
- `.claude/skills/*/SKILL.md` and `.claude/commands/**/*.md` in the current session's repo root (project).
- Skills and commands of **enabled** plugins (as `~/.claude/settings.json` lists them), from each plugin's **newest installed version** only. Their names are shown as Claude Code shows them, `plugin:name`.

Name and description come from the file's frontmatter (`name`, `description`), else the file or folder name and the first non-empty line. A file that can't be read or parsed is skipped silently. kaze only reads these files; it never writes them.

"Recently used" is the last 10 prompts started from chips or the palette, kept in memory only.

## 10.3 kaze's own actions in the palette

A fourth section, **kaze**, lists kaze's own actions by name with their key: new session, close session, next / previous session, copy, paste, open settings, toggle sidebar, scratch shell, next waiting, and one "Run workspace <name>" per saved workspace. Choosing one does what its key or button does. It sits after Skills and is filtered the same way.

## 10.4 Settings: Commands section

A new **Commands** section in the settings page, after Claude (it replaces "Shortcuts" in the settings design's "Later" line). Read-only in this version (owner's decision); changing a key still happens under `[keys]` in `config.toml`.

1. **Shortcuts:** every kaze action with the key it has on this machine, including the owner's `[keys]` overrides, and its config name (`palette`, `next_waiting`, …). Mouse actions from the README (double / triple click, Shift+drag in mouse apps) as plain lines below.
2. **Command line:** each thing `kaze` accepts, with a one-line description and a **Copy** button for the example: `kaze --workspace <name>`, `kaze --version`, `kaze status set …` / `kaze status clear …`, the `set` / `clear` line format that timed checks print (9.4), and `kaze notify` / `kaze statusline` marked "used by Claude Code hooks; you don't run these yourself".

The owner's Claude skills are not listed here; they live in the palette.

## 11. Keys

New shortcuts, all changeable under `[keys]` like the existing ones:

| Action | Config name | Windows | macOS |
|---|---|---|---|
| Open the palette | `palette` | Ctrl+Shift+K | Cmd+K |
| Go to the next session that needs you | `next_waiting` | Ctrl+Shift+J | Cmd+J |
| Show / hide the sidebar | `toggle_sidebar` | Ctrl+Shift+B | Cmd+B |
| Show / hide the scratch shell | `scratch_shell` | Ctrl+Shift+` | Ctrl+` |

These differ from the mockup (Ctrl+K, Ctrl+J, Ctrl+B, Ctrl+`) because of the keys anti-goal in section 4: kaze's Windows shortcuts already use Ctrl+Shift, and the plain Ctrl keys belong to Claude Code and the shell. On macOS Cmd+` switches windows, so the scratch shell uses Ctrl+`.

`next_tab` / `prev_tab` step through the sessions in sidebar order, including members of folded groups, skipping workspace members.

## 12. The owner's setup (not product code)

After the build, and only on the owner's word:

- Add the prompt hook through Settings → Claude → Fix.
- Write a **lap check** script that asks the remote machine whether a gauntlet run finished and calls `kaze status set lap "lap done · 3 tickets" --run "/v3-lap"`, then add it under `[[checks]]`. **Open:** how the script can ask the remote machine without a Claude session (a command-line tool or an HTTP call). The owner's tooling decides this; kaze's part doesn't change.
- Optionally post other chips the same way (red CI, review comments) from skills or checks.

## 13. Constraints

1. Windows first (daily use); macOS must build and behave the same where it applies. The macOS run happens on the owner's Mac before merge.
2. Drawn with kaze's own Gio code and theme tokens; no new UI dependency.
3. Tests never read or write the owner's real `~/.claude/settings.json`, `config.toml` or `workspaces.toml`, never send input to the owner's windows, and never start real Docker. Check-runner tests use a stand-in command, with bounded time and output.

## 14. Decided

**By the owner:**

- Sidebar on the left replacing the tab strip, like Warp.
- Repo groups open by default, folded only by the owner.
- Servers separate from Claude sessions, in a section at the bottom of the sidebar.
- Floating panes for workspace members, a per-session scratch shell, and panes that belong to their session.
- Generic: kaze reads the owner's skills and hooks; nothing is hard-coded to their workflow.
- A status bar chip (not a launch card or a "Today" page) for the Monday reminder.
- A read-only Commands section in settings listing shortcuts and command-line commands; click-to-rebind later.

**By Claude (the owner can override):**

- Timed checks as the way chips appear with no session running (the owner didn't object; strike it if wrong).
- The title rule (5.2) and lines added/removed from `git diff --shortstat HEAD`.
- Only the slash command the owner typed is shown, not skills Claude calls itself.
- A finished workspace member counts as stopped (red).
- Running a workspace no longer moves the main view.
- Esc goes to the program, not to floating panes.
- Into an existing session kaze only types; a new session sends the prompt at once.
- The key choices in 11.
- Worktrees of one repo share a group named after the main checkout.
- A shell prompt report means Claude exited; a row stuck in running after an Esc interrupt is accepted, drawn still.
- A session that needs the owner never gets typed into: chips and skills open a new session instead.
- Scratch shell hook messages count toward the parent row; members show their own.
- Timed checks report by printing lines; `kaze status` stays fire-and-forget, no second exe.
- On macOS a slim rail of buttons stays when the sidebar is hidden.
- Limits: 4 floating panes per session, 6 chips, 48-character chip text, 60-second check timeout, 1-minute minimum interval.
- A clicked chip stays hidden until its text changes.

## 15. Left to the builder

Exact sizes and spacing, the sidebar's minimum window width before it auto-hides, pane drag/resize hit areas, toast wording, how the settings page opens now that there is no tab strip (as the main view with a "Settings" row pinned at the top while open is the suggestion), and the plan split. Suggested order, each its own plan and PR:

1. **6a** Sidebar replacing the tab strip, repo groups, filter, the workspaces section (clicking a member makes it the main view until 6d), keys.
2. **6b** Agent awareness: the prompt hook and its settings check, running state, command and title on rows, lines added/removed, next-waiting.
3. **6c** `kaze status`, chips and timed checks. **Parked** by the owner on 2026-10-30 (no work until they say so); 6d and 6e go ahead without it.
4. **6d** Floating panes and the scratch shell.
5. **6e** The palette, the skill catalog, kaze actions in the palette, and the Commands settings section.
