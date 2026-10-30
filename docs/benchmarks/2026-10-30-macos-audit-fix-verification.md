# macOS audit fix verification

Verified 2026-10-30 on Apple M3, Go 1.27.3. Compared baseline `c71b7c5` with local branch `perf/mac-audit-fixes` at `f87a795` (four commits). Verification used detached worktrees; application source and the owner’s checkout were left unchanged.

## Result

The changes materially improve typical key latency and history memory. Native memory reporting and the shared per-frame workspace snapshot are implemented correctly in the reviewed paths. One history memory gap remains, and tail key latency still exceeds the original sub-10 ms goal.

| Measurement | Baseline | After fixes |
|---|---:|---:|
| Synthetic key latency, median | 17.107 ms | 2.739 ms |
| Synthetic key latency, p99 | 18.908 ms | 17.122 ms |
| Dropped key samples | 0 / 100 | 0 / 100 |
| 10,000 history rows, 119 characters, 120 columns | 25.98 MiB | 19.88 MiB |
| Same text, 240 columns | 46.78 MiB | 19.94 MiB |
| Cell size | 20 bytes | 16 bytes |
| Scroll-only frame duration, median | 2.980 ms | 3.087 ms |
| Scroll-only frame duration, p99 | 4.158 ms | 7.822 ms |
| Scroll-only frames over budget | 0 / 278 | 0 / 278 |

These are single before/after runs on an actively used Mac, not repeated controlled trials. The higher scroll p99 deserves another controlled run before calling it a regression; it remains within the frame budget. The key benchmark collected all 100 samples, although that process later reached the benchmark backstop (exit 3); this run does not establish normal application shutdown. The flood/scroll process exited normally (exit 0).

## Remaining finding: evicted history retains oversized storage

`internal/screen/scrollback.go`, `scrollback.push`, reuses an evicted line whenever its capacity is large enough. Trimming the new slice length does not release its old backing allocation.

Bounded reproduction at 240 columns and a 10,000-line history:

1. Start with an empty history and scroll in 10,000 five-character lines: 50,000 stored cells, 50,000 capacity cells, 0.76 MiB cell payload.
2. Start with 10,000 lines of 239 characters, then replace every history entry with a five-character line: the same 50,000 stored cells, but 2,560,000 capacity cells, retaining 39.06 MiB cell payload.

This is bounded retention, not unbounded growth. It means the compact-history benefit depends on what occupied the buffer earlier. Consider a threshold that replaces substantially oversized slices on eviction, and a regression test covering long lines replaced by short or blank lines. Avoid unconditional reallocation, which would undo the ring’s allocation reuse benefit.

## Checks

Passed:

- `go vet ./...`
- `go test -race ./internal/screen ./internal/term`
- Targeted UI race tests: `TestTypedKeyEchoShowsInTheFrameThatSentIt`, `TestProcessMBCountsNativeMemory`, `TestStatusBarUsesTheFramesRuns`
- Native macOS executable build
- Windows amd64 cross-build with cgo disabled
- Bounded history footprint and retained-capacity reproductions

`go test ./...` is not green. The failures observed below were reproduced unchanged at baseline `c71b7c5`, so they are not introduced by these four fixes:

- `internal/ptyx`: `TestResolveFindsTheProgramOnPath`
- `internal/ui`: `TestSideFolderIsTildeShortened`, `TestNextPrevFollowSidebarOrder`, `TestGroupHeaderFolds`
- `internal/ui`: `TestAlertRereadsABackgroundBranch` panics indexing an empty group list; the panic prevents the remainder of that package’s suite from completing.

The sidebar tests use Windows-style paths, which do not have the same path semantics on macOS. The suite still needs portable fixtures and a guard against the indexing panic before full macOS validation can be claimed.

## Implementation observations

- The 3 ms echo wait applies only to frames that sent session input. It improves the common echo path without adding an idle wait. It does not guarantee every echo arrives in the sending frame, as the p99 shows.
- The macOS memory readout now uses the process physical footprint through `proc_pid_rusage`. This includes native memory, unlike Go heap statistics. Physical footprint and RSS are different metrics; their values need not match.
- Sidebar and status drawing share the workspace run snapshot for the frame. The targeted snapshot test passes.

Raw benchmark logs, baseline failure reproduction and memory measurements are under `/tmp/kaze-audit-verification/`. Temporary memory test fixtures are preserved there for reproduction. No application fixes, commits or pushes were made during verification.
