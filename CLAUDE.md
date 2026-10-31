# kaze: notes for Claude Code

kaze is the owner's personal, lightweight terminal emulator, built around running Claude Code all day.
Go + Gio, Windows and macOS from one codebase, "Midnight" look. This repo is **private** (the owner's own).

**Start every session by reading `docs/HANDOFF.md`**: current status, decisions, known gaps, the Mac
checklist and what's next. The authority for behaviour is the spec
`docs/superpowers/specs/2026-10-28-kaze-terminal-design.md` (section 6.8 = the next plan).

## Layout

| Path | What lives there |
|---|---|
| `cmd/kaze` | main, fatal-error message box |
| `internal/vt` | escape-sequence parser (Williams state machine, allocation-free) |
| `internal/screen` | cell grid, dirty rows, scrollback ring, alt screen, resize |
| `internal/term` | Applier (vt.Handler → screen), modes/replies, Session (PTY + parser + screen, locking, notifier) |
| `internal/ptyx` | the only package that imports go-pty (ConPTY / Unix pty) |
| `internal/render` | CPU-drawn grid in 8 strips, glyph cache, box drawing, font fallback, contrast, selection |
| `internal/input` | key / paste / mouse / focus encodings |
| `internal/tabs` | tab manager, shell env (`KAZE_TAB_ID`, `TERM`, ...) |
| `internal/ui` | Gio window, tab bar, status bar, routing, selection gestures |
| `internal/config` | `config.toml` loading with warnings |
| `internal/bench`, `bench/` | in-app benchmark mode (`KAZE_BENCH`) and harness scripts |
| `spike/gio` | throwaway rendering spike; not product code |

## How we work here

- **Brainstorm before building** anything new; get the owner's yes on a written design, then a plan
  (specs in `docs/superpowers/specs/`, plans in `docs/superpowers/plans/`).
- **Strict TDD:** write the test, watch it fail (RED), then make it pass (GREEN).
- **RED tests must fail safely.** A failing test must never allocate unbounded memory or loop forever:
  bounded sizes, timeouts. (An unbounded RED test once crashed the machine.)
- **Never inject global keyboard or mouse input** (SendInput, SendKeys, posting to other windows) when
  testing the GUI. The owner works on the same machine; stray keystrokes have landed in their live
  sessions before. Test through the code paths, or use the bench mode, which drops real input by default.
- **Commits:** `type(scope): Verb ...` (e.g. `fix(render): Keep text readable on colored backgrounds`).
  **No AI attribution** of any kind (no `Co-Authored-By`, no "Generated with").
- **Never push to `main`.** Branch, push the branch, open a PR. The owner merges.
- **Private repo:** context and names are fine; **never commit secrets** (tokens, keys, passwords, credentials).
  If the repo ever goes public again, scrub first (see `docs/HANDOFF.md`, "If this repo goes public").
- **How to talk to the owner:** see `docs/HANDOFF.md`, "Working with the owner". Short version: plain language,
  TL;DR first, lead with a recommendation, one question at a time with context first, no em dashes.
- **History:** `docs/history/` holds the full Plan 1 ledger (every ruling, incident and deferred item) and key
  task reports. Check it before re-deciding something.

## Shipping a feature: lander and release

When a PR that changes what users get (a feature, a visible UI change, a user-facing fix) is merged,
follow it with a release, without being asked again. Internal changes (refactors, tests, docs, the
lander alone) don't get one.

1. **Release PR** on a `release/<version>` branch, with:
   - **Version:** bump `internal/version.Number` (and its test, RED first) and
     `CFBundleShortVersionString` in `assets/branding/macos/Info.plist`. A feature bumps the minor
     version (0.12 → 0.13); a fix-only release adds a patch number (0.13 → 0.13.1).
   - **Changelog:** a new row at the top of the full history (`docs/changelog.html`) and the lander's
     "What's new" section (`docs/index.html`, `#changelog`): the version, today's date, a short title
     and plain bullets of what users get. Keep only the three latest releases on the lander; preserve
     all entries on the changelog page. Move the "Latest" badge to the new entry on both pages.
   - **Lander:** if the feature is worth showing, add or update its card in the feature grid
     (`#sessions`) or its section. If the UI changed, say whether the screenshots need retaking
     (`docs/site-assets/SCREENSHOTS.md` has how); don't fake them.
   - **README:** keep it in step with the feature (its section and the Roadmap).
2. **Merge and tag:** after the checks pass, merge the release PR, then tag `v<version>` on `main` and push
   the tag. `.github/workflows/release.yml` builds the downloads, checksums and GitHub Release; watch it
   finish and confirm the release has `Kaze-macOS.zip`, both Windows zips and `SHA256SUMS`. Installed
   copies (0.11 and later) then offer the update in Settings, Updates.
3. **Tell the owner** the version, the release link and what to check.

The owner set this up on 2026-10-31, so a merged feature PR is the go-ahead for its release. If several
feature PRs land close together, one release can cover them all.

## Build and test

- Go version: see `go.mod` (1.27). macOS also needs the Xcode Command Line Tools (`xcode-select --install`), because Gio uses cgo on darwin.
- macOS: `go build -o kaze ./cmd/kaze && ./kaze`
- Windows: `go build -ldflags "-H=windowsgui" -o kaze.exe ./cmd/kaze`
- Release build (the one the owner runs daily, in `~\Desktop\kaze-app`): build from `main`, never a feature branch:
  `go build -trimpath -ldflags "-s -w -H=windowsgui" -o kaze.exe ./cmd/kaze` (about 14 MB; without `-s -w` it is about 19 MB).
  macOS: `go build -trimpath -ldflags "-s -w" -o kaze ./cmd/kaze`.
  Kaze is usually running, so first rename the old `kaze.exe` to `kaze-old.exe` (a running exe can be renamed),
  then build. Watch out: when `go build` overwrites an exe on Windows, it renames the old one to `kaze.exe~`
  and deletes it, so it removes any `kaze.exe~` already in that folder.
- Tests: `go test ./...` and `go vet ./...`. Run one package at a time when chasing a failure.
- Windows only: Smart App Control can block freshly built test binaries or `kaze.exe`
  ("An Application Control policy has blocked this file"). Rebuild with a changed build id,
  e.g. `go test -ldflags "-B 0x<random hex>" ./internal/<pkg>/` or `-ldflags "-X main.r=<random>"` (both change
  the test binary's hash). Retry with a new value if it is blocked again. Don't turn Smart App Control off.
