# Markdown Reader Implementation Plan

> **For agentic workers:** this plan is written for Codex (or any agent without Claude skills). Do the tasks in order, one commit each, ticking the `- [ ]` boxes as you go. Every task is strict TDD: write the test, run it and watch it fail (RED), then make it pass (GREEN). (A Claude agent may use superpowers:executing-plans.)

**Goal:** Cmd+click (Ctrl+click on Windows) an `.md` path in terminal output and read the rendered doc inside kaze, as a throwaway sidebar row nested under the session it came from.

**Architecture:** `internal/render` learns to find `.md` paths next to web links, with the same allocation-free scan, so the renderer draws them blue and underlined for free. A new Gio-free package `internal/mdoc` resolves a clicked path to a file, loads it (capped at 2 MB) and parses it with goldmark into a flat block model with an outline. The UI keeps a window-level list of open docs (`win.docs`), never a `tabs.Tab`; a shown doc covers the terminal the way Settings and the agents board do (`shownTab()` returns nil), and the sidebar draws each doc as a nested line under its session.

**Tech Stack:** Go 1.27, Gio v0.10.3, `github.com/yuin/goldmark` v1 (new; approved by the spec).

**Spec:** `docs/superpowers/specs/2026-11-01-kaze-markdown-reader-design.md`. The spec is the authority for behaviour; read it before Task 1.

## Global Constraints

- Only `.md` (any case). Paths to other files are never detected in the terminal.
- Read-only: kaze never writes, renames or deletes the file. Closing a doc row, its session or kaze never touches the file.
- The OS open function (`openURL`, reached only through `win.open`) only ever receives `http`/`https` URLs. Rendering a `.md` file never goes through the OS.
- Path detection: no per-frame allocation, no file system access while drawing, no extra frames from hovering, bounded by `maxWrapRows` (16).
- A doc is not a `tabs.Tab`: it stays out of the status bar tab count, Ctrl+Tab, Next waiting, the palette's session list, alerts and session restore.
- Doc rows are not saved in `sessions.json` and never restored.
- Missing file notice, exact text: `Can't find <path>` (the path as clicked).
- Read cap: 2 MB (`2 << 20` bytes). A cut file says so in the view.
- Reload: modification time (and size) check when the doc is shown, on window focus, and on a timer of about once a second only while a doc is the shown view. Keep the scroll position across a reload. A deleted file keeps its last content and the view says the file was removed.
- Title bar breadcrumb: `<repo or folder name> / <file name>`.
- Theme tokens only (`win.ui.th`, `win.pal`); readable in all five themes, Washi included.
- RED tests fail safely: bounded input sizes, deadlines on every wait, no unbounded loops.
- Never inject global keyboard or mouse input. Test through `pointerView`/`onPointer`, `sidePress`, `docKey`, `input.Router` frames.
- No test reads or writes the owner's real `~/.claude`, `config.toml`, `sessions.json` or `workspaces.toml`. Files go in `t.TempDir()`.
- Gates after every task: `go test ./...` and `go vet ./...` (when chasing a failure, one package at a time: `go test ./internal/ui/ -run TestName -v`).
- Commits: `type(scope): Verb ...`. **No AI attribution of any kind** (no `Co-Authored-By`, no "Generated with").
- Branch `feat/markdown-reader`. Never push to `main`. No release steps, no version bump, no changelog row: a release hold is in force (`CLAUDE.md`).

## Decisions this plan makes (where the spec left it to the builder)

| Topic | Decision |
|---|---|
| Doc model | Window-level `docsState` with `[]*docView`, each with its parent `tabs.TabID`; `win.docs.shown` is the shown-view switch, like `win.set.shown`. |
| Parser | goldmark with `extension.GFM` (tables, task lists, strikethrough, linkify). Linkify means a bare URL in a doc is a link, as in the terminal. |
| Path characters | ASCII letters and digits, Latin/Greek/Cyrillic letters, and `. _ - / \ ~ + @`. A drive letter `X:` is allowed only at the start before `\` or `/`. No spaces, parens, commas, `#`, `%`, quotes, backticks, `*` or CJK, so the text around a path never joins it. A URL always wins: `https://x/README.md` is a web link. A trailing `:<line>` or `:<line>:<col>` is skipped and not underlined. Trailing `.` is trimmed. |
| Member shown as the main view | The spec's "active session" there is the member itself, which is not a session row, so the doc nests under the first session in sidebar order (`sessionIDs()[0]`). With no session at all nothing opens and the status bar says `Open a session to read <name>`. |
| Close key | kaze's existing `close_tab` binding closes the shown doc row: Cmd+W on macOS, Ctrl+Shift+W on Windows (kaze has no plain Ctrl+W). |
| Row placement | Doc lines sit directly under their session row, above its subagent lines, so they don't jump when subagents come and go. Open order. |
| Doc line text | File name; under it the folder relative to the repo top (`docs/superpowers/specs`), else `~`-shortened. |
| Dedupe | Same session and same `mdoc.PathKey` (absolute, cleaned, case-folded on macOS and Windows). |
| In-doc doc links | Resolved like terminal paths, from the doc's own folder, with the same repo-root fallback. |
| Code blocks | Wrapped at the column width (no horizontal scroll), JetBrains Mono. |
| Tables | Columns shrink in proportion to their natural width (minimum 48 dp) and cells wrap. A table that can't fit even at the minimum is clipped at the column edge. |
| Italic | No italic face is bundled; italic runs request `font.Italic` at Medium weight (bold is SemiBold), so they still read differently. Adding an italic font is out of scope. |
| Task list boxes | Drawn as shapes, not glyphs (the fonts may lack U+2610). |
| Nesting | List and quote depth capped at 8 for layout. |
| "Updated" time | The doc header shows `Loaded 2:41 PM` (12-hour) next to the path, refreshed on reload; nothing ticks. The status bar is unchanged. |
| Reload I/O | `os.Stat` at most once a second on the UI goroutine before drawing, never inside `drawDoc`; a changed file is re-read and re-parsed synchronously (local files, capped at 2 MB). The timer runs only while a doc is shown. |
| Outline | Every heading level, indented 12 dp per level below the shallowest, capped at 3 steps. Hidden when the main area is narrower than 600 dp. |
| Subagent pane | Cmd+click on an `.md` path or web link in a live pane's prompt or report opens it, under the pane's session. No blue styling or hand cursor there; a path split across a wrapped line is not found. |
| Not in v1 | Docs in the palette, text selection and copy, search, Esc to leave the doc. |

## Review Focus

1. **Hostile or huge docs** (2 MB on one line with no spaces, 5,000 nested quotes or lists): parse and layout finish in bounded time, nothing recurses without limit, the window stays responsive. Tests: Task 6 `TestParseDeepNestingStaysBounded`, Task 7 `TestDocLayoutOfAHugeLineIsBounded`.
2. **Agent output with no spaces around the path** (CJK prose, `⏺`/`⎿` marks, backticks, `**bold**`, a box frame, a path wrapped onto the next row that starts with `\`): only the path is found. Tests: Task 1 table and `TestLinkAtJoinsAWrappedWindowsPath`.
3. **A session closed by something other than its row's ×** (workspace stop, `mgr.Close`, a scratch parent going away) while its doc is shown: the doc row goes, a terminal is shown, nothing dereferences a gone tab. Test: Task 3 `TestPruneDocsDropsDocsOfGoneSessions`.
4. **An editor saving by replace-and-rename** (the file briefly missing, the new mtime equal to the old within the same second): the view never sticks on "removed" once the file is back, and a same-second edit of a different size still reloads. Tests: Task 10.
5. **A theme switch while a doc is shown**: colours follow the new theme on the next frame without a relayout, and links stay readable on Washi. Test: Task 7 `TestDocColorsFollowTheThemeWithoutRelayout`.

---

### Task 0: Branch and commit the design

**Files:**
- Add (already in the working tree, untracked): `docs/superpowers/specs/2026-11-01-kaze-markdown-reader-design.md`, `docs/superpowers/plans/2026-11-01-kaze-markdown-reader.md`

- [x] **Step 1:** `git fetch origin && git switch -c feat/markdown-reader origin/main`. Untracked files come along. (If the `perf/audit-followup` PR has merged, `origin/main` has it; if not, that is fine: every function this plan names exists on both.)
- [x] **Step 2:** `git add docs/superpowers/specs/2026-11-01-kaze-markdown-reader-design.md docs/superpowers/plans/2026-11-01-kaze-markdown-reader.md` (only these two; leave every other untracked file alone).
- [x] **Step 3:** `go test ./... && go vet ./...` Expected: PASS (baseline).
- [x] **Step 4:** Commit `docs(reader): Add the markdown reader design and plan`

### Task 1: Find `.md` paths next to web links

**Files:**
- Modify: `internal/render/links.go`
- Test: `internal/render/links_test.go`

**Interfaces:**
- Produces:
  - `func IsWebLink(s string) bool`: true when `s` starts with `http://` or `https://`, any case. No allocation.
  - `appendURLs` is renamed `appendLinks(out [][2]int, rs []rune) [][2]int` and finds web links and `.md` paths in one left-to-right pass, sorted and never overlapping; `findURLs` becomes `findLinks`. `Links.At` and `Links.Visible` call `appendLinks`, so both kinds are returned (and the renderer's `queueLinkRows`/`styleLinks` style paths blue and underlined with no change there).
  - `Links.At` returns the path text without its `:<line>` suffix (`docs/a.md` for `docs/a.md:12`).
  - `continues` joins the next row when its first rune is `urlRune(r) || r == '\\'`.
  - Unexported helpers: `pathRune(r rune) bool`, `pathAt(rs []rune, i int) (end, next int)`.

- [x] **Step 1: Write the failing tests**

Rename the call in `TestFindURLs` to `findLinks` (its cases contain no `.md`, so it must stay green). Add:

```go
func TestFindDocPaths(t *testing.T) {
	cases := []struct{ in, want string }{
		{"I've written it to `docs/superpowers/specs/x-design.md`.", "docs/superpowers/specs/x-design.md"},
		{"⏺ Wrote 120 lines to docs/plan.md", "docs/plan.md"},
		{"  ⎿  Read README.md (42 lines)", "README.md"},
		{"│ See ./notes/todo.md for details │", "./notes/todo.md"},
		{"• Edited docs/HANDOFF.md (+12 -3)", "docs/HANDOFF.md"},
		{"Updated README.md:42 and docs/a.md:12:3.", "README.md docs/a.md"},
		{"open ~/notes/today.md", "~/notes/today.md"},
		{"/Users/pat/kaze/docs/x.MD", "/Users/pat/kaze/docs/x.MD"},
		{`C:\Users\pat\kaze\docs\x.md`, `C:\Users\pat\kaze\docs\x.md`},
		{`.\docs\x.md and C:/a/b.md`, `.\docs\x.md C:/a/b.md`},
		{"**docs/bold.md** (see docs/paren.md)", "docs/bold.md docs/paren.md"},
		{"保存先docs/a.md", "docs/a.md"},
		{"main.go x.mdx x.md.bak README.markdown .md a/.md", ""},
		{"https://github.com/a/b/blob/main/README.md", "https://github.com/a/b/blob/main/README.md"},
		{"see docs/a.md, docs/b.md; done", "docs/a.md docs/b.md"},
	}
	for _, c := range cases {
		var got []string
		rs := []rune(c.in)
		for _, sp := range findLinks(rs) {
			got = append(got, string(rs[sp[0]:sp[1]]))
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%q: got %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsWebLink(t *testing.T) // true: "https://a.io", "HTTP://A.IO/x"; false: "docs/a.md", "file:///etc/passwd", "mailto:a@b.c", "javascript:x", "http:/x", ""
func TestLinkAtFindsADocPathWithoutItsLine(t *testing.T) // printAll "edit docs/a.md:12 now"; At col 7 → "docs/a.md"; At col 15 (on ":12") → not found
func TestLinkAtJoinsAWrappedWindowsPath(t *testing.T)    // screen 12 cols; printAll `x C:\code\kaze\docs\plan.md` so a row starts with '\'; both rows' points → the whole path
func TestVisibleUnderlinesDocPathsButNotTheLineSuffix(t *testing.T) // "a docs/x.md:9" → one span From 2 To 10
func TestVisibleWithDocPathsAllocatesNothing(t *testing.T) // 23 rows of "wrote docs/plan-%d.md and README.md:3"; warm once; testing.AllocsPerRun(20, ...) == 0
```

- [x] **Step 2: Run** `go test ./internal/render/ -run 'Doc|WebLink|FindURLs' -v`. Expected: FAIL (undefined `findLinks`, `IsWebLink`).

- [x] **Step 3: Implement** in `internal/render/links.go`. Keep `urlRune`, `schemeLen`, `trimURLEnd` as they are. The scan:

```go
// appendLinks appends the [start, end) rune ranges of the web links and the
// .md paths in rs, in order and never overlapping. A web link wins over a path.
func appendLinks(out [][2]int, rs []rune) [][2]int {
	for i := 0; i < len(rs); i++ {
		if j, ok := urlAt(rs, i); ok { // the old appendURLs body for one start
			out = append(out, [2]int{i, j})
			i = j - 1
			continue
		}
		if !pathRune(rs[i]) {
			continue
		}
		end, next := pathAt(rs, i)
		if end > i {
			out = append(out, [2]int{i, end})
		}
		i = next - 1
	}
	return out
}
```

`pathAt` extends `j` over `pathRune`s, also over one `:` when the run so far is a single ASCII letter and the next rune is `\` or `/` (a drive), and stops before position `k` where `!alnum(rs[k-1]) && schemeLen(rs[k:]) > 0` (a URL starts inside, as in `dir/https://x`). `next` is the end of the run. `end` trims trailing `.`; when nothing was trimmed, `next` also skips up to two `:<digits>` groups. Return `end = i` (none) unless `rs[i:end]` ends in `.md` (case-insensitive) with at least one rune before the `.` that is not `/`, `\` or `.`. `pathRune` is the character set in the Decisions table (use `unicode.In(r, unicode.Latin, unicode.Greek, unicode.Cyrillic)` for non-ASCII letters, `unicode.IsDigit` only for ASCII digits).

Update the doc comments: `LinkSpan` and `Links` describe "web links and .md paths".

- [x] **Step 4: Run** `go test ./internal/render/ -v` and `go test ./internal/render/ -run XXX -bench VisibleLinks -benchmem`. Expected: PASS, and the benchmark still reports `0 allocs/op`.
- [x] **Step 5: Commit** `feat(render): Find .md paths next to web links`

### Task 2: Resolve and load a doc (`internal/mdoc`)

**Files:**
- Create: `internal/mdoc/resolve.go`, `internal/mdoc/load.go`
- Test: `internal/mdoc/resolve_test.go`, `internal/mdoc/load_test.go`

**Interfaces:**
- Produces (package `mdoc`, Gio-free, imports only the standard library):
  - `var ErrNotFound = errors.New("not found")`
  - `type Roots struct { Dir, Home string; Top func(dir string) string }`: `Dir` is the folder a relative path is relative to; `Home` expands `~/` and `~\`; `Top` gives the repo top for the fallback (kaze passes `gitinfo.Top`; nil skips the fallback).
  - `func Resolve(ref string, r Roots) (string, error)`: `filepath.FromSlash`, `~` expansion, then the absolute path alone, or `Join(Dir, ref)` then `Join(Top(Dir), ref)` (only when `Top(Dir)` is non-empty and differs from `Dir`). A candidate counts only if `os.Stat` says it is a regular file and `IsMarkdown` is true. Returns the absolute, cleaned path, or `ErrNotFound` (also for an empty `Dir` with a relative ref).
  - `func IsMarkdown(path string) bool`: extension `.md`, any case.
  - `func PathKey(path, goos string) string`: `filepath.Clean`, lowercased when `goos` is `darwin` or `windows`.
  - `const MaxBytes = 2 << 20`
  - `type Stamp struct { Mod time.Time; Size int64 }`
  - `type File struct { Data []byte; Cut bool; Total int64; Stamp Stamp }`
  - `func Load(path string) (File, error)`: opens, refuses anything not a regular file (checked on the open handle), reads at most `MaxBytes+1` bytes through `io.LimitReader`; when more than `MaxBytes`, `Cut` is true and `Data` ends at the last `\n` within the first `MaxBytes` (or at `MaxBytes` when there is none). `Total` is the file size.
  - `func StatFile(path string) (Stamp, error)`

- [x] **Step 1: Write the failing tests** (every file under `t.TempDir()`):

```go
func TestResolveRelativeDotAndTilde(t *testing.T)   // dir/docs/a.md exists: "docs/a.md", "./docs/a.md", `docs\a.md` on Windows only; Home=dir: "~/docs/a.md" → the same absolute path
func TestResolveAbsolute(t *testing.T)              // the absolute path itself; Dir "" is fine for an absolute ref
func TestResolveFallsBackToTheRepoTop(t *testing.T) // Dir root/sub, file root/docs/a.md, Top returns root → found; Top nil → ErrNotFound
func TestResolveMissing(t *testing.T)               // errors.Is(err, ErrNotFound); relative ref with Dir "" → ErrNotFound
func TestResolveRefusesFoldersAndOtherFiles(t *testing.T) // a folder named x.md, and dir/a.txt asked as "a.txt" → ErrNotFound
func TestPathKeyFoldsCaseOnMacAndWindows(t *testing.T)    // PathKey("/A/./b.md","darwin") == PathKey("/a/B.md","darwin"); differ for "linux"
func TestLoadSmallFile(t *testing.T)               // Data equal, Cut false, Total == len, Stamp.Size == len
func TestLoadCapsAtTwoMegabytes(t *testing.T)      // write MaxBytes+4096 bytes of 100-byte lines: Cut, len(Data) <= MaxBytes, Data ends in '\n', Total == written
func TestLoadRefusesAFolder(t *testing.T)          // Load(dir) returns an error
func TestStatFileSeesAChange(t *testing.T)         // write, StatFile, rewrite with a different size and os.Chtimes +2s → Stamp differs
```

- [x] **Step 2: Run** `go test ./internal/mdoc/ -v`. Expected: FAIL (package has no Go files / undefined).
- [x] **Step 3: Implement** `resolve.go` and `load.go` per the Interfaces.
- [x] **Step 4: Run** `go test ./internal/mdoc/ -v && go vet ./internal/mdoc/`. Expected: PASS.
- [x] **Step 5: Commit** `feat(mdoc): Resolve and load markdown files`

### Task 3: Open docs as a shown view (model and routing)

A doc is shown the way the agents board is: the terminal is not drawn and gets no input, `shownTab()` is nil, the active session stays the doc's session. This task draws only a blank page in the theme's background; rendering comes in Task 7.

**Files:**
- Create: `internal/ui/docs.go`
- Modify: `internal/ui/window.go` (struct fields; `frame`: input branch, drawing branch, `pruneDocs`; `shownTab`; `blinks`; `focusTarget`; `runShortcut` `actCloseTab`; `updateTitle`; `routeView`), `internal/ui/alerts_ui.go` (`selectTab`), `internal/ui/settings.go` (`openSettings`), `internal/ui/board.go` (`openBoard`), `internal/ui/sidebar_window.go` (`closeTab`), `internal/ui/panes_input.go` (`paneKeyView`), `internal/ui/panes_agent.go` (`openAgentPane`), `internal/ui/prompt_start.go` (`startPrompt`), `internal/ui/titlebar_draw.go` (`crumb`)
- Test: `internal/ui/docs_test.go`

**Interfaces:**
- Consumes: `mdoc.Load`, `mdoc.PathKey`, `mdoc.File`, `gitinfo.Top`, `gitinfo.Repo`.
- Produces (in `internal/ui/docs.go`):
  - `type docID uint64`
  - `type docView struct { id docID; session tabs.TabID; path, key, name, folder, repo string; file mdoc.File; loadedAt time.Time; removed bool; scroll int; anchor string }`. `name` is `filepath.Base(path)`; `repo` is the repo name for the breadcrumb (`gitinfo.Repo` name, else `folderName` of the doc's folder); `folder` is the doc's folder relative to `topFn(dir)` (`.` becomes `""`), else `tildeHome(dir, win.home)`.
  - `type docsState struct { list []*docView; shown, last docID; tag int }` (later tasks add fields: `adv` and `texts` in Task 7, `hits` in Task 8, `checked` in Task 10)
  - window fields: `docs docsState`; `topFn func(string) string // gitinfo.Top unless a test replaces it`.
  - `func (win *window) openDoc(session tabs.TabID, path string) *docView`: an existing doc with the same `session` and `PathKey(path, runtime.GOOS)` is shown and returned; else `mdoc.Load` (on error: `win.note("Can't open " + filepath.Base(path) + ": " + err.Error())`, return nil), append, show, return.
  - `func (win *window) showDoc(id docID)`: `dropSideEdit`, `hideSettings`, `closeBoard`, `win.mgr.SetActive(d.session)`, `win.docs.shown = id`, `endGesture`, `wantFocus = true`, `side.reveal = true`, invalidate.
  - `func (win *window) hideDoc()`: shown = 0, `wantFocus = true`, invalidate (no-op when nothing shown).
  - `func (win *window) closeDoc(id docID)`, `func (win *window) closeSessionDocs(session tabs.TabID)`, `func (win *window) pruneDocs(list []*tabs.Tab)` (drops docs whose session is not in `list`; hides the shown one).
  - `func (win *window) shownDoc() *docView`, `func (win *window) docByID(id docID) *docView`, `func (win *window) sessionDocs(session tabs.TabID) []*docView` (open order).
  - `func (win *window) drawDoc(gtx layout.Context, d *docView, r image.Rectangle)`: for now fills `r` with `win.pal.BG` and registers `&win.docs.tag` over `r`.
  - `func (win *window) docEvents(gtx layout.Context)`: takes `key.FocusFilter`, `key.Filter{Focus: tag, Optional: allMods}` and pointer `Press|Scroll` on `&win.docs.tag`; a key press that is a kaze shortcut (`win.km[input.Shortcut{Name: e.Name, Mods: e.Modifiers}]`) runs `win.runShortcut`; `key.EditEvent`s are dropped with `SnippetCmd`/`SelectionCmd` like `settingsEvents`. Task 8 adds scrolling.
- Wiring:
  - `shownTab()`: nil while `win.docs.shown != 0` too.
  - `frame`: input `else if win.docs.shown != 0 { win.docEvents(gtx) }` after the board branch; after `list := win.mgr.Tabs()` call `win.pruneDocs(list)`; drawing `else if d := win.shownDoc(); d != nil { win.drawDoc(gtx, d, termRect); win.wsr.hits = win.wsr.hits[:0] }` after the board branch.
  - `blinks()`: also false while a doc is shown.
  - `focusTarget()`: `&win.docs.tag` while a doc is shown (after the settings and board checks).
  - `runShortcut` `actCloseTab`: after the board and settings branches, `else if d := win.shownDoc(); d != nil { win.closeDoc(d.id) }`.
  - `selectTab`, `openSettings`, `openBoard`, `newTab`: call `win.hideDoc()`.
  - `closeTab(id)`: `win.closeSessionDocs(id)` first.
  - `paneKeyView`: no pane has the keyboard while a doc is shown.
  - `openAgentPane`: treat a shown doc like `win.set.shown` (switch with `selectTab`).
  - `startPrompt`: pass `win.set.shown || win.docs.shown != 0` as `typeHere`'s `settingsFront`.
  - `crumb()`: `case win.docs.shown != 0: return crumbParts{Repo: d.repo, Title: d.name}` (drop `Repo` when equal to `Title`).
  - `updateTitle`: `kaze · <name>` while a doc is shown.
  - `routeView`: after a `pointer.Event`, `if win.docs.shown != 0 { win.endGesture(); return }` (the click showed a doc; the rest of the frame's input is not the terminal's).

- [x] **Step 1: Write the failing tests** in `internal/ui/docs_test.go`:

```go
// writeDoc writes body to dir/name and returns the path.
func writeDoc(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOpenDocShowsItUnderItsSession(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	a, _ := win.mgr.New()
	b, _ := win.mgr.New()
	win.mgr.SetActive(b.ID)
	before, _ := win.mgr.State()
	order := win.sessionIDs()
	p := writeDoc(t, t.TempDir(), "plan.md", "# Plan\n")
	d := win.openDoc(a.ID, p)
	if d == nil || win.shownDoc() != d || d.session != a.ID || win.mgr.Active().ID != a.ID {
		t.Fatalf("doc %+v shown %v active %d", d, win.shownDoc(), win.mgr.Active().ID)
	}
	if win.shownTab() != nil || win.focusTarget() != &win.docs.tag {
		t.Fatal("the terminal is still in front")
	}
	after, _ := win.mgr.State()
	if len(after) != len(before) || !slices.Equal(win.sessionIDs(), order) {
		t.Fatal("a doc became a tab or a session")
	}
}
```

And:

```go
func TestOpenDocTwiceSwitchesToTheSameRow(t *testing.T)  // openDoc(a, p) twice → 1 doc; filepath.Join(dir, ".", "plan.md") → same doc; openDoc(b, p) → a second doc under b
func TestOpenDocDedupesByCaseOnMacAndWindows(t *testing.T) // skip unless darwin/windows: "PLAN.md" spelling of the same file → 1 doc
func TestSelectingTheSessionHidesTheDoc(t *testing.T)    // selectTab(a) → shownDoc() nil, len(win.docs.list) == 1, shownTab() == a
func TestCloseTabAlsoClosesItsDocs(t *testing.T)         // docs under a and b; closeTab(a) → only b's doc left; the shown one was a's → none shown
func TestPruneDocsDropsDocsOfGoneSessions(t *testing.T)  // tabs a and b; doc of a shown; win.mgr.Close(a.ID) (not closeTab); win.pruneDocs(win.mgr.Tabs()) → list empty, shownDoc() nil, shownTab() == b
func TestNeedsAlertStaysWhileADocIsShown(t *testing.T)   // alertWindow(t, 1); doc of tab 0 shown; win.deliver(queued{m: notify.Msg{Tab: int(id), Kind: notify.KindNeeds}}, t0); win.visit(win.shownTab()) → markOf(id) == markNeeds
func TestCycleTabFromADocLandsOnATerminal(t *testing.T)  // sessions a, b; doc of a shown; cycleTab(1) → active b, no doc shown. One session only: cycleTab(1) → active a, no doc shown
func TestCloseShortcutClosesTheShownDoc(t *testing.T)    // runShortcut(layout.Context{}, actCloseTab) → doc gone, tab a still open and shown
func TestSettingsAndBoardHideTheDoc(t *testing.T)        // openSettings → shownDoc nil; showDoc again; openBoard(0) → shownDoc nil
func TestBlinkStopsWhileADocIsShown(t *testing.T)        // CursorBlink true, focused: blinks() true, then openDoc → false
func TestDocCrumbIsRepoAndName(t *testing.T)             // win.repoFn returns ("k", "kaze-terminal") → crumb() == crumbParts{Repo: "kaze-terminal", Title: "plan.md"}
func TestTerminalGetsNoKeysWhileADocIsShown(t *testing.T) // copy TestTerminalGetsNoKeysWhileSettingsShown (settings_test.go) with openDoc instead of openSettings: typed "a" never reaches the PTY
```

For the crumb test give the doc's repo from `win.repoFn` when it is set (status.go already treats `repoFn` as a test hook for `gitinfo.Repo`); use `win.topFn` the same way for `gitinfo.Top`.

- [x] **Step 2: Run** `go test ./internal/ui/ -run 'Doc' -v`. Expected: FAIL (undefined `openDoc`, `win.docs`).
- [x] **Step 3: Implement** `internal/ui/docs.go` and the wiring above.
- [x] **Step 4: Run** `go test ./internal/ui/ -v -run 'Doc|Settings|Board|Cycle|Close|Blink|Alert'` then `go test ./... && go vet ./...`. Expected: PASS.
- [x] **Step 5: Commit** `feat(ui): Show a markdown doc in place of the terminal`

### Task 4: Cmd+click a path in the terminal opens it

**Files:**
- Create: `internal/ui/docs_open.go`
- Modify: `internal/ui/window.go` (`pointerView` Cmd+click branch), `internal/ui/links.go` (`open`, `linkUnder` comment)
- Test: `internal/ui/docs_open_test.go`

**Interfaces:**
- Consumes: `render.IsWebLink`, `mdoc.Resolve`, `mdoc.Roots`, `win.openDoc`, `win.isMember`, `win.paneOwner`, `win.sessionIDs`.
- Produces:
  - `func (win *window) termFolder(t *tabs.Tab) string`: `t.Session.Applier().Cwd()` (session locked), else `t.Dir`, else `win.opt.Dir`, else `win.home`. No git lookups (unlike `locOf`).
  - `func (win *window) docParent(t *tabs.Tab, inPane bool) tabs.TabID`: `t.Parent` for a scratch shell; for a workspace member, `win.paneOwner()` when `inPane`, else the first of `win.sessionIDs()` (0 when none); otherwise `t.ID`.
  - `func (win *window) openDocRef(t *tabs.Tab, inPane bool, ref string)`: session from `docParent` (0: `win.note("Open a session to read " + filepath.Base(ref))`); `mdoc.Resolve(ref, mdoc.Roots{Dir: win.termFolder(t), Home: win.home, Top: top})` with `top` = `win.topFn` or `gitinfo.Top`; not found: `win.note("Can't find " + ref)`; else `win.openDoc(session, path)`.
  - `open(u)` returns without calling anything unless `render.IsWebLink(u)`.
  - `pointerView`, Cmd+click branch: `if render.IsWebLink(u) { win.open(u) } else { win.openDocRef(t, v.tag != win.termTag, u) }`.

- [x] **Step 1: Write the failing tests**:

```go
func TestShortcutClickOpensADocPath(t *testing.T) {
	st := &recStarter{}
	win := testWindow(t, st)
	win.ui = newChrome()
	var opened []string
	win.openLink = func(u string) error { opened = append(opened, u); return nil }
	dir := t.TempDir()
	want := writeDoc(t, dir, filepath.Join("docs", "plan.md"), "# Plan\n")
	tab, _ := shellTab(t, win, st, "wrote docs/plan.md ok", func() bool {
		a := win.mgr.Active()
		return a != nil && textShown(a, 20, 'k')()
	})
	report(t, tab, dir)
	r := &pointerRun{t: t, win: win, tab: tab}
	r.send(pointer.Press, pointer.ButtonPrimary, 0, 8, 0)
	r.send(pointer.Release, 0, 0, 8, 0)
	if win.shownDoc() != nil {
		t.Fatal("a plain click opened the doc")
	}
	r.at += time.Second
	r.send(pointer.Press, pointer.ButtonPrimary, key.ModShortcut, 8, 0)
	r.send(pointer.Release, 0, key.ModShortcut, 8, 0)
	d := win.shownDoc()
	if d == nil || d.path != want || d.session != tab.ID {
		t.Fatalf("doc %+v, want %s under %d", d, want, tab.ID)
	}
	if len(opened) != 0 {
		t.Fatalf("a doc path went to the OS: %v", opened)
	}
}
```

And:

```go
func TestShortcutClickOnAMissingDocSaysSo(t *testing.T)    // shell prints "see docs/nope.md": Cmd+click → no doc, win.notice == "Can't find docs/nope.md"
func TestDocPathFallsBackToTheRepoRoot(t *testing.T)       // root/.git (os.Mkdir), root/docs/x.md, OSC 7 reports root/sub → opens root/docs/x.md
func TestDocPathUsesTabDirWithoutOSC7(t *testing.T)        // tab from win.mgr.NewOpts(tabs.StartOpts{Dir: dir}); no report → resolves against dir
func TestDocPathClickInMouseModeNeverReachesTheApp(t *testing.T) // like TestShortcutClickOpensALinkInMouseMode: "\x1b[?1002h\x1b[?1006hdocs/a.md"; the PTY receives nothing; the doc opens
func TestDocParentForEachTerminal(t *testing.T)            // session a → a; scratch NewOpts{Parent: a.ID} → a; member (openMember, needs win.runs) with inPane and a active → a; member, !inPane → sessionIDs()[0]; all sessions closed → 0
func TestOpenNeverHandsANonWebURLToTheOS(t *testing.T)     // win.open of "file:///etc/passwd", "docs/a.md", "javascript:alert(1)", "C:\\x.md" → openLink never called; "https://a.io" → called once
```

- [x] **Step 2: Run** `go test ./internal/ui/ -run 'DocPath|ShortcutClick|DocParent|NonWebURL' -v`. Expected: FAIL.
- [x] **Step 3: Implement** per the Interfaces.
- [x] **Step 4: Run** `go test ./internal/ui/ -run 'Link|Doc' -v`, then `go test ./... && go vet ./...`. Expected: PASS (the existing link tests stay green).
- [x] **Step 5: Commit** `feat(ui): Open .md paths from the terminal with Cmd+click`

### Task 5: The doc row in the sidebar

**Files:**
- Modify: `internal/ui/sidebar_model.go` (`sideRow.Docs`, `sideInput.Docs`, `buildSide`, `matches`), `internal/ui/sidebar_agents.go` (`rowSpan`), `internal/ui/sidebar.go` (`sideState`, `sideHits`, `sideList`), `internal/ui/sidebar_window.go` (`sideIn`, `drawSide`, `sidePointer`, `sidePress`), `internal/ui/settings_style.go` (`iconNames`, `icon` case `"doc"`)
- Create: `internal/ui/sidebar_docs.go`
- Test: `internal/ui/sidebar_docs_test.go`

**Interfaces:**
- Consumes: `win.sessionDocs`, `win.showDoc`, `win.closeDoc`, `win.docs.shown`.
- Produces:
  - `const sideDocH = 34` (dp; two lines like a subagent line).
  - `type sideDoc struct { ID docID; Name, Folder string }`; `sideRow.Docs []sideDoc`; `sideInput.Docs func(tabs.TabID) []sideDoc` (nil: none); `win.sideDocs(id tabs.TabID) []sideDoc`.
  - `rowSpan` adds `len(r.Docs)*gtx.Dp(sideDocH)`; `sideList` draws doc lines right under the row (`c.docLines`), then the subagent lines below them.
  - `type sideDocHit struct { id docID; rect, close image.Rectangle }`; `sideHits.docs []sideDocHit`.
  - `sideState.doc docID` (the shown doc, highlighted like a shown row; the session row is then not highlighted) and `sideState.hoverDoc docID` (shows that line's ×).
  - `func (c *chrome) docLines(gtx layout.Context, h *sideHits, r sideRow, st sideState, x0, x1, y int, closeCall op.CallOp, closeSize image.Point)`: per doc, the `"doc"` icon (12 dp, `th.Muted`, `th.Accent` when shown), the name at 12 sp in `th.Folder` (`th.ActiveFG` when shown, on an `th.Active` fill), the folder at 10 sp in `th.Muted`, × at the right on hover.
  - `sideRow.matches` also matches a doc's name.
  - `sidePress`: before the subagent lines, a doc hit: middle click or its × → `closeDoc`; primary → `showDoc`. `sidePointer` hover sets `hoverDoc` and redraws only when it changes.
  - `"doc"` icon: `line(6,3, 14,3, 19,8, 19,21, 6,21, 6,3); line(14,3, 14,8, 19,8); line(9,13, 16,13); line(9,17, 16,17)`.

- [ ] **Step 1: Write the failing tests**:

```go
func TestBuildSideNestsDocsUnderTheirSession(t *testing.T) // sideInput with Docs returning one sideDoc for tab 1 → that row's Docs has it, tab 2's has none, sessionOrder unchanged
func TestFilterMatchesADocName(t *testing.T)               // Filter "plan" keeps the row whose doc is plan.md
func TestDocRowClickShowsTheDoc(t *testing.T)              // sideWindow; a := openIn(...); openDoc(a, p); selectTab(a); sideFrame; sideClick on win.side.hits.docs[0].rect → shownDoc() set, win.side.st.doc == its id after the next frame
func TestSessionRowClickShowsTheTerminalAgain(t *testing.T) // doc shown; sideClick rowHit(a).rect → shownDoc() nil, shownTab() == a
func TestDocRowCloseAndMiddleClick(t *testing.T)           // middle click → doc gone; second doc: hover (pointer.Move over it) then click its close rect → gone; tab a still open
func TestDocRowsLeaveSessionOrderAlone(t *testing.T)       // two docs under a: sessionIDs() and nextWaiting's order unchanged; mgr.Tabs() count unchanged
func TestDocRowHoverRedrawsOnlyOnChange(t *testing.T)      // like TestHoverInvalidatesOnlyOnChange: two Moves over the same doc line → win.side.redraws grows by 1
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'Side|DocRow|Filter' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `go test ./internal/ui/ -v -run 'Side|Doc|Hover|Filter'`, then `go test ./... && go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Nest open docs under their session in the sidebar`

### Task 6: Parse markdown into blocks and an outline

**Files:**
- Modify: `go.mod`, `go.sum` (`go get github.com/yuin/goldmark@latest`; it must be a v1.x; note the version in the commit body)
- Create: `internal/mdoc/parse.go`, `internal/mdoc/slug.go`, `internal/mdoc/target.go`
- Test: `internal/mdoc/parse_test.go`, `internal/mdoc/slug_test.go`, `internal/mdoc/target_test.go`

**Interfaces:**
- Produces (package `mdoc`):
  - `type Style uint8` with `Bold, Italic, Code, Strike Style = 1 << iota` (bits).
  - `type Span struct { Text string; Style Style; Link string }` (`Link` is the destination as written, `""` for none; a hard line break is a `"\n"` span; a soft break is a space).
  - `type Kind uint8` with `Heading Kind = iota + 1, Para, CodeBlock, Item, TableBlock, Rule`.
  - `type Block struct { Kind Kind; Level, Indent, Quote int; Marker string; Task int8; Spans []Span; Code string; Table *Table; Slug string }`. `Level` 1..6 for headings. `Indent` is list depth (0 at the top). `Quote` is block-quote depth. `Indent` and `Quote` are capped at `MaxDepth = 8`. An `Item` is a list item's first paragraph with `Marker` (`"•"` or `"<n>."`, numbering from the list's start) and `Task` (0 none, 1 open, 2 done, marker then `""`); its later paragraphs and nested code are `Para`/`CodeBlock` blocks at the same `Indent`. `Code` has no trailing newline.
  - `type Align uint8` with `AlignNone, AlignLeft, AlignCenter, AlignRight`; `type Table struct { Align []Align; Rows [][][]Span }` (`Rows[0]` is the header).
  - `type Heading struct { Level int; Text, Slug string; Block int }`; `type Doc struct { Blocks []Block; Outline []Heading }`.
  - `func Parse(src []byte) *Doc`: `goldmark.New(goldmark.WithExtensions(extension.GFM))`, `Parser().Parse(text.NewReader(src))`, one `ast.Walk`. Read text from segments (`*ast.Text`'s `Segment.Value(src)`, `*ast.String`'s `Value`, code block `Lines()`), not the deprecated `Node.Text`. An image becomes a span of its alt text with `Italic` (`"image"` when empty). An HTML block becomes a `Para` of its raw lines; inline raw HTML becomes plain text. `*ast.AutoLink` becomes a link span (`www.` links get `http://`).
  - `func Slug(text string) string`: GitHub style: lowercase, keep letters, digits, `-`, `_`; spaces become `-`; everything else dropped. `Parse` dedupes like GitHub: `intro`, `intro-1`, `intro-2`.
  - `type TargetKind uint8` with `TargetNone, TargetWeb, TargetDoc, TargetAnchor`; `type Target struct { Kind TargetKind; URL, Path, Anchor string }`; `func Classify(dest string) Target`: `http`/`https` (any case) → Web; `#x` → Anchor `x`; no scheme (or a one-letter scheme followed by `\` or `/`, a drive) with a path part (before `#`, `url.PathUnescape`d) where `IsMarkdown` → Doc with `Path` and `Anchor`; everything else (`mailto:`, `file:`, `javascript:`, `x.go`) → None.

- [ ] **Step 1: Write the failing tests** (table style; compare with `reflect.DeepEqual` or field checks):

```go
func TestParseHeadingsAndOutline(t *testing.T)  // "# A\n## B\n### B\n" → 3 Heading blocks Level 1,2,3; Outline slugs "a","b","b-1", Block indexes 0,1,2
func TestParseInlineStyles(t *testing.T)        // "a **b** *c* `d` ~~e~~ [f](x.md) <https://g.io>" → spans with Bold, Italic, Code, Strike, Link "x.md", Link "https://g.io"
func TestParseNestedListsAndNumbering(t *testing.T) // "3. one\n4. two\n   - deep\n" → Items Marker "3.","4.", "•" with Indent 0,0,1
func TestParseTaskList(t *testing.T)            // "- [ ] a\n- [x] b\n" → Task 1 and 2, Marker ""
func TestParseBlockquote(t *testing.T)          // "> a\n>> b\n" → Para Quote 1, Para Quote 2
func TestParseCodeBlocks(t *testing.T)          // fenced "```go\nx := 1\n```" and 4-space indented → CodeBlock with Code "x := 1"
func TestParseTable(t *testing.T)               // "| a | b |\n|:--|--:|\n| 1 | 2 |" → Table Align [Left, Right], 2 rows, cell spans "a","b","1","2"
func TestParseRuleImageAndHTML(t *testing.T)    // "---" → Rule; "![alt](p.png)" → span "alt" Italic; "<div>x</div>" → Para containing "<div>x</div>"
func TestParseDeepNestingStaysBounded(t *testing.T) // 5000 nested "> " and 2000-deep "- " lists, run in a goroutine with a 10 s deadline (t.Fatal on timeout); every Quote and Indent <= MaxDepth
func TestSlugGitHubStyle(t *testing.T)          // "Hello, World!" → "hello-world"; "Plan 1: Core" → "plan-1-core"; "日本語" → "日本語"
func TestClassifyTargets(t *testing.T)          // "https://a.io"→Web; "HTTP://A.IO"→Web; "#intro"→Anchor; "../x.md"→Doc; "other.md#plan-1"→Doc+Anchor; "my%20doc.md"→Doc "my doc.md"; `C:\a\b.md`→Doc; "mailto:a@b.c","file:///x.md","javascript:x","main.go",""→None
```

- [ ] **Step 2: Run** `go test ./internal/mdoc/ -v`. Expected: FAIL (undefined `Parse`, `Slug`, `Classify`).
- [ ] **Step 3: Add goldmark and implement.** `go get github.com/yuin/goldmark@latest && go mod tidy`. Track list depth and quote depth on a stack while walking; append blocks on `entering` for leaf blocks and collect inline spans with the current style bits.
- [ ] **Step 4: Run** `go test ./internal/mdoc/ -v && go vet ./... && go build ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(mdoc): Parse markdown with goldmark into blocks and an outline`

### Task 7: Lay out and draw the document column

**Files:**
- Create: `internal/ui/doc_layout.go` (wrapping, measuring, cache), `internal/ui/doc_draw.go` (`drawDoc` replacing Task 3's stub)
- Modify: `internal/ui/docs.go` (`openDoc` parses and fits; new fields), `internal/ui/tabbar.go` (`faceText`)
- Test: `internal/ui/doc_layout_test.go`

**Interfaces:**
- Consumes: `mdoc.Parse`, `mdoc.Doc`, `mdoc.Block`, `win.ui.fit`, `win.ui.shaper`.
- Produces:
  - `docView` gains `doc *mdoc.Doc`, `lay docLayout`, `layouts int` (relayout counter for tests). `openDoc` (and later reloads) runs `mdoc.Parse(file.Data)` then `win.fitDoc(doc)`, which passes every span text, every code line and every cell through `win.ui.fit` once, so drawing never discovers fonts.
  - `func (c *chrome) faceText(gtx layout.Context, s string, size unit.Sp, f font.Font, col color.NRGBA) (op.CallOp, image.Point)`: like `sans` but any face and no `fit`.
  - `type docRole uint8` with `roleBody, roleHeading, roleMuted, roleCode, roleLink`; `func (win *window) docColor(r docRole) color.NRGBA`: body and heading `win.pal.FG`, muted `win.ui.th.Muted`, code `win.pal.FG`, link `win.pal.ANSI[4]` (the terminal's link blue).
  - `type docRun struct { text string; x, w int; face font.Font; size unit.Sp; role docRole; link int; under, strike bool }` (`link` indexes `docLayout.links`, -1 for none).
  - `type docLine struct { y, h int; runs []docRun }`; `type docBox struct { r image.Rectangle; kind docBoxKind }` with kinds `boxCode, boxRule, boxQuote, boxTableLine, boxTableHead, boxTaskOpen, boxTaskDone`.
  - `type docLayKey struct { doc *mdoc.Doc; width int; metric unit.Metric; shaper *text.Shaper }`; `type docLayout struct { key docLayKey; lines []docLine; boxes []docBox; links []string; headY []int; height int }` (`headY[i]` is the top of `Outline[i]`'s block).
  - `func (win *window) layoutDoc(gtx layout.Context, d *docView, width int) *docLayout`: returns `&d.lay` unchanged when the key matches; else lays out again and increments `d.layouts`.
  - Measuring: `func (win *window) runeAdv(gtx layout.Context, f font.Font, size unit.Sp, r rune) fixed.Int26_6`, cached in `win.docs.adv map[docAdvKey]fixed.Int26_6` (key: rune, face, px per em; cleared when it passes 8192 entries or the shaper changes). A word's width is the sum, rounded up. Words wider than the line are broken where they fill it.
  - Sizes (sp) and spacing (dp): body 14 with 22 line height; H1 24, H2 20, H3 17, H4 15, H5 14, H6 13, all SemiBold, 20 above (12 for H1 at the top), 8 below, H1 and H2 with a 1 px `th.Border` rule under; code 12.5 mono with 19 line height, 12 padding, box fill `mix(win.pal.BG, win.pal.FG, 0.07)` with 6 radius; inline code in mono on the same fill; paragraph gap 10; list indent 22 per level, marker right-aligned in that indent; quote bar 3 wide in `th.Border`, 14 indent per level, text `roleMuted`; rule 1 px `th.Border` with 12 above and below; table 13 with 8 cell padding, header SemiBold on `tint(th.Accent, 0x14)`, grid lines `th.Border`; task box 12 square, stroked `th.Muted`, filled `th.Accent` with a check when done. Bold = SemiBold, italic = `font.Italic` at Medium, link underlined.
  - `drawDoc(gtx, d, r)`: fill `r` with `win.pal.BG`; the content column is `min(r.Dx()-2*36dp, 720dp)` wide, centred (Task 8 takes the outline's width off `r` first); above the content, the path (`tildeHome`) in 11 sp mono `roleMuted` and `Loaded 3:04 PM` (`d.loadedAt.Format("3:04 PM")`); under them, when set, `Showing the first 2 MB of <Total in MB, one decimal> MB.` (cut) and `File removed. Showing the last loaded version.` (removed), in `th.Needs`. Draw only the lines and boxes that overlap the view (binary search by `y`), offset by `-d.scroll`. Record every drawn string in `win.docs.texts` (reset each frame) for tests, as Settings does with `s.texts`.

- [ ] **Step 1: Write the failing tests** (use `newChrome()`, `pageContext(w, h)`, a window from `testWindow` with `win.ui = newChrome()` and `win.applyTheme(theme.Default())`; build docs with `mdoc.Parse`):

```go
func TestDocLayoutWrapsWithinTheColumn(t *testing.T)     // a 200-word paragraph at width 400 → more than 3 lines; every run x+w <= 400
func TestDocLayoutBreaksAWordWiderThanTheColumn(t *testing.T) // one 300-rune path word → several lines, all within 400
func TestDocLayoutIndentsNestedLists(t *testing.T)       // depth-1 item's first text run x > depth-0 item's
func TestDocLayoutWrapsLongCodeLines(t *testing.T)       // a 500-rune code line → several lines, all runs mono (face.Typeface == chromeFace), all within width
func TestDocLayoutFitsWideTables(t *testing.T)           // 5 columns of 40-word cells at width 500 → all runs within 500
func TestDocLayoutHeadingPositions(t *testing.T)         // len(headY) == len(Outline), strictly increasing
func TestDocLayoutIsCachedUntilWidthChanges(t *testing.T) // layoutDoc twice at 400 → layouts == 1; at 401 → 2
func TestDocLayoutOfAHugeLineIsBounded(t *testing.T)     // 2 MB of "a" with no spaces; skip under -short; finishes within 20 s (goroutine + deadline) and every run within width
func TestDocColorsFollowTheThemeWithoutRelayout(t *testing.T) // drawDoc under Midnight, applyTheme(washi) via theme.ByKey, drawDoc again: layouts still 1, docColor(roleBody) == win.pal.FG, docColor(roleLink) == win.pal.ANSI[4]
func TestDrawDocShowsPathAndNotices(t *testing.T)        // d.file.Cut = true, Total 3_500_000, d.removed = true → win.docs.texts has the tilde path, "Showing the first 2 MB of 3.5 MB." and "File removed. Showing the last loaded version."
func TestDrawDocInEveryTheme(t *testing.T)               // for each theme.All(): applyTheme, drawDoc at 900x600 → no panic, texts non-empty
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'DocLayout|DrawDoc|DocColors' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `go test ./internal/ui/ -run 'Doc' -v`, then `go test ./... && go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Render markdown docs in a reading column`

### Task 8: The outline and scrolling

**Files:**
- Create: `internal/ui/doc_outline.go`
- Modify: `internal/ui/doc_draw.go`, `internal/ui/docs.go` (`docEvents`)
- Test: `internal/ui/doc_outline_test.go`

**Interfaces:**
- Consumes: `docLayout.headY`, `mdoc.Doc.Outline`.
- Produces:
  - `type docHits struct { view, outline image.Rectangle; heads []docHeadHit; links []docLinkHit; scrollMax, outScroll, outMax int }`, `type docHeadHit struct { rect image.Rectangle; i int }`, `type docLinkHit struct { rect image.Rectangle; link int }`; `win.docs.hits docHits`, refreshed by every `drawDoc` (window coordinates, visible parts only).
  - Outline column: shown when `r.Dx() >= gtx.Dp(600)`, `min(gtx.Dp(220), r.Dx()/3)` wide, drawn like the Settings menu (`page.nav`): `th.Chrome` fill and a 1 px `th.Border` edge; an `Outline` heading (18 sp SemiBold `th.ActiveFG`); one 30 dp entry per heading at 12 sp, `th.Muted`, indented 12 dp per level below the shallowest (at most 3 steps); the current one with the Settings selection look (`tint(th.Accent, 0x2e)` fill, `th.ActiveFG` text). The outline scrolls on its own with the wheel when longer than the column.
  - `func (win *window) docCurrent(gtx layout.Context, d *docView) int`: the last outline index with `headY <= d.scroll + gtx.Dp(24)`, -1 before the first heading.
  - `func (win *window) docScrollTo(d *docView, y int)`: clamps to `[0, win.docs.hits.scrollMax]`, invalidates.
  - `func (win *window) docKey(gtx layout.Context, e key.Event)`: Up/Down 40 dp, PageUp/PageDown the view height minus 40 dp, Home/End; kaze shortcuts still go to `runShortcut` (Task 3).
  - `func (win *window) docPointer(gtx layout.Context, e pointer.Event)`: wheel over the outline scrolls it, elsewhere the doc; a primary press on a heading entry calls `docScrollTo(d, headY[i]-gtx.Dp(12))`.
  - `docEvents` keeps its pointer filter at `Press|Scroll` only (never `Move`, `Enter` or `Leave`), so hovering the doc never wakes the window.
  - Hand cursors: `pointer.CursorPointer` areas over the heading entries, inside the page's input area (like `linkCursors`); cursor areas need no frames.

- [ ] **Step 1: Write the failing tests**:

```go
func TestDocKeysScroll(t *testing.T)               // long doc drawn at 900x600; Down → scroll 40; PageDown → +view-40; End → scrollMax; Home → 0
func TestDocWheelScrolls(t *testing.T)             // docPointer(Scroll Y=120 inside view) → scroll 120; never past scrollMax
func TestOutlineClickScrollsToHeading(t *testing.T) // press at centre(win.docs.hits.heads[2].rect) → scroll == headY[2]-12 (clamped)
func TestOutlineHighlightsTheSectionInView(t *testing.T) // docScrollTo(headY[1]) → docCurrent == 1; scroll 0 → 0 (or -1 when text precedes the first heading)
func TestOutlineHiddenOnNarrowWindow(t *testing.T) // drawDoc in a 500 dp wide rect → hits.outline empty, the content column starts at the left padding
func TestDocShortcutsStillRun(t *testing.T)        // docKey with the new_tab shortcut (Ctrl+Shift+T on the "windows" keymap testWindow builds) → a new tab is shown, no doc shown
func TestDocViewTakesKeysInAFrame(t *testing.T)    // sideWindow + input.Router: openDoc, sizedFrame (focus moves), queue key.Event{Name: key.NameDownArrow, State: key.Press}, sizedFrame → d.scroll == 40 and the session's PTY got nothing
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'DocKeys|DocWheel|Outline|DocShortcuts|DocViewTakes' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `go test ./internal/ui/ -run 'Doc|Outline' -v`, then `go test ./... && go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Add the doc outline and keyboard scrolling`

### Task 9: Links inside a doc

**Files:**
- Modify: `internal/ui/doc_draw.go` (record `docLinkHit`s, hand cursors over them), `internal/ui/doc_outline.go` (`docPointer` press on a link), `internal/ui/docs.go` (pending anchor)
- Test: `internal/ui/doc_links_test.go`

**Interfaces:**
- Consumes: `mdoc.Classify`, `mdoc.Resolve`, `win.open`, `win.openDoc`, `docLayout.links`, `Doc.Outline`.
- Produces:
  - `func (win *window) followDocLink(d *docView, dest string)`, run by a plain primary press on a link (no modifier needed):
    - `TargetWeb` → `win.open(t.URL)`.
    - `TargetAnchor` → `win.docAnchor(d, t.Anchor)`.
    - `TargetDoc` → `mdoc.Resolve(t.Path, mdoc.Roots{Dir: filepath.Dir(d.path), Home: win.home, Top: top})`; not found → `win.note("Can't find " + t.Path)`; else `nd := win.openDoc(d.session, path)` (switches when already open) and, with an anchor, `win.docAnchor(nd, t.Anchor)`.
    - `TargetNone` → nothing.
  - `func (win *window) docAnchor(d *docView, slug string)`: sets `d.anchor = slug`; the next `drawDoc`, after layout, scrolls to the matching `Outline` entry's `headY` and clears it (an unknown slug is cleared without moving).

- [ ] **Step 1: Write the failing tests** (two docs in one temp folder, `a.md` linking to the others; press at the centre of the drawn `docLinkHit` for the link under test):

```go
func TestDocWebLinkOpensTheBrowser(t *testing.T)     // [site](https://example.com/x) → openLink got exactly "https://example.com/x"
func TestDocLinkOpensAnotherDocUnderTheSameSession(t *testing.T) // [b](b.md) → shown doc b.md, session == a's session, 2 docs; clicking again from a.md → still 2 docs
func TestDocLinkResolvesFromTheDocsFolder(t *testing.T) // a.md in dir/docs links [c](../c.md) → dir/c.md opens
func TestDocAnchorLinkScrolls(t *testing.T)          // [go](#later) → after the next drawDoc, scroll == headY of "Later" (clamped)
func TestDocLinkToAHeadingInAnotherDoc(t *testing.T) // [b](b.md#part-2) → b.md shown and, after its first draw, scrolled to "Part 2"
func TestDocLinkMissingSaysSo(t *testing.T)          // [x](nope.md) → win.notice == "Can't find nope.md"
func TestDocOtherLinksDoNothing(t *testing.T)        // mailto:, file:///x.md, main.go, javascript:x → openLink never called, docs unchanged, no notice
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'DocWebLink|DocLink|DocAnchor|DocOtherLinks' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `go test ./internal/ui/ -run 'Doc' -v`, then `go test ./... && go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Follow links inside a doc`

### Task 10: Keep the doc current

**Files:**
- Create: `internal/ui/doc_reload.go`
- Modify: `internal/ui/docs.go` (`showDoc` checks), `internal/ui/window.go` (`setFocused`: check the shown doc on focus; `frame`: `docTick` before drawing)
- Test: `internal/ui/doc_reload_test.go`

**Interfaces:**
- Consumes: `mdoc.StatFile`, `mdoc.Load`, `mdoc.Parse`, `win.fitDoc`.
- Produces:
  - `const docPoll = time.Second`; `win.docs.checked time.Time`.
  - `func (win *window) refreshDoc(d *docView, now time.Time)`: `StatFile`; a not-exist error sets `d.removed` (content kept); any other error leaves everything as is; a `Stamp` different from `d.file.Stamp` (mtime or size), or the file back after `removed`, reloads: `Load`, `Parse`, `fitDoc`, `d.loadedAt = now`, `d.removed = false`; `d.scroll` is kept and clamped by the next layout. An unchanged stamp parses nothing.
  - `func (win *window) docTick(now time.Time) (time.Time, bool)`: false when no doc is shown; when `now >= checked+docPoll`, `refreshDoc(shown, now)` and `checked = now`; returns `checked+docPoll, true`. `frame` executes `op.InvalidateCmd{At: at}` when ok, before the drawing branch.
  - `showDoc` and `setFocused(true)` call `refreshDoc` on the shown doc and reset `checked`.

- [ ] **Step 1: Write the failing tests** (temp files; set times with `os.Chtimes` so the tests never depend on file system time granularity):

```go
func TestDocReloadsWhenTheFileChanges(t *testing.T)   // open, draw, docScrollTo(200); rewrite with a new heading, Chtimes +2s; docTick(now+1s) → Outline has the new heading, scroll still 200 (doc long enough), loadedAt updated
func TestDocReloadsOnSizeChangeInTheSameSecond(t *testing.T) // rewrite with different length, Chtimes back to the old mtime → reloads
func TestUnchangedFileIsNotReparsed(t *testing.T)     // docTick twice a second apart with no change → d.doc pointer unchanged
func TestDocKeepsContentWhenTheFileIsRemoved(t *testing.T) // os.Remove; docTick → d.removed, d.doc unchanged, drawDoc texts include "File removed. Showing the last loaded version."
func TestDocComesBackAfterAReplace(t *testing.T)      // remove, docTick (removed), write again, docTick → removed false, new content
func TestDocTimerOnlyWhileShown(t *testing.T)         // doc open but terminal shown → docTick false; showDoc → true with at == checked+1s; hideDoc → false
func TestShowingAndFocusCheckTheFile(t *testing.T)    // change the file, selectTab, showDoc(d) → reloaded without a tick; change again, setFocused(false), setFocused(true) → reloaded
```

- [ ] **Step 2: Run** `go test ./internal/ui/ -run 'DocReload|Unchanged|Removed|ComesBack|DocTimer|FocusCheck' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `go test ./internal/ui/ -run 'Doc' -v`, then `go test ./... && go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Reload a shown doc when its file changes`

### Task 11: Open paths from a subagent pane

**Files:**
- Modify: `internal/render/links.go` (`LinksIn`), `internal/ui/panes_agent.go` (`agentPaneEvents`, factor the body inset of `drawAgentPane` into `agentInner`)
- Test: `internal/render/links_test.go`, `internal/ui/panes_agent_test.go`

**Interfaces:**
- Consumes: `appendLinks`, `win.openDocRef`, `win.open`, `p.wrapped`, `win.fl.owner`.
- Produces:
  - `type LinkText struct { Start, End int; Text string }` (byte offsets into the string); `func LinksIn(s string) []LinkText` (allocates; for clicks only).
  - `func agentInner(gtx layout.Context, body image.Rectangle) image.Rectangle`: the rectangle `drawAgentPane` draws lines in (10 dp sides, 6 dp top and bottom); `drawAgentPane` uses it.
  - `func (win *window) agentPaneLink(gtx layout.Context, p *pane, at image.Point) (string, bool)`: line index `(at.Y - inner.Min.Y + p.scroll) / gtx.Dp(18)` into `p.wrapped`; the line's x start and font as `drawAgentPane` draws it (prompt +10 dp at 12 sp, waiting strip +8 dp at 11 sp Medium, report 12 sp, others 11 sp); for each `LinksIn(line)` entry, `[x+textWidth(prefix), x+textWidth(prefix+text))` contains `at.X`.
  - `agentPaneEvents`: a primary press with `key.ModShortcut` that hits a link opens it (`render.IsWebLink` → `win.open`, else `win.openDocRef(owner, true, ref)` where `owner` is `win.mgr.Lookup(win.fl.owner(p.id))`); otherwise the pane is raised as before.

- [ ] **Step 1: Write the failing tests**:

```go
func TestLinksInGivesByteOffsets(t *testing.T) // "見て docs/a.md と https://a.io" → two entries; s[Start:End] == Text for each
func TestAgentPaneShortcutClickOpensADoc(t *testing.T) // paneWindow; KindAgentStop with Report "Wrote docs/plan.md for review"; temp file under the session's folder (report OSC 7 for a to a temp dir); floatFrames; openAgentPane; floatFrames; press with ModShortcut at the measured x of "docs/plan.md" on its line → shownDoc under a; plain press → only raises
```

- [ ] **Step 2: Run** `go test ./internal/render/ ./internal/ui/ -run 'LinksIn|AgentPaneShortcut' -v`. Expected: FAIL.
- [ ] **Step 3: Implement** per the Interfaces.
- [ ] **Step 4: Run** `go test ./internal/ui/ -run 'Agent' -v`, then `go test ./... && go vet ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(ui): Open doc paths from a subagent pane`

### Task 12: Docs, both platforms, and the hand-over

**Files:**
- Modify: `README.md` (`## Mouse`, a new `## Reading docs` section after `## Subagents`, `## Roadmap` "Done"), `docs/HANDOFF.md` (`## Where things stand`, `## Next`)

- [ ] **Step 1: README.** In `## Mouse`, the Links bullet adds: `.md` paths are drawn the same way, and Cmd+click (Ctrl+click on Windows) opens one in kaze. New `## Reading docs` section, plain words, no em dashes: how to open, the outline, the row under the session, links inside a doc, that it reloads on change, the 2 MB cap, that rows are not restored, the close key (`close_tab`: Cmd+W / Ctrl+Shift+W). Roadmap "Done": `- Markdown reader: Cmd+click an .md path to read it in kaze, nested under its session.`
- [ ] **Step 2: HANDOFF.** A short "Markdown reader" paragraph at the top of `## Where things stand` (branch `feat/markdown-reader`, what works, the decisions table's non-obvious calls: member-as-main nesting, Ctrl+Shift+W on Windows, no italic face, no palette entry), and under `## Next`: verify on Windows (Ctrl+click, backslash paths, drive letters) and the owner's manual check below.
- [ ] **Step 3: Gates and builds.**

```bash
go test ./... && go vet ./...
GOOS=windows GOARCH=amd64 go build -o /dev/null ./cmd/kaze
go build -o kaze ./cmd/kaze
```

Expected: tests and vet PASS; both builds succeed. (Delete the local `kaze` binary afterwards; never commit it.)

- [ ] **Step 4: Manual check for the owner** (write this list into the PR description; do not drive the GUI with injected input): in a kaze tab run `printf 'see docs/superpowers/specs/2026-11-01-kaze-markdown-reader-design.md\n'` in the repo; the path is blue and underlined; Cmd+click opens it under that session; the outline highlights while scrolling; clicking the session row returns to the terminal; edit the file in another tab and watch it reload; Cmd+W closes the row; try all five themes.
- [ ] **Step 5: Commit** `docs(reader): Describe the markdown reader`
- [ ] **Step 6:** Push the branch (`git push -u origin feat/markdown-reader`) and open a PR to `main` only when the owner asks. No release PR, tag or version bump (release hold).
