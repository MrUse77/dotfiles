package transaction

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// InventoryFormatVersion is the first additive-only on-disk inventory schema.
const InventoryFormatVersion = 1

// InventoryLifecycle describes the durable transaction lifecycle.
type InventoryLifecycle string

const (
	InventoryPrepared           InventoryLifecycle = "prepared"
	InventoryCommitting         InventoryLifecycle = "committing"
	InventoryCommitFailed       InventoryLifecycle = "commit-failed"
	InventoryRollingBack        InventoryLifecycle = "rolling-back"
	InventoryRolledBack         InventoryLifecycle = "rolled-back"
	InventoryRecoveryIncomplete InventoryLifecycle = "recovery-incomplete"
	InventoryCompleted          InventoryLifecycle = "completed"
)

// InventoryEntryState describes a target's durable recovery state.
type InventoryEntryState string

const (
	EntryPending            InventoryEntryState = "pending"
	EntrySourceDrift        InventoryEntryState = "source-drift"
	EntryBackedUp           InventoryEntryState = "backed-up"
	EntryStaged             InventoryEntryState = "staged"
	EntryOriginalRelocated  InventoryEntryState = "original-relocated"
	EntryMutated            InventoryEntryState = "mutated"
	EntryRestored           InventoryEntryState = "restored"
	EntryOwnershipAmbiguous InventoryEntryState = "ownership-ambiguous"
	EntryFailed             InventoryEntryState = "failed"
)

// Inventory records every managed target and is persisted as human-readable,
// versioned JSON. New fields are additive so future unknown fields are ignored.
type Inventory struct {
	FormatVersion int                `json:"format_version"`
	RunID         string             `json:"run_id"`
	Lifecycle     InventoryLifecycle `json:"lifecycle"`
	Path          string             `json:"path"`
	Entries       []InventoryEntry   `json:"entries"`
}

// InventoryEntry is one row in the retained inventory.
type InventoryEntry struct {
	Target            plan.Target         `json:"target"`
	Original          plan.PreState       `json:"original"`
	BackupPath        string              `json:"backup_path"`
	StagePath         string              `json:"stage_path,omitempty"`
	TrashPath         string              `json:"trash_path,omitempty"`
	LinkValue         string              `json:"link_value,omitempty"`
	InstalledIdentity string              `json:"installed_identity,omitempty"`
	InstalledDigest   string              `json:"installed_digest,omitempty"`
	InstalledMode     os.FileMode         `json:"installed_mode,omitempty"`
	State             InventoryEntryState `json:"state"`
	Status            report.TargetStatus `json:"status"`
	ErrorDescription  string              `json:"error_description,omitempty"`
	Error             error               `json:"-"`
}

func (e InventoryEntry) MarshalJSON() ([]byte, error) {
	type flat InventoryEntry
	entry := flat(e)
	if e.Error != nil && entry.ErrorDescription == "" {
		entry.ErrorDescription = e.Error.Error()
	}
	return json.Marshal(entry)
}

func backupRootFor(target plan.Target) string { return filepath.Dir(target.BackupPath) }

// inventoryBeforeRename is a test seam for simulating path substitution at the
// only point where a path-based implementation would be vulnerable.
var inventoryBeforeRename func()

// inventoryBeforeWrite is a test seam for observing durable lifecycle snapshots.
var inventoryBeforeWrite func(*Inventory)

// inventoryPersistFailure is a test seam for persistence failure recovery tests.
var inventoryPersistFailure func(*Inventory) error

// persistInventory writes beside the first target's backup. The final filename is
// authoritative only after the atomic replacement completes.
func persistInventory(fs Filesystem, inv *Inventory) error {
	if inventoryPersistFailure != nil {
		if err := inventoryPersistFailure(inv); err != nil {
			return err
		}
	}
	if len(inv.Entries) == 0 {
		return nil
	}
	return persistInventoryAt(fs, inv, backupRootFor(inv.Entries[0].Target))
}

func persistInventoryAt(_ Filesystem, inv *Inventory, root string) error {
	rootFile, err := openBackupRoot(root, false)
	if err != nil {
		return fmt.Errorf("validate inventory root: %w", err)
	}
	defer rootFile.Close()

	inv.Path = filepath.Join(root, "inventory.json")
	if inv.FormatVersion == 0 {
		inv.FormatVersion = InventoryFormatVersion
	}
	if inventoryBeforeWrite != nil {
		inventoryBeforeWrite(inv)
	}
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}

	var tempName string
	var fd int
	for attempts := 0; attempts < 10; attempts++ {
		var nonce [8]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return fmt.Errorf("generate temporary inventory name: %w", err)
		}
		tempName = ".inventory.json.tmp-" + hex.EncodeToString(nonce[:])
		fd, err = unix.Openat(int(rootFile.Fd()), tempName, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, 0o600)
		if !errors.Is(err, unix.EEXIST) {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("create temporary inventory: %w", err)
	}
	temp := os.NewFile(uintptr(fd), tempName)
	defer func() { _ = temp.Close() }()
	if err := temp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod temporary inventory: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return fmt.Errorf("write temporary inventory: %w", err)
	}
	if err := temp.Sync(); err != nil {
		return fmt.Errorf("sync temporary inventory: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temporary inventory: %w", err)
	}
	if inventoryBeforeRename != nil {
		inventoryBeforeRename()
	}
	if err := unix.Renameat(int(rootFile.Fd()), tempName, int(rootFile.Fd()), "inventory.json"); err != nil {
		return fmt.Errorf("rename temporary inventory: %w", err)
	}
	if err := rootFile.Sync(); err != nil {
		return fmt.Errorf("sync inventory root: %w", err)
	}
	return nil
}
