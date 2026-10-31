# Performance and resource audit

Date: 2026-11-01. Audited main commit: `68ec070ce0dadcf8fd9b29df00ae2889c248ef75`
(Kaze 0.15). Host: Apple M3, macOS arm64, Go 1.27.3.

## Assessment

The most useful near-term work is to cache chrome geometry/text measurements,
remove per-step history allocations, and prune stale agent pane identities.
Full-screen scrolling is a larger, measured throughput opportunity. The parser
and existing strip renderer do not need a speculative rewrite.

No product code was changed. Measurements used temporary benchmark/probe files
in a source snapshot, plus CPU profiles. No global input, agent model calls or
owner settings were used. These are local CPU/allocation measurements, not native
GPU/compositor latency, whole-app RSS or Windows performance measurements.

## Ranked opportunities

| Order | Finding | Evidence | Suggested work | Risk |
|---|---|---|---|---|
| 1 | Repeated status layout and text measurement on every frame | At 600 px, long demo path/branch: about 1.87 ms, 478 measurements, 93,345 B and 868 allocations per layout | Cache layout by relevant model, width, scale and font changes; separate measuring from paint-op recording; avoid linear suffix/prefix searches | Medium |
| 2 | Every tool step copies its full retained history after 200 steps | 12,288 B and one allocation per event | Reuse storage or use a fixed ring; preserve ordered views and the dropped-step count | Low to medium |
| 3 | Agent pane identity map retains forgotten agents | After 256 completed agents and parent close, all 256 identity entries remain | Prune keys when the tracker forgets agents, while retaining live identities | Low |
| 4 | Full-screen line scrolling copies the whole grid | About 58% of CPU samples in memory copying for the 120x40 synthetic log stream; throughput falls with grid area | Prototype a full-screen row rotation or another batch-aware scroll path | High |
| 5 | Saturated subagent inbox scans all queued events per incoming message | At 4,096 queued events: about 147 microseconds per session+tool enqueue, holding the inbox mutex | Track latest source/session and superseded tool entries with bounded indexes; preserve lifecycle delivery | Medium |
| 6 | Dirty-row collection allocates per changed frame | 60 dirty rows: 1,016 B and seven allocations per collection | Add a caller-owned destination buffer; reuse selection scratch too | Low |

Priorities combine likely benefit and implementation size. They are not bug
severity ratings. The pane identity issue is a real lifetime-retention problem,
but each entry is small; it does not explain gigabytes of disk use.

## 1. Chrome: avoid measuring and recording unchanged labels

Sources: `internal/ui/statusbar.go:81`, `:119`, `:134`;
`internal/ui/statusbar_draw.go:53`; `internal/ui/tabbar.go:167`.

`drawStatus` computes the complete layout on every frame, even when only the
terminal cursor or output changed. Each compaction stage starts a fresh
`statusPlace`, rebuilding maps/slices and remeasuring repeated strings.
`cutLeft` and `cutRight` test candidates one rune at a time, so long text creates
many intermediate strings and text-layout operations. The measurement callback
calls the same `label`/`sans` routines used to record drawing operations.

The CPU profile of the long-path case attributes about 55% of samples
cumulatively to `widget.Label.LayoutDetailed`. It also includes allocator/OS
work, so this is not an end-to-end frame profile.

| Synthetic busy status model | Layout time | Measurements | Allocated bytes | Allocations |
|---|---:|---:|---:|---:|
| 1600 px, short path and branch | 11.4 microseconds | 18 | 1,032 | 28 |
| 600 px, short path and branch | 97.7 microseconds | 139 | 7,168 | 208 |
| 600 px, `feature/` plus 120 characters | 471-487 microseconds | 247 | 25,456 | 415 |
| 600 px, same branch plus a roughly 250-character path | 1.87 ms | 478 | 93,345 | 868 |

A throwaway per-layout measurement memo reduced the short narrow case to about
46 microseconds, but increased bytes and allocation counts. It barely helped
the long-path case. Do not ship that map as the solution. Prefer stable model
invalidation and retained geometry, plus a measurement path that does not record
unused paint operations. Optimize truncation using font-aware measured boundaries;
do not assume all proportional-font glyph advances are equal.

The title bar also measures text every frame and publishes a newly allocated
drag-rectangle slice (`titlebar_draw.go`, `maclights.go:43`). Reuse its geometry
when unchanged as part of the same pass. This extension is source-based advice,
not a separately measured speedup.

Validation: retain narrow-width order, long/custom shortcuts, theme/DPI/font
changes, usage transitions, notices and hit-rectangle correctness. Compare
identical workloads before and after. Do not cache Gio operation handles across
`Ops.Reset` unless their lifetime is explicitly supported.

## 2. Subagent history: avoid allocating the last 200 steps repeatedly

Source: `internal/subagents/subagents.go:140`.

Once full, every `EvTool` appends a step then clones the newest 200 steps.
The warmed benchmark measured 0.8-1.1 microseconds and 12,288 allocated bytes
per event. Time per event is modest, but the allocation is unnecessary and
accumulates across busy agents.

A ring removes both allocation and whole-history copying. Reusing a contiguous
buffer is a smaller first change if callers need the existing `[]Step` API.
Test rollover, step order, waiting/resume, dropped counts and the live-pane cache.

## 3. Pane identities: prune independently of visible panes

Sources: `internal/ui/panes_agent.go:27` and `:118`.

`floats.agentID` inserts one key for each inspected subagent. Neither pane
removal nor `dropGoneAgents` deletes it. The tracker correctly forgets old agents,
but the separate map retains their session/id strings until application exit.
A bounded 256-agent probe confirmed retention after parent closure.

Prune identities whose tracker entries no longer exist. Keep ids for agents that
remain tracked, so hiding/reopening a pane preserves its identity. Verify both
Done eviction and closing the parent/source session. This cleanup must also work
while Settings or the board covers the terminal.

## 4. Streaming: optimize row movement before the parser

Sources: `internal/screen/edit.go:43`; `internal/term/session.go:243`.

Every full-screen newline at the bottom copies `(rows-1)*cols` cells. A synthetic
64 KiB chunk of short ASCII log lines, applied through the actual VT parser and
screen with a warmed 1,000-line history, measured:

| Grid | CPU time per chunk | Throughput | Steady-state allocations |
|---|---:|---:|---:|
| 120x40 | 1.94-1.95 ms | 33.5-33.9 MB/s | 0 |
| 240x80 | 7.95-7.96 ms | 8.23-8.24 MB/s | 0 |

The 120x40 profile shows `shiftRows` at about 62.5% cumulative CPU and
`runtime.memmove` at 58%. By comparison, the parser-only existing benchmarks
(with a null handler) reach about 352 MB/s ASCII and 190 MB/s mixed input, with
zero steady-state allocations. Those parser numbers exclude screen work and
are not terminal throughput.

The session reader holds its screen mutex while applying a chunk of up to
64 KiB. The large-grid result therefore also identifies a potential lock-hold
source during a flood; actual UI stalls were not measured. Profile lock waiting
in the native benchmark before changing reader chunking.

Prefer an isolated prototype of the common full-screen scroll case. Preserve
partial scroll regions, insert/delete lines, alt screen, selection anchoring,
scrollback eviction and resizing. A row-ring design changes many assumptions;
measure it before committing to a larger screen representation rewrite.

## 5. Inbox: make overload handling cheap without losing state

Sources: `internal/ui/subagents_ui.go:43` and `:55`.

When the window is not draining frames, duplicate session checks search backward
through pending events. At capacity, each insert also scans the queue to locate
a superseded tool step, then shifts the slice to delete it. The measured
4,096-event, single-agent flood costs about 147 microseconds per session+tool
pair, with zero heap allocations in this particular case.

This is an overload-path improvement, not evidence of ordinary-session slowness.
Use bounded indexes or a queue representation with cheap deletion/coalescing.
Preserve the regression tests protecting stop reports, quiet-agent resumes and
old/new conversation transitions. Do not return to dropping the oldest event
indiscriminately.

## 6. Small rendering allocations

Sources: `internal/screen/screen.go:90`, `internal/render/renderer.go:257`.

`TakeDirty` grows a fresh integer slice. Sixty dirty rows take about 185-197 ns,
1,016 B and seven allocations. Accepting renderer-owned storage would remove this
churn; selection row deduplication also allocates a scratch bool slice.

A separate snapshot-only test of an unchanged 120x40 screen measured roughly
51-53 ns at the live bottom and 29-30 microseconds while scrolled back. The
scrolled-back path copies all visible rows while holding the session lock, even
without new output. This is lower priority at that size; a content/view generation
could skip unchanged history copies if larger native profiles justify it.
Neither snapshot test includes rasterization, GPU upload or compositor time.

## Existing safeguards worth keeping

- Packed 16-byte cells, trimmed scrollback tails and reuse of evicted lines.
- Two retained font-size caches, capped glyph/coverage/contrast caches and lazy
  system-font loading. Older HANDOFF text describing 20-byte cells is stale.
- Strip-based dirty rendering, cursor images separate from strip uploads, and
  renderer release for hidden views.
- Coalesced output notifications and event-driven updates instead of idle polling.
- Unchanged-size guard in `Manager.ResizeAll`; it is not resizing every tab on
  every ordinary frame, despite the older HANDOFF backlog wording.

Build caches and temporary binaries are separate from runtime memory. Go's
cache growth after test/race/platform builds is expected; repeatedly clearing it
forces cold rebuilds. Prefer cleaning task-owned temporary artifacts and using a
bounded validation matrix rather than turning off build caching.

## Validation and scope

Passed on the audited commit:

- `go test -p 2 ./...`
- `go vet -p 2 ./...`
- `go test -race -p 2 ./internal/term ./internal/tabs ./internal/render ./internal/subagents`
- Bounded temporary benchmarks and the pane-id retention probe described above.

No Windows execution, full native frame/keypress/idle-CPU benchmark or whole-app
heap/RSS profile was performed. Existing unrelated UI test-counter races were not
claimed fixed. Benchmark values are from two short runs on one machine and are
indications, not statistical production guarantees. No projected end-to-end
speedup is claimed.

Raw logs, profiles and the temporary probe sources are retained under
`/tmp/kaze-audit-20261101`; the task-owned build cache, source snapshot and test
binaries are removed after the audit. No commits or pushes were made.

Recommended implementation order: chrome measurement/cache pass, bounded step
storage, pane-id cleanup, then a separately benchmarked scrolling prototype.
Keep inbox and dirty-buffer work as smaller follow-ups. Follow repository TDD
rules for each production change and rerun native latency benchmarks before
claiming an improvement to the user's experience.


## Implemented follow-up in PR #60

The owner requested all six fixes alongside the existing UI fixes. The matching
baseline for this follow-up is PR head `5e88eb4`, rather than the earlier main
commit. All six changes are implemented with bounded regression tests:

- Cache title/status geometry by model, dimensions, scale, locale and font
  collection. Draw fresh operations each frame. Width measurement no longer
  records paint operations, and truncation uses a measured binary search.
- Keep the 200-step chronological slice in reused storage. This retains the
  current API and still copies its small contents, but eliminates per-event
  allocation. A ring is not required for this improvement.
- Delete forgotten pane identities on parent cleanup and Done-history eviction.
- Rotate contiguous row references within scroll regions. Main/alternate buffers,
  history, edits, dirty rows and resizing preserve logical row order.
- Batch obsolete-tool compaction to make room for a burst. Retain lifecycle events
  and newest actions; source session indexes stay bounded and clear on drain.
  Under overload, older tool history may be discarded in batches before it would
  otherwise have been evicted individually. The hard event cap remains 4,096.
- Reuse renderer-owned dirty-row and selection scratch buffers, plus Mac title
  drag-rectangle storage.

Matched short benchmarks on the same Apple M3, two runs each:

| Workload | Before | After |
|---|---:|---:|
| Cold 600 px status layout with long path/branch | 1.90-2.01 ms; 93,344 B/op | 136-137 microseconds; 8,976 B/op |
| Unchanged status geometry cache hit | Not cached | 39-51 ns; 0 allocations (geometry lookup only) |
| Retained tool-history step | 12,288 B/op; 1 allocation | 0 B/op; 0 allocations |
| Saturated single-agent inbox, amortized enqueue | 147 microseconds | 94-95 ns |
| 120x40 applied log stream | 33.9 MB/s | 72.8 MB/s |
| 240x80 applied log stream | 8.22-8.24 MB/s | 51.7-52.0 MB/s |
| Dirty-row collection into retained storage | 1,016 B/op; 7 allocations | 0 B/op; 0 allocations |

The cache-hit timing excludes status-model construction and drawing. The inbox
number averages batch compaction across a sustained tool flood; it is not the
worst-case lock-hold time or the many-distinct-agent case. No whole-app latency,
GPU/compositor, native idle CPU or Windows runtime speedup is claimed.

Verification passed: full Go tests, vet, core races (screen, term, tabs, render,
subagents), targeted UI races, native macOS build and Windows amd64/arm64 builds.
The original UI fixes remain in the PR. Release/version work follows an authorized
merge per CLAUDE.md; this follow-up does not itself merge or release the PR.
