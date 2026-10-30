# Plan 6a: the sidebar Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace kaze's tab strip with a left sidebar: sessions grouped by git repo, a filter, and a workspaces section at the bottom for workspace run members.

**Architecture:** A pure model (`buildSide`) turns the tab list, each tab's folder/branch/repo, the alert marks, the open workspace runs, the fold state and the filter text into groups, loose rows and run sections; `sessionOrder` derives next/prev order from it. `chrome.layoutSidebar` draws the model and returns hit rectangles; the window routes clicks on them. The tab strip, the run group chip and its dropdown, and the Settings chip are removed; their jobs move to the sidebar.

**Tech Stack:** Go 1.27, Gio v0.10.3. Tests with `go test`.

**Spec:** `docs/superpowers/specs/2026-10-30-kaze-agent-sidebar-design.md` (sections 5, 7 except floating panes, 11 for `toggle_sidebar`, 15 item 1). Read it before any task.

**Not in 6a** (later plans): renaming a row and the slash-command title rule, lines added/removed, the running state and next-waiting (6b); chips and timed checks (6c); floating panes and the scratch shell (6d, so a member click shows it as the main view for now); the palette, its search box and the Commands section (6e).

## Global Constraints

- Windows first; macOS must build and behave the same where it applies. `go vet ./...` must pass with `GOOS=darwin` and `GOOS=windows`.
- Go is at `%LOCALAPPDATA%\Programs\go\bin`. Build test binaries with `GOTMPDIR` under `C:\Users\john\Desktop\kaze-wt\` (Smart App Control blocks fresh binaries elsewhere).
- Idle CPU stays at zero: no tickers, no per-frame file I/O, hover only invalidates when the hovered thing changes.
- Every color from theme tokens (`c.th.*`, `c.memberColor`); all five themes, including the light Washi, must read well.
- Tests never read or write `~/.claude/settings.json`, `%APPDATA%\kaze\config.toml` or `workspaces.toml`, never send input to the owner's windows, never open visible windows, never start real Docker. RED tests must fail safely and be bounded (no unbounded loops or allocations).
- Commits: conventional, capitalized verb (`feat(ui): Draw the sidebar`). No AI attribution lines of any kind.
- No em dashes in user-visible text, docs or comments. Comments say why, in 1 to 2 lines.

## Review Focus

1. Closing the shown session must land on another session in sidebar order, never on a workspace member; closing the last session with only members left shows the first member (Task 5).
2. A tab with no folder report yet, or a folder outside any repo, is a loose row titled by its shell title; a tab whose folder moves to another repo changes group on the next report (Task 2).
3. More rows than fit: the list scrolls with the wheel, and selecting a row by keyboard (next/prev) scrolls it into view (Task 4, Task 5).
4. Hovering the sidebar on an idle window: one frame per hovered-row change, none while the pointer rests (Task 5).
5. The filter hides the shown session's row: the main view stays on it, and clearing the filter brings the row back highlighted (Task 6).

---

### Task 1: Repo identity for any tab

**Files:**
- Modify: `internal/gitinfo/gitinfo.go`
- Modify: `internal/ui/status.go` (the `locs` cache)
- Test: `internal/gitinfo/gitinfo_test.go`, `internal/ui/status_test.go`

**Interfaces:**
- Produces: `gitinfo.Repo(dir string) (key, name string)`; `key` is the absolute common git folder (`""` outside a repo), `name` the folder name to show.
- Produces: `func (win *window) locOf(t *tabs.Tab) locEntry`, with `locEntry` gaining `repo, repoName string`. `statusParts` uses it; Task 2's caller uses it for every tab.

- [ ] **Step 1: Write the failing tests** in `gitinfo_test.go`, building repos in `t.TempDir()` from files (no `git` binary):

```go
func TestRepoOfAPlainCheckout(t *testing.T)      // dir/.git/HEAD exists -> key == abs(dir/.git), name == "proj"
func TestRepoOfASubfolder(t *testing.T)          // dir/proj/sub -> same key and name as dir/proj
func TestRepoOfAWorktreeIsTheMainRepo(t *testing.T) // wt/.git file "gitdir: <main>/.git/worktrees/wt", that folder has commondir "../.." -> key == abs(main/.git), name == "main"
func TestRepoOfASubmodule(t *testing.T)          // .git file -> <super>/.git/modules/sub, no commondir -> key == that folder, name == "sub" (the checkout's top folder)
func TestRepoOutsideARepo(t *testing.T)          // -> "", ""
func TestRepoSelfPointingGitFile(t *testing.T)   // .git file pointing at itself -> "", "" and returns
```

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/gitinfo/ -run Repo`
Expected: FAIL, `undefined: Repo`.

- [ ] **Step 3: Implement `Repo`** reusing the walk in `Branch` (same `maxDepth`, `readSmall`, `parseGitdir`). Resolve the git folder; if it holds a `commondir` file, the common folder is `commondir` resolved against it (read once, not followed further). Name: when the common folder's base is `.git`, the base of its parent; otherwise the base of the checkout's top folder. Never spawn `git`.

- [ ] **Step 4: Run them to see them pass**

Run: `go test ./internal/gitinfo/`
Expected: PASS.

- [ ] **Step 5: Write the failing test** `TestLocOfCachesPerFolderReport` in `internal/ui/status_test.go`: with `win.branchFn` and a new `win.repoFn func(string) (string, string)` counting calls, `locOf` called 3 times for one tab with the same cwd/seq calls each once; a new `CwdSeq` calls them again.

- [ ] **Step 6: Implement `locOf`** by moving the cache code out of `statusParts` (lines 46-60 today) into it and adding the repo lookup next to the branch lookup; `statusParts` calls `locOf`.

- [ ] **Step 7: Run** `go test ./internal/ui/ -run 'LocOf|Status'` → PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/gitinfo internal/ui/status.go internal/ui/status_test.go
git commit -m "feat(gitinfo): Find the repo a folder belongs to, worktrees included"
```

### Task 2: The sidebar model

**Files:**
- Create: `internal/ui/sidebar_model.go`
- Test: `internal/ui/sidebar_model_test.go`

**Interfaces:**
- Consumes: `locEntry` (Task 1), `mark` (`alerts.go`), `memberStatus` (`workspace_status.go`).
- Produces:

```go
type sideRow struct {
	ID                   tabs.TabID
	Title, Folder, Branch string // Folder already ~-shortened
	Mark                 mark
}
type sideGroup struct {
	Key, Name string
	Rows      []sideRow // filtered
	Count     int       // all members, before the filter
	Waiting   int       // members with markNeeds
	Folded    bool      // as drawn: false while a filter is active
}
type sideMember struct {
	ID            tabs.TabID // 0 when its shell never started
	Title, Color  string
	Status        string // memberStatus text, "Finished" shown as "stopped"
	Down          bool   // stopped, failed or its shell exited
	Mark          mark
}
type sideRun struct {
	Run     uint64
	Name    string
	Down    int
	Members []sideMember
}
type sideModel struct {
	Groups []sideGroup
	Loose  []sideRow
	Runs   []sideRun
}
type sideInput struct {
	Tabs     []*tabs.Tab
	Loc      func(*tabs.Tab) locEntry
	Mark     func(tabs.TabID) mark
	Member   func(*tabs.Tab) (run uint64, ok bool) // is t a workspace run member
	Runs     []sideRun                             // built by the window from win.runs
	Folded   map[string]bool                       // by group Key
	Filter   string
	Home     string
	DefTitle string
}
func buildSide(in sideInput) sideModel
func sessionOrder(m sideModel) []tabs.TabID // groups then loose, every row, ignoring fold and filter
```

`sessionOrder` must be computed from a model built with `Filter: ""`; the window builds it that way for keys (Task 5).

- [ ] **Step 1: Write the failing tests** with fake `Loc`/`Mark`/`Member` funcs and `tabs.Tab` values built directly (no shells):

```go
func TestSideGroupsByRepoInTabOrder(t *testing.T)
// tabs a(repo X), b(no repo), c(repo Y), d(repo X) -> Groups [X:{a,d}, Y:{c}], Loose [b]
func TestSideOneMemberStillMakesAGroup(t *testing.T)        // single tab in repo -> 1 group, header shown
func TestSideNoFolderReportIsLoose(t *testing.T)            // Loc returns cwd "" -> Loose, Title from tabLabel
func TestSideMembersAreNotSessions(t *testing.T)            // Member ok -> not in Groups/Loose, nor sessionOrder
func TestSideWaitingCount(t *testing.T)                     // one markNeeds in group X -> Waiting == 1
func TestSideFoldIsKeptButFilterOpensIt(t *testing.T)       // Folded[X]=true -> Folded true; with Filter "a" -> Folded false
func TestSideFilterMatchesTitleFolderBranch(t *testing.T)   // case-insensitive; Count unchanged, Rows narrowed; empty groups dropped
func TestSideOrderIgnoresFoldAndFilter(t *testing.T)        // sessionOrder == [a, d, c, b]
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run Side` → FAIL, undefined `buildSide`.

- [ ] **Step 3: Implement** `buildSide` and `sessionOrder` in `sidebar_model.go`. Branch shown without a leading `feature/`, `bugfix/`, `fix/`, `feat/`, `chore/` or `docs/` segment (spec 5.2). Titles via `tabLabel(t, in.DefTitle)`; folders via `tildeHome`. `Runs` passes through unchanged.

- [ ] **Step 4: Run** `go test ./internal/ui/ -run Side` → PASS.

- [ ] **Step 5: Write the failing test** `TestSideRunsFromWindowRuns` for `func (win *window) sideRuns() []sideRun`: one run with members Running, Finished (shown as `stopped`, Down), Failed (Down) and an exited shell (Down) -> `Down == 3`; runs ordered by run id; a run with no member tab open is left out.

- [ ] **Step 6: Implement `sideRuns`** in `sidebar_model.go` from `win.runs` and `memberStatus`.

- [ ] **Step 7: Run** `go test ./internal/ui/ -run Side` → PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/ui/sidebar_model.go internal/ui/sidebar_model_test.go
git commit -m "feat(ui): Model the sidebar's repo groups, rows and workspace runs"
```

### Task 3: Config for the sidebar

**Files:**
- Modify: `internal/config/config.go`, `internal/ui/logic.go`
- Test: `internal/config/config_test.go`, `internal/ui/logic_test.go`

**Interfaces:**
- Produces: `config.Config.Sidebar bool` (TOML `sidebar`, default `true`); action `actToggleSidebar` named `toggle_sidebar`.

- [ ] **Step 1: Write the failing tests**
  - `TestSidebarDefaultsOnAndReadsFalse`: no key -> `Sidebar == true`; `sidebar = false` -> false; `sidebar = "no"` -> warning and true.
  - `TestToggleSidebarDefaults`: `defaultBindings("windows")["toggle_sidebar"] == {Name: "B", Mods: ModCtrl|ModShift}`, `defaultBindings("darwin")["toggle_sidebar"] == {Name: "B", Mods: ModCommand}`; `config.KnownActions["toggle_sidebar"]`.

- [ ] **Step 2: Run** `go test ./internal/config/ ./internal/ui/ -run 'Sidebar'` → FAIL.

- [ ] **Step 3: Implement**: the key, its default, `KnownActions`, `actionByName`, `defaultBindings` for both OSes.

- [ ] **Step 4: Run** → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config internal/ui/logic.go internal/ui/logic_test.go
git commit -m "feat(config): Add the sidebar setting and its toggle shortcut"
```

### Task 4: Draw the sidebar

**Files:**
- Create: `internal/ui/sidebar.go`
- Test: `internal/ui/sidebar_test.go`

**Interfaces:**
- Consumes: `sideModel` (Task 2).
- Produces:

```go
type sideState struct {
	shown     tabs.TabID // highlighted row or member
	settings  bool       // the pinned "Settings" row is open
	settingsOn bool      // and shown
	hover     tabs.TabID // row showing its ×
	scroll    int        // px the session list is scrolled
	filter    string     // as typed, for the placeholder/clear state
	buttons   bool       // macOS: toggle and gear sit in the sidebar's top row
}
type sideHits struct {
	rows      []sideRowHit // id, rect, close rect (close empty unless hovered)
	groups    []sideKeyHit // group Key, header rect
	members   []sideRowHit // member id, rect
	runs      []sideRunHit // run id, header rect
	settings, settingsClose image.Rectangle
	plus, filter, toggle, gear image.Rectangle
	list      image.Rectangle // the scrollable area, for wheel events
	scrollMax int
}
func (c *chrome) layoutSidebar(gtx layout.Context, m sideModel, st sideState, size image.Point) sideHits
```

Sizes (Claude's calls within the spec, 100% scale): width `sidebarW = 260` dp; row padding 8; group header 28 dp; session row three lines (12 sp title, 11 sp folder, 11 sp branch line); member line 24 dp; section header 26 dp. State marks per spec 5.2: amber dot with a soft halo for needs, green dot for done, muted dot otherwise (running arrives in 6b). Group header: dot stack (max 8, `maxStackDots`), name, count or amber `N waiting`. Workspaces section pinned to the bottom, header `WORKSPACES`, then per run its name and `all up` (green) or `N down` (red), then member lines: color bar (`c.memberColor`), title, status right-aligned (`stopped` and `Failed` in red). Text cut with `cutRunes`/`c.fit` so nothing overflows 260 dp.

- [ ] **Step 1: Write the failing tests** using the headless frame helper from `settings_frame_test.go`:

```go
func TestSidebarHitsMatchRows(t *testing.T)            // 2 groups + 1 loose -> len(rows)==3, group hits==2, rows inside size
func TestSidebarFoldedGroupHasNoRows(t *testing.T)     // Folded -> its rows absent, header hit present
func TestSidebarCloseOnlyWhenHovered(t *testing.T)     // hover=a -> a's close rect non-empty, others empty
func TestSidebarScrollsAndClampsAt(t *testing.T)       // 40 rows in 600px -> scrollMax > 0; scroll beyond is clamped; rows above the list are not hit
func TestSidebarSectionAtTheBottom(t *testing.T)       // run with 4 members -> 4 member hits, all below every row hit
func TestSidebarSettingsRowFirst(t *testing.T)         // settings open -> settings rect above the first group
func TestSidebarLongTitleStaysInside(t *testing.T)     // 200-rune title and CJK title -> row rect within width, no panic
func TestSidebarDrawsInEveryTheme(t *testing.T)        // each of the five themes: renders without panic, and the selected row's fill differs from the sidebar background
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run Sidebar` → FAIL.

- [ ] **Step 3: Implement `layoutSidebar`** in `sidebar.go`, following `layoutTabBar`'s use of `c.label`, `place` and `paint.FillShape`. The session list is clipped to `hits.list` and offset by `-scroll`; hits are recorded only for visible parts.

- [ ] **Step 4: Run** `go test ./internal/ui/ -run Sidebar` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/sidebar.go internal/ui/sidebar_test.go
git commit -m "feat(ui): Draw the sidebar with repo groups and the workspaces section"
```

### Task 5: Put the sidebar in the window

**Files:**
- Modify: `internal/ui/window.go` (`frame`, `drawBar`, `barEvents`, `barPress`), `internal/ui/titlebar.go`, `internal/ui/settings.go` (`cycleTab`), `internal/ui/logic.go` (`runShortcut`), `internal/ui/tabbar.go`
- Delete: `internal/ui/tabgroup.go` and the tab-strip parts of `tabbar.go` (`layoutTabBar`, `activeTab`, `settingsChip`, `tabLabelFit`), with their tests in `tabgroup_test.go`, `titlebar_test.go`, `tabfonts_test.go` replaced by sidebar tests
- Test: `internal/ui/sidebar_window_test.go`

**Interfaces:**
- Consumes: `buildSide`, `sessionOrder`, `sideRuns` (Task 2), `layoutSidebar` (Task 4), `Config.Sidebar`, `actToggleSidebar` (Task 3).
- Produces: `win.side` (state: `sideState`, folded `map[string]bool`, `hidden bool`, `hits sideHits`, a pointer tag); `func (win *window) closeTab(id tabs.TabID)`; `func (win *window) cycleTab(d int)` reworked; `barLayout` gains `Toggle image.Rectangle`.

Layout (spec 5.1):
- **Windows (custom title bar):** the top bar keeps its height; left to right: sidebar toggle button, the K mark, drag area, gear, caption buttons. The sidebar sits under the bar, left of the terminal, above the status bar.
- **macOS (native title bar):** no kaze top bar; the toggle and gear are in the sidebar's top row (`sideState.buttons`). While hidden, a rail 36 dp wide keeps the toggle and gear.
- The sidebar is hidden while `Config.Sidebar` is false (toggle saves it with `config.Set`) and, without saving, while the window is narrower than 560 dp.

Behavior:
- Row click shows that session (`selectTab`); row × or middle click closes it (`closeTab`); group header click toggles `folded[key]`; `+` opens a new session; member click shows that member as the main view; the Settings row shows settings, its × closes them; wheel over `hits.list` scrolls.
- `closeTab(id)`: when `id` is shown, first pick the next session after it in `sessionOrder` (else before it), then close; with no session left, show the first open member; then `win.mgr.Close(id)`. `runShortcut` and the sidebar both use it.
- `cycleTab(d)`: steps through `sessionOrder` wrapping, Settings (while open) at the start of the order as today; from a shown member, next goes to the first session and prev to the last.
- Selecting a row by keyboard scrolls it into view.
- Pointer: Press, Scroll, Move, Enter, Leave on the sidebar tag; Move/Enter/Leave invalidate only when `hover` changes.
- The window title, toasts' "Go ›", the workspace banner and alerts keep working unchanged.

- [ ] **Step 1: Write the failing tests** with `testWindow` and `recStarter`, feeding pointer events at the hit rects from one headless frame:

```go
func TestClickingARowShowsIt(t *testing.T)
func TestClosingTheShownSessionSkipsMembers(t *testing.T)   // order [a, member m, b] in manager; close a -> shown b, not m
func TestClosingTheLastSessionShowsAMember(t *testing.T)
func TestNextPrevFollowSidebarOrder(t *testing.T)           // tabs opened a(X), b(none), c(X) -> next from a is c, then b
func TestNextFromAMemberGoesToTheFirstSession(t *testing.T)
func TestGroupHeaderFolds(t *testing.T)                     // click header -> folded; click again -> open
func TestToggleSidebarHidesAndSaves(t *testing.T)           // ctrl+shift+b -> hidden; config.Set called with sidebar=false (fake writer, never the real file)
func TestNarrowWindowHidesTheSidebarWithoutSaving(t *testing.T)
func TestHoverInvalidatesOnlyOnChange(t *testing.T)         // 5 Move events over one row -> 1 invalidate; move to next row -> 2
func TestKeyboardSelectionScrollsIntoView(t *testing.T)     // 40 sessions, cycle to the last -> its row rect inside hits.list
func TestMacHasNoTopBar(t *testing.T)                       // custom=false -> terminal starts at y=0, toggle and gear hits in the sidebar
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Row|Closing|NextPrev|NextFrom|GroupHeader|ToggleSidebar|Narrow|Hover|KeyboardSelection|MacHas'` → FAIL.

- [ ] **Step 3: Implement**: the sidebar in `frame` (building the model twice per frame, unfiltered for order and filtered for drawing, is fine because `locOf` is cached), the title bar changes, the events, `closeTab`, `cycleTab`. Remove the tab strip, the group chip, its dropdown (`drawGroupMenu`, `groupEvents`, `groupKey`, `pruneGroupMenu`) and the Settings chip; `focusTarget` drops the menu case.

- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (the whole package, so removed code leaves no broken test).

- [ ] **Step 5: Run** `GOOS=darwin go vet ./... && GOOS=windows go vet ./...` → no output.

- [ ] **Step 6: Commit**

```bash
git add -A internal/ui
git commit -m "feat(ui): Replace the tab strip with the sidebar"
```

### Task 6: The filter field

**Files:**
- Modify: `internal/ui/sidebar.go`, `internal/ui/window.go` (including `focusTarget`, moved there in Task 5)
- Test: `internal/ui/sidebar_filter_test.go`

**Interfaces:**
- Produces: `win.side.ed widget.Editor` (`SingleLine: true`), drawn in `hits.filter` with placeholder `Filter sessions…`; `win.side.filtering bool` (the field has the keyboard).

Behavior (spec 5.4): clicking the field gives it the keyboard; every edit rebuilds the model with `Filter`; Esc clears the text and gives the keyboard back to the shown session; Enter shows the first visible row and gives it the keyboard; a click on the terminal leaves the text and gives the keyboard back. kaze's own shortcuts still work while it has the keyboard. Follow the editor handling in `controls.go` (`fieldEvents`, `drainField`) for focus and key filtering.

- [ ] **Step 1: Write the failing tests**

```go
func TestFilterNarrowsRows(t *testing.T)               // type "33" -> only rows containing "33"
func TestFilterEscClearsAndReturnsKeys(t *testing.T)   // Esc -> text "", next key reaches the session's PTY
func TestFilterEnterShowsFirstMatch(t *testing.T)
func TestFilterKeepsTheShownSession(t *testing.T)      // filter hides shown row -> shown unchanged; clear -> row back, highlighted
func TestFilterNoMatches(t *testing.T)                 // "zzz" -> a "No matches" line, no row hits
func TestShortcutsWorkWhileFiltering(t *testing.T)     // ctrl+shift+t while field focused -> new session, text kept
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run Filter` → FAIL.

- [ ] **Step 3: Implement** the field, its events and the focus rules.

- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): Filter the sidebar's sessions"
```

### Task 7: Workspace runs live in the section

**Files:**
- Modify: `internal/ui/workspace_run.go` (`runWorkspace`), `internal/ui/status.go`, `internal/ui/tabbar.go` (`layoutStatus`)
- Test: `internal/ui/workspace_run_test.go`, `internal/ui/status_test.go`

**Interfaces:**
- Consumes: `sideRuns` (Task 2).
- Produces: `statusParts.Servers []serverDot` (`Down, Up bool`); `layoutStatus` returns the dots' hit rect, stored in `win.serverDots`.

Behavior (spec 7):
- `runWorkspace` opens the members without changing the shown session: remember the shown tab before opening and `SetActive` it back after. When there was none (startup with a workspace), show the first member. Running a workspace that is already open shows its first open member, as today.
- Status bar: one 7 dp dot per member of every open run, green (Running), red (Down), muted (anything else), then the word `servers`, right-aligned before the usage numbers; nothing when no run is open. Clicking shows the first Down member, or the first member when none is down.

- [ ] **Step 1: Write the failing tests**

```go
func TestRunningAWorkspaceKeepsTheShownSession(t *testing.T)   // shown a; run a 3-member workspace (fake probes) -> shown still a; 3 members in sideRuns
func TestStartupWorkspaceShowsItsFirstMember(t *testing.T)
func TestServerDotsColors(t *testing.T)                        // Running, stopped, Starting -> green, red, muted
func TestServerDotsClickShowsTheDownMember(t *testing.T)
func TestNoServerDotsWithoutARun(t *testing.T)
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'WorkspaceKeeps|StartupWorkspace|ServerDots'` → FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(workspaces): Show runs in the sidebar's section and the status bar"
```

### Task 8: Docs and version

**Files:**
- Modify: `README.md` (tabs and shortcuts sections, the `[keys]` table, the config table with `sidebar`), `docs/HANDOFF.md`, `internal/version/version.go`
- Test: `internal/version/version_test.go`

- [ ] **Step 1: Update `version_test.go`** to expect `0.7`; run `go test ./internal/version/` → FAIL.
- [ ] **Step 2: Set `Number = "0.7"`**; run → PASS.
- [ ] **Step 3: Update the README**: the sidebar replaces tabs (rows, groups, fold, filter, the workspaces section, server dots), `toggle_sidebar` (Ctrl+Shift+B / Cmd+B) in the shortcut list and `[keys]` table, `sidebar = true` in the config table. Remove text about the tab strip and the group dropdown. Note that the `.syso` file still carries the old version until the owner reruns go-winres.
- [ ] **Step 4: Update `docs/HANDOFF.md`** with what 6a shipped and that 6b to 6e are next.
- [ ] **Step 5: Run** `go test ./...` → PASS except the known `ptyx` `TestResolveFindsTheProgramOnPath` failure that also fails on `main`.
- [ ] **Step 6: Commit**

```bash
git add README.md docs/HANDOFF.md internal/version
git commit -m "docs: Describe the sidebar and bump kaze to 0.7"
```
