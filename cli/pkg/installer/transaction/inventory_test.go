package transaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func TestAtomicInventory_RootSubstitutionBeforeRenameCannotRedirectWrite(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("create inventory root: %v", err)
	}
	attacker := t.TempDir()
	relocated := filepath.Join(parent, "root-retained")

	inventoryBeforeRename = func() {
		if err := os.Rename(root, relocated); err != nil {
			t.Fatalf("relocate root: %v", err)
		}
		if err := os.Symlink(attacker, root); err != nil {
			t.Fatalf("substitute root: %v", err)
		}
	}
	t.Cleanup(func() { inventoryBeforeRename = nil })

	inv := &Inventory{RunID: "run", Lifecycle: InventoryPrepared, Entries: []InventoryEntry{{Status: report.TargetPending}}}
	if err := persistInventoryAt(OSFilesystem(), inv, root); err != nil {
		t.Fatalf("persistInventoryAt() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(attacker, "inventory.json")); !os.IsNotExist(err) {
		t.Fatalf("inventory redirected through substituted root: %v", err)
	}
	info, err := os.Stat(filepath.Join(relocated, "inventory.json"))
	if err != nil {
		t.Fatalf("inventory missing from retained root: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("inventory mode = %o, want 0600", got)
	}
}

func TestBackupCreation_RootSubstitutionAfterValidationCannotRedirectWrite(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source := filepath.Join(repo, "source")
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, source, []byte("new"))
	mustWriteFile(t, destination, []byte("original"))
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	target := p.ManagedTargets()[0]
	if err := New(p).ensureBackupRoot(filepath.Dir(target.BackupPath)); err != nil {
		t.Fatalf("create backup root: %v", err)
	}
	root := filepath.Dir(target.BackupPath)
	retained := root + "-retained"
	attacker := t.TempDir()
	backupBeforeCreate = func() {
		if err := os.Rename(root, retained); err != nil {
			t.Fatalf("relocate root: %v", err)
		}
		if err := os.Symlink(attacker, root); err != nil {
			t.Fatalf("substitute root: %v", err)
		}
	}
	t.Cleanup(func() { backupBeforeCreate = nil })
	if err := New(p).backupTarget(target); err != nil {
		t.Fatalf("backupTarget: %v", err)
	}
	if got := readFileString(t, filepath.Join(retained, filepath.Base(target.BackupPath))); got != "original" {
		t.Fatalf("retained backup = %q, want original", got)
	}
	if _, err := os.Lstat(filepath.Join(attacker, filepath.Base(target.BackupPath))); !os.IsNotExist(err) {
		t.Fatalf("backup redirected through substituted root: %v", err)
	}
}

func TestTransaction_BackupCheckpointPersistsBeforeMutation(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source := filepath.Join(repo, "source")
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, source, []byte("new"))
	mustWriteFile(t, destination, []byte("original"))
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	tx := New(p)
	var checkpoint *Inventory
	backupCheckpointPersisted = func() {
		data, err := os.ReadFile(tx.Inventory().Path)
		if err != nil {
			t.Fatalf("read checkpoint: %v", err)
		}
		var persisted Inventory
		if err := json.Unmarshal(data, &persisted); err != nil {
			t.Fatalf("unmarshal checkpoint: %v", err)
		}
		checkpoint = &persisted
		if got := readFileString(t, destination); got != "original" {
			t.Fatalf("destination mutated before checkpoint: %q", got)
		}
	}
	t.Cleanup(func() { backupCheckpointPersisted = nil })
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if checkpoint == nil {
		t.Fatal("missing durable backed-up checkpoint")
	}
	if checkpoint.Entries[0].BackupPath == "" || checkpoint.Entries[0].State != EntryBackedUp {
		t.Fatalf("checkpoint entry = %+v, want backup path and backed-up state", checkpoint.Entries[0])
	}
}

func TestRollback_PersistsLifecycleAndPerTargetOutcomes(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source := filepath.Join(repo, "source")
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, source, []byte("new"))
	mustWriteFile(t, destination, []byte("original"))
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	var snapshots []InventoryLifecycle
	inventoryBeforeWrite = func(inv *Inventory) { snapshots = append(snapshots, inv.Lifecycle) }
	t.Cleanup(func() { inventoryBeforeWrite = nil })
	if err := tx.Rollback(); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if len(snapshots) < 3 || snapshots[0] != InventoryRollingBack || snapshots[len(snapshots)-1] != InventoryRolledBack {
		t.Fatalf("rollback lifecycle snapshots = %v, want rolling-back through rolled-back", snapshots)
	}
	var persisted Inventory
	data, err := os.ReadFile(tx.Inventory().Path)
	if err != nil {
		t.Fatalf("read inventory: %v", err)
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("unmarshal inventory: %v", err)
	}
	if persisted.Lifecycle != InventoryRolledBack || persisted.Entries[0].State != EntryRestored {
		t.Fatalf("persisted lifecycle/state = %q/%q, want %q/%q", persisted.Lifecycle, persisted.Entries[0].State, InventoryRolledBack, EntryRestored)
	}
}

func TestInventorySchema_IsVersionedAndRecordsLifecycle(t *testing.T) {
	inv := &Inventory{
		RunID:         "run",
		FormatVersion: InventoryFormatVersion,
		Lifecycle:     InventoryPrepared,
		Entries: []InventoryEntry{{
			Status:            report.TargetPending,
			State:             EntryPending,
			InstalledIdentity: "device:1,inode:2",
			InstalledDigest:   "digest",
			InstalledMode:     0o600,
			BackupPath:        "/backup",
			StagePath:         "/stage",
			TrashPath:         "/trash",
			ErrorDescription:  "description",
		}},
	}
	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal inventory: %v", err)
	}
	if got["format_version"] != float64(InventoryFormatVersion) {
		t.Errorf("format_version = %v, want %d", got["format_version"], InventoryFormatVersion)
	}
	if got["lifecycle"] != string(InventoryPrepared) {
		t.Errorf("lifecycle = %v, want %q", got["lifecycle"], InventoryPrepared)
	}
}
