# Performance fixes bundled with PR #60

Date: 2026-11-01. The owner requested the audit fixes on the existing UI PR.
Baseline: `5e88eb4` on `fix/chrome-polish`. No visual redesign or new user action.

1. Write bounded failing tests for allocation-free retained step history,
   forgotten pane identities, reusable dirty-row storage, unchanged chrome
   layout reuse/invalidation, row storage reuse during scrolling, and bounded
   inbox ordering under tool floods.
2. Keep chronological step slices with reusable storage; prune identities only
   when their tracker entry is gone. Reuse dirty/selection scratch buffers.
3. Cache only geometry, keyed by content, dimensions, metric and font collection.
   Continue drawing fresh operations/colors each frame. Measure cold truncation
   efficiently while preserving Unicode and the documented compaction order.
4. Rotate row references within scroll regions instead of copying every cell.
   Preserve contiguous row backing, main/alt buffers, resizing, history and edits.
   Keep the change only if behavior tests and matching throughput benchmarks pass.
5. Batch inbox compaction under overload, preserving lifecycle events and each
   agent's newest action; keep session transitions ordered and memory bounded.
6. Run full tests, vet, focused races, macOS/Windows builds and the matching audit
   benchmarks. Document measured results and limitations; update PR #60 without
   merging it. Clean task-owned temporary build artifacts after verification.

Every production behavior follows RED then GREEN. No global input, owner config
changes or real agent calls. Keep the owner's unrelated files and UI commit.
