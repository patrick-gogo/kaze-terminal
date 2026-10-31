# Agents settings implementation plan

Date: 2026-10-31. Issue: #31. Status: approved by the owner on 2026-10-31; implemented and verified on `feat/agents-settings`.

Design: [Agents settings](../specs/2026-10-31-kaze-agents-settings-design.md).
Branch: `feat/agents-settings`, based on main `d06becf`.

## Prerequisite and baseline

The required real Codex CLI hook transport and Stop `{}` check passed on macOS.
The design records the exact scope and limitations. `go test ./...` and
`go vet ./...` pass on this baseline. Race checks pass for notify and
claudesetup; UI race checking reproduces the HANDOFF-known failure in
`TestOnNotifyNeverBlocksAndStaysBounded`, in the test callback counters.
This exists before any feature code changes. Never inject global input or write the owner's agent settings
for tests. Use bounded temporary files, fake PTYs and the existing Gio harness.
For each behavior below, write the test, run it to observe the expected failure,
then implement and rerun it. Keep failing tests bounded in size and time.

## 1. Agent-aware hook protocol and CLI

Files: `internal/notify/notify.go`, `cmd/kaze/notify.go`, `cmd/kaze/main.go` and
their tests.

- Add a validated agent identity and interrupt notification kind. Existing
  messages and CLI calls remain Claude-compatible without an agent argument.
- Accept `kaze notify <kind> --agent codex`; reject malformed arguments quietly
  without contacting the listener. Do not infer identity from arbitrary text.
- Parse Codex's leading slash commands and `$skill` names; retain the existing
  sanitization, bounds, deadlines and always-success hook exit behavior.
- Print exactly `{}` for Codex done even outside Kaze or on delivery failure.
  Other kinds remain silent. Do not emit PermissionRequest approval decisions.
- Test wire validation, old messages, slash and dollar commands, malformed or
  oversized data, missing env, deadlines, and event-specific stdout.

## 2. Session state, interrupts and agent labels

Files: `internal/ui/agent.go`, `alerts_ui.go`, `sidebar_model.go`, `sidebar.go`,
palette models/drawing and associated tests.

- Record agent identity with each session's live agent state. Preserve the
  existing title and rename rules; clear identity on the next shell prompt.
- Show static `claude` or `codex` before a command in sidebar rows and palette
  session entries. Include identity in filtering. Never add timers or polling.
- Interrupt ends running with no done/needs alert, chime or flash. Preserve
  command/title context; ensure queued events cannot revive older running state.
- Isolate usage to Claude and retain scratch-shell parent routing.
- Test mixed-agent sessions, quick switches, shell return, interrupt ordering,
  alert suppression and scratch routing through real window code paths.

## 3. Safe Codex configuration inspection and repair

New package: `internal/codexsetup`. Share the existing ordered JSON helpers only
where this avoids divergent parsers without changing Claude behavior. Reuse the
repository TOML dependency for read-only inline hook inspection.

- Detect missing `.codex`, missing hooks.json, valid JSON/inline hooks, broken
  executable references, other commands, disabled or restrictive matcher groups,
  malformed data, and conflicting duplicate sources.
- Check PermissionRequest, Stop, UserPromptSubmit and Interrupt for the expected
  Kaze commands and agent flag. Respect existing working inline definitions.
- Fix adds missing handlers beside other programs and repoints only Kaze's
  broken handlers. Preserve unrelated events, unknown fields and commands.
- Never modify config.toml, notify, hook trust, or an inline-only configuration.
  Report actionable inline problems rather than introducing duplicate handlers.
- Preview first, then confirm, back up and write hooks.json. Recheck both source
  files before writing so changes made while a dialog is open are not lost.
- Tests use temp directories and an injected executable-existence check. Cover
  quoted Windows paths, multiple handlers, idempotence, exact source preservation,
  backups, unreadable files, stale previews and failure before replacement.

## 4. Agents settings cards

Files: `internal/ui/settings_claude.go`, a Codex settings file, `settings.go`,
`window.go`, `settings_commands.go` and tests.

- Rename Claude navigation to Agents. Shared alert controls keep their current
  config keys and persistence behavior. Clearly label Claude-only usage.
- Keep Claude Code setup behavior and add Codex setup with four status rows and
  Fix. No Fix when Codex's settings directory is absent.
- Route each setup's preview and confirmation independently. Cancel writes
  nothing. On success show a dismissible in-memory `/hooks` trust reminder.
- Read sources only on opening or refreshing the page and after a write.
- Update Shortcuts' CLI descriptions and any deep links or section tests.
- Tests exercise scrolling, status pills, preview, cancel/confirm, backups,
  reminder dismissal, absent Codex and the unchanged Claude Replace flow.

## 5. Documentation and verification

- Update README agent setup instructions and HANDOFF with the resulting scope,
  verification evidence, supported syntax and any remaining Windows checks.
- Run focused tests after each change, then `go test ./...`, `go vet ./...` and
  targeted race tests around notifications and session state.
- Build macOS and cross-build the supported Windows target where available.
- Inspect the native Agents page via an external input-blocked overlay with
  anonymous demo files. Never exercise Fix against the owner's real files.
- Repeat the real CLI lifecycle check against the finished `--agent codex`
  commands. Record separately what was proven for PermissionRequest, Interrupt
  and hook trust. Stop and consult the owner if the required transport fails.
- Inspect the diff for secrets, incidental changes and committed probe files.
  Commit/push/open a PR only when authorized; do not assume the prior screenshot
  PR's merge authorization applies to this feature.

## Completion evidence

Implemented all five steps. `go test ./...` and `go vet ./...` pass. Race tests
pass for cmd/kaze, notify, codexsetup and claudesetup, and for the new agent,
Codex settings and related notification tests in UI. The pre-existing full-UI
race failure documented above remains separate from this change. macOS arm64
and Windows amd64/arm64 builds pass.

Inspected native captures of shared controls and both setup cards using an
external input-blocked overlay and demo HOME. The real CLI retest delivered
prompt, done and interrupt with agent identity; Stop printed `{}` and ended
normally. Interrupt was triggered with SIGINT to the probe CLI alone while the
local stub response was pending. Probe processes were stopped afterwards.
Windows execution, real PermissionRequest UI and normal interactive hook-trust
review remain manual checks. No owner agent settings were modified.
