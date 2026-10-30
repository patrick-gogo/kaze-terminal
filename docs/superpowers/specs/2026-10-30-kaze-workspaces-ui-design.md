# Workspaces UI round 2: design

Date: 2026-10-30. Owner: Patrick. Builds on the [Workspaces decisions](2026-10-29-kaze-workspaces-decisions.md) (Plan 5b, merged). Everything there stands unless this document changes it.

Mockup: `.superpowers/brainstorm/84711-1793324126/content/editor-layout.html`, option **B** (chosen). Screens of the current UI that prompted this: the Workspaces list, the editor, and the running group dropdown, rendered offscreen on 2026-10-30.

## 1. Why

Workspaces are built but the owner has not used them for real yet. Looking at the current screens, it is hard to tell which command belongs to which tab once a workspace has several tabs with several commands each:

- The **editor** is one long flat form. "Tab 1", "Tab 2" look like every other row, so tab boundaries vanish, and the tab's color is picked at the bottom of its block. The owner's work project needs at least **5 tabs**; filling those in one column is cramped.
- The **Workspaces list** shows each tab as one plain line (title, folder, commands in the same color, columns not aligned) and cuts long command chains mid-word. The tab's color does not appear.
- The **running group dropdown** lists members by name and folder but not what each runs or how far it got.
- Folders must be typed by hand.

Success: setting up a 5-tab workspace feels roomy, and at every point (editing, browsing, running) you can tell at a glance which commands belong to which tab.

## 2. Goals

1. A two-pane workspace editor (tab list left, one tab's details right).
2. A system folder picker for each tab's folder.
3. A clearer Workspaces list.
4. A running dropdown that shows what each member runs and its status.
5. Small cleanups on the About page and settings sidebar.

## 3. Non-goals

- **No Claude session launching.** Starting a Claude session as part of a workspace was raised and dropped by the owner this round.
- No change to how commands run (one joined line after the prompt), shell readiness, waits, probes, the tab group chip, or the workspaces file format.
- No drag-and-drop reordering (↑ ↓ stay).
- No "failed" status for a finished command (kaze cannot see exit codes yet; see 7).
- No IME, no multi-folder import.

## 4. Anti-goals (would make this a failure even if it works)

- Any idle CPU cost or animation (kaze's idle CPU must stay at zero).
- The folder picker freezing kaze's window or terminal output while open.
- Losing draft text when switching between tabs in the editor (the existing "never drop a text field edit" rule applies across tab switches).
- A layout that only works in one theme; everything uses theme tokens and must read well in all five, including the light Washi.

## 5. The workspace editor (decided: layout B)

Opened with **Edit** (or **+ New workspace**). Header: the workspace **Name** field, **Cancel** and **Save**, as today. Below it, two panes.

**Left pane: the tab list**
- One row per tab, in run order: **color dot**, **title**, and a muted second line "N commands" (or "No commands").
- Click a row to select it. The selected row is highlighted. **The first tab is selected when the editor opens** (owner agreed).
- **+ Add tab** at the bottom adds a tab, selects it and puts the cursor in its Title field. A new tab takes the **next color not yet used** in this workspace (cyan, magenta, yellow, green, blue, red order), so 5 tabs get 5 colors without effort; after six it cycles.
- A tab with a validation problem (missing folder, bad port, empty title...) shows a **red mark** on its row, so problems can be found without opening each tab.

**Right pane: the selected tab**
- A **colored bar** down the left edge in the tab's color, and the tab's dot and title as the pane heading, so it is always clear which tab is being edited.
- **↑ ↓ Remove** at the top right of the pane, acting on the selected tab (same rules as today: first can't go up, last can't go down).
- Fields: **Title**, **Folder** (with **Choose…**, section 6), **Commands**, **Color**, **Wait for** (and Port when Wait is Port), as today.
- **Commands are numbered** 1, 2, 3 in run order, each with its own × remove, then **+ Add command**. The "Typed as one line" preview stays under the commands.
- Field errors show next to their field, as today.

**Unchanged:** the draft model (nothing is written until Save; Save never runs anything), leave-with-unsaved-changes prompt, stale-save refusal, duplicate-name check, theme switch keeping the draft, the startup-workspace row.

**Narrow windows (builder's call within this rule):** when the page is too narrow for two panes, the tab list stacks above the selected tab's details. Pick the breakpoint so neither pane gets unusably thin.

## 6. Folder picker

- A **Choose…** button sits right of each tab's Folder field.
- It opens the **system folder picker** (Finder's on macOS, Explorer's on Windows), starting in the folder currently in the field if it exists, else the home folder.
- Picking a folder puts its path in the field; cancelling changes nothing. The text field stays editable, so pasting a path still works.
- **If the tab's Title is empty** when a folder is picked, the title becomes the folder's name (`.../frontend` gives `frontend`). A title the owner already typed is never changed.
- The field **shows paths under the home folder as `~/...`** so the end of the path (the part that tells folders apart) stays visible. Paths outside the home folder show in full (my call). What is saved follows the existing rules for folders (`ExpandFolder`, tilde handling) unchanged.
- **Must not block** the window: kaze keeps drawing and tabs keep receiving output while the picker is open. Only one picker at a time; pressing Choose… again while one is open does nothing.
- The picker result goes to the tab it was opened for, even if the owner selected another tab in the meantime; if that tab was removed, the result is dropped.

**How it gets made (my call):** use each platform's standard picker without a new third-party dependency. On macOS that can be AppleScript's `choose folder` via `osascript`, or Cocoa's `NSOpenPanel` through cgo (Gio already uses cgo on darwin); on Windows, the Explorer-style folder dialog (`IFileOpenDialog` with the pick-folders option). The builder picks the mechanism per platform, favoring the one that looks native and keeps the window responsive.

## 7. Workspaces list

Each workspace keeps its header (name, state such as "Closed · 4 tabs") and its **Run / Edit / Delete** buttons. The tab lines change:

- One row per tab: **color dot**, **title**, **folder** as `~/...`, then the commands in order joined with ` → ` (`npm install → npm run dev`).
- Long command chains **wrap** to further lines indented under the commands, instead of being cut mid-word. A single command too long for one line may be shortened at the end with `…`.
- The wait condition becomes a small **muted tag** at the end ("after Docker", "after port 8000").
- Titles, folders and commands line up in columns within one workspace.

## 8. Running group dropdown

Each member row (number, color dot, title, folder, ×, alert state) gains a **second muted line** with **what that tab runs** (its commands joined with ` → `, shortened with `…` to fit), and a **status** word at the right of that line:

| Status | When |
|---|---|
| Starting | the shell has not shown a prompt yet |
| Waiting for Docker / port 8000 | the tab's wait condition is not met yet |
| Confirm to run | the prompt timed out and the owner must choose Run commands / Use shell (existing state). Named so it does not clash with the Claude "needs you" alert in the same row (my call). |
| Running | the joined line was typed and the shell has not come back to a prompt |
| Finished | the shell reported a new prompt after the line was typed |
| Failed | the run failed before the line was typed (existing errors, e.g. the shell exited) |
| Cancelled | existing state |
| No commands | the tab has nothing to run; shown from the start, never "Finished" (my call) |

"Finished" is detected from the prompt reports kaze already counts (the OSC 7 or OSC 133 A that made the shell "ready"): a report that arrives after the line was submitted means the commands returned. It does not say whether they succeeded. Shells without the README prompt snippet never report, so they stay "Running"; that is acceptable and matches how readiness already depends on the snippet. Status updates happen when those events already wake the UI; no polling or timers are added.

Colors: Confirm to run in the theme's needs color (amber), Failed in its error color, everything else muted.

## 9. Small cleanups

- **About page:** remove the build line (commit hash and date). Keep `Version 0.6`, the platform line, the license, and both buttons. **Copy build info** still copies the full details including the build line (my call; useful for bug reports).
- **Settings sidebar footer:** show `kaze 0.6` only, without the build line.
- **About tagline:** tighten the uneven gap in `風 · "wind"` so the kanji, dot and word are evenly spaced.

## 10. Testing expectations

Strict TDD as usual. Tests should cover at least: tab selection and adding (next unused color, new tab selected), draft text surviving tab switches, error marks on list rows, the picker filling folder and empty title (with the picker itself behind a fake), a picker result arriving after its tab was removed, list wrapping instead of truncating, dropdown status for each state including Finished after a later prompt, and the About page and footer no longer showing the build line while Copy build info still includes it. Visual checks in Midnight and Washi using offscreen renders. The macOS picker is verified by hand on the owner's Mac; the Windows picker on the Windows laptop.

## 11. Status

Approved by the owner on 2026-10-30 ("lock it and implement it right away"). Calls made without the owner and marked "my call" above: Copy build info keeps the build line; "Confirm to run" naming; "No commands" status; full paths outside home; the picker mechanism.

## 12. Left to the builder

- Exact sizes, spacing, the narrow-window breakpoint, and pane widths.
- The picker mechanism per platform (section 6).
- How the row-based settings page grows a two-pane area for the editor.
- Wording of muted labels, as long as it stays plain.
