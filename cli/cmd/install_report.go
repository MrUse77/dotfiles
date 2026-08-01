package cmd

import (
	"fmt"
	"io"

	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// printTwoPhaseExecutionReport renders the aggregate result of a two-phase
// installation attempt. It is used only by the missing-clone route; the
// existing single-plan printer remains unchanged.
func printTwoPhaseExecutionReport(w io.Writer, r *report.TwoPhaseExecutionReport) {
	if r == nil {
		fmt.Fprintln(w, "No installation report.")
		return
	}

	fmt.Fprintf(w, "Run ID: %s\n", r.RunID)
	fmt.Fprintf(w, "Outcome: %s\n", r.Outcome)
	if r.Outcome != report.OutcomeCompleted && r.PrimaryFailedPhase != "" {
		fmt.Fprintf(w, "Primary failed phase: %s\n", r.PrimaryFailedPhase)
	}
	fmt.Fprintln(w)

	printPhase(w, "Package", r.Package)
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Repository acquisition: %s\n", r.Repository.State)
	if r.Repository.Destination != "" {
		fmt.Fprintf(w, "  Destination: %s\n", r.Repository.Destination)
	}
	if r.Repository.Ref != "" {
		fmt.Fprintf(w, "  Ref: %s\n", r.Repository.Ref)
	}
	if r.Repository.Cause != nil {
		fmt.Fprintf(w, "  Cause: %v\n", r.Repository.Cause)
	}
	fmt.Fprintln(w)

	printConfiguration(w, r.Configuration)

	if r.Outcome != report.OutcomeCompleted {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Package effects and the repository clone (if acquired) remain in place.")
		fmt.Fprintln(w, "Only managed targets are subject to automatic rollback.")
	}
}

func printPhase(w io.Writer, label string, phase report.PhaseExecution) {
	fmt.Fprintf(w, "%s phase: %s\n", label, phase.State)
	if phase.PlanFingerprint != "" {
		fmt.Fprintf(w, "  Plan fingerprint: %s\n", phase.PlanFingerprint)
	}
	if phase.Report == nil {
		return
	}
	for _, target := range phase.Report.ManagedTargets {
		fmt.Fprintf(w, "  managed %s: %s\n", target.Destination, target.Status)
		if target.BackupPath != "" {
			fmt.Fprintf(w, "    retained backup: %s\n", target.BackupPath)
		}
	}
	for _, action := range phase.Report.ExternalActions {
		fmt.Fprintf(w, "  external %s: %s\n", action.Description, action.Status)
		if action.Error != nil {
			fmt.Fprintf(w, "    error: %v\n", action.Error)
		}
	}
	if phase.Report.RecoveryState != "" {
		fmt.Fprintf(w, "  rollback: %s\n", phase.Report.RecoveryState)
	}
	if phase.Report.RecoveryNextAction != "" {
		fmt.Fprintf(w, "  next action: %s\n", phase.Report.RecoveryNextAction)
	}
	for _, artifact := range phase.Report.RecoveryArtifacts {
		fmt.Fprintf(w, "  retained artifact for %s:\n", artifact.Destination)
		if artifact.BackupPath != "" {
			fmt.Fprintf(w, "    backup: %s\n", artifact.BackupPath)
		}
		if artifact.StagePath != "" {
			fmt.Fprintf(w, "    stage: %s\n", artifact.StagePath)
		}
		if artifact.TrashPath != "" {
			fmt.Fprintf(w, "    trash: %s\n", artifact.TrashPath)
		}
		if artifact.InventoryPath != "" {
			fmt.Fprintf(w, "    inventory: %s\n", artifact.InventoryPath)
		}
	}
}

func printConfiguration(w io.Writer, cfg report.ConfigurationExecution) {
	fmt.Fprintf(w, "Configuration phase: %s\n", cfg.State)
	if cfg.PlanFingerprint != "" {
		fmt.Fprintf(w, "  Plan fingerprint: %s\n", cfg.PlanFingerprint)
	}
	fmt.Fprintf(w, "  Transaction state: %s\n", cfg.TransactionState)
	if cfg.InventoryPath != "" {
		fmt.Fprintf(w, "  Inventory path: %s\n", cfg.InventoryPath)
	}
	if cfg.Report == nil {
		if cfg.TransactionState == report.TransactionNotStarted {
			fmt.Fprintln(w, "  Configuration transaction was not started.")
		}
		return
	}
	for _, target := range cfg.Report.ManagedTargets {
		fmt.Fprintf(w, "  managed %s: %s\n", target.Destination, target.Status)
		if target.BackupPath != "" {
			fmt.Fprintf(w, "    retained backup: %s\n", target.BackupPath)
		}
	}
	for _, action := range cfg.Report.ExternalActions {
		fmt.Fprintf(w, "  external %s: %s\n", action.Description, action.Status)
		if action.Error != nil {
			fmt.Fprintf(w, "    error: %v\n", action.Error)
		}
	}
	if cfg.Report.RecoveryState != "" {
		fmt.Fprintf(w, "  rollback: %s\n", cfg.Report.RecoveryState)
	}
	if cfg.Report.RecoveryNextAction != "" {
		fmt.Fprintf(w, "  next action: %s\n", cfg.Report.RecoveryNextAction)
	}
	for _, artifact := range cfg.Report.RecoveryArtifacts {
		fmt.Fprintf(w, "  retained artifact for %s:\n", artifact.Destination)
		if artifact.BackupPath != "" {
			fmt.Fprintf(w, "    backup: %s\n", artifact.BackupPath)
		}
		if artifact.StagePath != "" {
			fmt.Fprintf(w, "    stage: %s\n", artifact.StagePath)
		}
		if artifact.TrashPath != "" {
			fmt.Fprintf(w, "    trash: %s\n", artifact.TrashPath)
		}
		if artifact.InventoryPath != "" {
			fmt.Fprintf(w, "    inventory: %s\n", artifact.InventoryPath)
		}
	}
}
