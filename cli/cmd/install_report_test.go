package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func TestPrintTwoPhaseExecutionReport_LabelsPhasesAndFingerprints(t *testing.T) {
	var out bytes.Buffer
	printTwoPhaseExecutionReport(&out, &report.TwoPhaseExecutionReport{
		RunID:   "run-1",
		Outcome: report.OutcomeCompleted,
		Package: report.PhaseExecution{
			State:           report.PhaseCompleted,
			PlanFingerprint: "pkg-fp",
			Report: &report.ExecutionReport{
				ExternalActions: []report.ActionOutcome{
					{Description: "base tools", Status: report.ActionCompleted},
				},
			},
		},
		Repository: report.RepositoryExecution{
			State:       report.PhaseCompleted,
			Destination: "/dst",
			Ref:         "main",
		},
		Configuration: report.ConfigurationExecution{
			PhaseExecution: report.PhaseExecution{
				State:           report.PhaseCompleted,
				PlanFingerprint: "cfg-fp",
				Report: &report.ExecutionReport{
					ManagedTargets: []report.TargetOutcome{
						{Destination: "/home/user/.zshrc", Status: report.TargetMutated, BackupPath: "/backup/zshrc"},
					},
					ExternalActions: []report.ActionOutcome{
						{Description: "enable power profiles", Status: report.ActionCompleted},
					},
					InventoryPath: "/inventory.json",
				},
			},
			TransactionState: report.TransactionCompleted,
			InventoryPath:    "/inventory.json",
		},
	})

	got := out.String()
	for _, want := range []string{"run-1", "completed", "Package phase", "pkg-fp", "Repository acquisition", "/dst", "main", "Configuration phase", "cfg-fp", "/home/user/.zshrc", "enable power profiles", "Inventory path", "/inventory.json"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	for _, avoid := range []string{"not started", "failed", "Primary failed phase"} {
		if strings.Contains(got, avoid) {
			t.Errorf("completed report should not contain %q:\n%s", avoid, got)
		}
	}
}

func TestPrintTwoPhaseExecutionReport_CompletedOnlyWhenAllPhasesComplete(t *testing.T) {
	var out bytes.Buffer
	printTwoPhaseExecutionReport(&out, &report.TwoPhaseExecutionReport{
		RunID:              "run-1",
		Outcome:            report.OutcomeIncomplete,
		PrimaryFailedPhase: report.PhaseRepository,
		Package: report.PhaseExecution{
			State: report.PhaseCompleted,
			Report: &report.ExecutionReport{
				ExternalActions: []report.ActionOutcome{
					{Description: "base tools", Status: report.ActionCompleted},
				},
			},
		},
		Repository: report.RepositoryExecution{
			State: report.PhaseFailed,
			Cause: errors.New("network unreachable"),
		},
		Configuration: report.ConfigurationExecution{
			PhaseExecution:   report.PhaseExecution{State: report.PhaseNotStarted},
			TransactionState: report.TransactionNotStarted,
		},
	})
	got := out.String()
	if !strings.Contains(got, "incomplete") {
		t.Errorf("output missing incomplete outcome:\n%s", got)
	}
	if !strings.Contains(got, "Primary failed phase: repository") {
		t.Errorf("output missing primary failed phase:\n%s", got)
	}
	if !strings.Contains(got, "Repository acquisition: failed") {
		t.Errorf("output missing repository failure:\n%s", got)
	}
	if !strings.Contains(got, "network unreachable") {
		t.Errorf("output missing repository cause:\n%s", got)
	}
	if !strings.Contains(got, "Configuration transaction was not started") {
		t.Errorf("output missing not-started message:\n%s", got)
	}
	if strings.Contains(got, "Inventory path") {
		t.Errorf("not-started report must not reference inventory:\n%s", got)
	}
	if !strings.Contains(got, "Package effects and the repository clone") {
		t.Errorf("output missing partial-effects message:\n%s", got)
	}
}

func TestPrintTwoPhaseExecutionReport_PackageFailureKeepsEarlierOutcomes(t *testing.T) {
	var out bytes.Buffer
	printTwoPhaseExecutionReport(&out, &report.TwoPhaseExecutionReport{
		RunID:              "run-1",
		Outcome:            report.OutcomeIncomplete,
		PrimaryFailedPhase: report.PhasePackage,
		Package: report.PhaseExecution{
			State: report.PhaseFailed,
			Report: &report.ExecutionReport{
				ExternalActions: []report.ActionOutcome{
					{Description: "base tools", Status: report.ActionCompleted},
					{Description: "install packages", Status: report.ActionFailed, Error: errors.New("pacman failed")},
					{Description: "change shell", Status: report.ActionSkipped},
				},
			},
		},
		Repository:    report.RepositoryExecution{State: report.PhaseNotStarted},
		Configuration: report.ConfigurationExecution{PhaseExecution: report.PhaseExecution{State: report.PhaseNotStarted}, TransactionState: report.TransactionNotStarted},
	})
	got := out.String()
	if !strings.Contains(got, "base tools: completed") {
		t.Errorf("output missing completed action:\n%s", got)
	}
	if !strings.Contains(got, "install packages: failed") {
		t.Errorf("output missing failed action:\n%s", got)
	}
	if !strings.Contains(got, "change shell: skipped") {
		t.Errorf("output missing skipped action:\n%s", got)
	}
	if !strings.Contains(got, "Primary failed phase: package") {
		t.Errorf("output missing primary failed phase:\n%s", got)
	}
}

func TestPrintTwoPhaseExecutionReport_ConfigurationFailureShowsManagedRollback(t *testing.T) {
	var out bytes.Buffer
	printTwoPhaseExecutionReport(&out, &report.TwoPhaseExecutionReport{
		RunID:              "run-1",
		Outcome:            report.OutcomeIncomplete,
		PrimaryFailedPhase: report.PhaseConfiguration,
		Package: report.PhaseExecution{
			State: report.PhaseCompleted,
			Report: &report.ExecutionReport{
				ExternalActions: []report.ActionOutcome{
					{Description: "base tools", Status: report.ActionCompleted},
				},
			},
		},
		Repository: report.RepositoryExecution{
			State:       report.PhaseCompleted,
			Destination: "/dst",
			Ref:         "main",
		},
		Configuration: report.ConfigurationExecution{
			PhaseExecution: report.PhaseExecution{
				State: report.PhaseFailed,
				Report: &report.ExecutionReport{
					ManagedTargets: []report.TargetOutcome{
						{Destination: "/home/user/.zshrc", Status: report.TargetRestored, BackupPath: "/backup/zshrc"},
					},
					ExternalActions: []report.ActionOutcome{
						{Description: "enable power profiles", Status: report.ActionFailed, Error: errors.New("systemctl failed")},
					},
					RecoveryState:      report.RecoveryComplete,
					InventoryPath:      "/inventory.json",
					RecoveryArtifacts:  []report.RecoveryArtifact{{Destination: "/home/user/.zshrc", BackupPath: "/backup/zshrc", InventoryPath: "/inventory.json"}},
					RecoveryNextAction: report.ManualRecoveryNextAction,
				},
			},
			TransactionState: report.TransactionCompleted,
			InventoryPath:    "/inventory.json",
		},
	})
	got := out.String()
	for _, want := range []string{"rollback: complete", "Inventory path", "/inventory.json", "retained backup: /backup/zshrc", "Package effects and the repository clone"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "Primary failed phase: configuration") {
		t.Errorf("output missing primary failed phase:\n%s", got)
	}
}

func TestPrintTwoPhaseExecutionReport_NilReport(t *testing.T) {
	var out bytes.Buffer
	printTwoPhaseExecutionReport(&out, nil)
	if !strings.Contains(out.String(), "No installation report") {
		t.Errorf("output = %q, want no-report message", out.String())
	}
}

func TestPrintTwoPhaseExecutionReport_NoManagedTargetsReportsTransactionNotRequired(t *testing.T) {
	var out bytes.Buffer
	printTwoPhaseExecutionReport(&out, &report.TwoPhaseExecutionReport{
		RunID:   "run-1",
		Outcome: report.OutcomeCompleted,
		Package: report.PhaseExecution{
			State: report.PhaseCompleted,
			Report: &report.ExecutionReport{
				ExternalActions: []report.ActionOutcome{
					{Description: "base tools", Status: report.ActionCompleted},
				},
			},
		},
		Repository: report.RepositoryExecution{
			State:       report.PhaseCompleted,
			Destination: "/dst",
			Ref:         "main",
		},
		Configuration: report.ConfigurationExecution{
			PhaseExecution:   report.PhaseExecution{State: report.PhaseCompleted},
			TransactionState: report.TransactionNotRequired,
		},
	})
	got := out.String()
	if !strings.Contains(got, "Transaction state: not-required") {
		t.Errorf("output missing not-required transaction state:\n%s", got)
	}
	if strings.Contains(got, "Inventory path") {
		t.Errorf("no-managed-targets report must not reference inventory:\n%s", got)
	}
}

func TestExistingPrintExecutionReport_Unchanged(t *testing.T) {
	var out bytes.Buffer
	printExecutionReport(&out, &report.ExecutionReport{
		Fingerprint: "abc",
		ManagedTargets: []report.TargetOutcome{{
			Destination: "/home/user/.zshrc", Status: report.TargetMutated, BackupPath: "/backup/zshrc",
		}},
		ExternalActions: []report.ActionOutcome{{Description: "install packages", Status: report.ActionCompleted}},
	})
	for _, want := range []string{"abc", "/home/user/.zshrc", "/backup/zshrc", "install packages"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report output missing %q: %s", want, out.String())
		}
	}
}
