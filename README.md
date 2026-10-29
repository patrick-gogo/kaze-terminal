# kaze

kaze is my personal terminal: a small, fast terminal with tabs, written in Go with
[Gio](https://gioui.org). I built it for working in [Claude Code](https://claude.com/claude-code)
all day, so it does a few things well (tabs, fast redraws, Japanese text, Claude Code's
symbols) and leaves out most of what a full terminal suite has. It has no settings screen;
everything lives in one small config file.

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
| New line in Claude Code (without sending) | Alt+Enter | Option+Enter |

Plain Ctrl+C always goes to the shell as an interrupt. In Claude Code, Shift+Enter currently
submits like Enter, so use Alt+Enter (Option+Enter on macOS) for a new line. The tab, copy
and paste shortcuts can be changed under `[keys]` in the config.

## Mouse

- **Select:** drag with the left button. Copy with the copy shortcut, or turn on
  `copy_on_select` to copy when you let go. Dragging past the top or bottom edge scrolls.
- **Double-click** selects a word, **triple-click** a line. Keep dragging after the click to
  extend by words or lines.
- **Apps that use the mouse** (vim, htop and the like) get the clicks and the wheel. Hold
  **Shift** while dragging to select text in them anyway.
- **Wheel:** scrolls back through history. Typing jumps back to the bottom. Full-screen apps
  that don't use the mouse have no history to scroll.

## Config

kaze reads one TOML file at startup:

- Windows: `%AppData%\kaze\config.toml`
- macOS: `~/Library/Application Support/kaze/config.toml`

The file is optional. Restart kaze after editing it. A bad value falls back to its default and
shows up as a warning in the first tab; a TOML syntax error makes kaze use all defaults.

| Key | What it does | Default |
|---|---|---|
| `font_size` | Font size, 4 to 72 | `14` |
| `shell` | Program and arguments, e.g. `["pwsh", "-NoLogo"]` | PowerShell 7 / `$SHELL` |
| `scrollback_lines` | Lines of history per tab, up to 100000 | `10000` |
| `copy_on_select` | Copy a selection when the mouse button is released | `false` |
| `chime_needs` | Chime when Claude Code needs you (see Alerts) | `true` |
| `chime_done` | Chime when Claude Code finishes its turn | `false` |
| `chime_file` | Your own chime, a WAV file up to 2MB; `~` means your home folder | the bundled chime |
| `[colors]` | `fg`, `bg`, `cursor`, `selection`, `color0` to `color15`, as `"#rrggbb"` | the built-in Midnight palette |
| `[keys]` | `new_tab`, `close_tab`, `next_tab`, `prev_tab`, `copy`, `paste` | the shortcuts above |
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

PowerShell's own prediction and menu completion work well in kaze. To turn them on, add this
to your PowerShell profile (`notepad $PROFILE`):

```powershell
if ($Host.Name -eq 'ConsoleHost' -and -not [Console]::IsOutputRedirected) {
    Set-PSReadLineOption -PredictionSource History -PredictionViewStyle ListView
    Set-PSReadLineKeyHandler -Key Tab -Function MenuComplete
}
```

The guard keeps it quiet in scripts and other non-interactive sessions.

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

Next:

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
