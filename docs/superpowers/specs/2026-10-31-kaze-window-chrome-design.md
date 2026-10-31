# kaze: window chrome redesign (title bar and status bar)

Date: 2026-10-31. Owner: Patrick. Mockups (the owner picked these on 2026-10-31):

- `docs/design/window-chrome-mockup.html`: the title bar, with the status bar below it (`?p=mac|windows&d=on|off&s=calm|busy&t=midnight|washi`).
- `docs/design/status-bar-mockup.html`: the status bar on its own; direction **A · Segments** was chosen.

Where this document and the mockups disagree, this document wins.

## 1. Intent

The owner wants the frame around the terminal to feel premium and to look **the same on macOS and Windows**. Today macOS uses the system title bar plus a row of icons at the top of the sidebar, and Windows uses Kaze's own bar. The status bar is one line of small mono text.

## 2. Goals

1. **One Kaze-drawn title bar on both platforms**, with the same layout. Only the window buttons differ: native traffic lights on the left on macOS, minimize/maximize/close on the right on Windows.
2. **The title bar names the session you're in** (repo, title, command, state) and holds a real search field that opens the palette.
3. **The sidebar's icon row moves into the title bar.** The sidebar starts at the filter.
4. **A redesigned status bar** (direction A): grouped items with icons, small meters for Claude usage, pills for agents and updates, hover details for usage.

## 3. Non-goals

- New features. Every button and readout already exists; this moves and restyles them.
- Changing what the status bar measures, or how often. Memory is still read at most every few seconds, and usage still comes only from the status line hook.
- Tabs in the title bar, or a Windows 11 snap-layout flyout (Gio can't answer `HTMAXBUTTON`, as noted in HANDOFF).
- Animations of any kind.

## 4. Anti-goals

- **Idle CPU above zero.** Hover states and the usage popover change only on pointer events. No timers.
- **Losing native window behaviour on macOS:** the traffic lights must be the real ones (hover glyphs, Option-click, full screen). Dragging the bar moves the window, and double-clicking it does what the user's macOS setting says (zoom or minimize).
- **Gradients.** The owner rejected them. Theme tokens only, so all five themes work, including the light Washi.
- **Text that wraps or overflows** in a narrow window. Items shorten or drop in a fixed order (5.3, 6.4).

## 5. Title bar

### 5.1 Layout (left to right)

The bar is **42 dp** tall on both platforms.

| Part | macOS | Windows |
|---|---|---|
| Lead strip | Traffic lights (native), sidebar toggle, then **+** (new session) at the strip's right end | K mark, sidebar toggle, **+** at the right end |
| Session crumb | State mark, repo name (muted), `/`, session title (bold), last command (magenta mono), state pill | same |
| Search field | Placeholder "Search sessions, skills, commands", key cap `⌘K` | same, key cap `Ctrl K` |
| Right buttons | Scratch shell, Agents (badge), Settings | same |
| Window buttons | none on the right | Minimize, Maximize/Restore, Close (close turns red on hover) |

- **Lead strip:** while the sidebar is shown, it is exactly as wide as the sidebar, in the sidebar's colour, with a hairline on its right edge, so the sidebar reads as one column from the top of the window. With the sidebar hidden, the strip shrinks to its controls, keeps the bar's colour, and loses **+**, which is still in the palette and has a key.
- **Session crumb:**
  - It uses the sidebar's naming rule (rename, then the command's argument, then the folder), so the bar, the sidebar and the board always agree.
  - The repo is the git repo's name, left out outside a repo.
  - The state pill shows running (accent), needs you (amber) or done (green), and nothing when idle.
  - With Settings or the board shown, the crumb reads "Settings" or "Agents".
- **Search field:**
  - It is centred in the space left between the crumb and the right buttons: up to 400 dp, at least 190 dp, shortened with an ellipsis.
  - Clicking it opens the palette, which is the only thing it does. You never type into the title bar.
  - The key cap shows the actual `palette` binding.
- **Agents button:**
  - The icon and badge as in today's sidebar row: running plus waiting, amber when anything waits, hidden at zero.
  - It opens or closes the board.
- **Window title** (the OS window list, Mission Control, the taskbar): "kaze · <session title>" as today.

### 5.2 Behaviour

- **Drag:** any empty part of the bar (and the crumb, which is not a button) moves the window on both platforms.
- **Double-click on empty bar space:**
  - On Windows it maximizes or restores, as today.
  - On macOS it follows the user's setting (`AppleActionOnDoubleClick`: zoom by default, or minimize).
- **macOS traffic lights:**
  - The real buttons, shown again on Kaze's frameless window.
  - Vertically centred in the 42 dp bar, 14 pt from the left edge, and kept there through resizes and full screen.
  - In full screen the system hides them; the lead strip then starts with the sidebar toggle.
- **Hover:** each button gets the raised fill on hover and a pressed fill while held, the same tokens as today's caption buttons.

### 5.3 Narrow windows

As the window narrows, parts give way in this order:
1. The search field shrinks to its minimum, then shows only its icon and key cap.
2. The crumb drops the command, then the repo.
3. The title shortens with an ellipsis.

The buttons never disappear.

## 6. Status bar (direction A)

### 6.1 Layout

The bar is **32 dp** tall (today 24).

Left to right:
1. **Folder:** folder icon and the `~`-shortened path. Mono, the folder colour.
2. **Branch:** branch icon, the branch name (branch colour), then `+N` (green) and `−M` (red) from the session's line counts, hidden at zero. Left out outside a repo.
3. **Gap**, then the right-hand groups, each split by a 14 dp hairline:
   - **Update:** a pill "↑ Update 0.15" (accent soft). Only while an update is out.
   - **Agents:**
     - A pill with a running ring and "N agents".
     - When anything waits it turns amber, with a dot: "N agents · 1 waiting".
     - Hidden at zero.
   - **Servers:** one dot per workspace member (green up, red down, muted otherwise), then `up/total` (mono) and the label `SERVERS`. Hidden with no running workspace.
   - **Usage:** for each of 5h, wk and ctx:
     - the label (uppercase, dim)
     - a 38 × 4 dp meter
     - the percentage (mono)
     - Meter and number turn amber at the existing warning level and red at the critical one.
     - Hidden when usage is off or unknown.
   - **Kaze:** chip icon and "152 MB", then tabs icon and "7 tabs". Notices ("new tab failed …") still show here, as today.

### 6.2 Interaction

- Clickable groups get the raised fill on hover (7 dp radius):
  - **Update** opens Settings › Updates (as today).
  - **Agents** opens the board.
  - **Servers** shows the first stopped member, else the first member (as today).
  - **Usage** opens Settings › Agents.
- Folder, branch and Kaze are not clickable and have no hover.
- **Usage details:**
  - Hovering the usage group shows a card above the bar's right side.
  - It lists 5-hour, Week and Context, each with a meter, its percentage and its reset ("resets in 2h 14m", "resets Mon 9:00", "this session").
  - A footer explains the numbers come from Claude Code's status line.
  - It closes when the pointer leaves. It's drawn by Kaze over the terminal and takes no input.

### 6.3 Today's texts

The usage reset text shown at warning levels ("(resets 3:00)") moves into the details card. The bar shows only the meters and percentages.

### 6.4 Narrow windows

Groups drop in this order:
1. tabs
2. memory
3. the servers' count and label (the dots stay)
4. usage labels (meters and numbers stay)
5. the folder, which shortens from the left

Branch, agents, update and the usage numbers stay.

## 7. Platforms

- **macOS:**
  - The window opens frameless (`app.Decorated(false)`), which gives a full-size content view.
  - A small cgo file shows the standard window buttons again and positions them. It's driven by `app.AppKitViewEvent`, which hands Kaze the window's NSView, and re-applied on resize and on full screen enter and exit.
  - Double-click calls the window's zoom or miniaturize according to the user's setting.
- **Windows:** keeps today's frameless window and caption handling (`actionFor`, `captionState`), restyled and laid out per 5.1.
- **Feasibility gate:** the macOS traffic-light step is proved first, in a throwaway build (plan Task 1). If the native buttons can't be shown and positioned reliably, the fallback is Kaze-drawn lights. They'd be three circles with hover glyphs, wired to close (Gio's `ActionClose`) and to cgo calls for miniaturize and zoom. That fallback needs the owner's yes, because hover and Option-click behaviour differ from native.

## 8. What stays the same

- Every action, key and setting.
- The sidebar contents below the filter.
- The palette, Settings, the board and the floating panes.
- The bench mode and the idle CPU budget.

## 9. Claude's calls

1. 42 dp title bar and 32 dp status bar.
2. **+** moves from beside the filter to the lead strip; the filter takes the full width.
3. The macOS sidebar rail, used while the sidebar is hidden, goes away: its buttons now live in the title bar.
4. Folder and branch in the status bar are not clickable in v1.
5. The usage reset text moves into the hover card (6.3).
6. The order things drop in narrow windows (5.3, 6.4).
