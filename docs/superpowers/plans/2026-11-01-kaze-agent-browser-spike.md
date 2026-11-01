# Agent browser feasibility experiment

The owner authorized feasibility experiments on November 1, 2026. This plan covers
the throwaway spike only, not the product implementation or a release.

1. Use a clean Chrome profile and a localhost fixture. Record exact browser,
   Node, and MCP versions. Keep all browser actions inside this profile.
2. Test separate windows with shared sign-ins, separate browser contexts, and
   separate browser instances. Probe whether each connection can enumerate or
   read the other session. Test the page WebSocket endpoint as well as the
   browser endpoint; a distinct URL must not be mistaken for isolation.
3. Connect pinned Playwright MCP and Chrome DevTools MCP through a logging
   adapter. Block explicit focus requests before they reach Chrome. Exercise
   navigation, page structure, clicks, screenshots, console errors and failed
   requests. Record startup connections and browser creation before the first
   tool call, startup latency, and reconnection after a browser restart.
4. Observe foreground application changes through native macOS notifications.
   Launch Chrome without activation, create background windows explicitly, and
   record any attempted focus operations. Do not inject global input. Do not
   claim Windows behavior without running the same experiment on Windows.
5. Save reproducible scripts, bounded assertions, machine-readable results, and
   a short findings report. Clean up only the processes and profiles created by
   this run. No changes to the installed Kaze app or user agent configuration.

A filtering proxy in this spike is not a proven security boundary. Shared-profile
isolation requires a separate design decision if the experiments cannot establish
it. The existing design permits separate contexts, but separate contexts also need
a connection that cannot control other contexts.
