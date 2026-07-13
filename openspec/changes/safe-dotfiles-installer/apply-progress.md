# Apply Progress: Safe Dotfiles Installer

## Status

- **Change:** safe-dotfiles-installer
- **Artifact store:** openspec
- **Current phase:** apply
- **Work unit:** 2 of 4 (tasks 2.1–2.5)
- **Apply state:** ready

## Completed Tasks

- [x] 1.1 RED — plan contracts and canonical fingerprint tests
- [x] 1.2 GREEN — immutable plan model and read-only planner seams
- [x] 1.3 TRIANGULATE — pre-state discovery boundaries
- [x] 1.4 RED/GREEN — execution report contracts
- [x] 1.5 REFACTOR/verify work unit
- [x] 2.1 RED — inventory and backup-before-write tests
- [x] 2.2 GREEN — safe inventory, backup, and staging implementation
- [x] 2.3 TRIANGULATE — atomic mutation and drift tests
- [x] 2.4 RED/GREEN — rollback completeness tests
- [x] 2.5 REFACTOR/verify work unit

Task checkboxes were updated in `openspec/changes/safe-dotfiles-installer/tasks.md`.

## Files Changed

New files under `cli/pkg/installer/transaction/`:

- `cli/pkg/installer/transaction/transaction.go` — `Transaction`, `Prepare`, `Commit`, `Rollback`, `Execute`, drift checks, atomic staging, and report building
- `cli/pkg/installer/transaction/filesystem.go` — injectable `Filesystem`/`File` seams and `copyFile`/`copyTree` helpers using only `os`, `io`, `filepath`, and `os.Symlink`
- `cli/pkg/installer/transaction/inventory.go` — retained `Inventory`, `InventoryEntry`, and `persistInventory`
- `cli/pkg/installer/transaction/transaction_test.go` — inventory, backup-before-write, atomic mutation, drift, and staging tests
- `cli/pkg/installer/transaction/rollback_test.go` — reverse-order rollback, continued rollback after failure, and backup-retention tests

Updated OpenSpec artifact:

- `openspec/changes/safe-dotfiles-installer/tasks.md` — checked off tasks 2.1–2.5

No existing production code outside `cli/pkg/installer/transaction/` was modified. No commit was made per the parent instruction.

## TDD Cycle Evidence

### RED — 2.1 / 2.3 / 2.4

Initial test run failed because the transaction package did not exist:

```text
$ cd cli && go test ./pkg/installer/transaction -run 'Test(Prepare|Execute|Rollback)'
# github.com/MrUse77/dots-cli/pkg/installer/transaction [github.com/MrUse77/dots-cli/pkg/installer/transaction.test]
pkg/installer/transaction/rollback_test.go:35:8: undefined: New
pkg/installer/transaction/transaction_test.go:81:34: undefined: Inventory
...
```

### GREEN — 2.2 / 2.4

After adding `transaction.go`, `filesystem.go`, and `inventory.go`, the transaction suite passed:

```text
$ cd cli && go test ./pkg/installer/transaction -v
ok  	github.com/MrUse77/dots-cli/pkg/installer/transaction	0.010s
```

### TRIANGULATE — 2.3 / 2.4

Extended tests covered:

- file, directory, symlink, and absent target mutations
- special-character and root-level-file paths
- backup collision rejection
- drift detection for changed files and absent targets that became present
- failure at the first target, after multiple mutations, and reverse restoration order
- continued rollback after one restore failure
- backup retention after both success and rollback
- no `Remove(dest)` fallback during mutation

### REFACTOR — 2.5

- Extracted `backupTarget` and `restoreFromBackup` helpers to keep `mutateTarget` and `Rollback` readable.
- Staged file and symlink restores through a sibling temp path so a failed restore never leaves the destination missing.
- Used `t.TempDir()` for every filesystem test; no real home directory or external commands are invoked.
- Ran `go fmt`, `go vet`, and `git diff --check`; no issues.

New files under `cli/pkg/installer/`:

- `cli/pkg/installer/plan/plan.go` — immutable plan model, fingerprint, validation, and error types
- `cli/pkg/installer/plan/planner.go` — read-only `Planner` with injectable seams
- `cli/pkg/installer/plan/state.go` — `StateReader` and deterministic pre-state digests
- `cli/pkg/installer/plan/plan_test.go` — plan/fingerprint/validation tests
- `cli/pkg/installer/plan/state_test.go` — pre-state boundary tests
- `cli/pkg/installer/report/report.go` — pure typed `ExecutionReport` and error types
- `cli/pkg/installer/report/report_test.go` — report/error contract tests

Updated OpenSpec artifact:

- `openspec/changes/safe-dotfiles-installer/tasks.md` — checked off tasks 1.1–1.5

No existing production code under `cli/` was modified. No commit was made per the parent instruction.

## TDD Cycle Evidence

### RED — 1.1 / 1.4

Initial test run failed because the plan/report contracts did not exist:

```text
$ go test ./pkg/installer/plan ./pkg/installer/report
pkg/installer/plan: no non-test Go files in .../cli/pkg/installer/plan
# github.com/MrUse77/dots-cli/pkg/installer/plan [github.com/MrUse77/dots-cli/pkg/installer/plan.test]
pkg/installer/plan/plan_test.go:24:12: undefined: Target
pkg/installer/plan/plan_test.go:28:66: undefined: Options
...
```

### GREEN — 1.2 / 1.4

After adding `plan.go`, `planner.go`, `state.go`, and `report.go`, the same suites passed:

```text
$ go test ./pkg/installer/plan ./pkg/installer/report
ok  	github.com/MrUse77/dots-cli/pkg/installer/plan
ok  	github.com/MrUse77/dots-cli/pkg/installer/report
```

### TRIANGULATE — 1.3

Extended boundary cases in `state_test.go` (absent/file/dir/symlink, special files, unreadable files, deterministic directory digest across creation order) and the existing planner tests (missing source, prerequisite failure without mutation). All passed after implementing `state.go`.

### REFACTOR — 1.5

- Removed duplication by sharing fake helpers inside test packages.
- Renamed overlapping constants (`Symlink` collision between `MutationKind` and `PreStateType`) to `StateSymlink`, `StateFile`, etc.
- Reformatted with `go fmt`.
- Verified with `go vet` and `git diff --check`.

## Verification Commands and Results

```bash
cd cli
go test ./pkg/installer/transaction
# ok  	github.com/MrUse77/dots-cli/pkg/installer/transaction

go vet ./pkg/installer/transaction
# (no output)

go test ./...
# ok  	github.com/MrUse77/dots-cli/pkg/installer/plan
# ok  	github.com/MrUse77/dots-cli/pkg/installer/report
# ok  	github.com/MrUse77/dots-cli/pkg/installer/transaction

go vet ./...
# (no output)

go build ./...

go test ./pkg/installer/plan ./pkg/installer/report
# ok  	github.com/MrUse77/dots-cli/pkg/installer/plan
# ok  	github.com/MrUse77/dots-cli/pkg/installer/report

go vet ./pkg/installer/plan ./pkg/installer/report
# (no output)

git diff --check
# (no output)

# Broader regression check:
go test ./...
# ok  	github.com/MrUse77/dots-cli/pkg/installer/plan
# ok  	github.com/MrUse77/dots-cli/pkg/installer/report
# ?   	github.com/MrUse77/dots-cli/cmd	[no test files]
# ?   	github.com/MrUse77/dots-cli/pkg/installer	[no test files]
# ?   	github.com/MrUse77/dots-cli/pkg/theme	[no test files]
```

## Deviations from Design


- The implementation copies each existing target to its retained backup entry before mutation, then atomically replaces the target. Rollback restores from the retained copy, so backups remain on disk after rollback. This satisfies the proposal rule that backups must remain available after rollback while still using atomic sibling renames for mutation.
- Directory mutation uses a temporary trash path for the original tree so the staged directory can be renamed into place; the trash is removed on success and is not required for rollback because the retained backup copy is authoritative.
- The persisted inventory file is written inside the first target's backup root (`<parent>/.dots-backups/<RunID>/inventory.json`). Each backup root is adjacent to its target parent, satisfying the same-filesystem atomic-rename requirement.

## Remaining Tasks

Work Unit 3 (tasks 3.1–3.6): structured external actions and executor ordering.

```text
- [ ] 3.1 RED — external command boundary tests
- [ ] 3.2 GREEN — external runner and classification model
- [ ] 3.3 RED — catalog adaptation tests
- [ ] 3.4 GREEN/TRIANGULATE — adapt current installer helpers behind the catalog
- [ ] 3.5 RED/GREEN — executor sequencing tests and implementation
- [ ] 3.6 REFACTOR/verify work unit

- Kept `TargetDiscoverer` and `ActionCatalog` as interfaces with no default implementation for Work Unit 1; the real catalog discovery will be added in Work Unit 3.
- Backup path escaping uses hex encoding of the cleaned absolute destination instead of base64; the design only required a reversible, path-safe encoding.
- No commit was produced because the parent explicitly prohibited commits for this work unit.

## Workload / PR Boundary


This work unit is confined to the recoverable filesystem transaction package under `cli/pkg/installer/transaction/`. It does not touch `cmd/install.go`, existing installer helpers, or any non-`cli/` files. It is the second PR slice in the `feature-branch-chain` delivery strategy recorded in `state.yaml`.

---

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
