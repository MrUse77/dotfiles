package transaction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func TestRollbackOwnershipPreservesExternalReplacement(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source, destination := filepath.Join(repo, "source"), filepath.Join(home, "destination")
	mustWriteFile(t, source, []byte("installed"))
	mustWriteFile(t, destination, []byte("original"))
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, destination, []byte("external"))
	if err := tx.Rollback(); err == nil {
		t.Fatal("Rollback() succeeded after external replacement")
	}
	if got := readFileString(t, destination); got != "external" {
		t.Errorf("destination = %q, want external", got)
	}
	entry := entryFor(t, tx.Inventory(), destination)
	if entry.State != EntryOwnershipAmbiguous {
		t.Errorf("state = %q, want ownership-ambiguous", entry.State)
	}
	if _, err := os.Lstat(entry.BackupPath); err != nil {
		t.Errorf("backup removed: %v", err)
	}
}

func TestDirectorySwapFailureRetainsRecoveryArtifacts(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source, destination := filepath.Join(repo, "source"), filepath.Join(home, "destination")
	mustMkdir(t, source)
	mustWriteFile(t, filepath.Join(source, "new"), []byte("new"))
	mustMkdir(t, destination)
	mustWriteFile(t, filepath.Join(destination, "old"), []byte("old"))
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyTree}})
	fs := &hookFS{Filesystem: OSFilesystem(), renameHook: func(old, new string) error {
		if new == destination && filepath.Base(old) != filepath.Base(destination) {
			return os.ErrPermission
		}
		return nil
	}}
	tx := New(p, WithFilesystem(fs))
	if _, err := tx.Execute(); err == nil {
		t.Fatal("Execute() succeeded")
	}
	entry := entryFor(t, tx.Inventory(), destination)
	if entry.StagePath == "" || entry.TrashPath == "" {
		t.Fatalf("retained paths stage=%q trash=%q", entry.StagePath, entry.TrashPath)
	}
	if _, err := os.Lstat(entry.StagePath); err != nil {
		t.Errorf("stage not retained: %v", err)
	}
	if _, err := os.Lstat(entry.TrashPath); err != nil {
		t.Errorf("trash not retained: %v", err)
	}
}

func TestTransaction_Execute_DirectoryRelocationCheckpointFailureRequiresManualRecovery(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source, destination := filepath.Join(repo, "source"), filepath.Join(home, "destination")
	mustMkdir(t, source)
	mustWriteFile(t, filepath.Join(source, "new"), []byte("new"))
	mustMkdir(t, destination)
	mustWriteFile(t, filepath.Join(destination, "old"), []byte("old"))

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyTree}})
	tx := New(p)
	injected := errors.New("relocation checkpoint persistence denied")
	failed := false
	inventoryPersistFailure = func(inv *Inventory) error {
		if !failed && inv.Entries[0].State == EntryOriginalRelocated {
			failed = true
			return injected
		}
		return nil
	}
	t.Cleanup(func() { inventoryPersistFailure = nil })

	rpt, err := tx.Execute()
	if err == nil {
		t.Fatal("Execute() succeeded after relocation checkpoint persistence failure")
	}
	if !failed {
		t.Fatal("relocation checkpoint persistence was not reached")
	}
	entry := entryFor(t, tx.Inventory(), destination)
	if entry.TrashPath == "" {
		t.Fatal("TrashPath was cleared after relocation checkpoint failure")
	}
	if _, err := os.Lstat(entry.TrashPath); err != nil {
		t.Errorf("trash not retained: %v", err)
	}
	if rpt.RecoveryState != report.RecoveryManualRecoveryRequired {
		t.Errorf("RecoveryState = %q, want %q", rpt.RecoveryState, report.RecoveryManualRecoveryRequired)
	}
	if rpt.RecoveryNextAction != report.ManualRecoveryNextAction {
		t.Errorf("RecoveryNextAction = %q, want manual recovery action", rpt.RecoveryNextAction)
	}
	if tx.Inventory().Lifecycle != InventoryRecoveryIncomplete {
		t.Errorf("Lifecycle = %q, want %q", tx.Inventory().Lifecycle, InventoryRecoveryIncomplete)
	}
}
