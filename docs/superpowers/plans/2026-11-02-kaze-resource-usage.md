# Kaze resource usage: implementation plan for issue #92

Status: approved by the owner on 2026-11-02; implemented locally. See
`docs/audits/2026-11-02-resource-usage.md` for verification and platform limits.

The owner approved the visual mockup on 2026-11-01 in
[issue #92](https://github.com/patrick-gogo/kaze-terminal/issues/92#issuecomment-6557188559).
The source is
[resource-usage-mockup.html](https://github.com/patrick-gogo/kaze-terminal/blob/design/resource-usage-mockup/docs/design/resource-usage-mockup.html)
on `design/resource-usage-mockup`. Preserve its typography, meters, theme colors,
rounded hover card and compact bar: `CPU [meter] 4% · 1.8 GB`.

This plan extends the existing checkout's fixes for #88, #90 and #89. After
implementation, the owner authorized committing, pushing and opening a review PR
on 2026-11-02. No installed copy is replaced. The release hold in CLAUDE.md
remains active. No version bump, release PR or publishing is included.

## Scope and implementation decisions

- Count this Kaze process and its observable descendants, including shells,
  Claude, Codex and local server processes. Do not collect command lines or
  environment variables for measurement. Do not include unrelated processes,
  remote hosts, or external shared services merely contacted by a command.
- Show Kaze itself first, then session totals. Scratch shells use their fixed
  `Tab.Parent` owner. Workspace members have no fixed parent session in the
  current model: show a stable row per member, labeled with workspace and member
  name, rather than charging a movable floating pane to the session beneath it.
  Moving a pane never changes totals. This is an explicit interpretation of the
  approved per-session breakdown; no new workspace ownership model is added.
- Every process contributes at most once. Assign descendants to the nearest
  shell root, and identify processes by PID plus creation time, not PID alone.
  Retain observed ownership while that same process remains alive, even if its
  original parent exits. Exclude an unrelated process that later reuses its PID.
  Processes that detach before the first observation cannot be reliably assigned;
  disclose that limit instead of promising complete lifetime accounting.
- If an observed owned process outlives a closed tab, keep its attribution and
  label that row as closed while it remains live. Unassigned direct children of
  Kaze get a separate Other Kaze processes row, keeping displayed totals honest.
- CPU is elapsed process user plus kernel CPU time divided by elapsed wall time
  and the machine's online logical processor count. Show that count in the card.
  100% means all logical processors busy. Clamp display to 0..100%; do not use
  GOMAXPROCS as the denominator. First readings and failed reads show unavailable,
  not a fabricated zero. Normalize each process before summing rows and totals.
- Display interval CPU values; determine amber at >=50% and red at >=80% from
  the time-weighted average of the last three valid intervals. Reset baselines
  and smoothing after minimize/resume or discontinuities, so a long hidden gap
  is not shown as current usage.
- RAM remains an absolute size with no percentage or warning thresholds. Match
  existing binary units, rounded MB below 1 GB and one decimal GB above it;
  explain in the card that 1 MB = 1,048,576 bytes. macOS uses physical footprint;
  Windows uses private committed bytes, which can include paged-out memory.
  Do not label Windows commit as resident RAM. Platform semantics appear in the
  hover footer, and sums use only measurements with those same semantics.
- Partial or denied measurements are explicitly incomplete. Show available
  subtotals marked as partial, with the unavailable count/reason in the card.
  If there is no valid value, display an unavailable marker. A process exiting
  during enumeration is normal churn, not a zero sample or a persistent error.
- No GPU collection. The issue's scoped decision is that portable per-process
  collection is not worth its overhead and complexity. This is not a claim that
  GPU use is zero: Gio/window compositing can use the GPU. The card says GPU is
  not measured; no GPU meter, fake zero or whole-machine substitute is added.

## Refresh policy clarification

The existing memory code is not a periodic timer. `statusModel` reads memory on
an already-requested frame when its cached value is older than two seconds.
A completely idle window currently does not wake every two seconds for memory.

Preserve that behavior: check the same two-second gate on existing visible
frames, submit at most one background scan, and invalidate once when a result
arrives. A completion frame cannot schedule another sample until the gate is
eligible. No ticker, sleeping polling worker or new periodic redraw is added.
Hover entry can request a sample if eligible; an idle open card does not keep
waking the application. Show sample age, and say "Refreshes on activity, at most
every 2 s; paused while minimized" instead of promising a periodic update.

Suspend scan requests explicitly when the window is minimized. An already
running native call may finish, but its result cannot trigger ongoing work while
hidden. Resume resets CPU baselines and requests a fresh sample on the next
visible frame. Benchmarks and ordinary test windows do not start real monitors
unless explicitly opted in. Closing stops the worker and suppresses late UI
callbacks. This sampling policy and the workspace-row interpretation above are
approved as part of this plan.

## Task 1: expose stable shell roots

Files: `internal/ptyx/session.go`, a small optional interface in `internal/ptyx`,
`internal/term/session.go`, focused tests in those packages.

1. RED: tests for a live shell root PID, unknown roots from fake PTYs, closed
   sessions and concurrent access. Keep test subprocesses isolated and bounded.
2. GREEN: expose the launched shell PID through an optional interface implemented
   using `session.cmd.Process.Pid`; forward it through `term.Session`. Preserve
   the existing `ptyx.Session` contract so test fakes need not all change.
3. Do not use Foreground as the root: it changes with the active job and is
   unavailable for ConPTY. Snapshot root IDs and UI labels before worker work;
   no mutable tabs or Gio objects cross into the collector.

## Task 2: process sampling and aggregation

Files: new `internal/resources` package with platform collectors, pure aggregation
and bounded fixtures/benchmarks. Use existing x/sys and native calls, no new
third-party dependency and no ps, top, PowerShell, WMI or helper shell processes.

1. RED: deterministic snapshots cover child/grandchild ownership, overlapping
   roots, scratch ownership, independent workspace rows, PID reuse, reparenting,
   closed sessions, unrelated PIDs, missing readings, first samples, process
   churn, delta resets, CPU normalization and threshold smoothing. Test memory
   units and partial totals. Use small synthetic process lists, never load tests
   that grow without bound.
2. GREEN: enumerate parent relationships once per scan. Read usage only for Kaze
   and attributed processes. Bound enumeration, retained state and work queues;
   if a scan hits a bound, report partial results. Keep only live identities and
   a three-interval smoothing history. Root/session generation changes invalidate
   stale results rather than charging them to a different tab.
3. macOS candidate API path: libproc PID enumeration and process information for
   parent/creation identity; `proc_pid_rusage` for cumulative CPU nanoseconds and
   physical footprint. Validate supported struct versions and return values
   against the installed SDK before implementation.
4. Windows candidate API path: Toolhelp process snapshot for parent IDs,
   query handles for `GetProcessTimes` and `GetProcessMemoryInfo`, and processor
   count across processor groups. Validate required rights, structure layouts,
   and 32/64-bit assumptions; close every handle on all paths. Verify identity
   before accepting usage from a process that may have exited during the scan.
5. Keep other-platform builds compiling with an explicitly unavailable provider,
   without silently substituting Go heap size for process-tree RAM.
6. Native integration tests launch only temporary helper processes and compare
   bounded snapshots/deltas. Failure to inspect protected processes must not
   require elevated privileges or fail the whole monitor.

## Task 3: one background worker and cached snapshots

Files: `internal/ui/resources.go`, `window.go`, `status.go`, relevant lifecycle
handlers and tests. Keep `privateMB` for existing benchmark reporting.

1. RED: fake clock/provider tests prove the two-second gate, no idle polling,
   one in-flight scan, no backlog, nonblocking UI request/result paths,
   minimized suppression, reset after restore, shutdown and stale-result drops.
2. GREEN: initialize monitoring only in the real app path after roots are
   available; consume immutable snapshots on the UI goroutine. Native enumeration
   and process reads stay off the UI thread. Reuse the memory cadence, not a new
   timer. Audit Gio's current minimized configuration events before wiring them.
3. Preserve ordinary test and bench isolation. Sampling never reads saved
   conversations, writes user configuration, or starts an external process.

## Task 4: status group and hover card

Files: `statusbar.go`, `statusbar_draw.go`, `chrome_cache.go`, new resource card
code and status layout/input tests.

1. RED: injected readings cover calm, amber, red, warming up, unavailable,
   partial and stale states; MB/GB width transitions; scale and font changes;
   all themes; and narrow, short and busy windows.
2. GREEN: replace the memory-only group with resources, separate from Claude
   limits. Reuse Manrope labels, JetBrains Mono values, theme colors and the
   existing meter/card geometry. Resource thresholds remain independent of the
   Claude usage package. Place it before Claude usage as in the approved mockup.
3. Collapse CPU label first, then its meter, retaining `4% · 1.8 GB`. At extreme
   widths, hide the resource group as a whole before permitting overlap. Retain
   the existing relative priority of the other status groups and test the new
   explicit drop order. Include every displayed resource value in geometry
   caching; a label, unit or availability change must invalidate the layout.
4. Hover displays Kaze resource usage, CPU/core normalization, RAM semantics,
   per-session rows, sampling age/policy, scope, partial/missing readings and
   GPU not measured. Use the same rounded card styling as Claude usage.
   Clamp the card within the window; wrap footer copy; ellipsize long labels.
   Bound the row viewport and allow scrolling for many sessions, without
   losing the totals or scope footer. Resource hover has no invented click
   action or pointer cursor suggesting one.
5. Run headless pointer/geometry tests, then render native Gio operations for
   a batched light/dark, wide/narrow and many-session inspection. No activating
   windows or global keyboard/mouse injection.

## Task 5: correctness and overhead evidence

1. Run `go test -timeout=5m ./...`, `go vet ./...`, focused race checks for
   resources/ptyx/term/UI, native macOS builds and Windows amd64/arm64 builds.
2. Measure collector time, allocations and actual sampler CPU cost with 1, 4
   and 16 isolated session roots, recording machine/core count and process
   population. Include warm/cold scans and mean/p95/max, not a single best run.
   Compare idle visible/minimized behavior with monitoring disabled/enabled.
3. Acceptance: no recurring resource-driven wakes or scans in a truly idle or
   minimized window; request/result handoff stays nonblocking; no leaked process
   handles or retained dead identities; active sampling averages below 0.5% of
   one logical core at a two-second cadence for the representative 4-session
   case. This is a target to measure, not a claimed result. If exceeded, optimize
   bounded collection and remeasure before claiming the monitor is lightweight.
4. Native Windows runtime verification is distinct from cross-compilation.
   Record missing platform evidence candidly, including minimize/resume,
   descendants, CPU normalization, memory semantics and access denial.

## Task 6: docs and reviewable handoff

Update README's status-bar description and resource units/scope. Add the audit
report with test results, measured overhead, GPU feasibility decision and native
limitations; update docs/HANDOFF.md. Screenshots showing the old memory readout
will need recapture when this feature ships. Do not fake lander screenshots.

Review the full diff against the approved mockup and scope. Preserve the earlier
uncommitted fixes and unrelated owner files. Leave implementation reviewable and
uncommitted unless the owner separately requests commit/push. Version, changelog,
lander release notes and publishing remain with the combined-release coordinator.
