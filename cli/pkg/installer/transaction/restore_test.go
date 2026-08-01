package transaction

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func mustWriteRestoreFixture(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func restorePlan(t *testing.T, targets []plan.Target) plan.InstallationPlan {
	t.Helper()
	p, err := plan.NewInstallationPlan("restore-test", targets)
	if err != nil {
		t.Fatalf("NewInstallationPlan: %v", err)
	}
	return p
}

func TestTransaction_RestoreTarget_File(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".zshrc")
	backup := filepath.Join(home, ".dots-backups", "restore-test", "2f2e7a73687263")

	mustWriteRestoreFixture(t, backup, []byte("original zsh"), 0o600)
	mustWriteRestoreFixture(t, dest, []byte("installed zsh"), 0o644)

	p := restorePlan(t, []plan.Target{{
		Destination: dest,
		Kind:        plan.CopyFile,
		PreState:    plan.PreState{Type: plan.StateFile, Mode: 0o600},
		BackupPath:  backup,
	}})

	if err := New(p).RestoreTarget(p.ManagedTargets()[0]); err != nil {
		t.Fatalf("RestoreTarget() error = %v", err)
	}
	if got := readFileString(t, dest); got != "original zsh" {
		t.Errorf("destination = %q, want original zsh", got)
	}
	info, err := os.Lstat(dest)
	if err != nil {
		t.Fatalf("stat restored destination: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("restored mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestTransaction_RestoreTarget_Directory(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".config", "hypr")
	backup := filepath.Join(home, ".dots-backups", "restore-test", "2e636f6e6669672f68797072")

	mustWriteRestoreFixture(t, filepath.Join(backup, "hyprland.conf"), []byte("original"), 0o644)
	mustWriteRestoreFixture(t, filepath.Join(dest, "hyprland.conf"), []byte("installed"), 0o644)

	p := restorePlan(t, []plan.Target{{
		Destination: dest,
		Kind:        plan.CopyTree,
		PreState:    plan.PreState{Type: plan.StateDirectory, Mode: 0o755},
		BackupPath:  backup,
	}})

	if err := New(p).RestoreTarget(p.ManagedTargets()[0]); err != nil {
		t.Fatalf("RestoreTarget() error = %v", err)
	}
	if got := readFileString(t, filepath.Join(dest, "hyprland.conf")); got != "original" {
		t.Errorf("restored file = %q, want original", got)
	}
}

func TestTransaction_RestoreTarget_Symlink(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, "theme-link")
	backup := filepath.Join(home, ".dots-backups", "restore-test", "2e7468656d652d6c696e6b")
	target := filepath.Join(home, "real-theme")

	mustWriteRestoreFixture(t, target, []byte("theme"), 0o644)
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		t.Fatalf("mkdir backup root: %v", err)
	}
	if err := os.Symlink(target, backup); err != nil {
		t.Fatalf("symlink backup: %v", err)
	}
	if err := os.Symlink("elsewhere", dest); err != nil {
		t.Fatalf("symlink destination: %v", err)
	}

	p := restorePlan(t, []plan.Target{{
		Destination: dest,
		Kind:        plan.Symlink,
		PreState:    plan.PreState{Type: plan.StateSymlink},
		BackupPath:  backup,
	}})

	if err := New(p).RestoreTarget(p.ManagedTargets()[0]); err != nil {
		t.Fatalf("RestoreTarget() error = %v", err)
	}
	link, err := os.Readlink(dest)
	if err != nil {
		t.Fatalf("readlink restored destination: %v", err)
	}
	if link != target {
		t.Errorf("restored link = %q, want %q", link, target)
	}
}

func TestTransaction_RestoreTarget_AbsentRemovesInstalledDestination(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, "installed-only-file")

	mustWriteRestoreFixture(t, dest, []byte("created by install"), 0o644)

	p := restorePlan(t, []plan.Target{{
		Destination: dest,
		Kind:        plan.CopyFile,
		PreState:    plan.PreState{Type: plan.StateAbsent},
		BackupPath:  "",
	}})

	if err := New(p).RestoreTarget(p.ManagedTargets()[0]); err != nil {
		t.Fatalf("RestoreTarget() error = %v", err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Errorf("destination still exists after restore, stat error = %v", err)
	}
}

func TestTransaction_RestoreTarget_MissingBackupFails(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".zshrc")
	backup := filepath.Join(home, ".dots-backups", "restore-test", "nope")

	mustWriteRestoreFixture(t, dest, []byte("installed"), 0o644)

	p := restorePlan(t, []plan.Target{{
		Destination: dest,
		Kind:        plan.CopyFile,
		PreState:    plan.PreState{Type: plan.StateFile, Mode: 0o644},
		BackupPath:  backup,
	}})

	err := New(p).RestoreTarget(p.ManagedTargets()[0])
	if err == nil {
		t.Fatal("expected error for missing backup")
	}
	if _, statErr := os.Lstat(dest); statErr != nil {
		t.Errorf("destination must not be touched on failed restore: %v", statErr)
	}
}

func TestTransaction_RestoreTarget_NoInventoryNoPanic(t *testing.T) {
	home := t.TempDir()
	dest := filepath.Join(home, ".zshrc")
	backup := filepath.Join(home, ".dots-backups", "restore-test", "2f2e7a73687263")

	mustWriteRestoreFixture(t, backup, []byte("original"), 0o600)
	mustWriteRestoreFixture(t, dest, []byte("installed"), 0o644)

	p := restorePlan(t, []plan.Target{{
		Destination: dest,
		Kind:        plan.CopyFile,
		PreState:    plan.PreState{Type: plan.StateFile, Mode: 0o600},
		BackupPath:  backup,
	}})

	// RestoreTarget must work without a preceding Prepare: the transaction
	// inventory is nil and the socket-preservation helpers must not panic.
	if err := New(p).RestoreTarget(p.ManagedTargets()[0]); err != nil {
		t.Fatalf("RestoreTarget() error = %v", err)
	}
	if got := readFileString(t, dest); got != "original" {
		t.Errorf("destination = %q, want original", got)
	}
}
