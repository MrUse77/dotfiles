package transaction

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// Task 5.4: mutateTarget dispatch on plan.Remove.

// Unchanged retired target is backed up, deleted, and recorded as removed.
func TestTransaction_Remove_UnchangedRetiredTargetIsDeletedAndRecorded(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	aDest := filepath.Join(home, ".config", "a")
	mustMkdir(t, filepath.Dir(aDest))
	mustWriteFile(t, aDest, []byte("installed a"))

	p := buildPlan(t, repo, home, []plan.Target{{Destination: aDest, Kind: plan.Remove}})
	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Lstat(aDest); !os.IsNotExist(err) {
		t.Fatalf("retired destination still exists: %v", err)
	}
	entry := entryFor(t, tx.Inventory(), aDest)
	if entry.State != EntryRemoved {
		t.Errorf("entry.State = %q, want %q", entry.State, EntryRemoved)
	}
	if entry.Status != report.TargetMutated {
		t.Errorf("entry.Status = %q, want %q", entry.Status, report.TargetMutated)
	}
	if got := readFileString(t, entry.BackupPath); got != "installed a" {
		t.Errorf("backup content = %q, want original", got)
	}
}

// Retired target already absent: removal is skipped and never recreated.
func TestTransaction_Remove_AbsentRetiredTargetIsSkipped(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	aDest := filepath.Join(home, ".config", "a")
	mustMkdir(t, filepath.Dir(aDest))

	p := buildPlan(t, repo, home, []plan.Target{{Destination: aDest, Kind: plan.Remove}})
	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, err := os.Lstat(aDest); !os.IsNotExist(err) {
		t.Fatalf("skipped removal must not recreate the target: %v", err)
	}
	entry := entryFor(t, tx.Inventory(), aDest)
	if entry.State != EntrySkipped {
		t.Errorf("entry.State = %q, want %q", entry.State, EntrySkipped)
	}
	if entry.Status != report.TargetSkipped {
		t.Errorf("entry.Status = %q, want %q", entry.Status, report.TargetSkipped)
	}
	if _, err := os.Lstat(entry.BackupPath); !os.IsNotExist(err) {
		t.Errorf("absent target produced a backup entry: %v", err)
	}
}

// A later mutation failure rolls back an already-deleted removal from backup.
func TestTransaction_Remove_RollbackRestoresDeletedFile(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	// Removal target sorts before the failing replacement by destination.
	aDest := filepath.Join(home, ".config", "a")
	mustMkdir(t, filepath.Dir(aDest))
	mustWriteFile(t, aDest, []byte("installed a"))
	bDest := filepath.Join(home, "other", "b")
	bSrc := filepath.Join(repo, "b")
	mustWriteFile(t, bSrc, []byte("new b"))
	mustMkdir(t, filepath.Dir(bDest))
	mustWriteFile(t, bDest, []byte("old b"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Destination: aDest, Kind: plan.Remove},
		{Source: bSrc, Destination: bDest, Kind: plan.CopyFile},
	})
	tx := New(p, WithFilesystem(&hookFS{
		Filesystem:     OSFilesystem(),
		failCreateTemp: map[string]error{filepath.Dir(bDest): errors.New("stage denied")},
	}))
	_, err := tx.Execute()
	if err == nil {
		t.Fatal("Execute() expected failure on the replacement target")
	}
	// The removal must have been rolled back: original content restored.
	if got := readFileString(t, aDest); got != "installed a" {
		t.Errorf("removed target not restored after rollback: %q", got)
	}
	entry := entryFor(t, tx.Inventory(), aDest)
	if entry.State != EntryRestored {
		t.Errorf("entry.State = %q, want %q (EntryRemoved -> EntryRestored)", entry.State, EntryRestored)
	}
	if got := readFileString(t, bDest); got != "old b" {
		t.Errorf("failing replacement destination = %q, want unchanged", got)
	}
}

// Directory removal rollback restores the whole deleted tree.
func TestTransaction_Remove_RollbackRestoresDeletedDirectory(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	aDest := filepath.Join(home, ".config", "a")
	mustMkdir(t, filepath.Join(aDest, "sub"))
	mustWriteFile(t, filepath.Join(aDest, "sub", "file"), []byte("installed tree"))
	bDest := filepath.Join(home, "other", "b")
	bSrc := filepath.Join(repo, "b")
	mustWriteFile(t, bSrc, []byte("new b"))
	mustMkdir(t, filepath.Dir(bDest))
	mustWriteFile(t, bDest, []byte("old b"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Destination: aDest, Kind: plan.Remove},
		{Source: bSrc, Destination: bDest, Kind: plan.CopyFile},
	})
	tx := New(p, WithFilesystem(&hookFS{
		Filesystem:     OSFilesystem(),
		failCreateTemp: map[string]error{filepath.Dir(bDest): errors.New("stage denied")},
	}))
	_, err := tx.Execute()
	if err == nil {
		t.Fatal("Execute() expected failure on the replacement target")
	}
	if got := readFileString(t, filepath.Join(aDest, "sub", "file")); got != "installed tree" {
		t.Errorf("removed directory not restored after rollback: %q", got)
	}
	entry := entryFor(t, tx.Inventory(), aDest)
	if entry.State != EntryRestored {
		t.Errorf("entry.State = %q, want %q", entry.State, EntryRestored)
	}
}
