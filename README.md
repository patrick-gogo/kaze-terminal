<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/branding/kaze-logo.svg">
    <img src="assets/branding/kaze-logo-light.svg" alt="kaze" width="360">
  </picture>
</p>

kaze is my personal terminal: a small, fast terminal with tabs, written in Go with
[Gio](https://gioui.org). I built it for working in [Claude Code](https://claude.com/claude-code)
all day, so it does a few things well (tabs, fast redraws, Japanese text, Claude Code's
symbols) and leaves out most of what a full terminal suite has. It has a small settings page for
the everyday choices, and everything it changes lives in one config file you can also edit by hand.

## Status

- **Windows:** works. This is what I use every day (Windows 11, PowerShell 7).
- **macOS:** untested. The code is written for macOS too, but the Mac milestone (running and
  benchmarking it on a real Mac) is still pending, so expect rough edges there.

## Requirements

- Go 1.27.0 or newer (see `go.mod`).
- macOS only: the Xcode Command Line Tools, because Gio uses cgo there:

  ```
  xcode-select --install
  ```

Windows needs nothing else; the Windows build uses no cgo.

## Install

With Go installed, this puts a `kaze` binary in your Go bin folder (`go env GOPATH`, then `bin`):

```
go install github.com/patrick-gogo/kaze-terminal/cmd/kaze@latest
```

On Windows, add the GUI flag so no console window opens next to kaze:

```
go install -ldflags "-H=windowsgui" github.com/patrick-gogo/kaze-terminal/cmd/kaze@latest
```

## Build and run from a clone

```
git clone https://github.com/patrick-gogo/kaze-terminal.git
cd kaze-terminal
```

Windows (`-H=windowsgui` stops a console window from opening next to kaze):

```
go build -ldflags "-H=windowsgui" -o kaze.exe ./cmd/kaze
.\kaze.exe
```

macOS:

```
go build -o kaze ./cmd/kaze
./kaze
```

For the Kaze icon in the Dock, build and open the macOS app bundle instead:

```
./tools/package-macos.sh
open dist/Kaze.app
```

The app bundle is written to `dist/Kaze.app`. A bare `./kaze` binary still works, but macOS gives it a generic executable icon.

The shell is PowerShell 7 (`pwsh`) on Windows, falling back to Windows PowerShell, and your
`$SHELL` (or `/bin/zsh`) as a login shell on macOS. Set `shell` in the config to use another;
if it fails to start, kaze falls back to the default shell and says so in the first tab. If no
shell can start at all, kaze shows the error in a message box on Windows (on macOS it prints
it to stderr) and exits.

On Windows the kaze icon (exe file, taskbar, title bar, Alt+Tab) and the version details come
from `cmd/kaze/rsrc_windows_*.syso`, which `go build` picks up by itself. They are generated
from `cmd/kaze/winres/winres.json` and `assets/branding/kaze-icon-512.png`. After changing the
icon or the version, regenerate them from `cmd/kaze`:

```
go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --out rsrc --arch amd64,arm64
```

### Smart App Control (Windows)

With Smart App Control on, Windows sometimes blocks a fresh unsigned build ("An Application
Control policy has blocked this file"). Rebuild so the binary gets a different hash, for example:

```
go build -ldflags "-H=windowsgui -s -w -X main.r=12345" -o kaze.exe ./cmd/kaze
```

Change the number and rebuild until the binary is allowed. The same thing can happen to test
binaries; re-run `go test` with `-ldflags "-X main.r=<random>"`.

## Shortcuts

| Action | Windows | macOS |
|---|---|---|
| New tab | Ctrl+Shift+T | Cmd+T |
| Close tab | Ctrl+Shift+W | Cmd+W |
| Next tab | Ctrl+Tab | Ctrl+Tab |
| Previous tab | Ctrl+Shift+Tab | Ctrl+Shift+Tab |
| Copy the selection | Ctrl+Shift+C | Cmd+C |
| Paste | Ctrl+Shift+V | Cmd+V |
| Open settings | Ctrl+, | Cmd+, |
| New line in Claude Code (without sending) | Alt+Enter | Option+Enter |

Plain Ctrl+C always goes to the shell as an interrupt. In Claude Code, Shift+Enter currently
submits like Enter, so use Alt+Enter (Option+Enter on macOS) for a new line. The tab, copy
paste and settings shortcuts can be changed under `[keys]` in the config.

## Mouse

- **Select:** drag with the left button. Copy with the copy shortcut, or turn on
  `copy_on_select` to copy when you let go. Dragging past the top or bottom edge scrolls.
- **Double-click** selects a word, **triple-click** a line. Keep dragging after the click to
  extend by words or lines.
- **Apps that use the mouse** (vim, htop and the like) get the clicks and the wheel. Hold
  **Shift** while dragging to select text in them anyway.
- **Wheel:** scrolls back through history. Typing jumps back to the bottom. Full-screen apps
  that don't use the mouse have no history to scroll.

## Settings

Open it with the gear button in the title bar (Windows) or with Ctrl+, (Cmd+, on macOS). It opens as a
tab-like chip next to your tabs and closes with its ×. It has three sections in a menu on the left:

- **Appearance:** theme, font size, line spacing, cursor shape and blinking.
- **Claude:** the alert and usage choices (chimes, taskbar flash, how long the "done" toast stays,
  the usage readout in the status bar) and a check of your Claude Code setup (below).
- **Workspaces:** saved launch recipes (see Workspaces below).

A change applies right away and is saved when you make it. kaze edits only that one key in
`config.toml` and leaves every other line and your comments as they are, so a hand-edited file
stays yours.

### Themes

| Theme | Look |
|---|---|
| Midnight | The original: soft dark blue-black with a violet accent and rounded tabs. The default. |
| Sumi | Ink-black with warm paper text and a vermilion accent; tabs get an underline. |
| Tide | Deep navy with a turquoise accent; pill-shaped tabs. |
| Washi | The light theme: warm paper background, dark ink text, indigo accent. |
| Sakura | Dark plum with soft pink accents. |

A theme sets the terminal colors and the window colors together. Colors you set under `[colors]`
still win over the theme's terminal palette.

### Claude Code setup check

The Claude section reads `~/.claude/settings.json` and shows the state of the three entries kaze
needs: the Stop hook (`notify done`), the Notification hook (`notify needs`) and the status line
(`statusline`):

| State | Meaning |
|---|---|
| OK | The entry is there and the kaze exe it points to exists. |
| missing | No kaze entry. |
| broken | A kaze entry whose exe is gone (for example after moving kaze). |
| taken | The status line is another program's. kaze never overwrites it by itself. |

**Fix** adds missing entries and repoints broken ones; **Replace** swaps another program's status
line for kaze's. Both show exactly what will change in a confirm dialog first, and write only after
you confirm. Before writing, kaze copies the file to `settings.json.bak-<date>-<time>`, and it
touches no other entry. If the file is empty or not valid JSON, kaze reports that and offers no fix.

## About

Open Settings with Ctrl+, (Cmd+, on macOS), then choose **About** for the Kaze logo, version, build, and platform. **Copy build info** copies those details for a bug report; **View on GitHub** opens the repository.

## Workspaces

A workspace is a saved launch recipe: a name and a list of tabs. Each tab has a title, a folder,
zero or more command lines, a color and something to wait for first. Running a workspace opens
its tabs in one go, each in its folder with its commands typed for you, so a project with an
API, a front end and a log tail starts with one click.

Make one under Settings > Workspaces: **+ New workspace**, then add tabs. For each tab:

- **Title** and **folder** (a text field; there is no Browse button yet).
- **Commands:** one line per row.
- **Color:** cyan, magenta, yellow, green, blue or red. It colors the tab's dot.
- **Wait for:** Nothing, Docker, or a port. The tab's commands are held until Docker is up, or
  until something answers on that port on this machine.

Nothing is saved until you press Save, and Cancel drops the draft. **Run** opens the tabs after
the ones you already have. If any of the workspace's tabs is still open, the button reads
**Go to tabs** and switches to the first one instead of opening copies.

### The tab group

A running workspace shows as one group in the tab strip, not one tab per member. The chip has a
stack of dots in the members' colors, the workspace's name and a count, like `●●●● samurai 4 ▾`.
While one member is shown it reads `samurai › admin`.

- Click the chip to open a list of the members. Pick one with a click, with the number keys 1 to
  9, or with the arrows and Enter. Esc or a click outside closes the list.
- A member that needs you rings its dot in amber and turns the chip's count amber, so you see it
  while the list is closed. "Done" works the same in green. Visiting the member clears it.
- Ctrl+Tab and Ctrl+Shift+Tab step through the members one by one, like any other tabs.
- The chip has no × of its own, so you can't close four servers by accident. Close members one at a
  time with your close-tab shortcut (Ctrl+Shift+W, Cmd+W on macOS) or the × in the list. The group
  goes away with its last member.
- Only a workspace run makes a group. You can't drag ordinary tabs into one.

### How commands are sent

A tab's commands are typed as one line, then Enter. In PowerShell the lines are joined as
`a; if ($?) { b }; if ($?) { c }`, and in zsh, bash and sh as `{ a; } && { b; } && { c; }`, so a line runs only
if the one before it worked. It runs in the tab's own shell, so a venv activation or `cd` stays in
effect, the shell stays open afterwards, and the exact line shows in the shell's history. If your
shell isn't pwsh, powershell, zsh, bash or sh and any tab of the workspace has commands, kaze
refuses the whole run and shows a status-bar message naming that tab; nothing opens. (At startup
the message goes into the usual first tab instead.)

kaze types only after the shell has shown its first prompt. It knows that from the folder report
described under "Folder and branch in the status bar" below, so add that snippet to your profile
for hands-off launching. Without it, after 3 seconds the tab shows a banner with **Run commands**
and **Use shell**: the first types the commands anyway, the second leaves the tab to you. Typing in
a tab that is still waiting cancels its commands. A workspace waiting in a minimized window starts
its commands when the window is shown again.

When a tab waits for Docker, kaze starts Docker Desktop once if it isn't running. Docker and port
waits give up after 3 minutes with a message in that tab, and the commands are not run.

### The workspaces file

Workspaces are stored in `workspaces.toml` next to `config.toml`. kaze owns its layout and
rewrites it when you save, so comments are not kept. If you edit it by hand, kaze checks it
strictly when it loads; a file with a problem is reported, and kaze never overwrites it, so fix or
delete it before saving from the settings page.

### Starting kaze with a workspace

Pick one under Settings > Workspaces (`startup_workspace` in `config.toml`), or run
`kaze --workspace "My project"`. Either way kaze opens only that workspace's tabs, without the
usual home-folder tab. `--workspace` wins when both are given. An unknown name opens a normal tab
and shows a warning.

### Known limits

- No folder Browse button yet; type the path.
- Commands can't be reordered, only added and removed.
- Closing the window drops a workspace draft you haven't saved.

## Config

kaze reads one TOML file at startup:

- Windows: `%AppData%\kaze\config.toml`
- macOS: `~/Library/Application Support/kaze/config.toml`

The file is optional. The settings page writes to it for you; restart kaze after editing it by hand.
A bad value falls back to its default and shows up as a warning in the first tab; a TOML syntax
error makes kaze use all defaults.

| Key | What it does | Default |
|---|---|---|
| `font_size` | Font size, 4 to 72 | `14` |
| `shell` | Program and arguments, e.g. `["pwsh", "-NoLogo"]` | PowerShell 7 / `$SHELL` |
| `scrollback_lines` | Lines of history per tab, up to 100000 | `10000` |
| `copy_on_select` | Copy a selection when the mouse button is released | `false` |
| `chime_needs` | Chime when Claude Code needs you (see Alerts) | `true` |
| `chime_done` | Chime when Claude Code finishes its turn | `false` |
| `chime_file` | Your own chime, a WAV file up to 2MB; `~` means your home folder | the bundled chime |
| `theme` | `midnight`, `sumi`, `tide`, `washi` or `sakura` | `midnight` |
| `line_spacing` | `tight`, `normal` or `roomy` | `normal` |
| `cursor_style` | `block`, `bar` or `underline` | `block` |
| `cursor_blink` | Blink the cursor (off keeps idle CPU at zero) | `false` |
| `taskbar_flash` | Flash the taskbar button on alerts (Windows) | `true` |
| `done_toast` | How long the "done" toast stays: `3s`, `6s`, `10s` or `until_seen` | `6s` |
| `show_usage` | Show the Claude usage readout in the status bar | `true` |
| `startup_workspace` | The workspace to run when kaze starts (chosen in Settings > Workspaces, stored as its ID) | none, a normal tab |
| `[colors]` | `fg`, `bg`, `cursor`, `selection`, `color0` to `color15`, as `"#rrggbb"` | the built-in Midnight palette |
| `[keys]` | `new_tab`, `close_tab`, `next_tab`, `prev_tab`, `copy`, `paste`, `open_settings` | the shortcuts above |
| `font_family` | **Not supported yet.** Setting it only adds a warning; kaze always uses its bundled font. | |

Shortcuts are written like `"ctrl+shift+t"`. Modifiers: `ctrl`, `shift`, `alt` (or `option`),
`cmd`, `super` (or `win`). Keys: a letter or symbol, `tab`, `enter`, `space`, `esc`,
`backspace`, `delete`, `pageup`, `pagedown`, `home`, `end`, the arrows (`up`, `down`, `left`,
`right`) and `f1` to `f12`.

Example:

```toml
font_size = 15
shell = ["pwsh", "-NoLogo"]
scrollback_lines = 20000
copy_on_select = true
chime_done = true
chime_file = "~/sounds/ding.wav"

[colors]
bg = "#0f1117"

[keys]
new_tab = "ctrl+t"
```

kaze sets `TERM=xterm-256color`, `COLORTERM=truecolor` and `TERM_PROGRAM=kaze` for its shells,
plus `KAZE_TAB_ID` with the tab's number, and `KAZE_NOTIFY` (the local address kaze listens on
for alerts) with `KAZE_NOTIFY_TOKEN` (a random secret for that kaze run). Apps you launch from a
kaze tab inherit these variables too, the same way they inherit `PATH`.

## Optional shell setup

History suggestions and Tab completion come from your shell, not from kaze, so kaze leaves your
shell configuration alone. Here is how to turn them on.

### PowerShell (Windows)

PowerShell's own prediction and menu completion work well in kaze. To turn them on, add this
to your PowerShell profile (`notepad $PROFILE`):

```powershell
if ($Host.Name -eq 'ConsoleHost' -and -not [Console]::IsOutputRedirected) {
    Set-PSReadLineOption -PredictionSource History -PredictionViewStyle ListView
    Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete
}
```

The guard keeps it quiet in scripts and other non-interactive sessions.

### zsh (macOS)

kaze starts your login shell, which is zsh on macOS (`/bin/zsh -l`). Tab completion is built into
zsh. Gray suggestions from your history need one plugin,
[zsh-autosuggestions](https://github.com/zsh-users/zsh-autosuggestions), which you install with
[Homebrew](https://brew.sh):

```sh
brew install zsh-autosuggestions
```

Then add this to the end of `~/.zshrc`:

```zsh
# Tab completion, with a menu you can move through with the arrow keys
autoload -Uz compinit && compinit
zstyle ':completion:*' menu select

# Gray suggestions from your history as you type; Right arrow or End accepts one
source "$(brew --prefix)/share/zsh-autosuggestions/zsh-autosuggestions.zsh"
```

If you use [Oh My Zsh](https://ohmyz.sh), it already runs `compinit`. Skip the `autoload` line,
keep the `zstyle` line, and either keep the `source` line or install the plugin the Oh My Zsh way:

```sh
git clone https://github.com/zsh-users/zsh-autosuggestions ${ZSH_CUSTOM:-~/.oh-my-zsh/custom}/plugins/zsh-autosuggestions
```

then add it to the plugin list in `~/.zshrc`, for example `plugins=(git zsh-autosuggestions)`.

To check it, open a new kaze tab (existing tabs keep the old settings):

1. Type the start of a command you ran before, such as `git st`. The rest (`atus`) appears in
   gray after the cursor. Press Right arrow to accept it.
2. Type `ls /Libr` and press Tab. zsh completes it to `/Library/`. Press Tab again to list what is
   inside, and once more to highlight an entry; move with the arrow keys and press Enter to pick.

The suggestion uses color 8 (bright black) from the active theme, so it stays dim but readable in
every kaze theme, including the light Washi.

## Folder and branch in the status bar

kaze shows the active tab's folder and git branch (`~\src\kaze  ⎇ main`) on the left of the
status bar. The shell has to tell kaze where it is with an OSC 7 sequence from its prompt.
The branch is read from `.git/HEAD` (no git process runs) whenever the prompt reports, when you
switch tabs, and when the kaze window comes back into focus, so a branch that Claude Code
switched shows up too.

PowerShell, in your profile:

```powershell
if (-not $global:__kazePrompt) { $global:__kazePrompt = $function:prompt }
function prompt {
    $segs = (Get-Location).ProviderPath.Split('\') | ForEach-Object {
        if ($_ -match '^[A-Za-z]:$') { $_ } else { [uri]::EscapeDataString($_) }
    }
    [Console]::Write([char]27 + ']7;file://' + $env:COMPUTERNAME + '/' + ($segs -join '/') + [char]7)
    & $global:__kazePrompt
}
```

It percent-encodes each folder name, so names with spaces, `#`, `%` or Japanese come through
whatever the console code page is. Loading it twice is safe. It works in PowerShell 7 and 5.1.

zsh, in `~/.zshrc`:

```zsh
_kaze_osc7() { printf '\e]7;file://%s%s\a' "$HOST" "${${${PWD//\%/%25}//\#/%23}// /%20}" }
autoload -Uz add-zsh-hook; add-zsh-hook precmd _kaze_osc7
```

Without this the folder and branch stay hidden. UNC paths (`\\server\share`) are not shown.

## Alerts (Claude Code hooks)

kaze marks a tab and shows a small toast when Claude Code in that tab needs you (a permission
prompt or a question) or has finished its turn. Click "Go ›" on the toast to switch to the tab.
"Needs you" plays a soft chime (unless you're already looking at that tab); "done" is silent
unless you turn on `chime_done`. While kaze is minimized, its taskbar button flashes (Windows).

kaze learns about this from Claude Code hooks that run `kaze notify`. Add them to your Claude
Code settings (`~/.claude/settings.json`), with the full path to your kaze binary:

```json
{
  "hooks": {
    "Notification": [
      {
        "matcher": "permission_prompt|elicitation_dialog|elicitation_url_dialog|agent_needs_input",
        "hooks": [
          { "type": "command", "command": "\"C:\\Users\\you\\kaze\\kaze.exe\" notify needs", "timeout": 3 }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          { "type": "command", "command": "\"C:\\Users\\you\\kaze\\kaze.exe\" notify done", "timeout": 3 }
        ]
      }
    ]
  }
}
```

If you already have hooks, add these entries next to them. On macOS the command is
`"/path/to/kaze" notify needs` (and `notify done`).

- The hooks do nothing outside kaze: `kaze notify` looks for the kaze variables, finds none
  and exits quietly, so Claude Code in another terminal is unaffected.
- `kaze notify` always exits 0 and prints nothing, so a hook can never block or slow Claude Code.
- Even without the matcher, kaze ignores the "idle for a minute" and "login finished"
  notifications.
- **Windows with Smart App Control:** after each rebuild, run `kaze.exe notify done` once by hand.
  That lets Smart App Control check the new exe right away, instead of during a hook, where the
  check can take longer than the 3 second timeout.

## Claude usage in the status bar

kaze shows Claude's 5-hour and weekly plan usage and how full the current tab's context is, at the
right of the status bar: `5h 23%  ·  wk 41%  ·  ctx 34%`. A number turns amber at 80% and red at
95%, and then also says when that limit resets.

The numbers come from Claude Code's status line. Point it at `kaze statusline` in
`~/.claude/settings.json`:

```json
{
  "statusLine": { "type": "command", "command": "\"C:\\Users\\you\\kaze\\kaze.exe\" statusline" }
}
```

On macOS the command is `"/path/to/kaze" statusline`. Claude Code allows one status line, so this
replaces any you have.

- `kaze statusline` prints the same line under Claude's prompt, so it is useful in other terminals
  too. Inside a kaze tab it also sends the numbers to kaze.
- The 5-hour and weekly numbers are for the whole account, so kaze shows the latest from any tab.
  Context is per session, so it shows only for the tab you are on, and goes away when Claude exits
  back to the shell prompt (this needs the folder snippet above).
- Plan usage appears only for Claude Pro and Max subscriptions, after the first reply in a session.
- It always exits 0 and finishes within about 300ms.

## Version

`kaze --version` prints the version and the commit and time it was built from, then exits without
opening a window. The settings page shows the version at the bottom.

`kaze --workspace "<name>"` starts kaze with that workspace's tabs (see Workspaces).

## Benchmarks

Numbers for the first milestone on Windows (key latency, Claude Code streaming, launch,
scrolling, idle CPU, memory), including the goals it missed:
[docs/benchmarks/2026-10-28-milestone-1.md](docs/benchmarks/2026-10-28-milestone-1.md).
`bench/` has the harness to run them yourself.

## Roadmap

Done:

- Plan 1: the terminal core with tabs.
- Plan 3: alerts with a chime when Claude Code needs you, kaze's own title bar on Windows, and
  the current folder and git branch in the status bar.
- Plan 4: Claude's 5-hour and weekly usage and the tab's context in the status bar.
- Plan 5a: the settings page (Appearance and Claude), five themes and the Claude Code setup check.
- Plan 5b: Workspaces (launch a saved set of tabs in one click), shown as a tab group.

Next:

- A redesign of the alert notification card (GitHub issue #9).
- Real panes, which would replace tab groups as the way a workspace is shown.
- Plan 2: child tabs for Claude Code subagents.
- The Mac milestone: run, fix and benchmark kaze on macOS.

Later:

- A browser button.
- A graph view of my notes vault.

## License

kaze's code is under the [MIT License](LICENSE). The bundled fonts in `assets/fonts`
(JetBrains Mono and a subset of Noto Sans JP) are not covered by it: each keeps its own
SIL Open Font License 1.1, in `assets/fonts/OFL-JetBrainsMono.txt` and
`assets/fonts/OFL-NotoSansJP.txt`, with sources in `assets/fonts/SOURCES.txt`.
