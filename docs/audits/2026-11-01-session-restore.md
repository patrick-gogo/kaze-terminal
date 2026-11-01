# Issue #75 verification

Implemented on `feat/session-restore`, based on `main` at `ab512b0`, and submitted
for PR review at the owner's request. No owner settings changes, installed-app
replacements or releases.

## Automated coverage

Bounded tests cover snapshot round trips, field/file/version limits, failed atomic
replacement preserving the last snapshot, exclusive state ownership, coalesced saves,
closed-tab removal, opt-out removal, startup bootstrap preserving the old snapshot,
origin identity changes and stale-event rejection. Manager tests exercise structure,
selection and a background OSC 7 cwd report without a UI frame.

UI tests cover two Claude and two Codex conversations in one folder, exact resume
arguments, restored names/selection/grouping, workspace duplication prevention,
explicit-workspace precedence, missing folders and invalid snapshots, scratch hook
isolation, readiness banners, input cancellation, once-only submission, background
return to shell, toggle visibility without workspaces, and updater flush ordering
and failed-relaunch ownership. Test windows use temporary state paths and fake PTYs.

Final checks passed on the completed implementation:

- `go test ./...`
- `go vet ./...`
- `go test -race ./internal/sessionstate ./internal/tabs`
- `go test -race ./internal/ui -run 'TestRestore|TestRestored|TestScratchCannot|TestUpdateFlushes'`
- macOS arm64 native build and amd64 cgo cross-build
- Windows amd64 and arm64 cross-builds
- `git diff --check`

Build outputs are under `/tmp/kaze-restore-build`, outside the repository. Full UI
race testing was not claimed; the existing fake chime/flash counter race documented
in HANDOFF is outside the focused restoration tests.

## Real CLI probes

Used temporary isolated HOME, CLAUDE_CONFIG_DIR and CODEX_HOME directories, an empty
throwaway repository, anonymous hook capture scripts and an HTTP server bound to
127.0.0.1. No owner credentials, installed hooks or live sessions were used. The
response content and token counts came from a local stub; these were real installed
CLI processes and real persisted conversation files, not external model responses.
Only direct probe PTY input/output was used, never global keyboard or mouse input.

Versions: Claude Code 2.1.316 and Codex 0.166.0. For each tool, created two distinct
conversations in the same folder with one stubbed turn each. Each creation exited 0,
emitted SessionStart with a UUID, transcript path and `source: startup`, and wrote a
conversation file. Both tools' explicit resume commands opened those conversations
in real interactive PTYs without an initial prompt. All four idle resumes made zero
requests to the stub server. Probe PTYs were terminated after six seconds of observation.

Claude emitted `source: resume` with the exact same UUID on idle interactive resume.
Codex loaded the recorded user/assistant history, but did not emit SessionStart while
idle in this probe. A separate bounded resumed turn for each conversation exited 0
and emitted `source: resume` with the original UUID for both tools. This is why Kaze
seeds the pending restored identity rather than depending on an immediate resume hook.
There were eight stubbed generation requests in the final round trip: four creation
turns and four explicit probe follow-up turns. Idle resumption submitted no turn.

Separate missing-UUID interactive probes for both tools exited 1 and reported no saved
conversation. They did not select another conversation. An empty Claude session that
had never saved a turn was also not resumable, consistent with the missing-file shell
fallback. Earlier probe attempts stopped at first-run dialogs; only the completed
isolated round trips above establish resume behavior.

CLI help verified `claude --resume <UUID>` and `codex resume <UUID>`. Official Codex
references consulted: [CLI commands](https://learn.chatgpt.com/docs/developer-commands#codex-resume)
and [hooks](https://learn.chatgpt.com/docs/hooks). The installed CLI tests establish
which lifecycle fields and commands this implementation relies on.

## Remaining platform checks

Windows execution and a real installed-app update/restart have not been run here.
Updater HTTP/download/swap/relaunch sequencing is tested with local fixtures, and the
Windows targets are cross-built; neither substitutes for those runtime checks.
The four real CLI round trips validate CLI behavior; Kaze's full restore integration
is exercised through its UI/manager code with controlled PTYs, not a restart of the
owner's daily Kaze window. No screenshots were retaken; Settings gains a Startup toggle
and the update confirmation explains restoration, so captures showing those areas
should be refreshed for the eventual release.
