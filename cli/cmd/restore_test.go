package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
)

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustWriteInventory(t *testing.T, root, runID string, entries []transaction.InventoryEntry) {
	t.Helper()
	dir := filepath.Join(root, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	inv := transaction.Inventory{
		FormatVersion: 1,
		RunID:         runID,
		Lifecycle:     transaction.InventoryCompleted,
		Entries:       entries,
	}
	data, err := json.Marshal(inv)
	if err != nil {
		t.Fatalf("marshal inventory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inventory.json"), data, 0o644); err != nil {
		t.Fatalf("write inventory: %v", err)
	}
}

func TestParseRunID(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"20260712T120000-abcd1234", true},
		{"20260712T120000", true},
		{"basura", false},
		{"", false},
		{"20260712T120000-", true},
	}
	for _, tc := range cases {
		got, ok := parseRunID(tc.id)
		if ok != tc.want {
			t.Errorf("parseRunID(%q) ok = %v, want %v", tc.id, ok, tc.want)
		}
		if ok {
			wantTime := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
			if !got.Equal(wantTime) {
				t.Errorf("parseRunID(%q) time = %v, want %v", tc.id, got, wantTime)
			}
		}
	}
}

func TestListBackupRuns(t *testing.T) {
	root := t.TempDir()
	mustWriteInventory(t, root, "20260712T120000-abcd", []transaction.InventoryEntry{
		{Target: plan.Target{Destination: "/a"}},
		{Target: plan.Target{Destination: "/b"}},
		{Target: plan.Target{Destination: "/c"}},
	})
	mustWriteInventory(t, root, "20260713T103000-efgh", []transaction.InventoryEntry{
		{Target: plan.Target{Destination: "/d"}},
	})
	if err := os.MkdirAll(filepath.Join(root, "not-a-run"), 0o755); err != nil {
		t.Fatalf("mkdir junk dir: %v", err)
	}

	runs, err := listBackupRuns(root)
	if err != nil {
		t.Fatalf("listBackupRuns() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2", len(runs))
	}
	if runs[0].ID != "20260713T103000-efgh" {
		t.Errorf("runs[0].ID = %q, want newest first", runs[0].ID)
	}
	if runs[0].Targets != 1 || runs[1].Targets != 3 {
		t.Errorf("target counts = %d,%d, want 1,3", runs[0].Targets, runs[1].Targets)
	}
}

func TestListBackupRuns_EmptyRoot(t *testing.T) {
	root := t.TempDir()
	runs, err := listBackupRuns(root)
	if err != nil {
		t.Fatalf("listBackupRuns() error = %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("len(runs) = %d, want 0", len(runs))
	}
}

func TestRestoreCandidates(t *testing.T) {
	home := t.TempDir()
	unchanged := filepath.Join(home, "unchanged.conf")
	changed := filepath.Join(home, "changed.conf")
	absent := filepath.Join(home, "absent.conf")

	mustWriteFile(t, unchanged, []byte("same"))
	mustWriteFile(t, changed, []byte("edited by user"))

	reader := plan.DefaultStateReader()
	unchangedState, err := reader.Read(unchanged)
	if err != nil {
		t.Fatalf("read unchanged state: %v", err)
	}

	inv := &transaction.Inventory{Entries: []transaction.InventoryEntry{
		{
			Target:          plan.Target{Destination: unchanged, Kind: plan.CopyFile, PreState: plan.PreState{Type: plan.StateFile}},
			InstalledDigest: unchangedState.Digest,
			State:           transaction.EntryMutated,
		},
		{
			Target:          plan.Target{Destination: changed, Kind: plan.CopyFile, PreState: plan.PreState{Type: plan.StateFile}},
			InstalledDigest: "sha256:otro-digest",
			State:           transaction.EntryMutated,
		},
		{
			Target: plan.Target{Destination: absent, Kind: plan.CopyFile, PreState: plan.PreState{Type: plan.StateAbsent}},
			State:  transaction.EntryMutated,
		},
		{
			Target: plan.Target{Destination: filepath.Join(home, "already.conf"), Kind: plan.CopyFile, PreState: plan.PreState{Type: plan.StateFile}},
			State:  transaction.EntryRestored,
		},
		{
			Target: plan.Target{Destination: filepath.Join(home, "ambiguous.conf"), Kind: plan.CopyFile, PreState: plan.PreState{Type: plan.StateFile}},
			State:  transaction.EntryOwnershipAmbiguous,
		},
	}}

	cands, err := restoreCandidates(inv)
	if err != nil {
		t.Fatalf("restoreCandidates() error = %v", err)
	}
	if len(cands) != 3 {
		t.Fatalf("len(cands) = %d, want 3 (restored/ambiguous excluded)", len(cands))
	}
	byDest := make(map[string]restoreCandidate, len(cands))
	for _, c := range cands {
		byDest[c.Entry.Target.Destination] = c
	}
	if byDest[unchanged].Modified {
		t.Errorf("unchanged target marked modified")
	}
	if !byDest[changed].Modified {
		t.Errorf("edited target not marked modified")
	}
	if !byDest[absent].Removal {
		t.Errorf("absent target not marked as removal")
	}
}
