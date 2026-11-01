# Audit follow-up implementation plan

Design: `../specs/2026-11-01-kaze-audit-followup-design.md`.

1. Add bounded regression tests for hidden same-length reports, layout metric
   invalidation, long-token wrapping work and fresh helper turns. Run RED, then
   implement and run focused GREEN checks. Retain comparable wrapping benchmarks.
2. Add tests for combined Git discovery, stable canonical identity reuse, prompt
   HEAD refresh and stale asynchronous results. Run RED, implement background
   lookup with bounded workers/cache, run GREEN and existing sidebar/status tests.
3. Add bounded tests for chunk/byte reservation, zero allocation on rejection,
   concurrent producers, close cleanup, active-write accounting and UI feedback.
   Run RED, implement, run GREEN and existing paste/echo/Submit tests.
4. Synchronize fake alert counters, add CI for tests/vet and platform builds,
   update audit/handoff and run full tests/vet, core and UI race checks and macOS/
   Windows builds. Compare bounded benchmarks without claiming native latency.

5. Reproduce macOS session shortcuts with a hidden local NSWindow, then add a
   bounded native RED/GREEN test for forwarding, modifiers, other-window isolation
   and removal. Add router tests for both session directions and overrides. Fix
   board Tab registration if covered tests reproduce its separate delivery gap.
6. Validate, commit only task changes, push this branch and open a PR. The owner
   explicitly prohibited merging; do not merge or release.
