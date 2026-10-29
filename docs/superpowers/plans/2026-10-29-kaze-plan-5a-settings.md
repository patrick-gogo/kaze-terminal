# kaze Plan 5a: Settings page, themes, Claude section, version

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A live settings page in kaze with five themes, appearance options, the Claude alert/usage options with a Claude Code setup check, and the build version.

**Architecture:** Themes become data (`internal/theme`) that both the renderer palette and the chrome draw from, replacing the package-level color vars. The settings page is a view the window shows instead of the terminal, with its own chip in the tab strip; it is not a `tabs.Manager` tab, so no shell is started and the manager stays untouched. Each control changes the live config struct, re-applies it, and saves just that key into `config.toml` with an in-place line editor.

**Tech stack:** Go 1.27, Gio v0.10.3, BurntSushi/toml (read only), `runtime/debug` build info.

**Spec:** `docs/superpowers/specs/2026-10-29-kaze-settings-page-design.md`; mockup `docs/design/settings-and-themes.html` (the `T` object holds the theme values).

## Global constraints

- Idle CPU stays zero: no tickers. Only the cursor blink (when on, focused, terminal shown) schedules wakes.
- Midnight must look exactly as today: its values equal the current code constants.
- `config.toml` saves keep every other line and comment; new keys go above the first `[table]` header; writes are atomic (temp file + rename); an unparsable file is never written.
- Claude Code's `settings.json` is written only after the owner confirms a dialog that shows the exact change, after a `settings.json.bak-YYYYMMDD-HHMMSS` copy. Non-kaze hooks are never changed. A kaze entry pointing at a different existing exe is OK, not broken.
- Theme keys: `midnight`, `sumi`, `tide`, `washi`, `sakura`. Unknown → warning, Midnight.
- New config keys and defaults: `theme = "midnight"`, `line_spacing = "normal"` (`tight|normal|roomy`), `cursor_style = "block"` (`block|bar|underline`), `cursor_blink = false`, `taskbar_flash = true`, `done_toast = "6s"` (`3s|6s|10s|until_seen`), `show_usage = true`. Bad values warn and use the default.
- Font size in the page: 8 to 32, step 1.
- Version constant `0.5`. Build line: `build <7-char commit> · <Jan 2, 3:04 PM local>`, `+ local changes` when `vcs.modified=true`, `build unknown` without VCS info.
- Plain language in UI text; no em dashes.

## Review focus

1. Switching theme while a toast, an alert dot and the usage bar are showing: everything repaints in the new theme the same frame, nothing keeps Midnight colors.
2. Font size or line spacing change with several tabs: every tab's PTY is resized to the new grid; no tab keeps the old cell size.
3. A `config.toml` with comments, a `[colors]` table and CRLF line endings: saving `theme` keeps all of it and puts `theme` above `[colors]`.
4. `~/.claude/settings.json` missing, empty, or not valid JSON: the Claude section shows "can't read" and offers no Fix.
5. Washi (light) with the contrast nudge: dim text on Washi's paper is pulled darker, never toward white.

---

### Task 1: Theme data

**Files:** create `internal/theme/theme.go`, `internal/theme/theme_test.go`

**Interfaces (produces):**
- `type TabShape int` with `Rounded`, `Underline`, `Pill`.
- `type Chrome struct` with one `color.NRGBA` field per current chrome var: `Chrome, Active, ActiveFG, Muted, Border, Accent, StatusFG, Folder, Branch, CapGlyph, CapHover, CapPress, CloseHover, ClosePress, Needs, Done, ToastBG, ToastLine, ToastFG, ToastGo, ToastTitle, UsageHigh`.
- `type Theme struct { Key, Name string; Term render.Palette; UI Chrome; Tabs TabShape }`.
- `func All() []Theme` (order: midnight, sumi, tide, washi, sakura); `func ByKey(k string) (Theme, bool)`; `func Default() Theme` (midnight).

**Values:** Midnight's `Term` is `render.DefaultPalette()` and its `UI` the current constants in `internal/ui/tabbar.go`, `alerts.go`, `usage.go`. The other four take bg/chrome/fg/dim/muted/border/accent/cursor/sel/toast and the 16 ANSI colors from the mockup's `T`; derive the rest: `ActiveFG` = fg brightened (white on dark themes, fg on Washi), `StatusFG` = dim, `Folder` = fg, `Branch` = ANSI 2, `Needs` = ANSI 3, `Done` = ANSI 2, `ToastFG` = fg, `ToastGo` = accent, `ToastTitle` = ActiveFG, `ToastLine` = border, `UsageHigh` = ANSI 9, caption hover/press = border lightened/darkened slightly, close hover/press = Midnight's red. Washi: `Needs` `#9a6a00`, `Done` `#2e7d4f`, `UsageHigh` `#c0392b`. Tabs: Sumi `Underline`, Tide `Pill`, others `Rounded`.

- [ ] Test `TestMidnightMatchesTodaysColors`: `Default().Term == render.DefaultPalette()`, `UI.Accent == #8b7bff`, `UI.Chrome == #0b0d12`, `UI.Needs == #f2b84b`, `UI.UsageHigh == #ff6b6b`.
- [ ] Test `TestAllThemes`: five keys in order; `ByKey("sumi").Tabs == Underline`, `ByKey("tide").Tabs == Pill`; `ByKey("nope")` returns false; every theme's `Term.FG` vs `Term.BG` contrast ≥ 7 (use a local luminance helper); Washi `Term.BG` is light (luminance > 0.8).
- [ ] Run `go test ./internal/theme/` → FAIL, implement, PASS. Commit `feat(theme): Add the five built-in themes`.

### Task 2: Draw everything from the theme

**Files:** modify `internal/ui/tabbar.go`, `alerts.go`, `alerts_ui.go`, `usage.go`, `status.go`, `titlebar.go`, `window.go`, `logic.go`; `internal/render/palette.go`, `renderer.go`; tests in `internal/ui/theme_test.go`, `internal/render/palette_test.go`.

**Interfaces:**
- `chrome` gains `th theme.Chrome` and `shape theme.TabShape`; every `colX` use becomes `c.th.X`; the package-level color vars are deleted (except `rgb`).
- `func (win *window) applyTheme(t theme.Theme)`: sets `win.pal = applyColors(t.Term, cfg.Colors)`, `win.ui.th`, `win.ui.shape`, drops `win.rend` (rebuilt next frame) and invalidates.
- Tab drawing honors `shape`: Rounded = today; Underline = no fill, 2dp accent line under the active tab; Pill = fully rounded, active gets accent at 13% alpha fill plus a 1px accent outline.
- `render.readable(fg, bg, ink color.NRGBA)`: candidates are white and `ink`; renderer passes `ink` = the darker of `pal.FG` and `pal.BG`. Midnight output is unchanged (its ink is `#0f1117`).
- `Run` picks the theme with `theme.ByKey(cfg.Theme)` (Task 3 adds the key; until then use Default).

- [ ] Test `TestReadablePullsTowardThemeInk`: on bg `#f6f2ea` with fg `#d8d2c4`, `readable(fg, bg, #2b2a33)` has contrast ≥ 4.5 and is darker than fg.
- [ ] Test `TestMidnightRenderUnchanged`: existing renderer tests pass unchanged (run them).
- [ ] Test `TestApplyThemeRepaintsChrome`: after `applyTheme(sumi)`, `win.ui.th.Accent == #e2553d`, `win.pal.BG == #111111`, `win.rend == nil`; a `[colors] bg` override still wins over the theme.
- [ ] Test `TestToastAndUsageUseTheme`: with Washi applied, `usageColor(usage.Warn) == #9a6a00`.
- [ ] Run `go test ./internal/ui/ ./internal/render/` → PASS. Commit `refactor(ui): Draw the chrome and terminal from the current theme`.

### Task 3: New config keys and the in-place writer

**Files:** modify `internal/config/config.go`, `config_test.go`; create `internal/config/write.go`, `write_test.go`.

**Interfaces:**
- `Config` gains `Theme string`, `LineSpacing string`, `CursorStyle string`, `CursorBlink bool`, `TaskbarFlash bool`, `DoneToast string`, `ShowUsage bool` with the TOML names and defaults from Global constraints; `Load` validates them like the others (warn + default).
- `func Set(path, key string, value any) error`: value is `string`, `bool`, `int` or `float64`. Reads the file (missing → creates it with just the key). Parses with toml first; parse error → returns it, writes nothing. Replaces the first top-level `key = ...` line (before any `[table]`), keeping a trailing `# comment`; else inserts `key = value` right before the first `[table]` line, or at the end. Keeps the file's line endings (CRLF stays CRLF). Writes `path.tmp` then renames.

- [ ] Tests: `TestSetReplacesKeepingComments` (file with a header comment, `font_size = 14  # big`, `[colors]` with `bg`) → after `Set(p, "font_size", 16)` the comment and `[colors]` are byte-identical and the line reads `font_size = 16  # big`; `TestSetInsertsAboveFirstTable` (`theme` lands before `[colors]` and `Load` reads it as top-level); `TestSetKeepsCRLF`; `TestSetRefusesBrokenFile` (file `font_size = = 3` unchanged after an error); `TestSetCreatesMissingFile`; `TestSetQuotesStrings` (value with `"` and `\` round-trips through `Load`); `TestLoadNewKeys` defaults and one bad value per key warns.
- [ ] Run → FAIL, implement, PASS. Commit `feat(config): Add appearance and alert keys and save single keys in place`.

### Task 4: Line spacing, cursor style and blink in the renderer

**Files:** modify `internal/render/fonts.go`, `glyphs.go`, `renderer.go`, tests `renderer_test.go`; modify `internal/ui/window.go`.

**Interfaces:**
- `type CursorStyle int` (`CursorBlock`, `CursorBar`, `CursorUnderline`) and `type Options struct { LineScale float32; Cursor CursorStyle }` in `render`.
- `NewRenderer(fontSize float32, palette Palette, o Options) *Renderer`. `sharedGlyphs` keys by size **and** scale. Cell height = `round(base * LineScale)`, glyphs and box drawing vertically centered in the extra space (box drawing fills the whole cell so lines still join). Scales: tight `0.92`, normal `1.0`, roomy `1.2`.
- Bar cursor: 2px wide at the cell's left; underline: 2px at the bottom; both in `pal.Cursor`, drawn as ops (no image). Block unchanged.
- `func (r *Renderer) SetCursorShown(bool)`: blink phase; hidden draws nothing for the cursor.
- Window: when `cursor_blink` is on, the terminal is shown and the window focused, toggle every 530ms via `op.InvalidateCmd{At: next}`; any key input shows the cursor and restarts the phase. Off, unfocused or settings shown: cursor steady, no wakes.

- [ ] Tests: `TestLineScaleChangesCellHeight` (roomy cellH > normal cellH; tight < normal; cellW equal); `TestBoxDrawingFillsScaledCell` (a `│` at roomy reaches row 0 and the last row of the cell); `TestBarAndUnderlineCursor` (painted ops include a 2px rect at the left / bottom; no cursor image); `TestCursorHiddenPhase`; window test `TestBlinkSchedulesWakeOnlyWhenFocused`.
- [ ] Run → PASS. Commit `feat(render): Add line spacing, cursor styles and an optional blink`.

### Task 5: Version

**Files:** create `internal/version/version.go`, `version_test.go`; modify `cmd/kaze/main.go`.

**Interfaces:** `const Number = "0.5"`; `type Build struct { Commit string; Time time.Time; Modified bool }`; `func Read(info *debug.BuildInfo, ok bool) Build`; `func (b Build) Line() string` (`build 6fa6324 · Oct 29, 9:19 AM`, ` + local changes` appended when modified, `build unknown`); `func Text() string` = `"kaze " + Number + "\n" + Line()`. `kaze --version` prints `Text()` and exits 0 (before window code, like `notify`).

- [ ] Tests: settings from a fake `BuildInfo` (`vcs.revision`, `vcs.time` RFC3339, `vcs.modified`), no VCS settings → `build unknown`, `ok=false` → unknown.
- [ ] Run → PASS. Commit `feat(cli): Show the version and build`.

### Task 6: The settings view and its controls

**Files:** create `internal/ui/settings.go` (state, open/close, layout of nav and sections), `internal/ui/controls.go` (drawing + hit areas), `internal/ui/settings_test.go`; modify `tabbar.go` (Settings chip after the tabs, gear button), `titlebar.go` (gear hit area left of minimize), `logic.go` (`open_settings` action, default Ctrl+, / Cmd+,), `window.go` (show settings instead of the terminal).

**Interfaces:**
- `win.set settingsState { open, shown bool; section int; focus int; ... }`; `func (win *window) openSettings()` (opens or shows it), `closeSettings()`. Selecting a terminal tab sets `shown=false`; clicking the chip shows it; its × closes.
- Control kinds in `controls.go`: `button`, `toggle`, `segmented` (options + selected), `stepper` (min, max, value), `swatch` (theme). Each laid out with its rect recorded in a per-frame `[]control{kind, rect, id}` list and one `event.Op` tag; a click at a rect, or Enter/Space on the focused one, calls `win.activate(id, arg)`. Tab / Shift+Tab move focus; the focused control gets a 1px accent ring.
- Layout per mockup: nav 190dp wide with Appearance / Claude (Workspaces is greyed with "coming next"), the dim "Later" line, the version block (`kaze 0.5`, `version.Read(debug.ReadBuildInfo()).Line()`) at the bottom. Rows: label (+ dim hint) left, control right, hairline between rows.
- While shown: no terminal key input, renderer not laid out, PTYs keep running; the window title is `kaze · Settings`.

- [ ] Tests: `TestCtrlCommaOpensSettings`; `TestSettingsChipAndTabSwitch` (open → chip present and shown; select tab 1 → hidden but open; click chip → shown; × → closed); `TestClickTogglesControl` (a toggle row flips via its recorded rect); `TestTabMovesFocusAndEnterActivates`; `TestTerminalGetsNoKeysWhileSettingsShown`; `TestVersionBlockShowsBuild` (layout with a stubbed build → the label list contains `kaze 0.5`).
- [ ] Run → PASS. Commit `feat(ui): Add the settings page with its controls`.

### Task 7: Appearance section

**Files:** create `internal/ui/settings_appearance.go`, test `settings_appearance_test.go`; modify `window.go`.

**Interfaces:** `func (win *window) setTheme(key string)`, `setFontSize(int)`, `setLineSpacing(string)`, `setCursorStyle(string)`, `setBlink(bool)`: each updates `win.opt.Config`, applies live (theme via Task 2's `applyTheme`; font/spacing/cursor drop `win.rend` so the next frame rebuilds it and resizes every tab's PTY), then `config.Set(config.Path(), key, value)`. A save error shows in the status-bar notice and the live change stays. Swatches show the theme name, five ANSI chips (1 to 5) and the accent; the current one is ringed.

- [ ] Tests: `TestThemeSwatchAppliesAndSaves` (temp config path via a `win.cfgPath` field; click Tide → `win.ui.th.Accent == #40e0d0` and the file has `theme = "tide"`); `TestFontStepperClamps` (at 32, + does nothing; saved value 32); `TestFontChangeResizesAllTabs` (two tabs with recording PTYs; after +1 both got a resize to the new grid); `TestSaveErrorShowsNotice` (unwritable path → notice set, theme still applied); Review focus 3 test reused through `config.Set`.
- [ ] Run → PASS. Commit `feat(ui): Add the Appearance settings`.

### Task 8: Claude section and Claude Code setup check

**Files:** create `internal/claudesetup/setup.go`, `setup_test.go`, `internal/ui/settings_claude.go`, `settings_claude_test.go`; modify `alerts.go` (toast duration), `alerts_ui.go` (flash flag), `status.go` (usage toggle), `window.go` (chime player rebuilt when a chime turns on).

**Interfaces:**
- `claudesetup`: `type State int` (`OK`, `Missing`, `Broken`, `Taken`, `Unreadable`); `type Entry struct { State State; Exe, Command string }`; `type Report struct { Stop, Needs, StatusLine Entry }`; `func Inspect(settings []byte, exists func(string) bool) Report` (a kaze entry is a command whose first word, unquoted, has base name `kaze.exe` or `kaze` and whose args are `notify done`, `notify needs` or `statusline`; `Needs` is found under `Notification`); `func Fix(settings []byte, r Report, exe string) (out []byte, summary []string, err error)` adds missing hooks (Stop: `"<exe>" notify done`, timeout 3; Notification with matcher `permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input`: `"<exe>" notify needs`, timeout 3), repoints Broken entries, adds a Missing status line; never touches others; keeps unknown keys; output is 2-space indented JSON. `func Replace(settings []byte, exe string) ([]byte, string)` for Taken (returns the removed command). `func Write(path string, data []byte, now time.Time) error` copies to `settings.json.bak-YYYYMMDD-HHMMSS`, then writes atomically.
- UI rows (spec 6.3): needs chime + Test (plays `win.chime` now), done chime, taskbar flash, done toast (3s/6s/10s/Until seen; `until_seen` = no `Until`), show usage, and the setup row: state per entry in plain words (`Alerts when Claude needs you: OK · kaze-app\kaze.exe`), running exe path in dim text, and Fix / Replace buttons when relevant. Fix/Replace open a dialog over the page listing `summary` lines and the JSON lines that change, with Cancel / Write buttons. The report is read when the section is shown and after a write, never per frame.

- [ ] Tests (claudesetup): the owner's current shape (hooks plus other node hooks, kaze status line) → all OK; missing Stop → Fix adds exactly one Stop hook and leaves the others byte-equal after re-marshal; kaze entry with a non-existent exe → Broken, Fix repoints it; different existing exe → OK; status line `bash x.sh` → Taken, Fix leaves it, Replace swaps it and reports `bash x.sh`; empty file / invalid JSON → Unreadable and Fix returns an error (Review focus 4); `Write` creates the backup with the old bytes.
- [ ] Tests (UI): `TestDoneToastSetting` (`3s` → toast `Until` = now+3s; `until_seen` → zero); `TestFlashOffSuppressesFlash`; `TestShowUsageOff` (statusParts().Usage empty); `TestChimeTestButtonPlays`; `TestFixDialogNeedsConfirm` (Fix click → dialog open, file unchanged; Write click → file changed and backup exists).
- [ ] Run → PASS. Commit `feat(ui): Add the Claude settings and the Claude Code setup check`.

### Task 9: Docs, version and finish

README: "Settings" section (Ctrl+, / gear, what saves where, new keys table), themes list, `kaze --version`. HANDOFF: Plan 5a state, next = Plan 5b Workspaces. Spec section 6.6 note. Run every package's tests, build `kaze-app\kaze.exe`, warm it up (`notify done`, `statusline`, `--version`). Commit `docs: Document the settings page and themes`.
