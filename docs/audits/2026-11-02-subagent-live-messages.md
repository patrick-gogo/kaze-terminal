# Subagent live messages (#85)

Implemented on `feat/subagent-live-messages`, based on `feat/markdown-reader`.
Merge the reader PR first; retarget this branch's PR to main after that merge.
No release, version, changelog or tag changes.

## Behavior

- JSONL is read only for a visible Claude subagent pane. Its hook carries the parent
  transcript path and agent ID; the existing per-session identity is a fallback.
  Codex helpers never start a reader.
- Each pane owns its offset and partial-record buffer. Reads seek to the last offset,
  consume at most 256 KiB per one-second poll, and close the file immediately.
  Replaced/truncated files reset the reader. Nothing watches closed/hidden panes.
- Assistant text is cleaned with `notify.CleanReport`. Thinking blocks and tool
  results are ignored. Adjacent tool calls become one count summary between texts.
- The retained tail has at most 128 entries and 128 KiB of text. Individual text and
  reports use the existing 24 KiB cap; pending JSON records are capped at 256 KiB.
  Oversized records are skipped through their newline; partial lines wait for append.
  A hidden-earlier-messages line marks discarded history.
- Report priority is nonempty hook text, last SubagentHandback message, last assistant
  text, then "No report". Display and Copy report use the same result.
- Existing bottom-follow and scroll-up behavior is preserved. Pane layout caches
  include the transcript identity, version and availability.
- The reader's deferred Task 11 is implemented here: Cmd+click / Ctrl+click web links
  and markdown paths in pane text. Paths split across wrapped lines are not joined.
  Plain clicks raise the pane. Opening a doc stops terminal input in that frame.

## Validation

Strict RED/GREEN tests cover parsing, folded runs, thinking exclusion, report order,
cleaning, partial/oversized lines, offset continuation, missing/replaced files, caps,
visible-pane polling, Codex exclusion, fallback, scroll-follow, report copying,
changed transcript cache identity and link clicks. All fixtures use temp directories.
An invalid UTF-8 link-prefix regression was reproduced and fixed as part of Task 11.

`go test ./...`, `go vet ./...`, focused race tests, native macOS and Windows amd64
cross-builds pass. Actual Gio headless pane renders in Midnight and Washi were
visually inspected, with no visible window or global input. No owner configuration,
conversation transcripts or installed copies were modified.

## Owner checks

Use a real Claude Code subagent: watch text and folded counts arrive, scroll up as
more arrives, return to the bottom, then check the final report and Copy report.
Confirm a Handback-only completion displays its report. Check pane link clicks,
including Windows Ctrl+click. Native Windows runtime and actual CLI integration
remain unverified; cross-building and fixtures do not establish those behaviors.
