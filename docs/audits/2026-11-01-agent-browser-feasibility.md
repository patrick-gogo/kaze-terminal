# Agent browser feasibility findings

Date: November 1, 2026. Scope: issue #30 feasibility experiment only.

The core workflow is feasible on macOS with existing MCP tools. The draft's
shared-browser isolation is **not established**. Stock browser endpoints expose
other sessions, including across browser contexts. A separate browser instance
per session passed the tested connection-scoping checks, but changes the draft's
one-instance design and costs more resources. Windows remains unverified.

This records a throwaway experiment, not a product implementation or an approved
architecture. No user configuration changes or app release were made.

## Environment and evidence

- Base: `b16c58e3fbec56f166fe077ba486a7feb52c40ad` (main after PR #83).
- macOS 26.6.2, Darwin 25.6.0, Apple Silicon arm64.
- Installed Chrome `156.0.8078.79`; Node `22.14.0` used by the runner.
- Playwright MCP `0.0.84`, Chrome DevTools MCP `1.12.0`, ws `8.22.1`.
- Dependencies are pinned in `spike/agent-browser/package-lock.json`. No additional
  browser was downloaded. Chrome DevTools usage statistics and CrUX are disabled.
- Full run: `spike/agent-browser/results/macos-arm64.json`. Earlier exploratory
  runs informed the harness; the saved full run is the source for numbers below.
- Harness and reproduction: `spike/agent-browser/README.md`.

## Results

| Question | Observation | Decision |
|---|---|---|
| Shared sign-ins across normal windows? | A fake cookie set in window A was visible in window B. | Works within one profile. |
| Separate windows restrict agent access? | A second CDP connection listed and read window B. Both MCP servers also listed an injected foreign window. | No. |
| Separate contexts restrict access? | Cookies were separate, but the same browser endpoint attached to and read the other context. Both MCP servers listed its page. | Contexts alone are insufficient. |
| Does a page-specific WebSocket restrict access? | It listed foreign targets and successfully attached to and read a foreign page. | No. A different endpoint path is not isolation. |
| Separate browser instances? | The assigned endpoint did not list the neighbor's targets, rejected a foreign target ID, and did not share cookies. Both MCP servers omitted the neighbor instance's page. | Feasible connection boundary; separate sign-ins required. |
| Browser starts only on first use? | Both MCP servers completed initialize and tools/list with zero CDP connections and zero Chrome launches, including a one-second idle sample. | Pass for lazy Chrome. |
| First call below three seconds? | Playwright navigation: 609 ms. DevTools page listing: 519 ms, including lazy Chrome launch and initial attachment. | Pass in this warm-cache run. |
| Normal browser tools? | Navigation, structure snapshots, filling an input, clicking Apply, PNG screenshots, console errors and failed network requests all succeeded through the relay. | Both candidates work. |
| Survive browser closure? | Same MCP process and stdio connection survived Browser.close. Next tool call launched Chrome and succeeded: 578 ms / 509 ms. | Pass for simulated idle shutdown. |
| Stay in the background? | Observer received its baseline and stayed running. Zero activation events for any of the eight test Chrome instances. | Pass only for the exercised paths with the focus guard. |
| Windows? | No Windows machine was available in this environment. | Unverified, not a pass. |

The final run completed 14 Playwright tool calls and 13 DevTools tool calls with
zero tool errors. Five bounded Node tests pass, including a real WebSocket relay
test proving lazy connection and suppression of explicit activation commands.
The local fixture contains only fake sign-in state and intentional errors.

## What the focus result actually proves

Chrome was launched using `open -g -n` with a fresh profile and
`--no-startup-window`. Pages were created using CDP's `background: true`.
The native observer uses `NSWorkspaceDidActivateApplicationNotification`; it does
not require Accessibility permissions or inject input. Only this run's Chrome
PIDs are counted, so the owner's unrelated application changes cannot be mistaken
for test-browser activation.

The relay blocks `Page.bringToFront`, `Target.activateTarget`, and window-bounds
changes, rejects legacy tunneled commands, and forces new targets into the
background. It deliberately returns success for suppressed activation requests.

Playwright's tab-select action emitted `Page.bringToFront`. DevTools emitted it
when the test explicitly requested `select_page(bringToFront=true)`. Both were
blocked before reaching Chrome. Existing browser tools therefore need a Kaze
policy for activation; background launch alone is insufficient.

This does **not** establish that arbitrary page JavaScript, popups, dialogs,
downloads, or future browser versions can never activate a window. Those need
additional coverage before making the draft's unconditional no-focus promise.
No global keyboard or mouse events were sent. All typing and clicks used CDP
targeted at this run's localhost fixture.

## Resource and lifecycle implications

Both standard MCP servers already run Node during tool discovery. The draft's
literal "Node only runs when the agent uses the browser" is not achieved by
registering either stock Node server directly.

In a one-second pre-browser idle sample, process CPU time did not advance at the
`ps` reporting resolution. Worker RSS was 131,168 KiB for Playwright and 162,800 KiB
for DevTools, about 128 and 159 MiB. These are single-process RSS observations,
not total physical memory or a long-duration idle benchmark. A running Chrome
adds its own processes and memory even though it is separate from Kaze.

Meeting the stronger startup-cost requirement would need a lightweight broker that
can expose pinned tool schemas before starting Node, or an explicitly accepted
Node worker cost. This spike does not build that broker.

Reconnection was tested by closing Chrome via CDP; an automatic idle timer and
foreground-aware idle cancellation were not implemented. The next MCP call
reconnected without restarting either server. All owned browser profiles and
processes were cleaned up after the run.

## Recommendation

Proceed to a first-version design using **a separate, lazily launched browser
instance and persistent profile per Kaze session**, with a Kaze-controlled focus
guard. This is the smallest mechanism actually validated here for limiting what
an assigned browser connection can reach. It is a proposed change to the draft,
not an approved product architecture. It trades shared sign-ins and some memory
efficiency for a simpler connection boundary.

Both tool servers cover the required functionality. Start the product plan with
Playwright MCP as the provisional candidate: it passed the same fixture checks
and its sampled idle worker RSS was lower. Pin its version. Revisit this choice
if real Claude/Codex sessions or the Windows run show incompatibilities.

Keeping one browser instance and shared sign-ins would require a substantially
more involved, session-filtering protocol broker. It would need to enforce target
ownership, attached session IDs, context access, discovery events, new pages,
browser-wide commands, and focus behavior. This spike's transparent logging relay
does not provide that boundary, and the failed stock-endpoint checks do not prove
that such a broker is impossible.

Separate instances are not an OS sandbox: a shell-capable local agent could find
other local debugging ports. This matches the draft's trusted-local-program
assumption. The successful isolation probes describe assigned-connection scope,
not protection against a malicious program with the owner's OS account.

Before implementation approval, finish the Windows background/focus run and settle
the per-session-profile and Node-startup tradeoffs. Real Claude Code and Codex
tool calls, setup-file repair, sandbox reachability, and agent tool selection also
remain integration gates; this spike used a minimal local MCP protocol client.

## Source checks

The tested installed packages and recorded schemas are the authority for this
run. Upstream documentation was checked to guide the experiments:

- [Playwright MCP](https://github.com/microsoft/playwright-mcp): CDP connection,
  separate profiles/contexts, browser tools, and programmatic server support.
- [Chrome DevTools MCP configuration](https://github.com/ChromeDevTools/chrome-devtools-mcp/blob/main/docs/configuration.md):
  WebSocket attachment, page routing, and statistics opt-out.
- [Chrome debugging profile changes](https://developer.chrome.com/blog/remote-debugging-port):
  remote debugging uses a dedicated non-default user-data directory.
- [CDP Target domain](https://chromedevtools.github.io/devtools-protocol/tot/Target/):
  target enumeration, browser contexts, attachment, and target creation.
