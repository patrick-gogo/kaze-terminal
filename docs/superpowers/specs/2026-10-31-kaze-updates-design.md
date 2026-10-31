# Kaze updates: check, tell, install in place

Status: approved by the owner (2026-10-31).

## Why

Kaze now ships as downloads (GitHub Releases, `v` + `internal/version.Number` tags; PR #35).
People who download it should get new versions without coming back to the lander. The owner
asked for kaze to notice a new version, say so, and update with one button. They chose
**install in place**: kaze downloads the new build, swaps itself and restarts.

## Goals

- Kaze notices when a newer release exists and says so quietly.
- One button downloads it, replaces kaze and restarts into the new version.
- Works for the Mac app (`Kaze.app`) and the Windows `kaze.exe` from the release zips.

## Non-goals

- Signing or notarizing builds (still unsigned; see PR #35).
- Updating a kaze built from source (`go build`, `go install`, `dist/Kaze.app` in a clone). Those
  get the notice, but the button opens the release page instead.
- Delta updates, background silent installs, release channels or beta builds.
- Keeping sessions across the restart. Sessions end; kaze says so before it restarts.

## Anti-goals (a failure even if it works)

- Kaze wakes up or uses CPU while idle because of updates. The check is one timer wake a day.
- An update leaves the person with no working kaze (a half-replaced app or exe).
- Tests that touch the network or the owner's real install.
- A pop-up that interrupts work. The notice is passive; installing always asks first.

## How it behaves

### The check

- About 10 seconds after kaze starts, then every 24 hours while it runs, kaze asks GitHub for
  the latest release: `GET https://api.github.com/repos/patrick-gogo/kaze-terminal/releases/latest`.
  No token, nothing about the person is sent beyond what any HTTPS request carries (IP, a
  `User-Agent: kaze/<version>`). 10 second timeout; failures are silent and retried at the next
  daily check.
- The tag (`v0.11`) is compared with `version.Number` numerically, part by part (`0.10` < `0.11`
  < `0.11.1` < `1.0`). Only a strictly newer version counts. Drafts and prereleases are ignored
  (the `latest` endpoint already skips them).
- Config: `check_updates = true` by default, and a switch in Settings > About ("Check for
  updates"). Off means no request at all.

### The notice

- **Status bar:** an accent "Update 0.11" on the right, before the memory and tab count. Clicking
  it opens Settings > About.
- **Settings > About:** under the version, "Kaze 0.11 is available", a **What's new** link (the
  release page) and an **Update and restart** button. Without an update it shows "Up to date",
  or the last check's error in muted text, and a **Check now** button.
- No toast, no chime, no pop-up.

### Installing

1. **Update and restart** asks first, in the About page itself: "Kaze will close your N sessions
   and restart in 0.11." with **Update now** and **Cancel**. Running workspaces can be run again.
2. Kaze downloads the zip for its platform from the release's assets:
   `Kaze-macOS.zip` (darwin), `Kaze-Windows-x64.zip` (windows/amd64),
   `Kaze-Windows-arm64.zip` (windows/arm64). Progress shows on the button ("Downloading 40%").
   Limits: 200 MB, 5 minute timeout, HTTPS to `github.com` and its asset host only.
3. **Integrity:** the release also carries `SHA256SUMS` (added to the release workflow). Kaze
   checks the zip's SHA-256 against it and refuses a mismatch. (This catches corruption and a
   swapped asset, not a compromised repo; signing would be the step after.)
4. Unzip to a temporary folder **on the same volume** as the install, so the swap is a rename.
5. **Swap and restart:**
   - **macOS:** kaze finds its bundle from `os.Executable()` (`…/Kaze.app/Contents/MacOS/kaze`).
     Rename `Kaze.app` to `Kaze.app.old`, rename the new bundle to `Kaze.app`, start it with
     `open -n`, then quit. If the rename into place fails, rename the old one back.
   - **Windows:** rename the running `kaze.exe` to `kaze-old.exe` (a running exe can be renamed),
     move the new one to `kaze.exe`, start it, then quit. On failure, rename back.
   - The new kaze deletes `Kaze.app.old` / `kaze-old.exe` when it starts.
6. **When it can't install** (not a release build, the folder isn't writable, the download or
   check fails): the button becomes **Download** and opens the release page, with the reason in
   muted text.

### What counts as a release build

- **macOS:** running from a bundle whose `Info.plist` has the bundle id `com.patrickgogo.kaze`
  and which is not inside a git checkout's `dist/` folder.
- **Windows:** the exe's folder is not inside a git checkout (no `.git` above it), and the exe is
  named `kaze.exe`.
- Others (source builds) get the notice and the **Download** button only.

## Constraints

- Go + Gio, both platforms from one codebase; network code lives in a new package
  (`internal/update`), the UI in `internal/ui`.
- Idle stays at zero CPU: one timer wake per day; downloads report progress with an invalidate at
  most every 250 ms.
- Strict TDD. Tests use `httptest` servers and temp folders; the GitHub URL and the install path
  are injectable. No test reaches the network or the real `~/Applications`, `config.toml` or
  the running exe.
- Never inject global input when testing the GUI.
- The first release that has the updater is the first that can be updated *from*. Testing the
  whole flow for real needs two releases (for example 0.11 then 0.11.1).

## Decided by Claude (the owner can overrule)

- Check 10 s after start and then daily; on by default with a switch in About.
- The notice is the status bar and the About page only.
- Ask before restarting, inside the About page rather than a dialog.
- `SHA256SUMS` in each release, checked before installing.
- Source builds get the notice but not the in-place install.

## Left to the builder

- Exact layout of the About page additions and the status bar label, within the theme tokens.
- How the new process is started on each platform (as long as the old one exits only after the
  new one was launched).
- How the download reports progress to the UI goroutine.
