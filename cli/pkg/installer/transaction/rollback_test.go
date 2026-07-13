package transaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// ---------- 2.4 RED/GREEN: rollback completeness ----------

func TestTransaction_Rollback_FailureAtFirstTarget(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	// Remove the source after planning so the first mutation fails.
	if err := os.Remove(src); err != nil {
		t.Fatalf("remove source: %v", err)
	}

	tx := New(p)
	rpt, err := tx.Execute()
	if err == nil {
		t.Fatal("expected mutation error")
	}

	if got := readFileString(t, dest); got != "original" {
		t.Errorf("dest content = %q, want original", got)
	}
	if rpt.ManagedTargets[0].Status != report.TargetFailed {
		t.Errorf("status = %q, want failed", rpt.ManagedTargets[0].Status)
	}
}

func TestTransaction_Rollback_FailureAfterMultipleMutations(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	dests := make([]string, 3)
	for i := range dests {
		dests[i] = filepath.Join(home, fmt.Sprintf("target-%d", i))
		mustWriteFile(t, dests[i], []byte(fmt.Sprintf("original-%d", i)))
	}

	sources := make([]string, 3)
	for i := range sources {
		sources[i] = filepath.Join(repo, fmt.Sprintf("src-%d", i))
		mustWriteFile(t, sources[i], []byte(fmt.Sprintf("new-%d", i)))
	}

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: sources[0], Destination: dests[0], Kind: plan.CopyFile},
		{Source: sources[1], Destination: dests[1], Kind: plan.CopyFile},
		{Source: sources[2], Destination: dests[2], Kind: plan.CopyFile},
	})

	// Delete the third source after planning so the first two commit and the third fails.
	if err := os.Remove(sources[2]); err != nil {
		t.Fatalf("remove source: %v", err)
	}

	tx := New(p)
	rpt, err := tx.Execute()
	if err == nil {
		t.Fatal("expected mutation error")
	}

	for i := 0; i < 2; i++ {
		if got := readFileString(t, dests[i]); got != fmt.Sprintf("original-%d", i) {
			t.Errorf("target %d content = %q, want original", i, got)
		}
		if rpt.ManagedTargets[i].Status != report.TargetRestored {
			t.Errorf("target %d status = %q, want restored", i, rpt.ManagedTargets[i].Status)
		}
	}
	if got := readFileString(t, dests[2]); got != fmt.Sprintf("original-%d", 2) {
		t.Errorf("target 2 content = %q, want original", got)
	}
	if rpt.ManagedTargets[2].Status != report.TargetFailed {
		t.Errorf("target 2 status = %q, want failed", rpt.ManagedTargets[2].Status)
	}
}

func TestTransaction_Rollback_ReverseOrder(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	dests := make([]string, 3)
	sources := make([]string, 3)
	for i := range dests {
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

	// Remove the third source to trigger failure after the first two mutations.
	if err := os.Remove(sources[2]); err != nil {
		t.Fatalf("remove source: %v", err)
	}

	rec := &recordingFS{Filesystem: OSFilesystem()}
	tx := New(p, WithFilesystem(rec))
	if _, err := tx.Execute(); err == nil {
		t.Fatal("expected mutation error")
	}

	// Rollback restores by copying backups back. We observe the reverse order via
	// Open calls that read the retained backup entries.
	var rollbackOpens []string
	for _, call := range rec.openCalls {
		if strings.Contains(call, ".dots-backups") {
			rollbackOpens = append(rollbackOpens, call)
		}
	}
	wantOrder := []string{targets[1].BackupPath, targets[0].BackupPath}
	if len(rollbackOpens) != len(wantOrder) {
		t.Fatalf("rollback open count = %d, want %d; calls=%v", len(rollbackOpens), len(wantOrder), rec.openCalls)
	}
	for i, want := range wantOrder {
		if rollbackOpens[i] != want {
			t.Errorf("rollback open #%d backup = %q, want %q", i, rollbackOpens[i], want)
		}
	}
}

func TestTransaction_Rollback_PartiallyChangedCurrentTarget(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	srcA := filepath.Join(repo, "src-a")
	mustWriteFile(t, srcA, []byte("new-a"))
	srcB := filepath.Join(repo, "src-b")
	mustWriteFile(t, srcB, []byte("new-b"))

	destA := filepath.Join(home, "target-a")
	mustWriteFile(t, destA, []byte("original-a"))
	destB := filepath.Join(home, "target-b")
	mustWriteFile(t, destB, []byte("original-b"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: srcA, Destination: destA, Kind: plan.CopyFile},
		{Source: srcB, Destination: destB, Kind: plan.CopyFile},
	})

	// Delete the second source after planning: first target commits, second target's
	// original is moved to backup but staging fails.
	if err := os.Remove(srcB); err != nil {
		t.Fatalf("remove source: %v", err)
	}

	tx := New(p)
	rpt, err := tx.Execute()
	if err == nil {
		t.Fatal("expected mutation error")
	}

	if got := readFileString(t, destA); got != "original-a" {
		t.Errorf("target-a content = %q, want original-a", got)
	}
	if got := readFileString(t, destB); got != "original-b" {
		t.Errorf("target-b content = %q, want original-b", got)
	}
	if rpt.ManagedTargets[0].Status != report.TargetRestored {
		t.Errorf("target-a status = %q, want restored", rpt.ManagedTargets[0].Status)
	}
	if rpt.ManagedTargets[1].Status != report.TargetFailed {
		t.Errorf("target-b status = %q, want failed", rpt.ManagedTargets[1].Status)
	}
}

func TestTransaction_Rollback_ContinuesAfterRestoreFailure(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	dests := make([]string, 3)
	sources := make([]string, 3)
	for i := range dests {
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

	// Modify the third destination after planning to cause drift and trigger rollback
	// of the first two targets.
	mustWriteFile(t, dests[2], []byte("changed"))

	targets := p.ManagedTargets()
	hook := &hookFS{
		Filesystem: OSFilesystem(),
		failOpen:   map[string]error{targets[0].BackupPath: errors.New("restore denied")},
	}
	tx := New(p, WithFilesystem(hook))
	rpt, err := tx.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	var rbErr *report.RollbackError
	if !errors.As(err, &rbErr) {
		t.Fatalf("expected *report.RollbackError, got %T", err)
	}
	if len(rbErr.Failures) != 1 {
		t.Fatalf("rollback failures = %d, want 1", len(rbErr.Failures))
	}
	if rbErr.Failures[0].Destination != dests[0] {
		t.Errorf("failure destination = %q, want %q", rbErr.Failures[0].Destination, dests[0])
	}

	// First target could not be restored, but the second one was.
	if got := readFileString(t, dests[0]); got != "new-0" {
		t.Errorf("target 0 content = %q, want new-0 (restore failed)", got)
	}
	if got := readFileString(t, dests[1]); got != "original-1" {
		t.Errorf("target 1 content = %q, want original-1", got)
	}
	if rpt.ManagedTargets[1].Status != report.TargetRestored {
		t.Errorf("target 1 status = %q, want restored", rpt.ManagedTargets[1].Status)
	}
}

func TestRollbackCombinedRestoreAndInventoryPersistenceFailures(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source, destination := filepath.Join(repo, "source"), filepath.Join(home, "destination")
	mustWriteFile(t, source, []byte("installed"))
	mustWriteFile(t, destination, []byte("original"))
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})

	tx := New(p, WithFilesystem(&hookFS{
		Filesystem: OSFilesystem(),
		failOpen:   map[string]error{p.ManagedTargets()[0].BackupPath: errors.New("restore denied")},
	}))
	if err := tx.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	persistErr := errors.New("inventory persistence denied")
	inventoryPersistFailure = func(*Inventory) error { return persistErr }
	t.Cleanup(func() { inventoryPersistFailure = nil })

	err := tx.Rollback()
	if !errors.Is(err, persistErr) {
		t.Fatalf("Rollback() error = %v, does not include persistence failure", err)
	}
	var rollbackErr *report.RollbackError
	if !errors.As(err, &rollbackErr) || len(rollbackErr.Failures) != 1 {
		t.Fatalf("Rollback() error = %v, want one restoration failure", err)
	}
	if got := tx.Inventory().Lifecycle; got != InventoryRecoveryIncomplete {
		t.Errorf("lifecycle = %q, want %q", got, InventoryRecoveryIncomplete)
	}
	entry := entryFor(t, tx.Inventory(), destination)
	if entry.Status != report.TargetFailed || entry.Error == nil {
		t.Errorf("outcome = status %q, error %v; want failed restoration outcome", entry.Status, entry.Error)
	}
	if _, statErr := os.Lstat(entry.BackupPath); statErr != nil {
		t.Errorf("backup not retained: %v", statErr)
	}
	rpt := tx.buildReport(err)
	if rpt.RecoveryState != report.RecoveryManualRecoveryRequired || len(rpt.RecoveryArtifacts) != 1 {
		t.Errorf("recovery report = state %q artifacts %+v", rpt.RecoveryState, rpt.RecoveryArtifacts)
	}
}

func TestRollbackSymlinkOwnershipPreservesExternalReplacement(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	source, destination := filepath.Join(repo, "source"), filepath.Join(home, "destination")
	if err := os.Symlink("installed", source); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("original", destination); err != nil {
		t.Fatal(err)
	}
	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.Symlink}})
	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("external", destination); err != nil {
		t.Fatal(err)
	}

	if err := tx.Rollback(); err == nil {
		t.Fatal("Rollback() succeeded after external symlink replacement")
	}
	if got, err := os.Readlink(destination); err != nil || got != "external" {
		t.Errorf("destination link = %q, %v; want external", got, err)
	}
	entry := entryFor(t, tx.Inventory(), destination)
	if entry.State != EntryOwnershipAmbiguous {
		t.Errorf("state = %q, want ownership-ambiguous", entry.State)
	}
	if _, err := os.Lstat(entry.BackupPath); err != nil {
		t.Errorf("backup not retained: %v", err)
	}
}

func TestTransaction_Rollback_BackupsRetainedAfterSuccess(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	entry := entryFor(t, tx.Inventory(), dest)
	if _, err := os.Stat(entry.BackupPath); err != nil {
		t.Errorf("backup was not retained after success: %v", err)
	}
}

func TestTransaction_Rollback_SourceDriftDoesNotCreateBackup(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	if err := os.Remove(src); err != nil {
		t.Fatalf("remove source: %v", err)
	}

	tx := New(p)
	if _, err := tx.Execute(); err == nil {
		t.Fatal("expected mutation error")
	}

	entry := entryFor(t, tx.Inventory(), dest)
	if _, err := os.Stat(entry.BackupPath); !os.IsNotExist(err) {
		t.Errorf("backup was created before source binding validation: %v", err)
	}
}

func TestTransaction_Rollback_ReportContainsManualRecoveryPaths(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	if err := os.Remove(src); err != nil {
		t.Fatalf("remove source: %v", err)
	}

	tx := New(p)
	rpt, err := tx.Execute()
	if err == nil {
		t.Fatal("expected mutation error")
	}

	entry := entryFor(t, tx.Inventory(), dest)
	if rpt.PrimaryCause == nil {
		t.Error("PrimaryCause is nil")
	}
	found := false
	for _, out := range rpt.ManagedTargets {
		if out.Destination == dest && out.BackupPath == entry.BackupPath {
			found = true
		}
	}
	if !found {
		t.Error("report missing target outcome with backup path for manual recovery")
	}
}

func TestTransaction_Rollback_DirectoryRestoreCopyFailureKeepsDestination(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	srcDir := filepath.Join(repo, "config")
	mustMkdir(t, filepath.Join(srcDir, "hypr"))
	mustWriteFile(t, filepath.Join(srcDir, "hypr", "conf"), []byte("new"))

	destDir := filepath.Join(home, ".config")
	mustMkdir(t, filepath.Join(destDir, "hypr"))
	mustWriteFile(t, filepath.Join(destDir, "hypr", "conf"), []byte("old"))

	srcFile := filepath.Join(repo, "file")
	mustWriteFile(t, srcFile, []byte("new file"))
	destFile := filepath.Join(home, "file")
	mustWriteFile(t, destFile, []byte("old file"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: srcDir, Destination: destDir, Kind: plan.CopyTree},
		{Source: srcFile, Destination: destFile, Kind: plan.CopyFile},
	})
	targets := p.ManagedTargets()

	// Drift on the second target triggers rollback of the committed directory.
	mustWriteFile(t, destFile, []byte("changed"))

	errRestoreCopy := errors.New("restore copy denied")
	fs := &hookFS{
		Filesystem:  OSFilesystem(),
		failReadDir: map[string]error{targets[0].BackupPath: errRestoreCopy},
	}
	tx := New(p, WithFilesystem(fs))
	_, err := tx.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	var rbErr *report.RollbackError
	if !errors.As(err, &rbErr) {
		t.Fatalf("expected *report.RollbackError, got %T", err)
	}
	if len(rbErr.Failures) != 1 || rbErr.Failures[0].Destination != destDir {
		t.Errorf("unexpected rollback failures: %+v", rbErr.Failures)
	}

	if _, err := os.Stat(destDir); err != nil {
		t.Fatalf("destination directory missing after failed restore: %v", err)
	}
	if got := readFileString(t, filepath.Join(destDir, "hypr", "conf")); got != "new" {
		t.Errorf("dest content = %q, want mutated new", got)
	}
}

// recordingFS records filesystem calls for order assertions.
type recordingFS struct {
	Filesystem
	renameCalls []renameCall
	createCalls []string
	openCalls   []string
}

type renameCall struct {
	Old string
	New string
}

func (r *recordingFS) Rename(oldpath, newpath string) error {
	r.renameCalls = append(r.renameCalls, renameCall{Old: oldpath, New: newpath})
	return r.Filesystem.Rename(oldpath, newpath)
}

func (r *recordingFS) Create(path string) (File, error) {
	r.createCalls = append(r.createCalls, path)
	return r.Filesystem.Create(path)
}

func (r *recordingFS) Open(path string) (File, error) {
	r.openCalls = append(r.openCalls, path)
	return r.Filesystem.Open(path)
}
