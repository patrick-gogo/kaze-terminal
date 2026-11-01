# Main follow-up code audit

Audited commit: `b16c58e3fbec56f166fe077ba486a7feb52c40ad` (main, Kaze 0.19).
Date: 2026-11-01. Host: macOS arm64, Apple M3, Go 1.27.3.

## Recommendation

Fix subagent text wrapping first, then pane cache invalidation and reopened-helper
state. These are separate from the six performance fixes in the earlier audit,
which are already present. Follow with Git lookup caching and input queue limits.
There is no evidence here that the parser or strip renderer needs replacing.

The initial audit changed no product code. Probes were supplied through a temporary Go overlay,
so the two deliberately failing regression tests do not alter the normal suite.
No global input, owner settings, network benchmarks, commits or releases were used.

## Confirmed findings

### 1. High: wrapping long unbroken text stalls the UI and creates heavy allocation

Source: `internal/ui/panes_agent.go:313`, `:332`, `:347`.

`wrapSans` measures the entire remaining word for each output line. `fitPrefix`
rebuilds every remaining rune boundary, then measures multiple prefixes. Each
measurement calls `sans`, recording paint operations for text that is only being
measured. The repeated suffix/prefix work grows sharply with input length.

This path is used for prompts and reports. Their allowed sizes are 16 KiB and
24 KiB, so the measured examples are within the accepted input limits. The cache
helps unchanged frames, but opening a pane or changing its width does this work
again on the UI goroutine.

| Synthetic input, 400 px width | Time | Allocated bytes per wrap |
|---|---:|---:|
| 2,048 repeated ASCII characters, warmed benchmark | 3.86–3.88 ms | 1.40–1.41 MB |
| 8,192 repeated ASCII characters, warmed benchmark | 194–207 ms | 179.6 MB |
| 4,096 repeated ASCII characters, fresh chrome, one iteration | 47–50 ms | 48.6 MB |

The warmed and fresh-chrome cases have different cache states and are not a
matched scaling comparison. The allocated-byte figures are cumulative heap
allocation, not peak resident memory. The larger planned sample was stopped;
no 24 KiB timing is claimed.

Suggested fix: shape/wrap text once with constrained layout, or retain measured
advances and rune boundaries. Use measurement without paint recording. Preserve
indentation, blank lines and Unicode boundaries. Add bounded long-token tests and
benchmarks before changing the implementation.

### 2. Medium: a pane can show the previous turn's report indefinitely

Source: `internal/ui/panes_agent.go:247`.

The outer wrapping cache uses text lengths, step count, state and width. It does
not compare text contents or an agent revision. The inner block cache compares
contents correctly, but the outer early return bypasses it.

Reproduced sequence: render completed helper report `alpha`; while its pane is
hidden, reopen the same helper and complete another turn with report `bravo`,
with no additional tool steps. Draw again at the same width. The pane still says
`Report|alpha`. Both reports have five bytes, and all outer key fields match.
This also fits a minimized window that drains multiple lifecycle events together.

Suggested fix: key by content/revision and layout metrics, retaining the existing
block caches. Cover a whole start/stop turn between draws with same-length text.

### 3. Medium: reopened helpers show stale actions and mixed turn metadata

Sources: `internal/subagents/subagents.go:239`; `internal/ui/sidebar_agents.go:38`.

`reopen` clears the new turn's activity counters and ended time, but retains the
old steps, dropped count, report and start time. The sidebar uses the retained
last step as the current action. A focused probe completed a helper that ran
`Bash old command`, then reopened it ten minutes later without a new tool call.
Its row immediately showed `Bash old command` and `10m`, while the new activity
summary was empty. The old report also remained in the model.

The stale current action is a confirmed display bug. Whether elapsed time should
represent helper lifetime or the current turn needs an explicit consistent rule;
the current model mixes a fresh activity summary with retained turn state.

Suggested fix: clear or separate per-turn steps/report on reopen, and explicitly
choose the time semantics. Keep reusable step capacity while clearing retained
string references. Test a second turn after a first turn with actual tool steps;
the existing reopen test's first turn has no steps and misses this case.

## Remaining optimization opportunities

### Git repository lookup still performs directory scans on the UI thread

Sources: `internal/ui/status.go:32`; `internal/gitinfo/gitinfo.go:96`, `:121`.

Every new folder report, tab switch or focus invalidation calls Git lookup from
the UI thread. On macOS and Windows, `trueCase` opens every ancestor directory;
`storedName` reads all its names before searching. Branch and repo lookup also
walk toward `.git` separately. The existing `locOf` cache avoids this on ordinary
unchanged frames, but does not avoid the cost on these events.

A temporary checkout benchmark measured `Repo` alone at 0.92–1.43 ms and about
191 KB allocated per lookup. Adding 3,000 sibling entries in its immediate
parent measured 1.95 ms and about 397 KB. These results depend on the host's
ancestor directories; they are not a universal disk cost or Windows measurement.

Recommendation: cache stable repository identity/canonical paths separately from
the changing HEAD, share discovery between Branch and Repo, and move filesystem
work off the frame path with stale-result protection. Preserve case-insensitive
grouping and worktree identity.

### Input overload copies data that it then discards; queue size is by chunks

Source: `internal/term/session.go:198`.

`enqueue` clones the payload before its nonblocking channel send. A full queue
therefore still allocates. A rejected 64 KiB chunk measured 14–19 microseconds,
65,536 bytes and one allocation per attempt. The queue holds 1,024 chunks without
an aggregate byte budget; clipboard reads allow up to 64 MiB per paste. Silent
input dropping is an existing documented limitation, not a newly discovered one.

Recommendation: reserve queue bytes before copying, release that reservation
after writing, and surface rejected user input without blocking the UI. Preserve
whole bracketed-paste messages and command Submit feedback. A preflight `len(ch)`
check alone would not solve concurrent producers or the aggregate memory budget.

### Link scanning is lower priority

Sources: `internal/render/linkstyle.go:11`; `internal/render/links.go:71`.

When any snapshot rows are queued, link detection scans all visible rows under
the session lock. Synthetic link-free grids measured 28 microseconds at 120x40
and 104–110 microseconds at 240x80, with zero steady-state allocations. Incremental
logical-line scanning could help large busy views, but this evidence puts it well
behind wrapping and filesystem work. Preserve wrapped-link boundary handling.

## Engineering improvements

- Add a PR test/vet workflow. The only repository workflow is `release.yml`; its
  PR trigger covers packaging files, and its jobs build without running Go tests.
  Ordinary Go changes currently get no test job from these checked-in workflows.
- Repair the known fake chime/flash data races in `alerts_test.go:191` and `:264`
  so UI race checks can become a reliable gate. This audit reproduced those races;
  they are test-fixture findings, not proof of a production chime/flash race.
- Refresh HANDOFF's current-state sections. It still says Kaze has never been
  built on macOS and repeats completed release/feature work in its Next list,
  despite newer sections describing verified native Mac behavior. Keep historical
  notes explicitly historical so the next review does not revive fixed findings.
- Combining marks are still dropped in `screen.Print`, and macOS Option still
  routes as Alt. These are existing correctness backlog items, not performance
  regressions or new discoveries from this audit.

## Validation and limits

- `go test ./...`: passed on the pulled main.
- `go vet ./...`: passed.
- Core race tests passed for sessionstate, tabs, term, subagents, screen and render.
- The full UI race run reproduced the documented fake chime/flash races in
  `TestOnNotifyNeverBlocksAndStaysBounded`, then hit its 60-second overall timeout
  while setting up another test. Full UI race validation is incomplete; it is not
  reported as passing. Its log is `ui-race.log`.
- Two overlay regression probes failed exactly as described in findings 2 and 3.
- Short CPU/allocation benchmarks ran twice; measurements exclude native window,
  GPU/compositor latency, Windows runtime behavior and whole-app RSS.

Temporary probe sources, overlays and logs are retained at
`/var/folders/_s/s1qd55pj36qddd8_xhz_v1cm0000gn/T/kaze-audit-main-0cjuucn3`.
The 8 KiB wrapping log is `bench.log`; smaller fresh-chrome probes are in
`wrap-bench.log`; other measurements are in `small-bench.log`.

## Implemented follow-up

The owner authorized the follow-up on 2026-11-01. Implemented on
`perf/audit-followup` from the audited main, without a commit, push or install.
Design and implementation plan are in `docs/superpowers/{specs,plans}/` under
`2026-11-01-kaze-audit-followup*`.

### Changes

- Subagent text is shaped once at a constrained width, then split using the
  shaper's line rune counts. Wrapping records no paint operations. Words, UTF-8
  and grapheme boundaries, indentation and blank lines are retained; the existing
  chrome font-coverage policy still applies. A maximum 24 KiB token is covered by
  a bounded correctness test after the optimization.
- Both wrapping caches include font collection, metric and locale changes. The
  outer cache compares actual prompt/report contents, activity and state. An
  unchanged multiline pane allocates nothing and does not rescan its blocks.
- Reopening a helper clears the old report, step strings, dropped count and
  activity, retains step capacity and restarts elapsed time for the new turn.
- Git location discovery runs off the UI goroutine, with at most two outstanding
  workers and one current request per tab. Old requests cannot overwrite newer
  folder/sequence/invalidation results or recreate a closed tab. Same-folder
  metadata remains visible during refresh. Discovery completion retries line
  counting if the original folder event preceded repository discovery.
- A shared Git lookup discovers HEAD and repository together. A mutex-protected
  512-entry path-spelling cache avoids repeated directory enumeration, checking
  each cached component's parent identity and modification time. HEAD and checkout
  membership remain fresh on events, including git init and worktree changes.
- Input reserves capacity before copying and includes the active write in both
  the 1,024-message and 64 MiB + 4 KiB payload-byte limits. A maximum clipboard
  paste plus bracket markers still fits. Full queues reject entire messages with
  zero allocation. Close/exit drain queued payloads. Only accepted input advances
  echo tracking. User input rejection shows a status notice; the palette keeps
  that notice instead of reporting that a rejected skill was typed.
- The concurrent notification stress test now uses race-safe callbacks, while
  synchronous chime/flash assertion fixtures remain unchanged. Added a macOS/
  Windows PR test, vet and build workflow with macOS races and a Windows ARM64
  cross-build. Workflow YAML parses locally; hosted CI has not run from this
  unpushed branch. HANDOFF now distinguishes historical milestones from current
  status and removes several fixed items from its stale backlog descriptions.

### Measurements

Two short runs on the same Apple M3; these are CPU/allocation microbenchmarks.
The wrapping workload matches the original 400 px repeated-ASCII probe.

| Workload | Before | After |
|---|---:|---:|
| Wrap 2 KiB unbroken text | 3.86–3.88 ms; about 1.4 MB allocated | 43–46 microseconds; about 2.5 KB |
| Wrap 8 KiB unbroken text | 194–207 ms; about 180 MB allocated | 168–170 microseconds; about 10 KB |
| Uncached Branch + Repo versus warmed combined cached lookup | 898–928 microseconds; about 192 KB | 25–28 microseconds; about 4.5 KB |
| Reject a 64 KiB input chunk | 65,536 B; 1 allocation | 0 B; 0 allocations |

The Git comparison is measured side by side in the follow-up benchmark; its
ancestor-directory cost is host-dependent. Cache hits do not eliminate filesystem
validation, and the asynchronous UI path is a separate improvement. Wrapping
timings include warm shaper-cache iterations, not only cold opening. No native
window latency, idle CPU, GPU/compositor or whole-app RSS improvement is claimed.

### Verification

Passed:

- Bounded RED then GREEN regressions for stale reports, layout invalidation,
  fresh helper turns, rejected input allocation/budgets and palette feedback.
- `go test ./...` and `go vet ./...`.
- Race checks for sessionstate, tabs, term, subagents, screen, render, gitinfo and
  the full UI suite. The full UI race run passed in about 63 seconds. Focused UI
  race checks passed again after final review changes.
- Native macOS arm64 build and Windows amd64/arm64 cross-builds.
- Workflow YAML parsing and `git diff --check`.

Final test and benchmark logs are under `/tmp/kaze-followup-K0GevT`; task-owned
temporary executables are removed after validation. No live window was opened and
no global input or owner settings were used. Windows runtime, hosted CI, installed
update/relaunch and native latency remain unverified by this follow-up. The usual
screenshots do not need retaking: the chrome layout is unchanged.

The lower-priority full-visible-grid link scan remains unchanged. Combining marks
and Option behavior also remain separate product work, as specified in the design.

## macOS session-switching follow-up

The owner reported both session-switching shortcuts failing in the macOS terminal
and requested a PR without merging. A small AppKit probe reproduced the failure:
`NSWindow.sendEvent` consumes Control+Tab and Control+Shift+Tab before the Gio view
receives `keyDown`. Direct delivery to the view produces the expected command.
There were no `next_tab` or `prev_tab` overrides in the local config.

The Mac view attachment now installs a local AppKit key monitor for these two
physical Tab combinations. It checks both event window and attached view, forwards
press/release through that view's normal Gio conversion and returns nil to prevent
duplicate delivery. Plain Tab, other modifiers/keys and other windows pass through.
Configured Go actions still determine what the keys do. Both native monitors are
removed when their attachment detaches/closes; view/window captures are weak.

`sh tools/test-session-keys.sh` compiles the exact native helper into a regression
probe. RED failed before the monitor was implemented; GREEN passed forwarding,
unchanged modifiers, exactly-once delivery, window isolation, nonmatching keys and
monitor removal. Its windows remain hidden and unfocused, with activation
prohibited; it calls only its own process's event path, never global input.
The PR's macOS CI runs the same probe. Go router tests cover next/previous cycling,
wraparound, workspace/scratch exclusion, terminal/board/Settings/pane focus and
config overrides. Existing Settings-in-cycle behavior is retained.

The owner authorized committing/pushing the scoped follow-up and opening the PR.
Do not merge, install, bump versions or publish a release from this work.
