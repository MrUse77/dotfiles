# Safe filesystem transaction corrections

## Decision

Replace path-based check-then-use operations at the transaction boundary with a POSIX descriptor-relative safety layer, and make the inventory the durable record of a transaction state machine. Planner-built targets carry a source binding; legacy direct `plan.Target` values without `SourceDigest` retain the current compatibility behavior.

The implementation is confined to `cli/pkg/installer/{plan,transaction,report}`. It targets Unix-like systems that provide `openat`/`fstatat`/`renameat` semantics through `golang.org/x/sys/unix`. On platforms where the required no-follow, descriptor-relative guarantees are unavailable, the safety layer returns an unsupported-platform error before mutation; it must not fall back to path-based traversal.

## Review path

1. Review source-binding and exact-mode primitives first: they establish the content and metadata that can be installed.
2. Review backup-root and inventory durability second: they establish durable recovery evidence before mutation.
3. Review swap, ownership, rollback, and reporting last: they consume the prior guarantees and preserve recovery artifacts on failure.

## Current boundary and required changes

| Current symbol | Gap | Design change |
|---|---|---|
| `plan.Target` | Holds resolved path and digest but no immutable object identity/type binding | Add unexported/serialized source-binding metadata: planned object type, device/inode (where supported), and symlink link value. Keep `SourceDigest` as the compatibility gate. |
| `Planner.Build` / `resolveSource` | Resolves and later reopens paths independently | Capture digest and identity from the same no-follow handles used to inspect the source tree; planner always fills binding metadata and `SourceDigest`. |
| `plan.SourceDigestForPath` and `directoryDigest` | `Stat`, `WalkDir`, and `Open` can traverse a substituted path | Replace execution use with descriptor-bound snapshot/reader APIs; retain the public helper only where compatibility tests need it, but do not use it to authorize mutation. |
| `transaction.Filesystem` / `copyFile` / `copyTree` | Cannot express descriptor-relative no-follow traversal, inode checks, or secure directory creation | Add a narrowly scoped `SafeFS` capability backed by `unix`; transaction requires it for bound targets and fails closed if unavailable. Preserve the existing fake-friendly `Filesystem` seam for ordinary operation/failure injection. |
| `Transaction.ensureBackupRoot` | `Mkdir`/`Chmod` trusts path components | Walk the root chain from a trusted directory descriptor using `openat(..., O_DIRECTORY|O_NOFOLLOW)` and `mkdirat`; validate owner and exact safe permissions before every use. |
| `persistInventory` / `writeFile` | Writes `inventory.json` in place, then chmods | Atomically write a 0600 temporary sibling, sync it, rename it, then sync the parent directory. Only the final filename is authoritative. |
| `Transaction.mutateTarget` | Backs up destination before source binding check | Verify and open the source binding before backup, stage, or rename. |
| `commitTree` / `restoreDirectory` | Failure cleanup can erase stage/trash artifacts | Record swap phase and artifact paths before each rename; preserve stage/trash/backup when recovery is incomplete. |
| `Rollback` | `RemoveAll` and restore overwrite paths without proving ownership | Compare the live destination against the installed artifact identity/digest before deleting or restoring. |
| `plan.PreState`, copy helpers | Store/apply only `.Perm()` | Capture and apply `os.FileMode` permission plus supported special bits exactly; mask only non-persistable type bits. |
| `report.ExecutionReport` | Cannot express manual recovery locations or incomplete result | Add recovery state, per-target retained paths, and safe next action; distinguish complete rollback from partial recovery. |

## Data model and contracts

### Source binding

Add a `SourceBinding` value to `plan.Target` (or equivalent fields kept inside the immutable plan), populated only by `Planner.Build`:

- `Kind`: planned regular file, directory, or symlink shape.
- `Identity`: device/inode for the opened root object where the OS supports it.
- `Digest`: the existing SHA-256 `SourceDigest` value.
- `LinkValue`: the literal source link value for a symlink target.
- `TreeManifest`: deterministic relative entries containing type, full applicable mode, link value, digest, and object identity as read from descriptor-relative traversal.

`SourceDigest == ""` is the explicit legacy-direct-target compatibility boundary. Such a target remains executable using existing semantics, but its execution must be marked internally as unbound and may never cause a bound target to skip validation. Planner construction rejects any target for which it cannot produce a non-empty digest and complete binding.

The plan fingerprint includes all binding fields, so the reviewed plan is bound to what the planner observed.

### Durable inventory

Extend `transaction.Inventory` with a format version, lifecycle state, authoritative inventory path, and each entry's operation/recovery data:

- lifecycle: `prepared`, `committing`, `commit-failed`, `rolling-back`, `rolled-back`, `recovery-incomplete`, `completed`;
- entry state: pending, source-drift, backed-up, staged, original-relocated, mutated, restored, ownership-ambiguous, failed;
- `InstalledIdentity`/digest and installed mode used to prove rollback ownership;
- retained `BackupPath`, `StagePath`, `TrashPath`, and a string error description.

The JSON remains human-readable. The format gets an explicit version and new fields are additive; no incompatible inventory migration is introduced. The final `inventory.json` is authoritative only after atomic rename. Temporary names are recognizable as non-authoritative and recovery never reads them as the inventory.

### Recovery result

Add `RecoveryState` and `RecoveryArtifact` (destination, backup, stage, trash, inventory path) to `report`. `ExecutionReport` exposes whether rollback was complete, incomplete, or manual recovery is required, along with a conservative next action: inspect the named inventory and retained artifacts; do not delete or overwrite ambiguous paths. `RollbackError` aggregates both per-target failures and a separately identifiable inventory-persistence failure using `errors.Join`.

## Execution flow

### 1. Planning

`Planner.Build` normalizes candidates, then opens every source through the safe reader without following substituted links:

- a regular-file target is opened with `O_NOFOLLOW`, `fstat`ed, hashed from that descriptor, and bound to that descriptor identity;
- a directory root is opened as a directory descriptor, walked with `openat`/`fstatat(..., AT_SYMLINK_NOFOLLOW)`, and each file is hashed from its opened descriptor; no child is reopened by absolute path;
- a source symlink is inspected with `readlinkat` and its link value plus resolved, descriptor-verified content are represented in the binding.

An unreadable object, unsupported type, symlink escape, identity mismatch during inspection, or unstable tree produces a planning error. The planner keeps existing repository containment validation, but descriptor proof—not `EvalSymlinks` alone—is the authority.

### 2. Prepare and persist

For every target, `Prepare` derives the deterministic run-scoped backup path already supplied by `plan.BackupPath`, detects collisions across the complete target set, and validates the backup chain before writing anything. The chain is opened component-by-component from a trusted user-owned directory descriptor; each component must be a real directory, current-user-owned, and non-group/world-writable. New `.dots-backups` and run directories are created with 0700 and verified after creation.

After all entries are represented, persist `prepared` inventory atomically before any managed destination mutation. Revalidate the root descriptor before each backup creation and inventory replacement, preventing a check/use substitution of the root.

### 3. Bind source before mutation

At the start of `mutateTarget`, before destination backup, acquire a `BoundSource` handle. It opens the source no-follow, verifies type and identity against the binding, calculates/verifies the digest using the same open descriptors, and returns a reader/copy manifest that is consumed directly by staging. Any mismatch, open failure, or ambiguity marks the entry `source-drift`, persists the inventory, and performs no backup, staging, or rename for that target.

Legacy direct targets skip this binding acquisition only when `SourceDigest` is empty. They still run existing destination pre-state checks and safe backup/inventory rules.

### 4. Stage and commit

Files and symlinks are staged as a sibling in the destination directory, populated from `BoundSource`, synced, chmod'd to the exact planned mode, then atomically renamed. `chmod` happens before rename and receives the full supported permission/special-bit mask, not `.Perm()`. The installed identity/digest is measured from the staged/opened object and recorded in the inventory before/after rename as appropriate.

Directory copies use the bound tree manifest and descriptor-relative reads. Each created child receives its exact source mode after content creation; directory modes are applied after children so traversal remains possible. The top-level staged directory mode is finalized before the swap.

For replacement directories, persist an entry transition before each irreversible action:

1. `staged`: stage path retained and complete.
2. `original-relocated`: rename destination to a unique sibling trash path; inventory now names both locations.
3. Rename stage to destination.
4. `mutated`: record installed identity, retain backup, and only then remove trash on complete success.

If step 3 fails, attempt trash-to-destination restoration. If restoration succeeds, retain the stage until the failure inventory is persisted, then remove it only when it has no recovery value. If it fails, retain stage, trash, backup, and inventory, set `recovery-incomplete`, and report manual recovery. No best-effort cleanup may remove retained recovery artifacts.

### 5. Rollback

Rollback processes successfully mutated entries in reverse order. Before changing a destination, it reads it without following symlinks and compares it to the entry's recorded installed identity/digest/link value and type.

- If ownership matches and the pre-state was absent, remove only the transaction-owned destination.
- If ownership matches and pre-state existed, stage restoration from the protected backup with exact pre-state modes and atomically rename it into place.
- If ownership is absent or ambiguous, do not remove or overwrite the live path. Retain backup, inventory, stage/trash artifacts, mark `ownership-ambiguous`, and continue with other entries.

Persist the updated inventory after every material state transition. A persistence failure is joined with operation failures and forces `recovery-incomplete`; the returned report lists both independently.

## Integrity and filesystem rules

- Trusted handles, not string paths, are the authority for all hostile source and backup-root traversal.
- Destination-parent staging remains same-directory to preserve rename atomicity; unsafe or substituted parent resolution fails the target before mutation.
- Inventory temporary files use a unique sibling name, `O_CREAT|O_EXCL` plus 0600 at creation, write-all, `fsync`, close, atomic rename, and parent-directory sync. Errors leave the temporary file non-authoritative and are reported without corrupting a prior inventory.
- Backup files/directories are created through the validated run-root descriptor. Paths are retained for human recovery but never trusted later without revalidation.
- Supported exact modes include `ModePerm | ModeSetuid | ModeSetgid | ModeSticky`; file type bits are not passed to chmod. If the OS/filesystem refuses a requested supported special bit, commit fails before rename and preserves recovery state. Tests may skip unsupported filesystem semantics explicitly rather than silently accepting mode loss.
- The design does not promise safety against a hostile actor that can replace already-open descriptors, control the process, or has equivalent privileges.

## File plan

| Area | Planned changes |
|---|---|
| `cli/pkg/installer/plan/plan.go`, `planner.go`, `state.go` | Source-binding model, descriptor-bound planner inspection/digest manifest, full mode capture, fingerprint serialization. |
| `cli/pkg/installer/transaction/filesystem.go` plus Unix-specific implementation | Safe descriptor-relative interface and OS implementation; failure-injectable adapter for transaction tests. |
| `cli/pkg/installer/transaction/inventory.go` | Versioned lifecycle/entry persistence, secure atomic writer, root-scoped persistence. |
| `cli/pkg/installer/transaction/transaction.go` | Bind-before-backup sequencing, safe staging/swap state updates, ownership-aware rollback, retained-artifact handling. |
| `cli/pkg/installer/report/report.go` | Partial-recovery state, artifact locations, safe next action, combined rollback/persistence reporting. |
| focused `*_test.go` files in `plan`, `transaction`, and `report` | Hostile path, interruption, swap/rollback, exact-mode, compatibility, and report assertions. |

## Test strategy

Use table-driven tests and `t.TempDir()` for all filesystem scenarios. Keep syscall-specific tests behind supported-platform checks; use the injected filesystem adapter to fail every write, sync, chmod, and rename boundary deterministically.

- Planner: regular file, directory, and symlink bindings are non-empty and fingerprinted; source type/content/link substitution fails before mutation; direct digest-less targets remain executable.
- Source consumption: replace the source or a directory child with a symlink between inspection and consumption; prove no attacker content is copied and no backup/stage is created.
- Backup/inventory: intermediate and final symlink insertion, foreign/unsafe parent ownership/mode, root revalidation, 0700 roots, atomic interruption before rename, 0600 temporary/final modes, and valid prior inventory preservation.
- Commit: exact normal/special modes for files and trees, staging rename failures, and failure at each directory swap boundary with retained paths/inventory.
- Rollback: owned file/tree/link restoration, absent-target deletion only when owned, externally substituted destinations preserved, reverse ordering, continuation after one failure, and joined rollback plus inventory-persist errors.
- Reporting: complete rollback never claims partial state; incomplete recovery includes affected target, inventory, backup, stage/trash locations, and manual-recovery instruction.

Run narrow package tests first, then `go test ./...` from `cli/`. Strict TDD is required: each behavior starts with a focused failing test, then the minimum production change, then the relevant package and full suite.

## Chained PR plan

Estimated scope is 500–900 changed lines, so this MUST be chained with no `size:exception`. The user selected ask-always delivery; the chain strategy remains an explicit pre-apply decision. The slices below are strategy-neutral and each includes its own tests. Once selected, use one strategy consistently (recommended: feature-branch chain because all three slices form one security transaction boundary).

| PR | Boundary and expected budget | Depends on | Verification / out of scope |
|---|---|---|---|
| 1 — Bound source and exact modes | `plan` source-binding/manifest plus descriptor-bound consumption primitives; bind-before-backup; full mode application; direct-target compatibility tests. **~220–330 lines.** | None | Targeted `plan` and transaction source/mode tests. Does not add backup-root/inventory lifecycle or rollback changes. |
| 2 — Protected recovery metadata | Safe backup-root traversal/validation, collision checks, versioned inventory data, atomic 0600 persistence, root revalidation, interruption tests. **~170–280 lines.** | PR 1 | Transaction inventory/root tests. Does not change ownership-aware rollback or directory swap recovery. |
| 3 — Recoverable swaps and rollback | Persisted swap phases, directory failure preservation, installed ownership proof, conservative rollback, partial-recovery report/error model. **~220–350 lines.** | PR 2 | Transaction/report failure-injection and rollback tests plus `go test ./...`. No TUI or historical-slice changes. |

Dependency diagram for each child PR (mark the current PR):

```text
PR 1 Bound source/modes  →  PR 2 Protected metadata  →  PR 3 Recoverable rollback
```

Before apply, record the chosen chain strategy, first PR boundary, branch bases, and per-PR actual additions+deletions. Every PR description must state its predecessor, successor, scope, verification, and out-of-scope boundary; rebase/retarget any polluted diff.

## Rollout and rollback

There is no data migration or automatic crash resume. Deploy slices in order. Existing inventory remains readable as the human-recovery record; new inventory is versioned and additive. If a runtime operation fails, preserve artifacts and present the inventory path—never automatically remove ambiguous state. Repository rollback is a revert of this independent change; retained runtime backups remain deliberately untouched.

## Design checklist

- [ ] Planner-built targets cannot omit source binding.
- [ ] Bound source verification and consumption use the same descriptors before destination mutation.
- [ ] Legacy direct targets with empty `SourceDigest` remain compatible without weakening bound targets.
- [ ] Backup roots and inventory writes never traverse unverified symlinks.
- [ ] Only atomically renamed, 0600 inventory is authoritative.
- [ ] Rollback acts only on artifacts it can prove the transaction owns.
- [ ] Every partial swap/rollback path retains and reports recovery artifacts.
- [ ] Modes retain normal and supported special bits exactly.
- [ ] Implementation stays under `cli/` and each PR stays below 400 changed lines.
