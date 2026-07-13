# Apply Progress: safe-dotfiles-installer-filesystem-corrections

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
- GREEN: execution reports now expose recovery state, exact retained artifact paths, and the conservative manual-recovery instruction.
- Focused verification: `cd cli && go test ./pkg/installer/transaction ./pkg/installer/report -count=1` passed.
- Full verification: `cd cli && go test ./... -count=1 && go vet ./... && go build ./...`; `git diff --check` passed.
- Judgment Day correction: combined restoration and inventory-persistence failure is now injected together; the returned joined error retains both causes, the failed target outcome and recovery artifacts remain reportable, and lifecycle becomes `recovery-incomplete`.
- Judgment Day correction: an external replacement of an installed symlink is conservatively preserved during rollback; the entry becomes `ownership-ambiguous` and its backup is retained.
- TRIANGULATE 3.9: `TestTransaction_Rollback_ReverseOrder` validates rollback processes targets in reverse mutation order.
- TRIANGULATE 3.10: `TestTransaction_Rollback_ContinuesAfterRestoreFailure` validates rollback continues after individual failures, retaining failed target artifacts.
- REFACTOR 3.12: Extracted `recordRollbackFailure` helper to consolidate repetitive failure-recording and inventory-persistence pattern in `Rollback()`. The helper separates operation failures (RollbackError) from persistence failures (`errors.Join`).
- REFACTOR 3.13: Added per-value doc comments for `InventoryLifecycle` (state machine transitions) and `InventoryEntryState` (normal/failure flows). Added detailed doc for `RecoveryState`, `RecoveryArtifact`, and `ManualRecoveryNextAction` in `report`.
