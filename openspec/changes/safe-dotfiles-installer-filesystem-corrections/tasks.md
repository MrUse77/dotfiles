# Tasks: safe-dotfiles-installer-filesystem-corrections

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 610–960 across 3 PRs (PR1: ~220–330, PR2: ~170–280, PR3: ~220–350) |
| 400-line budget risk | Accepted (user granted size:exception — individual PRs may exceed 400 lines if needed) |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 (feature-branch-chain) |
| Delivery strategy | exception-ok (size:exception granted — PRs proceed even if >400 lines; chain strategy preserved) |
| Chain strategy | feature-branch-chain |

Decision needed before apply: No (strategy resolved: feature-branch-chain, 3 PRs; size:exception accepted)
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: Accepted (size:exception granted)

**Size exception note**: User explicitly stated that exceeding 400 changed lines in an individual PR is acceptable. This does NOT collapse the feature-branch-chain strategy or merge the three PRs into one. Each PR retains its focused scope and boundary. The exception only means a PR is not blocked or re-split if its natural implementation exceeds 400 lines.

---

## Chain Context

```text
PR 1 Bound source/modes  →  PR 2 Protected metadata  →  PR 3 Recoverable rollback
```

- **Tracker branch**: `feat/safe-filesystem-corrections` (draft/no-merge)
- **PR 1 branch**: `feat/safe-filesystem-corrections/pr1-bound-source-modes`
- **PR 2 branch**: `feat/safe-filesystem-corrections/pr2-protected-metadata`
- **PR 3 branch**: `feat/safe-filesystem-corrections/pr3-recoverable-rollback`
- **PR #1 targets**: tracker branch
- **PR #2 targets**: PR 1 branch
- **PR #3 targets**: PR 2 branch
- **Tracker PR targets**: main (merged only after all children reviewed)

All work under `cli/`. Test command: `cd cli && go test ./...`. Strict TDD active.

---

## PR 1 — Bound Source and Exact Modes

**Scope**: Source-binding model, descriptor-bound planner inspection/digest, bind-before-backup sequencing, full mode capture/application, legacy direct-target compatibility.
**File scope**: `cli/pkg/installer/plan/plan.go`, `plan/planner.go`, `plan/state.go`, `transaction/filesystem.go`, `transaction/transaction.go`, plus tests.
**Start state**: Current `plan.Target` has `SourceDigest` but no identity/type binding; planner uses path-based stat/walk/open; transaction copies modes via `.Perm()` only.
**Finish state**: Planner-built targets carry complete `SourceBinding`; transaction verifies binding before backup/staging; exact modes (perm + setuid/setgid/sticky) applied; legacy digest-less targets remain compatible.
**Rollback**: Revert the PR branch; no persistent state changes.

### RED — Failing tests first

- [x] **1.1 RED**: SourceBinding model tests
  - File: `cli/pkg/installer/plan/plan_test.go`
  - Assert planner-built targets have non-empty `SourceBinding` (Kind, Identity, Digest, TreeManifest).
  - Assert direct internal targets with empty `SourceDigest` have no binding enforced.
  - Assert fingerprint includes all binding fields.
  - Run: `cd cli && go test ./pkg/installer/plan/ -run TestSourceBinding -v` → FAIL

- [x] **1.2 RED**: TOCTOU-safe source consumption tests
  - File: `cli/pkg/installer/transaction/transaction_test.go`
  - Create source file, plan target, then replace source with symlink to attacker content before consumption.
  - Assert transaction detects drift (PlanDriftError), no backup/staging/rename created.
  - Assert type change (file→symlink, dir→file) between plan and consumption is drift.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestSourceDrift -v` → FAIL

- [x] **1.3 RED**: Exact mode preservation tests
  - Files: `cli/pkg/installer/plan/state_test.go`, `cli/pkg/installer/transaction/transaction_test.go`
  - Assert `PreState` captures full `os.FileMode` (perm + setuid/setgid/sticky), not just `.Perm()`.
  - Assert installed file receives exact source mode (e.g., 04755 → setuid preserved).
  - Assert installed directory receives exact source mode with children applied before parent.
  - Run: `cd cli && go test ./pkg/installer/plan/ -run TestExactMode -v` → FAIL; `cd cli && go test ./pkg/installer/transaction/ -run TestExactMode -v` → FAIL

- [x] **1.4 RED**: Legacy direct-target compatibility tests
  - File: `cli/pkg/installer/plan/plan_test.go`
  - Construct target directly with empty `SourceDigest`, no binding.
  - Assert target is executable — no digest enforcement, no binding check.
  - Assert planner-built target always has non-empty digest and binding.
  - Assert compatibility path never weakens bound-target validation.
  - Run: `cd cli && go test ./pkg/installer/plan/ -run TestLegacyDirectTarget -v` → FAIL

### GREEN — Minimum production code to pass

- [x] **1.5 GREEN**: Implement `SourceBinding` model in `plan`
  - File: `cli/pkg/installer/plan/plan.go`
  - Add `SourceBinding` struct: `Kind`, `Identity` (device/inode), `Digest`, `LinkValue`, `TreeManifest`.
  - Add `TreeManifest` entry: relative path, type, full mode, link value, digest, identity.
  - Add unexported binding fields to `Target`; planner fills them; direct construction leaves empty.
  - Add plan fingerprint serialization that includes all binding fields.
  - Run: `cd cli && go test ./pkg/installer/plan/ -run TestSourceBinding -v` → PASS

- [x] **1.6 GREEN**: Implement `SafeFS` interface and Unix implementation
  - File: `cli/pkg/installer/transaction/filesystem.go` (+ `filesystem_unix.go` if needed)
  - Define `SafeFS` interface: `OpenNoFollow(path) (*os.File, error)`, `FstatAt(dirfd, name) (unix.Stat_t, error)`, `ReadlinkAt(dirfd, name) (string, error)`, `OpenDirNoFollow(path) (*os.File, error)`.
  - Implement using `golang.org/x/sys/unix`: `openat(..., O_NOFOLLOW)`, `fstatat(..., AT_SYMLINK_NOFOLLOW)`, `readlinkat`.
  - Return unsupported-platform error on non-Unix; do not fall back to path-based.
  - Add injectable fake for failure-injection tests.
  - Run: build passes; existing tests still pass.

- [x] **1.7 GREEN**: Implement descriptor-bound planner inspection
  - File: `cli/pkg/installer/plan/planner.go`
  - Replace path-based `Stat`/`WalkDir`/`Open` in source resolution with `SafeFS` descriptor-relative calls.
  - Regular file: open `O_NOFOLLOW`, `fstat`, hash from descriptor, bind identity.
  - Directory: open as dir descriptor, walk with `openat`/`fstatat(AT_SYMLINK_NOFOLLOW)`, hash each child from its descriptor. No absolute-path reopen.
  - Symlink: `readlinkat` for link value, resolve and verify content through descriptors.
  - Fill `SourceBinding` completely; reject if digest or identity unavailable.
  - Run: `cd cli && go test ./pkg/installer/plan/ -v` → all plan tests PASS

- [x] **1.8 GREEN**: Implement bind-before-backup in transaction
  - File: `cli/pkg/installer/transaction/transaction.go`
  - In `mutateTarget`, before any destination backup/staging/rename, acquire `BoundSource` handle via `SafeFS`.
  - Open source no-follow, verify type and identity against binding, compute/verify digest from same descriptors.
  - On mismatch/failure: mark entry `source-drift`, persist inventory, skip backup/staging/rename for that target.
  - Legacy direct targets (empty `SourceDigest`): skip binding acquisition, proceed with existing semantics.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestSourceDrift -v` → PASS

- [x] **1.9 GREEN**: Implement exact mode capture and application, including backup-tree roots
  - Files: `cli/pkg/installer/plan/state.go`, `cli/pkg/installer/transaction/transaction.go`
  - Capture full `os.FileMode` (perm | setuid | setgid | sticky) in `PreState`; mask only type bits.
  - Apply full mode to staged files/dirs via `chmod` with the supported-bit mask, not `.Perm()`.
  - For directories: apply children modes first, then parent mode after children created.
  - If OS/filesystem refuses a supported special bit, fail before rename, preserve recovery state.
  - Run: `cd cli && go test ./pkg/installer/plan/ -run TestExactMode -v` → PASS; `cd cli && go test ./pkg/installer/transaction/ -run TestExactMode -v` → PASS

- [x] **1.10 GREEN**: Legacy direct-target compatibility in transaction
  - File: `cli/pkg/installer/transaction/transaction.go`
  - Ensure legacy targets (empty `SourceDigest`) skip binding acquisition but still run existing pre-state checks and backup/inventory rules.
  - Ensure bound targets never skip validation because of the compatibility path.
  - Run: `cd cli && go test ./pkg/installer/plan/ -run TestLegacyDirectTarget -v` → PASS

### TRIANGULATE — Edge cases and platform coverage

- [x] **1.11 TRIANGULATE**: Directory tree manifest with mixed types
  - File: `cli/pkg/installer/plan/plan_test.go`
  - Create source directory with regular files, subdirectories, and symlinks.
  - Assert `TreeManifest` contains each entry with correct type, full mode, digest, and identity.
  - Assert planner rejects unstable tree (entries change during walk).
  - Run: `cd cli && go test ./pkg/installer/plan/ -run TestTreeManifest -v` → PASS

- [x] **1.12 TRIANGULATE**: Symlink source binding with link value verification
  - File: `cli/pkg/installer/transaction/transaction_test.go`
  - Plan a symlink target, then change link value before consumption.
  - Assert drift detected; no mutation occurs.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestSymlinkDrift -v` → PASS

- [x] **1.13 TRIANGULATE**: Platform skip annotations
  - Files: `cli/pkg/installer/transaction/filesystem_unix_test.go` (or build tags)
  - Add `//go:build unix` or runtime checks; skip tests requiring `openat`/`fstatat` on unsupported platforms.
  - Run: `cd cli && go test ./... -short` → PASS (no panics on any platform)

### REFACTOR — Clean up and document

- [x] **1.14 REFACTOR**: Extract descriptor-helper utilities
  - File: `cli/pkg/installer/transaction/filesystem.go`
  - Extract common patterns (open-verify-hash, open-dir-walk) into reusable helpers.
  - Run: `cd cli && go test ./...` → PASS; `go vet ./...` → clean

- [x] **1.15 REFACTOR**: Document legacy-direct-target boundary
  - File: `cli/pkg/installer/plan/plan.go`
  - Add doc comment on `Target.SourceDigest` explaining: empty = legacy direct construction, accepted without enforcement; planner always fills it; compatibility never weakens bound targets.
  - Run: `cd cli && go doc ./pkg/installer/plan/` → readable

**PR 1 verification**: `cd cli && go test ./pkg/installer/plan/ ./pkg/installer/transaction/ -v` → all PASS. `go vet ./...` → clean. `git diff --stat` → ≤330 lines.

### PR 1 correction — Root source-mode drift (RELIABILITY-001)

- [x] Record the supported root mode in every planner-built file and directory `SourceBinding`.
- [x] Before backup, staging, or destination mutation, reject a bound file or directory whose root mode differs from the planned mode with `PlanDriftError`.
- [x] Preserve compatibility for directly constructed targets with empty `SourceDigest`.
- [x] Add file and directory mode-mutation regression tests and run focused uncached tests plus the PR validation suite.

### PR 1 correction — Symlink binding consumption (RELIABILITY-002)

- [x] Commit planner-bound symlink targets using `SourceBinding.LinkValue`, not the mutable `InventoryEntry.LinkValue` captured during `Prepare`.
- [x] Add an X→Y→X regression proving the reviewed X value is committed after validation and a changed value is rejected before destination mutation.
- [x] Preserve legacy direct symlink behavior for targets with empty `SourceDigest`.

### PR 1 correction — Coherent source-binding capture (RISK-001)

- [x] Capture declared source identity and symlink link text, then recheck both after resolving and descriptor-binding the target.
- [x] Retry boundedly on a capture mismatch and return `SourceBindingDriftError` if the source remains unstable.
- [x] Add deterministic substitution regressions proving a changed symlink produces either a coherent retry binding or a planning error, never mixed fields.

---

## PR 2 — Protected Recovery Metadata

**Scope**: Symlink-safe backup-root traversal/validation, collision detection, versioned inventory data model, atomic 0600 persistence, root revalidation before use, interruption tests.
**File scope**: `cli/pkg/installer/transaction/inventory.go`, `transaction/transaction.go`, `transaction/filesystem.go`, plus tests.
**Depends on**: PR 1 (SafeFS interface, descriptor-relative primitives).
**Start state**: `ensureBackupRoot` uses `Mkdir`/`Chmod` trusting paths; `persistInventory` writes in place then chmods; no versioning or lifecycle.
**Finish state**: Backup root chain validated descriptor-by-descriptor with O_NOFOLLOW; inventory is versioned, lifecycle-tracked, atomically persisted at 0600; collisions refused; interruption tests prove durability.
**Rollback**: Revert PR 2 branch onto PR 1; PR 1 remains intact.

### RED — Failing tests first

- [x] **2.1 RED**: Symlink-safe backup root creation tests
  - File: `cli/pkg/installer/transaction/transaction_test.go`
  - Insert symlink at each level of backup root chain; assert creation refused with clear error.
  - Assert foreign/unsafe parent ownership or group/world-writable mode refused.
  - Assert new `.dots-backups` and run directories created with 0700 and verified after creation.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestBackupRootSymlink -v` → FAIL

- [x] **2.2 RED**: Atomic inventory persistence tests
  - File: `cli/pkg/installer/transaction/inventory_test.go` (new)
  - Interrupt write mid-flight (inject error after temp write, before rename).
  - Assert old `inventory.json` intact (or absent if first write); temp file exists but is not authoritative.
  - Assert final `inventory.json` is 0600; temp file is 0600 at creation.
  - Assert valid prior inventory preserved on error.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestAtomicInventory -v` → FAIL

- [x] **2.3 RED**: Versioned inventory content tests
  - File: `cli/pkg/installer/transaction/inventory_test.go`
  - Assert inventory JSON has format version field.
  - Assert lifecycle states: `prepared`, `committing`, `commit-failed`, `rolling-back`, `rolled-back`, `recovery-incomplete`, `completed`.
  - Assert entry states: pending, source-drift, backed-up, staged, original-relocated, mutated, restored, ownership-ambiguous, failed.
  - Assert each entry records `InstalledIdentity`, digest, mode, `BackupPath`, `StagePath`, `TrashPath`, error description.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestInventorySchema -v` → FAIL

- [x] **2.4 RED**: Backup path collision tests
  - File: `cli/pkg/installer/transaction/transaction_test.go`
  - Create two targets whose deterministic backup paths would overlap.
  - Assert both targets refused; clear collision error.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestBackupCollision -v` → FAIL

### GREEN — Minimum production code to pass

- [x] **2.5 GREEN**: Implement `SafeFS.OpenDirectoryChain` with O_NOFOLLOW
  - File: `cli/pkg/installer/transaction/filesystem.go`
  - Walk backup root chain from trusted user-owned directory descriptor using `openat(..., O_DIRECTORY|O_NOFOLLOW)` and `mkdirat`.
  - Validate each component: real directory, current-user-owned, non-group/world-writable.
  - Create `.dots-backups` and run directories with 0700 via `mkdirat`; verify after creation (re-stat through descriptor).
  - Return error on symlink, unsafe ownership, or wrong permissions at any level.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestBackupRootSymlink -v` → PASS

- [x] **2.6 GREEN**: Implement atomic inventory writer
  - File: `cli/pkg/installer/transaction/inventory.go`
  - Write temp sibling file: `O_CREAT|O_EXCL`, 0600 at creation (before write).
  - Write all JSON content, `fsync` file, close.
  - Atomic `rename` to `inventory.json`.
  - `fsync` parent directory after rename.
  - On any error: temp file remains non-authoritative (recognizable name); prior inventory untouched.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestAtomicInventory -v` → PASS

- [x] **2.7 GREEN**: Implement versioned inventory schema
  - File: `cli/pkg/installer/transaction/inventory.go`
  - Add `FormatVersion` field (e.g., `1`).
  - Add `Lifecycle` field with state enum.
  - Add per-entry `State`, `InstalledIdentity`, `InstalledDigest`, `InstalledMode`, `BackupPath`, `StagePath`, `TrashPath`, `ErrorDescription`.
  - JSON remains human-readable; new fields additive.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestInventorySchema -v` → PASS

- [x] **2.8 GREEN**: Implement backup root validation and collision detection in transaction
  - File: `cli/pkg/installer/transaction/transaction.go`
  - In `Prepare`: validate backup root chain via `SafeFS.OpenDirectoryChain` before writing anything.
  - Revalidate root descriptor before each backup creation and inventory replacement.
  - Compute deterministic backup paths for all targets; detect collisions across complete set; refuse both on collision.
  - Persist `prepared` inventory atomically before any managed destination mutation.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestBackupCollision -v` → PASS

### TRIANGULATE — Edge cases

- [x] **2.9 TRIANGULATE**: Intermediate symlink insertion at each backup root level
  - File: `cli/pkg/installer/transaction/transaction_test.go`
  - For each directory in the backup root chain, insert a symlink replacing the real directory.
  - Assert detection at each level; no backup written through the symlink.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestBackupRootIntermediateSymlink -v` → PASS

- [x] **2.10 TRIANGULATE**: Foreign parent ownership and mode
  - File: `cli/pkg/installer/transaction/transaction_test.go`
  - Create backup parent owned by different user or with group-write permission.
  - Assert refused with specific error naming the violating component.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestBackupRootUnsafeParent -v` → PASS

- [x] **2.11 TRIANGULATE**: Inventory temp file name recognition
  - File: `cli/pkg/installer/transaction/inventory_test.go`
  - Assert temp file name is recognizable as non-authoritative (e.g., `.inventory.json.tmp.<random>`).
  - Assert recovery never reads temp name as inventory.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestInventoryTempName -v` → PASS

### REFACTOR — Clean up and document

- [x] **2.12 REFACTOR**: Consolidate root descriptor revalidation
  - File: `cli/pkg/installer/transaction/filesystem.go`
  - Extract root revalidation (open chain, check ownership, check permissions) into a single reusable helper called before each backup creation and inventory replacement.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -v` → all PASS

- [x] **2.13 REFACTOR**: Review inventory schema forward compatibility
  - File: `cli/pkg/installer/transaction/inventory.go`
  - Ensure unknown fields in future versions are ignored (not errored) when reading.
  - Add doc comment on version field and additive-only policy.
  - Run: `cd cli && go vet ./...` → clean

**PR 2 verification**: `cd cli && go test ./pkg/installer/transaction/ -v` → all PASS. `go vet ./...` → clean. `git diff --stat` (vs PR 1 branch) → ≤280 lines.

---

### PR 2 completion evidence

- [x] Inventory persistence retains a validated backup-root descriptor and uses `openat`/`renameat` against that descriptor; a substitution immediately before rename cannot redirect the write.
- [x] Rollback persists `rolling-back`, each restored or failed entry outcome, then `rolled-back` or `recovery-incomplete`.
- [x] TDD evidence: focused RED coverage was added for root substitution and persisted rollback outcomes; GREEN implementation passes `cd cli && go test ./pkg/installer/transaction -count=1`.
- [x] Judgment Day correction: backup creation retains the validated root descriptor and copies through `openat`/`mkdirat` descriptor operations; substitution after root validation cannot redirect backups.
- [x] Judgment Day correction: immediately after a durable backup, inventory persists `backed-up` plus its backup path before destination mutation; the checkpoint is regression-tested from its on-disk JSON.
- [x] Judgment Day correction: backup content and metadata are descriptor-relatively synced after final chmod, every completed backup directory is synced after its entries, and the root is synced after entry creation. Failure injection at every required sync proves neither checkpoint nor mutation proceeds.

## PR 3 — Recoverable Swaps and Rollback

**Scope**: Persisted swap phases, directory swap failure preservation, installed ownership proof, ownership-aware conservative rollback, partial-recovery reporting with retained artifact locations.
**File scope**: `cli/pkg/installer/transaction/transaction.go`, `transaction/inventory.go`, `cli/pkg/installer/report/report.go`, plus tests.
**Depends on**: PR 2 (atomic inventory, versioned schema, SafeFS).
**Start state**: Transaction mutates targets but does not persist intermediate swap phases; rollback uses `RemoveAll` and overwrite without ownership proof; report cannot express partial recovery.
**Finish state**: Each swap phase persisted before irreversible action; directory swap failures retain all artifacts; rollback verifies ownership before mutation; report names affected targets, retained paths, and safe next action.
**Rollback**: Revert PR 3 branch onto PR 2; PR 1 and PR 2 remain intact.

### RED — Failing tests first

- [x] **3.1 RED**: Ownership-aware rollback tests
  - File: `cli/pkg/installer/transaction/rollback_test.go`
  - Mutate target, then externally replace installed content with different file/symlink.
  - Call rollback: assert externally substituted content NOT removed or overwritten.
  - Assert backup, inventory, stage/trash retained; entry marked `ownership-ambiguous`.
  - Assert rollback of owned file (no external change) restores or removes correctly.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestRollbackOwnership -v` → FAIL

- [x] **3.2 RED**: Directory swap failure recovery tests
  - File: `cli/pkg/installer/transaction/transaction_test.go`
  - Inject failure at each directory swap boundary:
    - After stage complete, before trash rename: assert stage retained, original untouched.
    - After trash rename, before stage-to-dest rename: assert trash and stage retained; attempt trash-to-dest restoration; if restoration fails, assert all artifacts preserved.
    - After stage-to-dest rename, before trash cleanup: assert mutated state, backup retained.
  - Assert inventory records each phase transition.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestDirectorySwapFailure -v` → FAIL

- [x] **3.3 RED**: Partial recovery reporting tests
  - File: `cli/pkg/installer/report/report_test.go`
  - Simulate incomplete rollback: assert report includes `RecoveryState` = incomplete/manual.
  - Assert `RecoveryArtifact` lists destination, backup path, stage path, trash path, inventory path for each affected target.
  - Assert safe next action: "inspect named inventory and retained artifacts; do not delete or overwrite ambiguous paths."
  - Assert complete rollback never claims partial state.
  - Run: `cd cli && go test ./pkg/installer/report/ -run TestPartialRecovery -v` → FAIL

- [x] **3.4 RED**: Combined rollback + inventory persistence error tests
  - File: `cli/pkg/installer/transaction/rollback_test.go`
  - Inject rollback failure AND inventory persistence failure simultaneously.
  - Assert returned error aggregates both via `errors.Join`.
  - Assert report lists both operation failures and persistence failures independently.
  - Assert `recovery-incomplete` state set.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestCombinedRollbackError -v` → FAIL

### GREEN — Minimum production code to pass

- [x] **3.5 GREEN**: Implement swap phase recording in inventory
  - File: `cli/pkg/installer/transaction/inventory.go`, `transaction/transaction.go`
  - Before each irreversible directory swap action, persist inventory with entry state transition:
    - `staged`: stage path complete and retained.
    - `original-relocated`: destination renamed to unique trash sibling; inventory names both locations.
    - `mutated`: installed identity recorded, backup retained, trash removable only after this state persisted.
  - Use atomic inventory writer from PR 2.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestDirectorySwapFailure -v` → PASS

- [x] **3.6 GREEN**: Implement directory swap with artifact preservation on failure
  - File: `cli/pkg/installer/transaction/transaction.go`
  - Directory swap ordering:
    1. Stage complete → persist `staged`.
    2. Rename destination to unique trash sibling → persist `original-relocated`.
    3. Rename stage to destination.
    4. On step 3 failure: attempt trash-to-dest restoration.
    5. If restoration succeeds: retain stage until failure inventory persisted, then remove stage only when no recovery value.
    6. If restoration fails: retain stage, trash, backup, inventory; set `recovery-incomplete`; report manual recovery.
  - No best-effort cleanup removes retained recovery artifacts.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestDirectorySwapFailure -v` → PASS

- [x] **3.7 GREEN**: Implement ownership-aware rollback
  - File: `cli/pkg/installer/transaction/transaction.go`
  - Process mutated entries in reverse order.
  - Before changing destination: read it without following symlinks (`SafeFS`), compare to entry's recorded `InstalledIdentity`, digest, link value, and type.
  - If ownership matches and pre-state was absent: remove only transaction-owned destination.
  - If ownership matches and pre-state existed: stage restoration from backup with exact pre-state modes, atomic rename into place.
  - If ownership absent or ambiguous: do not remove or overwrite live path. Retain backup, inventory, stage/trash; mark `ownership-ambiguous`; continue with other entries.
  - Persist inventory after every material state transition.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestRollbackOwnership -v` → PASS

- [x] **3.8 GREEN**: Implement RecoveryState, RecoveryArtifact, partial recovery in report
  - File: `cli/pkg/installer/report/report.go`
  - Add `RecoveryState` type: `complete`, `incomplete`, `manual-recovery-required`.
  - Add `RecoveryArtifact` struct: destination, backup path, stage path, trash path, inventory path.
  - Add to `ExecutionReport`: recovery state, list of `RecoveryArtifact` for affected targets, safe next action string.
  - Distinguish complete rollback (no recovery artifacts) from partial (artifacts listed).
  - Implement `RollbackError` aggregating per-target failures and inventory-persistence failure via `errors.Join`.
  - Run: `cd cli && go test ./pkg/installer/report/ -run TestPartialRecovery -v` → PASS; `cd cli && go test ./pkg/installer/transaction/ -run TestCombinedRollbackError -v` → PASS

### TRIANGULATE — Edge cases

- [ ] **3.9 TRIANGULATE**: Rollback reverse ordering with multi-target plans
  - File: `cli/pkg/installer/transaction/rollback_test.go`
  - Create plan with 3+ targets, mutate all, then rollback.
  - Assert rollback processes in reverse mutation order.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestRollbackReverseOrder -v` → PASS

- [ ] **3.10 TRIANGULATE**: Rollback continuation after one target fails
  - File: `cli/pkg/installer/transaction/rollback_test.go`
  - Inject failure on one target's rollback (e.g., backup unreadable).
  - Assert other targets still rolled back; failed target's artifacts retained; report lists both succeeded and failed targets.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestRollbackContinuation -v` → PASS

- [x] **3.11 TRIANGULATE**: Symlink rollback ownership
  - File: `cli/pkg/installer/transaction/rollback_test.go`
  - Install symlink target, then externally replace with different symlink (different link value).
  - Assert rollback does not overwrite; marks ownership-ambiguous; retains artifacts.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -run TestRollbackSymlinkOwnership -v` → PASS

### REFACTOR — Clean up and document

- [ ] **3.12 REFACTOR**: Consolidate error aggregation
  - File: `cli/pkg/installer/transaction/transaction.go`
  - Ensure all rollback + persistence error paths use `errors.Join` consistently.
  - Add helper for building `RollbackError` with clear separation of operation vs persistence failures.
  - Run: `cd cli && go test ./pkg/installer/transaction/ -v` → all PASS

- [ ] **3.13 REFACTOR**: Document recovery-incomplete state machine
  - File: `cli/pkg/installer/transaction/inventory.go`, `cli/pkg/installer/report/report.go`
  - Add doc comment on inventory lifecycle state transitions, especially `recovery-incomplete`.
  - Document what manual recovery entails (inspect inventory, retained artifacts; do not auto-delete).
  - Run: `cd cli && go doc ./pkg/installer/transaction/ ./pkg/installer/report/` → readable

**PR 3 verification**: `cd cli && go test ./...` → all PASS. `go vet ./...` → clean. `git diff --stat` (vs PR 2 branch) → ≤350 lines. Full integration: `cd cli && go test ./pkg/installer/... -v` → all plan, transaction, and report tests PASS.

---

## Summary: PR Boundaries and Verification

| PR | Branch | Targets | Estimated lines | Key verification |
|----|--------|---------|-----------------|------------------|
| 1 | `feat/safe-filesystem-corrections/pr1-bound-source-modes` | tracker | ~220–330 | `go test ./pkg/installer/plan/ ./pkg/installer/transaction/` |
| 2 | `feat/safe-filesystem-corrections/pr2-protected-metadata` | PR 1 branch | ~170–280 | `go test ./pkg/installer/transaction/` |
| 3 | `feat/safe-filesystem-corrections/pr3-recoverable-rollback` | PR 2 branch | ~220–350 | `go test ./...` |

Each PR: RED → GREEN → TRIANGULATE → REFACTOR. Tests in same commit as behavior. No TUI or historical-slice changes.
