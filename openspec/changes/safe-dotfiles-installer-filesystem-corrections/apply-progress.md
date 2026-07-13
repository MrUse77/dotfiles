# Apply Progress: safe-dotfiles-installer-filesystem-corrections — PR 1

## Status

- **Change**: `safe-dotfiles-installer-filesystem-corrections`
- **Artifact store**: OpenSpec files
- **Delivery**: feature-branch-chain, PR 1 of 3, target tracker branch
- **Size exception**: user-granted (`exception-ok`)
- **Strict TDD**: active; test runner `cd cli && go test ./...`

## PR 1 Scope Completed

Hostile-safe descriptor-bound consumption for planner-bound sources and exact
mode preservation, preserving legacy direct `Target` values with empty
`SourceDigest`. No PR 2 or PR 3 features were added.

## What changed

### Production code

- `cli/pkg/installer/plan/source_binding.go` (new)
  - `SourceBinding`, `FileIdentity`, `TreeManifestEntry` models.
  - Descriptor-bound planner inspection for files and directories using
    `unix.Open`/`Openat`/`Fstatat`/`Readlinkat` with `O_NOFOLLOW`.
  - `FileModeFromUnix` helper to correctly convert raw `st_mode` bits to
    `os.FileMode`.
  - `buildSourceBinding` fills `SourceDigest` and `SourceBinding` for planner
    targets; legacy direct targets leave both empty.

- `cli/pkg/installer/plan/plan.go`
  - `Target.SourceBinding` field and `NewInstallationPlan` constructor for
    direct internal targets.
  - Fingerprint serialization includes `SourceBinding`.
  - Doc comment on `Target.SourceDigest` explaining the legacy compatibility
    boundary.

- `cli/pkg/installer/plan/planner.go`
  - Uses `buildSourceBinding` instead of path-only `resolveSource`+`sourceDigest`.

- `cli/pkg/installer/plan/state.go`
  - `supportedMode` captures `ModePerm|ModeSetuid|ModeSetgid|ModeSticky`.

- `cli/pkg/installer/transaction/filesystem.go`
  - `chmodMode` applies the same supported-bit mask for tree copies.
  - Directory modes applied after children are created.

- `cli/pkg/installer/transaction/transaction.go`
  - `validateSourceDigest` opens files and directories with `O_NOFOLLOW`,
    verifies identity, and for directories verifies the full `TreeManifest`
    before any backup/staging.
  - Descriptor-bound directory copy `copyTreeBound` verifies each entry
    (type/identity/mode/digest/link value) while copying.
  - `commitTree` uses the bound directory handle for planner-built targets;
    legacy direct targets continue to use the existing path-based `copyTree`.
  - Legacy direct targets (`SourceDigest == ""`) skip binding acquisition.

### Tests

- `cli/pkg/installer/plan/plan_test.go`
  - `TestSourceBinding_PlannerBindsFileIdentityAndFingerprint`
  - `TestSourceBinding_DirectoryTreeManifest`

- `cli/pkg/installer/plan/state_test.go`
  - `TestExactModeCapture`

- `cli/pkg/installer/transaction/transaction_test.go`
  - `TestTransaction_Execute_SourceDriftBeforeBackup`
  - `TestTransaction_Execute_LegacyDirectTarget`
  - `TestTransaction_Execute_ExactFileMode`
  - `TestTransaction_Execute_ExactDirectoryMode`
  - `TestTransaction_Execute_DirectorySourceDrift`
  - Updated `TestTransaction_Commit_SymlinkUsesBoundLinkValue` to expect drift
    when the link value changes after planning.

- `cli/pkg/installer/transaction/rollback_test.go`
  - Renamed `TestTransaction_Rollback_BackupsRetainedAfterRollback` to
    `TestTransaction_Rollback_SourceDriftDoesNotCreateBackup` and adjusted
    assertions for bind-before-backup behavior.

## TDD cycle evidence

| Cycle | RED test | Failure observed | GREEN change | Verification |
|-------|----------|------------------|--------------|--------------|
| SourceBinding file | `TestSourceBinding_PlannerBindsFileIdentityAndFingerprint` | Missing `SourceBinding` fields | Added `SourceBinding` model and builder | PASS |
| Source drift file | `TestTransaction_Execute_SourceDriftBeforeBackup` | Drift detected after backup | Moved source binding validation before backup | PASS |
| Exact modes | `TestExactModeCapture`, `TestTransaction_Execute_Exact*Mode` | `.Perm()` only | `supportedMode`/`chmodMode` full bitmask | PASS |
| Legacy direct | `TestTransaction_Execute_LegacyDirectTarget` | Direct targets rejected | Skip binding when `SourceDigest == ""` | PASS |
| Directory binding | `TestSourceBinding_DirectoryTreeManifest` | No `TreeManifest` | Descriptor-bound directory manifest builder | PASS |
| Directory drift | `TestTransaction_Execute_DirectorySourceDrift` | Tree drift not detected before backup | `verifyTreeManifest` before backup + `copyTreeBound` verify+copy | PASS |
| Directory modes | `TestTransaction_Execute_ExactDirectoryMode` | Special bits lost on directories | `copyTreeBound` applies child modes before parent | PASS |

## Verification commands

```bash
cd cli && go test ./...                                                      # PASS
cd cli && go vet ./...                                                       # clean
cd cli && go build ./...                                                     # clean
cd cli && go fmt ./...                                                       # formatted (PR1 files only)
cd /home/agustin/Dev/dotfiles-source-binding && git diff --check             # clean
```

## Deviations / notes

- The earlier executor’s apply-progress referenced a non-existent
  `internal/safefs` package. PR 1 was implemented without that abstraction;
  descriptor-bound Unix calls are used directly in `plan` and `transaction`.
  This keeps the change smaller and avoids introducing an unused package.
- The implementation uses `golang.org/x/sys/unix` directly and therefore
  targets Unix-like platforms. Cross-platform compilation for non-Unix would
  require build-tagged stubs; that is out of scope for PR 1.
- `TreeManifestEntry.Mode` stores only chmod-persistable bits
  (`ModePerm|ModeSetuid|ModeSetgid|ModeSticky`), not file-type bits.

## PR 1 exact-mode completion

- `backupTarget` now chmods a copied backup-directory root after recursive copying, so its original supported mode bits are restored despite umask.
- Added focused umask regression coverage for installed and backup files, plus top-level and nested installed and backup directories.

## Remaining work

PR 2 and PR 3 tasks remain unchecked in `tasks.md`:

- PR 2: protected recovery metadata (symlink-safe backup roots, atomic
  inventory, versioned schema, collisions).
- PR 3: recoverable swaps and rollback (ownership-aware rollback,
  directory-swap failure preservation, partial-recovery reporting).

## PR boundary

- **Current PR**: PR 1 — Bound Source and Exact Modes
- **Target**: tracker branch `feat/safe-filesystem-corrections`
- **Out of scope for this PR**: backup-root symlink safety, atomic inventory,
  swap/recovery reporting.

## PR 1 correction — RELIABILITY-001

- `SourceBinding.Mode` now records the supported root mode for planner-bound file
  and directory sources from the same descriptor used for their identity/content
  binding.
- Transaction validation compares that planned root mode before backup, staging,
  or destination mutation and returns `PlanDriftError` on a mode change.
- Added file and directory regressions that mutate the root mode after `Build`;
  both assert the original destination remains unchanged and no backup is made.
- Verified compatibility with direct empty-digest targets.

### Correction validation

```bash
cd cli && go test ./pkg/installer/plan/ ./pkg/installer/transaction/ -run 'TestSourceBinding_RecordsRootSourceMode|TestTransaction_Execute_Drift_RootSourceModeChangedAfterPlan' -count=1  # PASS
cd cli && go test ./pkg/installer/transaction/ -run 'TestTransaction_Execute_LegacyDirectTarget|TestTransaction_Execute_SourceDriftBeforeBackup|TestTransaction_Execute_DirectorySourceDrift' -count=1  # PASS
cd cli && go test ./... -count=1  # PASS
cd cli && go vet ./...  # clean
cd cli && go build ./...  # clean
```

## PR 1 correction — RELIABILITY-002

- Planner-bound symlink commits now use `SourceBinding.LinkValue` after
  `validateSourceDigest` verifies the current source link text. The mutable
  inventory value captured by `Prepare` is retained only for legacy direct
  targets with an empty `SourceDigest`.
- Added `TestTransaction_Commit_SymlinkConsumesPlannerBoundLinkValue`, which
  simulates X→Y→X between planning, preparation, and commit. It proves commit
  installs the reviewed planner value X rather than stale prepared value Y.
- `TestTransaction_Commit_SymlinkUsesBoundLinkValue` continues to prove that a
  changed link value is rejected with `PlanDriftError` before mutation.

### Correction validation

```bash
cd cli && go test ./pkg/installer/transaction/ -run 'TestTransaction_Commit_Symlink(ConsumesPlannerBoundLinkValue|UsesBoundLinkValue)$' -count=1  # PASS
```

## PR 1 correction — RISK-001 coherent source-binding capture

- `buildSourceBinding` now captures a declared source `Lstat` identity and symlink
  link text before resolving/opening the target, then rechecks those declared
  fields before accepting the descriptor-bound result.
- A capture mismatch retries up to three times. Persistent instability returns the
  typed `SourceBindingDriftError` through planning rather than returning a mixed
  declared-symlink identity/link value with a different resolved target digest.
- `sourceBindingCaptureHook` is an unexported deterministic test seam used to
  simulate substitutions during the capture window.
- Added regressions for a one-time substitution (retry returns a coherent second
  binding) and persistent substitution (bounded retry rejects planning).
