# Resource usage, issue #92

Implemented locally after the owner approved the mockup and the written plan.
The current branch also contains the earlier fixes for #88, #90 and #89. The
owner authorized committing, pushing and opening a review PR on 2026-11-02. No
installed-app replacement, version bump or release is included. The combined-release
hold remains active.

## Behavior

The status bar now shows `CPU [meter] 4% · 1.8 GB` before Claude usage. CPU
normalizes against the machine's online logical processor count: 100% means all
logical cores busy. The warning color uses a time-weighted average of the last
three valid intervals, amber at 50% and red at 80%. RAM has no quota or threshold.
The CPU label and meter collapse before the group disappears at extreme widths.

The hover card shows totals, core count, sample age, units and Kaze itself followed
by session rows. Scratch shells count under their parent session. Workspace
members retain independent rows labeled by workspace/member; pane placement does
not change ownership. Observed surviving processes retain their row after a tab
closes. Other Kaze children are listed separately. Each process is counted once.

Unknown values say n/a. Asterisked subtotals and unavailable counts identify
partial results. The first CPU sample establishes a baseline. GPU is explicitly
not measured, following the approved scope: portable per-process GPU collection
was not considered worth its complexity and overhead. This is not a claim that
Gio or native compositing uses no GPU.

Rows scroll while the totals and footer stay fixed in ordinary windows. Very
short windows scroll the entire card so its scope and unavailable details remain
reachable. The card stays within the window and remains open when the pointer
moves from the status group into it; leaving closes it. It has no click action.

## Measurement and implementation

`internal/resources` separates native collection from pure ownership/delta logic.
Lists and retained identities are bounded at 16,384 processes. Identity includes
creation time, preventing an observed PID's replacement from inheriting ownership
or a CPU baseline. Observed reparented descendants retain their identity/owner;
processes detaching before observation may be missed. Remote workloads and shared
external services are outside the scope. No command lines or environment values
are read by the collector, and no shell/helper process is started for collection.

macOS uses libproc parent/start metadata and rusage physical footprint. Its CPU
counters require Mach timebase conversion on Apple Silicon. A regression first
reproduced the incorrect units and now checks the native delta against getrusage
under bounded CPU work. Apple's [Mach timebase documentation](https://developer.apple.com/documentation/kernel/mach_timebase_info_t)
describes the conversion to nanoseconds.

Windows uses a Toolhelp process snapshot, creation identity and CPU from
[GetProcessTimes](https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-getprocesstimes),
and private committed bytes from K32GetProcessMemoryInfo. GetProcessTimes reports
100 ns units; kernel and user time are summed. Processor count includes all
processor groups. Query handles close on all paths; limited-query access is tried
if full memory-query rights are denied, preserving any available CPU reading.
Memory can still be unavailable independently. The native Windows test checks
self readings and the 64-bit PROCESS_MEMORY_COUNTERS_EX layout, but has only been
cross-compiled here.

The RAM footer identifies physical footprint, including compressed memory, on
macOS and private committed bytes, possibly paged out, on Windows. Binary units
match the prior readout: MB = 1,048,576 bytes; GB = 1,024 MB. There is no RAM
percentage or physical-memory denominator.

A stable shell PID is exposed through an optional PTY interface rather than the
foreground process group. A single in-flight background job receives copied
roots/labels and returns an immutable snapshot through a one-slot channel.
Native scans run outside the UI thread. Root walks/label construction are skipped
on streaming frames unless a scan or result validation is due. Ordinary tests
and benchmarks do not start a real monitor; the real Run path opts in.

The existing two-second memory gate is reused on existing frames, with no ticker.
A completion invalidates once and cannot itself sustain polling. Minimized
configuration pauses requests and suppresses completion redraws. Resume resets
CPU baselines and rejects pre-minimize results. Shutdown suppresses late results
and callbacks; an already-entered native call is allowed to finish. An idle card
shows its sample age rather than forcing periodic refreshes.

## Verification

- `go test -timeout=5m ./...` and `go vet ./...`.
- `go test -race -timeout=5m ./internal/resources ./internal/ptyx ./internal/term ./internal/ui`.
- Focused UI resource race checks after the pointer lifecycle fix.
- Native macOS build; Windows amd64/arm64 cross-builds; Windows resource test
  binary cross-compilation.
- Deterministic tests for ownership, scratch/workspace roots, overlapping roots,
  PID reuse, cycles, reparenting, closed rows, denied identity/usage, partial
  totals, CPU normalization, weighting, discontinuity resets and MB/GB units.
- Worker tests for one in-flight scan, cadence, stale-result rejection, close,
  minimized configuration and resume. Optional idle observation waits beyond
  the two-second gate, both visible and paused, and finds no recurring scans or
  redraw requests (`KAZE_RESOURCE_BENCH=1 go test ./internal/ui -run
  '^TestResourceIdleAndMinimizedHaveNoRecurringWork$' -count=1`).
- Native macOS self/child readings, live/closed shell PID exposure, and CPU
  counter comparison against getrusage. Only temporary sleep helpers and bounded
  CPU work run in these tests; no real user agents or sessions are controlled.
- Status geometry across busy/narrow states, resource cache invalidation,
  unavailable formatting, pointer hover/leave behavior, and scrollable card
  bounds at 1600x700, 600x500 and 320x260. Detail text contrast is at least 4.5:1
  across the five themes.
- Headless Gio GPU renders inspect Midnight/Washi, 1000px/600px bars, calm/busy
  readings and many sessions, then 320x260 partial/unavailable cards at both
  scroll extremes. Temporary capture code was removed. Local probe images:
  `/tmp/kaze-resources-preview.png` and
  `/tmp/kaze-resources-short-preview.png`. These use illustrative injected
  readings, not measurements or replacement lander screenshots. No native
  window was activated and no global input was injected.

## Sampling overhead

Apple M3, 8 logical cores, approximately 710 visible system processes. Each case
uses isolated sleep processes as 1, 4 or 16 session roots. One cold scan is followed
by 50 warm scans. The opt-in test reports wall time, Go allocations and native CPU
counter deltas. Results from the corrected Mach conversion:

| Roots | Cold | Warm mean | Warm p95 | Warm maximum | Allocated/scan | CPU/scan |
|---|---|---|---|---|---|---|
| 1 | 0.502 ms | 0.390 ms | 0.490 ms | 0.660 ms | 249 KB | 0.409 ms |
| 4 | 0.699 ms | 0.398 ms | 0.485 ms | 0.604 ms | 251 KB | 0.426 ms |
| 16 | 0.694 ms | 0.446 ms | 0.556 ms | 0.627 ms | 289 KB | 0.472 ms |

At a two-second cadence the 4-root CPU cost projects to **0.0213% of one logical
core**, below the plan's 0.5% target. This is a local collector microbenchmark
extrapolation, not an end-to-end application idle-CPU or latency result. Native
Windows overhead remains unmeasured.

Reproduce with `KAZE_RESOURCE_BENCH=1 go test ./internal/resources -run
'^TestNativeSamplingOverhead$' -count=1 -v`. The monitor itself never launches
these test helpers.

## Remaining platform and shipping checks

Windows native collection, handle behavior, access denial and minimize/resume
need execution on Windows; compilation alone does not verify them. Headless and
configuration-event tests do not replace an installed-app mouse/minimize check
on macOS. No installed copy was replaced by this work.

README and handoff describe the resource readout and its limits. Existing lander
captures show the old memory-only group and will need recapture when this ships.
Release version/changelog/publishing remain with the combined-release coordinator.
