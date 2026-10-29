# Workspaces: review decisions

Date: 2026-10-29. Owner: Patrick. Reviews the [Workspaces proposal](2026-10-29-kaze-workspaces-proposal.md) (Codex) against the earlier [settings design, section 6.2](2026-10-29-kaze-settings-page-design.md). Where this document and the proposal disagree, this document wins. Everything in the proposal not changed here stands.

Mockups: `docs/design/workspaces-mockup.html` (flow) and `docs/design/tab-group-mockup.html` (tab group, variant A chosen).

## 1. A running workspace is a tab group (Patrick, variant A)

Four or five tabs per project is too much tab-strip space. Until kaze has real panes, a workspace's tabs live in **one group** in the tab strip.

- **Collapsed chip:** a stack of small dots in each member tab's color, the workspace name and a member count, for example `●●●● samurai 4 ▾`. It takes the space of one tab wherever the workspace was started.
- **Current member:** while one of its tabs is shown, the chip reads `samurai › admin` and is drawn as the active tab.
- **Dropdown:** clicking the chip opens a list of the members in recipe order: number, color dot, title, short folder, and alert state ("needs you" in amber). Pick with a click, the number keys `1` to `9`, or `↑` `↓` `Enter`; `Esc` or a click outside closes it. The list header shows the workspace name and its state (Starting / Running / Needs attention, "3 of 4").
- **Alerts:** a member that needs the owner rings its dot in amber and turns the chip's count amber, so it shows while collapsed; "done" works the same in green. Visiting the member clears it, as for normal tabs.
- **Keyboard:** `next_tab` / `prev_tab` (Ctrl+Tab / Ctrl+Shift+Tab) step through every tab in display order, entering a group's members one by one, so nothing is unreachable without the mouse.
- **Closing:** the chip has no × of its own (closing four servers by accident is costly). Members close one at a time like any tab (Ctrl+Shift+W or the × in the dropdown row). The group disappears when its last member closes; with one member left it stays a group so the name remains visible.
- **Scope:** only workspace runs create groups in this version. Ordinary tabs cannot be dragged into or out of a group.
- **Later:** real tiled and floating panes (Zellij-like) are a separate future plan. They would replace groups as the way a workspace is shown and also serve Plan 2's "related view". Not part of Plan 5b.

## 2. Shell readiness (changes proposal section 5)

- The shell is ready at the **first prompt marker** after it starts: the first OSC 7 folder report (kaze's pwsh and zsh snippets emit it from inside the prompt function, so it arrives exactly when the prompt is drawn) or a standard OSC 133 `A` (prompt start) mark if the shell sends one. No kaze-only marker.
- If neither arrives within 3 seconds, show the proposal's "Waiting for shell prompt" state with **Run commands** and **Use shell**. Never send on a timer alone (agrees with the proposal).
- The README's shell snippets stay the documented way to get automatic launching.

## 3. Several commands: one typed line (changes proposal section 5)

- A tab's command lines are typed as **one line**, joined so each runs only if the previous one succeeded, followed by one Enter:
  - PowerShell: `a; if ($?) { b }; if ($?) { c }`
  - zsh / bash: `a && b && c`
- It runs in the tab's own interactive shell, so a venv activation or `cd` stays in effect, a long-running last command never receives later lines as input, and the exact line is visible in the shell's history.
- No generated temporary scripts. Command text is the owner's own and is not re-quoted.
- A shell kaze does not know how to join for (anything but pwsh, powershell, zsh, bash, sh) can still run a workspace whose tabs have no commands; if any tab has commands, preflight refuses the whole run with a message naming that tab, and nothing opens (agrees with the proposal).
- The proposal's input-queue fix stays: delivery of the line reports accepted / full / closed and is never retried.

## 4. Smaller changes

- **Folder field:** a text field only in this version. A "Browse…" button (the system folder picker) is a later addition.
- **Text fields:** standard editing, paste and Unicode display. IME composition (typing Japanese) is not required.
- **Platform proof:** the shell-readiness and one-line tests run on Windows here; the macOS run happens on the owner's Mac before merge.
- **Owner setup:** after the build, and only on Patrick's word, create the `samurai` workspace: api (`samurai_cart_v3\backend`: `docker compose up -d`, `.\venv\Scripts\Activate.ps1`, waits for Docker), admin (`samurai_cart_v3\frontend`: `npm run dev`, waits for Docker), storefront (`samurai_cart_v3_frontend`: `npm run dev -- --port 3001`, waits for Docker), api logs (`samurai_cart_v3`: `docker compose logs -f --tail=100 api`, waits for port 8000).

## 5. Decision table (supersedes proposal section 9)

| Topic | Proposal | Decided |
|---|---|---|
| How a run shows in the tab strip | One tab per recipe tab | One tab group (variant A) |
| No prompt marker | Confirm at 3s | Same; first OSC 7 or OSC 133 A counts as the marker |
| Several commands | One same-shell script | One joined line typed after the prompt |
| Editing | Draft with Save / Cancel | Same |
| Startup reference | Stable ID | Same |
| Partial workspace | Visible count, no refill | Same |
| Folder picker | Text field plus chooser | Text field only for now |
| IME in editor fields | Required | Not required |
