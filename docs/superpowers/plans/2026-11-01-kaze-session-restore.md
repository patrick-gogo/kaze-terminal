# Issue #75 implementation plan

Design approved by owner on 2026-11-01. Proceeding under the owner's instruction to implement.

1. Add a bounded versioned snapshot, atomic file replacement, OS lifetime lock and a single coalescing writer. RED tests: round trip, invalid state, failed replacement, exclusive ownership and save/close behavior.
2. Add event-driven manager snapshots for structure, selection and shell cwd/prompt changes. Keep metadata immutable and callbacks outside manager locks. RED tests: background cwd and structural changes.
3. Connect origin-terminal agent identities before sidebar routing; reject invalid UUIDs, separate scratch identities, clear stale agent changes and shell returns. RED tests for each lifecycle.
4. Restore startup entries once, with exact resume arguments, prompt readiness, fallback shells, custom names, selection and workspace membership. RED tests for startup precedence, same-folder distinct identities, missing entries and command cancellation.
5. Add the default-on setting and Startup card, including empty/error workspace lists. RED tests for config and toggling persistence.
6. Flush before updater relaunch, coordinate child startup with the parent's state lock, and prevent teardown overwrite. RED tests for update success/failure and lock handoff.
7. Verify CLI help and isolated PTY startup/resume behavior without owner settings or credentials. Run full tests/vet, focused races and macOS/Windows builds. Record unverified Windows runtime and installed updater checks explicitly.
8. Update README, HANDOFF and previous non-goals. Review diff. Commit and push only on owner request; version bump and release follow feature merge.

## Completion

All implementation steps are complete. Full tests/vet, focused races and
macOS/Windows builds for both architectures pass. Real isolated CLI round trips
verified two conversations per agent in one folder, exact resume identities, and
zero model requests on idle interactive resume. See
`../../audits/2026-11-01-session-restore.md` for evidence and remaining Windows/runtime
and installed-updater checks. The owner subsequently requested PR submission; no
installation or release was performed.
