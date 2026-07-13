package report

import "testing"

func TestPartialRecoveryReport(t *testing.T) {
	rpt := ExecutionReport{
		RecoveryState: RecoveryManualRecoveryRequired,
		RecoveryArtifacts: []RecoveryArtifact{{
			Destination: "/home/user/.config", BackupPath: "/backup/config", StagePath: "/stage/config", TrashPath: "/trash/config", InventoryPath: "/backup/inventory.json",
		}},
		RecoveryNextAction: ManualRecoveryNextAction,
	}
	if rpt.RecoveryState != RecoveryManualRecoveryRequired {
		t.Fatalf("state = %q", rpt.RecoveryState)
	}
	if got := rpt.RecoveryArtifacts[0]; got.InventoryPath == "" || got.StagePath == "" || got.TrashPath == "" {
		t.Errorf("incomplete artifact: %+v", got)
	}
	if rpt.RecoveryNextAction != "inspect named inventory and retained artifacts; do not delete or overwrite ambiguous paths" {
		t.Errorf("next action = %q", rpt.RecoveryNextAction)
	}
}
