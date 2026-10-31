# Window Chrome Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** one Kaze-drawn title bar shared by macOS and Windows, and the direction-A status bar, as in `docs/design/window-chrome-mockup.html`.

**Architecture:**
- The title bar's geometry comes from a pure layout function (`layoutTitle`), replacing `layoutBar`, so it can be tested without Gio.
- Drawing and input reuse today's caption machinery (`captionState`, `capTag`, `ActionInputOp(ActionMove)`), extended with new targets.
- macOS turns frameless, and a small cgo file brings back and positions the native traffic lights.
- The status bar is rebuilt from a pure model (`statusModel`), with its own layout that drops groups in a fixed order, so narrow widths and levels are testable.

**Tech Stack:** Go 1.27, Gio v0.10.3, cgo (Objective-C, darwin only).

**Spec:** `docs/superpowers/specs/2026-10-31-kaze-window-chrome-design.md`

## Global Constraints

- Title bar 42 dp, status bar 32 dp, on both platforms.
- Theme tokens only, no gradients; readable in all five themes, Washi included.
- Idle CPU zero: hover and the usage card change only on pointer events; no new timers.
- Every existing action keeps its key and works from its new place.
- macOS traffic lights are the native buttons (fallback only with the owner's yes, spec 7).
- Never inject global keyboard or mouse input. GUI checks go through code paths, and native captures through a throwaway input-blocked overlay (as in `docs/site-assets/SCREENSHOTS.md`).
- No test reads or writes the owner's real config or `~/.claude`.
- Commits: `type(scope): Verb ...`, no AI attribution.

## Review Focus

1. **macOS full screen and back:** the traffic lights return to the centred position, and the lead strip doesn't overlap them. Check in Task 1 and Task 3.
2. **Very narrow window (600 dp) with an update out, 4 agents waiting, servers and high usage:** nothing overlaps or wraps. The drop order is spec 5.3 and 6.4. Tests in Tasks 2 and 5.
3. **Sidebar hidden:** the lead strip shrinks, **+** goes, and the crumb starts right after the toggle. Tested in Task 2.
4. **Settings or the board open:** the crumb reads "Settings" or "Agents", and the panes and terminal get no clicks through the bar. Tested in Task 3.
5. **Washi:** pills, meters and hover fills keep their contrast. Checked in Task 6's captures.

---

### Task 1: Gate: native traffic lights on a frameless macOS window

Throwaway first, then kept if it works.

**Files:**
- Create: `internal/ui/macwin_darwin.go` (cgo), `internal/ui/macwin_other.go` (no-op stubs)
- Modify: `cmd/kaze/main.go` (behind a temporary `KAZE_FRAMELESS=1` env check for the spike only)

**Interfaces:**
- Produces:
  - `func macShowLights(view uintptr, barHeightPt, leftPt float64)`: shows the close, miniaturize and zoom buttons, centres them vertically in the bar's height and sets the first button's x.
  - `func macTitleDoubleClick(view uintptr)`: reads `AppleActionOnDoubleClick` and calls `zoom:` or `miniaturize:`.
  - `func macLightsInset(view uintptr) float64`: the x where the lights end, 0 in full screen.
  - Stubs on other platforms do nothing and return 0.

- [ ] **Step 1:** Write the cgo. Get the window with `[(NSView*)view window]` and the buttons with `standardWindowButton:`. Unhide them, then set their frame origins in the title bar container's coordinates.
  - Re-apply on `NSWindowDidResizeNotification`, `NSWindowDidEnterFullScreenNotification` and `NSWindowDidExitFullScreenNotification`, using an observer registered once per window.
  - Never change the window level, style mask or delegate that Gio set.
- [ ] **Step 2:** In a spike build only, open the window with `app.Decorated(false)` on darwin. On `app.AppKitViewEvent`, call `macShowLights(view, 42, 14)`.
- [ ] **Step 3:** Build into the scratchpad. Capture the window with `screencapture -l <id>` (no input): normal, after the window is resized from code (`app.Size`), and in full screen entered from code. Expected: the lights are centred in the 42 dp strip at 14 pt every time, and hidden in full screen.
- [ ] **Step 4:** Ask the owner to hover the lights, Option-click zoom, double-click the bar and use full screen once in the spike build. If any of it fails, stop and take the fallback question to the owner (spec 7).
- [ ] **Step 5:** Remove the env switch and keep the cgo files. Commit: `feat(ui): Show native traffic lights on a frameless Mac window`

### Task 2: Title bar layout

**Files:**
- Modify: `internal/ui/titlebar.go` (replace `layoutBar`, `barLayout`, `barDims`)
- Test: `internal/ui/titlebar_test.go`

**Interfaces:**
- Produces:

```go
type titleIn struct {
	Size           image.Point // the bar, px
	Mac            bool
	LightsW        int  // px the traffic lights take from the left (Task 1's inset); 0 off Mac or in full screen
	SideW          int  // the sidebar's width; 0 while hidden
	Crumb          crumbParts
	Dims           titleDims // the fixed sizes, from dp
}
type crumbParts struct{ Repo, Title, Cmd, State string } // State "" | "running" | "needs" | "done"
type titleLayout struct {
	Lead                         image.Rectangle // the strip over the sidebar
	Mark, Toggle, Plus           image.Rectangle // Mark on Windows only; Plus only while the sidebar shows
	Crumb                        image.Rectangle
	CrumbShow                    crumbParts      // what fits (spec 5.3)
	Search                       image.Rectangle
	SearchIconOnly               bool
	Scratch, Agents, Gear        image.Rectangle
	Buttons                      [3]image.Rectangle // Windows only
	Drag                         []image.Rectangle  // empty bar space, crumb included
}
func layoutTitle(in titleIn) titleLayout
```

- `captionButton` gains `capPlus` and `capAgents`. `captionAt` and `captionState.pointer` work on `titleLayout`.

- [ ] **Step 1: Write the failing tests**

```go
func TestTitleSameLayoutBothPlatforms(t *testing.T)   // 1200px: Toggle, Search, Scratch, Agents, Gear have equal X on Mac and Windows when Windows' caption buttons are discounted
func TestTitleLeadMatchesSidebar(t *testing.T)        // SideW 248dp: Lead.Dx()==248dp, Plus at Lead's right end; SideW 0: no Plus, Lead ends after Toggle
func TestTitleMacReservesLights(t *testing.T)         // LightsW 70: Toggle.Min.X >= 70; LightsW 0 (full screen): Toggle starts at the padding
func TestTitleWindowsButtons(t *testing.T)            // three 46dp buttons flush right, Gear left of them; none on Mac
func TestTitleSearchClamp(t *testing.T)               // wide: Search.Dx()==400dp, centred in the free space; narrow: >=190dp; narrower: SearchIconOnly
func TestTitleCrumbDropOrder(t *testing.T)            // shrinking width drops Cmd, then Repo, then shortens Title; buttons never disappear (600dp still has all)
func TestTitleDragCoversEmptySpace(t *testing.T)      // Drag rects cover everything outside buttons and Search, never overlapping them
```

- [ ] **Step 2:** Run `go test ./internal/ui -run Title`. Expected: FAIL (undefined).
- [ ] **Step 3:** Implement `layoutTitle` in `titlebar.go`. Sizes in dp: bar 42, icon buttons 30 × 28, caption buttons 46 wide, mark box 26, side padding 14 (8 inside the lead), search min 190 / max 400 / height 28. The crumb measures with a passed `measure func(s string, mono bool) int` field on `titleIn`, so tests use fixed widths.
- [ ] **Step 4:** Run `go test ./internal/ui -run Title`. Expected: PASS. Then run `go test ./internal/ui`, whose old `layoutBar` tests are replaced in this same commit by the ones above.
- [ ] **Step 5: Commit** `feat(ui): Lay out one title bar for both platforms`

### Task 3: Draw the title bar, wire it, and go frameless on Mac

**Files:**
- Modify:
  - `internal/ui/window.go` (`frame`: barH 42 everywhere; `drawBar`; caption events; double-click; the `AppKitViewEvent` hook)
  - `internal/ui/tabbar.go` (`layoutCaptions`, `layoutSearch` restyle)
  - `internal/ui/decor_windows.go` and `decor_other.go` (`customTitleBar = true` on both)
  - `cmd/kaze/main.go` (`app.Decorated(false)`)
  - `internal/ui/sidebar.go` and `sidebar_window.go` (remove the icon row, the rail and the filter's **+**)
  - `internal/ui/settings_style.go` (icons `plus`, `folder`, `branch`, `chip`, `tabs`, `up`, added to `iconNames` first)
- Test: `internal/ui/titlebar_draw_test.go`, existing sidebar and titlebar tests updated

**Interfaces:**
- Consumes: Task 1's `macShowLights`, `macTitleDoubleClick` and `macLightsInset`; Task 2's `layoutTitle`.
- Produces:
  - `func (win *window) crumb() crumbParts`: the sidebar label of the shown session, its repo name, its command and its state. "Settings" or "Agents" while those are shown.
  - Clicks: `capPlus` → `newTab()`; `capAgents` → `toggleBoard()`. Existing targets are unchanged.

- [ ] **Step 1: Write the failing tests**

```go
func TestCrumbUsesSidebarLabel(t *testing.T)        // a session with /ship-ticket TODO-142 → Title "TODO-142", Cmd "/ship-ticket", Repo from loc, State "running"
func TestCrumbOnSettingsAndBoard(t *testing.T)      // Settings shown → Title "Settings"; board → "Agents"; Cmd and State empty
func TestTitleClicksReachActions(t *testing.T)      // press+release on Plus opens a tab; on Agents toggles the board; on Search opens the palette; on Gear opens Settings
func TestSidebarHasNoIconRow(t *testing.T)          // layoutSidebar: no toggle/palette/scratch/agents/gear hits; filter spans the width; no plus hit
func TestTitleBarDrawsInEveryTheme(t *testing.T)    // drawBar in all five themes on Mac and Windows layouts without panics and with every button rect non-empty at 1200px
func TestMacDoubleClickCallsZoom(t *testing.T)      // a double click on Drag on darwin calls the injected macTitleDoubleClick once (function variable, stubbed in tests)
```

- [ ] **Step 2:** Run `go test ./internal/ui -run 'Crumb|TitleClicks|NoIconRow|TitleBarDraws|MacDoubleClick'`. Expected: FAIL.
- [ ] **Step 3:** Implement. Draw per spec 5.1:
  - **Lead:** the sidebar's colour and a right hairline while the sidebar shows.
  - **Crumb:** the state mark (`stateDot`), then the parts with `chrome.sans` and `chrome.label`, then the state pill (accent, amber or green soft fill, 9 dp radius).
  - **Search:** a card fill, a 1 px line and a raised key cap.
  - **Buttons:** 7 dp rounded hover fills; the Agents badge as in the old sidebar row. Close hover stays `CloseHover`.
  - **Mac:** on `app.AppKitViewEvent`, keep the view handle and call `macShowLights`. Read `macLightsInset` each frame. It's a cheap getter, cached until a resize or full-screen notification marks it stale.
- [ ] **Step 4:** Run the same tests, then all of `go test ./internal/ui`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Draw the shared title bar and drop the sidebar icon row`

### Task 4: Status model

**Files:**
- Modify: `internal/ui/status.go`, `internal/usage/usage.go`
- Test: `internal/ui/status_test.go`, `internal/usage/usage_test.go`

**Interfaces:**
- Produces:

```go
// usage.Detail is one limit for the hover card.
type Detail struct {
	Key   string // "5h", "wk", "ctx"
	Name  string // "5-hour", "Week", "Context"
	Pct   int
	Level Level
	Reset string // "resets in 2h 14m", "resets Mon 9:00", "this session"
}
func Details(u notify.Usage, now time.Time) []Detail

type statusModel struct {
	Folder, Branch   string
	Add, Del         int
	Update           string // "0.15" or ""
	Agents, Waiting  int
	Servers          []serverDot
	Usage            []usage.Detail
	MemMB, Tabs      int
	Notice           string
}
func (win *window) statusModel(now time.Time) statusModel // replaces statusParts
```

- [ ] **Step 1: Write the failing tests**

```go
func TestUsageDetails(t *testing.T)            // 5h 23% resets in 2h 14m, wk 41% resets Mon 9:00, ctx 34% "this session"; levels at 80/95 as Segments
func TestStatusModelCounts(t *testing.T)       // agents running+waiting, waiting from the board's waiting and session waits, servers from runs, lines from linesOf
func TestStatusModelHidesUnknown(t *testing.T) // ShowUsage off → no Usage; outside a repo → no Branch, Add, Del
```

- [ ] **Step 2:** Run `go test ./internal/ui ./internal/usage -run 'UsageDetails|StatusModel'`. Expected: FAIL.
- [ ] **Step 3:** Implement. `Details` reuses `level` and `resetText`. `statusModel` reuses `statusText`'s memory read (same interval) and `curRuns`.
- [ ] **Step 4:** Run again. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Model the status bar's groups`

### Task 5: Status bar layout, drawing and hover card

**Files:**
- Modify: `internal/ui/tabbar.go` (replace `layoutStatus`, `layoutUsage`, `layoutServers`), `internal/ui/window.go` (status events, hover tag)
- Create: `internal/ui/statusbar.go` (pure layout and drawing)
- Test: `internal/ui/statusbar_test.go`

**Interfaces:**
- Produces:

```go
type statusGroup int // grpFolder, grpBranch, grpUpdate, grpAgents, grpServers, grpUsage, grpMem, grpTabs
type statusLayout struct {
	Groups map[statusGroup]image.Rectangle // only those shown
	Compact struct{ ServersLabel, UsageLabels bool } // dropped parts (spec 6.4)
}
func layoutStatusBar(m statusModel, width int, measure func(string) int, d statusDims) statusLayout
```

- Hits: Update → `updClick`; Agents → `toggleBoard`; Servers → `showServer`; Usage → Settings › Agents. Hovering grpUsage sets `win.usageCard = true` until the pointer leaves, which costs one frame each way.

- [ ] **Step 1: Write the failing tests**

```go
func TestStatusOrderAndSeparators(t *testing.T) // full model at 1200px: groups left to right as spec 6.1; no two rects overlap
func TestStatusDropOrder(t *testing.T)          // shrinking: tabs, mem, servers label, usage labels go in that order; branch/agents/update/usage numbers stay at 600px
func TestStatusHidesEmptyGroups(t *testing.T)   // zero agents, no update, no runs, no usage → only folder, branch, mem, tabs
func TestStatusWaitingPill(t *testing.T)        // Waiting>0 → agents text "4 agents · 1 waiting" with the needs colour
func TestStatusClicks(t *testing.T)             // press on each clickable group calls its action; folder/branch/mem do nothing
func TestUsageCardOnHoverOnly(t *testing.T)     // pointer Enter on usage → card drawn with three lines; Leave → gone; no InvalidateCmd timers requested
func TestStatusInEveryTheme(t *testing.T)       // draws in all five themes without panic
```

- [ ] **Step 2:** Run `go test ./internal/ui -run 'Status|UsageCard'`. Expected: FAIL.
- [ ] **Step 3:** Implement per spec 6.1 and 6.2:
  - **Group padding:** 9 dp, 7 dp hover radius, 14 dp hairline separators.
  - **Meters:** 38 × 4 dp with a track colour mixed from `Border`. Levels use `usageColor`.
  - **Pills:** 18 dp tall with the accent soft fill, or the amber one when something waits.
  - **Usage card:** 290 dp wide, anchored to the right of the usage group, above the bar, drawn after the toasts so it sits on top. It takes no input.
- [ ] **Step 4:** Run again, then `go test ./internal/ui`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Redesign the status bar`

### Task 6: Check in the real app, then the docs

**Files:**
- Modify: `README.md` (the sidebar, title bar and status bar sections), `docs/HANDOFF.md`, `docs/site-assets/SCREENSHOTS.md` (note that the captures need retaking)

- [ ] **Step 1:** Build to the scratchpad. Using the throwaway input-blocked capture overlay and the demo home (as for `kaze-agents.png`), capture:
  - Midnight and Washi
  - sidebar shown and hidden
  - a 700 dp-wide window
  - Settings open
  
  Check each against the mockup.
- [ ] **Step 2:** Run `GOOS=windows GOARCH=amd64 go build ./cmd/kaze` and `GOARCH=arm64`. Expected: both build. Note in HANDOFF that the Windows run itself is unverified until the owner tries it.
- [ ] **Step 3:** Run `go test ./...`, `go vet ./...` and `go test -race ./internal/ui -run 'Title|Status'`. Expected: PASS.
- [ ] **Step 4:** Update the docs. Commit: `docs: Describe the new title bar and status bar`

After merge: release 0.15 per CLAUDE.md, with a changelog row. Every lander capture shows the old chrome, so retake them all with the existing demo home and overlay.
