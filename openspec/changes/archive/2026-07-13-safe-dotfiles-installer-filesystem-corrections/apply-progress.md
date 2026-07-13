# Apply Progress: safe-dotfiles-installer-filesystem-corrections

## TDD Cycle Evidence

The rows marked **recovered historical evidence** are traceable to the completed task narratives and merged PR #12 commit `06296b7`; they do not claim newly observed chronological output. Current-session results are labeled separately.

| Completed task group | RED evidence | GREEN evidence | TRIANGULATE evidence | REFACTOR / SAFETY-NET evidence |
|---|---|---|---|---|
| PR 1 tasks 1.1–1.4 (recovered historical evidence) | `cd cli && go test ./pkg/installer/plan/ -run TestSourceBinding -v`, `go test ./pkg/installer/transaction/ -run TestSourceDrift -v`, `go test ./pkg/installer/plan/ -run TestExactMode -v`, and `go test ./pkg/installer/plan/ -run TestLegacyDirectTarget -v` were recorded as FAIL before implementation. | The same focused SourceBinding, SourceDrift, ExactMode, and LegacyDirectTarget commands were recorded as PASS for tasks 1.5–1.10. | `TestTreeManifest`, `TestSymlinkDrift`, and `go test ./... -short` were recorded as PASS for 1.11–1.13. | `go test ./...` and `go vet ./...` were recorded as PASS for 1.14; `go doc ./pkg/installer/plan/` was recorded readable for 1.15. PR 1 corrections add root-mode, symlink X→Y→X, and coherent-capture regressions. |
| PR 2 tasks 2.1–2.4 (recovered historical evidence) | `go test ./pkg/installer/transaction/ -run TestBackupRootSymlink -v`, `-run TestAtomicInventory -v`, `-run TestInventorySchema -v`, and `-run TestBackupCollision -v` were recorded as FAIL before implementation. | Those focused commands were recorded as PASS for tasks 2.5–2.8. | `TestBackupRootIntermediateSymlink`, `TestBackupRootUnsafeParent`, and `TestInventoryTempName` were recorded as PASS for 2.9–2.11. | `go test ./pkg/installer/transaction/ -v` and `go vet ./...` were recorded as PASS for 2.12–2.13. The recovered PR 2 narrative records root-substitution, durable checkpoint, and sync-order safety-net regressions. |
| PR 3 tasks 3.1–3.4 (recovered historical evidence) | `go test ./pkg/installer/transaction/ -run TestRollbackOwnership -v`, `-run TestDirectorySwapFailure -v`, `go test ./pkg/installer/report/ -run TestPartialRecovery -v`, and `go test ./pkg/installer/transaction/ -run TestCombinedRollbackError -v` were recorded as FAIL before implementation. | The corresponding focused commands were recorded as PASS for tasks 3.5–3.8; PR #12 `06296b7` records the completed PR 3 delivery. | `TestTransaction_Rollback_ReverseOrder`, `TestTransaction_Rollback_ContinuesAfterRestoreFailure`, and `TestRollbackSymlinkOwnership` are recorded as the 3.9–3.11 alternate/failure-path checks. | `go test ./pkg/installer/transaction/ -v` and `go doc ./pkg/installer/transaction/ ./pkg/installer/report/` were recorded for 3.12–3.13; the combined restoration-plus-inventory persistence and external-symlink replacement regressions are recorded safety nets. |
| PR 3 report-test correction (current session) | `cd cli && go test ./pkg/installer/transaction ./pkg/installer/report -run 'TestTransaction_Execute_DirectoryRelocationCheckpointFailureRequiresManualRecovery|TestManualRecoveryNextActionIsConservative' -count=1 -v` failed: the initial assertion incorrectly expected `PrimaryCause` to unwrap the injected error and zero rollback failures. | The transaction test passed after asserting the produced `ExecutionReport`'s wrapped primary-cause text, manual-recovery state/action, retained artifact paths, and one failed rollback outcome. The non-behavioral constant-only report test, `TestManualRecoveryNextActionIsConservative`, was then removed; report coverage is provided by those produced-report assertions. | `cd cli && go test ./pkg/installer/report ./pkg/installer/transaction -count=1` passed. | `cd cli && go test ./...`, `go test -cover ./...`, `go vet ./...`, `go build ./...`, and `go fmt ./...` passed; `git diff --check` is recorded after this update. |

## PR 2 — Protected Recovery Metadata

Complete.

- Retained validated backup-root descriptors now anchor inventory temporary-file creation, sync, and atomic `renameat`; root substitution before rename is covered.
- Inventory lifecycle and entry recovery outcomes are persisted through rollback completion or incomplete recovery.
- TDD evidence: `cd cli && go test ./pkg/installer/transaction -count=1` passes.
- Judgment Day remediation: backup creation remains anchored to the validated backup-root descriptor, including descriptor-relative file/tree/symlink writes. A substitution-after-validation regression confirms no attacker-root write.
- Judgment Day remediation: after backup validation, a durable `backed-up` inventory checkpoint is written before mutation. The lifecycle regression reads the checkpoint on disk and confirms the original destination remains unchanged.
- Judgment Day remediation: descriptor-relative backup fsync ordering now flushes each regular file after metadata, each completed directory, and the backup root after its entry changes. Failure injection at every required sync proves no `backed-up` checkpoint or destination mutation proceeds.
- Verification: `cd cli && go test ./pkg/installer/transaction -run TestTransaction_Commit_BackupSyncFailurePreventsCheckpointAndMutation -count=1 && go test ./... -count=1 && go vet ./... && go build ./...`; `git diff --check` passed.

## PR 3 — Recoverable Swaps and Ownership-Aware Rollback

Complete.

- RED evidence: `go test ./pkg/installer/transaction -run 'Test(RollbackOwnership|DirectorySwapFailure)' -count=1 -v` failed before implementation because rollback overwrote an externally replaced target and swap paths were not retained.
- GREEN: installed digest, mode, and device/inode identity are recorded after commit; rollback refuses ambiguous live destinations, retains recovery artifacts, and persists `ownership-ambiguous`.
- GREEN: directory replacement persists `staged` and `original-relocated` before renames. An unrecoverable final swap failure retains stage, trash, backup, and inventory and marks recovery incomplete.
- GREEN: execution reports now expose recovery state, exact retained artifact paths, and the conservative manual-recovery instruction. `TestTransaction_Execute_DirectoryRelocationCheckpointFailureRequiresManualRecovery` asserts the produced `ExecutionReport`; the removed constant-only report test is not treated as behavioral coverage.
- Focused verification: `cd cli && go test ./pkg/installer/transaction ./pkg/installer/report -count=1` passed.
- Full verification: `cd cli && go test ./... -count=1 && go vet ./... && go build ./...`; `git diff --check` passed.
- Judgment Day correction: combined restoration and inventory-persistence failure is now injected together; the returned joined error retains both causes, the failed target outcome and recovery artifacts remain reportable, and lifecycle becomes `recovery-incomplete`.
- Judgment Day correction: an external replacement of an installed symlink is conservatively preserved during rollback; the entry becomes `ownership-ambiguous` and its backup is retained.
- TRIANGULATE 3.9: `TestTransaction_Rollback_ReverseOrder` validates rollback processes targets in reverse mutation order.
- TRIANGULATE 3.10: `TestTransaction_Rollback_ContinuesAfterRestoreFailure` validates rollback continues after individual failures, retaining failed target artifacts.
- REFACTOR 3.12: Extracted `recordRollbackFailure` helper to consolidate repetitive failure-recording and inventory-persistence pattern in `Rollback()`. The helper separates operation failures (RollbackError) from persistence failures (`errors.Join`).
- REFACTOR 3.13: Added per-value doc comments for `InventoryLifecycle` (state machine transitions) and `InventoryEntryState` (normal/failure flows). Added detailed doc for `RecoveryState`, `RecoveryArtifact`, and `ManualRecoveryNextAction` in `report`.
