# Plan 6e: the palette Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One key (or the title bar's search box) opens a palette that lists what is waiting on the owner, every session, every skill and slash command in their Claude setup, and kaze's own actions, and acts on the chosen one; Settings gains a read-only Commands section.

**Architecture:** A new `internal/skills` package reads the owner's skill and command files when the palette opens (off the UI goroutine, cached by modification time, never written). A pure `buildPalette` turns sessions, members, skills, recent prompts, kaze actions and the query into sectioned items; the window draws them over the main view and acts. Starting a skill follows spec 9.3: type into the current Claude session without Enter, or open a new session and type `claude '<prompt>'` plus Enter once its shell is ready. One table of kaze's commands feeds both the palette's kaze section and the Settings Commands section.

**Tech Stack:** Go 1.27, Gio v0.10.3, standard library only (no YAML dependency; `go.mod` has none). Tests with `go test`.

**Spec:** `docs/superpowers/specs/2026-10-30-kaze-agent-sidebar-design.md` sections 9.3 (starting a prompt), 10 (10.1 to 10.4), 11 (the `palette` key), 15 item 5. Look and behavior: the 6e column of `docs/design/agent-6b-6d-6e-mockup.html` (read `buildPal`, `drawPal`, `kazeActions`, `runSkill` and the `commands` branch of `drawSettings`). Where the spec and the mockup disagree, the spec wins unless this plan's Decisions say otherwise.

**Depends on Plan 6b (`feat/agent-awareness`), not yet on `main`.** Branch this plan from `main` only after 6b has merged. It uses these 6b names as a fixed interface; if 6b lands with different names, adapt the calls, not the behavior:
- `type agentState struct{ claude, running bool; command, arg string; shellAt uint64 }` and `func (win *window) agentOf(id tabs.TabID) agentState` (`internal/ui/agent.go`).
- `sideRow` fields `Command string`, `Claude, Running bool`; `sideInput.Agent`, `sideInput.Name`.
- `actNextWaiting` (config name `next_waiting`), `func (win *window) nextWaiting()`.
- `func (win *window) dropSideEdit()` (6b's replacement for `dropFilter`, also ending a rename).
- "Needs the owner" is the alert mark: `win.alerts.markOf(id) == markNeeds` (6b keeps needs in marks, not in `agentState`).

**Merge notes.** 6e and 6b both change these functions; take 6b's version and add 6e's lines on top:
- `focusTarget` (`window.go`): 6e puts the palette's field first, before 6b's rename field and the filter.
- `sidePress` (`sidebar_window.go`): 6e adds the macOS palette button case next to `h.toggle` / `h.gear`; 6b added the rename double click and the "Next waiting" button.
- `runShortcut` (`window.go`): 6e adds `actPalette`; 6b added `actNextWaiting`.
- `defaultBindings` (`logic.go`): 6e adds `palette` next to 6b's `next_waiting`; also `actionByName` and `config.KnownActions`.
- If Plan 6d (floating panes, scratch shell) merged first: acting on a workspace member opens it floating (use 6d's member-click function instead of `selectTab`), `scratch_shell` joins the command table (Task 3), and the macOS rail keeps 6d's scratch button below the palette button.

**Not in 6e:** chips and timed checks (6c, parked): the "Waiting on you" section lists only sessions in the needs-you state, and the Commands section leaves out `kaze status` and the check line format. Changing shortcuts from Settings (read-only list).

## Decisions

1. **Match rule:** the query is lowercased, trimmed, and one leading `/` dropped; an item matches when its text contains the query anywhere (`strings.Contains`), case-insensitive (spec 10.1). Inside each section, items rank: name starts with the query (for `plugin:name`, the part after `:` counts too), then name contains it, then only the other text (description, folder, branch, command, key) contains it. Ties keep the section's base order: Waiting and Sessions in sidebar order, then members by run; Skills recently used first (most recent first), then by name, case-insensitive; kaze in the command table's order. An empty query matches everything in base order.
2. **A needs-you session shows once,** under Waiting on you, not again under Sessions (as the mockup's `buildPal` does). Sessions lists every other session and every workspace member.
3. **Frontmatter without YAML:** only a file whose first line is exactly `---` has frontmatter, ending at the next `---` or `...` line. Top-level `key: value` lines are read for `name` and `description` only. Values: matching `"` or `'` quotes removed (`\"`, `\\` in double quotes, `''` in single quotes unescaped); block scalars (`|`, `>`, with `-`/`+`) and indented continuation lines are joined with single spaces. Everything else is ignored. At most 32 KB of a file is read. Name must match `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`, else the folder or file name is used, else the file is skipped. Description: control characters dropped, whitespace collapsed, cut to 200 runes; fallback is the first non-empty body line that is not `---`, with leading `#` and spaces trimmed.
4. **Plugin newest version:** Claude Code's own record wins: `~/.claude/plugins/installed_plugins.json` lists each `name@marketplace` with `installPath` and `version`; take the entry with the highest version (rule below), ties by later `lastUpdated`, and use its `installPath` when it is a folder inside `~/.claude/plugins`. Without a usable record, scan `~/.claude/plugins/cache/<marketplace>/<name>/*/` and pick the highest folder name semver-ish: a name matching `^v?\d+(\.\d+)*([-+].*)?$` compares numerically part by part, missing parts count as 0, and a `-pre` suffix is older than the same version without one; any semver-ish name beats a non-semver name (the owner's cache has 12-hex git hashes for some plugins); among non-semver names the newer folder modification time wins, then the larger name.
5. **Enabled plugins:** `enabledPlugins` in `~/.claude/settings.json` (an object of `"name@marketplace": true`), read with `encoding/json`; only the value `true` counts. Plugin entries are shown as `plugin:name`. Unreadable settings mean no plugin entries; user and project entries still show.
6. **Names and duplicates:** a command's name is its file name without `.md` (subfolders only organize). Shown with a leading `/`. When a user and a project entry share a name, the project one wins; in one scope a skill beats a command of the same name.
7. **Type without Enter:** reuse the PTY write path typed text takes in `route`: `takeOver(t)`, `t.Session.Write(input.EncodeText(prompt + " "))`, `snapToBottom`. Not `EncodePaste`: a bracketed paste reaches Claude Code as pasted text, not as a typed slash command. The prompt is cleaned first (control characters and newlines dropped, at most 512 runes), so nothing typed can press Enter.
8. **New session readiness:** the new tab opens in the current session's folder (its last OSC 7 report, else the default folder). The line `claude <quoted prompt>` plus `\r` is written on the first prompt marker (`promptSeen`, Plan 5b's signal); if none arrives by `confirmAfter` (3 s), nothing is sent on the timer: the tab shows the same "Waiting for shell prompt" banner Plan 5b uses for workspaces, with **Run commands** (writes the line now) and **Use shell** (drops it). This follows the workspaces decision "never send on a timer alone" (docs/superpowers/specs/2026-10-29-kaze-workspaces-decisions.md section 2). One `op.InvalidateCmd` wake at that deadline to show the banner, no ticker. The owner typing into that tab first, or the tab closing or exiting, cancels it.
9. **Quoting:** single quotes for both supported shells (`$`, backticks and `;` stay literal): POSIX `'` becomes `'\''`; PowerShell `'` and the curly quotes `‘ ’ ‚ ‛` are doubled. Any other shell (cmd.exe) gets an error notice and nothing opens. The spec's `claude "<prompt>"` is the idea; single quotes are the safe form.
10. **Recent 10:** the last 10 distinct prompts started from the palette, most recent first, moved to the front on reuse, memory only, kept on the window for as long as kaze runs.
11. **"Toast" means the status-bar notice** (`win.note`), as 6b does for "Nothing is waiting on you.": `Typed /x into <title>. Press Enter to send.` and `Opened a new session for /x.`
12. **Current session:** the active tab when it is a session (not a workspace member) and the settings page is not in front. Otherwise there is none, and a skill opens a new session.

## Global Constraints

- Windows first; macOS must build and behave the same where it applies. `GOOS=windows go vet ./...` must pass; darwin vet of Gio packages can't run on Windows (the owner checks on the Mac).
- Go is at `%LOCALAPPDATA%\Programs\go\bin`; set `GOTMPDIR` under `C:\Users\john\Desktop\kaze-wt\`. If Smart App Control blocks a test binary, rerun with `go test -ldflags "-X main.r=<random>"` or `-ldflags "-B 0x<random hex>"`, a new value each retry. Never turn Smart App Control off.
- Idle CPU stays at zero: no tickers. A closed palette costs nothing; an open one redraws on input, a skill load arriving, or the hovered row changing, never while the pointer rests. The new-session launch adds one wake at its deadline only.
- Skill files are read only when the palette opens, on a goroutine, at most one scan in flight; unchanged files (same modification time and size) are not read again; nothing under `~/.claude` or `.claude` is ever written.
- Every color from theme tokens (`c.th.*`, `c.memberColor`); all five themes, including the light Washi, must read well.
- Tests never read or write the owner's real `~/.claude`, `config.toml` or `workspaces.toml`: build fake skill and plugin trees in `t.TempDir()` and point the window's `claudePath` / `cfgPath` there. Never send input to the owner's windows, never open visible windows, never run real Docker or `claude`.
- RED tests must fail safely and be bounded: fixed sizes, bounded loops, channel waits with a timeout (2 s).
- Known failures before this plan (Windows, also failing on `main`): `internal/ui` `TestPickFillsFolderTildeAndEmptyTitle`, `TestListMembersShowCommandsWithoutCutting`; `internal/workspaces` `TestExpandFolder`, `TestJoinedPOSIXFieldsKeepCurrentShellContext`; `internal/ptyx` `TestResolveFindsTheProgramOnPath`. Confirm on the base branch before Task 1; anything else red is yours.
- Commits: conventional, capitalized verb (`feat(ui): Open the palette`). No AI attribution lines of any kind.
- No em dashes in user-visible text, docs or comments. Comments say why, in 1 to 2 lines.

## Review Focus

1. A hostile or broken skill tree (a 10 MB SKILL.md, binary bytes, frontmatter that never closes, a symlink loop under `commands/`, 5000 files): the palette still opens, bad files are skipped, reads stay bounded and the UI goroutine never waits on the disk (Task 1, Task 6).
2. The session's state changes between opening the palette and pressing Enter (Claude starts needing the owner): the 9.3 rule is decided at the moment of acting, so nothing is typed into a permission menu (Task 5).
3. A prompt with `'`, `$`, backticks, `;`, a newline or Japanese text, sent to a new session: quoted so the shell runs exactly `claude` with one argument; cmd.exe gets an error and no session (Task 5).
4. Plugins the owner really has: hash-named version folders, two versions installed, a disabled plugin, an enabled plugin that is not installed, no `installed_plugins.json` (Task 2).
5. The new session closes, or its shell exits, before it is ready: the pending line is dropped and never written to another tab (Task 5).

---

### Task 1: The skill catalog (user and project)

**Files:**
- Create: `internal/skills/frontmatter.go`, `internal/skills/catalog.go`
- Modify: `internal/gitinfo/gitinfo.go`
- Test: `internal/skills/frontmatter_test.go`, `internal/skills/catalog_test.go`, `internal/gitinfo/gitinfo_test.go`

**Interfaces:**
- Produces:

```go
package skills

type Scope int
const (User Scope = iota; Project; Plugin)

type Skill struct {
	Name  string // as typed after "/": "ship-ticket", or "superpowers:brainstorming" for a plugin
	Desc  string // one line, at most 200 runes
	Scope Scope
	Path  string
}
type Sources struct {
	ClaudeDir string // the owner's ~/.claude; "" lists no user or plugin entries
	Project   string // the current session's checkout top; "" lists no project entries
}
type Cache struct{ /* files map[string]fileEntry; open func(string) (io.ReadCloser, error) */ }

func NewCache() *Cache
func (c *Cache) Load(src Sources) []Skill // deduped (Decision 6), sorted by Name case-insensitively, at most MaxSkills
func parseFile(data []byte, fallback string) (name, desc string, ok bool)

const (MaxSkills = 1000; maxRead = 32 << 10; maxCmdDepth = 3)
```

- Produces: `func gitinfo.Top(dir string) string`, the checkout's top folder (a worktree's own top), `""` outside a repo; reuses `locate`.

Sources read (spec 10.2): `<ClaudeDir>/skills/*/SKILL.md`, `<ClaudeDir>/commands/**/*.md`, `<Project>/.claude/skills/*/SKILL.md`, `<Project>/.claude/commands/**/*.md`. `commands/**` uses `filepath.WalkDir` (it does not follow symlinked folders), at most `maxCmdDepth` levels, skipping folders that start with `.`. A file is reparsed only when its modification time or size changed; entries for files no longer seen are dropped. `Cache` is used by one goroutine at a time (the caller guarantees it, Task 6).

- [ ] **Step 1: Write the failing tests** (`frontmatter_test.go`), table-driven:

```go
func TestParseFrontmatter(t *testing.T)
// "---\nname: ship-ticket\ndescription: Open the PR\n---\nbody" -> "ship-ticket", "Open the PR"
// description: "quoted \"x\"" -> `quoted "x"`;  'it''s' -> "it's"
// description: >-\n  folded\n  text -> "folded text";  description: |\n  a\n  b -> "a b"
// "\ufeff---\r\nname: x\r\n---\r\n" (BOM, CRLF) -> "x"
// no frontmatter, body "\n\n# Heading here\nmore" -> fallback name, "Heading here"
// "---\nname: x\n" never closed -> treated as no frontmatter: fallback name, desc "---" lines skipped -> "name: x"
// name "bad name!" -> fallback name;  fallback "" too -> ok == false
// description of 1000 runes with \x1b and \t -> <= 200 runes, no control characters
```

- [ ] **Step 2: Run** `go test ./internal/skills/ -run Frontmatter` → FAIL, `undefined: parseFile`.

- [ ] **Step 3: Implement `frontmatter(data []byte) (fields map[string]string, body []byte, ok bool)` and `parseFile`** in `frontmatter.go` per Decision 3. No YAML library.

- [ ] **Step 4: Run** `go test ./internal/skills/ -run Frontmatter` → PASS.

- [ ] **Step 5: Write the failing tests** (`catalog_test.go`), each building a tree in `t.TempDir()`:

```go
func TestLoadUserAndProject(t *testing.T)      // user skills/a/SKILL.md, commands/b.md, commands/sub/c.md; project .claude/skills/d -> names a, b, c, d with scopes
func TestLoadProjectWinsOverUser(t *testing.T) // both define "x" -> one "x", Scope Project
func TestLoadSkillBeatsCommand(t *testing.T)   // user skills/x and commands/x.md -> one "x" from the SKILL.md
func TestLoadSkipsBadFiles(t *testing.T)       // unreadable (os.Open fails via c.open), binary bytes, 10 MB file -> skipped or read only maxRead bytes; others listed
func TestLoadIsBoundedInDepthAndCount(t *testing.T) // commands nested 6 deep -> only up to depth 3; 1200 files -> len == MaxSkills
func TestLoadRereadsOnlyChangedFiles(t *testing.T)  // c.open counts calls: Load twice -> second Load opens 0; os.Chtimes one file -> 1; delete one -> gone
func TestLoadNeverWrites(t *testing.T)         // listing of names, sizes and mod times of the whole tree before == after Load
func TestLoadEmptySources(t *testing.T)        // Sources{} -> empty, no panic
```

Add `TestTopOfAWorktree` and `TestTopOutsideARepo` to `gitinfo_test.go` (repos built from files, as the existing `Repo` tests do).

- [ ] **Step 6: Run** `go test ./internal/skills/ ./internal/gitinfo/ -run 'Load|Top'` → FAIL.

- [ ] **Step 7: Implement `Cache`, `NewCache`, `Load`** in `catalog.go` and `gitinfo.Top`.

- [ ] **Step 8: Run** `go test ./internal/skills/ ./internal/gitinfo/` → PASS.

- [ ] **Step 9: Commit**

```bash
git add internal/skills internal/gitinfo
git commit -m "feat(skills): Read the owner's skills and commands, cached by modification time"
```

### Task 2: Plugin skills, newest version only

**Files:**
- Create: `internal/skills/plugins.go`
- Modify: `internal/skills/catalog.go` (`Load` adds plugin entries)
- Test: `internal/skills/plugins_test.go`

**Interfaces:**
- Consumes: `Cache`, `parseFile` (Task 1).
- Produces:

```go
func enabledPlugins(settings []byte) []string                      // "name@marketplace" with value true, sorted
func pluginRoot(claudeDir, key string, installed []byte) (string, bool) // Decision 4
func newerVersion(a, b string) bool                                // a newer than b, semver-ish
```

`Load` reads `<ClaudeDir>/settings.json` and `<ClaudeDir>/plugins/installed_plugins.json` through the same mod-time cache, then for each enabled plugin lists `<root>/skills/*/SKILL.md` and `<root>/commands/**/*.md`, naming each `plugin:name` (plugin = the key's part before the last `@`).

- [ ] **Step 1: Write the failing tests**

```go
func TestNewerVersion(t *testing.T)
// "1.0.1" > "1.0.0"; "0.12.0" > "0.11.0"; "1.10" > "1.9"; "v2" > "1.99"; "1.0.0" > "1.0.0-beta";
// "0.1.0" > "f8c5e7a1dcb7"; not ("1.0.0" > "1.0.0")
func TestEnabledPlugins(t *testing.T)
// {"enabledPlugins":{"a@m":true,"b@m":false,"c@m":"yes"}} -> ["a@m"]; invalid JSON -> nil; no key -> nil
func TestPluginRootFromInstalledRecord(t *testing.T)
// record lists two entries for "rl@m" (1.0.0 and 1.0.1) -> the 1.0.1 installPath
// installPath outside ClaudeDir/plugins or missing on disk -> falls back to the cache scan
func TestPluginRootScansTheCache(t *testing.T)
// no record; cache/m/rl/{1.0.0,1.0.1} -> 1.0.1; cache/m/sc/{1da406f428c2,f8c5e7a1dcb7} with Chtimes making 1da4 newer -> 1da4
func TestLoadPluginSkills(t *testing.T)
// enabled "superpowers@m" with skills/brainstorming and commands/help.md in its newest root,
// an older version with skills/old -> "superpowers:brainstorming", "superpowers:help"; no "superpowers:old"
func TestLoadSkipsDisabledAndMissingPlugins(t *testing.T) // false value, enabled but not installed -> no entries, user entries still listed
```

- [ ] **Step 2: Run** `go test ./internal/skills/ -run 'Version|Plugin'` → FAIL.

- [ ] **Step 3: Implement** in `plugins.go` with `encoding/json` per Decisions 4 and 5.

- [ ] **Step 4: Run** `go test ./internal/skills/` → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/skills
git commit -m "feat(skills): List skills of enabled plugins from their newest version"
```

### Task 3: The palette key and one table of kaze's commands

**Files:**
- Modify: `internal/config/config.go`, `internal/ui/logic.go`
- Create: `internal/ui/commands.go`
- Test: `internal/config/config_test.go`, `internal/ui/logic_test.go`, `internal/ui/commands_test.go`

**Interfaces:**
- Produces: action `actPalette`, config name `palette`, defaults Ctrl+Shift+K (Windows) and Cmd+K (macOS) (spec 11).
- Produces:

```go
// resolveBindings is buildKeymap's name-to-shortcut step, exposed for display.
func resolveBindings(goos string, overrides map[string]string) (map[string]input.Shortcut, []string)
func formatShortcut(goos string, sc input.Shortcut) string // "Ctrl+Shift+K", "Cmd+K", "Ctrl+Tab", "Ctrl+`"

type kazeCommand struct {
	Config string // config name under [keys]
	Label  string // shown in the palette and Settings
	Act    action
}
var kazeCommands []kazeCommand
```

- `buildKeymap` keeps its signature and builds on `resolveBindings`; the window stores the result as `win.binds map[string]input.Shortcut`.
- `formatShortcut` modifier order and words: Windows `Ctrl`, `Alt`, `Shift`, `Win`; macOS `Ctrl`, `Option`, `Shift`, `Cmd`. Key names: letters upper case, `Tab`, `Enter`, `Esc`, `Up`/`Down`/`Left`/`Right`, `PageUp`, `F1`..`F12`, symbols as typed.
- `kazeCommands`, in this order (labels from the mockup): `new_tab` New session; `close_tab` Close session; `next_tab` Next session; `prev_tab` Previous session; `copy` Copy; `paste` Paste; `open_settings` Open settings; `toggle_sidebar` Toggle sidebar; `next_waiting` Go to the next session that needs you; `palette` Open the palette; `scratch_shell` Scratch shell (only when 6d's `actScratchShell` exists on the base branch).

- [ ] **Step 1: Write the failing tests**

```go
func TestPaletteDefaults(t *testing.T)   // defaultBindings("windows")["palette"] == {Name:"K", Mods:ModCtrl|ModShift}; darwin {Name:"K", Mods:ModCommand}; config.KnownActions["palette"]; actionByName["palette"] == actPalette
func TestPaletteKeyNotPlainCtrlK(t *testing.T) // no default binding on windows is {K, ModCtrl}, {J, ModCtrl} or {B, ModCtrl} (spec 4)
func TestResolveBindingsKeepsOverrides(t *testing.T) // overrides {"palette":"ctrl+shift+p"} -> binds["palette"] == {P, Ctrl|Shift}; bad text -> default and one warning
func TestFormatShortcut(t *testing.T) // the examples above, plus windows {T, Ctrl|Shift} -> "Ctrl+Shift+T", darwin {",", Command} -> "Cmd+,", windows {X, Super} -> "Win+X", darwin {A, Alt} -> "Option+A"
func TestEveryKnownActionIsListed(t *testing.T) // each config.KnownActions name appears in kazeCommands exactly once, and each entry's Act == actionByName[Config]
```

- [ ] **Step 2: Run** `go test ./internal/config/ ./internal/ui/ -run 'Palette|ResolveBindings|FormatShortcut|EveryKnownAction'` → FAIL.

- [ ] **Step 3: Implement**: the action and defaults (merge note: `defaultBindings`), `resolveBindings`, `formatShortcut`, `kazeCommands`; `runShortcut` gains `case actPalette: win.togglePalette()` with a stub `togglePalette` that Task 6 fills (merge note: `runShortcut`).

- [ ] **Step 4: Run** `go test ./internal/config/ ./internal/ui/` → PASS (baseline aside).

- [ ] **Step 5: Commit**

```bash
git add internal/config internal/ui/logic.go internal/ui/logic_test.go internal/ui/commands.go internal/ui/commands_test.go internal/ui/window.go
git commit -m "feat(config): Add the palette shortcut and list kaze's commands in one place"
```

### Task 4: The palette model

**Files:**
- Create: `internal/ui/palette_model.go`
- Modify: `internal/ui/sidebar_model.go` (add `sessionRows`)
- Test: `internal/ui/palette_model_test.go`

**Interfaces:**
- Consumes: `sideRow`, `sideMember`, `sideModel` (6a/6b), `skills.Skill` (Task 1), `kazeCommand` (Task 3).
- Produces:

```go
type palSection int
const (palWaiting palSection = iota; palSessions; palSkills; palKaze)

type palItem struct {
	Section          palSection
	Key              string // stable across rebuilds: "tab:<id>", "skill:<name>", "act:<config>", "ws:<id>"
	Name, Command    string // Command: the row's " · /command" part, sessions only
	Detail           string // right-aligned dim text
	Mark             mark
	Running, Member  bool
	Color            string // member color
	Tab              tabs.TabID
	Prompt           string // "/" + skill name
	Act              action
	Workspace        string // workspace id for "Run workspace <name>"
}
type palAction struct {
	Name, Key, Config string // Key: formatShortcut text, "" for none
	Act               action
	Workspace         string
}
type palInput struct {
	Query    string
	Sessions []sideRow    // sidebar order, unfiltered
	Members  []sideMember // by run
	Skills   []skills.Skill
	Recent   []string     // prompts, most recent first
	Actions  []palAction
}
func buildPalette(in palInput) []palItem
func palQuery(q string) string
func sessionRows(m sideModel) []sideRow // groups then loose, every row, like sessionOrder

type recentPrompts struct{ list []string }
func (r *recentPrompts) add(p string)
const maxRecent = 10
```

Details (Decisions 1, 2, 10; mockup `buildPal`): Waiting items are sessions with `Mark == markNeeds`, Detail `needs you · go to session`. Session Detail is the state word (`running`, `done`, `idle` for a Claude session, `shell` otherwise) plus ` · go to session`; member Detail `server · ` + its status. Skill Name is `"/" + Name`, Detail its description. Kaze items come from `Actions`, Detail the key text. Session text searched: title, command, folder, branch; member: title, status; skill: name, description; action: name.

- [ ] **Step 1: Write the failing tests** (no window, plain values):

```go
func TestPaletteSectionsInOrder(t *testing.T)   // one of each kind -> sections Waiting, Sessions, Skills, Kaze in that order
func TestNeedsSessionOnlyUnderWaiting(t *testing.T)
func TestPaletteQueryContainsAnywhere(t *testing.T) // "ticket" matches "/ship-ticket" and a description "...ticket..."; "SHIP" matches case-insensitively; "/ship" == "ship"
func TestPaletteRanksNamePrefixFirst(t *testing.T)  // query "rev": "/review-mine" before "/ship-ticket" (desc "...review...") before nothing else
func TestPluginNameRanksByItsOwnPart(t *testing.T)  // "brain" puts "/superpowers:brainstorming" in the prefix tier
func TestSkillsRecentFirstThenByName(t *testing.T)  // Recent ["/b"] with skills c, a, b -> b, a, c
func TestSessionsKeepSidebarOrder(t *testing.T)
func TestRunWorkspaceItems(t *testing.T)            // Actions with Workspace "samurai" -> Name "Run workspace samurai", Key "ws:samurai"
func TestPaletteNoMatches(t *testing.T)            // "zzz" -> empty slice
func TestRecentKeepsTenDistinct(t *testing.T)       // add 12 prompts and one repeat -> 10, repeat at the front, no duplicates
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Palette|NeedsSession|PluginName|SkillsRecent|SessionsKeep|RunWorkspace|Recent'` → FAIL.

- [ ] **Step 3: Implement** in `palette_model.go`; rank with a stable sort inside each section.

- [ ] **Step 4: Run** the same → PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/ui/palette_model.go internal/ui/palette_model_test.go internal/ui/sidebar_model.go
git commit -m "feat(ui): Model the palette's sections, matching and order"
```

### Task 5: Starting a prompt (spec 9.3)

**Files:**
- Modify: `internal/workspaces/line.go`
- Create: `internal/ui/prompt_start.go`
- Modify: `internal/ui/workspace_run.go` (`takeOver`), `internal/ui/window.go` (`frame`: the launch wake)
- Test: `internal/workspaces/line_test.go`, `internal/ui/prompt_start_test.go`

**Interfaces:**
- Consumes: `agentOf`, `markOf` (6b), `locOf`, `selectSession`, `promptSeen`, `confirmAfter`, `takeOver`, `snapToBottom`, `note`, `recentPrompts` (Task 4).
- Produces:

```go
func workspaces.QuoteArg(k ShellKind, s string) (string, error) // Decision 9; error for ShellUnknown or a control character

func cleanPrompt(s string) string // Decision 7
// typeHere reports whether the prompt is typed into cur (true) or a new session opens (false).
func typeHere(cur *tabs.Tab, isSession, settingsFront bool, a agentState, m mark, shift bool) bool
func (win *window) startPrompt(prompt string, shift bool)
func (win *window) typeInto(t *tabs.Tab, text string)

type pendingLaunch struct {
	tab  tabs.TabID
	line string // without the trailing "\r"
	due  time.Time
}
func (win *window) launchTick(now time.Time) (wake time.Time, ok bool) // window field: launches []pendingLaunch
```

Rules (spec 9.3, Decisions 7, 8, 11, 12): `typeHere` is true only when cur is a session, settings are not in front, shift is false, `a.claude` is true and `m != markNeeds`. Typing writes `cleanPrompt(p) + " "`, sets the notice `Typed /x into <title>. Press Enter to send.` and gives the terminal the keyboard. Otherwise: quote first (on error the notice `Can't start /x in this shell (<kind>): only PowerShell, zsh, bash and sh are supported.` and nothing opens); then `mgr.NewOpts(tabs.StartOpts{Dir: <current cwd>})`, `selectSession`, append a pending launch due `now + confirmAfter`, notice `Opened a new session for /x.` `startPrompt` adds the prompt to the recent list either way it starts. `launchTick` runs each frame next to `wsTick`; `takeOver(t)` drops t's pending launch.

- [ ] **Step 1: Write the failing tests** for `QuoteArg` (`line_test.go`):

```go
func TestQuoteArg(t *testing.T)
// POSIX "it's $HOME `x`; y" -> `'it'\''s $HOME `+"`x`"+`; y'`
// PowerShell "it's" -> `'it''s'`; "it’s" -> `'it’’s'`; "日本語" -> `'日本語'`
// ShellUnknown -> error; "a\nb" -> error for both kinds
```

- [ ] **Step 2: Run** `go test ./internal/workspaces/ -run QuoteArg` → FAIL.

- [ ] **Step 3: Implement `QuoteArg`.** Run → PASS.

- [ ] **Step 4: Write the failing tests** (`prompt_start_test.go`, `testWindow` with `recStarter`, the shell set to `pwsh` unless stated, agent state set through 6b's `noteAgent`, marks through `deliver`):

```go
func TestTypeHereRule(t *testing.T) // table: claude idle/done/running -> true; needs -> false; plain shell -> false; shift -> false; member -> false; settings in front -> false; nil cur -> false
func TestTypingIntoClaudeHasNoEnter(t *testing.T)   // claude session: startPrompt("/ship-ticket", false) -> PTY got exactly "/ship-ticket ", no "\r"; notice names the title
func TestTypedPromptIsCleaned(t *testing.T)          // "/x\r\n\x1b[2J" -> control characters dropped: PTY got exactly "/x[2J " (no \r, \n or \x1b)
func TestNeedsOpensANewSession(t *testing.T)         // needs mark -> new tab in the current cwd, nothing written to the old tab
func TestNewSessionTypesClaudeWhenReady(t *testing.T) // new tab writes nothing until its PTY emits an OSC 7 report; then exactly "claude '/v3-lap'\r"
func TestNewSessionAsksAfterTheDeadline(t *testing.T) // no prompt mark; at due nothing is written and the waiting banner shows; Run commands writes the line, Use shell drops it; one InvalidateCmd at due
func TestPendingLaunchDroppedOnClose(t *testing.T)   // close the new tab before ready -> launches empty, no write to any tab
func TestOwnerTypingCancelsLaunch(t *testing.T)       // owner key into the new tab first -> launch dropped
func TestUnknownShellOpensNothing(t *testing.T)       // shell cmd.exe, plain shell current -> no new tab, error notice
func TestRuleDecidedWhenActing(t *testing.T)          // claude idle when the palette opens, needs arrives, then Enter -> new session, old tab untouched
```

- [ ] **Step 5: Run** `go test ./internal/ui/ -run 'TypeHere|Typing|TypedPrompt|NeedsOpens|NewSession|PendingLaunch|OwnerTyping|UnknownShell|RuleDecided'` → FAIL.

- [ ] **Step 6: Implement** `prompt_start.go`, the `takeOver` line and the frame wake.

- [ ] **Step 7: Run** `go test ./internal/workspaces/ ./internal/ui/` → PASS (baseline aside).

- [ ] **Step 8: Commit**

```bash
git add internal/workspaces/line.go internal/workspaces/line_test.go internal/ui
git commit -m "feat(ui): Start a prompt in the current Claude session or a new one"
```

### Task 6: The palette in the window

**Files:**
- Create: `internal/ui/palette.go` (drawing), `internal/ui/palette_window.go` (state, events, acting)
- Modify: `internal/ui/window.go` (`frame`, `focusTarget`), `internal/ui/logic.go` (`runShortcut`)
- Test: `internal/ui/palette_test.go`, `internal/ui/palette_window_test.go`

**Interfaces:**
- Consumes: `buildPalette`, `palItem`, `sessionRows`, `recentPrompts` (Task 4); `startPrompt` (Task 5); `skills.Cache`, `skills.Sources` (Tasks 1-2); `gitinfo.Top`; `kazeCommands`, `win.binds`, `formatShortcut` (Task 3); `workspaces.Load`, `runWorkspace`.
- Produces:

```go
type paletteUI struct {
	open           bool
	ed             widget.Editor // the query; SingleLine
	sel            string        // selected item's Key; "" is the first item
	scroll, hover  int           // hover: item index under the pointer, -1 none
	hits           palHits
	tag, veil      int           // list pointer events; outside clicks
	filters        []event.Filter
	skills         []skills.Skill // last loaded list, kept across opens
	scanning       bool
	loaded         chan []skills.Skill // buffered 1; the scan goroutine sends, the frame drains
	recent         recentPrompts
	spaces         []workspaces.Workspace // read on open
}
type palState struct {
	items         []palItem
	sel, scroll, hover int
	ed            *widget.Editor
}
type palHits struct {
	box, field, list image.Rectangle
	rows             []palRowHit
	scrollMax        int
}
type palRowHit struct {
	index int
	rect  image.Rectangle
}
func (c *chrome) layoutPalette(gtx layout.Context, st palState, main image.Rectangle) palHits
func (win *window) openPalette()
func (win *window) closePalette()
func (win *window) togglePalette()
func (win *window) paletteEvents(gtx layout.Context)
func (win *window) drawPalette(gtx layout.Context, main image.Rectangle)
func (win *window) actPalette(gtx layout.Context, it palItem, shift bool)
func (win *window) skillSources() skills.Sources // ClaudeDir = filepath.Dir(win.claudePath) ("" when claudePath is ""); Project = gitinfo.Top(current cwd)
```

Window fields: `win.pal paletteUI`, `win.skillCache *skills.Cache`, `win.skillsFn func(skills.Sources) []skills.Skill` (nil uses the cache; tests fake it).

Behavior (spec 10.1, mockup):
- `openPalette`: `dropSideEdit()`, clear the query, select the first item, read the saved workspaces, start one skill scan if none is in flight (goroutine: `skillsFn(src)`, send on `loaded`, `Invalidate`), give the field the keyboard. It shows the last loaded skills at once; a finished scan replaces them. `togglePalette` closes an open palette.
- Items are rebuilt from live state every frame while open, so states stay current; the selection follows its Key, else the first item.
- Keys on the field: Up / Down move (clamped, scrolled into view), Enter acts, Shift+Enter acts with shift, Esc closes. kaze's Ctrl / Cmd shortcuts: copy and paste work in the field (as `filterKey` does), the palette key closes, any other closes the palette and then runs.
- Pointer: a click on a row acts (Shift held means shift); a click anywhere outside the box closes and does nothing else; hover moves the selection, redrawing only when the hovered index changes; the wheel scrolls the list.
- Acting closes the palette first. A Waiting or Sessions item: `selectSession`. A member: what clicking its sidebar line does (`selectTab`; 6d: open floating). A skill: `startPrompt(it.Prompt, shift)`. A kaze item: `runShortcut(gtx, it.Act)`, or `runWorkspace(it.Workspace)`.
- `focusTarget` returns `&win.pal.ed` first while open (merge note).

Look (mockup `.pal`, Claude's calls within it, 100% scale): a box centered on the main view, `palW = 560` dp wide (at most 92% of the main view), `palTop = 60` dp below its top; the main view behind dimmed with `tint(th.Chrome, 0x88)`; box fill `th.ToastBG`, border `th.ToastLine`, radius 10 dp. Query field 13.5 sp with placeholder `Type to search waiting items, sessions and skills…` in `th.Muted`, a 1 px `th.Border` line under it. List at most `palListMaxH = 380` dp, scrolling; section headers `WAITING ON YOU`, `SESSIONS`, `SKILLS`, `KAZE` in 10.5 sp `th.Muted`; rows `palRowH = 28` dp: a mark (amber `th.Needs` dot for waiting; the sidebar's state mark for sessions; the member color for members), the name in `th.ToastFG` (skills in `c.memberColor("magenta")`), ` · /command` dim, the detail right-aligned in `th.Muted`, cut to 60% of the row with `c.fit`. Selected row fill `th.Active`. With no items, one `NO MATCHES` header line. Footer 11 sp `th.Muted`: `↑↓ move`, `Enter run in this session`, `Shift+Enter new session`, `Esc close`. Only rows inside the list's visible part are laid out.

- [ ] **Step 1: Write the failing drawing tests** (`palette_test.go`, the headless frame helpers):

```go
func TestPaletteBoxIsCentered(t *testing.T)        // main 1200x800 -> box 560 dp wide, centered on main, top at main.Min.Y + 60 dp
func TestPaletteNarrowMainView(t *testing.T)       // main 300 px wide -> box within main, no panic
func TestPaletteRowsAndHeaders(t *testing.T)       // 2 sessions, 3 skills -> 5 row hits; texts include "SESSIONS", "SKILLS"
func TestPaletteListScrolls(t *testing.T)          // 200 skills -> scrollMax > 0; list height <= 380 dp; rows laid out only inside list
func TestPaletteLongTextStaysInside(t *testing.T)  // 300-rune description and CJK name -> every row hit inside the box
func TestPaletteNoMatchesLine(t *testing.T)
func TestPaletteDrawsInEveryTheme(t *testing.T)    // five themes: no panic; selected row fill differs from the box fill
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'PaletteBox|PaletteNarrow|PaletteRows|PaletteList|PaletteLong|PaletteNoMatches|PaletteDraws'` → FAIL.

- [ ] **Step 3: Implement `layoutPalette`** in `palette.go`, following `layoutSidebar`'s use of `c.label`, `place`, `paint.FillShape` and clipping.

- [ ] **Step 4: Run** the same → PASS.

- [ ] **Step 5: Write the failing window tests** (`palette_window_test.go`; `win.claudePath` points into `t.TempDir()` or `win.skillsFn` is a fake; a test helper `waitSkills(t, win)` receives from `win.pal.loaded` with a 2 s timeout and puts the list in place):

```go
func TestPaletteKeyOpensAndCloses(t *testing.T)        // Ctrl+Shift+K -> open, field focused; again -> closed, terminal focused
func TestPaletteListsSkillsFromTheTree(t *testing.T)    // fake ~/.claude in TempDir with skills/review-mine -> after waitSkills, a "/review-mine" item
func TestSkillsScannedOnlyOnOpen(t *testing.T)          // counting skillsFn: 10 idle frames -> 0 calls; open -> 1; reopen while scanning -> still 1
func TestPaletteIdleCostsNothing(t *testing.T)          // open, settle; 5 frames with no input -> no InvalidateCmd and no extra redraws counted
func TestPaletteHoverRedrawsOnChange(t *testing.T)      // 5 Moves over one row -> 1 redraw; next row -> 2
func TestArrowsAndEnterActOnASession(t *testing.T)      // Down to session b, Enter -> b shown, palette closed
func TestEnterOnASkillTypesIt(t *testing.T)             // claude session, Enter on "/ship-ticket" -> PTY got "/ship-ticket ", recent has it
func TestShiftEnterOpensANewSession(t *testing.T)
func TestClickOutsideCloses(t *testing.T)               // press on the sidebar area -> closed, the sidebar row was not selected
func TestEscClosesAndReturnsKeys(t *testing.T)          // Esc -> closed; next key reaches the PTY
func TestKazeActionRuns(t *testing.T)                   // query "new session", Enter -> one more tab
func TestRunWorkspaceFromThePalette(t *testing.T)       // workspaces.toml in TempDir with "samurai" -> item present; Enter -> runWorkspace called (fake probes)
func TestOtherShortcutClosesThenRuns(t *testing.T)      // Ctrl+Shift+T while open -> palette closed, new session
func TestQueryFiltersLive(t *testing.T)                 // type "rev" -> only matching items; selection back on the first
func TestSelectionFollowsItsItem(t *testing.T)          // select session c; a needs message moves a above it -> still c selected
```

- [ ] **Step 6: Run** `go test ./internal/ui/ -run 'PaletteKey|PaletteLists|SkillsScanned|PaletteIdle|PaletteHover|ArrowsAndEnter|EnterOnASkill|ShiftEnter|ClickOutside|EscCloses|KazeAction|RunWorkspaceFrom|OtherShortcut|QueryFilters|SelectionFollows'` → FAIL.

- [ ] **Step 7: Implement** `palette_window.go`: state, `paletteEvents` (called in `frame` before `sideEvents`, so the veil wins), `drawPalette` (after the main view and banner, before toasts), the scan goroutine, `actPalette`, `focusTarget`, `togglePalette`.

- [ ] **Step 8: Run** `go test ./internal/ui/` → PASS (baseline aside).

- [ ] **Step 9: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): Open a palette of waiting items, sessions, skills and kaze actions"
```

### Task 7: The search box (Windows) and the palette button (macOS)

**Files:**
- Modify: `internal/ui/titlebar.go` (`barLayout`, `layoutBar`, `captionAt`), `internal/ui/window.go` (`drawBar`, `frame` drag areas, `captionEvent`), `internal/ui/sidebar.go` (top row, `layoutRail`), `internal/ui/sidebar_window.go` (`sidePress`)
- Test: `internal/ui/titlebar_test.go`, `internal/ui/sidebar_window_test.go`

**Interfaces:**
- Produces: `barLayout.Search image.Rectangle` and `barLayout.DragR image.Rectangle` (the move area right of the box; `Drag` becomes the part left of it); `capSearch captionButton`; `barDims.searchMax, searchMin int`; `sideHits.palette image.Rectangle`; `func searchGlyph(gtx layout.Context, r image.Rectangle) clip.Op` (a magnifier drawn from a circle stroke and a handle).

Layout (spec 5.1, mockup `.search`): Windows bar left to right: toggle, K mark, drag, **search box**, drag, gear, caption buttons. The box is centered in the space between the mark and the gear, at most 460 dp and at least 160 dp wide, 24 dp high, radius 6 dp, border `th.Border` (`th.Muted` on hover); inside, the glyph and `Search sessions, skills…` in `th.Muted`, the palette key (`formatShortcut`) right-aligned. When that space is under 200 dp the box is left out and `Drag` takes it all. The box and the drag areas never overlap, so Windows' move hit test never swallows a click on the box. A click (press and release on it, like the caption buttons) opens the palette.
macOS: the sidebar's top row gets the palette button between the toggle and the gear; the rail shows toggle, palette, gear top to bottom (6d's scratch button, if present, after the palette). Merge note: `sidePress` gains `case p.In(h.palette): win.openPalette()`.

- [ ] **Step 1: Write the failing tests**

```go
func TestBarHasASearchBoxBetweenTheDragAreas(t *testing.T) // 1200 px bar: Search centered between Mark and Gear, <= 460 dp; Drag, DragR, Search pairwise disjoint; Drag ∪ Search ∪ DragR spans Mark.Max.X..Gear.Min.X
func TestBarDropsSearchWhenNarrow(t *testing.T)            // too little room -> Search empty, DragR empty, Drag as before
func TestCaptionAtSearch(t *testing.T)                     // captionAt(center of Search) == capSearch
func TestClickingSearchOpensThePalette(t *testing.T)       // press + release on win.bar.Search -> pal.open
func TestMacSidebarPaletteButton(t *testing.T)             // custom=false: hits.palette between toggle and gear; click -> pal.open
func TestMacRailPaletteButton(t *testing.T)                // sidebar hidden: rail hits.palette below toggle; click -> pal.open
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'SearchBox|DropsSearch|CaptionAtSearch|ClickingSearch|MacSidebarPalette|MacRailPalette'` → FAIL.

- [ ] **Step 3: Implement**; `frame` pushes `system.ActionInputOp(system.ActionMove)` for both `Drag` and `DragR`, and the capTag area for `Search` with the other buttons.

- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline aside).

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(ui): Open the palette from the title bar or the sidebar's top row"
```

### Task 8: Settings: the Commands section

**Files:**
- Create: `internal/ui/settings_commands.go`
- Modify: `internal/ui/settings.go` (`defaultSections`, the "Later" line in `nav`)
- Test: `internal/ui/settings_commands_test.go`

**Interfaces:**
- Consumes: `kazeCommands`, `win.binds`, `formatShortcut` (Task 3).
- Produces: `func commandsSection() settingsSection` (title `Commands`, hint `Read-only for now. Change a shortcut under [keys] in config.toml.`), `func drawCommands(p *page, r image.Rectangle, win *window) int`, `func commandsAct(win *window, id string, arg int)`, `var commandLines []cmdLine` with `type cmdLine struct{ Example, Copy, Desc string }` (`Copy` "" means no button).

Content (spec 10.4, mockup `drawSettings` `commands`), drawn like About with `p.text` and `p.ctrl`:
1. `SHORTCUTS`: one table line per `kazeCommands` entry: label, the key on this machine (`formatShortcut(runtime.GOOS, win.binds[Config])`, overrides included), the config name dim. Then `Select a word / a line` with `double / triple click`, and `Select in mouse apps` with `Shift + drag`.
2. `COMMAND LINE`: `kaze --workspace <name>` (Start kaze with a workspace; copies `kaze --workspace "<name>"`), `kaze --version` (Print the version and build; copies itself), `kaze notify done / needs / prompt` and `kaze statusline` (Used by Claude Code hooks; you don't run these yourself; no Copy button). The `kaze status` lines wait for 6c.
- Copy works as `drawAbout` does: `commandsAct` stores the text, the next draw writes it through `win.toClipboard` (tests) or `clipboard.WriteCmd`, and notes `Copied: <text>`.
- Order: Appearance, Workspaces, Claude, Commands, About. The nav's "Later" line drops "Shortcuts": `Later: Terminal, Shell setup`.

- [ ] **Step 1: Write the failing tests** (`settings_commands_test.go`, reading `win.set.texts`):

```go
func TestCommandsAfterClaude(t *testing.T)          // section titles == [Appearance Workspaces Claude Commands About]
func TestCommandsListsEveryShortcut(t *testing.T)   // each kazeCommands Label and Config appears; "Ctrl+Shift+K" on windows bindings
func TestCommandsShowsOverrides(t *testing.T)       // config keys {"palette":"ctrl+shift+p"} -> "Ctrl+Shift+P", not "Ctrl+Shift+K"
func TestCommandsCopy(t *testing.T)                 // activate the --workspace Copy -> toClipboard got `kaze --workspace "<name>"`
func TestHookCommandsHaveNoCopy(t *testing.T)       // no control on the notify and statusline lines
func TestLaterLineDropsShortcuts(t *testing.T)
func TestCommandsDrawsInEveryTheme(t *testing.T)
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Commands|LaterLine|HookCommands'` → FAIL.

- [ ] **Step 3: Implement.**

- [ ] **Step 4: Run** `go test ./internal/ui/` → PASS (baseline aside).

- [ ] **Step 5: Commit**

```bash
git add internal/ui
git commit -m "feat(settings): List kaze's shortcuts and command line in a Commands section"
```

### Task 9: Docs and version

**Files:**
- Modify: `README.md`, `docs/HANDOFF.md`, `internal/version/version.go`, `internal/version/version_test.go`

- [ ] **Step 1:** version test expects one minor above the base branch's `version.Number` (0.9 when 6b's 0.8 is the base) → FAIL; set `Number` → PASS.
- [ ] **Step 2:** README: the palette (key, search box / macOS button, sections, what Enter and Shift+Enter do, typing without Enter vs a new session, where skills come from, that kaze only reads them), `palette` in the shortcuts list and `[keys]` table, the Commands section. HANDOFF: what 6e shipped, the 6b dependency, 6c still parked (so no chips in the palette), the go-winres rerun for the new version.
- [ ] **Step 3:** `go test ./...` → only the known baseline failures; `GOOS=windows go vet ./...` → no output.
- [ ] **Step 4: Commit**

```bash
git add README.md docs/HANDOFF.md internal/version
git commit -m "docs: Describe the palette and the Commands section and bump kaze's version"
```

---

## Self-review

1. **Spec coverage.** 9.3: Task 5 (type without Enter, needs / not Claude / Shift opens a new session, readiness, quoting, notice). 10.1: Tasks 4 and 6 (sections, contains-anywhere matching, a leading `/` ignored, recent first, one-line descriptions, ↑ ↓ Enter Shift+Enter Esc and click outside, acting). 10.2: Tasks 1 and 2 (user, project, enabled plugins at the newest version, `plugin:name`, frontmatter with fallbacks, skip bad files, read only, cached by modification time, recent 10 in memory). 10.3: Tasks 3 and 4 (kaze section from `kazeCommands`, one "Run workspace" per saved workspace). 10.4: Task 8. 11: Task 3 (`palette`, Ctrl+Shift+K / Cmd+K). 5.1's search box and macOS buttons: Task 7. Chips (9.1, 9.2, 10.1's first item) and `kaze status` in 10.4: left out on purpose (6c parked).
2. **Step scan.** Each code step names its file, signature and the values it must use; behavior is pinned by named tests with their assertions. Bodies are left to the implementer except the decision rules above.
3. **Type consistency.** `palItem.Key` is the selection's identity in Tasks 4 and 6; `startPrompt(prompt string, shift bool)` in Tasks 5 and 6; `win.binds` and `formatShortcut(goos, sc)` in Tasks 3, 6, 7 and 8; `skills.Sources{ClaudeDir, Project}` in Tasks 1, 2 and 6; `typeHere` takes 6b's `agentState` and `mark`.
4. **Review Focus.** Each line has its test: hostile trees (`TestLoadSkipsBadFiles`, `TestLoadIsBoundedInDepthAndCount`, `TestSkillsScannedOnlyOnOpen`), state change before acting (`TestRuleDecidedWhenActing`), quoting (`TestQuoteArg`, `TestUnknownShellOpensNothing`), real plugin layouts (`TestPluginRootScansTheCache`, `TestLoadSkipsDisabledAndMissingPlugins`), a launch whose tab goes away (`TestPendingLaunchDroppedOnClose`).
5. **Proportion.** Signatures, rules and test names only; no function bodies.
