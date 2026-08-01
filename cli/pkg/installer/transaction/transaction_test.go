package transaction

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"

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
	dest := filepath.Join(home, ".config", "target")
	mustMkdir(t, filepath.Dir(dest))
	mustWriteFile(t, dest, []byte("d"))

	p := buildPlan(t, repo, home, []plan.Target{
		{Source: src, Destination: dest, Kind: plan.CopyFile},
	})
	targets := p.ManagedTargets()
	want := plan.BackupPath(home, p.RunID, dest)
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

func TestBackupRootSymlink_IsRefused(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, "source")
	mustWriteFile(t, source, []byte("new"))
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, destination, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	root := filepath.Dir(p.ManagedTargets()[0].BackupPath)
	if err := os.Mkdir(filepath.Dir(root), 0o700); err != nil {
		t.Fatalf("create backup parent: %v", err)
	}
	attacker := t.TempDir()
	if err := os.Symlink(attacker, root); err != nil {
		t.Fatalf("insert backup-root symlink: %v", err)
	}

	err := New(p).Prepare()
	if err == nil || !strings.Contains(err.Error(), root) {
		t.Fatalf("Prepare() error = %v, want symlink error naming %q", err, root)
	}
	if got := readFileString(t, destination); got != "original" {
		t.Errorf("destination = %q, want unchanged", got)
	}
	if _, err := os.Stat(filepath.Join(attacker, "inventory.json")); !os.IsNotExist(err) {
		t.Errorf("inventory was written through symlink: %v", err)
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

func TestTransaction_Execute_MoonArchRuntimeWithCleanHome(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, "moonarch", "runtime")
	mustMkdir(t, source)
	mustWriteFile(t, filepath.Join(source, "moonarch"), []byte("runtime"))
	destination := filepath.Join(home, ".local", "moonarch", "runtime")

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyTree}})
	if _, err := New(p).Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := readFileString(t, filepath.Join(destination, "moonarch")); got != "runtime" {
		t.Errorf("runtime content = %q, want runtime", got)
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

func TestTransaction_Execute_Drift_DeclaredSourceSymlinkSwappedAfterPlan(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	reviewed := filepath.Join(repo, "reviewed")
	mustWriteFile(t, reviewed, []byte("reviewed content"))
	attacker := filepath.Join(repo, "attacker")
	mustWriteFile(t, attacker, []byte("attacker content"))
	source := filepath.Join(repo, "source")
	if err := os.Symlink(reviewed, source); err != nil {
		t.Fatalf("create source symlink: %v", err)
	}
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, destination, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	if err := os.Remove(source); err != nil {
		t.Fatalf("remove source symlink: %v", err)
	}
	if err := os.Symlink(attacker, source); err != nil {
		t.Fatalf("swap source symlink: %v", err)
	}

	tx := New(p)
	_, err := tx.Execute()
	var drift *report.PlanDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Execute() error = %v, want PlanDriftError", err)
	}
	if got := readFileString(t, destination); got != "original" {
		t.Errorf("destination = %q, want original", got)
	}
	if _, err := os.Lstat(tx.Inventory().Entries[0].BackupPath); !os.IsNotExist(err) {
		t.Errorf("backup was created before source validation: %v", err)
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
	if err := os.MkdirAll(backupPath, 0o700); err != nil {
		t.Fatalf("create backup collision: %v", err)
	}
	tx := New(p)
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
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o700); err != nil {
		t.Fatalf("mkdir backup root: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte("collision"), 0o600); err != nil {
		t.Fatalf("write collision: %v", err)
	}

	tx := New(p)
	err := tx.Prepare()
	if err == nil {
		t.Fatal("expected error")
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

func TestTransaction_Execute_SourceDriftBeforeBackup(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, "source")
	mustWriteFile(t, source, []byte("reviewed"))
	attacker := filepath.Join(repo, "attacker")
	mustWriteFile(t, attacker, []byte("attacker"))
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, destination, []byte("original"))

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	if err := os.Remove(source); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	if err := os.Symlink(attacker, source); err != nil {
		t.Fatalf("replace source with symlink: %v", err)
	}

	tx := New(p)
	_, err := tx.Execute()
	var drift *report.PlanDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Execute() error = %v, want PlanDriftError", err)
	}
	if got := readFileString(t, destination); got != "original" {
		t.Errorf("destination = %q, want original", got)
	}
	if _, err := os.Lstat(tx.Inventory().Entries[0].BackupPath); !os.IsNotExist(err) {
		t.Errorf("backup was created before source validation: %v", err)
	}
}

func TestTransaction_Execute_LegacyDirectTarget(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, "source")
	mustWriteFile(t, source, []byte("legacy"))
	destination := filepath.Join(home, "destination")

	p, err := plan.NewInstallationPlan("legacy", []plan.Target{{
		Source: source, Destination: destination, Kind: plan.CopyFile, PreState: plan.PreState{Type: plan.StateAbsent},
		BackupPath: plan.BackupPath(filepath.Dir(destination), "legacy", destination),
	}})
	if err != nil {
		t.Fatalf("NewInstallationPlan() error = %v", err)
	}
	if _, err := New(p).Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := readFileString(t, destination); got != "legacy" {
		t.Errorf("destination = %q, want legacy", got)
	}
}

func TestTransaction_Execute_ExactFileMode(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, "source")
	mustWriteFile(t, source, []byte("content"))
	if err := os.Chmod(source, os.FileMode(0o755)|os.ModeSetuid); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	destination := filepath.Join(home, "destination")

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	if _, err := New(p).Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatalf("lstat destination: %v", err)
	}
	if got, want := info.Mode(), os.FileMode(0o755)|os.ModeSetuid; got != want {
		t.Errorf("destination mode = %#o, want %#o", got, want)
	}
}

func TestTransaction_Execute_ExactDirectoryMode(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	srcDir := filepath.Join(repo, "config")
	mustMkdir(t, filepath.Join(srcDir, "sub"))
	mustWriteFile(t, filepath.Join(srcDir, "sub", "file"), []byte("x"))
	if err := os.Chmod(srcDir, os.FileMode(0o755)|os.ModeSticky); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	if err := os.Chmod(filepath.Join(srcDir, "sub"), os.FileMode(0o755)|os.ModeSetgid); err != nil {
		t.Fatalf("chmod sub: %v", err)
	}
	destDir := filepath.Join(home, ".config")

	p := buildPlan(t, repo, home, []plan.Target{{Source: srcDir, Destination: destDir, Kind: plan.CopyTree}})
	if _, err := New(p).Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	mask := os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	info, err := os.Lstat(destDir)
	if err != nil {
		t.Fatalf("lstat destination: %v", err)
	}
	if got, want := info.Mode()&mask, os.FileMode(0o755)|os.ModeSticky; got != want {
		t.Errorf("destination mode = %#o, want %#o", got, want)
	}
	info, err = os.Lstat(filepath.Join(destDir, "sub"))
	if err != nil {
		t.Fatalf("lstat sub: %v", err)
	}
	if got, want := info.Mode()&mask, os.FileMode(0o755)|os.ModeSetgid; got != want {
		t.Errorf("sub mode = %#o, want %#o", got, want)
	}
}

func TestTransaction_Execute_ExactBackupFileModesUnderUmask(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, "source")
	mustWriteFile(t, source, []byte("new"))
	if err := os.Chmod(source, 0o755|os.ModeSetuid); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	destination := filepath.Join(home, "destination")
	mustWriteFile(t, destination, []byte("old"))
	if err := os.Chmod(destination, 0o640); err != nil {
		t.Fatalf("chmod destination: %v", err)
	}

	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyFile}})
	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	mask := os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat %s: %v", path, err)
		}
		if got := info.Mode() & mask; got != want {
			t.Errorf("mode for %s = %#o, want %#o", path, got, want)
		}
	}
	assertMode(destination, 0o755|os.ModeSetuid)
	assertMode(entryFor(t, tx.Inventory(), destination).BackupPath, 0o640)
}

func TestTransaction_Execute_ExactBackupDirectoryModes(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	source := filepath.Join(repo, "source")
	sourceNested := filepath.Join(source, "nested")
	mustMkdir(t, sourceNested)
	mustWriteFile(t, filepath.Join(sourceNested, "new.conf"), []byte("new"))
	if err := os.Chmod(source, 0o750|os.ModeSticky); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	if err := os.Chmod(sourceNested, 0o750|os.ModeSetgid); err != nil {
		t.Fatalf("chmod source nested: %v", err)
	}

	destination := filepath.Join(home, "destination")
	destinationNested := filepath.Join(destination, "nested")
	mustMkdir(t, destinationNested)
	mustWriteFile(t, filepath.Join(destinationNested, "old.conf"), []byte("old"))
	if err := os.Chmod(destination, 0o750|os.ModeSticky); err != nil {
		t.Fatalf("chmod destination: %v", err)
	}
	if err := os.Chmod(destinationNested, 0o750|os.ModeSetgid); err != nil {
		t.Fatalf("chmod destination nested: %v", err)
	}

	oldUmask := syscall.Umask(0o077)
	defer syscall.Umask(oldUmask)

	p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: plan.CopyTree}})
	tx := New(p)
	if _, err := tx.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	mask := os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky
	assertMode := func(path string, want os.FileMode) {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("lstat %s: %v", path, err)
		}
		if got := info.Mode() & mask; got != want {
			t.Errorf("mode for %s = %#o, want %#o", path, got, want)
		}
	}

	assertMode(destination, 0o750|os.ModeSticky)
	assertMode(filepath.Join(destination, "nested"), 0o750|os.ModeSetgid)
	backup := entryFor(t, tx.Inventory(), destination).BackupPath
	assertMode(backup, 0o750|os.ModeSticky)
	assertMode(filepath.Join(backup, "nested"), 0o750|os.ModeSetgid)
}

func TestTransaction_Execute_DirectorySourceDrift(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	srcDir := filepath.Join(repo, "config")
	mustMkdir(t, srcDir)
	mustWriteFile(t, filepath.Join(srcDir, "conf"), []byte("reviewed"))
	destDir := filepath.Join(home, ".config")
	mustMkdir(t, destDir)
	mustWriteFile(t, filepath.Join(destDir, "conf"), []byte("old"))

	p := buildPlan(t, repo, home, []plan.Target{{Source: srcDir, Destination: destDir, Kind: plan.CopyTree}})

	attacker := filepath.Join(repo, "attacker")
	mustWriteFile(t, attacker, []byte("attacker"))
	if err := os.Remove(filepath.Join(srcDir, "conf")); err != nil {
		t.Fatalf("remove source file: %v", err)
	}
	if err := os.Symlink(attacker, filepath.Join(srcDir, "conf")); err != nil {
		t.Fatalf("replace with symlink: %v", err)
	}

	tx := New(p)
	_, err := tx.Execute()
	var drift *report.PlanDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Execute() error = %v, want PlanDriftError", err)
	}
	if got := readFileString(t, filepath.Join(destDir, "conf")); got != "old" {
		t.Errorf("destination = %q, want old", got)
	}
	if _, err := os.Lstat(tx.Inventory().Entries[0].BackupPath); !os.IsNotExist(err) {
		t.Errorf("backup was created before source validation: %v", err)
	}
}

func TestTransaction_Execute_Drift_RootSourceModeChangedAfterPlan(t *testing.T) {
	for _, tt := range []struct {
		name        string
		kind        plan.MutationKind
		setup       func(*testing.T, string)
		destination func(*testing.T, string)
	}{
		{
			name:        "file",
			kind:        plan.CopyFile,
			setup:       func(t *testing.T, source string) { mustWriteFile(t, source, []byte("reviewed")) },
			destination: func(t *testing.T, destination string) { mustWriteFile(t, destination, []byte("original")) },
		},
		{
			name: "directory",
			kind: plan.CopyTree,
			setup: func(t *testing.T, source string) {
				mustMkdir(t, source)
				mustWriteFile(t, filepath.Join(source, "config"), []byte("reviewed"))
			},
			destination: func(t *testing.T, destination string) {
				mustMkdir(t, destination)
				mustWriteFile(t, filepath.Join(destination, "config"), []byte("original"))
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			source := filepath.Join(repo, "source")
			destination := filepath.Join(home, "destination")
			tt.setup(t, source)
			tt.destination(t, destination)

			p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: tt.kind}})
			if err := os.Chmod(source, 0o700); err != nil {
				t.Fatalf("chmod source: %v", err)
			}

			tx := New(p)
			_, err := tx.Execute()
			var drift *report.PlanDriftError
			if !errors.As(err, &drift) {
				t.Fatalf("Execute() error = %v, want PlanDriftError", err)
			}
			if _, err := os.Lstat(tx.Inventory().Entries[0].BackupPath); !os.IsNotExist(err) {
				t.Errorf("backup was created before source validation: %v", err)
			}
			if tt.kind == plan.CopyFile {
				if got := readFileString(t, destination); got != "original" {
					t.Errorf("destination = %q, want original", got)
				}
				return
			}
			if got := readFileString(t, filepath.Join(destination, "config")); got != "original" {
				t.Errorf("destination = %q, want original", got)
			}
		})
	}
}

// ---------- PR2 durability correction ----------

func TestTransaction_Commit_BackupSyncFailurePreventsCheckpointAndMutation(t *testing.T) {
	for _, tt := range []struct {
		name          string
		kind          plan.MutationKind
		setupTarget   func(*testing.T, string)
		setupSource   func(*testing.T, string)
		requiredSyncs int
	}{
		{
			name:          "file backup file and root directory syncs",
			kind:          plan.CopyFile,
			setupTarget:   func(t *testing.T, path string) { mustWriteFile(t, path, []byte("original")) },
			setupSource:   func(t *testing.T, path string) { mustWriteFile(t, path, []byte("replacement")) },
			requiredSyncs: 2,
		},
		{
			name: "directory backup file child directories and root syncs",
			kind: plan.CopyTree,
			setupTarget: func(t *testing.T, path string) {
				mustMkdir(t, filepath.Join(path, "nested"))
				mustWriteFile(t, filepath.Join(path, "nested", "original"), []byte("original"))
			},
			setupSource: func(t *testing.T, path string) {
				mustMkdir(t, filepath.Join(path, "nested"))
				mustWriteFile(t, filepath.Join(path, "nested", "replacement"), []byte("replacement"))
			},
			requiredSyncs: 4,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for failAt := 1; failAt <= tt.requiredSyncs; failAt++ {
				t.Run(fmt.Sprintf("sync_%d", failAt), func(t *testing.T) {
					repo, home := t.TempDir(), t.TempDir()
					source := filepath.Join(repo, "source")
					destination := filepath.Join(home, "destination")
					tt.setupSource(t, source)
					tt.setupTarget(t, destination)
					p := buildPlan(t, repo, home, []plan.Target{{Source: source, Destination: destination, Kind: tt.kind}})
					tx := New(p)
					if err := tx.Prepare(); err != nil {
						t.Fatalf("Prepare: %v", err)
					}

					var syncs int
					backupSync = func(fd int) error {
						syncs++
						if syncs == failAt {
							return errors.New("injected backup sync failure")
						}
						return unix.Fsync(fd)
					}
					checkpointed := false
					backupCheckpointPersisted = func() { checkpointed = true }
					t.Cleanup(func() {
						backupSync = unix.Fsync
						backupCheckpointPersisted = nil
					})

					err := tx.Commit()
					if err == nil || !strings.Contains(err.Error(), "injected backup sync failure") {
						t.Fatalf("Commit() error = %v, want injected backup sync failure", err)
					}
					if checkpointed {
						t.Fatal("backed-up checkpoint persisted after backup sync failure")
					}
					if got := entryFor(t, tx.Inventory(), destination).State; got == EntryBackedUp || got == EntryMutated {
						t.Fatalf("entry state = %q after backup sync failure", got)
					}
					if tt.kind == plan.CopyFile {
						if got := readFileString(t, destination); got != "original" {
							t.Fatalf("destination = %q, want original", got)
						}
					} else if got := readFileString(t, filepath.Join(destination, "nested", "original")); got != "original" {
						t.Fatalf("destination tree mutated: %q", got)
					}
				})
			}
		})
	}
}

// ---------- review correction: review-ff6ca3152ae6ea7c ----------

// RISK-001: a symlink source must fail if its link value changes after planning.
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

	err := tx.Commit()
	var drift *report.PlanDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Commit() error = %v, want PlanDriftError", err)
	}
	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("readlink dest: %v", err)
	}
	if got != oldValue {
		t.Errorf("dest link = %q, want original value %q", got, oldValue)
	}
}

func TestTransaction_Commit_SymlinkConsumesPlannerBoundLinkValue(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	reviewedValue := filepath.Join(repo, "reviewed-real")
	mustWriteFile(t, reviewedValue, []byte("reviewed"))
	stalePreparedValue := filepath.Join(repo, "stale-prepared-real")
	mustWriteFile(t, stalePreparedValue, []byte("stale"))

	src := filepath.Join(repo, "link-src")
	if err := os.Symlink(reviewedValue, src); err != nil {
		t.Fatalf("symlink source: %v", err)
	}
	dest := filepath.Join(home, "link-dest")

	p := buildPlan(t, repo, home, []plan.Target{{
		Source: src, Destination: dest, Kind: plan.Symlink,
	}})

	// Simulate the X → Y → X link-value transition without replacing the
	// symlink inode that the planner bound. Prepare observes stale Y, while
	// commit validation observes the reviewed X value again.
	fs := &readlinkSequenceFS{
		Filesystem: OSFilesystem(),
		values:     []string{stalePreparedValue, reviewedValue},
	}
	tx := New(p, WithFilesystem(fs))
	if err := tx.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if got := entryFor(t, tx.Inventory(), dest).LinkValue; got != stalePreparedValue {
		t.Fatalf("prepared link value = %q, want stale value %q", got, stalePreparedValue)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	got, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("readlink destination: %v", err)
	}
	if got != reviewedValue {
		t.Errorf("destination link = %q, want reviewed planner value %q", got, reviewedValue)
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

type readlinkSequenceFS struct {
	Filesystem
	values []string
}

func (f *readlinkSequenceFS) Readlink(name string) (string, error) {
	if len(f.values) == 0 {
		return f.Filesystem.Readlink(name)
	}
	value := f.values[0]
	f.values = f.values[1:]
	return value, nil
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
