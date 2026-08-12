package transaction

import (
	"errors"
	"fmt"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// RecoverInventory restores an uncommitted transaction from its durable
// inventory. Ambiguous entry states fail closed and retain every recovery
// artifact for manual inspection.
func RecoverInventory(inventory *Inventory, opts ...Option) error {
	if inventory == nil {
		return errors.New("recover inventory: inventory is nil")
	}
	if inventory.FormatVersion != InventoryFormatVersion {
		return fmt.Errorf("recover inventory: unsupported format %d", inventory.FormatVersion)
	}
	if inventory.RunID == "" {
		return errors.New("recover inventory: run ID is missing")
	}
	if inventory.Lifecycle == InventoryRolledBack {
		return nil
	}
	if inventory.Lifecycle == InventoryRecoveryIncomplete {
		return errors.New("recover inventory: prior recovery is incomplete; manual inspection required")
	}

	allTargets := make([]plan.Target, 0, len(inventory.Entries))
	for _, entry := range inventory.Entries {
		allTargets = append(allTargets, entry.Target)
	}
	recoveryPlan, err := plan.NewInstallationPlan(inventory.RunID, allTargets)
	if err != nil {
		return fmt.Errorf("recover inventory: rebuild plan: %w", err)
	}
	tx := New(recoveryPlan, opts...)
	tx.inventory = inventory

	for i := range inventory.Entries {
		entry := &inventory.Entries[i]
		switch entry.State {
		case EntryMutated, EntryRemoved:
			tx.mutated = append(tx.mutated, entry.Target)
		case EntryBackedUp, EntryStaged, EntryOriginalRelocated:
			actual, err := tx.reader.Read(entry.Target.Destination)
			if err != nil {
				return fmt.Errorf("recover inventory target %q: %w", entry.Target.Destination, err)
			}
			if preStatesEqual(entry.Target.PreState, actual) {
				continue
			}
			candidateInstalled := actual.Type == plan.StateAbsent ||
				(entry.Target.SourceDigest != "" && actual.Digest == entry.Target.SourceDigest)
			if !candidateInstalled {
				return fmt.Errorf("recover inventory target %q: destination state is ambiguous", entry.Target.Destination)
			}
			if err := tx.RestoreTarget(entry.Target); err != nil {
				return fmt.Errorf("recover inventory target %q: %w", entry.Target.Destination, err)
			}
			entry.State = EntryRestored
			entry.Status = report.TargetRestored
			entry.Error = nil
		case EntryPending, EntrySourceDrift, EntrySkipped, EntryRestored:
			// No durable destination mutation needs reversal.
		case EntryOwnershipAmbiguous, EntryFailed:
			return fmt.Errorf("recover inventory target %q: state %q requires manual inspection", entry.Target.Destination, entry.State)
		default:
			return fmt.Errorf("recover inventory target %q: unknown state %q", entry.Target.Destination, entry.State)
		}
	}
	return tx.Rollback()
}
