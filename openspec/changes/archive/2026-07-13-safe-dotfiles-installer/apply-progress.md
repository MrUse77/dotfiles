# Apply Progress: Safe Dotfiles Installer

## Status

- **Change:** safe-dotfiles-installer
- **Artifact store:** openspec (authoritative repository-local store)
- **Current phase:** verify
- **Apply state:** all work units complete; verification pending report
- **Authoritative completion:** Work Units 1–4 are complete. Work Unit 4 merged through PR #14 (`83ede74`); the current branch contains only the task-checkbox reconciliation.

## Completed Tasks

All 25 implementation and apply-gate checkboxes in `tasks.md` are complete:

- Work Unit 1: 1.1–1.5 — immutable plan and typed reporting.
- Work Unit 2: 2.1–2.5 — recoverable filesystem transaction.
- Work Unit 3: 3.1–3.6 — structured external actions and executor ordering.
- Work Unit 4: 4.1–4.6 — review UI and Cobra cutover.
- Apply gate: delivery decision recorded; chained delivery followed; single-PR exception explicitly not applicable.

## Files Changed and Delivery Boundary

- **PR/work-unit chain:** `feature-branch-chain` as recorded in `tasks.md` and `state.yaml`.
- **Work Unit 1:** plan/report contracts (PR #10 baseline and follow-up corrections).
- **Work Unit 2:** transaction/inventory/rollback implementation and tests (PR #12).
- **Work Unit 3:** action catalog, structured external runner, executor, and focused tests (PR #13, `1719235`).
- **Work Unit 4:** review UI, Cobra composition, and focused tests (PR #14, `d1b00b0`, merged as `83ede74`).
- **Current branch:** only `openspec/changes/safe-dotfiles-installer/tasks.md` checkbox reconciliation; no product-code changes.

## TDD Cycle Evidence

The historical record below is deliberately conservative. It preserves the earlier detailed evidence and records what can be proven from the artifact, commit history, present test files, and current GREEN run. It does not reconstruct unrecorded chronological RED/TRIANGULATE runs.

| Work unit / tasks | RED evidence | GREEN evidence | TRIANGULATE / REFACTOR evidence | Current status |
|---|---|---|---|---|
| 1 / 1.1–1.5 | Recorded in the historical narrative below for plan/report contracts. | Historical package runs recorded; present suites pass. | Historical boundary/refactor narrative recorded. | Complete; current full suite GREEN. |
| 2 / 2.1–2.5 | Recorded in the historical narrative below for transaction scenarios. | Historical transaction/full-suite runs recorded; present suites pass. | Historical boundary/refactor narrative recorded. | Complete; current full suite GREEN. |
| 3 / 3.1–3.6 | **Recovered delegated-worker evidence (not originally persisted at apply time):** catalog slice RED observed because `NewActionCatalog` was undefined; executor slice RED focused test failed before `NewExecutor` existed; direct-copy slice RED focused test initially failed before implementation. | **Recovered delegated-worker evidence:** catalog GREEN `cd cli && go test ./pkg/installer -run 'Test(ActionCatalog|ManagedTargets)' -count=1` passed; executor GREEN `cd cli && go test ./pkg/installer -run 'TestExecutor'` passed; direct-copy focused GREEN passed. | **Recovered delegated-worker evidence:** catalog/executor TRIANGULATE/REFACTOR `cd cli && go test ./pkg/installer/...` passed; direct-copy TRIANGULATE/REFACTOR full installer tests and managed font/icon shell-copy grep passed. Direct-copy validation: `cd cli && go test ./pkg/installer -count=1`, `cd cli && go test ./pkg/installer/...`, and managed font/icon shell-copy grep passed. | Complete; recovered execution evidence is now preserved with provenance; current full suite GREEN. |
| 4 / 4.1–4.6 | **Recovered delegated-worker evidence (not originally persisted at apply time):** RED observed focused review test failure before fixing rendered-state mutation. Later bounded correction RED focused cancellation tests failed under previous aborted behavior. | **Recovered delegated-worker evidence:** focused UI tests passed after the rendered-state fix; focused cancellation tests passed after the bounded correction. | **Recovered delegated-worker evidence:** TRIANGULATE/REFACTOR command tests, full installer tests, formatting, vet, build, and diff checks passed; the bounded correction was followed by full validation. | Complete; recovered execution evidence is now preserved with provenance; current full suite GREEN. |

### Current Validation

Run from `cli/` on 2026-07-13:

```text
$ go fmt ./...
$ go test ./...
PASS: cmd, installer, external, plan, report, transaction, ui
$ go test -cover ./...
PASS (package coverage: cmd 36.5%, installer 32.3%, external 81.8%, plan 81.3%, report 75.0%, transaction 66.1%, ui 81.7%)
$ go vet ./...
PASS (no output)
$ go build ./...
PASS
$ git diff --check
PASS (no output)
```

## Deviations from Design

- Backup path escaping uses hex encoding of the cleaned absolute destination; it remains reversible and path-safe.
- The transaction retains backup copies and restores from those copies; backup inventory persists after success and rollback.
- The inventory is stored in the first target backup root. This remains adjacent to a managed target and is retained for recovery.
- No implementation deviation was recorded for Work Units 3–4. Their recovered execution evidence is recorded in the TDD Cycle Evidence table above. **Provenance:** this evidence was recovered verbatim from delegated worker handoffs after apply; it was not originally persisted at apply time.

## Remaining Tasks

None. There are no unchecked implementation task markers in `tasks.md`.

## Historical Review Correction Evidence (preserved verbatim)

## Review Correction: review-3c6a78d80a8f01c8

Applied as a single bounded ordinary-review correction for lineage `review-3c6a78d80a8f01c8`.
Correction IDs: `RELIABILITY-001`, `RELIABILITY-002`, `RESILIENCE-002`.
Budget: max 200 changed lines; actual 189 changed lines in `cli/pkg/installer/transaction/`.

### Files Changed

- `cli/pkg/installer/transaction/transaction.go`
- `cli/pkg/installer/transaction/filesystem.go`
- `cli/pkg/installer/transaction/transaction_test.go`
- `cli/pkg/installer/transaction/rollback_test.go`

### TDD Cycle Evidence

| Cycle | Test | Command | Result |
|-------|------|---------|--------|
| RED | `TestTransaction_PersistInventoryError_IsPropagated` | `go test ./pkg/installer/transaction -run TestTransaction_PersistInventoryError_IsPropagated` | FAIL (inventory write error discarded) |
| RED | `TestTransaction_Commit_BackupPathCollisionAfterPrepare` | `go test ./pkg/installer/transaction -run TestTransaction_Commit_BackupPathCollisionAfterPrepare` | FAIL (no error; destination mutated) |
| RED | `TestTransaction_Rollback_DirectoryRestoreCopyFailureKeepsDestination` | `go test ./pkg/installer/transaction -run TestTransaction_Rollback_DirectoryRestoreCopyFailureKeepsDestination` | FAIL (destination directory missing after copy failure) |
| GREEN | all correction tests | `go test ./pkg/installer/transaction -run 'TestTransaction_(PersistInventoryError_IsPropagated\|Commit_BackupPathCollisionAfterPrepare\|Rollback_DirectoryRestoreCopyFailureKeepsDestination)'` | PASS |

### Implementation Summary

- **RESILIENCE-002:** every `persistInventory` call now checks the returned error and propagates it, using `errors.Join` when another primary error is already present.
- **RELIABILITY-001:** `copyFile` now refuses to create a destination that already exists, preventing Commit-time backup-path overwrites after Prepare collision checks.
- **RELIABILITY-002:** directory rollback now copies the backup into a staged temporary directory and only swaps it into place after the copy succeeds, so a copy failure leaves the live destination intact.

### Verification

```bash
cd cli
go test ./pkg/installer/transaction
go test ./...
go vet ./...
go build ./...

This work unit is confined to the plan/report contracts. It does not touch `cmd/install.go`, existing installer helpers, or any non-`cli/` files. It is the first PR slice in the `feature-branch-chain` delivery strategy recorded in `state.yaml`.

---

## Review Correction — lineage review-581fecf6896cd0f6

**Correction IDs:** RELIABILITY-001, RISK-001
**Scope:** external source-symlink bypass in source containment validation.
**Branch:** feat/safe-installer-plan
**Allowed paths:** `cli/pkg/installer/plan/plan.go`, `cli/pkg/installer/plan/planner.go`, `cli/pkg/installer/plan/plan_test.go`.

### Problem

`Planner.Build` validated source containment using the declared source path, not the symlink-resolved target. A symlink located inside the repository but resolving to a path outside the repository was accepted, allowing the installer to reference external files.

### Fix

- Added `resolveSource` in `plan.go` to `Lstat` a source, follow symlinks via `filepath.EvalSymlinks`, and verify the resolved target is a regular file or directory.
- Updated `validateTargets` to check containment against both the declared source path and the resolved source target.
- Simplified `validateSourceReadable` in `planner.go` to reuse `resolveSource`.

### Files Changed

- `cli/pkg/installer/plan/plan.go` (+31 lines)
- `cli/pkg/installer/plan/planner.go` (+2/-8 lines)
- `cli/pkg/installer/plan/plan_test.go` (+52 lines)

Total correction diff: **93 changed lines** (77 net added). Within the 200-line correction budget.

### TDD Cycle Evidence

#### RED

Added `TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected`: created a repo-contained symlink pointing to a file in `t.TempDir()` outside the repo and asserted `*SourceOutsideRepoError`. The test failed because `os.Stat` followed the symlink and `isWithinRepo` only checked the symlink path.

```text
$ go test ./pkg/installer/plan -run TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected -v
--- FAIL: TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected (0.00s)
    plan_test.go:442: expected error for symlink resolving outside repo
```

#### GREEN

Implemented `resolveSource` and resolved-source containment in `validateTargets`. The regression test passed.

```text
=== RUN   TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected
--- PASS: TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected (0.00s)
```

#### TRIANGULATE

Added `TestBuildPlan_RepoSymlinkResolvingInsideIsAccepted` to prove that a repo-contained symlink resolving to another repo-contained file is still accepted.

```text
=== RUN   TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected
--- PASS: TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected (0.00s)
=== RUN   TestBuildPlan_RepoSymlinkResolvingInsideIsAccepted
--- PASS: TestBuildPlan_RepoSymlinkResolvingInsideIsAccepted (0.00s)
```

#### REFACTOR

- Kept `validateSourceReadable` as a thin wrapper over `resolveSource` to avoid duplicating file-type validation.
- Ran `go fmt`, `go vet`, and `git diff --check`; no issues.

### Validation Commands and Results

```bash
cd cli
go test ./pkg/installer/plan
go test ./pkg/installer/report
go vet ./pkg/installer/plan
go vet ./pkg/installer/report
go test ./...
git diff --check
```

All passed.

### Remaining Work

Work Unit 3 (tasks 3.1–3.6) remains unstarted.

### Deviations

At the time of this correction, no Work Unit 2 work had started. No commit, push, or PR was created.

---

## Review Correction — lineage review-f6d497a78065047a

**Correction IDs:** RISK-001, RELIABILITY-001, RESILIENCE-002, RELIABILITY-003
**Scope:** installation-plan immutability, fingerprint error handling, nested special-file rejection
**Branch:** feat/safe-installer-plan
**Allowed paths:** `cli/pkg/installer/plan/plan.go`, `planner.go`, `state.go`, `plan_test.go`, `state_test.go`; `cli/pkg/installer/report/report.go`, `report_test.go`; `openspec/.../apply-progress.md`

### Problem

1. `InstallationPlan` exposed mutable `ManagedTargets` and `ExternalActions` slices. Callers could mutate target data, command argument slices, and environment maps after fingerprinting, invalidating the reviewed plan.
2. `fingerprint` panicked on JSON marshal failure instead of returning an error.
3. `collectDirectoryEntries` recorded nested unsupported special files as `unsupported` entries rather than rejecting them.

### Fix

- Made `InstallationPlan` slices unexported and added `ManagedTargets()` / `ExternalActions()` accessors that return deep copies.
- Added deep-copy helpers (`cloneTargets`, `cloneActions`) and used them in `Planner.Build` so both caller mutation and post-build catalog/discoverer mutation cannot affect the stored plan.
- Changed `fingerprint` to return `(string, error)` and `Build` to return a typed `*FingerprintError` on fingerprint failure.
- Added a `canonicalMarshal` seam so the failure path is testable without relying on panic recovery.
- Rejected nested unsupported special files in `collectDirectoryEntries` with an explicit error.

### Files Changed

- `cli/pkg/installer/plan/plan.go` — unexported plan slices, accessors, clone helpers, `FingerprintError`, fingerprint error return
- `cli/pkg/installer/plan/planner.go` — deep-copy plan storage, typed fingerprint error handling
- `cli/pkg/installer/plan/state.go` — reject nested unsupported special files
- `cli/pkg/installer/plan/plan_test.go` — immutability and fingerprint-error tests
- `cli/pkg/installer/plan/state_test.go` — nested special-file test

Total correction diff: **188 changed lines** (94 net added). Within the 200-line correction budget.

### TDD Cycle Evidence

#### RED

Added failing tests:

- `TestInstallationPlan_IsImmutableAfterBuild` subtests for target-slice mutation, action-command args/env mutation, and catalog mutation after build.
- `TestBuildPlan_FingerprintError` by injecting a failing `canonicalMarshal`.
- `TestStateReader_DirectoryWithNestedUnsupportedSpecialFile` with a nested named pipe.

```text
$ go test ./pkg/installer/plan ./pkg/installer/report
--- FAIL: TestInstallationPlan_CatalogMutationAfterBuildDoesNotAffectPlan
    plan_test.go:...: catalog mutation leaked into plan args: got "mutated"
--- FAIL: TestStateReader_DirectoryWithNestedUnsupportedSpecialFile
    state_test.go:...: expected error for nested unsupported special file
FAIL
```

#### GREEN

Implemented deep-copy accessors, `FingerprintError`, and nested special-file rejection. All new tests passed.

```text
$ go test ./pkg/installer/plan ./pkg/installer/report
ok  	github.com/MrUse77/dots-cli/pkg/installer/plan
ok  	github.com/MrUse77/dots-cli/pkg/installer/report
```

#### TRIANGULATE

The combined immutability test exercises three independent mutation vectors (caller field mutation, caller slice append, external catalog mutation). The fingerprint test exercises both the error path and the typed-error contract. The state test adds a nested special file inside a directory that also contains a regular file.

#### REFACTOR

- Inlined `cloneActions` to avoid small helper proliferation.
- Compressed `FingerprintError` and removed redundant comments.
- Ran `go fmt`, `go vet`, and `git diff --check`; no issues.

### Validation Commands and Results

```bash
cd cli
go test ./...
go vet ./...
git diff --check
```

All passed.

### Deviations

- `InstallationPlan.ManagedTargets` and `.ExternalActions` are now methods instead of fields. This is a breaking but necessary API change to satisfy the immutable-plan requirement.
- At the time of this correction, no Work Unit 2 work had started. No commit, push, or PR was created.

### Historical Status Note

The correction record above captured Work Unit 2 as unstarted at that earlier point in the accepted history. It is superseded by the authoritative current status and completed-task list at the top of this artifact.
