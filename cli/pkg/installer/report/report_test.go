package report

import (
	"errors"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestExecutionReport_TypedTargetAndActionOutcomes(t *testing.T) {
	report := ExecutionReport{
		Fingerprint: "sha256:fingerprint",
		ManagedTargets: []TargetOutcome{
			{Destination: "/home/user/.zshrc", Status: TargetMutated, BackupPath: "/home/user/.dots-backups/run/.zshrc"},
			{Destination: "/home/user/.config/hypr", Status: TargetFailed, Error: errors.New("permission denied")},
		},
		ExternalActions: []ActionOutcome{
			{Description: "update system", Status: ActionCompleted},
			{Description: "install paru", Status: ActionFailed, Error: errors.New("network error")},
		},
	}

	if report.ManagedTargets[0].Status != TargetMutated {
		t.Errorf("target status = %q, want mutated", report.ManagedTargets[0].Status)
	}
	if report.ManagedTargets[1].Status != TargetFailed {
		t.Errorf("target status = %q, want failed", report.ManagedTargets[1].Status)
	}
	if report.ExternalActions[0].Status != ActionCompleted {
		t.Errorf("action status = %q, want completed", report.ExternalActions[0].Status)
	}
	if report.ExternalActions[1].Status != ActionFailed {
		t.Errorf("action status = %q, want failed", report.ExternalActions[1].Status)
	}
}

func TestExecutionReport_RetainedBackupPaths(t *testing.T) {
	paths := []string{
		"/home/user/.dots-backups/run/2f686f6d652f757365722f2e636f6e6669672f68797072",
		"/home/user/.dots-backups/run/2f686f6d652f757365722f2e7a73687263",
	}

	report := ExecutionReport{
		Fingerprint:     "fp",
		BackupPaths:     paths,
		ManagedTargets:  []TargetOutcome{},
		ExternalActions: []ActionOutcome{},
	}

	if len(report.BackupPaths) != 2 {
		t.Fatalf("len(BackupPaths) = %d, want 2", len(report.BackupPaths))
	}
	if report.BackupPaths[0] != paths[0] {
		t.Errorf("BackupPaths[0] = %q, want %q", report.BackupPaths[0], paths[0])
	}
}

func TestExecutionReport_PrimaryCauseAndRollbackFailures(t *testing.T) {
	rollbackFailures := []TargetOutcome{
		{Destination: "/home/user/.config/waybar", Status: TargetFailed, Error: errors.New("restore failed")},
	}

	report := ExecutionReport{
		Fingerprint:      "fp",
		PrimaryCause:     errors.New("mutation failed"),
		RollbackFailures: rollbackFailures,
	}

	if report.PrimaryCause == nil {
		t.Fatal("PrimaryCause is nil")
	}
	if report.PrimaryCause.Error() != "mutation failed" {
		t.Errorf("PrimaryCause = %v", report.PrimaryCause)
	}
	if len(report.RollbackFailures) != 1 {
		t.Fatalf("len(RollbackFailures) = %d, want 1", len(report.RollbackFailures))
	}
	if report.RollbackFailures[0].Destination != "/home/user/.config/waybar" {
		t.Errorf("RollbackFailures[0].Destination = %q", report.RollbackFailures[0].Destination)
	}
}

func TestTypedErrors(t *testing.T) {
	target := plan.Target{Destination: "/home/user/.zshrc"}
	action := plan.ExternalAction{Description: "update system"}

	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "PlanError",
			err:  &PlanError{Phase: "discovery", Cause: errors.New("boom")},
			want: "plan error during discovery: boom",
		},
		{
			name: "PlanDriftError",
			err: &PlanDriftError{
				Target:   target,
				Expected: plan.PreState{Type: plan.StateFile, Digest: "old"},
				Actual:   plan.PreState{Type: plan.StateFile, Digest: "new"},
			},
			want: "plan drift for /home/user/.zshrc",
		},
		{
			name: "BackupError",
			err:  &BackupError{Target: target, Cause: errors.New("no space")},
			want: "backup failed for /home/user/.zshrc: no space",
		},
		{
			name: "MutationError",
			err:  &MutationError{Target: target, Cause: errors.New("io error")},
			want: "mutation failed for /home/user/.zshrc: io error",
		},
		{
			name: "RollbackError",
			err: &RollbackError{Failures: []TargetOutcome{
				{Destination: "/home/user/.config", Error: errors.New("locked")},
			}},
			want: "rollback incomplete: 1 failure(s)",
		},
		{
			name: "ExternalActionError",
			err:  &ExternalActionError{Action: action, Cause: errors.New("exit 1")},
			want: `external action "update system" failed: exit 1`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
