# Kaze optimization audit on macOS

Date: 2026-10-30. Revision: `c71b7c5` (main, including Plan 6a sidebar).
Scope: rendering, input latency, idle behavior, history memory, sidebar overhead, and background work. Audit only; no application optimization changes made.

## Recommendation

First investigate input-to-frame scheduling against the existing <10ms typing target. Next improve history memory efficiency and correct the Mac memory readout. The sidebar is already inexpensive in the measured cases; a sidebar rewrite has low priority.

## Measurements

Machine: Apple M3, darwin/arm64, Go 1.27.3. Native Gio app, default 1100x720 dp window, default sidebar, 14sp font, cursor blinking off. Shell: `/bin/zsh -f` with history disabled. These are development builds, not stripped release packages.

| Scenario | Result | Interpretation |
|---|---|---|
| 100 synthetic keys, 50ms spacing | p50 17.107ms, p99 18.908ms, max 19.077ms, 0 dropped | Exceeds the <10ms target; excludes OS keyboard delivery and display presentation |
| Frames during that key run | p50 0.519ms, p99 1.883ms | Typical frame construction is much cheaper than end-to-end key latency |
| Last output notification to drawn echo | p50 17.038ms, p99 18.812ms | Points toward waiting before drawing; exact cause remains untraced |
| Scroll through 10,000 history lines | 278 frames, 4.633s, p50 2.980ms, p99 4.158ms, max 4.923ms, 0 late frames | Meets the 16.7ms frame-construction budget in this run |
| Whole flood/scroll run | 569 frames, p99 4.056ms, one 178.107ms frame | Typical throughput is good; the rare stall still deserves a trace |
| Isolated-shell launch | Observed 156–294ms to first text | Not a result for the owner's full shell profile or Claude startup |
| Stable idle tail | Sampled 0.0% CPU; CPU time unchanged across consecutive final samples | No sustained idle CPU regression seen; this is a short check, not a long-duration leak test |

OS RSS peaks in these short samples: idle one tab 137MiB; idle four tabs 122MiB; key run 139MiB; one full-history/flood tab 183MiB; four full-history tabs 275MiB. Different font/cache/visibility histories make the two idle runs non-comparable: do not interpret four tabs as inherently cheaper than one. RSS includes shared resident pages and is not private footprint.

## Findings, in priority order

### 1. Typing misses the latency goal while typical drawing is fast

Evidence: all 100 key samples landed between 16.068 and 19.077ms. The benchmark recorded one intermediate drawn frame per key; output-to-drawn-echo timing was approximately the same as total latency. Typical frame construction was 0.52ms.

Relevant path: `internal/term/session.go` readLoop/notifyLoop → `internal/tabs/manager.go` notify → `app.Window.Invalidate` → `internal/ui/window.go` frame/termEvents → `internal/render/renderer.go` snapshot/Layout.

Inference: presentation cadence or an avoidable frame of waiting is a stronger candidate than expensive ordinary cell rasterization. This audit does not establish which notification or Gio scheduling boundary is responsible.

Next action: timestamp output parsing completion, notification, invalidation, frame start, input routing, snapshot, and frame submission for the same sequence number. Profile the rare slow frame separately. Aim to reduce the wait while retaining synchronized-output correctness and idle sleep. Do not introduce continuous redraws or busy polling to make the benchmark look faster.

Acceptance: repeat 100+ keys under comparable conditions; show p50/p99, frame count, late frames, and unchanged idle behavior. Keep OS/compositor latency outside the claim unless separately measured.

### 2. Dense scrollback dominates the per-session memory cost

`internal/screen/cell.go` uses a 20-byte Cell. `internal/screen/scrollback.go` retains a full-width Cell slice for each history row, including trailing blank cells. The ring reuses rows once full, which is good, but line count is the only history budget.

A bounded heap-retention test with 10,000 history rows measured:

| Width | Retained Go heap increase | Raw cell payload alone |
|---|---|---|
| 120 columns | 25.98MiB | 22.89MiB |
| 240 columns | 46.78MiB | 45.78MiB |

Rows in the test contained 119 ASCII characters, so the wider case demonstrates the cost of retaining blank tails. These are screen/parser heap measurements, excluding the native window, textures, and most font/UI state. At the accepted 100,000-line limit, a 240-column history's raw cells alone are about 458MiB per tab.

Next action: benchmark a compact history representation that retains row width/blank-cell semantics but stores only necessary spans, or interns repeated style data. Consider an optional byte budget alongside the line limit if approved. Preserve selection, copy, wide cells, resized history, color attributes, and eviction indexing. Do not optimize the live screen first unless a profile points there.

Acceptance: measure representative short logs, full-width colored output, and CJK history at fixed widths; verify copy/selection and resize tests and show memory plus scroll p99 before/after.

### 3. Mac memory reporting understates the app's process cost

`internal/ui/mem_other.go` returns no native process metric. `privateMB()` in `internal/ui/window.go` falls back to `runtime.MemStats.Sys - HeapReleased`. That is Go runtime memory, not RSS or private physical footprint; it omits native/GPU allocations.

In the completed flood run, the benchmark's `private_mb` was 88.06MiB while OS RSS peaked at 183.2MiB. These are different quantities, not evidence of a 95MiB leak. The status readout and benchmark field name should not suggest equivalent process memory across platforms.

Next action: use a native Mac process metric with a precise label (prefer physical footprint if available, otherwise explicitly labeled RSS), retain the existing low-frequency-on-frame sampling, and document what each platform reports. Avoid claiming Windows/Mac memory parity from the current values.

### 4. Sidebar/model allocations are a secondary cleanup

A warmed, headless benchmark reused the chrome/shaper and reset Gio ops each iteration:

| Sessions | Sidebar layout | Layout allocation | Build model | Model allocation |
|---|---|---|---|---|
| 4 | 12.78µs | 608B / 4 allocs | 0.493µs | 1,120B / 12 allocs |
| 16 | 28.63µs | 2,528B / 6 allocs | 1.682µs | 4,640B / 28 allocs |
| 64 | 28.81µs | 2,528B / 6 allocs | 5.902µs | 19,744B / 80 allocs |

These exclude GPU work and cold font setup. Similar 16/64-session drawing cost agrees with visible-row culling in `internal/ui/sidebar.go`. Model construction still visits every session.

`drawSide()` builds the sidebar model and calls `sideRuns()`. `statusParts()` separately calls `sideRuns()` again and asks for another Tabs slice. Several frame helpers clone/scan the same session list. At ordinary tab counts the measured cost is small.

Next action, after the latency and memory work: compute one immutable session/run snapshot per frame and reuse it between sidebar and status. Reuse scratch slices where ownership is clear. Avoid elaborate persistent caches until profiles show a need; invalidation mistakes could cost more than the allocations saved.

## Healthy behavior worth preserving

- Idle runs have no benchmark ticker when keys/type/scroll are disabled; no sustained idle CPU was observed in the sampled tail.
- Background sessions continue parsing without redrawing all their ordinary output; prompt/title/exit changes can wake the UI.
- Sidebar rows outside the visible list are culled before labels are drawn.
- Renderer glyph masks, fallback decisions, contrast fixes, and retained font-size caches have explicit bounds.
- Scrollback reuses evicted row storage once full.
- ResizeAll exits immediately when the grid size is unchanged.
- Workspace dependency contexts are cancelled when they have no waiting consumers; successful/settled runs do not need permanent polling.
- Git branch/repo lookup uses cached folder sequence state and native files rather than spawning git each frame.

## Method and limits

1. Pulled main with `git pull --ff-only origin main` and recorded revision c71b7c5.
2. Built the native app and flood generator into `/tmp/kaze-optimization-audit`. A build overlay only redirected `config.Path()` via `KAZE_AUDIT_CONFIG` to an isolated config. No owner config or HOME changes. Normal production code was not edited.
3. Used existing KAZE_BENCH instrumentation, real PTYs, zsh without user startup files, and input disabled. No global keyboard/mouse injection. Only processes started by the audit were stopped.
4. Collected ps CPU time/RSS each second and JSON benchmark summaries. Ran short 1/4-tab idle, key, flood/scroll, and full-history samples.
5. Some directly launched Mac windows became obscured and stopped emitting useful frame measurements. Initial logs without summaries were excluded from latency claims. Foreground retries completed all 100 keys and the full scroll; they ultimately exited through the benchmark's documented backstop (exit 3), so their numbers do not validate normal window-close behavior.
6. Used temporary package-local benchmark fixtures for warmed sidebar layout/model allocation and bounded history heap retention. Removed the fixtures after recording results.
7. This is not a Windows audit, a real Claude streaming replay, a full shell-profile launch test, a compositor latency measurement, or a long-duration leak test. No optimization is claimed proven without a comparable before/after run.

Raw JSON logs, process samples, benchmark text, isolated config, overlay, and fixture copies are in `/tmp/kaze-optimization-audit` for this session. Relevant existing harness documentation: `bench/README.md` and `bench/run.sh`. The stock macOS harness changes HOME for isolation; the audit used a config-path overlay instead.
