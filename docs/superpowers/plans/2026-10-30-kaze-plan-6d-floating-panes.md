# Plan 6d: floating panes and the scratch shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Workspace members open as floating panes over the current session (drag, resize, raise, Expand, ×), and each session gets one scratch shell that a key toggles as a floating pane.

**Architecture:** A per-window `floats` model keeps one `paneSet` per session (panes in open order, each with a dp rectangle relative to the terminal area and a z stamp). Each visible pane draws its tab with its **own** `render.Renderer` and has its **own** PTY size; the shown session keeps the existing renderer and the shared size. Terminal input is refactored to work on a `termView` (tab, tag, grid, size), so the shown session and every pane use the same key, paste, mouse and selection code. A scratch shell is an ordinary tab with `Tab.Parent` set: hidden from the sidebar and next/prev, and its hook messages are counted on the parent row.

**Tech Stack:** Go 1.27, Gio v0.10.3. Tests with `go test`.

**Spec:** `docs/superpowers/specs/2026-10-30-kaze-agent-sidebar-design.md`, sections 7 (member clicks, server dots), 8 (floating panes, the scratch shell), 11 (`scratch_shell`), 14 (decisions) and 15 (left to the builder). Read them before any task. Look and behavior: the 6d column of `docs/design/agent-6b-6d-6e-mockup.html` (its JS: `openDrawer`, `toggleScratch`, `renderFloats`). The mockup was committed on `feat/agent-awareness`; until Plan 6b merges, read it with `git show feat/agent-awareness:docs/design/agent-6b-6d-6e-mockup.html`. **Where the mockup and the spec differ, the spec wins:** Esc goes to the program and never closes a pane (spec 8.1), and a member pane's × only hides it (spec 8.2; the mockup removes it, which is the same thing for us because the member keeps running).

**Branch:** from `main` (Plan 6a merged), e.g. `feat/floating-panes`. Plan 6b (`feat/agent-awareness`) is in progress in parallel; this plan must not depend on anything 6b adds (`agent.go`, `agentState`, row rename, `next_waiting`, line counts, `dropSideEdit`). See "Merge notes with 6b" below.

**Not in 6d:** the palette and its "open a member floating" action (6e uses `floatMember` from Task 7); chips (6c, parked); saving panes across restarts (spec 3, non-goal); real tiled splits (non-goal).

## Decisions (made by this plan; the spec left them to the builder)

1. **One renderer per visible pane, not a shared one.** `render.Renderer` keeps per-session strip images and consumes the screen's dirty rows (`Screen.TakeDirty`), so one renderer drawing several sessions per frame would re-rasterize every pane in full on every frame and steal each session's dirty rows. Each visible pane gets its own `*render.Renderer` built with the same font size, palette and options as the main one; glyphs and fonts are already shared between renderers of one size (`sharedGlyphs`), so the extra cost is strip memory only (a 560x260 dp pane at 150% is about 1.3 MB). A pane's renderer is dropped when the pane hides or its session is switched away, and all pane renderers are dropped when the main one is rebuilt (theme, font size, DPI). **Invariant:** a tab is drawn by at most one renderer per frame (a member that becomes the main view leaves every pane set first). `internal/render` does not change.
2. **Pane sizes and resize.** Each pane's tab has its own PTY size: `tabs.Manager.ResizeTab(id, cols, rows)` marks the tab as sized on its own, and `ResizeAll` (the window size) skips it. `Manager.Dock(id)` puts a tab back on the shared size; the frame docks the shown tab every frame (a no-op unless it was a pane), and closing a member pane docks it. The pane grid is the pane rectangle minus the 1 px border, the 30 dp header and the same padding as the main view (8 dp sides, 4 dp top and bottom), through `gridSize`. Resizing a pane resizes its PTY as soon as the cell count changes, like a window resize.
3. **Geometry is kept in dp relative to the terminal area's top-left**, so hiding the sidebar or moving to another monitor keeps panes in place. The stored rectangle is never changed by a smaller window: each frame draws `fitPane(stored, area)` (shrunk to the area, then moved inside it), so panes come back to their size when the window grows (spec 8.1 "panes move and shrink to fit"). A drag or resize starts from the fitted rectangle and stores the result.
4. **Sizes** (dp, at 100%): minimum 280 x 140; header 30; corner radius 8; a member pane opens at 560 x 260, at (80 + 30n, 40 + 30n) where n is the number of visible panes (the mockup cascade); the scratch shell opens at 520 x 240 at (150, 80).
5. **Hit areas.** A 6 dp band on each of the four edges resizes that edge; a 14 dp square at each corner resizes both edges; the rest of the header drags; the header's Expand and × buttons sit on the right; everything else is the terminal. Edge and corner zones are drawn after the header and body so they win. Each zone sets the matching Gio cursor (`pointer.CursorNorthResize`, `CursorNorthWestResize`, `CursorGrab` on the header, and so on) as a static op, so hovering never asks for a frame.
6. **z-order.** One window-wide counter stamps `z` when a pane opens, is shown, or is pressed anywhere. Panes draw in ascending z, so Gio hit-tests the front pane first. The front pane is the highest z among the visible panes of the shown session; it gets the `Accent` border, the others `Border`.
7. **Keyboard.** `floats.keys` is the pane tab with the keyboard (0: the session). Pressing anywhere in a pane raises it and gives it the keyboard; pressing the session's terminal gives it back; switching sessions gives it to the new session. Shortcuts keep working from a pane (`onKey` goes through `routeKey` as today). Esc and every other key go to the program in the pane. `copy`/`paste` act on the view with the keyboard; `close_tab`, `next_tab` and `prev_tab` act on sessions (a pane closes only with its × or, for the scratch shell, its toggle key, spec 8.1). Focus-in/out reports move with the keyboard (`sendFocus`).
8. **Per-session pane memory.** `floats.sets` maps a session's tab ID to its `paneSet`. Switching sessions hides the old set (renderers dropped, tabs no longer visible) and shows the new one where it was. Sets live in memory only and are dropped when their session closes. While the main view is a workspace member (after Expand) or Settings, no panes are drawn and the scratch key does nothing; a member click then shows the member as the main view (6a behavior), because there is no session to float over.
9. **At most 4 panes per session** (`maxPanes = 4`), counting the scratch shell even while hidden (so toggling it back never pushes a member out). Opening a fifth closes the oldest **member** pane (by open order) and docks its tab; the scratch shell is never the one replaced. A member floats in one session at a time: opening it from another session removes it from the first set (spec 8.2).
10. **Scratch shell lifetime.** A scratch shell is a normal tab opened with `tabs.StartOpts{Dir: <session's last OSC 7 folder, else its start folder>, Parent: <session ID>}`. It gets its own `KAZE_TAB_ID` and the notify env like any tab. `Parent != 0` means: never made active by `NewOpts`, left out of `buildSide` (so out of the sidebar, `sessionOrder`, next/prev, `afterClose`), never shown as the main view. The toggle hides or shows the pane; hiding keeps the shell running and its screen. If its shell has exited, the next toggle closes that tab and starts a new one. Closing the session closes its scratch shell first; a per-frame prune closes any scratch tab whose parent is gone. **Hook messages:** `onNotify` rewrites `m.Tab` from a scratch tab to its parent before anything else (chime, flash, inbox), so done, needs and prompt count on the parent row and "the latest message wins" falls out of normal ordering. Usage reports are not rewritten (the context number belongs to the shell that sent it). `Tab.Parent` is set once at creation, so reading it off the UI goroutine through `Manager.Lookup` is safe.
11. **Visible panes repaint on output.** `Manager.SetVisible(id, on)` is a new per-tab flag next to `watched`: a visible background tab asks for a frame on every output. It is separate from `Watch` because workspace runs turn `Watch` off when a member's first prompt arrives.

## Global Constraints

- Windows first; macOS must build and behave the same where it applies. `GOOS=windows go vet ./...` must pass; darwin vet of the Gio packages cannot run on Windows (the owner checks on the Mac). The macOS scratch key is Ctrl+` (spec 11); check its key name on the Mac.
- Go is at `%LOCALAPPDATA%\Programs\go\bin`. Build test binaries with `GOTMPDIR` under `C:\Users\john\Desktop\kaze-wt\` (Smart App Control blocks fresh binaries elsewhere). If Smart App Control still blocks a test binary, rerun with a changed build id: `go test -ldflags "-X main.r=<random>" ./internal/<pkg>/` or `-ldflags "-B 0x<random hex>"`; retry with a new value if blocked again. Never turn Smart App Control off.
- Idle CPU stays at zero: no tickers, no per-frame file I/O. Hovering pane edges and headers never asks for a frame (cursors are static ops). A drag asks for one frame per pointer event, none when it ends. Hidden panes and panes of other sessions never ask for frames on output.
- Every color from theme tokens (`c.th.*`, `c.memberColor`, `win.pal.BG` for the pane's terminal background); all five themes, including the light Washi, must read well. No raw hex.
- Tests never open visible windows, never send input to the owner's windows (no SendInput, no posting to other windows; drive `win.frame` headless through `input.Router` as the sidebar tests do), never read or write `~/.claude/settings.json`, `%APPDATA%\kaze\config.toml` or `workspaces.toml`, never start real Docker. RED tests must fail safely and be bounded: fixed sizes, deadlines on any wait (`waitSent` style, 3 s), no unbounded loops or allocations.
- Known failures before this plan (they also fail on `main`; do not fix them here): `internal/ptyx` `TestResolveFindsTheProgramOnPath`; `internal/ui` `TestPickFillsFolderTildeAndEmptyTitle`, `TestListMembersShowCommandsWithoutCutting`; `internal/workspaces` `TestExpandFolder`, `TestJoinedPOSIXFieldsKeepCurrentShellContext`.
- Commits: conventional, capitalized verb (`feat(ui): Float workspace members over the session`). No AI attribution lines of any kind (no `Co-Authored-By`, no "Generated with").
- No em dashes in user-visible text, docs or comments. Comments say why, in 1 to 2 lines.

## Merge notes with 6b

6b (`feat/agent-awareness`) edits some of the same functions. Whoever merges second resolves them like this:

- **`focusTarget`** (`window.go`): 6b adds the rename field first. Order after both: rename field, filter field, Settings, the pane with the keyboard (this plan), the session's `termTag`.
- **`sidePress`** (`sidebar_window.go`): 6b adds row double-click rename and changes the filter line. This plan changes only the member branch (`lineAt` hit that is a member calls `floatMember` instead of `selectTab`). Keep both.
- **`dropSideEdit`** (6b's replacement for `dropFilter` at `selectTab`, `newTab` and `onPointer`): this plan calls `win.dropFilter()` in the pane press path; after 6b merges, that call becomes `win.dropSideEdit()`, like 6b's other call sites.
- **`deliver`** (`alerts_ui.go`): 6b adds `applyAgent(m, t)` and `wantCount(t)`. This plan does not touch `deliver`; it rewrites `m.Tab` in `onNotify`, so 6b's calls already receive the parent tab for a scratch shell's messages. Check `onNotify`'s new `KindPrompt` branch sits **after** the rewrite.
- **`logic.go` / `config.KnownActions` / `defaultBindings`**: both plans add an action (`next_waiting`, `scratch_shell`). Keep both.
- **`internal/version`**: set one minor above `main` at the time you finish (0.8 if 6b has not merged, 0.9 if it has).

## Review Focus

1. Closing a session whose scratch shell is shown (or hidden, or exited): the scratch tab closes too, nothing is left running, the window does not land on the scratch tab, and the last session closing still quits (Task 8).
2. Shrinking the window below a pane's size, or hiding/showing the sidebar: panes stay fully inside the terminal area, their PTYs shrink, and they come back to their size when the window grows (Task 5).
3. A member pane's tab exits or its workspace run is cancelled while floating: the pane disappears on the next frame, no renderer or visible flag is left behind, and the keyboard returns to the session (Task 7).
4. Typing while a pane has the keyboard, including Esc, Ctrl+C and pasting: every byte goes to the pane's shell and none to the session behind it; Esc never closes the pane (Task 6).
5. Several panes stacked over each other: a click on an overlapped area hits only the front pane, a click on the session area outside all panes reaches the session, and dragging a pane under another one never hands the gesture to the other pane (Task 6).

---

### Task 1: Tabs that float: parent, start folder, own size, visibility

**Files:**
- Modify: `internal/tabs/manager.go`
- Test: `internal/tabs/manager_test.go`, `internal/tabs/start_opts_test.go`

**Interfaces:**
- Produces:

```go
// Tab gains (both set once at creation, safe to read from any goroutine):
Parent TabID  // the session a scratch shell belongs to; 0 for every other tab
Dir    string // the folder it was started in, "" for the window's default
// StartOpts gains:
Parent TabID  // non-zero: a scratch shell; NewOpts does not make it active
func (m *Manager) SetVisible(id TabID, on bool)          // a visible background tab asks for frames on every output
func (m *Manager) ResizeTab(id TabID, cols, rows int)    // own size; ResizeAll skips it; unchanged size does nothing
func (m *Manager) Dock(id TabID)                         // back to the shared size; no-op unless ResizeTab sized it
```

`entry` gains `visible atomic.Bool` and `own bool`, `ownCols, ownRows int` (guarded by `m.mu`).

- [ ] **Step 1: Write the failing tests**

```go
func TestParentTabIsNotMadeActive(t *testing.T)      // a := New(); s := NewOpts({Parent: a.ID}) -> Active().ID == a.ID; s.Parent == a.ID
func TestTabKeepsItsStartFolder(t *testing.T)        // NewOpts({Dir: "C:\\x"}) -> Dir == "C:\\x"; New() -> Dir == ""
func TestVisibleBackgroundTabRequestsFrames(t *testing.T) // like TestWatchedBackgroundTabRequestsFrames: SetVisible(id, true), output -> onChange called (bounded wait); SetVisible false, more output -> no new call within 200ms
func TestWatchOffKeepsVisible(t *testing.T)          // SetVisible true, Watch(id,false) -> output still requests a frame
func TestResizeTabIsSkippedByResizeAll(t *testing.T) // ResizeTab(b, 40, 10); ResizeAll(100, 30) -> b's PTY last size [40 10], a's [100 30]
func TestDockReturnsToTheSharedSize(t *testing.T)    // after the above, Dock(b) -> b's PTY [100 30]; Dock of an unsized tab sends no resize
func TestResizeTabUnknownIsIgnored(t *testing.T)     // ResizeTab(999, 1, 1), Dock(999) do not panic
```

- [ ] **Step 2: Run** `go test ./internal/tabs/ -run 'Parent|StartFolder|Visible|WatchOff|ResizeTab|Dock'` → FAIL (undefined fields and methods).
- [ ] **Step 3: Implement** in `manager.go`. `newTab` skips `m.active.Store(id)` when `o.Parent != 0`. `notify` treats `visible` like `watched`. `ResizeTab` and `ResizeAll` call `Session.Resize` outside the lock, as `ResizeAll` does today.
- [ ] **Step 4: Run** `go test ./internal/tabs/` → PASS.
- [ ] **Step 5: Commit**

```bash
git add internal/tabs
git commit -m "feat(tabs): Let a tab float with its own size and a parent session"
```

### Task 2: The scratch shell shortcut

**Files:**
- Modify: `internal/config/config.go` (`KnownActions`), `internal/ui/logic.go` (`action`, `actionByName`, `defaultBindings`)
- Test: `internal/config/config_test.go`, `internal/ui/logic_test.go`

**Interfaces:**
- Produces: `actScratch` named `scratch_shell`; defaults Windows `{Name: "`", Mods: ModCtrl|ModShift}`, macOS `{Name: "`", Mods: ModCtrl}` (spec 11). `runShortcut` gets a `case actScratch: win.toggleScratch()` in Task 8; until then the case is absent and the key does nothing.

- [ ] **Step 1: Write the failing tests**
  - `TestScratchShellDefaults`: `defaultBindings("windows")["scratch_shell"] == {Name: "`", Mods: key.ModCtrl | key.ModShift}`; `defaultBindings("darwin")["scratch_shell"] == {Name: "`", Mods: key.ModCtrl}`; `config.KnownActions["scratch_shell"]`.
  - `TestScratchShellOverride`: `buildKeymap("windows", map[string]string{"scratch_shell": "ctrl+alt+s"})` maps `{Name: "S", Mods: Ctrl|Alt}` to `actScratch` with no warning; `parseShortcut("ctrl+shift+`")` gives `{Name: "`", Mods: Ctrl|Shift}`.
  - `TestScratchKeyIsAShortcutNotText`: `routeKey("windows", km, "`", Ctrl|Shift, term.Modes{})` returns `act == actScratch` and no data.
- [ ] **Step 2: Run** `go test ./internal/config/ ./internal/ui/ -run 'Scratch'` → FAIL.
- [ ] **Step 3: Implement** the action, its name, both defaults and the known name.
- [ ] **Step 4: Run** `go test ./internal/config/ ./internal/ui/ -run 'Scratch|Keymap|Shortcut'` → PASS.
- [ ] **Step 5: Commit**

```bash
git add internal/config internal/ui/logic.go internal/ui/logic_test.go
git commit -m "feat(config): Add the scratch shell shortcut"
```

### Task 3: The pane model

**Files:**
- Create: `internal/ui/panes.go`
- Test: `internal/ui/panes_test.go`

**Interfaces:**
- Produces (pure: no Gio frame, no manager, no window):

```go
const (
	maxPanes     = 4
	paneHeaderDp = 30
	paneEdgeDp   = 6
	paneCornerDp = 14
	paneRadiusDp = 8
)
var (
	paneMinDp     = image.Pt(280, 140)
	memberSizeDp  = image.Pt(560, 260)
	scratchRectDp = image.Rect(150, 80, 670, 320)
)
type paneKind int
const (
	paneMember paneKind = iota
	paneScratch
)
type pane struct {
	id     tabs.TabID
	kind   paneKind
	r      image.Rectangle // dp, relative to the terminal area's top-left
	z      uint64
	opened uint64
	hidden bool            // the scratch shell only: toggled off, still running
	body   int             // event tag: keys, paste and the terminal's pointer input
	frame  int             // event tag: header and edges
	hits   paneHits        // last frame, window px (Task 5)
	grab   paneGrab        // the drag or resize in progress (Task 6)
}
type paneSet struct{ panes []*pane } // open order
type floats struct {
	sets  map[tabs.TabID]*paneSet // by owning session
	seq   uint64                  // z and open-order stamps
	keys  tabs.TabID              // the pane with the keyboard; 0 = the session
	rends map[tabs.TabID]*render.Renderer // Task 5
	shown []tabs.TabID            // tabs marked visible last frame, for SetVisible diffs (Task 5)
}
func (f *floats) set(session tabs.TabID) *paneSet                     // creates on first use
func (f *floats) open(session, id tabs.TabID, kind paneKind) (p *pane, dropped tabs.TabID)
func (f *floats) raise(session, id tabs.TabID)
func (f *floats) remove(id tabs.TabID) (session tabs.TabID, ok bool)  // from whichever set holds it
func (f *floats) owner(id tabs.TabID) tabs.TabID                      // the session whose set holds id, 0 if none
func (f *floats) prune(exists func(tabs.TabID) bool) (orphans []tabs.TabID)
func (s *paneSet) find(id tabs.TabID) *pane
func (s *paneSet) scratch() *pane
func (s *paneSet) visible() []*pane // not hidden, ascending z
func (s *paneSet) front() *pane     // highest z among visible, nil if none

type edge uint8 // bit set; 0 means move
const (
	edgeN edge = 1 << iota
	edgeS
	edgeW
	edgeE
)
func fitPane(r, area image.Rectangle, min image.Point) image.Rectangle
func edgeAt(r image.Rectangle, p image.Point, band, corner int) (e edge, ok bool)
func dragRect(start image.Rectangle, e edge, delta image.Point, min image.Point) image.Rectangle
```

Rules: `open` of a pane already in the set unhides and raises it; of a member held by another session's set moves it (removes it there first); a new member pane gets `memberSizeDp` at (80 + 30n, 40 + 30n), n = visible panes in the set; a new scratch pane gets `scratchRectDp`. When the set then holds more than `maxPanes`, the member pane with the lowest `opened` is removed and returned as `dropped`. `prune` drops panes whose tab is gone and sets whose session is gone, returning the scratch tab IDs of dropped sets (the caller closes them), and clears `keys` when that pane is gone. `fitPane` shrinks `r` to at most `area`'s size (not below `min` unless the area is smaller), then moves it inside `area`. `edgeAt` reports a corner within `corner` px of a corner, else an edge within `band` px of a side, `ok == false` outside `r`. `dragRect` with `e == 0` moves; otherwise moves the named sides, never below `min` (a side that would pass the minimum stops there, the opposite side stays put).

- [ ] **Step 1: Write the failing tests**

```go
func TestOpenCascadesMemberPanes(t *testing.T)       // 3 members in session 1 -> r.Min (80,40), (110,70), (140,100); size 560x260; z increasing
func TestOpenAgainRaisesAndUnhides(t *testing.T)     // open a, open b, open a -> a.z > b.z, len 2
func TestFifthPaneReplacesTheOldestMember(t *testing.T) // scratch then members m1..m4 -> dropped == m1; scratch still present; len 4
func TestScratchIsNeverReplaced(t *testing.T)        // hidden scratch + 4 members -> 4 panes, scratch kept, dropped = oldest member
func TestMemberFloatsInOneSessionAtATime(t *testing.T) // open m in s1, then in s2 -> owner(m) == s2, s1's set has no m
func TestVisibleSkipsHiddenInZOrder(t *testing.T)    // raise order b, a; hidden scratch -> visible() == [b, a]; front() == a
func TestPruneDropsGoneTabsAndReturnsOrphanScratch(t *testing.T) // session 1 gone -> its set dropped, its scratch ID returned; keys cleared when its pane is gone
func TestFitPaneShrinksAndMoves(t *testing.T)        // r 600x300 at (500,400) in area 800x500 -> inside area, size 600x300 moved to (200,200); area 400x200 -> 400x200 at area.Min
func TestFitPaneKeepsAFittingPane(t *testing.T)      // r inside area -> unchanged
func TestEdgeAtZones(t *testing.T)                   // r (100,100)-(500,300), band 6, corner 14: (102,102) -> N|W; (300,102) -> N; (498,200) -> E; (300,200) -> 0, ok; (50,50) -> !ok
func TestDragRectMovesAndResizes(t *testing.T)       // e 0, delta (10,5) -> moved; e S|E delta (50,20) -> bigger; e W delta (+1000,0) -> width == min.X, right side unchanged
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Open|Fifth|ScratchIsNever|MemberFloats|VisibleSkips|Prune|FitPane|EdgeAt|DragRect'` → FAIL (undefined).
- [ ] **Step 3: Implement** `panes.go`. Every opening, showing or raise takes `f.seq++` for `z`; a new pane also takes one for `opened`.
- [ ] **Step 4: Run** the same command → PASS.
- [ ] **Step 5: Commit**

```bash
git add internal/ui/panes.go internal/ui/panes_test.go
git commit -m "feat(ui): Model floating panes per session"
```

### Task 4: Terminal input works on any view

A refactor with no behavior change, so panes reuse the session's input code.

**Files:**
- Modify: `internal/ui/window.go` (`termEvents`, `route`, `onPointer`, `autoScroll`, `runShortcut`'s copy and paste, `frame`'s call), `internal/ui/bench.go` if `benchBatch` needs the view
- Test: `internal/ui/pointer_test.go`

**Interfaces:**
- Produces:

```go
// termView is one terminal on screen: the shown session or a floating pane.
type termView struct {
	tab        *tabs.Tab
	tag        event.Tag       // keys, paste and pointer input
	grid       image.Rectangle // window px
	cols, rows int
}
func (win *window) mainView(t *tabs.Tab) termView // termTag, the main grid, win.mgr.Size()
func (win *window) termEvents(gtx layout.Context, v termView, cw, ch int)
func (win *window) route(gtx layout.Context, v termView, batch []event.Event, cw, ch int)
func (win *window) onPointer(gtx layout.Context, v termView, e pointer.Event, m term.Modes, cw, ch int)
func (win *window) autoScroll(gtx layout.Context, v termView, cw, ch int) bool
func (win *window) keyView() (termView, bool) // the view with the keyboard; until Task 6 always the main view
```

`win.gridMin` is replaced by `v.grid.Min`, `win.mgr.Size()` in `onPointer`/`autoScroll` by `v.cols, v.rows`, `win.termTag` in `route`'s `SnippetCmd`/`SelectionCmd` by `v.tag`. `route` keeps its main-view rule (after a key that switches tabs, the rest of the batch goes to the new active tab) only when `v.tag == win.termTag`; for any other view it stops at a key that changed the active tab or closed the view's tab. `actPaste` and `actCopy` use `keyView()`.

- [ ] **Step 1: Write the failing test** `TestPointerUsesTheViewsGrid` in `pointer_test.go`: a tab with SGR mouse reporting on (as `TestMouseReporting...` sets it up), `win.onPointer(gtx, termView{tab, tag, image.Rect(300, 200, 300+10*cw, 200+5*ch), 10, 5}, press at (300+3*cw+1, 200+2*ch+1))` → the shell is sent the SGR press for column 4, row 3 (1-based); a press beyond the view's last column reports column 10, not the main grid's.
- [ ] **Step 2: Run** `go test ./internal/ui/ -run PointerUsesTheViewsGrid` → FAIL (signature).
- [ ] **Step 3: Implement** the refactor; `frame` builds `win.mainView(active)` once and passes it to `termEvents`, `autoScroll` and `drawTerminal`'s grid.
- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline failures aside). The existing pointer, selection, paste and bench tests are the proof of no behavior change.
- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "refactor(ui): Route terminal input through a view"
```

### Task 5: Draw panes with their own renderers and sizes

**Files:**
- Create: `internal/ui/panes_draw.go`
- Modify: `internal/ui/window.go` (`frame`, the `window` struct gains `fl floats`), `internal/ui/settings.go` or wherever `rendStale` is set (drop pane renderers with the main one)
- Test: `internal/ui/panes_draw_test.go`

**Interfaces:**
- Consumes: Task 1 (`ResizeTab`, `Dock`, `SetVisible`), Task 3 (`floats`, `fitPane`), Task 4 (`termView`).
- Produces:

```go
type paneHits struct {
	outer, header, body image.Rectangle // window px
	expand, close       image.Rectangle // empty when the pane has no such button
}
type paneLook struct {
	title, status string
	statusDown    bool       // status drawn in UsageHigh (red)
	color         string     // member color name, "" for the scratch shell
	mark          mark       // alert mark of the pane's tab
	front         bool
	expand        bool       // members only
	folder        string     // scratch only: the session's folder, ~-shortened
	key           string     // scratch only: the scratch_shell key text, e.g. "Ctrl Shift `"
}
func (c *chrome) layoutPane(gtx layout.Context, r image.Rectangle, lk paneLook) paneHits // chrome only; the terminal is drawn by the caller in hits.body
func (win *window) paneOwner() tabs.TabID           // the shown tab if it is a session (not a member, not a scratch shell, not Settings), else 0
func (win *window) paneViews(gtx layout.Context, termRect image.Rectangle) []termView // fitted, in z order; also ResizeTab per pane
func (win *window) drawPanes(gtx layout.Context, views []termView)
func (win *window) syncVisible(views []termView)    // SetVisible diffs against fl.shown; drops renderers of tabs no longer shown
func (win *window) paneRend(id tabs.TabID) *render.Renderer // creates with the main renderer's px, palette and options
```

Look (spec 8.1, mockup `.fp`/`.fph`): rounded rectangle (`paneRadiusDp`), body filled with `win.pal.BG`, 1 px border `th.Border` (front: `th.Accent`), header `th.Chrome` with a bottom line `th.Border`; header left to right: member color bar (`c.memberColor`) and alert dot (`c.alertDot`), title in `th.ActiveFG`, status in `th.Muted` (or `th.UsageHigh` when down); right side: `Expand` (members) and `×`, hover-free (no hover state, so no frames). Scratch header: `scratch shell`, the session's folder, the key text, `×`. Text cut with `c.fit`/`cutRunes` so nothing leaves the header. Draw order inside `frame`: main terminal, workspace banner, panes (ascending z), status bar, toasts.

Frame wiring: after `active` is known, `win.mgr.Dock(active.ID)` and, if `active` is in any set, `win.fl.remove(active.ID)` (the invariant in Decision 1); `win.fl.prune(win.mgr.Has)` and close the returned orphan scratch tabs with `win.mgr.Close`; `paneViews` only when `paneOwner() != 0`; `syncVisible` every frame (with no owner, every pane is hidden). Pane renderers get `SetFocused(win.focused && win.fl.keys == id)` and are dropped when `rendStale` or the px size changes.

- [ ] **Step 1: Write the failing tests** with `sideWindow`, `openIn`, `openMember`, `sizedFrame`, adding panes directly with `win.fl.open` (interaction comes in Task 6):

```go
func TestEachVisiblePaneHasItsOwnRenderer(t *testing.T)  // 2 member panes in session a -> len(fl.rends) == 2, distinct, neither == win.rend
func TestHiddenPanesDropTheirRenderers(t *testing.T)     // switch to session b -> fl.rends empty; mgr no longer visible for them; back to a -> 2 again at the same rects
func TestPanePTYGetsThePaneGrid(t *testing.T)            // pane 560x260 dp at scale 1 -> its recPTY lastSize == gridSize(560-2-16, 260-2-30-8, cw, ch); the session's PTY keeps the main size
func TestWindowResizeLeavesFittingPanes(t *testing.T)    // 1000x600 then 1200x700 -> session PTY changes, pane PTY unchanged
func TestShrinkingTheWindowFitsPanes(t *testing.T)       // pane at (500,300) 560x260; window 700x400 -> hits.outer inside the terminal area, PTY smaller; back to 1000x600 -> stored rect and PTY size restored
func TestHidingTheSidebarMovesPanesWithTheTerminal(t *testing.T) // hits.outer.Min.X shifts by the sidebar width
func TestExpandedMemberLeavesEverySet(t *testing.T)      // member m floating in a; SetActive(m) -> after one frame owner(m) == 0, m docked (main size)
func TestNoPanesOverSettings(t *testing.T)               // openSettings -> no pane hits, fl.rends empty
func TestFrontPaneDrawsTheAccentBorder(t *testing.T)     // via layoutPane: front -> a pixel on the border equals th.Accent; not front -> th.Border (read back with the headless frame's image, as theme tests do, or assert the look passed)
func TestPanesDrawInEveryTheme(t *testing.T)             // each of the five themes: frame with a member and a scratch pane renders without panic; header fill != body fill
func TestIdlePanesAskForNoFrame(t *testing.T)            // 2 visible panes, no output, two frames -> no InvalidateCmd executed by pane code (count via a test hook on gtx, or win.fl redraw counter stays 0)
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Renderer|HiddenPanes|PanePTY|WindowResizeLeaves|Shrinking|HidingTheSidebarMoves|ExpandedMember|NoPanesOver|FrontPane|PanesDraw|IdlePanes'` → FAIL.
- [ ] **Step 3: Implement** `panes_draw.go` and the frame wiring.
- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline aside).
- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): Draw floating panes with their own renderer and size"
```

### Task 6: Move, resize, raise and type into panes

**Files:**
- Create: `internal/ui/panes_input.go`
- Modify: `internal/ui/window.go` (`frame` event phase, `focusTarget`, `onPointer` press path, `setFocused`), `internal/ui/alerts_ui.go` (`selectTab` clears `fl.keys`)
- Test: `internal/ui/panes_input_test.go`

**Interfaces:**
- Consumes: Tasks 3 to 5.
- Produces:

```go
type paneGrab struct {
	on    bool
	e     edge            // 0: moving
	from  image.Point     // press position, window px
	start image.Rectangle // fitted rect at the press, dp
}
func (win *window) paneEvents(gtx layout.Context, cw, ch int) // frame tags then body tags of every visible pane of paneOwner()
func (win *window) paneFrame(gtx layout.Context, p *pane, e pointer.Event)
func (win *window) focusPane(id tabs.TabID)  // raise, keys = id, wantFocus, sendFocus moves
func (win *window) focusSession()            // keys = 0, wantFocus, sendFocus moves
func (win *window) closePane(id tabs.TabID)  // ×: members are removed and docked, the scratch shell is hidden
```

Behavior (spec 8.1, Decisions 5 to 7):
- The `frame` tag covers the header and the four edge bands; its press raises and focuses the pane; then: on `close` → `closePane`; on `expand` → Task 7's `expandMember`; on an edge or corner (`edgeAt`) → resize grab; elsewhere in the header → move grab. `pointer.Drag` applies `dragRect` (px delta converted to dp with `gtx.Metric.PxPerDp`) and asks for one frame; `Release`/`Cancel` end the grab.
- The `body` tag is the pane's terminal: its press raises and focuses the pane (then `dropFilter()`, see merge notes), and its events go through `win.route(gtx, view, batch, cw, ch)` with the pane's `termView`; pointer filters and Gio key filters as `termEvents` builds them for `termTag`.
- A press on the session's terminal (`onPointer` with the main view) calls `focusSession()` when a pane had the keyboard.
- `focusTarget` returns `&p.body` for `fl.keys` when that pane is visible in the shown session's set and Settings is not shown; otherwise as today.
- `keyView()` (Task 4) returns the pane's view when `fl.keys` is set.
- `visit` also clears the alert of the pane with the keyboard.
- Input is ignored while `win.bench.IgnoreInput()`.

- [ ] **Step 1: Write the failing tests** (headless frames; press/drag/release queued on `input.Router` at window points from `p.hits`):

```go
func TestClickingAPaneGivesItTheKeyboard(t *testing.T)   // click body -> fl.keys == p.id, focusTarget() == &p.body; typing "a" -> pane PTY got "a", session PTY got nothing
func TestClickingTheSessionTakesTheKeyboardBack(t *testing.T) // then click the terminal outside the pane -> fl.keys == 0; "b" -> session PTY
func TestEscGoesToThePaneProgram(t *testing.T)           // pane focused, Esc -> pane PTY got "\x1b"; pane still visible, fl.keys unchanged
func TestPasteGoesToThePane(t *testing.T)                // ctrl+shift+v with pane focused -> clipboard.ReadCmd targets &p.body; DataEvent "x" -> pane PTY
func TestShortcutsWorkFromAPane(t *testing.T)            // ctrl+shift+t with pane focused -> a new session is shown, keys == 0
func TestCloseTabClosesTheSessionNotThePane(t *testing.T) // ctrl+shift+w with a member pane focused -> the session closes, the member keeps running
func TestDraggingTheHeaderMovesThePane(t *testing.T)     // press at header centre, drag (+50,+20), release -> p.r.Min moved by (50,20) dp at scale 1; one frame per drag event
func TestDraggingTheCornerResizesThePTY(t *testing.T)    // bottom-right corner drag (+100,+60) -> p.r bigger, pane PTY lastSize grows, session PTY unchanged
func TestResizeStopsAtTheMinimum(t *testing.T)           // left edge dragged +1000 -> width == 280 dp
func TestClickRaisesTheBackPane(t *testing.T)            // two overlapping panes; click the visible part of the back one -> it becomes front (accent), the other keeps its rect
func TestOverlapHitsOnlyTheFrontPane(t *testing.T)       // click in the overlap -> only the front pane's PTY sees the SGR press (both panes with mouse reporting on)
func TestDragUnderAnotherPaneKeepsTheGesture(t *testing.T) // drag pane A's header across pane B -> A moves, B unchanged, B gets no input
func TestHoveringEdgesAsksForNoFrame(t *testing.T)       // 10 Move events across edges and header -> no invalidate counted
func TestFocusReportsFollowTheKeyboard(t *testing.T)     // focus reporting (CSI ? 1004 h) on both; click pane -> session gets "\x1b[O", pane gets "\x1b[I"
func TestSwitchingSessionsReturnsTheKeyboard(t *testing.T) // pane focused, next_tab -> keys == 0, focusTarget() == termTag
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'ClickingAPane|ClickingTheSession|EscGoes|PasteGoes|ShortcutsWorkFrom|CloseTabCloses|Dragging|ResizeStops|ClickRaises|Overlap|DragUnder|HoveringEdges|FocusReports|SwitchingSessions'` → FAIL.
- [ ] **Step 3: Implement** `panes_input.go`, the `focusTarget` case and the press paths.
- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline aside).
- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): Move, resize and type into floating panes"
```

### Task 7: Workspace members float

**Files:**
- Modify: `internal/ui/sidebar_window.go` (`sidePress` member branch), `internal/ui/status.go` (`showServer`), `internal/ui/sidebar.go` (`sideState` gains `floating`), `internal/ui/sidebar_window.go` (`drawSide` passes it), `internal/ui/panes_input.go`
- Test: `internal/ui/panes_member_test.go`, `internal/ui/sidebar_window_test.go` (update `TestPlusOpensASessionAndMemberClickShowsIt`)

**Interfaces:**
- Consumes: Tasks 3 to 6.
- Produces:

```go
func (win *window) floatMember(id tabs.TabID) // the one entry point; 6e's palette calls it
func (win *window) expandMember(id tabs.TabID)
// sideState gains:
floating tabs.TabID // the front member pane of the shown session, highlighted like shown
```

Behavior (spec 7, 8.2):
- `floatMember(id)`: when `id` is the shown tab, nothing. When `paneOwner() == 0` (a member or Settings is the main view), `selectTab(id)` as in 6a. Otherwise `win.fl.open(owner, id, paneMember)`, dock and close nothing else; if `dropped != 0`, `win.mgr.Dock(dropped)`; then `focusPane(id)`.
- A sidebar member click and the status bar's server dots call `floatMember` (dots: first down member, else the first, as `showServer` picks today).
- `expandMember(id)`: `win.fl.remove(id)`, `win.mgr.Dock(id)`, `selectTab(id)`, `win.side.reveal = true`. Picking any session goes back (no extra code).
- × (`closePane`) on a member removes and docks it; it never closes the tab or stops the server.
- A member pane's header shows `memberStatus` text (Running, Starting, `stopped` in red...) and the tab's alert mark, the same values as its line in the workspaces section.

- [ ] **Step 1: Write the failing tests**

```go
func TestMemberClickFloatsOverTheSession(t *testing.T)    // session a shown; click member line -> Active().ID == a.ID, owner(m) == a.ID, pane visible and focused
func TestMemberClickWhenItIsTheMainViewDoesNothing(t *testing.T)
func TestMemberClickWhileAMemberIsShownShowsIt(t *testing.T) // main view m1 (expanded); click m2 -> Active() == m2, no pane
func TestExpandMakesTheMemberTheMainView(t *testing.T)    // click Expand -> Active() == m, owner(m) == 0, m's PTY at the main size, its line highlighted (side.st.shown == m)
func TestPickingASessionAfterExpandGoesBack(t *testing.T) // then click row a -> Active() == a, a's other panes shown again
func TestCloseHidesAMemberWithoutStoppingIt(t *testing.T) // × -> no pane, tab still open, run still has it, no write to its PTY
func TestOpeningAMemberInAnotherSessionMovesIt(t *testing.T) // float m in a, show b, float m -> owner(m) == b; back to a -> no m pane
func TestFifthMemberReplacesTheOldest(t *testing.T)       // 5 member clicks -> 4 panes, first one gone and docked
func TestServerDotsFloatTheDownMember(t *testing.T)       // one stopped member -> dots click floats it
func TestMemberExitRemovesItsPane(t *testing.T)           // close the member tab (run cancelled) -> next frame no pane, no renderer, keys == 0
func TestFloatingMemberIsHighlighted(t *testing.T)        // side.st.floating == m while its pane is front in the shown session
```

Update `TestPlusOpensASessionAndMemberClickShowsIt` so its member click asserts the pane, not a main-view switch.

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'MemberClick|Expand|PickingASessionAfter|CloseHides|OpeningAMember|FifthMember|ServerDotsFloat|MemberExit|FloatingMember|PlusOpens'` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline aside).
- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(workspaces): Float workspace members over the session"
```

### Task 8: The scratch shell

**Files:**
- Create: `internal/ui/scratch.go`
- Modify: `internal/ui/sidebar_model.go` (`buildSide` skips `t.Parent != 0`), `internal/ui/sidebar_window.go` (`closeTab`, `afterClose`'s member loop skips scratch tabs), `internal/ui/logic.go`/`window.go` (`runShortcut` `case actScratch`), `internal/ui/alerts_ui.go` (`onNotify` rewrite), `internal/ui/titlebar.go` and `window.go` (`barLayout.Scratch`, `capScratch`, `captionEvent`), `internal/ui/sidebar.go` and `sidebar_window.go` (`sideHits.scratch` in the macOS top row and the rail), `internal/ui/review_fixes_test.go` (`recStarter` records `dirs []string`)
- Test: `internal/ui/scratch_test.go`, `internal/ui/titlebar_test.go`, `internal/ui/sidebar_model_test.go`

**Interfaces:**
- Consumes: Task 1 (`StartOpts.Parent`, `Tab.Dir`), Tasks 3 to 6.
- Produces:

```go
func (win *window) toggleScratch()
func (win *window) scratchDir(session *tabs.Tab) string // locOf(session).cwd, else session.Dir
func hookTab(m *tabs.Manager, id tabs.TabID) tabs.TabID  // the parent for a scratch shell, else id; safe off the UI goroutine
// barLayout gains Scratch image.Rectangle (left of Gear); captionButton gains capScratch.
// sideHits gains scratch image.Rectangle (macOS top row and rail).
```

Behavior (spec 8.3, Decision 10):
- `toggleScratch`: with `paneOwner() == 0`, nothing. Otherwise, if the set has a scratch pane whose tab is open and not exited: visible → hide it (and `focusSession()` if it had the keyboard); hidden → show, raise, `focusPane`. If there is none, or its tab exited: close the old tab, open `win.mgr.NewOpts(tabs.StartOpts{Dir: win.scratchDir(s), Parent: s.ID})` (an error goes to the status bar notice like `newTab`), `win.fl.open(s.ID, id, paneScratch)`, dock any `dropped` member, `focusPane`.
- The key (`actScratch`), the Windows title bar button (a terminal glyph, `th.CapGlyph`, between the drag area and the gear, spec 5.1) and the macOS sidebar/rail button call it.
- `closeTab(id)`: before closing a session, close its scratch tab and drop its set.
- `onNotify`: first line after the `mgr` checks, for every kind except `KindUsage`: `m.Tab = int(hookTab(mgr, id))` (match `notify.Msg.Tab`'s type), and use the new id for the active/focused check.
- Not listed in the sidebar, not reached by next/prev, never the main view, never renamed.

- [ ] **Step 1: Write the failing tests**

```go
func TestScratchToggleStartsInTheSessionsFolder(t *testing.T) // report(a, C:\code\repo\sub) -> one new tab with Parent == a.ID, recStarter dirs last == that folder; pane visible and focused; Active() still a
func TestScratchFallsBackToTheStartFolder(t *testing.T)       // no OSC 7 report, a opened with Dir "C:\\x" -> dir "C:\\x"
func TestScratchToggleHidesAndKeepsItRunning(t *testing.T)    // toggle twice -> hidden, tab still open, same tab ID on the third toggle, same rect
func TestScratchIsNotASession(t *testing.T)                   // buildSide has no row for it; sessionIDs() excludes it; next_tab never shows it
func TestOneScratchPerSession(t *testing.T)                   // toggle in a and in b -> two scratch tabs with parents a and b; a's toggle shows only a's
func TestExitedScratchIsReplaced(t *testing.T)                // close its recPTY (shell exit), toggle -> old tab closed, a new tab started
func TestClosingTheSessionClosesItsScratch(t *testing.T)      // close a (scratch shown or hidden) -> scratch tab gone; with a the last session, mgr.Empty()
func TestScratchToggleDoesNothingOverAMember(t *testing.T)    // main view is a member -> no tab started
func TestScratchHookCountsTowardItsSession(t *testing.T)      // onNotify(done from the scratch ID) then drainAlerts -> markOf(a) == markDone, markOf(scratch) == 0; a needs from the session after a done from its scratch -> markNeeds (latest wins)
func TestScratchUsageIsNotMoved(t *testing.T)                 // a usage report from the scratch ID stays on the scratch ID
func TestScratchButtonInTheTitleBar(t *testing.T)             // custom bar: Scratch rect non-empty, left of Gear; click -> toggles
func TestScratchButtonOnMac(t *testing.T)                     // custom=false: sideHits.scratch non-empty in the sidebar and in the rail; click -> toggles
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Scratch|ClosingTheSessionCloses'` → FAIL.
- [ ] **Step 3: Implement** `scratch.go` and the listed edits.
- [ ] **Step 4: Run** `go test ./internal/ui/ ./internal/tabs/` → PASS (baseline aside).
- [ ] **Step 5: Run** `GOOS=windows go vet ./...` → no output.
- [ ] **Step 6: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): Add a scratch shell per session"
```

### Task 9: Docs and version

**Files:**
- Modify: `README.md` (floating panes, the scratch shell, `scratch_shell` in the shortcut list and the `[keys]` table; a member click now floats it), `docs/HANDOFF.md` (what 6d shipped, the macOS checks owed: the Ctrl+` key name and the scratch button in the rail), `internal/version/version.go`
- Test: `internal/version/version_test.go`

- [ ] **Step 1: Update `version_test.go`** to expect one minor above `main` (see merge notes); run `go test ./internal/version/` → FAIL.
- [ ] **Step 2: Set `Number`**; run → PASS.
- [ ] **Step 3: Update the README and HANDOFF.** Say that Esc goes to the program in a pane, × hides a member pane without stopping it, panes are not kept across restarts, and at most 4 panes per session.
- [ ] **Step 4: Run** `go test ./...` → PASS except the known baseline failures listed in Global Constraints.
- [ ] **Step 5: Commit**

```bash
git add README.md docs/HANDOFF.md internal/version
git commit -m "docs: Describe floating panes and the scratch shell"
```

---

## Self-review

1. **Spec coverage.** 8.1: drag/resize (Task 6), stays inside and fits a smaller window (Task 5), click raises and takes the keyboard, accent border (Tasks 5, 6), Esc to the program (Task 6), panes per session with memory (Tasks 3, 5), 4 per session replacing the oldest member (Tasks 3, 7), pane resize resizes the PTY (Tasks 1, 5, 6). 8.2: member floats, one session at a time, Expand, × hides only (Task 7). 8.3: scratch tab hidden from the list and next/prev, hook messages to the parent, chimes as any session, one per session, current folder, toggle keeps it running, exited shell restarts, closing the session closes it (Task 8). 7: member click and server dots open floating, clicking the main-view member does nothing (Task 7). 11: `scratch_shell` keys (Task 2). 5.1: scratch button in the title bar and the macOS rail (Task 8). 14: Esc decision honored. 15: hit areas and sizes decided above.
2. **Step scan.** Every code step names its file and signatures; every test step names its tests and assertions with the spec's numbers (4 panes, 280 x 140 minimum, Ctrl+Shift+` / Ctrl+`).
3. **Type consistency.** `floats`, `paneSet`, `pane`, `paneKind`, `edge`, `paneHits`, `paneGrab`, `termView` are defined in Tasks 3 to 6 and used with the same names after; `floatMember`, `expandMember`, `closePane`, `focusPane`, `focusSession`, `toggleScratch`, `hookTab` each have one definition. `Manager.ResizeTab`/`Dock`/`SetVisible` and `StartOpts.Parent`/`Tab.Parent`/`Tab.Dir` come from Task 1.
4. **Review Focus.** Each line has its test: 1 `TestClosingTheSessionClosesItsScratch`; 2 `TestShrinkingTheWindowFitsPanes`, `TestHidingTheSidebarMovesPanesWithTheTerminal`; 3 `TestMemberExitRemovesItsPane`; 4 `TestEscGoesToThePaneProgram`, `TestPasteGoesToThePane`, `TestClickingAPaneGivesItTheKeyboard`; 5 `TestOverlapHitsOnlyTheFrontPane`, `TestDragUnderAnotherPaneKeepsTheGesture`.
5. **Proportion.** Signatures, rules and test names only; the one refactor (Task 4) is described by what changes, not rewritten.
