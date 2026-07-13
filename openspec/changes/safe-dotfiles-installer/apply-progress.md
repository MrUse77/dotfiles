# Apply Progress: Safe Dotfiles Installer

## Status

- **Change:** safe-dotfiles-installer
- **Artifact store:** openspec
- **Current phase:** apply
- **Work unit:** 1 of 4 (tasks 1.1–1.5)
- **Apply state:** ready

## Completed Tasks

- [x] 1.1 RED — plan contracts and canonical fingerprint tests
- [x] 1.2 GREEN — immutable plan model and read-only planner seams
- [x] 1.3 TRIANGULATE — pre-state discovery boundaries
- [x] 1.4 RED/GREEN — execution report contracts
- [x] 1.5 REFACTOR/verify work unit

Task checkboxes were updated in `openspec/changes/safe-dotfiles-installer/tasks.md`.

## Files Changed

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

- Kept `TargetDiscoverer` and `ActionCatalog` as interfaces with no default implementation for Work Unit 1; the real catalog discovery will be added in Work Unit 3.
- Backup path escaping uses hex encoding of the cleaned absolute destination instead of base64; the design only required a reversible, path-safe encoding.
- No commit was produced because the parent explicitly prohibited commits for this work unit.

## Remaining Tasks

Work Unit 2 (tasks 2.1–2.5): recoverable filesystem transaction.

```text
- [ ] 2.1 RED — inventory and backup-before-write tests
- [ ] 2.2 GREEN — safe inventory, backup, and staging implementation
- [ ] 2.3 TRIANGULATE — atomic mutation and drift tests
- [ ] 2.4 RED/GREEN — rollback completeness tests
- [ ] 2.5 REFACTOR/verify work unit
```

## Workload / PR Boundary

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

### Deviations

None. No Work Unit 2 work was started. No commit, push, or PR was created.

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
- No Work Unit 2 work was started. No commit, push, or PR was created.

### Remaining Tasks

Work Unit 2 (tasks 2.1–2.5) remains unstarted.
