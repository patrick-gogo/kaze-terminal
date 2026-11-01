# Automatic session restoration (issue #75)

Status: approved by owner on 2026-11-01; implemented locally on `feat/session-restore`.
Plan: `../plans/2026-11-01-kaze-session-restore.md`. Verification: `../../audits/2026-11-01-session-restore.md`.

## Outcome

Opening Kaze brings back the sessions from the previous run automatically, including
after Update and restart. Claude Code and Codex reopen their exact conversations,
ready for input. Ordinary terminals reopen as fresh shells in their last known folders.

Restoration is on by default. Settings > Workspaces gets a Startup card with
“Restore sessions on launch” (`restore_sessions = true`). Its hint explains that
conversations and folders return, while running commands stop when Kaze closes.
Turning it off clears saved restoration state and prevents further snapshots until
enabled again. Existing startup-workspace selection stays in that card.

This changes the earlier no-restoration decision in the terminal and sidebar specs.
It does not introduce a background terminal host. Running commands, server processes,
shell variables, editor state and terminal scrollback do not survive closing Kaze.
Interrupted agent turns are not automatically submitted again.

## What returns

- Main sessions in their existing order, with last known cwd, custom name, color and
  the previously selected session. IDs are remapped to fresh tab IDs on startup.
- Exact Claude/Codex conversation identity when a main-terminal hook supplied it.
  Resumption never uses a folder's latest conversation or a conversation name.
- Workspace members return with their saved grouping and display metadata, as fresh
  shells or exact resumed conversations. Their recipe commands and dependency waits
  are not replayed. Running a restored workspace selects its existing members.
- Scratch shells, floating geometry, subagent cards and team orchestration are excluded.
  The main agent's own saved conversation may include historical subagent results;
  Kaze does not launch or claim to restore live helpers. Issue #53 retains its scope.

## Startup rules

1. An explicit `--workspace` starts only the requested recipe, preserving current
   error behavior if it cannot start. This run becomes the next saved session set.
2. Otherwise, enabled restoration with a usable nonempty snapshot wins over
   `startup_workspace`. Never launch both automatically.
3. With no usable restored sessions, use `startup_workspace`, then the normal shell
   fallback. An intentionally empty snapshot follows this same fallback.
4. Missing individual folders open shells in the normal default folder with a visible
   explanation and no automatic agent launch in a different folder. Missing tools or
   missing/invalid conversation identities leave usable shells with an explanation.
   Other entries continue restoring. Malformed or future-version snapshots produce
   a startup warning and use the normal startup path.

Only one Kaze instance owns the saved session set for a configuration directory.
Use an OS-released exclusive lock, held for that run. A second independently launched
instance opens fresh (or its explicit workspace), reports that restoration belongs
to the first instance, and neither restores nor writes the owner's snapshot. No
polling loop is required; process exit releases the lock even after a crash.

## Reliable identity and resume

Installed CLI help checked on 2026-11-01: Claude Code 2.1.316 accepts
`claude --resume <session UUID>`; Codex 0.166.0 accepts `codex resume <session UUID>`.
Both commands must omit a prompt, continuation flags and permission overrides.
[Official Codex CLI reference](https://learn.chatgpt.com/docs/developer-commands#codex-resume)
documents explicit session selection; [Codex hooks](https://learn.chatgpt.com/docs/hooks)
documents SessionStart, including resume and clear/fork lifecycle sources.

Kaze already forwards `session_id`, `transcript_path` and `cwd`, and both setup
checkers include SessionStart. Before relying on this, verify real startup and resume
payloads from both installed CLIs in isolated test homes. No owner settings change.
Check invalid-ID behavior as well: a failed resume must not silently create a new
conversation. Keep unsupported cases as shell fallbacks with an explanation.

Store restoration identity against the originating terminal ID before scratch hooks
are mapped to their parent's sidebar row. Scratch/subagent messages must never
replace the main terminal's saved identity. Clear old identity when switching agent
tools or receiving a new SessionStart without a valid identity, and stop treating a
conversation as live after its terminal reports a newer shell prompt. Process-name
detection alone cannot supply a conversation ID.

Use the existing supported-shell quoting and prompt-readiness machinery for fixed
resume commands with validated UUID arguments. Paths go through the shell start cwd,
never through executable command text. Type only once after the first prompt marker;
typing by the user cancels a pending launch. Without a prompt marker, show the existing
readiness banner after three seconds and require its Run action. Unknown shells keep
a shell with an explanation. Normal working shells resume without a new confirmation.
Do not persist arbitrary commands or original agent launch flags.

## State and lifecycle

Add an `internal/sessionstate` package for versioned `sessions.json` beside
`config.toml`. Keep a maximum of 128 entries, a 1 MiB file cap and bounded string
fields, with visible warnings for exceeded limits. Store only cwd, custom title,
color, workspace display identity, selected entry and optional agent/UUID/transcript
path. No transcript contents, output buffers, prompts, credentials or environment.

Write a restricted-permission temporary file in the same directory and replace the
snapshot atomically using a Windows-compatible implementation. A failed write leaves
the previous snapshot intact. Use one worker and a replaceable latest-state slot,
with coalesced saves after meaningful changes (250 ms), no periodic polling. Cache
structural state; do not scan transcripts or serialize from rendering code.

Include best-effort crash recovery from the latest completed save. Changes during the
last coalescing interval can be lost in a crash. Observe creation, close, selection,
rename, cwd, agent identity and return-to-shell events, including while minimized.
Before shutdown, drain pending identity/cwd events and flush a final snapshot before
closing shells. Do not snapshot the empty manager created by teardown. Explicit tab
closure saves its removal; quitting with open tabs saves those tabs.

On update, collect and flush the latest snapshot on the UI goroutine after download
and installation, immediately before relaunch. Use an explicit updater handoff so the
new process waits for the old owner's shells and state lock to be released before it
restores. Freeze terminal mutations during this short handoff. The old process must
not overwrite the snapshot afterward. If relaunch fails, retain ownership and restore
normal interaction. Bound the handoff wait; on failure keep the snapshot intact and
show an actionable startup error instead of duplicating agent sessions.

Restoration runs once per process. Benchmark mode and test windows do not access the
owner's snapshot or locks. Tests inject temporary state paths, shell starts and clocks.

## Verification and delivery

Write bounded failing tests before implementation for snapshot limits and round trips,
atomic write failure, locking and updater handoff, exact conversation matching,
scratch isolation, agent changes, shell return, closed-session removal, disabled
restoration, startup precedence, workspace grouping, readiness cancellation and
per-entry failure recovery. Cover minimized-event updates and crash-save coalescing.

Run `go test ./...`, `go vet ./...`, focused race tests, and macOS and Windows builds.
Use isolated CLI/PTY integration probes for two Claude and two Codex conversations in
one repo. No global input injection, owner session restart or owner credentials.
Record which lifecycle behaviors are real versus simulated. Windows runtime and a
real installed updater restart require platform verification; cross-builds do not
satisfy those checks.

After implementation, update README, HANDOFF and the superseded spec decisions.
Prepare the feature for review without committing or pushing unless asked. Version
and changelog changes follow the release workflow after the feature PR merges.
