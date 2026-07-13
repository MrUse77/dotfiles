package transaction

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// Inventory records every managed target, its original pre-state, retained backup
// location, and execution outcome. It is persisted to disk so users can recover
// manually after an uncatchable interruption.
type Inventory struct {
	RunID   string           `json:"run_id"`
	Path    string           `json:"path"`
	Entries []InventoryEntry `json:"entries"`
}

// InventoryEntry is one row in the retained inventory.
type InventoryEntry struct {
	Target     plan.Target         `json:"target"`
	Original   plan.PreState       `json:"original"`
	BackupPath string              `json:"backup_path"`
	LinkValue  string              `json:"link_value,omitempty"`
	Status     report.TargetStatus `json:"status"`
	Error      error               `json:"error,omitempty"`
}

// MarshalJSON suppresses the error field when nil so the persisted inventory is
// readable. Unexported-error values cannot round-trip, so the inventory is for
// human recovery, not in-process restoration.
func (e InventoryEntry) MarshalJSON() ([]byte, error) {
	type flat InventoryEntry
	if e.Error != nil {
		return json.Marshal(&struct {
			Error string `json:"error"`
			flat
		}{
			Error: e.Error.Error(),
			flat:  flat(e),
		})
	}
	return json.Marshal(flat(e))
}

// backupRootFor returns the run-scoped backup directory for a target.
func backupRootFor(target plan.Target) string {
	return filepath.Dir(target.BackupPath)
}

// persistInventory writes the inventory to disk inside the first target's backup
// root. Multiple targets with different parents each get a separate backup root,
// but the inventory is always written next to the backups it describes.
func persistInventory(fs Filesystem, inv *Inventory) error {
	if len(inv.Entries) == 0 {
		return nil
	}
	root := backupRootFor(inv.Entries[0].Target)
	path := filepath.Join(root, "inventory.json")
	inv.Path = path
	data, err := json.MarshalIndent(inv, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal inventory: %w", err)
	}
	if err := writeFile(fs, path, data, 0o600); err != nil {
		return fmt.Errorf("write inventory: %w", err)
	}
	return nil
}

func writeFile(fs Filesystem, path string, data []byte, mode os.FileMode) error {
	f, err := fs.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return fs.Chmod(path, mode)
}
