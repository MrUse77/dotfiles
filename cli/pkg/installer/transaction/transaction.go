package transaction

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// Transaction executes managed filesystem mutations with retained backups and
// automatic rollback. It never runs external commands.
type Transaction struct {
	plan      plan.InstallationPlan
	fs        Filesystem
	reader    plan.StateReader
	inventory *Inventory
	mutated   []plan.Target
}

// Option configures a Transaction.
type Option func(*Transaction)

// WithFilesystem injects a filesystem implementation.
func WithFilesystem(fs Filesystem) Option {
	return func(t *Transaction) { t.fs = fs }
}

// WithStateReader injects a pre-state reader.
func WithStateReader(r plan.StateReader) Option {
	return func(t *Transaction) { t.reader = r }
}

// New creates a Transaction for the given plan.
func New(p plan.InstallationPlan, opts ...Option) *Transaction {
	t := &Transaction{
		plan:   p,
		fs:     OSFilesystem(),
		reader: plan.DefaultStateReader(),
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Inventory returns the current inventory. It is nil before Prepare is called.
func (t *Transaction) Inventory() *Inventory { return t.inventory }

// Prepare allocates the inventory, creates backup roots, and checks for backup
// collisions without mutating any managed target.
func (t *Transaction) Prepare() error {
	inv := &Inventory{RunID: t.plan.RunID}
	t.inventory = inv

	for _, tgt := range t.plan.ManagedTargets() {
		entry := InventoryEntry{
			Target:     tgt,
			Original:   tgt.PreState,
			BackupPath: tgt.BackupPath,
			Status:     report.TargetPending,
		}
		if tgt.Kind == plan.Symlink {
			entry.LinkValue = boundSymlinkValue(t.fs, tgt.Source)
		}

		root := filepath.Dir(tgt.BackupPath)
		if err := t.ensureBackupRoot(root); err != nil {
			entry.Status = report.TargetFailed
			entry.Error = &report.BackupError{Target: tgt, Cause: fmt.Errorf("backup root: %w", err)}
			inv.Entries = append(inv.Entries, entry)
			if persistErr := persistInventory(t.fs, inv); persistErr != nil {
				return errors.Join(entry.Error, persistErr)
			}
			return entry.Error
		}

		exists, err := pathExists(t.fs, tgt.BackupPath)
		if err != nil {
			entry.Status = report.TargetFailed
			entry.Error = &report.BackupError{Target: tgt, Cause: err}
			inv.Entries = append(inv.Entries, entry)
			if persistErr := persistInventory(t.fs, inv); persistErr != nil {
				return errors.Join(entry.Error, persistErr)
			}
			return entry.Error
		}
		if exists {
			entry.Status = report.TargetFailed
			entry.Error = &report.BackupError{Target: tgt, Cause: fmt.Errorf("backup collision at %q", tgt.BackupPath)}
			inv.Entries = append(inv.Entries, entry)
			if persistErr := persistInventory(t.fs, inv); persistErr != nil {
				return errors.Join(entry.Error, persistErr)
			}
			return entry.Error
		}

		inv.Entries = append(inv.Entries, entry)
	}

	return persistInventory(t.fs, inv)
}

// Commit performs the bound mutations. It returns a non-nil error on the first
// failure; already-mutated targets are eligible for Rollback.
func (t *Transaction) Commit() error {
	if t.inventory == nil {
		if err := t.Prepare(); err != nil {
			return err
		}
	}
	for i := range t.inventory.Entries {
		if err := t.mutateTarget(&t.inventory.Entries[i]); err != nil {
			if persistErr := persistInventory(t.fs, t.inventory); persistErr != nil {
				return errors.Join(err, persistErr)
			}
			return err
		}
		if persistErr := persistInventory(t.fs, t.inventory); persistErr != nil {
			return persistErr
		}
	}
	return nil
}

// Rollback restores mutated targets in reverse order. It continues after
// individual failures and returns a RollbackError when any restoration failed.
// Backups are never deleted; the installer copies them back to the target path.
func (t *Transaction) Rollback() error {
	var failures []report.TargetOutcome

	for i := len(t.mutated) - 1; i >= 0; i-- {
		tgt := t.mutated[i]
		entry := t.inventoryEntry(tgt.Destination)

		if tgt.PreState.Type == plan.StateAbsent {
			if err := t.fs.RemoveAll(tgt.Destination); err != nil {
				failures = append(failures, report.TargetOutcome{
					Destination: tgt.Destination,
					Status:      report.TargetFailed,
					BackupPath:  tgt.BackupPath,
					Error:       err,
				})
				if entry != nil {
					entry.Status = report.TargetFailed
					entry.Error = err
				}
				continue
			}
		} else {
			if err := t.restoreFromBackup(tgt); err != nil {
				failures = append(failures, report.TargetOutcome{
					Destination: tgt.Destination,
					Status:      report.TargetFailed,
					BackupPath:  tgt.BackupPath,
					Error:       err,
				})
				if entry != nil {
					entry.Status = report.TargetFailed
					entry.Error = err
				}
				continue
			}
		}

		if entry != nil {
			entry.Status = report.TargetRestored
			entry.Error = nil
		}
	}

	if persistErr := persistInventory(t.fs, t.inventory); persistErr != nil {
		if len(failures) > 0 {
			return errors.Join(&report.RollbackError{Failures: failures}, persistErr)
		}
		return persistErr
	}

	if len(failures) > 0 {
		return &report.RollbackError{Failures: failures}
	}
	return nil
}

// Execute runs Prepare, Commit, and Rollback as a single recoverable operation.
// It returns the execution report and the primary error. If rollback itself
// fails, the returned error is a RollbackError with the original cause in the
// report.
func (t *Transaction) Execute() (*report.ExecutionReport, error) {
	if err := t.Prepare(); err != nil {
		return t.buildReport(err), err
	}
	if err := t.Commit(); err != nil {
		rbErr := t.Rollback()
		rpt := t.buildReport(err)
		if rbErr != nil {
			var rb *report.RollbackError
			if errors.As(rbErr, &rb) {
				rpt.RollbackFailures = rb.Failures
			}
			return rpt, rbErr
		}
		return rpt, err
	}

	return t.buildReport(nil), nil
}

func (t *Transaction) validateSourceDigest(tgt plan.Target) error {
	if tgt.SourceDigest == "" {
		return nil
	}
	actual, err := plan.SourceDigestForPath(resolvedSource(tgt))
	if err == nil && actual == tgt.SourceDigest {
		return nil
	}
	drift := &report.PlanDriftError{Target: tgt}
	if entry := t.inventoryEntry(tgt.Destination); entry != nil {
		entry.Status = report.TargetFailed
		entry.Error = drift
	}
	return drift
}

func (t *Transaction) ensureBackupRoot(root string) error {
	dotsBackups := filepath.Dir(root)
	if err := t.fs.Mkdir(dotsBackups, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	if err := t.fs.Mkdir(root, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	_ = t.fs.Chmod(dotsBackups, 0o700)
	_ = t.fs.Chmod(root, 0o700)
	return nil
}

func (t *Transaction) inventoryEntry(dest string) *InventoryEntry {
	for i := range t.inventory.Entries {
		if t.inventory.Entries[i].Target.Destination == dest {
			return &t.inventory.Entries[i]
		}
	}
	return nil
}

func (t *Transaction) mutateTarget(entry *InventoryEntry) error {
	tgt := entry.Target

	actual, err := t.reader.Read(tgt.Destination)
	if err != nil {
		entry.Status = report.TargetFailed
		entry.Error = &report.PlanDriftError{Target: tgt, Expected: tgt.PreState, Actual: plan.PreState{}}
		return entry.Error
	}
	if !preStatesEqual(tgt.PreState, actual) {
		entry.Status = report.TargetFailed
		entry.Error = &report.PlanDriftError{Target: tgt, Expected: tgt.PreState, Actual: actual}
		return entry.Error
	}

	root := filepath.Dir(tgt.BackupPath)
	if err := t.ensureBackupRoot(root); err != nil {
		entry.Status = report.TargetFailed
		entry.Error = &report.BackupError{Target: tgt, Cause: fmt.Errorf("backup root: %w", err)}
		return entry.Error
	}

	if tgt.PreState.Type != plan.StateAbsent {
		if err := t.backupTarget(tgt); err != nil {
			entry.Status = report.TargetFailed
			entry.Error = &report.BackupError{Target: tgt, Cause: err}
			return entry.Error
		}
		backupState, err := t.reader.Read(tgt.BackupPath)
		if err != nil || !preStatesEqual(tgt.PreState, backupState) {
			_ = t.fs.RemoveAll(tgt.BackupPath)
			cause := fmt.Errorf("backup validation failed")
			if err != nil {
				cause = fmt.Errorf("backup validation failed: %w", err)
			}
			entry.Status = report.TargetFailed
			entry.Error = &report.BackupError{Target: tgt, Cause: cause}
			return entry.Error
		}
	}
	if err := t.validateSourceDigest(tgt); err != nil {
		return err
	}

	parent := filepath.Dir(tgt.Destination)
	base := filepath.Base(tgt.Destination)

	var commitErr error
	switch tgt.Kind {
	case plan.CopyFile:
		commitErr = t.commitFile(tgt, parent, base)
	case plan.CopyTree:
		commitErr = t.commitTree(tgt, parent, base)
	case plan.Symlink:
		commitErr = t.commitSymlink(tgt, entry.LinkValue, parent, base)
	default:
		commitErr = fmt.Errorf("unsupported mutation kind %q", tgt.Kind)
	}

	if commitErr != nil {
		entry.Status = report.TargetFailed
		entry.Error = &report.MutationError{Target: tgt, Cause: commitErr}
		return entry.Error
	}

	entry.Status = report.TargetMutated
	return nil
}

func (t *Transaction) backupTarget(tgt plan.Target) error {
	switch tgt.PreState.Type {
	case plan.StateFile:
		return copyFile(t.fs, tgt.Destination, tgt.BackupPath, tgt.PreState.Mode)
	case plan.StateDirectory:
		if err := t.fs.Mkdir(tgt.BackupPath, tgt.PreState.Mode); err != nil {
			return fmt.Errorf("mkdir backup: %w", err)
		}
		if err := copyTree(t.fs, tgt.Destination, tgt.BackupPath); err != nil {
			_ = t.fs.RemoveAll(tgt.BackupPath)
			return fmt.Errorf("copy tree backup: %w", err)
		}
		return nil
	case plan.StateSymlink:
		link, err := t.fs.Readlink(tgt.Destination)
		if err != nil {
			return fmt.Errorf("readlink backup: %w", err)
		}
		if err := t.fs.Symlink(link, tgt.BackupPath); err != nil {
			return fmt.Errorf("symlink backup: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported pre-state %q", tgt.PreState.Type)
	}
}

func (t *Transaction) restoreFromBackup(tgt plan.Target) error {
	parent := filepath.Dir(tgt.Destination)
	base := filepath.Base(tgt.Destination)

	switch tgt.PreState.Type {
	case plan.StateFile:
		return t.restoreFile(tgt, parent, base)
	case plan.StateDirectory:
		return t.restoreDirectory(tgt, parent, base)
	case plan.StateSymlink:
		return t.restoreSymlink(tgt, parent, base)
	default:
		return fmt.Errorf("unsupported pre-state %q", tgt.PreState.Type)
	}
}

func (t *Transaction) restoreDirectory(tgt plan.Target, parent, base string) error {
	stageDir, err := t.fs.MkdirTemp(parent, "."+base+".dots-restore-*")
	if err != nil {
		return fmt.Errorf("stage restore directory: %w", err)
	}

	if err := copyTree(t.fs, tgt.BackupPath, stageDir); err != nil {
		_ = t.fs.RemoveAll(stageDir)
		return fmt.Errorf("copy tree restore: %w", err)
	}
	if err := t.fs.Chmod(stageDir, tgt.PreState.Mode); err != nil {
		_ = t.fs.RemoveAll(stageDir)
		return fmt.Errorf("chmod restore directory: %w", err)
	}

	trashPath := stageDir + ".dots-trash"
	if err := t.fs.Rename(tgt.Destination, trashPath); err != nil {
		_ = t.fs.RemoveAll(stageDir)
		return fmt.Errorf("relocate replacement directory: %w", err)
	}
	if err := t.fs.Rename(stageDir, tgt.Destination); err != nil {
		_ = t.fs.Rename(trashPath, tgt.Destination)
		_ = t.fs.RemoveAll(stageDir)
		return fmt.Errorf("commit restore directory: %w", err)
	}
	_ = t.fs.RemoveAll(trashPath)
	return nil
}

func (t *Transaction) restoreFile(tgt plan.Target, parent, base string) error {
	tmp, tmpName, err := t.fs.CreateTemp(parent, "."+base+".dots-restore-*")
	if err != nil {
		return fmt.Errorf("stage restore file: %w", err)
	}

	in, err := t.fs.Open(tgt.BackupPath)
	if err != nil {
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("open backup: %w", err)
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = in.Close()
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("copy backup: %w", err)
	}
	_ = in.Close()
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("sync restore file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("close restore file: %w", err)
	}
	if err := t.fs.Chmod(tmpName, tgt.PreState.Mode); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("chmod restore file: %w", err)
	}
	if err := t.fs.Rename(tmpName, tgt.Destination); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("commit restore file: %w", err)
	}
	return nil
}

func (t *Transaction) restoreSymlink(tgt plan.Target, parent, base string) error {
	link, err := t.fs.Readlink(tgt.BackupPath)
	if err != nil {
		return fmt.Errorf("readlink restore: %w", err)
	}

	tmpName, err := tempSibling(t.fs, parent, base)
	if err != nil {
		return fmt.Errorf("stage restore symlink: %w", err)
	}
	if err := t.fs.Symlink(link, tmpName); err != nil {
		return fmt.Errorf("create restore symlink: %w", err)
	}
	if err := t.fs.Rename(tmpName, tgt.Destination); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("commit restore symlink: %w", err)
	}
	return nil
}

func (t *Transaction) commitFile(tgt plan.Target, parent, base string) error {
	source := resolvedSource(tgt)
	info, err := t.fs.Lstat(source)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source %q is not a regular file", source)
	}

	tmp, tmpName, err := t.fs.CreateTemp(parent, "."+base+".dots-staging-*")
	if err != nil {
		return fmt.Errorf("stage file: %w", err)
	}

	in, err := t.fs.Open(source)
	if err != nil {
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("open source: %w", err)
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = in.Close()
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("copy source: %w", err)
	}
	_ = in.Close()
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("sync staging file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("close staging file: %w", err)
	}
	if err := t.fs.Chmod(tmpName, info.Mode().Perm()); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("chmod staging file: %w", err)
	}
	if err := t.fs.Rename(tmpName, tgt.Destination); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("commit file: %w", err)
	}

	t.mutated = append(t.mutated, tgt)
	return nil
}

func (t *Transaction) commitTree(tgt plan.Target, parent, base string) error {
	source := resolvedSource(tgt)
	info, err := t.fs.Lstat(source)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source %q is not a directory", source)
	}

	tmpDir, err := t.fs.MkdirTemp(parent, "."+base+".dots-staging-*")
	if err != nil {
		return fmt.Errorf("stage directory: %w", err)
	}

	if err := copyTree(t.fs, source, tmpDir); err != nil {
		_ = t.fs.RemoveAll(tmpDir)
		return fmt.Errorf("copy tree: %w", err)
	}
	if err := t.fs.Chmod(tmpDir, info.Mode().Perm()); err != nil {
		_ = t.fs.RemoveAll(tmpDir)
		return fmt.Errorf("chmod staging directory: %w", err)
	}

	if tgt.PreState.Type != plan.StateAbsent {
		trashPath := tmpDir + ".dots-trash"
		if err := t.fs.Rename(tgt.Destination, trashPath); err != nil {
			_ = t.fs.RemoveAll(tmpDir)
			return fmt.Errorf("relocate original directory: %w", err)
		}
		if err := t.fs.Rename(tmpDir, tgt.Destination); err != nil {
			_ = t.fs.Rename(trashPath, tgt.Destination)
			_ = t.fs.RemoveAll(tmpDir)
			return fmt.Errorf("commit directory: %w", err)
		}
		t.mutated = append(t.mutated, tgt)
		// Best-effort cleanup of the original tree. The retained backup remains.
		_ = t.fs.RemoveAll(trashPath)
	} else {
		if err := t.fs.Rename(tmpDir, tgt.Destination); err != nil {
			_ = t.fs.RemoveAll(tmpDir)
			return fmt.Errorf("commit directory: %w", err)
		}
		t.mutated = append(t.mutated, tgt)
	}
	return nil
}

func resolvedSource(tgt plan.Target) string {
	if tgt.ResolvedSource != "" {
		return tgt.ResolvedSource
	}
	return tgt.Source
}

func boundSymlinkValue(fs Filesystem, source string) string {
	if resolved, err := fs.Readlink(source); err == nil {
		return resolved
	}
	return source
}

func (t *Transaction) commitSymlink(tgt plan.Target, linkValue, parent, base string) error {
	tmpName, err := tempSibling(t.fs, parent, base)
	if err != nil {
		return fmt.Errorf("stage symlink: %w", err)
	}
	if err := t.fs.Symlink(linkValue, tmpName); err != nil {
		return fmt.Errorf("create staging symlink: %w", err)
	}
	if err := t.fs.Rename(tmpName, tgt.Destination); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("commit symlink: %w", err)
	}
	t.mutated = append(t.mutated, tgt)
	return nil
}

func tempSibling(fs Filesystem, parent, base string) (string, error) {
	b := make([]byte, 8)
	for i := 0; i < 10; i++ {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		name := fmt.Sprintf(".%s.dots-staging-%s", base, hex.EncodeToString(b))
		path := filepath.Join(parent, name)
		exists, err := pathExists(fs, path)
		if err != nil {
			return "", err
		}
		if !exists {
			return path, nil
		}
	}
	return "", errors.New("could not allocate unique staging name")
}

func preStatesEqual(a, b plan.PreState) bool {
	if a.Type != b.Type {
		return false
	}
	switch a.Type {
	case plan.StateAbsent:
		return true
	case plan.StateFile, plan.StateDirectory:
		return a.Mode == b.Mode && a.Digest == b.Digest
	case plan.StateSymlink:
		return a.Mode == b.Mode && a.LinkValue == b.LinkValue && a.Digest == b.Digest
	}
	return false
}

func (t *Transaction) buildReport(cause error) *report.ExecutionReport {
	rpt := &report.ExecutionReport{
		Fingerprint:  t.plan.Fingerprint,
		PrimaryCause: cause,
	}
	if t.inventory == nil {
		return rpt
	}
	for i := range t.inventory.Entries {
		e := &t.inventory.Entries[i]
		rpt.ManagedTargets = append(rpt.ManagedTargets, report.TargetOutcome{
			Destination: e.Target.Destination,
			Status:      e.Status,
			BackupPath:  e.BackupPath,
			Error:       e.Error,
		})
		if e.BackupPath != "" && e.Original.Type != plan.StateAbsent {
			rpt.BackupPaths = append(rpt.BackupPaths, e.BackupPath)
		}
	}
	return rpt
}
