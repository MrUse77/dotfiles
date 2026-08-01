package report

import (
	"errors"
	"testing"
)

func TestTwoPhaseOutcomeConstants(t *testing.T) {
	cases := []struct {
		got  AttemptOutcome
		want AttemptOutcome
	}{
		{OutcomeCompleted, "completed"},
		{OutcomeIncomplete, "incomplete"},
		{OutcomeFailed, "failed"},
		{OutcomeCancelled, "cancelled"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("outcome = %q, want %q", c.got, c.want)
		}
	}
}

func TestInstallPhaseConstants(t *testing.T) {
	cases := []struct {
		got  InstallPhase
		want InstallPhase
	}{
		{PhasePackage, "package"},
		{PhaseRepository, "repository"},
		{PhaseConfiguration, "configuration"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("phase = %q, want %q", c.got, c.want)
		}
	}
}

func TestPhaseStateConstants(t *testing.T) {
	cases := []struct {
		got  PhaseState
		want PhaseState
	}{
		{PhaseNotStarted, "not-started"},
		{PhaseCompleted, "completed"},
		{PhaseFailed, "failed"},
		{PhaseSkipped, "skipped"},
		{PhaseCancelled, "cancelled"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("state = %q, want %q", c.got, c.want)
		}
	}
}

func TestTransactionStateConstants(t *testing.T) {
	cases := []struct {
		got  TransactionState
		want TransactionState
	}{
		{TransactionNotStarted, "not-started"},
		{TransactionStarted, "started"},
		{TransactionCompleted, "completed"},
		{TransactionNotRequired, "not-required"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("state = %q, want %q", c.got, c.want)
		}
	}
}

func TestTwoPhaseExecutionReport_Struct(t *testing.T) {
	report := TwoPhaseExecutionReport{
		RunID:              "run-1",
		Outcome:            OutcomeCompleted,
		PrimaryFailedPhase: PhasePackage,
		Package: PhaseExecution{
			State:           PhaseCompleted,
			PlanFingerprint: "pkg-fp",
			Report:          &ExecutionReport{Fingerprint: "pkg-fp"},
		},
		Repository: RepositoryExecution{
			State:       PhaseCompleted,
			Destination: "/home/user/.cache/dotfiles",
			Ref:         "main",
			Cause:       errors.New("cause"),
		},
		Configuration: ConfigurationExecution{
			PhaseExecution: PhaseExecution{
				State:           PhaseCompleted,
				PlanFingerprint: "cfg-fp",
			},
			TransactionState: TransactionCompleted,
			InventoryPath:    "/home/user/.dots-backups/run/inventory.json",
		},
	}
	if report.RunID != "run-1" {
		t.Errorf("RunID = %q", report.RunID)
	}
	if report.Configuration.TransactionState != TransactionCompleted {
		t.Errorf("TransactionState = %q", report.Configuration.TransactionState)
	}
	if report.Configuration.InventoryPath != "/home/user/.dots-backups/run/inventory.json" {
		t.Errorf("InventoryPath = %q", report.Configuration.InventoryPath)
	}
	if report.Repository.Cause == nil {
		t.Error("Repository.Cause is nil")
	}
}

func TestExecutionReport_InventoryPathField(t *testing.T) {
	report := ExecutionReport{Fingerprint: "fp", InventoryPath: "/path/to/inventory.json"}
	if report.InventoryPath != "/path/to/inventory.json" {
		t.Errorf("InventoryPath = %q", report.InventoryPath)
	}
}
