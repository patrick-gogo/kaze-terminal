# Start folder verification (issue #77)

Implemented locally on `feat/start-folder` in `/Users/jpatrickzxc/Documents/coding/kaze-terminal-start-folder`, based on
main at `858cb07` (merged session restoration), with the subsequent release metadata
incorporated before PR submission. No owner files or running sessions were modified;
no installation or feature release was performed.

## Coverage

- Config defaults and raw-string preservation; `~`, `~/` and `~\` expansion;
  absolute folders, missing folders, files and non-string values. Invalid values
  produce a warning and retain the home default.
- Fresh first tab and new plain tab routing, with the actual cwd stored in each
  tab's immutable metadata before OSC 7. Changing the default preserves older tabs.
- Workspace, explicit `--workspace` and restored-tab directories take precedence.
- A configured folder removed before tab creation falls back to home with a notice.
- Settings picker success and clear-to-home persist the key. Cancellation, picker
  errors, missing paths and empty successful results preserve the old file/value.
  Busy state prevents duplicate dialogs, including on an empty workspace list.
- Narrow layout keeps both actions inside their card and fits the long path hint.
- A real `/bin/sh -c pwd` launched through Kaze's PTY start path verified a cwd
  containing spaces and Japanese characters. It used a temporary directory, no
  injected keyboard or mouse input, and a bounded three-second deadline.

Tests were written and observed failing before their implementation. An existing
command-list test assumed no settings text could contain an ellipsis; it now permits
the literal “Choose…” button caption while continuing to reject cut command lines.

## Checks passed

- `go test ./...`
- `go vet ./...`
- `go test -race ./internal/config`
- `go test -race ./internal/ui -run 'TestStartDir|TestStartup|TestRestore|TestRestored|TestScratchCannot'`
- Native macOS arm64 and macOS amd64 cgo builds
- Windows amd64 and arm64 cross-builds
- `git diff --check`

Builds are outside the repository in `/tmp/kaze-start-folder-build`.

## Native layout inspection

Rendered Gio's real settings operations into offscreen headless GPU windows, with
anonymous paths, in Midnight and Washi at 1100×850 and 480×850. No window activation,
global input or owner settings were involved. The first pass found that the new
path and pair of actions exceeded the narrow card. One correction fitted mono path
hints and flowed oversized button groups vertically; a second pass confirmed the
new row's path and actions stay inside the card in all four variants. Existing
unrelated long settings headings/hints can still clip at minimum window width.

The temporary capture test was removed; behavior/layout regressions remain in the
suite. Preview PNGs are `/tmp/kaze-start-folder-{midnight,washi}-{1100,480}.png`.
These are QA artifacts, not replacements for the website's real application captures.
No website image was edited. Workspaces/Startup screenshots should be retaken when
this feature ships if the Startup section is shown.

The picker integration uses the existing native implementation; its asynchronous
contract was tested with fakes. Windows runtime/native picker interaction has not
been manually verified here. Full UI race testing is not claimed; the pre-existing
fake chime/flash test counter race is documented in HANDOFF.
