# Kaze markdown reader: design

Date: 2026-11-01. Status: draft for the owner's approval.

## Why

Claude Code and Codex (through the superpowers skills) write specs, plans and reports as `.md` files and
end with "I've written it to `docs/superpowers/specs/...md`". Kaze shows that as plain text. Opening it
means finding the folder in an editor or Finder, and the owner admits that is enough friction that they
skip reading the doc altogether. The point of this feature is to turn "the agent mentioned a file" into
"I'm reading it" with one click, without leaving kaze.

Success: the owner Cmd+clicks the path in the agent's reply and is reading the rendered doc a moment
later, next to the session that wrote it, and gets back to the session with one click.

## Goals

1. **Clickable `.md` paths in the terminal.** Cmd+click (Ctrl+click on Windows) on a path ending in `.md`
   in terminal output opens it in kaze. Same gesture, hand cursor and wrapped-row handling as the existing
   web links (PR #68, `internal/render/links.go`, `internal/ui/links.go`).
2. **A read-only doc view** with a heading outline on the left (layout B from the mockups, owner's choice)
   and the rendered document in a centred column on the right.
3. **A sidebar row per open doc**, nested under the session it was opened from.
4. **Links inside a doc** follow the same rules as links in the terminal (below).
5. **Stays current:** when the agent edits the file, the view shows the new content.

## Non-goals

- **Other file types.** Only `.md`. The owner's words: for `.go`, `.tsx` and the like, "id just open an
  ide". Paths to other files are not detected in the terminal.
- **Editing.** The view is read-only. Kaze never writes to the file.
- **Restoring docs after a restart.** Doc rows are throwaway (owner: "throwaway bro to avoid clutter").
  Session restore brings sessions back without their docs. The files themselves are untouched; only
  the open-in-kaze row goes away.
- **Splits or side-by-side reading.** A doc replaces the terminal in the main area, like Settings does.
  Splits may come with "real panes" later; not part of this.
- **Images, mermaid diagrams, HTML blocks, footnotes, math.** Shown as their source text or skipped
  (see "Rendering").
- **OSC 8 hyperlinks** (programs marking text as a link). Separate feature.

## Anti-goals (would make this a failure even if it works)

- Kaze opening, running or handing to the OS anything other than an `http`/`https` URL or a `.md` file it
  renders itself. Terminal output must never be able to make kaze launch a program. The existing rule in
  `internal/ui/links.go` ("Only http and https links are ever found") stays true for the OS hand-off.
- Making the terminal slower. Path detection must keep the existing link scan's properties: no per-frame
  allocation, no file system access while drawing, no extra frames from hovering.
- A doc row that looks like a session (confusing the sidebar counts, Ctrl+Tab order, "Next waiting", the
  palette's Sessions section or session restore).

## How it works (decided with the owner)

### Opening from the terminal

- Cmd+click (Ctrl+click on Windows) on an `.md` path, in the main terminal, in scrollback or in a floating
  pane, opens it. A plain click still selects text, as with web links. Works while an app such as Claude
  Code has the mouse, without the app seeing the click (same as web links today).
- The doc opens as a new row in the sidebar, nested under the session whose terminal held the path, and
  becomes the shown view.
- Clicking a path whose doc is already open (same resolved file, under that session) switches to the
  existing row instead of opening a second one.

### Terminals that aren't a session row (my call)

Docs only ever nest under session rows in the sessions list, never in the Workspaces section, so the
sidebar gets one new kind of nested line, not two. For each kind of terminal:

| Clicked in | Folder that resolves a relative path | Doc goes under |
|---|---|---|
| A session's terminal | that session's | that session |
| A scratch shell | the scratch shell's | its parent session (`Tab.Parent`) |
| A subagent pane | the session it belongs to | that session |
| A floating workspace member pane | the member's (it may run in its own folder) | the session it floats over |
| A workspace member shown as the main view | the member's | the active session (`mgr.Active()`) |

The folder always comes from the terminal the path was printed in, because that is what the path is
relative to. "Closing the session closes its docs" applies to the session in the right column only.

### While a doc is in front (my call)

- **Alerts:** a doc is treated like Settings. The session behind it doesn't count as seen, so a "needs
  you" alert from that session stays up, chimes and shows in the toast while the owner reads. They can't
  see Claude's prompt from the doc, so clearing it would hide a real request.
- **Active session:** stays the doc's session. Ctrl+Tab, Next waiting and closing a session pick from
  there as if its terminal were shown, and land on a terminal, not on a doc.
- **Keyboard:** goes to the doc view (scrolling), never to the session's shell.

### The doc view (layout B)

- **Title bar breadcrumb:** the repo or folder name, then the file name (`kaze-terminal / markdown-tab-design.md`).
- **Outline column** on the left, styled like the Settings section list: an "Outline" heading, then the
  doc's headings, deeper levels indented. The section currently in view is highlighted. Clicking a
  heading scrolls the doc to it.
- **Document column:** centred, a comfortable reading width. The file's path in small mono text above the
  content. Body text in the UI sans font (Manrope), code in JetBrains Mono, Midnight (or the current
  theme's) colours.
- Status bar as usual; it may show when the file was last loaded ("updated 2s ago").

### The sidebar row

- Nested under its session, the way subagents sit under theirs: a document mark, the file name, and the
  folder under it in muted text.
- Clicking the row shows the doc; clicking the session row shows the terminal again.
- × on hover closes the row (and middle click, like sessions).
- Closing the session closes its doc rows.

### Links inside a doc

A plain click (no Cmd needed; there is nothing to type into or select by accident):

- **Another `.md` file** (relative to the doc's own folder, or absolute): opens as its own row under the
  same session, or switches to it if already open. The owner goes back by clicking the first doc's row.
  No back button.
- **`http` / `https`:** opens in the system browser, same as the terminal.
- **`#heading` in the same doc** (or `other.md#heading`): scrolls to that heading.
- **Anything else** (a `.go` file, `mailto:`, `file:`): drawn as a link, does nothing.

### Lifetime

- Doc rows live only while kaze runs. Not saved in `sessions.json`, not restored.
- Closing a doc row, its session, or kaze never touches the file.

## Constraints

- Go + Gio, macOS and Windows from one codebase (`CLAUDE.md`). Strict TDD; RED tests must fail safely
  (bounded sizes and timeouts).
- Never inject global keyboard or mouse input when testing the GUI.
- Path detection lives next to the URL detection in `internal/render/links.go` and must keep its
  allocation-free, bounded-scan properties (`maxWrapRows`).
- The open-in-the-OS function (`openURL`) must only ever receive `http`/`https` URLs. Rendering a `.md`
  file is done by kaze itself and never goes through the OS.
- A doc is not a `tabs.Tab` (no PTY, no shell). It must stay out of everything that counts or walks
  sessions: tab count in the status bar, Ctrl+Tab, Next waiting, the palette's session list, alerts,
  session restore.

## What's left to the builder

- **How a doc is modelled.** Recommended: a window-level list of open docs, each with its parent
  `tabs.TabID`, shown through a "shown view" switch like `win.set.shown` for Settings, rather than a new
  kind of `tabs.Tab`. Builder's call.
- **Markdown parsing.** Kaze has no markdown dependency today. Options: `github.com/yuin/goldmark`
  (pure Go, CommonMark plus GFM tables, no cgo) or a small hand-written parser for the subset below.
  My call: goldmark, because GFM tables and nested lists are where hand-written parsers go wrong. The
  builder may push back with a reason.
- **Rendering.** Drawn with Gio widgets, not the terminal grid. Must handle: headings 1 to 6, paragraphs,
  bold, italic, inline code, links, bullet and numbered lists (nested), task list items, block quotes,
  fenced and indented code blocks (no syntax highlighting; horizontal scroll or wrap is the builder's
  call), GFM tables, horizontal rules. Images show their alt text. Raw HTML shows as plain text.
- **What counts as an `.md` path in the terminal.** A run of path characters ending in `.md` (any case),
  with or without a leading `./`, `~/` or `/` (and a drive letter on Windows). Trailing punctuation
  trimmed as URLs do. A trailing `:<line>` is allowed and ignored. Paths with spaces are out of scope.
  The builder settles the exact character set with tests from real Claude Code and Codex output
  (backticks around the path, a path after `⎿`, a path inside a box-drawing frame).
- **Resolving a relative path.** Against the session's folder: its OSC 7 folder (`Session.Cwd`), else
  `Tab.Dir`, else the window's default. If that file doesn't exist, try the git repo root of that folder.
  If nothing exists, don't open anything; show a short status bar notice ("Can't find <path>").
- **When a path is shown as a link.** My call: draw `.md` paths like web links (blue, underlined, hand
  cursor) from the text alone, and check the file exists only on click. That keeps file system access out
  of drawing. A path that turns out not to exist gets the notice above.
- **Keeping the view current.** Reload when the file changes on disk. No new dependency for file
  watching; a modification-time check is enough. My call: check when the doc is shown, on window focus,
  and on a slow timer (about once a second) only while a doc is the shown view, so an idle window with a
  terminal in front never wakes. Keep the scroll position across a reload. If the file is deleted, keep
  the last content and say "file removed" in the view.
- **Large files.** Cap what is read (my call: 2 MB) and say so in the view if a file is cut.
- **Keyboard in the doc view:** scrolling with arrows, Page Up/Down, Home/End and the wheel. Cmd+W /
  Ctrl+W closes the doc row if kaze has such a key for sessions. Text selection and copy are nice to have,
  not required for the first version.
- **Theming:** use the current theme's chrome and terminal colours, so it reads right in all five themes,
  including the light Washi.
- **Palette:** whether open docs appear in the Cmd+K palette. Not required.
- **Search inside a doc:** out of scope for the first version.
- **Mockups:** layout B is `.superpowers/brainstorm/2862-1793528341/content/reader-layout-v3.html`
  (local, not committed). The written description above is the authority.

## Testing notes

- `internal/render`: table tests for `.md` path detection next to the URL ones: real agent output,
  wrapped paths, trailing punctuation, `:line` suffixes, Windows paths, non-`.md` paths ignored, and the
  no-allocation property.
- Resolution: relative, `./`, `~/`, absolute, repo-root fallback, missing file, using a temp dir.
- `internal/ui`: opening, deduplication, nesting under the right session, closing with the session, not
  saved by session restore, not counted as a tab, links inside a doc (each kind), the OS hand-off never
  receiving a non-web URL.
- Reload: write to a temp file, check the view picks up the change and keeps its scroll position.
