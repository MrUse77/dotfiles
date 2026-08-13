# Archive Report: MoonArch Versioned Configuration Releases

**Change**: moonarch-versioned-config-releases
**Archived**: 2026-08-13
**Artifact store mode**: openspec (filesystem merge + archive folder move)
**Planning home**: `openspec/`
**Archived to**: `openspec/changes/archive/2026-08-13-moonarch-versioned-config-releases/`

## Final State (terminal record at close)

This report describes the state of the change AT CLOSE per the Final-State Authority hierarchy
(structured status and launch-prompt final-state facts outrank intermediate snapshots).

- **Implementation**: Complete — 39/39 tasks. Phases 5 and 6 shipped as merged PRs #109 and #111;
  Phase 7 shipped as merged PR #113 (commit `b4e4bf9`), all with green CI.
- **Verification verdict at close**: PASS WITH WARNINGS — 20/20 requirements, 62/62 scenarios
  COMPLIANT, zero CRITICAL findings.
- **Task Completion Gate**: Passed. The persisted `tasks.md` artifact shows 39/39 checked and
  0 unchecked implementation tasks (`- [ ]` count = 0) at archive time. No archive-time
  checkbox reconciliation was required.
- **Native Review Receipt Gate**: `reviewGate` is structurally ABSENT for this candidate.
  Receipt-driven development is disabled by explicit maintainer choice and no review was ever
  started for this candidate. Archive proceeds under ordinary repository policy. No receipt was
  demanded and the absence was not treated as a defect; no reviewer was launched.

### Open-at-Close Warnings (non-blocking, remain open)

Per the orchestrator's final-state facts (outranking the intermediate `verify-report` snapshot),
the following warnings remain open at close and are NOT current blockers:

1. Live GitHub tag publication was not exercised against the real repository; deterministic
   contracts (bridge, immutable identity, installer guard) are proven by recorded fixtures plus a
   fake `curl` and static workflow analysis.
2. Informational coverage below 80% in `cmd` (70.5%) and `transaction` (69.7%) — informational
   per strict-TDD rules, not blocking.
3. Pre-existing README MD060 (outside this change's modified files).
4. Redundant `working-directory: .` in `ci.yml` "Go test" step (functionally correct).

### Source Ranking Notes

- The launch-prompt final-state facts (merged PRs, PASS WITH WARNINGS, warnings open at close)
  agree with the `verify-report` snapshot and are corroborated by the repo's merged PR history.
  No contradiction requiring an unranked record was found.
- Intermediate `apply-progress.md` and `verify-report.md` snapshots are preserved in the archive
  as history; their "current work unit" and coverage statements reflect the time they were
  written, not the close state.

## Spec Sync (delta → main specs)

| Domain | Action | Details |
|--------|--------|---------|
| backup-inventory | Updated (merge) | ADDED 1 requirement (`Release Provenance Is Additive`, 3 scenarios); 6 existing requirements preserved → 7 total |
| installation-transaction | Updated (merge) | MODIFIED 2 requirements (`Managed-Target Discovery Completes Before Execution`, `Plan Drift Detection Before Mutation` — full block replacement per delta, including "(Previously: ...)" notes); ADDED 2 requirements (`Retired Managed Targets Are Explicit Deletions`, `Conservative Legacy Baselines`); 5 untouched requirements preserved → 9 total |
| moonarch-theme-selector | Updated (merge) | ADDED 1 requirement (`Configuration Apply Preserves Mutable Theme Selection`, 4 scenarios); 3 existing requirements preserved → 4 total |
| config-release-operations | Created (mechanical copy) | New full spec: 5 requirements, 14 scenarios |
| config-release-publication | Created (mechanical copy) | New full spec: 2 requirements, 5 scenarios |
| config-release-resolution | Created (mechanical copy) | New full spec: 4 requirements, 10 scenarios |
| moonarch-cli-self-update | Created (mechanical copy) | New full spec: 3 requirements, 12 scenarios |

No REMOVED requirements appear in any delta, so no destructive-merge warning was triggered per
`rules.archive` in `openspec/config.yaml`.

### Mechanical Copy Evidence (verbatim `diff -r` output)

Each new-domain main spec was copied with the mandated temp-file + `cp` + `diff -r` + `mv`
pattern. Empty output below is the only passing evidence (byte-identical):

```text
=== diff -r config-release-operations ===
=== diff -r config-release-publication ===
=== diff -r config-release-resolution ===
=== diff -r moonarch-cli-self-update ===
```

## Archive Move Evidence (verbatim `diff -r` output)

The change folder was snapshotted recursively, then moved with `git mv` (failed: folder is
untracked — `fatal: source directory is empty`) with the `mv` fallback, then compared against the
pre-move snapshot. Empty output below is the only passing evidence (byte-identical):

```text
fatal: source directory is empty, source=openspec/changes/moonarch-versioned-config-releases, destination=openspec/changes/archive/2026-08-13-moonarch-versioned-config-releases
=== diff -r snapshot vs archived ===
diff exit status: 0
```

The `git mv` fatal is expected and benign: the change folder contains no tracked files. The
recursive snapshot ↔ archived-tree comparison produced zero differences (exit 0). The archive
report itself is additive-only and excluded from the comparison (it did not exist in the source
snapshot).

## Archive Contents Checklist

- [x] proposal.md
- [x] specs/ (7 delta specs: backup-inventory, config-release-operations, config-release-publication, config-release-resolution, installation-transaction, moonarch-cli-self-update, moonarch-theme-selector)
- [x] design.md
- [x] tasks.md — 39/39 tasks checked, 0 unchecked
- [x] apply-progress.md
- [x] verify-report.md
- [x] exploration.md
- [x] Active changes directory no longer contains this change

## Intentional-Warnings Archive

This archive is recorded as intentional-with-warnings (non-critical): the verification verdict is
PASS WITH WARNINGS with zero CRITICAL findings, so no CRITICAL override is involved. The four
warnings above are documented open-at-close and were accepted as non-blocking by the orchestrator.
No partial-archive or checkbox-reconciliation override was used.
