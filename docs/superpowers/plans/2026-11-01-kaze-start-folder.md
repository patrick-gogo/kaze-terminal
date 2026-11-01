# Issue #77 implementation plan

Proceed under the owner's explicit instruction to implement without further approvals.

1. Add bounded RED tests for config string parsing, home expansion, existing-directory
   validation, empty/home fallback, missing paths, regular files and invalid types.
   Implement `StartDir` and the shared resolver; keep raw config text for persistence.
2. Add RED tests for fresh-start and new-tab directories, workspace/restore precedence,
   deleted-folder fallback and immutable starting-directory metadata. Implement explicit
   plain-tab start directories and the generic shell fallback using the resolver.
3. Add RED tests for the Startup row, picker success/cancel/error, duplicate-dialog
   prevention, clearing to home and bad picker results. Reuse existing row typography,
   shared picker queue and config key writer; no disk lookups during layout.
4. Inspect the settings layout in a controlled render without global input. Make at
   most one correction pass. Run focused tests, full tests/vet, relevant races and
   macOS/Windows builds. Update README and HANDOFF with results and platform limits.
5. Leave the feature on its isolated branch for review. Commit/push only when requested;
   follow the merged-feature release workflow separately from this unmerged feature.

## Completion

Implementation and verification complete locally. Config, first/new-tab launch,
workspace/restoration precedence, picker behavior, and narrow layout have tests.
Full suite/vet, focused races and macOS/Windows builds for both architectures pass.
Native offscreen QA used wide/narrow Midnight/Washi captures with one correction
and one confirmation round. See `../../audits/2026-11-01-start-folder.md` for evidence.

The owner subsequently requested a PR for review; commit and push this feature branch
under that authorization. Do not merge the feature PR.
