package transaction

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// ---------- fakes for plan construction ----------

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

type fixedRunID struct{ id string }

func (f fixedRunID) Generate(now time.Time) string { return f.id }

type fakeDiscoverer struct {
	targets []plan.Target
	err     error
}

func (f *fakeDiscoverer) Discover(repoRoot, homeDir string, opts plan.Options) ([]plan.Target, error) {
	return f.targets, f.err
}

type fakeCatalog struct {
	actions []plan.ExternalAction
	err     error
}

func (f *fakeCatalog) ExternalActions(opts plan.Options) ([]plan.ExternalAction, error) {
	return f.actions, f.err
}

func buildPlan(t *testing.T, repoRoot, homeDir string, targets []plan.Target) plan.InstallationPlan {
	t.Helper()
	planner := plan.New(
		plan.WithDiscoverer(&fakeDiscoverer{targets: targets}),
		plan.WithCatalog(&fakeCatalog{}),
		plan.WithClock(fakeClock{now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}),
		plan.WithRunIDSource(fixedRunID{id: "20260712T120000Z-test"}),
	)
	p, err := planner.Build(repoRoot, homeDir, plan.Options{Mode: "user"})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	return p
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(b)
}

func entryFor(t *testing.T, inv *Inventory, dest string) *InventoryEntry {
	t.Helper()
	for i := range inv.Entries {
		if inv.Entries[i].Target.Destination == dest {
			return &inv.Entries[i]
		}
	}
	t.Fatalf("no inventory entry for %s", dest)
	return nil
}

// ---------- 2.1 RED/GREEN: inventory and backup-before-write ----------

func TestTransaction_Prepare_CreatesInventoryAndBackupRoots(t *testing.T) {
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
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	inv := tx.Inventory()
	if len(inv.Entries) != 1 {
		t.Fatalf("len(Entries) = %d, want 1", len(inv.Entries))
	}
	entry := &inv.Entries[0]
	if entry.Status != report.TargetPending {
		t.Errorf("Status = %q, want pending", entry.Status)
	}
	if entry.BackupPath == "" {
		t.Error("BackupPath is empty")
	}
	if entry.Original.Type != plan.StateFile {
		t.Errorf("Original.Type = %q, want file", entry.Original.Type)
	}

	// Destination must not have been mutated during preparation.
	if got := readFileString(t, dest); got != "original" {
		t.Errorf("dest content = %q, want original", got)
	}

	backupRoot := filepath.Dir(entry.BackupPath)
	info, err := os.Stat(backupRoot)
	if err != nil {
		t.Fatalf("stat backup root: %v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Errorf("backup root mode = %o, want 0700", info.Mode().Perm())
	}

	if inv.Path == "" {
		t.Error("Inventory.Path is empty")
	}
	if _, err := os.Stat(inv.Path); err != nil {
		t.Fatalf("inventory file not retained: %v", err)
	}
}

func TestTransaction_Prepare_DeterministicBackupLocation(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("s"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("d"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})
	targets := p.ManagedTargets()
	want := plan.BackupPath(filepath.Dir(dest), p.RunID, dest)
	if targets[0].BackupPath != want {
		t.Errorf("BackupPath = %q, want %q", targets[0].BackupPath, want)
	}
}

func TestTransaction_Prepare_BackupPathCollision(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("s"))
	dest := filepath.Join(home, "dest")
	mustWriteFile(t, dest, []byte("d"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})
	backupPath := p.ManagedTargets()[0].BackupPath
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		t.Fatalf("mkdir backup root: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte("collision"), 0o600); err != nil {
		t.Fatalf("write collision: %v", err)
	}

	tx := New(p)
	err := tx.Prepare()
	if err == nil {
		t.Fatal("expected error for backup collision")
	}

	// The destination must remain untouched because the collision blocked preparation.
	if got := readFileString(t, dest); got != "d" {
		t.Errorf("dest content = %q, want d", got)
	}
}

func TestTransaction_Prepare_AbsentTargetInventory(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("s"))
	dest := filepath.Join(home, "absent-target")

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	entry := &tx.Inventory().Entries[0]
	if entry.Original.Type != plan.StateAbsent {
		t.Errorf("Original.Type = %q, want absent", entry.Original.Type)
	}
	if _, err := os.Stat(entry.BackupPath); !os.IsNotExist(err) {
		t.Error("backup entry for absent target should not exist on disk")
	}
}

func TestTransaction_Prepare_RootLevelFileBackupRoot(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, ".zshrc")
	mustWriteFile(t, src, []byte("zsh"))
	dest := filepath.Join(home, ".zshrc")
	mustWriteFile(t, dest, []byte("old zsh"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	backupRoot := filepath.Join(home, ".dots-backups", p.RunID)
	if _, err := os.Stat(backupRoot); err != nil {
		t.Fatalf("backup root for root-level file not created: %v", err)
	}
}

// ---------- 2.3 TRIANGULATE: atomic mutation and drift ----------

func TestTransaction_Execute_FileTarget(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new content"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original content"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	tx := New(p)
	rpt, err := tx.Execute()
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := readFileString(t, dest); got != "new content" {
		t.Errorf("dest content = %q, want new content", got)
	}

	entry := entryFor(t, tx.Inventory(), dest)
	if readFileString(t, entry.BackupPath) != "original content" {
		t.Errorf("backup content was not retained")
	}
	if entry.Status != report.TargetMutated {
		t.Errorf("entry.Status = %q, want mutated", entry.Status)
	}
	if rpt.ManagedTargets[0].Status != report.TargetMutated {
		t.Errorf("report status = %q, want mutated", rpt.ManagedTargets[0].Status)
	}
}

func TestTransaction_Execute_CopyFileUsesResolvedSource(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	resolved := filepath.Join(repo, "real", "source")
	mustMkdir(t, filepath.Dir(resolved))
	mustWriteFile(t, resolved, []byte("resolved content"))
	declared := filepath.Join(repo, "declared")
	if err := os.Symlink(filepath.Join("real", "source"), declared); err != nil {
		t.Fatalf("symlink source: %v", err)
	}
	dest := filepath.Join(home, "target")

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: declared, Destination: dest, Kind: plan.CopyFile},
	})
	if got := p.ManagedTargets()[0].ResolvedSource; got != resolved {
		t.Fatalf("ResolvedSource = %q, want %q", got, resolved)
	}

	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := readFileString(t, dest); got != "resolved content" {
		t.Errorf("dest content = %q, want resolved content", got)
	}
}

func TestTransaction_Execute_DirectoryTarget(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	srcDir := filepath.Join(repo, "config")
	mustMkdir(t, filepath.Join(srcDir, "hypr"))
	mustWriteFile(t, filepath.Join(srcDir, "hypr", "conf"), []byte("new"))
	destDir := filepath.Join(home, ".config")
	mustMkdir(t, filepath.Join(destDir, "hypr"))
	mustWriteFile(t, filepath.Join(destDir, "hypr", "conf"), []byte("old"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: srcDir, Destination: destDir, Kind: plan.CopyTree},
	})

	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := readFileString(t, filepath.Join(destDir, "hypr", "conf")); got != "new" {
		t.Errorf("dest content = %q, want new", got)
	}
	if got := readFileString(t, filepath.Join(tx.Inventory().Entries[0].BackupPath, "hypr", "conf")); got != "old" {
		t.Errorf("backup content = %q, want old", got)
	}
}

func TestTransaction_Execute_AbsentTargetCreation(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "new-target")

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := readFileString(t, dest); got != "new" {
		t.Errorf("dest content = %q, want new", got)
	}
	entry := entryFor(t, tx.Inventory(), dest)
	if entry.Status != report.TargetMutated {
		t.Errorf("Status = %q, want mutated", entry.Status)
	}
	if _, err := os.Stat(entry.BackupPath); !os.IsNotExist(err) {
		t.Error("absent target should not leave a backup entry on disk")
	}
}

func TestTransaction_Execute_SymlinkTarget(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	linkValue := filepath.Join(repo, "real")
	mustWriteFile(t, linkValue, []byte("real data"))
	src := filepath.Join(repo, "link-src")
	if err := os.Symlink(linkValue, src); err != nil {
		t.Fatalf("symlink source: %v", err)
	}

	dest := filepath.Join(home, "link-dest")
	oldValue := filepath.Join(home, "old-real")
	mustWriteFile(t, oldValue, []byte("old data"))
	if err := os.Symlink(oldValue, dest); err != nil {
		t.Fatalf("symlink dest: %v", err)
	}

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.Symlink},
	})

	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("readlink dest: %v", err)
	}
	if got != linkValue {
		t.Errorf("dest link = %q, want %q", got, linkValue)
	}

	entry := entryFor(t, tx.Inventory(), dest)
	backupLink, err := os.Readlink(entry.BackupPath)
	if err != nil {
		t.Fatalf("readlink backup: %v", err)
	}
	if backupLink != oldValue {
		t.Errorf("backup link = %q, want %q", backupLink, oldValue)
	}
}

func TestTransaction_Execute_Drift_SourceChangedAfterPlan(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	resolved := filepath.Join(repo, "real", "src")
	mustMkdir(t, filepath.Dir(resolved))
	mustWriteFile(t, resolved, []byte("reviewed content"))
	src := filepath.Join(repo, "src")
	if err := os.Symlink(filepath.Join("real", "src"), src); err != nil {
		t.Fatalf("symlink source: %v", err)
	}
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})
	mustWriteFile(t, resolved, []byte("changed after plan"))

	_, err := New(p).Execute()
	if err == nil {
		t.Fatal("expected source drift error")
	}
	var driftErr *report.PlanDriftError
	if !errors.As(err, &driftErr) {
		t.Fatalf("expected *report.PlanDriftError, got %T", err)
	}
	if got := readFileString(t, dest); got != "original" {
		t.Errorf("dest content = %q, want original", got)
	}
}

func TestTransaction_Execute_Drift_FileChanged(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	// The target changes between planning and execution.
	mustWriteFile(t, dest, []byte("changed after plan"))

	tx := New(p)
	rpt, err := tx.Execute()
	if err == nil {
		t.Fatal("expected drift error")
	}
	var driftErr *report.PlanDriftError
	if !errors.As(err, &driftErr) {
		t.Fatalf("expected *report.PlanDriftError, got %T", err)
	}

	if got := readFileString(t, dest); got != "changed after plan" {
		t.Errorf("dest content = %q, want unchanged", got)
	}
	if rpt.ManagedTargets[0].Status != report.TargetFailed {
		t.Errorf("report status = %q, want failed", rpt.ManagedTargets[0].Status)
	}
}

func TestTransaction_Execute_Drift_AbsentNowPresent(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	// Someone created the target after planning.
	mustWriteFile(t, dest, []byte("surprise"))

	tx := New(p)
	if _, err := tx.Execute(); err == nil {
		t.Fatal("expected drift error")
	}
	if got := readFileString(t, dest); got != "surprise" {
		t.Errorf("dest content = %q, want surprise", got)
	}
}

func TestTransaction_Execute_SpecialCharacterPaths(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src with spaces and ñ")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "dest with spaces and ñ")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got := readFileString(t, dest); got != "new" {
		t.Errorf("dest content = %q, want new", got)
	}
	entry := entryFor(t, tx.Inventory(), dest)
	if readFileString(t, entry.BackupPath) != "original" {
		t.Error("backup not retained for special-character path")
	}
}

func TestTransaction_Execute_NoDeleteThenCopyFallback(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	fs := &countingFS{Filesystem: OSFilesystem()}
	tx := New(p, WithFilesystem(fs))
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if fs.removeDestCount > 0 {
		t.Errorf("Remove(dest) called %d times; mutation must use atomic rename, not delete-then-copy", fs.removeDestCount)
	}
}

func TestTransaction_BackupValidationFails(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "target")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	backupPath := p.ManagedTargets()[0].BackupPath
	fs := &hookFS{
		Filesystem: OSFilesystem(),
		failCreate: map[string]error{backupPath: errors.New("cannot create backup")},
	}
	tx := New(p, WithFilesystem(fs))
	_, err := tx.Execute()
	if err == nil {
		t.Fatal("expected backup error")
	}
	var backupErr *report.BackupError
	if !errors.As(err, &backupErr) {
		t.Fatalf("expected *report.BackupError, got %T", err)
	}
	if got := readFileString(t, dest); got != "original" {
		t.Errorf("dest content = %q, want original", got)
	}
}

func TestTransaction_PersistInventoryError_IsPropagated(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "dest")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	backupPath := p.ManagedTargets()[0].BackupPath
	invPath := filepath.Join(filepath.Dir(backupPath), "inventory.json")
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		t.Fatalf("mkdir backup root: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte("collision"), 0o600); err != nil {
		t.Fatalf("write collision: %v", err)
	}

	errInventoryWrite := errors.New("inventory write denied")
	fs := &hookFS{
		Filesystem: OSFilesystem(),
		failCreate: map[string]error{invPath: errInventoryWrite},
	}
	tx := New(p, WithFilesystem(fs))
	err := tx.Prepare()
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errInventoryWrite) {
		t.Errorf("expected inventory write error propagated, got: %v", err)
	}
	var backupErr *report.BackupError
	if !errors.As(err, &backupErr) {
		t.Errorf("expected backup collision error retained, got: %v", err)
	}
}

func TestTransaction_Commit_BackupPathCollisionAfterPrepare(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "src")
	mustWriteFile(t, src, []byte("new"))
	dest := filepath.Join(home, "dest")
	mustWriteFile(t, dest, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})

	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	entry := entryFor(t, tx.Inventory(), dest)
	// Simulate a backup path that appears after Prepare collision checks.
	if err := os.WriteFile(entry.BackupPath, []byte("race"), 0o600); err != nil {
		t.Fatalf("write race file: %v", err)
	}

	err := tx.Commit()
	if err == nil {
		t.Fatal("expected error for backup path collision at Commit")
	}
	var backupErr *report.BackupError
	if !errors.As(err, &backupErr) {
		t.Fatalf("expected *report.BackupError, got %T", err)
	}
	if got := readFileString(t, dest); got != "original" {
		t.Errorf("dest content = %q, want original", got)
	}
}

// ---------- review correction: review-ff6ca3152ae6ea7c ----------

// RISK-001: symlink commit content must be bound during Prepare so Commit cannot
// re-read a source link that changed after the plan was reviewed.
func TestTransaction_Commit_SymlinkUsesBoundLinkValue(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	firstValue := filepath.Join(repo, "first-real")
	mustWriteFile(t, firstValue, []byte("first"))
	secondValue := filepath.Join(repo, "second-real")
	mustWriteFile(t, secondValue, []byte("second"))

	src := filepath.Join(repo, "link-src")
	if err := os.Symlink(firstValue, src); err != nil {
		t.Fatalf("symlink source: %v", err)
	}

	dest := filepath.Join(home, "link-dest")
	oldValue := filepath.Join(home, "old-real")
	mustWriteFile(t, oldValue, []byte("old"))
	if err := os.Symlink(oldValue, dest); err != nil {
		t.Fatalf("symlink dest: %v", err)
	}

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.Symlink},
	})

	tx := New(p)
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	// After Prepare bound the link value, an attacker or race changes the source
	// symlink to a different target.
	if err := os.Remove(src); err != nil {
		t.Fatalf("remove source symlink: %v", err)
	}
	if err := os.Symlink(secondValue, src); err != nil {
		t.Fatalf("relink source: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("readlink dest: %v", err)
	}
	if got != firstValue {
		t.Errorf("dest link = %q, want bound value %q", got, firstValue)
	}
}

// RESILIENCE-001 / RELIABILITY-001: if the final staged rename for a directory
// replacement fails, the original directory must be restored and the target must
// not be recorded as mutated.
func TestTransaction_Commit_DirectoryFinalRenameFailureRestoresOriginal(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	srcDir := filepath.Join(repo, "config")
	mustMkdir(t, filepath.Join(srcDir, "hypr"))
	mustWriteFile(t, filepath.Join(srcDir, "hypr", "conf"), []byte("new"))

	destDir := filepath.Join(home, ".config")
	mustMkdir(t, filepath.Join(destDir, "hypr"))
	mustWriteFile(t, filepath.Join(destDir, "hypr", "conf"), []byte("old"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: srcDir, Destination: destDir, Kind: plan.CopyTree},
	})

	errCommitRename := errors.New("commit rename denied")
	fs := &hookFS{
		Filesystem: OSFilesystem(),
		renameHook: func(oldpath, newpath string) error {
			// Fail only the staged directory -> destination rename, not the
			// subsequent recovery rename from the trash path.
			if newpath == destDir && strings.Contains(oldpath, ".dots-staging-") && !strings.HasSuffix(oldpath, ".dots-trash") {
				return errCommitRename
			}
			return nil
		},
	}

	tx := New(p, WithFilesystem(fs))
	_, err := tx.Execute()
	if err == nil {
		t.Fatal("expected error for failed final directory rename")
	}

	if _, statErr := os.Stat(destDir); statErr != nil {
		t.Fatalf("destination directory missing after failed commit: %v", statErr)
	}
	if got := readFileString(t, filepath.Join(destDir, "hypr", "conf")); got != "old" {
		t.Errorf("dest content = %q, want original old", got)
	}

	// The target must not appear mutated; rollback would have nothing to do.
	entry := entryFor(t, tx.Inventory(), destDir)
	if entry.Status == report.TargetMutated {
		t.Error("directory target recorded as mutated despite failed commit")
	}
}

// countingFS records destructive operations on destination paths.
type countingFS struct {
	Filesystem
	removeDestCount int
}

func (c *countingFS) Remove(path string) error {
	c.removeDestCount++
	return c.Filesystem.Remove(path)
}

// hookFS can be configured to fail specific filesystem calls.
type hookFS struct {
	Filesystem
	failRename     map[string]error
	failCreate     map[string]error
	failOpen       map[string]error
	failCreateTemp map[string]error
	failReadDir    map[string]error
	renameHook     func(oldpath, newpath string) error
}

func (h *hookFS) Rename(oldpath, newpath string) error {
	if h.renameHook != nil {
		if err := h.renameHook(oldpath, newpath); err != nil {
			return err
		}
	}
	if err, ok := h.failRename[oldpath]; ok {
		return err
	}
	if err, ok := h.failRename[newpath]; ok {
		return err
	}
	return h.Filesystem.Rename(oldpath, newpath)
}

func (h *hookFS) Create(path string) (File, error) {
	if err, ok := h.failCreate[path]; ok {
		return nil, err
	}
	return h.Filesystem.Create(path)
}

func (h *hookFS) Open(path string) (File, error) {
	if err, ok := h.failOpen[path]; ok {
		return nil, err
	}
	return h.Filesystem.Open(path)
}

func (h *hookFS) CreateTemp(dir, pattern string) (File, string, error) {
	if err, ok := h.failCreateTemp[dir]; ok {
		return nil, "", err
	}
	return h.Filesystem.CreateTemp(dir, pattern)
}

func (h *hookFS) ReadDir(path string) ([]os.DirEntry, error) {
	if err, ok := h.failReadDir[path]; ok {
		return nil, err
	}
	return h.Filesystem.ReadDir(path)
}
