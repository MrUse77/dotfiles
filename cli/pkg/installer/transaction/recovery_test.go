package transaction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestRecoverInventory_RestoresPersistedMutationsAfterCrash(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "desired")
	destination := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(source, []byte("after"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := os.WriteFile(destination, []byte("before"), 0o600); err != nil {
		t.Fatalf("write destination: %v", err)
	}
	preState, err := plan.DefaultStateReader().Read(destination)
	if err != nil {
		t.Fatalf("read pre-state: %v", err)
	}
	runID := "20260811T120000-recovery"
	target := plan.Target{
		Source:      source,
		Destination: destination,
		Kind:        plan.CopyFile,
		PreState:    preState,
		BackupPath:  plan.BackupPath(home, runID, destination),
	}
	configPlan, err := plan.NewInstallationPlan(runID, []plan.Target{target})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	tx := New(configPlan)
	if err := tx.Prepare(); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	encoded, err := json.Marshal(tx.Inventory())
	if err != nil {
		t.Fatalf("marshal persisted inventory: %v", err)
	}
	var persisted Inventory
	if err := json.Unmarshal(encoded, &persisted); err != nil {
		t.Fatalf("unmarshal persisted inventory: %v", err)
	}
	if err := RecoverInventory(&persisted); err != nil {
		t.Fatalf("recover inventory: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read recovered destination: %v", err)
	}
	if string(got) != "before" {
		t.Fatalf("recovered content = %q, want before", got)
	}
	if persisted.Lifecycle != InventoryRolledBack || persisted.Entries[0].State != EntryRestored {
		t.Fatalf("recovered inventory = %#v", persisted)
	}
}

func TestRecoverInventory_AmbiguousDestinationFailsClosed(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	destination := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(destination, []byte("user change"), 0o600); err != nil {
		t.Fatalf("write destination: %v", err)
	}
	inventory := &Inventory{
		FormatVersion: InventoryFormatVersion,
		RunID:         "20260811T120000-ambiguous",
		Lifecycle:     InventoryCommitting,
		Entries: []InventoryEntry{{
			Target: plan.Target{
				Destination:  destination,
				Kind:         plan.CopyFile,
				SourceDigest: "candidate-digest",
				PreState:     plan.PreState{Type: plan.StateFile, Mode: 0o600, Digest: "original-digest"},
			},
			State: EntryBackedUp,
		}},
	}

	if err := RecoverInventory(inventory); err == nil {
		t.Fatal("expected ambiguous recovery to fail closed")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if string(got) != "user change" {
		t.Fatalf("destination content = %q, want user change preserved", got)
	}
}
