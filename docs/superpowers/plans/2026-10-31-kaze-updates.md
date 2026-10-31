# Kaze Updates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** kaze checks GitHub for a newer release, shows it in the status bar and About, and installs it in place with one confirmed click.

**Architecture:** a new `internal/update` package holds everything that touches the network or the file system (version compare, release lookup, download, checksum, unzip, swap, relaunch), all with injectable URLs, clients and paths. `internal/ui` keeps a small `updater` state on the window, starts work on goroutines and drains results each frame, like the usage reader.

**Tech Stack:** Go 1.27, Gio v0.10.3, `net/http`, `archive/zip`, `crypto/sha256`.

**Spec:** `docs/superpowers/specs/2026-10-31-kaze-updates-design.md`

## Global Constraints

- Releases: `https://api.github.com/repos/patrick-gogo/kaze-terminal/releases/latest`; tags `v` + `version.Number`.
- Assets: `Kaze-macOS.zip`, `Kaze-Windows-x64.zip`, `Kaze-Windows-arm64.zip`, `SHA256SUMS`.
- Check 10 s after start, then every 24 h; request timeout 10 s; download limit 200 MB and 5 min.
- Config key `check_updates`, default true.
- No test touches the network, the real install, `config.toml` or the running exe.
- Idle CPU stays zero: one timer wake per check; download progress invalidates at most every 250 ms.

## Review Focus

- A failed swap (rename into place fails) puts the old app or exe back.
- A zip with `../` paths or absolute paths is refused.
- A checksum mismatch or a missing `SHA256SUMS` entry refuses the install.
- A source build never gets replaced: the button opens the release page.
- Turning `check_updates` off means no request is made.

---

### Task 1: `internal/update`: versions and the release lookup

**Files:** create `internal/update/update.go`, `internal/update/update_test.go`.

**Produces:**
- `Compare(a, b string) int`: numeric, part by part, a missing part is 0, a leading `v` ignored; a part that is not a number makes the version older than any number.
- `type Release struct { Version, Page string; Assets map[string]string }` (asset name to download URL).
- `Latest(ctx context.Context, c *http.Client, url string) (Release, error)`: GETs the GitHub JSON (`tag_name`, `html_url`, `assets[].name`, `assets[].browser_download_url`), sets `User-Agent: kaze/<version>`, caps the body at 1 MB.
- `AssetName(goos, goarch string) string`: the zip for a platform, "" when there is none.

Tests: `TestCompareVersions` (0.10 < 0.11 < 0.11.1 < 1.0, v-prefix, equal), `TestLatestParsesTheRelease` (httptest), `TestLatestFailsOnBadStatus`, `TestAssetNames`.

### Task 2: `internal/update`: download, checksum, unzip

**Files:** create `internal/update/fetch.go`, `fetch_test.go`.

**Produces:**
- `Download(ctx, c *http.Client, url, dst string, limit int64, progress func(done, total int64)) error`.
- `CheckSum(sums []byte, name, file string) error`: finds `name` in a `sha256sum` style list, compares.
- `Unzip(zipFile, dir string, limit int64) error`: refuses absolute paths, `..`, symlinks; keeps the exec bit; caps the total size.

Tests: download writes the body and reports progress; a body over the limit fails and leaves no file; checksum match, mismatch and missing name; unzip keeps the exec bit; unzip refuses `../evil` and `/abs`.

### Task 3: `internal/update`: install target, swap, relaunch

**Files:** create `internal/update/install.go`, `install_test.go`.

**Produces:**
- `type Target struct { Path, Old string; App bool }`: the bundle (darwin) or exe (windows) to replace and where the old one goes.
- `FindTarget(exe, goos string) (Target, error)`: darwin needs `…/X.app/Contents/MacOS/kaze` whose `Info.plist` has `com.patrickgogo.kaze`; windows needs `kaze.exe`; either fails when a `.git` folder is above it (a source checkout).
- `Install(t Target, unzipped string) error`: finds the new bundle or exe in `unzipped`, renames `Path` to `Old`, the new one to `Path`, and renames back if that fails.
- `Cleanup(t Target)`: removes `Old`.
- `Relaunch(t Target) error`: `open -n <bundle>` on darwin, starts the exe on windows.

Tests (temp folders): `FindTarget` accepts a release bundle and refuses one inside a git checkout or with another bundle id; `Install` swaps and keeps the old copy; a failed move puts the old one back; `Cleanup` removes it.

### Task 4: config, release checksum, UI, docs

**Files:** `internal/config/config.go` (+test), `.github/workflows/release.yml`, create `internal/ui/updates.go` and `updates_test.go`, modify `settings_about.go`, `status.go` (and its drawing and click handling), `window.go`, `cmd/kaze/main.go` (cleanup at start), `README.md`, `docs/HANDOFF.md`, `internal/version` (0.11).

- `check_updates` bool, default true.
- The release workflow writes `SHA256SUMS` for the zips and uploads it.
- `window.upd`: next check time, result, error, download progress, confirm flag; `updTick(now)` returns the next wake; results arrive on a channel drained in `frame`.
- Status bar: "Update 0.11" in the accent color, right side, click opens Settings > About.
- About: "Check for updates" switch, status line, **Check now**, **Update and restart** → confirm → download, verify, unzip, install, relaunch, quit; **Download** (release page) when not installable.
- Tests use an injected `updater` (release URL, client, exe path, relaunch and quit hooks).
