# Agent browser feasibility spike

Throwaway experiments for issue #30. These files do not add a Kaze feature,
install agent configuration, or use an everyday browser profile.

## Reproduce on macOS

Requires Node 22.12+ and an installed Google Chrome. No browser download is needed.
Run from this directory:

```sh
PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 PUPPETEER_SKIP_DOWNLOAD=1 npm ci --ignore-scripts --no-audit --no-fund
mkdir -p artifacts
xcrun clang -fobjc-arc -framework AppKit focus-monitor.m -o artifacts/focus-monitor
npm test
npm run probe > artifacts/full-run.log 2>&1
```

The scripts use `open -g -n` with a temporary Chrome profile, no startup window,
and a random loopback debugging port. All pages are served by a localhost fixture.
They use CDP input directed at fixture pages; they never send global input.
The native helper only observes foreground-application notifications. The relay
blocks explicit page activation and rewrites new targets to background creation.
It is a focus experiment, **not a security proxy**: it deliberately does not
filter foreign targets, contexts, or arbitrary CDP methods.

`--isolation` and `--mcp` run the two phases separately. `KAZE_SPIKE_CHROME`
overrides the installed browser executable. Default behavior runs both phases.
Browser profiles and this run's processes are removed in `finally`. Dependencies,
screenshots, and the native monitor binary are ignored by Git.

## Evidence

See `results/macos-arm64.json` for the completed run and
`../../docs/audits/2026-11-01-agent-browser-feasibility.md` for the interpretation.
Tool schemas and screenshots are generated into `artifacts/` but not committed.
The JSON report saves text and image byte counts, never base64 screenshot bodies.

The fixtures use deliberately fake cookies, console errors, and broken requests.
The isolation test expects to demonstrate cross-session access through an
unrestricted endpoint. A passing experiment is not a passing isolation design.

## Windows gate

Windows has not been run. The Node runner has an executable override and a Windows
launch path, but its `windowsHide` option does not prove Chrome stays unfocused.
There is no Windows foreground-window monitor in this spike. Before a product
plan can claim Windows feasibility, run the scenarios with installed Edge/Chrome
and instrument foreground-window changes, background window creation, popup
behavior, reconnect, and process cleanup on a Windows machine.

## Limits

- MCP requests use a minimal local JSON-RPC client. Real Claude Code and Codex
  model turns, setup repair, sandbox access, and agent tool selection are not tested.
- Reconnection simulates idle shutdown by closing the browser through CDP; no
  product idle timer has been built.
- A blocked focus request returns success to keep existing clients working. That
  behavior is test instrumentation, not a final UX or protocol contract.
- Native focus observations cover the tested launch, navigation, click, screenshot,
  page selection, and reconnect paths. They do not prove that arbitrary page scripts,
  popups, dialogs, downloads, or browser-version changes never take focus.
- Browser endpoints are local capabilities, not an OS sandbox. A shell-capable
  local agent could find other listening ports; the separate-instance result only
  describes what is reachable through its assigned browser connection.
- One-second idle CPU samples and warm dependency-cache startup timings are narrow
  observations, not performance guarantees. Browser RSS sums double-count shared
  pages and are not physical-footprint measurements.
