# Workspaces proposal review

Status: reviewed 2026-10-29. Decisions: [workspaces decisions](superpowers/specs/2026-10-29-kaze-workspaces-decisions.md) (they win over the proposal where the two differ). Based on main after Settings PR #11.

1. Read the [behavior proposal](superpowers/specs/2026-10-29-kaze-workspaces-proposal.md) for user flows, storage, startup, failures, and the explicit decision table.
2. Read the [implementation plan](superpowers/plans/2026-10-29-kaze-plan-5b-workspaces-proposal.md) for integration points, shell spike, tasks, tests, and release gates.
3. Compare with the earlier approved [settings design, section 6.2](superpowers/specs/2026-10-29-kaze-settings-page-design.md). Proposed differences are not yet approved.

The main review questions are whether to replace the automatic three-second command fallback with confirmation, and how to submit several startup commands safely while preserving the interactive shell and activated environment. Settle those before implementation.
