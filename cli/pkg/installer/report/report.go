// Package report provides pure, typed execution outcomes for the installer.
package report

import (
	"fmt"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

// TargetStatus records the outcome of a managed target.
type TargetStatus string

const (
	TargetPending  TargetStatus = "pending"
	TargetBackedUp TargetStatus = "backed-up"
	TargetMutated  TargetStatus = "mutated"
	TargetRestored TargetStatus = "restored"
	TargetFailed   TargetStatus = "failed"
)

// ActionStatus records the outcome of an external action.
type ActionStatus string

const (
	ActionPending   ActionStatus = "pending"
	ActionCompleted ActionStatus = "completed"
	ActionFailed    ActionStatus = "failed"
	ActionSkipped   ActionStatus = "skipped"
)

// TargetOutcome carries the result for one managed target.
type TargetOutcome struct {
	Destination string
	Status      TargetStatus
	BackupPath  string
	Error       error
}

// ActionOutcome carries the result for one external action.
type ActionOutcome struct {
	Description string
	Status      ActionStatus
	Error       error
}

// RecoveryState describes whether automatic rollback resolved all targets or
// left safe recovery work for the user.
type RecoveryState string

const (
	// RecoveryComplete means all mutated targets were successfully restored.
	RecoveryComplete RecoveryState = "complete"

	// RecoveryIncomplete means rollback finished but one or more targets could
	// not be restored. No retained artifacts require manual intervention.
	RecoveryIncomplete RecoveryState = "incomplete"

	// RecoveryManualRecoveryRequired means rollback finished with ambiguous
	// or failed targets that were NOT restored. The retained artifacts
	// (backups, stage copies, trash, inventory) must be inspected manually.
	// The installer will not auto-delete or overwrite any of these paths.
	RecoveryManualRecoveryRequired RecoveryState = "manual-recovery-required"
)

// RecoveryArtifact names every retained path that may be needed for manual
// recovery when rollback could not restore one or more targets.
type RecoveryArtifact struct {
	Destination   string // the original target path (may contain externally-changed content)
	BackupPath    string // retained backup of the original destination content
	StagePath     string // retained staged copy that was not moved into place
	TrashPath     string // retained original destination that was relocated during a swap
	InventoryPath string // path to the full inventory JSON for this plan run
}

// ManualRecoveryNextAction is deliberately conservative: the installer cannot
// safely determine ownership of externally-changed paths, so it will not
// delete or overwrite them. The user must inspect the named inventory file,
// cross-reference retained artifacts, and decide manually which paths to
// restore or clean up.
const ManualRecoveryNextAction = "inspect named inventory and retained artifacts; do not delete or overwrite ambiguous paths"

// ExecutionReport is the immutable, typed result of an installation attempt.
type ExecutionReport struct {
	Fingerprint        string
	InventoryPath      string
	ManagedTargets     []TargetOutcome
	ExternalActions    []ActionOutcome
	BackupPaths        []string
	PrimaryCause       error
	RollbackFailures   []TargetOutcome
	RecoveryState      RecoveryState
	RecoveryArtifacts  []RecoveryArtifact
	RecoveryNextAction string
}

// AttemptOutcome is the aggregate result of a two-phase installation attempt.
type AttemptOutcome string

const (
	OutcomeCompleted  AttemptOutcome = "completed"
	OutcomeIncomplete AttemptOutcome = "incomplete"
	OutcomeFailed     AttemptOutcome = "failed"
	OutcomeCancelled  AttemptOutcome = "cancelled"
)

// InstallPhase identifies one phase of a two-phase installation.
type InstallPhase string

const (
	PhasePackage       InstallPhase = "package"
	PhaseRepository    InstallPhase = "repository"
	PhaseConfiguration InstallPhase = "configuration"
)

// PhaseState is the state of a single phase within a two-phase installation.
type PhaseState string

const (
	PhaseNotStarted PhaseState = "not-started"
	PhaseCompleted  PhaseState = "completed"
	PhaseFailed     PhaseState = "failed"
	PhaseSkipped    PhaseState = "skipped"
	PhaseCancelled  PhaseState = "cancelled"
)

// TransactionState is the state of the managed transaction in the configuration phase.
type TransactionState string

const (
	TransactionNotStarted  TransactionState = "not-started"
	TransactionStarted     TransactionState = "started"
	TransactionCompleted   TransactionState = "completed"
	TransactionNotRequired TransactionState = "not-required"
)

// PhaseExecution is the result of a single phase that produced an execution report.
type PhaseExecution struct {
	State           PhaseState
	PlanFingerprint string
	Report          *ExecutionReport
}

// ConfigurationExecution extends PhaseExecution with the managed transaction state.
type ConfigurationExecution struct {
	PhaseExecution
	TransactionState TransactionState
	InventoryPath    string
}

// RepositoryExecution records the result of repository acquisition.
type RepositoryExecution struct {
	State       PhaseState
	Destination string
	Ref         string
	Cause       error
}

// TwoPhaseExecutionReport is the aggregate result of a package phase, repository
// acquisition, and configuration phase that share a single run identity.
type TwoPhaseExecutionReport struct {
	RunID              string
	Outcome            AttemptOutcome
	PrimaryFailedPhase InstallPhase
	Package            PhaseExecution
	Repository         RepositoryExecution
	Configuration      ConfigurationExecution
}

// PlanError represents a failure during the planning phase.
type PlanError struct {
	Phase string
	Cause error
}

func (e *PlanError) Error() string { return fmt.Sprintf("plan error during %s: %v", e.Phase, e.Cause) }
func (e *PlanError) Unwrap() error { return e.Cause }

// PlanDriftError indicates the target state changed after planning.
type PlanDriftError struct {
	Target   plan.Target
	Expected plan.PreState
	Actual   plan.PreState
}

func (e *PlanDriftError) Error() string {
	return fmt.Sprintf("plan drift for %s", e.Target.Destination)
}

// BackupError represents a failure to create or validate a backup.
type BackupError struct {
	Target plan.Target
	Cause  error
}

func (e *BackupError) Error() string {
	return fmt.Sprintf("backup failed for %s: %v", e.Target.Destination, e.Cause)
}

// MutationError represents a failure during a managed-target mutation.
type MutationError struct {
	Target plan.Target
	Cause  error
}

func (e *MutationError) Error() string {
	return fmt.Sprintf("mutation failed for %s: %v", e.Target.Destination, e.Cause)
}

// RollbackError aggregates failures that occurred while restoring managed targets.
type RollbackError struct {
	Failures []TargetOutcome
}

func (e *RollbackError) Error() string {
	return fmt.Sprintf("rollback incomplete: %d failure(s)", len(e.Failures))
}

// ExternalActionError represents a failure in an external command.
type ExternalActionError struct {
	Action plan.ExternalAction
	Cause  error
}

func (e *ExternalActionError) Error() string {
	return fmt.Sprintf("external action %q failed: %v", e.Action.Description, e.Cause)
}

func (e *ExternalActionError) Unwrap() error { return e.Cause }
