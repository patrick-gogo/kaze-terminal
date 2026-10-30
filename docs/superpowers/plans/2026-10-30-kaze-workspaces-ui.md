# Workspaces UI round 2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make workspaces easy to read and set up: a two-pane editor with a folder picker, a clearer list, a running dropdown that shows what each tab runs, and small About cleanups.

**Architecture:** All UI work stays in `internal/ui`, drawn with the existing settings `page` helpers (`p.row`, `p.ctrl`, `p.hit`). The Workspaces section gains a custom `draw` (like About) so the editor can split into two panes. The native folder picker lives in a new small package `internal/pick` behind a function variable the UI calls on a goroutine, so tests use a fake.

**Tech Stack:** Go 1.27, Gio. macOS picker via `osascript` (AppleScript `choose folder`); Windows via PowerShell's `FolderBrowserDialog` (Explorer-style under PowerShell 7). No new module dependencies.

**Spec:** `docs/superpowers/specs/2026-10-30-kaze-workspaces-ui-design.md`

## Global Constraints

- No idle CPU: no timers, tickers or polling added; status is computed when a frame draws.
- Theme tokens only (`p.col.*`, `win.ui.th.*`, `memberColor`); check Midnight and Washi renders.
- The draft model is unchanged: nothing is written until Save, Save never runs anything.
- Field ids stay `ws.tab.<key>.<what>` and `ws.cmd.<key>`; existing tests keep pressing them.
- Strict TDD; RED tests must fail safely (bounded, no real dialogs, no real processes in tests).
- Never inject global input; the real picker is only exercised by hand.
- Commits: `type(scope): Verb ...`, no AI attribution. Branch only; never push to `main`.
- Copy (exact): button `Choose…`; statuses `Starting`, `Waiting for Docker`, `Waiting for port N`, `Confirm to run`, `Running`, `Finished`, `Failed`, `Cancelled`, `No commands`; list tags `after Docker`, `after port N`.

## Review Focus

1. **Picker result for a tab that was removed or a draft that was closed** while the dialog was open: drop it silently, never write into another tab or a new draft. (Task 3 test.)
2. **Switching tabs in the editor while a field has the keyboard**: the typed text stays in the draft and the editing field is cleared, nothing typed lands in the newly selected tab. (Task 3 test.)
3. **Removing the selected tab**: selection moves to the tab now at that index (or the last), never to nothing. (Task 3 test.)
4. **A folder path with spaces, quotes or Japanese** passed to the picker as its start folder: must reach the dialog unquoted-safe (argv / env, never pasted into a script). (Task 2 test.)
5. **Dropdown status for a member whose shell never reports prompts** stays `Running`, never flips to `Finished` on unrelated output. (Task 6 test.)

---

### Task 1: About and sidebar cleanup

**Files:**
- Modify: `internal/ui/settings_about.go` (drop `text(win.set.build, 11)`; tagline spacing)
- Modify: `internal/ui/settings.go:652-674` (nav footer: `kaze 0.6` only)
- Test: `internal/ui/settings_about_test.go`

- [ ] **Step 1: Write the failing tests.** In `TestAboutSectionShowsBuildAcrossThemes`, drop `win.set.build` from the wanted texts and add: `if strings.Contains(text, win.set.build) { t.Errorf("%s still shows the build line", th.Key) }` (the nav footer is drawn by the same `drawSettings`, so this covers both places). Keep `TestAboutCopyUsesDisplayedVersionAndBuild` asserting the copy contains `build test123`.
- [ ] **Step 2: Run** `go test ./internal/ui -run TestAbout -count=1`. Expected: FAIL "still shows the build line".
- [ ] **Step 3: Implement.** Remove the build text from `drawAbout` and the build lines from `page.nav` (keep the `kaze 0.6` line and mark). Change `aboutName` to `風・"wind"` (U+30FB katakana middle dot, no spaces), then render the About page offscreen and confirm even spacing; if it still looks uneven, use `風 · wind` and pick whichever reads evenly. Shrink the centering allowance (`gtx.Dp(320)`) by the removed line's height.
- [ ] **Step 4: Run** the same command. Expected: PASS.
- [ ] **Step 5: Commit** `fix(settings): Drop the build line from About and the sidebar`.

### Task 2: Native folder picker package

**Files:**
- Create: `internal/pick/pick.go`, `internal/pick/pick_darwin.go`, `internal/pick/pick_windows.go`, `internal/pick/pick_other.go`
- Test: `internal/pick/pick_test.go`

**Interfaces:**
- Produces: `func Folder(start string) (path string, ok bool, err error)`: blocks until the dialog closes; `ok=false, err=nil` on cancel. `var ErrUnsupported = errors.New("pick: no folder picker on this system")`.
- Internal, per OS: `func folderCmd(start string) *exec.Cmd` and `func parse(out []byte, runErr error) (string, bool, error)`.

- [ ] **Step 1: Write the failing tests** (build-tagged per OS where the command differs):
  - `TestFolderCmdPassesStartAsArgument` (darwin): `folderCmd("/Users/a b/\"x\"/日本")`: `Path` ends in `osascript`, the script text (the `-e` values) does **not** contain the start path, and the last arg equals the start path exactly.
  - `TestFolderCmdPassesStartInEnv` (windows): the start path is in `cmd.Env` as `KAZE_PICK_START=<path>` and not in any arg.
  - `TestParseCancelIsNotAnError` (darwin): `parse(nil, exitErr)` where the error output mentions `-128` gives `ok=false, err=nil`; a non-cancel failure gives `err != nil`.
  - `TestParseTrimsTrailingSlashAndNewline`: `parse([]byte("/Users/me/proj/\n"), nil)` gives `"/Users/me/proj", true, nil`.
- [ ] **Step 2: Run** `go test ./internal/pick -count=1`. Expected: FAIL (package does not build).
- [ ] **Step 3: Implement.** Darwin script, with the start folder only in argv: `on run argv` / `set d to POSIX file (item 1 of argv)` / `POSIX path of (choose folder with prompt "Choose the tab's folder" default location d)` / `end run`, then `--` and the start path; an empty start omits `default location`. Windows: `pwsh` if on PATH else `powershell`, `-NoProfile -STA -Command` with a script that builds `System.Windows.Forms.FolderBrowserDialog`, sets `SelectedPath` from `$env:KAZE_PICK_START`, and writes the chosen path; hide the console window like `internal/workspaces/probe_windows.go` does. Other OSes: `Folder` returns `ErrUnsupported`.
- [ ] **Step 4: Run** `go test ./internal/pick -count=1` and `GOOS=windows go vet ./internal/pick`. Expected: PASS / clean.
- [ ] **Step 5: Commit** `feat(pick): Add a native folder picker for macOS and Windows`.

### Task 3: Editor model: selection, colors, picker results

**Files:**
- Modify: `internal/ui/settings_workspaces.go` (draft state, `draftAct`)
- Test: `internal/ui/settings_workspaces_test.go`

**Interfaces:**
- Consumes: `pick.Folder` (Task 2) through `var pickFolder = pick.Folder` in `internal/ui`.
- Produces: `wsDraft.sel int` (selected tab key; set to the first tab's key in `newDraft`); `func (d *wsDraft) selected() (int, *wsDraftTab)`; `func nextColor(tabs []*wsDraftTab) string`; actions `ws.sel` (arg = tab key) and `ws.tab.<key>.pick`; `wsPage.picking bool`, `wsPage.picked chan wsPicked` with `type wsPicked struct{ draft *wsDraft; key int; path string; ok bool; err error }`; `func (win *window) drainPicked()` (called at the start of `wsRows` / the section draw).

- [ ] **Step 1: Write the failing tests:**
  - `TestEditorSelectsFirstTabAndSwitches`: after `ws.edit`, `d.sel == f.tabKey(0)`; pressing `ws.sel` with `f.tabKey(2)` makes it 2; only tab 2's `ws.tab.<k>.title` field is in `f.win.set.fields` after a draw.
  - `TestAddTabTakesNextUnusedColorAndIsSelected`: draft colors cyan, magenta, yellow, green; `ws.addtab` gives a tab with color `blue` that is selected and editing its title.
  - `TestSwitchingTabsKeepsTypedText`: type `"api-2"` into tab 0's title while editing it, select tab 1, select tab 0: title is `"api-2"`, and `f.win.set.editing` was cleared on the switch.
  - `TestRemovingSelectedTabSelectsNeighbor`: select tab 3 of 4, remove it: `d.sel` is the new last tab's key.
  - `TestPickFillsFolderTildeAndEmptyTitle`: with `pickFolder` faked to return `filepath.Join(home, "work", "frontend")`, add a tab, press `ws.tab.<k>.pick`, wait for the fake to return (channel), draw: folder field is `~/work/frontend` (or `~\work\frontend` on Windows) and title is `frontend`; a tab that already has a title keeps it.
  - `TestPickResultForRemovedTabIsDropped`: start a pick on tab 2, remove tab 2 before the fake returns, draw: no tab's folder changed and `picking` is false. Same for Cancel closing the draft.
  - `TestSecondPickWhileOpenDoesNothing`: two presses start the fake once.
- [ ] **Step 2: Run** `go test ./internal/ui -run 'TestEditor|TestAddTab|TestSwitching|TestRemovingSelected|TestPick|TestSecondPick' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** `pick` runs `pickFolder(start)` on a goroutine (start = expanded current folder if it exists, else home), sends `wsPicked` on a buffered (1) channel and calls `win.w.Invalidate()` when `win.w != nil`. `drainPicked` applies a result only if `p.draft == res.draft` and the key still exists; errors go to `win.note("Could not open the folder picker: ...")`. Switching or removing tabs calls `win.editField("")` when the editing field belongs to another tab. `ws.addtab` uses `nextColor` and sets `sel`.
- [ ] **Step 4: Run** the same command and then `go test ./internal/ui -count=1`. Expected: PASS (existing editor tests still pass; update only ones that relied on every tab being drawn at once, by selecting the tab first).
- [ ] **Step 5: Commit** `feat(workspaces): Select one tab at a time and fill folders from a picker`.

### Task 4: Two-pane editor drawing

**Files:**
- Modify: `internal/ui/settings.go` (extract `func (p *page) sectionRows(r image.Rectangle, title, hint string, rows []settingRow) int` from `page.pane`)
- Create: `internal/ui/settings_workspaces_draw.go` (`func wsDraw(p *page, r image.Rectangle, win *window) int`, `func (p *page) wsTabItem(r image.Rectangle, t *wsDraftTab, sel, bad bool)`)
- Modify: `internal/ui/settings_workspaces.go` (`workspacesSection` sets `draw: wsDraw`; `editorRows` split into `editorHead(d, errs) []settingRow`, `tabRows(d, t, i, errs) []settingRow`, `editorFoot(d, errs) []settingRow`)
- Test: `internal/ui/settings_workspaces_test.go`

**Interfaces:**
- Consumes: Task 3's `d.sel`, `d.selected()`, `ws.sel`, `ws.tab.<k>.pick`.

- [ ] **Step 1: Write the failing tests:**
  - `TestEditorListShowsEveryTabWithCommandCount`: after `ws.edit` on the 4-tab fixture, page texts include each title and `2 commands` / `1 command`; there is one `ws.sel` control per tab, all left of the selected tab's Title field's rect.
  - `TestEditorMarksTabsWithProblems`: blank tab 2's title; the `ws.sel` control for tab 2 is marked (record the mark as text `!` drawn in the item, or expose `bad` via a test hook `wsItemBad map[int]bool` filled each frame; pick one and assert it).
  - `TestEditorShowsChooseAndNumberedCommands`: a `ws.tab.<k>.pick` control with text `Choose…` exists; texts include `Command 1` and `Command 2` for the 2-command tab; command remove buttons read `×`.
  - `TestEditorStacksWhenNarrow`: drawn in a 420-wide page, every `ws.sel` control sits above the Title field (Max.Y <= field Min.Y).
  - `TestListDrawsWithoutDraft`: with no draft, `wsDraw` draws the list rows exactly as before (existing list tests pass unchanged).
- [ ] **Step 2: Run** `go test ./internal/ui -run 'TestEditor|TestList' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** Without a draft, `wsDraw` calls `sectionRows` with `listRows()`. With one: header rows full width, then list (`gtx.Dp(200)` wide) and details side by side when `r.Dx() >= gtx.Dp(560)`, else stacked. List item: dot (`memberColor`), title (or `Untitled`), muted `plural(len(cmds),"command")` (`No commands` when all empty), red dot at the right when `errs.tab[key]` non-empty or `errs.missing[key]`; selected item gets `tint(col.accent, 0x26)`; register with `p.hit(ctlButton, rect, "ws.sel", key, false)`. `+ Add tab` under the list. Details: heading row = tab title with ↑ ↓ Remove, then `tabRows`; then a 3dp bar in the tab's color down the details' left edge over its full height. Footer (Cancel / Save, tabs error line) full width. Return the bottom y.
- [ ] **Step 4: Run** the same command and `go test ./internal/ui -count=1`. Expected: PASS.
- [ ] **Step 5: Visual check.** Render the editor offscreen (temporary test, deleted after) in Midnight and Washi with 5 tabs; confirm against mockup B.
- [ ] **Step 6: Commit** `feat(workspaces): Edit workspaces in two panes, one tab at a time`.

### Task 5: Clearer Workspaces list

**Files:**
- Modify: `internal/ui/settings.go` (`settingRow.members []rowMember`; `p.row` draws them under the label)
- Modify: `internal/ui/settings_workspaces.go` (`listRows` fills `members` instead of `memberLine` strings; drop `memberLine` if unused)
- Test: `internal/ui/settings_workspaces_test.go`

**Interfaces:**
- Produces: `type rowMember struct{ color, title, folder, cmds, tag string }`.

- [ ] **Step 1: Write the failing tests:**
  - `TestListMembersShowCommandsWithoutCutting`: a tab whose commands join to 140 runes: page texts contain every command word (no `…` inside the chain) across one or more lines.
  - `TestListMemberTags`: Docker tab shows `after Docker`, port tab `after port 8000`; neither shows `after Docker is up`.
  - `TestListMembersAlign`: in one workspace, every member's folder text starts at the same x (record x via a test-visible slice, or assert via `texts` order plus a `memberX` hook filled when drawing).
- [ ] **Step 2: Run** `go test ./internal/ui -run TestList -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** Columns: dot, title column width = widest title in the card, folder column = widest folder capped at 40% of the pane (shortened with `truncateMiddle`), commands fill the rest and wrap with `p.wrap` at ` → ` boundaries (indent continuation lines to the commands column); a single over-long command may end in `…`. Tag in `col.dim` after the commands on the last line, or on its own line if it does not fit.
- [ ] **Step 4: Run** the same command and `go test ./internal/ui -count=1`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(workspaces): Show each tab's color and full commands in the list`.

### Task 6: Running dropdown: what it runs and status

**Files:**
- Modify: `internal/ui/workspace_run.go` (`wsMember.promptAt uint64`; record in `wsEvent` when the line is accepted; `func promptCount(s *term.Session) uint64` = CwdSeq+PromptSeq)
- Create: `internal/ui/workspace_status.go` (`func (win *window) memberStatus(m *wsMember, t *tabs.Tab) (text string, needs, failed bool)`)
- Modify: `internal/ui/tabgroup.go:466-585` (row height `gtx.Dp(44)`; second line)
- Test: `internal/ui/tabgroup_test.go`, `internal/ui/workspace_run_test.go`

**Interfaces:**
- Consumes: `workspaces.TabRun` states (`StartingShell`, `Waiting`, `NeedsConfirmation`, `Launched`, `Failed`, `Cancelled`), `m.run.Line`, `m.run.Wait`, `m.run.Port`, `m.run.DepReady`.

- [ ] **Step 1: Write the failing tests** (in `workspace_run_test.go`, using the run fixture and its `prompt(i)` helper):
  - `TestMemberStatusFollowsTheRun`: before any prompt: `Starting`; Docker not up: `Waiting for Docker`; port tab: `Waiting for port 8000`; after the line is submitted: `Running`; after one more `f.prompt(i)`: `Finished`.
  - `TestMemberStatusConfirmAndFailed`: timed-out tab gives `Confirm to run` with `needs=true`; a tab whose shell exited first gives `Failed` with `failed=true`.
  - `TestMemberStatusNoCommands`: a tab with no commands gives `No commands` right after start and after later prompts.
  - `TestMemberStatusStaysRunningWithoutPrompts`: after submit, output that is not a prompt marker (plain text, `\x1b]0;title\a`) keeps `Running`.
  - In `tabgroup_test.go`, `TestGroupMenuShowsCommandsAndStatus`: with the menu open, texts include each member's joined commands (`docker compose up -d → ...`) and a status word.
- [ ] **Step 2: Run** `go test ./internal/ui -run 'TestMemberStatus|TestGroupMenu' -count=1`. Expected: FAIL.
- [ ] **Step 3: Implement.** `memberStatus` derives from `m.run.State` per the spec table; `Finished` when `State == Launched`, `Line != ""`, `t != nil` and `promptCount(t.Session) > m.promptAt`. The menu's second line: commands joined with ` → ` (from the recipe tab kept on `wsMember`, add `cmds []string` set in `launch`), shortened to fit with `…`, and the status at the right: `th.Needs` when needs, `th.UsageHigh` when failed, else `th.Muted`. Existing alert label (`needs you` / `done`) stays on the first line.
- [ ] **Step 4: Run** the same command and `go test ./... -count=1`. Expected: PASS except the known `ptyx` `TestResolveFindsTheProgramOnPath`.
- [ ] **Step 5: Visual check** of the open dropdown offscreen in Midnight and Washi.
- [ ] **Step 6: Commit** `feat(workspaces): Show what each running tab runs and how far it got`.

### Task 7: Docs, build, hand checks

**Files:**
- Modify: `README.md` (Workspaces section: two-pane editor, Choose…, statuses), `docs/HANDOFF.md` (Plan 5c line)

- [ ] **Step 1:** Update the README Workspaces text to match (no em dashes) and the HANDOFF status.
- [ ] **Step 2:** `go vet ./... && go test ./... -count=1`; `GOOS=windows go vet ./...`. Expected: clean, only the known `ptyx` failure.
- [ ] **Step 3:** Rebuild the Mac app (`tools/package-macos.sh`), install to `~/Applications/Kaze.app` with a backup, and leave the picker and editor for the owner's hand check (no global input injection).
- [ ] **Step 4: Commit** `docs: Document the two-pane workspace editor and run statuses`.
