package transaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// Task 5.5: additive release provenance.

func TestInventory_ReleaseProvenance_RoundTrip(t *testing.T) {
	inv := &Inventory{
		FormatVersion:     InventoryFormatVersion,
		RunID:             "run",
		Lifecycle:         InventoryCompleted,
		ReleaseProvenance: &ReleaseProvenance{Tag: "config-v1.2.3", Digest: "abc123"},
		Entries: []InventoryEntry{{
			Status: report.TargetMutated,
			State:  EntryRemoved,
		}},
	}
	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	var got Inventory
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal inventory: %v", err)
	}
	if got.ReleaseProvenance == nil {
		t.Fatal("ReleaseProvenance lost in round-trip")
	}
	if got.ReleaseProvenance.Tag != "config-v1.2.3" || got.ReleaseProvenance.Digest != "abc123" {
		t.Errorf("ReleaseProvenance = %#v, want {config-v1.2.3 abc123}", got.ReleaseProvenance)
	}
	if got.Entries[0].State != EntryRemoved {
		t.Errorf("EntryRemoved state lost in round-trip: %q", got.Entries[0].State)
	}
}

// A schema-1 inventory (no release field) decodes with nil provenance and
// every recorded entry intact.
func TestInventory_Schema1Decode_NilReleaseProvenance(t *testing.T) {
	golden := `{
  "format_version": 1,
  "run_id": "20260712T120000-old",
  "lifecycle": "completed",
  "path": "/home/user/.dots-backups/20260712T120000-old/inventory.json",
  "entries": [
    {
      "target": {
        "Destination": "/home/user/.zshrc",
        "Kind": "copy-file",
        "PreState": {"Type": "file", "Mode": 420},
        "BackupPath": "/home/user/.dots-backups/20260712T120000-old/2f2e7a73687263"
      },
      "original": {"Type": "file", "Mode": 420},
      "backup_path": "/home/user/.dots-backups/20260712T120000-old/2f2e7a73687263",
      "state": "mutated",
      "status": "mutated"
    }
  ]
}`
	var inv Inventory
	if err := json.Unmarshal([]byte(golden), &inv); err != nil {
		t.Fatalf("schema-1 inventory must decode: %v", err)
	}
	if inv.FormatVersion != 1 {
		t.Errorf("FormatVersion = %d, want 1", inv.FormatVersion)
	}
	if inv.ReleaseProvenance != nil {
		t.Errorf("schema-1 inventory decoded with provenance: %#v (want unknown/nil)", inv.ReleaseProvenance)
	}
	if len(inv.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(inv.Entries))
	}
	entry := inv.Entries[0]
	if entry.Target.Destination != "/home/user/.zshrc" || entry.Target.Kind != plan.CopyFile {
		t.Errorf("schema-1 entry target not intact: %#v", entry.Target)
	}
	if entry.State != EntryMutated || entry.Status != report.TargetMutated {
		t.Errorf("schema-1 entry state/status not intact: %q/%q", entry.State, entry.Status)
	}
}

// A nil provenance is omitted from JSON entirely (additive, never breaking).
func TestInventory_ReleaseProvenance_OmittedWhenNil(t *testing.T) {
	inv := &Inventory{
		FormatVersion: InventoryFormatVersion,
		RunID:         "run",
		Lifecycle:     InventoryPrepared,
		Entries: []InventoryEntry{{
			Status: report.TargetPending,
			State:  EntryPending,
		}},
	}
	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	if strings.Contains(string(data), "release") {
		t.Errorf("nil provenance must be omitted from inventory JSON: %s", data)
	}
}

// A historical (schema-1) inventory without provenance still restores: the
// absence of provenance is unknown identity, not an invalid run.
func TestRestore_Schema1InventoryWithoutProvenanceRestores(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".zshrc")
	backup := filepath.Join(home, ".dots-backups", "run1", "2f2e7a73687263")
	mustWriteRestoreFixture(t, dest, []byte("installed"), 0o644)
	mustWriteRestoreFixture(t, backup, []byte("original"), 0o644)

	golden := `{
  "format_version": 1,
  "run_id": "run1",
  "lifecycle": "completed",
  "path": "` + filepath.Join(home, ".dots-backups", "run1", "inventory.json") + `",
  "entries": [
    {
      "target": {
        "Destination": "` + dest + `",
        "Kind": "copy-file",
        "PreState": {"Type": "file", "Mode": 420},
        "BackupPath": "` + backup + `"
      },
      "original": {"Type": "file", "Mode": 420},
      "backup_path": "` + backup + `",
      "state": "mutated",
      "status": "mutated"
    }
  ]
}`
	var inv Inventory
	if err := json.Unmarshal([]byte(golden), &inv); err != nil {
		t.Fatalf("schema-1 inventory must decode: %v", err)
	}
	if inv.ReleaseProvenance != nil {
		t.Fatalf("schema-1 inventory decoded with provenance: %#v", inv.ReleaseProvenance)
	}
	tgt := inv.Entries[0].Target
	if err := New(restorePlan(t, []plan.Target{tgt})).RestoreTarget(tgt); err != nil {
		t.Fatalf("RestoreTarget() error = %v", err)
	}
	if got := readFileString(t, dest); got != "original" {
		t.Errorf("restored file = %q, want original (provenance must not block restore)", got)
	}
}
