# kaze: subagents design (tree, live pane, board)

Date: 2026-10-31. Owner: Patrick. Mockup: `docs/design/agents-combined-mockup.html` (clickable, `?v=tree|pane|board`). The owner picked ideas A, B and C from `docs/design/agents-mockup.html` and asked for them combined. Where this document and the mockup disagree, this document wins.

Builds on the [agent sidebar design](2026-10-30-kaze-agent-sidebar-design.md) (sidebar rows, states, floating panes, the notify channel) and the [settings redesign](../plans/2026-10-31-kaze-settings-redesign.md). Everything there stands unless this document changes it.

## 1. Intent and why

Claude Code often hands work to subagents (Explore, Plan, general-purpose, the owner's own agents). Today kaze sees one row saying "running" and nothing about what is going on underneath. The owner wants to **see what each subagent is working on**, without opening transcripts and without more tabs.

Three views, one feature:

- **A. Tree:** each session row in the sidebar lists its subagents underneath.
- **B. Live pane:** clicking a subagent opens a read-only floating pane with what it is doing, step by step, and its final report.
- **C. Board:** one page with every subagent across all sessions, grouped as running, waiting on you, and done.

## 2. Goals

1. A session row shows its subagents: type, task, elapsed time, and the tool call it is on right now.
2. Clicking a subagent shows its steps live and, when it finishes, its final report.
3. One key (and a button) shows every subagent in every session, so nothing waiting is missed.
4. Works with stock Claude Code through documented hooks. Nothing reads Claude Code's private files.

## 3. Non-goals

- Controlling subagents: no stop, retry, reply or approve from kaze. Approvals stay in the session's terminal.
- Reading transcripts (`~/.claude/projects/...jsonl`, `meta.json`). Their format is not documented and can change. kaze shows only what hooks tell it.
- Agent teams and in-process teammates. Only subagents started with the Agent tool.
- Codex. Claude Code only for now.
- History across a kaze restart. Everything lives in memory.
- Token or cost numbers per subagent. Hooks do not report them.
- Progress bars or percentages (the owner rejected the fake ones in the first mockup).

## 4. Anti-goals (failure even if it works)

- **Slowing Claude.** The new hooks run on every tool call, so `kaze notify` must stay fast: it reads stdin with the existing limits, sends at most one small message, prints nothing, and always exits 0. In the main conversation it sends nothing except for the Agent tool (5.2).
- **Idle CPU above zero.** No polling and no file watching. The one timer (elapsed minutes, 6.1) runs only while a subagent is running.
- **Wrong "waiting on you".** Better to show a session as waiting than to pin the wait on the wrong subagent (5.4).
- **Leaking a prompt into the UI unchecked.** Every string from a hook is cleaned the same way prompt arguments are today (no control characters, length caps).
- Writing Claude Code's settings without the settings page's show, confirm and back-up steps.

## 5. Where the data comes from

### 5.1 Hooks

The Claude setup check (Settings, Claude) gains three hooks, shown and fixed like Stop, Notification and UserPromptSubmit today:

| Hook | Command | What kaze learns |
|---|---|---|
| `SubagentStart` | `kaze notify agent-start` | `agent_id`, `agent_type`: a subagent began |
| `SubagentStop` | `kaze notify agent-stop` | `agent_id`, `last_assistant_message`: it finished, and its report |
| `PreToolUse` (matcher `*`) | `kaze notify tool` | inside a subagent (`agent_id` present): its current step. In the main conversation: only the Agent tool's `description` and `subagent_type` |

All of them go through the existing notify channel (loopback, token, `KAZE_TAB_ID`), so they only work from kaze tabs, scratch shells and workspace members, like today.

`kaze notify tool` with no `agent_id` and a tool other than `Agent` (or the older name `Task`) exits at once without connecting. That keeps the main conversation's cost to one short process start per tool call.

### 5.2 Naming a subagent (its task)

`SubagentStart` has the type but not the task. The Agent tool call that started it does: the main conversation's `PreToolUse` for `Agent` carries `tool_input.description` (for example "Find the auth middleware") and `tool_input.subagent_type`.

- kaze keeps, per tab, a short queue of pending Agent calls (description and type, at most 16).
- On `SubagentStart`, kaze takes the oldest pending call in that tab with the same type and uses its description as the task. If none matches, the task is the type alone ("Explore").
- If the Agent call came from inside another subagent (it has an `agent_id`), that subagent is the new one's **parent** (6.1).

### 5.3 Steps

Each `kaze notify tool` from inside a subagent adds a step: the tool name and a one-line summary taken from `tool_input`:

| Tool | Summary |
|---|---|
| Bash | `command` (first line) |
| Read, Edit, Write | `file_path`, shortened to the last two folders |
| Grep, Glob | `pattern`, plus `path` when set |
| WebFetch | `url` host and path |
| WebSearch | `query` |
| anything else | the tool name only |

Summaries are cut to 120 characters and cleaned like prompt arguments. kaze keeps the last 200 steps per subagent; older ones drop off with a "N earlier steps" line.

The latest step is the subagent's **current action** in the tree.

### 5.4 Waiting on you

A permission prompt fires `Notification` (`permission_prompt`), which kaze already receives as "needs".

- **If the message carries an `agent_id`** (the hooks docs list it as a common field "for the subagent the hook fires in"), that subagent is **waiting on you**. Its next step or its stop clears the wait.
- **If it doesn't**, the session is waiting as today, and no subagent is marked. The board then shows the session's card in "Waiting on you" with "a subagent may be asking" when any of its subagents is running.

Not yet confirmed on a real session. **The first plan task checks it** by capturing the real hook input from a throwaway project (never the owner's `~/.claude`). Both paths are built either way, because older Claude Code versions may lack the field.

`kaze notify needs` starts sending `agent_id` when present. Nothing else about today's alert changes (same chime, flash, toast).

## 6. The views

### 6.1 Tree (in the sidebar)

- Under a session row, one line per subagent, indented, in start order: a state mark, the **type** in the theme's magenta, the **task**, and on the right the elapsed time in whole minutes ("4m"; "<1m" before that). A second, dimmer line shows the current action ("Grep `authMiddleware` in src/").
- States: running (accent ring, drawn still), waiting on you (amber dot), done (green check, dimmed line). There is no failed state: hooks don't say, so every stopped subagent is done.
- **Background subagents** keep running after the session's turn ends. The row may say done while its tree still shows running subagents; that is right, and the chip keeps counting them.
- A subagent started by another subagent sits one step further in. Deeper than two levels draws at the second level.
- The session row shows a chip "3 agents" (running and waiting count) next to its state word. A folded repo group adds them up.
- Finished subagents stay in the tree until the session's next prompt, then leave the tree (they stay on the board).
- At most 6 subagent lines per session; more shows "+4 more", which opens the board filtered to that session.
- The elapsed minutes refresh once a minute while any subagent runs. No timer otherwise.
- Clicking a subagent line opens its live pane (6.2) in that session, switching to the session first if needed.
- The sidebar filter also matches subagent types and tasks.

### 6.2 Live pane

- A floating pane (agent sidebar design, section 8): same header, drag, resize, front-on-click, max 4 per session. It is **drawn by kaze, not a terminal**, and takes no keyboard input except scrolling.
- **Header:** state mark, type, task, elapsed time, ×. × hides the pane; the subagent keeps running.
- **Body:** the steps, newest at the bottom, each as `HH:MM:SS  Tool  summary` with the tool name in mono. The pane follows the newest step unless the owner scrolled up (same rule as terminal panes).
- **When it finishes:** a "Report" block under the steps with `last_assistant_message`, wrapped, selectable and copyable with the usual copy key. Long reports scroll in the pane. Plain text: no markdown rendering in v1.
- **While waiting on you:** an amber strip "Waiting for your approval in the session below" at the top of the body.
- One pane per subagent. Clicking another subagent opens a second pane (up to the limit); clicking one that is already open brings it to the front.
- Panes for subagents of a closed session close with it.

### 6.3 Board

- A full page in the main area, like Settings: it replaces the terminal view until closed. Opened by:
  - the **Agents** button in the sidebar's top icon row, with a count badge of running plus waiting subagents (hidden at zero; amber when anything waits);
  - the status bar's "**N agents running**" text (hidden at zero);
  - the palette ("Show agents");
  - the `agents_board` key: **Cmd+Shift+A** on macOS, **Ctrl+Shift+A** on Windows. Changeable under `[keys]` like the others.
- **Three columns:** Running, Waiting on you, Done. Narrow windows stack them.
- **A card per subagent:** type, task, session title and repo, elapsed time (or "took 6m" when done), current action or the report's first line. A waiting card has the amber edge.
- Session-level waits from 5.4's fallback show as session cards in Waiting on you.
- **Clicking a card** closes the board, goes to that session and opens the subagent's live pane.
- **Closing:** "Back to <session> ×" at the top right, the same key again, or Esc (the board has no program behind it, so Esc is free here).
- Done keeps the last 50 finished subagents across all sessions, newest first, until kaze quits. Closing a session removes its cards.
- With the hooks missing, the board says "kaze needs three more Claude Code hooks to see subagents." with a button that opens Settings, Claude.

## 7. Settings

- **Claude, setup check:** three new rows (Subagent start, Subagent stop, Tool steps) with the same OK / Missing / Broken pills and Fix flow. "Fix all" includes them.
- No on/off switch in v1: without the hooks the feature shows nothing besides the board's hint. (Claude's call; a switch is easy to add later.)
- **Shortcuts:** the new `agents_board` key is listed and searchable.

## 8. Tests the builder must keep

- Every hook command exits 0, prints nothing, and finishes inside the existing one-second budget, including with no kaze running, with garbage stdin and with a huge `tool_input` (bounded reads).
- `kaze notify tool` in the main conversation for a non-Agent tool makes no connection.
- Pairing (5.2): two Explore calls then two starts get their own descriptions in order; a start with no pending call uses the type.
- Waiting (5.4): with `agent_id` the right subagent is marked and cleared by its next step; without it the session is marked and no subagent is.
- Summaries strip control and escape characters and respect the caps.
- No test reads or writes the owner's real `~/.claude`, `config.toml` or `workspaces.toml`.
- Idle: no timer runs with zero running subagents.

## 9. Left to the builder

- Exact sizes, spacing and colors, from the mockup and the theme tokens; it must read well in all five themes, including Washi.
- How the agent state is stored (per tab, keyed by `agent_id`) and cleaned up when a tab closes.
- Message names on the notify channel, as long as old `kaze notify` commands keep working.

## 10. Claude's calls (for the owner to check)

1. Hooks only, no transcript reading (3, 5).
2. The waiting fallback marks the session, not a guessed subagent (5.4).
3. Finished subagents leave the tree at the next prompt but stay on the board (6.1, 6.3).
4. Esc closes the board (unlike panes, where Esc goes to the program).
5. Reports are plain text in v1.
6. No on/off switch in v1 (7).
7. Limits: 16 pending Agent calls per tab, 200 steps per subagent, 6 tree lines per session, 50 done cards.
