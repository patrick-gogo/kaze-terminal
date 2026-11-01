# kaze: subagents design (tree, live pane, board)

Date: 2026-10-31. Owner: Patrick. Mockup: `docs/design/agents-combined-mockup.html` (clickable, `?v=tree|pane|board`). The owner picked ideas A, B and C from `docs/design/agents-mockup.html` and asked for them combined. Where this document and the mockup disagree, this document wins.

Builds on the [agent sidebar design](2026-10-30-kaze-agent-sidebar-design.md) (sidebar rows, states, floating panes, the notify channel) and the [settings redesign](../plans/2026-10-31-kaze-settings-redesign.md). Everything there stands unless this document changes it.

**Replaces** the main design's section 6.3 (subagent child tabs next to the lead, with a live transcript split). The tree, live pane and board cover the same goal ("subagents visible and readable live, attached to the lead that spawned them") without extra tabs.

**Includes issue #23, Part 1** (pass the Claude session identity through `kaze notify`) as its foundation, and does **issue #23, Part 3** (match subagents to their lead) through it. Parts 2, 4, 5 and 6 of #23 stay separate.

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

### 5.0 Session identity (issue #23, Part 1)

Every Claude Code hook receives `session_id`, `transcript_path` and `cwd` on stdin. `kaze notify` forwards them on every message (the existing ones and the new ones below), each length-capped like `Reason`, so the body stays under the 4 KB cap.

- kaze keeps the latest three per tab in one small struct, overwritten on each message.
- A subagent is keyed by **session and `agent_id`**, so two conversations in one row (a session and its scratch shell, both running Claude) never mix.
- **A new `session_id` in a tab** (`/clear`, or a fresh `claude`) is a new conversation: that tab's subagents, pending Agent calls and waiting marks are cleared from the tree and the board's Running and Waiting columns. Finished ones stay in Done.
- Messages without a `session_id` (older hook commands, Codex) still work as today; they just don't feed subagents.
- `transcript_path` is stored but **not read** by this feature (3). It is there for #23's later parts.


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

Summaries are cut to 120 characters and cleaned like prompt arguments. kaze keeps the last 200 steps per subagent; older ones drop off. Every step is also counted by kind (searches, shell commands, reads, edits, web requests, other), dropped ones included, for the live pane's summary line (6.2).

The latest step is the subagent's **current action** in the tree.

**Prompt (#78, 2026-11-01):** the main conversation's Agent call also sends its `prompt` (Codex: `spawn_agent`'s `message`) as `task_prompt`, cleaned like a report and capped at 16 KB. It pairs with the subagent the same way the description does (5.2). A Codex helper's next turn (its own `UserPromptSubmit`, sent as an agent start) carries that turn's message, which replaces the prompt; a turn whose message kaze didn't get clears it, so an old prompt never sits above a new answer. The owner's next prompt forgets Agent calls that never started (a denied call), so they can't lend their description or prompt to a later subagent of the same type.

### 5.4 Waiting on you

A permission prompt fires `Notification` (`permission_prompt`), which kaze already receives as "needs".

- **If the message carries an `agent_id`** (the hooks docs list it as a common field "for the subagent the hook fires in"), that subagent is **waiting on you**. Its next step or its stop clears the wait.
- **If it doesn't**, the session is waiting as today, and no subagent is marked. The board then shows the session's card in "Waiting on you" with "a subagent may be asking" when any of its subagents is running.

**Checked on Claude Code 2.1.314, 2026-10-31** (throwaway project, `claude -p`): `PreToolUse` inside a subagent carries `agent_id` and `agent_type`; the main conversation's call is `tool_name: "Agent"` with `description` and `subagent_type`; every event has the parent's `session_id`; `SubagentStop` carries `last_assistant_message`. Whether `permission_prompt` carries `agent_id` still needs an interactive run (`-p` never asks). **Interactive run, same day:** Claude Code launched both subagents with `requestShape: "background"` and `requestNonInteractive: true` (from their `meta.json`), and no Notification fired. Subagents in this version don't ask you anything themselves, so per-subagent "Waiting on you" will rarely or never trigger; the session fallback below covers prompts in the main conversation. Both paths stay in, in case a later Claude Code lets subagents ask. **The first plan task checks it** by capturing the real hook input from a throwaway project (never the owner's `~/.claude`). Both paths are built either way, because older Claude Code versions may lack the field.

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
- **Body (changed by #78, 2026-11-01; was a list of every step):** it reads like a conversation, top to bottom:
  1. **Prompt:** the prompt the subagent was given (5.3), with a thin bar on its left. Left out when kaze never got it.
  2. **One summary line** of what it has done, like Claude Code's collapsed view: "Searched for 13 patterns, ran 15 shell commands so far" while it runs ("Working…" before its first step), without "so far" once done. Individual tool calls are never listed.
  3. **While waiting on you:** the amber strip "Waiting for your approval in the session below", at the bottom where the pane is following.
  4. **When it finishes:** a "Report" block with `last_assistant_message`, copyable. Plain text: no markdown rendering.
- The prompt and report keep their line breaks, blank lines and indentation (a tab counts as four spaces). Long lines wrap; a word wider than the pane (a long path or URL) breaks where it fills the line. The pane follows the bottom unless the owner scrolled up (same rule as terminal panes).
- The prompt and report are laid out once and cached; a new step lays out only the summary line again.
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
- Session identity (#23 Part 1): the three fields are forwarded, capped, and optional; a new `session_id` in a tab clears its running and waiting subagents but keeps Done.
- Summaries strip control and escape characters and respect the caps.
- No test reads or writes the owner's real `~/.claude`, `config.toml` or `workspaces.toml`.
- Idle: no timer runs with zero running subagents.

## 9. Left to the builder

- Exact sizes, spacing and colors, from the mockup and the theme tokens; it must read well in all five themes, including Washi.
- How the agent state is stored (per tab, keyed by `agent_id`) and cleaned up when a tab closes.
- Message names on the notify channel, as long as old `kaze notify` commands keep working.

## 10. Claude's calls (for the owner to check)

1. Hooks only, no transcript reading (3, 5), even though #23 Part 1 hands kaze the transcript path. **Owner's decision (2026-10-31):** the live pane shows steps and the final report only. Reading the transcript when a pane opens (the subagent's own messages, and steps from before kaze saw it) may come later, with #23 Part 4's transcript reader. That also replaces the main design's 6.3 data source, which watched transcript files. **Updated by #78 (2026-11-01):** the pane now shows the prompt, a summary line and the report instead of steps (6.2). The subagent's in-between messages still need the transcript and are still left for later.
2. The waiting fallback marks the session, not a guessed subagent (5.4).
3. Finished subagents leave the tree at the next prompt but stay on the board (6.1, 6.3).
4. Esc closes the board (unlike panes, where Esc goes to the program).
5. Reports are plain text in v1.
6. No on/off switch in v1 (7).
7. Limits: 16 pending Agent calls per tab, 200 steps per subagent, 6 tree lines per session, 50 done cards.
