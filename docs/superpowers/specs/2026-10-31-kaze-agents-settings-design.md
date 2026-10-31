# kaze: Agents settings (Claude Code and Codex) design

Date: 2026-10-31. Issue: #31. Status: approved by the owner on 2026-10-31; implemented and verified on `feat/agents-settings`.

Recovered the draft referenced by issue #31; it was not present on main.

Builds on the [settings page design](2026-10-29-kaze-settings-page-design.md), the [agent sidebar design](2026-10-30-kaze-agent-sidebar-design.md) (Plan 6b agent awareness) and the [agent browser design](2026-10-30-kaze-agent-browser-design.md). Everything there stands unless this document changes it.

## 1. Intent and why

The settings page has a **Claude** section, and every agent feature in kaze (alerts, chime, the `/command` and running state on the row, the usage readout, the setup check) only knows Claude Code.

Claude Code and Codex are peers: one builds a feature, the other reviews and fixes it, and they swap. Codex also takes UI and visual work, so both need equal treatment on the Mac. So a Codex session in one tab should get the same treatment as a Claude session in the next tab, and the settings page should say "agents", not "Claude".

kaze stays generic (agent sidebar design, section 1): it supports agents through their own public hook and config mechanisms, never through private endpoints or by reading their internal files.

## 2. What Codex offers (researched 2026-10-31, Codex CLI 0.166 on the owner's Mac)

| kaze feature | Claude Code source | Codex source | Parity |
|---|---|---|---|
| Needs you | `Notification` hook | `PermissionRequest` hook (runs when Codex is about to ask for approval) | Yes |
| Done | `Stop` hook | `Stop` hook | Yes, see 6.2 on output |
| Command each session runs | `UserPromptSubmit` hook | `UserPromptSubmit` hook (payload has `prompt`) | Yes |
| Interrupted (Esc) | none (known limit in 6b) | `Interrupt` hook | **Better than Claude** |
| Usage (5h, week, context) | `statusLine` command | no external status line command; `tui.status_line` only picks built-in footer items | **No** |
| Browser tools (MCP) | `mcpServers` in Claude config | `[mcp_servers.<id>]` in `~/.codex/config.toml` | Yes |

Facts that shape the design:

- Codex hooks live in `~/.codex/hooks.json` (or inline `[hooks]` in `~/.codex/config.toml`), are **on by default**, but **each non-managed hook must be reviewed and trusted by the user with `/hooks` inside Codex** before it runs.
- Codex's older `notify` key (one command, turn-complete only) is **already taken** on the owner's Mac by the bundled Computer Use client. kaze uses hooks instead and never touches `notify`.
- Codex `Stop` hooks that exit 0 must print JSON; plain text is invalid. kaze's `notify` prints nothing today.
- Hook payloads carry `session_id`, `cwd`, `hook_event_name` and `model`, so `kaze notify` can tell a Codex hook apart if needed.

Sources: Codex docs at learn.chatgpt.com (Hooks; Configuration Reference), and the owner's `~/.codex/config.toml`.

## 3. Goals

1. **Rename the section "Claude" to "Agents".** Shared alert settings once, then one setup block per agent.
2. **Codex alerts:** needs you, done and the chime work for Codex sessions exactly as for Claude.
3. **Codex on the row:** the running state and the command the owner typed show for Codex sessions too, and an Esc interrupt in Codex clears "running".
4. **Which agent:** each agent row shows which agent it is (Claude or Codex), since the owner flips between them in neighbouring tabs.
5. **Codex setup check with Fix**, like the Claude one, plus a clear "now trust it in Codex with `/hooks`" step.
6. **Room for the browser tool.** The agent browser feature (its design 6.7) will add a "Browser tool" line to both setup blocks. That line, and any MCP or `config.toml` writing, are **built by the browser plan, not this one** (*my call*). This plan writes only `hooks.json`.

## 4. Non-goals

- Codex usage numbers. The owner doesn't need them (owner, 2026-10-31), and Codex has no supported way to hand them to kaze anyway. Reading Codex's session files or private endpoints is out (same rule the owner set for Claude's per-model number in Plan 4). Revisit if Codex adds a status line command.
- Touching Codex's `notify` key.
- Other agents (Gemini, Aider and so on). The design should not make a third agent hard, but nothing is built for one.
- Per-agent chime sounds or per-agent alert switches. One set of alert settings covers every agent (*my call*; easy to add later).
- Codex-specific behaviour beyond the hooks above (approvals, sandbox modes, profiles).

## 5. The page

Section title **Agents** (was Claude), in the same place in the sidebar of the settings page. Hint: "Alerts come from each agent's hooks. Usage comes from Claude Code's status line."

Rows, in order:

| Row | Control | Change from today |
|---|---|---|
| Chime when an agent needs you | toggle + Test | label only (was "when Claude needs you") |
| Chime when an agent is done | toggle | label only |
| Flash the taskbar | toggle | none |
| "Done" toast stays for | segmented | none |
| Show Claude usage in the status bar | toggle | label says Claude, because only Claude reports it |
| **Claude Code setup** | status lines + Fix / Replace | unchanged, plus the browser tool line once that feature exists |
| **Codex setup** | status lines + Fix | **new** (6.3) |

Config keys stay the same (`chime_needs`, `chime_done`, `taskbar_flash`, `done_toast`, `show_usage`): they are already agent-neutral, so there is no migration. The `open_settings` deep links or any code that names the section by id may rename `claude` to `agents` (builder's call).

The Commands section's lines for `kaze notify` and `kaze statusline` say "Used by Claude Code and Codex hooks" and "Used by Claude Code's status line".

## 6. Behaviour

### 6.1 Which agent a session runs

- kaze learns the agent from the hook that fired: `kaze notify` gets an `--agent codex` argument in the Codex hooks (Claude hooks stay as they are, so the installed Claude setup keeps working; no argument means Claude). *(My call; reading `model` or other payload fields is a fallback the builder may use.)*
- The agent sticks to the session until the shell prompt comes back (the same rule that ends a Claude row today).
- The row shows a small agent tag before the command, for example `claude · /review` and `codex · /prompts:fix`. Exact look left to the builder, static, matching the row's existing marks.

### 6.2 Codex hooks kaze installs

In `~/.codex/hooks.json` (JSON is easier to edit safely than the owner's hand-kept `config.toml`, and reuses `claudesetup`'s JSON handling; *my call*):

| Event | Command |
|---|---|
| `PermissionRequest` | `<kaze> notify needs --agent codex` |
| `Stop` | `<kaze> notify done --agent codex` |
| `UserPromptSubmit` | `<kaze> notify prompt --agent codex` |
| `Interrupt` | `<kaze> notify interrupt --agent codex` (new kind: ends "running", no alert, no chime) |

Rules for `kaze notify` under Codex:

- Always exit 0 within about 1s, as today.
- **`PermissionRequest`: print nothing and make no decision**, so Codex's normal approval prompt continues.
- **`Stop`: print `{}`** (valid JSON that asks for nothing) so Codex accepts the output. Builder to verify against the real Codex that `{}` neither blocks nor continues the turn.
- **`UserPromptSubmit`: print nothing**, because plain output would be added to the conversation.
- Codex commands look different from Claude's (custom prompts as `/prompts:<name>`, skills as `$<name>`). The command shown on the row is the leading `/...` or `$...` word, cleaned and capped the same way. *(My call.)*

### 6.3 Codex setup check

Mirrors the Claude check in `internal/claudesetup`:

- Lines: "Alerts when Codex needs you", "Alerts when Codex is done", "Command each session runs", "Interrupts", and later "Browser tool".
- States per line: OK / missing / broken (points at a kaze that no longer exists) / taken (another command there; kaze adds its own entry beside it, since Codex runs every matching hook, so "taken" never blocks).
- **Fix** shows the change, asks to confirm, writes a timestamped `.bak` of `hooks.json` first (and of `config.toml` when the browser MCP entry is added), and never touches other entries.
- After a write, the row says: "Now open Codex and run /hooks to trust kaze's hooks." kaze cannot do this step. *(Codex requires it; the check can't see whether it was done, so the line stays until the owner dismisses it. My call.)*
- If `~/.codex` doesn't exist, the row says "Codex not found" and offers nothing.
- The check also **reads** inline `[hooks]` in `~/.codex/config.toml` (read only). If kaze's hooks are already there, the line says OK ("set up in config.toml") and Fix does not add a second copy, because Codex would then fire every alert twice. *(My call.)*
- The "/hooks to trust" reminder is plain in-memory state: shown after a Fix write until dismissed or until kaze restarts. Nothing is saved. *(My call.)*

### 6.4 Windows

The same hooks should work on the Windows laptop with `kaze.exe` paths (quoted for spaces). Marked **"should work, not verified"** until the owner tries Codex there, like the Windows folder picker today. *(My call.)*

### 6.5 First task: prove a Codex hook reaches kaze

`kaze notify` only acts when it sees `KAZE_NOTIFY`, `KAZE_NOTIFY_TOKEN` and `KAZE_TAB_ID` and can open a TCP connection to 127.0.0.1. Nobody has yet checked that Codex passes the environment through to hooks and doesn't block that connection. The plan's first task is that check against the real Codex in a kaze tab (a hook that logs its env and connects), plus the `{}` output on `Stop`. If either fails, the builder stops and brings it back to the owner before building the rest. *(My call.)*

## 7. Constraints

- No change in behaviour for Claude sessions; the installed Claude hooks keep working without a Fix.
- Idle CPU stays zero; no new polling.
- Strict TDD and safe RED tests; never inject global input.
- Never write `config.toml` or `hooks.json` without the confirm dialog and a backup.

## 8. Left to the builder

- Package layout (*suggestion:* `internal/codexsetup` beside `internal/claudesetup`, sharing the JSON helpers).
- Look of the agent tag on the row.
- Whether the section id is renamed internally.

## 9. Calls made in this document (for the owner to check)

1. Section name **Agents**.
2. One set of alert settings for all agents, no per-agent switches.
3. Codex hooks go in `~/.codex/hooks.json`, not `config.toml`.
4. `--agent codex` on the Codex hook commands; no argument means Claude.
5. Codex commands shown as the leading `/...` or `$...` word.
6. The "/hooks to trust" reminder stays until dismissed.
7. Codex usage is out until Codex offers a supported source.
8. The browser tool line and MCP registration belong to the browser plan, not this one.
9. The check reads inline `[hooks]` in `config.toml` (read only) to avoid duplicates.
10. The "/hooks" reminder is not saved across restarts.
11. Windows is "should work, not verified".
12. The first task proves a Codex hook can reach kaze; stop and ask if it can't.

## 10. Compatibility check completed on 2026-10-31

Tested installed Codex CLI 0.166.0 inside a real Kaze PTY, from main build
`d06becf`, with an isolated HOME and CODEX_HOME. A temporary build overlay
blocked real input and recorded notifications received by Kaze; no global
keyboard or mouse events were injected. All probe files stayed outside the repo.

The real CLI used a loopback Responses stub to complete one turn without account
credentials or an external model request. Its SessionStart, UserPromptSubmit
and Stop hooks each inherited KAZE_NOTIFY, KAZE_NOTIFY_TOKEN and KAZE_TAB_ID,
and connected to Kaze's 127.0.0.1 listener. The Stop hook passed `{}` on stdin to
the existing `kaze notify done`, then returned `{}` on stdout. Kaze received one
authenticated done notification for the tab. Codex exited 0 with one completed
turn and no Stop-output error or continuation.

The probe used the documented one-invocation hook-trust bypass only for its
known temporary hooks. The feature must require the user's normal `/hooks`
review and must not set or bypass trust. The first probe attempt was discarded
because its startup script also ran during shell initialization; the successful
retake used an inherited guard and an exclusive startup marker. Probe processes
were stopped and checked after the retake.

This proves the transport and Stop-output prerequisites. It does not yet prove
PermissionRequest end-to-end, Windows behavior, or trusted-hook
installation through the product settings UI. Those remain validation work.

Official reference: [Codex hooks](https://learn.chatgpt.com/docs/hooks), including
hook trust, common input/output fields, Stop and Interrupt.

## 11. Settings redesign integration

Use the existing 0.12 settings cards and status pills. The Agents page has shared
Alerts and Claude usage controls, followed by Claude Code setup and Codex setup
cards. Keep one Fix action per setup block. A Codex Fix preview lists only
changes to hooks.json, and a confirmation writes a backup first. The inline
config.toml hooks are read only. If an inline source is unreadable or ambiguous,
report that problem and do not silently add duplicate hooks.

Keep existing Claude hook commands valid. No argument defaults to Claude;
`--agent codex` selects Codex semantics. Only Codex Stop emits `{}`; the other
Codex hook commands remain silent and never make approval decisions.

## 12. Finished-command verification

The finished `--agent codex` commands were run by the real Codex CLI in the same
isolated Kaze PTY. UserPromptSubmit delivered `/prompts:probe` and `auth`, with
no hook output. Stop printed exactly `{}` plus a newline, delivered done with
agent identity, and completed one turn with exit 0. A second run interrupted
only that probe CLI with SIGINT while its local response was pending. Codex
fired Interrupt, which delivered an interrupt message with no hook output or
done notification. The CLI exited 1 for cancellation, as expected. No global
keyboard or mouse input was used.

Native macOS captures of shared controls and the Codex setup card were inspected.
UI tests cover preview, cancellation, confirmation, backups, stale sources,
trust-reminder dismissal, scratch routing, identity and Claude compatibility.
PermissionRequest's CLI delivery is covered by tests; its real approval dialog
and normal hook-trust review remain manual checks, along with Windows execution.
