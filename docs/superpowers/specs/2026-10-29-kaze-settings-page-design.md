# kaze: settings page design

Date: 2026-10-29. Owner: Patrick. Mockup: `docs/design/settings-and-themes.html` (approved by Patrick 2026-10-29: "looks good", plus the version display).

This reverses the main spec's non-goal "A settings screen. Configuration is a file." (`2026-10-28-kaze-terminal-design.md` section 3). The config file stays the source of truth; the page edits it.

## 1. Intent and why

Patrick wants to change how kaze looks and behaves without editing TOML, and to start his whole dev setup in one click instead of `~/scripts/samurai-dev.ps1` in Windows Terminal. He also wants to see which kaze build he is running, because kaze is rebuilt often. The page should feel as premium as the rest of kaze (Midnight-quality chrome), not like a form dump.

## 2. Goals (first build)

1. A settings page with three sections: **Appearance**, **Workspaces**, **Claude**, plus the **version and build** shown at the bottom of the section menu.
2. **Five built-in themes**, switchable live: Midnight (current), Sumi, Tide (both from the original look mockup), Washi (new, light) and Sakura (new).
3. **Workspaces**: named sets of tabs started in one click, replacing the samurai-dev launcher.
4. Every change applies at once (no restart) and is saved to disk.

## 3. Non-goals

- The other sections from the first suggestion: Terminal (shell, scrollback, copy on select, paste warning), Shell setup, Shortcuts, a full About page. Later.
- A theme editor or theme files. `[colors]` in the config still overrides single colors on top of the chosen theme, as today.
- Fonts other than the bundled JetBrains Mono / Noto Sans JP.
- Split panes inside a workspace (kaze has none): a workspace opens tabs.
- Importing `samurai-dev.ps1` automatically. The samurai workspace is created once, by hand, as setup (section 9). The script's greeting, migration check and window maximize are not reproduced.
- Checking GitHub for newer builds.

## 4. Anti-goals (failure even if it works)

- **Idle CPU above zero** because the settings tab or a workspace is open. No tickers except where stated (cursor blink, wait polling while a workspace is starting).
- **Losing the owner's hand edits or comments** in `config.toml` when the page saves.
- **Writing Claude Code's settings without showing the exact change first.**
- Breaking any existing config key or its meaning.
- A theme switch that needs a restart, or leaves part of the window in the old colors.

## 5. Where it lives

- **A special tab** titled "Settings" in the tab strip, with no shell behind it. At most one; opening it again focuses it. Closing it like any tab.
- **Opened by** a gear button in kaze's title bar, left of the minimize button (Windows, custom title bar), and by **Ctrl+,** (Cmd+, on macOS, where the title bar is native).
- **Layout** (as in the mockup): section menu on the left (Appearance, Workspaces, Claude, then a dim "Later: Terminal, Shell setup, Shortcuts" line, then the version block at the bottom); the chosen section on the right. Drawn in the current theme's tokens.
- It uses the mouse and keyboard (Tab / Shift+Tab between controls, Enter / Space to press, Esc leaves a text field).

## 6. Sections

### 6.1 Appearance

- **Theme**: five swatches (name, five sample colors, the accent). Click applies at once to every tab, the chrome, toasts and the status bar.
- **Font size**: stepper, 8 to 32, default 14 (config `font_size`; the file still accepts 4 to 72).
- **Line spacing**: Tight / Normal / Roomy. Normal is today's line height; the others are about 10% less / 20% more (builder picks exact values that keep box-drawing joined).
- **Cursor**: Block / Bar / Underline. Default Block (today's look).
- **Blinking cursor**: off by default. When on, it blinks only while the window is focused and the cursor's tab is shown, using timed wakes (about 530ms), so a hidden or unfocused kaze still costs nothing.

### 6.2 Workspaces

- A **workspace** has a name and an ordered list of tabs. Each tab has: **title**, **folder**, **commands** (zero or more lines, edited one line per row with "+ add line"), **color** (one of six: the theme's cyan, magenta, yellow, green, blue, red), and **wait**: none / **Docker is up** / **port N answers**.
- **Run** opens the workspace's tabs after the current ones, in order. Each tab starts the default shell in its folder; once its wait is met **and** the shell has shown its first prompt (first OSC 7 report, or 3s after start for shells without the snippet), kaze types the command lines into the shell one by one, as if the owner typed them, each followed by Enter. So the shell's profile has loaded, the shell stays open afterwards, and Ctrl+C on `npm run dev` leaves a usable prompt. While waiting, the tab shows one kaze-drawn line ("waiting for Docker…" / "waiting for port 8000…") and polls: Docker every 2s, a port every 1s, both giving up after 3 minutes with a message in that tab and the command not run.
- **Docker is up** means `docker info` succeeds. If it fails at Run, kaze starts Docker Desktop once (Windows: the per-user or machine-wide install path, whichever exists; macOS: `open -a Docker`).
- **Running a workspace that is already open** (any of its tabs still open) switches to its first open tab instead of opening duplicates.
- **Edit**: inline editor with the fields above; add, remove and move tabs up/down; rename; delete the workspace (with a confirm). **+ New workspace** starts an empty one.
- **Run a workspace when kaze starts**: None or one workspace name.
- **Starting kaze with a workspace** (the startup setting or `--workspace`) opens only that workspace's tabs, without the usual home-folder tab. `--workspace` wins over the startup setting; they never both run. An unknown name opens the normal home tab and shows a warning.
- Also runnable by **`kaze --workspace <name>`** (see 9.3) and from a small menu on the title bar's "+" button (right-click or a chevron next to it: builder's choice of the two).
- The workspace's tab dot uses its color until an alert dot replaces it.

### 6.3 Claude

- **Chime when Claude needs you** (config `chime_needs`) with a **Test** button that plays it; **Chime when Claude is done** (`chime_done`).
- **Flash the taskbar** when kaze is in the background (new key, default on, today's behavior).
- **"Done" toast stays for**: 3s / 6s (default, today's) / 10s / Until seen.
- **Show usage in the status bar** (new key, default on).
- **Claude Code setup** health, read-only by default: reads `~/.claude/settings.json` and shows, for the Stop hook, the Notification hook and the status line, whether there is a **kaze entry** (a command that runs a file named `kaze.exe` / `kaze` with `notify done`, `notify needs` or `statusline`) and which exe path it uses. States: **OK** (entry present, its exe file exists), **missing**, **broken** (its exe file is gone), **taken** (status line only: another program's command is there).
- The running exe's path is shown for comparison, but **a different path is not an error**: test builds and `go run` live elsewhere, and the owner's daily exe is the one his settings point at.
- **Fix** only (a) adds a missing kaze hook, (b) repoints a broken kaze entry, or (c) for a missing status line, adds kaze's. It uses the running exe's path and never touches non-kaze hooks. A status line that is **taken** is replaced only by a separate "Replace" button whose dialog names the command it removes. Every write shows the exact JSON change first and, after the owner confirms, copies the file to `settings.json.bak-<date-time>` before writing.

### 6.4 Version and build

- At the bottom of the section menu: the version (for example `kaze 0.5`) on one line, then `build <short commit> · <build time>` (local time, 12-hour, for example `Oct 29, 9:19 AM`), and `+ local changes` when the build had uncommitted edits.
- Commit, time and the local-changes flag come from Go's embedded build info (`runtime/debug.ReadBuildInfo`, `vcs.revision`, `vcs.time`, `vcs.modified`); when missing, show `build unknown`. The version number is a constant in the code, bumped by each plan (this one makes it 0.5).
- `kaze --version` prints the same text.

## 7. Themes

Exact starting values are in the mockup's `T` object (`docs/design/settings-and-themes.html`): background, chrome, text, dim, muted, border, accent, cursor, selection, toast background and the 16 terminal colors for each theme. Midnight's values are today's code values and must not change.

- **Tab shape** is part of the theme: rounded tops (Midnight, Washi, Sakura), underline (Sumi), pill (Tide).
- **Tide** has no glow (Gio cannot blur cheaply); its active tab gets a thin teal outline instead.
- **Every chrome color in the code gets a per-theme value**, including those not in the mockup (caption-button hover, toast border and text, folder text, needs/done dots, high-usage red). The builder derives them from each theme's tokens.
- **The minimum-contrast nudge** must pull toward the current theme's background, not Midnight's, or Washi's text goes wrong.
- **Washi is light**: alert, branch and usage colors must stay readable on its paper background (the builder darkens amber/green/red for Washi as needed; the minimum-contrast nudge already protects terminal text).
- Theme choice is stored as `theme = "midnight" | "sumi" | "tide" | "washi" | "sakura"`; an unknown name warns and uses Midnight.

## 8. Saving

- New keys are written **above** the first table header (`[colors]`, `[keys]`), or TOML would read them as part of that table.
- **Single settings** (theme, font size, line spacing, cursor, blink, chimes, flash, toast time, show usage, startup workspace) are written into `config.toml` by changing only that key's line, or appending it, so comments and everything else in the file stay as they were. If the file cannot be parsed, the page shows the error and does not write.
- **Workspaces** live in their own file, `workspaces.toml`, next to `config.toml`. The page owns it and rewrites it whole on save (comments there are not kept; the file header says so).
- Saves happen on each change, written atomically (temp file + rename).
- New config keys and their defaults are documented in the README config section.

## 9. Constraints and setup

1. Windows first (daily use); macOS must build and behave the same where it applies (Cmd+, , `open -a Docker`, no taskbar flash).
2. Everything drawn with kaze's own Gio chrome code and theme tokens; no new heavy UI dependency.
3. `kaze --workspace <name>` when kaze is already running: builder may open a second kaze window instead of reaching the running one; reaching the running one is a later improvement.
4. **Owner setup after the build** (not product code): create the `samurai` workspace in his `workspaces.toml` with four tabs: api (`samurai_cart_v3\backend`, two lines: `docker compose up -d`, then `.\venv\Scripts\Activate.ps1`), admin (`samurai_cart_v3\frontend`, `npm run dev`), storefront (`samurai_cart_v3_frontend`, `npm run dev -- --port 3001`), api logs (`samurai_cart_v3`, `docker compose logs -f --tail=100 api`); the api tab waits for Docker and brings the containers up; admin and storefront wait for Docker; api logs waits for port 8000.

## 10. Decided by Claude (Patrick can override)

- Settings as a tab, not a separate window.
- Workspaces in a separate file; single keys edited in place.
- Toast duration choices, line-spacing steps, font-size range 8 to 32 in the page.
- Blink off by default.
- Running an open workspace focuses it rather than duplicating.
- Fix button for Claude setup, behind a confirm dialog and a backup; it adds or repairs kaze entries only, and a different exe path is not treated as wrong.
- Workspace commands are lines typed into the shell after its first prompt.
- Starting with a workspace skips the usual home tab; `--workspace` wins over the startup setting.
- Version 0.5 after this work.

## 11. Left to the builder

Control styling within the theme tokens, exact spacing and sizes, the "+" menu interaction (right-click or chevron), exact line-spacing values, Washi's alert/usage color adjustments, how the waiting line is drawn, the plan split (suggested: Plan 5a = settings tab, Appearance, themes, Claude section, version; Plan 5b = Workspaces).
