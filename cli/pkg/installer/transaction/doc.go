// Package transaction executes managed filesystem mutations with retained backups
// and automatic rollback.
//
// Lifecycle states
//
// The inventory lifecycle captures the durable state of each execution attempt:
//
//	prepared → committing → completed                       (success)
//	             ↓
//	         commit-failed → rolling-back → rolled-back      (automatic recovery, full)
//	                               ↓
//	                        recovery-incomplete               (partial recovery, manual)
//
// The lifecycle transitions are:
//   - prepared: inventory allocated, backup roots created, no targets mutated
//   - committing: mutations in progress; a persisted committing state means
//     the crash window is open
//   - completed: all mutations committed successfully; no rollback needed
//   - commit-failed: a mutation failed; rollback is triggered automatically
//     when Execute calls Rollback
//   - rolling-back: restoration in progress; a persisted rolling-back state
//     means the crash window is open during rollback
//   - rolled-back: all mutated targets were restored to their pre-installation
//     state; backups are retained for verification
//   - recovery-incomplete: one or more targets could not be restored; the
//     ExecutionReport contains RecoveryArtifacts (backup, stage, trash paths)
//     and RecoveryState is manual-recovery-required
//
// Entry states track each target through the lifecycle:
//
//	pending → backed-up → staged → original-relocated → mutated → restored
//	   ↓          ↓                                    ↓
//	source-drift  failed                             ownership-ambiguous
//
// Backup safety
//
// Backup roots are created under ~/.dots-backups/<run-id>/ with mode 0700,
// verified owned by the effective user, and never group/world writable. All
// backup-root operations use descriptor-relative syscalls (openat, mkdirat,
// renameat, symlinkat) to prevent path-substitution races after validation.
package transaction
