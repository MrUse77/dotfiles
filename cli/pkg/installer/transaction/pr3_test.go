package transaction

import (
	"errors"
	"fmt"
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

// 3.9: rollback reverses mutation order across 3 targets with mid-plan backup failure.
func TestTransaction_Rollback_MidPlanBackupFailureReversesOrder(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	var dests, sources [3]string
	for i := 0; i < 3; i++ {
		dests[i] = filepath.Join(home, fmt.Sprintf("target-%d", i))
		sources[i] = filepath.Join(repo, fmt.Sprintf("src-%d", i))
		mustWriteFile(t, dests[i], []byte(fmt.Sprintf("original-%d", i)))
		mustWriteFile(t, sources[i], []byte(fmt.Sprintf("new-%d", i)))
	}

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: sources[0], Destination: dests[0], Kind: plan.CopyFile},
		{Source: sources[1], Destination: dests[1], Kind: plan.CopyFile},
		{Source: sources[2], Destination: dests[2], Kind: plan.CopyFile},
	})

	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	// Inject backup collision at target 2's backup path after prepare succeeds.
	// During Commit, targets 0 and 1 will back up and mutate; target 2 will fail at backup.
	targets := p.ManagedTargets()
	mustWriteFile(t, targets[2].BackupPath, []byte("collision"))

	if err := tx.Commit(); err == nil {
		t.Fatal("expected backup error for target 2")
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// targets 0 and 1 were mutated and then restored in reverse order.
	for i := 0; i < 2; i++ {
		if got := readFileString(t, dests[i]); got != fmt.Sprintf("original-%d", i) {
			t.Errorf("target %d content = %q, want original-%d", i, got, i)
		}
		entry := entryFor(t, tx.Inventory(), dests[i])
		if entry.Status != report.TargetRestored {
			t.Errorf("target %d status = %q, want restored", i, entry.Status)
		}
	}

	// target 2 was never mutated; it remains at original state with failed status.
	if got := readFileString(t, dests[2]); got != "original-2" {
		t.Errorf("target 2 content = %q, want original-2", got)
	}
	entry2 := entryFor(t, tx.Inventory(), dests[2])
	if entry2.Status != report.TargetFailed {
		t.Errorf("target 2 status = %q, want failed", entry2.Status)
	}

	if tx.Inventory().Lifecycle != InventoryRolledBack {
		t.Errorf("lifecycle = %q, want rolled-back", tx.Inventory().Lifecycle)
	}
}

// 3.10: 3 targets, middle fails rollback, rollback continues to first, aggregates outcomes.
func TestTransaction_Rollback_MiddleRestoreFailsContinuesToFirst(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	var dests, sources [3]string
	for i := 0; i < 3; i++ {
		dests[i] = filepath.Join(home, fmt.Sprintf("target-%d", i))
		sources[i] = filepath.Join(repo, fmt.Sprintf("src-%d", i))
		mustWriteFile(t, dests[i], []byte(fmt.Sprintf("original-%d", i)))
		mustWriteFile(t, sources[i], []byte(fmt.Sprintf("new-%d", i)))
	}

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: sources[0], Destination: dests[0], Kind: plan.CopyFile},
		{Source: sources[1], Destination: dests[1], Kind: plan.CopyFile},
		{Source: sources[2], Destination: dests[2], Kind: plan.CopyFile},
	})

	targets := p.ManagedTargets()
	// HookFS fails Open for the middle target's backup path during rollback restore.
	// Backup during Commit uses direct syscalls (not through Filesystem), so this
	// does not affect Commit.
	fs := &hookFS{
		Filesystem: OSFilesystem(),
		failOpen:   map[string]error{targets[1].BackupPath: errors.New("restore denied")},
	}

	tx := New(p, WithFilesystem(fs))

	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	// All 3 are now mutated.
	for i := 0; i < 3; i++ {
		if got := readFileString(t, dests[i]); got != fmt.Sprintf("new-%d", i) {
			t.Fatalf("target %d should be mutated to new-%d, got %q", i, i, got)
		}
	}

	// Rollback: reverse order is target 2 (succeeds), target 1 (fails), target 0 (succeeds).
	err := tx.Rollback()
	if err == nil {
		t.Fatal("expected RollbackError")
	}
	var rbErr *report.RollbackError
	if !errors.As(err, &rbErr) {
		t.Fatalf("expected *report.RollbackError, got %T", err)
	}
	if len(rbErr.Failures) != 1 {
		t.Fatalf("rollback failures = %d, want 1", len(rbErr.Failures))
	}
	if rbErr.Failures[0].Destination != dests[1] {
		t.Errorf("failure destination = %q, want %q", rbErr.Failures[0].Destination, dests[1])
	}

	// target 2 (last mutated, first in rollback) was restored.
	if got := readFileString(t, dests[2]); got != "original-2" {
		t.Errorf("target 2 = %q, want original-2", got)
	}
	entry2 := entryFor(t, tx.Inventory(), dests[2])
	if entry2.Status != report.TargetRestored {
		t.Errorf("target 2 status = %q, want restored", entry2.Status)
	}

	// target 1 (middle) restore failed; it retains the committed content.
	if got := readFileString(t, dests[1]); got != "new-1" {
		t.Errorf("target 1 = %q, want new-1 (restore failed)", got)
	}
	entry1 := entryFor(t, tx.Inventory(), dests[1])
	if entry1.Status != report.TargetFailed {
		t.Errorf("target 1 status = %q, want failed", entry1.Status)
	}

	// target 0 (first mutated, last in rollback) was restored.
	if got := readFileString(t, dests[0]); got != "original-0" {
		t.Errorf("target 0 = %q, want original-0", got)
	}
	entry0 := entryFor(t, tx.Inventory(), dests[0])
	if entry0.Status != report.TargetRestored {
		t.Errorf("target 0 status = %q, want restored", entry0.Status)
	}

	if tx.Inventory().Lifecycle != InventoryRecoveryIncomplete {
		t.Errorf("lifecycle = %q, want recovery-incomplete", tx.Inventory().Lifecycle)
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

// 3.13: focused lifecycle transition test: commit-failed → rolling-back → recovery-incomplete.
func TestTransaction_Lifecycle_CommitFailedThroughRecoveryIncomplete(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	source := filepath.Join(repo, "source")
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, source, []byte("new"))
	mustWriteFile(t, destination, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	tx := New(p)

	// After Prepare, lifecycle is prepared.
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if got := tx.Inventory().Lifecycle; got != InventoryPrepared {
		t.Fatalf("after Prepare lifecycle = %q, want %q", got, InventoryPrepared)
	}

	// After Commit, lifecycle is completed.
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if tx.Inventory().Lifecycle != InventoryCompleted {
		t.Fatalf("after Commit lifecycle = %q, want %q", tx.Inventory().Lifecycle, InventoryCompleted)
	}

	// Cause rollback to fail so we can observe recovery-incomplete.
	mustWriteFile(t, destination, []byte("external"))
	if err := tx.Rollback(); err == nil {
		t.Fatal("expected RollbackError for ambiguous ownership")
	}

	if tx.Inventory().Lifecycle != InventoryRecoveryIncomplete {
		t.Fatalf("after Rollback lifecycle = %q, want %q", tx.Inventory().Lifecycle, InventoryRecoveryIncomplete)
	}
	entry := entryFor(t, tx.Inventory(), destination)
	if entry.State != EntryOwnershipAmbiguous {
		t.Errorf("entry state = %q, want ownership-ambiguous", entry.State)
	}
}
