# Audit follow-up design

The owner's instruction to address the 2026-11-01 main audit authorizes this
follow-up. Keep the existing terminal/chrome appearance and idle event model.

1. Wrap subagent text without recording paint operations or repeatedly shaping
   arbitrarily long suffixes. Preserve words, Unicode boundaries, blank lines and
   indentation. Cache by actual contents, activity, state, width, font and metrics.
2. Reopened helpers start a fresh turn: clear old steps, report, dropped count and
   activity, reuse step storage, and restart elapsed time at the new start event.
3. Discover repository/branch together. Perform real filesystem lookup on bounded
   background work; retain canonical repository identity independently of HEAD.
   Results must be rejected after folder changes, closure or newer invalidation.
   Keep test-provided lookup functions deterministic. No polling when idle.
4. Reserve input capacity before copying, bounded by chunks and total bytes,
   including the active write. Use a 64 MiB + 4 KiB byte budget so one maximum
   clipboard paste plus its bracket markers fits. Reject whole messages, preserve
   Submit feedback and show a brief notice when user input is rejected. Release
   capacity on write completion and discard queued input on close/exit.
5. Repair fake alert counters for race validation and add PR test/vet/build CI.
   Refresh misleading current-state handoff notes and record matched benchmarks.

The inexpensive link scan, combining marks and Option behavior remain separate
work: the audit did not justify a renderer change or a new input feature here.
No installation, version bump, commit, push or release is authorized by this task.

## Session shortcut follow-up

The owner reported Ctrl+Tab and Ctrl+Shift+Tab failing in the macOS terminal and
authorized a PR, explicitly without merging. A native hidden-window probe shows
NSWindow consumes these combinations before GioView.keyDown. Add a local AppKit
key monitor tied to Kaze's view/window lifetime. Forward only physical Tab with
Control (and optional Shift) to that view's ordinary keyDown/keyUp methods, then
consume that event so it is delivered exactly once. Preserve all other shortcuts,
plain Tab, and events for other windows. Continue routing configurable actions in
Go, including panes and the board. Add native window-isolation/cleanup tests and
headless input-router tests. Commit/push task files and open a PR; do not merge,
install or release. Preserve unrelated local files and the owner's CLAUDE edit.
