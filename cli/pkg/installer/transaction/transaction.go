package transaction

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

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
	inv := &Inventory{FormatVersion: InventoryFormatVersion, RunID: t.plan.RunID, Lifecycle: InventoryPrepared}
	t.inventory = inv

	seenBackups := make(map[string]plan.Target)
	for _, tgt := range t.plan.ManagedTargets() {
		if other, exists := seenBackups[tgt.BackupPath]; exists {
			return fmt.Errorf("backup collision at %q for destinations %q and %q", tgt.BackupPath, other.Destination, tgt.Destination)
		}
		seenBackups[tgt.BackupPath] = tgt
	}
	for _, tgt := range t.plan.ManagedTargets() {
		entry := InventoryEntry{
			Target:     tgt,
			Original:   tgt.PreState,
			BackupPath: tgt.BackupPath,
			Status:     report.TargetPending,
			State:      EntryPending,
		}
		if tgt.Kind == plan.Symlink {
			entry.LinkValue = boundSymlinkValue(t.fs, tgt.Source)
		}
		if tgt.Kind == plan.CopyTree {
			if err := t.fs.MkdirAll(filepath.Dir(tgt.Destination), 0o755); err != nil {
				return fmt.Errorf("create destination parent: %w", err)
			}
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
	t.inventory.Lifecycle = InventoryCommitting
	if err := persistInventory(t.fs, t.inventory); err != nil {
		return err
	}
	for i := range t.inventory.Entries {
		if err := t.mutateTarget(&t.inventory.Entries[i]); err != nil {
			t.inventory.Lifecycle = InventoryCommitFailed
			if persistErr := persistInventory(t.fs, t.inventory); persistErr != nil {
				return errors.Join(err, persistErr)
			}
			return err
		}
		if persistErr := persistInventory(t.fs, t.inventory); persistErr != nil {
			return persistErr
		}
	}
	t.inventory.Lifecycle = InventoryCompleted
	return persistInventory(t.fs, t.inventory)
}

// recordRollbackFailure records a per-target failure, updates the inventory entry,
// persists the inventory, and continues to the next target.
// persistFailures are collected separately so the caller can join them with
// the RollbackError.
func (t *Transaction) recordRollbackFailure(
	failures *[]report.TargetOutcome,
	persistFailures *[]error,
	tgt plan.Target,
	entry *InventoryEntry,
	state InventoryEntryState,
	cause error,
) {
	outcome := report.TargetOutcome{
		Destination: tgt.Destination,
		Status:      report.TargetFailed,
		BackupPath:  tgt.BackupPath,
		Error:       cause,
	}
	*failures = append(*failures, outcome)
	if entry != nil {
		entry.Status = report.TargetFailed
		entry.State = state
		entry.Error = cause
	}
	if err := persistInventory(t.fs, t.inventory); err != nil {
		*persistFailures = append(*persistFailures, err)
	}
}

// Rollback restores mutated targets in reverse order. It continues after
// individual failures and returns a RollbackError when any restoration failed.
// Backups are never deleted; the installer copies them back to the target path.
func (t *Transaction) Rollback() error {
	var failures []report.TargetOutcome
	var persistFailures []error

	t.inventory.Lifecycle = InventoryRollingBack
	if err := persistInventory(t.fs, t.inventory); err != nil {
		persistFailures = append(persistFailures, err)
	}

	for i := len(t.mutated) - 1; i >= 0; i-- {
		tgt := t.mutated[i]
		entry := t.inventoryEntry(tgt.Destination)

		if entry == nil || !t.ownsInstalledTarget(entry) {
			t.recordRollbackFailure(&failures, &persistFailures, tgt, entry, EntryOwnershipAmbiguous, errors.New("installed target ownership is ambiguous"))
			continue
		}

		if tgt.PreState.Type == plan.StateAbsent {
			if err := t.fs.RemoveAll(tgt.Destination); err != nil {
				t.recordRollbackFailure(&failures, &persistFailures, tgt, entry, EntryFailed, err)
				continue
			}
		} else {
			if err := t.restoreFromBackup(tgt); err != nil {
				t.recordRollbackFailure(&failures, &persistFailures, tgt, entry, EntryFailed, err)
				continue
			}
		}

		if entry != nil {
			entry.Status = report.TargetRestored
			entry.State = EntryRestored
			entry.Error = nil
		}
		if err := persistInventory(t.fs, t.inventory); err != nil {
			persistFailures = append(persistFailures, err)
		}
	}

	if len(failures) > 0 || len(persistFailures) > 0 {
		t.inventory.Lifecycle = InventoryRecoveryIncomplete
	} else {
		t.inventory.Lifecycle = InventoryRolledBack
	}
	if err := persistInventory(t.fs, t.inventory); err != nil {
		persistFailures = append(persistFailures, err)
	}

	var result error
	if len(failures) > 0 {
		result = &report.RollbackError{Failures: failures}
	}
	if len(persistFailures) > 0 {
		result = errors.Join(append([]error{result}, persistFailures...)...)
	}
	return result
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

func (t *Transaction) validateSourceDigest(tgt plan.Target) (*os.File, error) {
	if tgt.SourceDigest == "" {
		return nil, nil
	}
	binding := tgt.SourceBinding
	if binding.Digest == "" || binding.Digest != tgt.SourceDigest {
		return nil, t.planDrift(tgt)
	}
	if binding.PathIdentity != (plan.FileIdentity{}) {
		info, err := os.Lstat(tgt.Source)
		if err != nil || !matchesIdentity(info, binding.PathIdentity) {
			return nil, t.planDrift(tgt)
		}
	}
	if binding.LinkDigest != "" {
		link, err := t.fs.Readlink(tgt.Source)
		if err != nil || digestLink(link) != binding.LinkDigest {
			return nil, t.planDrift(tgt)
		}
	}
	source := resolvedSource(tgt)
	switch binding.Kind {
	case "file":
		file, err := openSourceFile(source)
		if err != nil {
			return nil, t.planDrift(tgt)
		}
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || !matchesIdentity(info, binding.Identity) || chmodMode(info.Mode()) != binding.Mode {
			_ = file.Close()
			return nil, t.planDrift(tgt)
		}
		digest, err := digestOpenFile(file)
		if err != nil || digest != binding.Digest {
			_ = file.Close()
			return nil, t.planDrift(tgt)
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			_ = file.Close()
			return nil, t.planDrift(tgt)
		}
		return file, nil

	case "directory":
		dir, err := openSourceDir(source)
		if err != nil {
			return nil, t.planDrift(tgt)
		}
		info, err := dir.Stat()
		if err != nil || !info.IsDir() || !matchesIdentity(info, binding.Identity) || chmodMode(info.Mode()) != binding.Mode {
			_ = dir.Close()
			return nil, t.planDrift(tgt)
		}
		if err := t.verifyTreeManifest(dir, "", tgt); err != nil {
			_ = dir.Close()
			return nil, t.planDrift(tgt)
		}
		// Rewind and consume the verified descriptor instead of reopening the
		// mutable source path after validation.
		if _, err := dir.Seek(0, io.SeekStart); err != nil {
			_ = dir.Close()
			return nil, t.planDrift(tgt)
		}
		return dir, nil

	case "symlink":
		link, err := t.fs.Readlink(tgt.Source)
		if err != nil || link != binding.LinkValue {
			return nil, t.planDrift(tgt)
		}
		return nil, nil
	}

	return nil, t.planDrift(tgt)
}

func (t *Transaction) planDrift(tgt plan.Target) error {
	drift := &report.PlanDriftError{Target: tgt}
	if entry := t.inventoryEntry(tgt.Destination); entry != nil {
		entry.Status = report.TargetFailed
		entry.State = EntrySourceDrift
		entry.Error = drift
	}
	return drift
}

func matchesIdentity(info os.FileInfo, want plan.FileIdentity) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return uint64(stat.Dev) == want.Device && uint64(stat.Ino) == want.Inode
}

func matchesIdentityFromStat(stat *unix.Stat_t, want plan.FileIdentity) bool {
	return uint64(stat.Dev) == want.Device && uint64(stat.Ino) == want.Inode
}

func identityFromStat(stat *unix.Stat_t) plan.FileIdentity {
	return plan.FileIdentity{Device: uint64(stat.Dev), Inode: uint64(stat.Ino)}
}

func digestOpenFile(file *os.File) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func digestLink(link string) string {
	sum := sha256.Sum256([]byte(link))
	return hex.EncodeToString(sum[:])
}

func openSourceFile(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func openSourceDir(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func (t *Transaction) ensureBackupRoot(root string) error {
	dir, err := openBackupRoot(root, true)
	if dir != nil {
		_ = dir.Close()
	}
	return err
}

// validateBackupRoot reopens the existing root without following any component.
// It is called before each backup and inventory write, not only during Prepare.
func validateBackupRoot(root string) error {
	dir, err := openBackupRoot(root, false)
	if dir != nil {
		_ = dir.Close()
	}
	return err
}

func openBackupRoot(root string, create bool) (*os.File, error) {
	root = filepath.Clean(root)
	if filepath.Base(filepath.Dir(root)) == ".dots-backups" {
		return openBackupRunRoot(root, create)
	}
	current := root
	var missing []string
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("unsafe backup root component %q: symlink", current)
			}
			if !info.IsDir() {
				return nil, fmt.Errorf("unsafe backup root component %q: not a directory", current)
			}
			break
		}
		if !errors.Is(err, os.ErrNotExist) || !create {
			return nil, fmt.Errorf("backup root component %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil, fmt.Errorf("backup root %q has no existing safe parent", root)
		}
		missing = append([]string{filepath.Base(current)}, missing...)
		current = parent
	}

	fd, err := unix.Open(current, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open backup root parent %q: %w", current, err)
	}
	if err := validateBackupDirectory(fd, current, false); err != nil {
		return nil, err
	}
	componentPath := current
	for _, name := range missing {
		componentPath = filepath.Join(componentPath, name)
		if err := unix.Mkdirat(fd, name, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, fmt.Errorf("create backup root component %q: %w", componentPath, err)
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return nil, fmt.Errorf("stat backup root component %q: %w", componentPath, err)
		}
		if err := validateBackupStat(&stat, componentPath, true); err != nil {
			return nil, err
		}
		next, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, fmt.Errorf("open backup root component %q: %w", componentPath, err)
		}
		_ = unix.Close(fd)
		fd = next
	}
	if err := validateBackupDirectory(fd, root, true); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), root), nil
}

// openBackupRunRoot anchors traversal at the target's home directory, then opens
// `.dots-backups` and the run ID exclusively through descriptor-relative calls.
func openBackupRunRoot(root string, create bool) (*os.File, error) {
	anchor := filepath.Dir(filepath.Dir(root))
	parts := []string{".dots-backups", filepath.Base(root)}
	fd, err := unix.Open(anchor, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open backup root parent %q: %w", anchor, err)
	}
	if err := validateBackupDirectory(fd, anchor, false); err != nil {
		return nil, err
	}
	componentPath := anchor
	for _, name := range parts {
		componentPath = filepath.Join(componentPath, name)
		var stat unix.Stat_t
		err := unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		if errors.Is(err, unix.ENOENT) && create {
			if err := unix.Mkdirat(fd, name, 0o700); err != nil && !errors.Is(err, unix.EEXIST) {
				return nil, fmt.Errorf("create backup root component %q: %w", componentPath, err)
			}
			err = unix.Fstatat(fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
		}
		if err != nil {
			return nil, fmt.Errorf("stat backup root component %q: %w", componentPath, err)
		}
		if err := validateBackupStat(&stat, componentPath, true); err != nil {
			return nil, err
		}
		next, err := unix.Openat(fd, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
		if err != nil {
			return nil, fmt.Errorf("open backup root component %q: %w", componentPath, err)
		}
		_ = unix.Close(fd)
		fd = next
	}
	return os.NewFile(uintptr(fd), root), nil
}

func validateBackupDirectory(fd int, component string, require0700 bool) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("stat backup root component %q: %w", component, err)
	}
	return validateBackupStat(&stat, component, require0700)
}

func validateBackupStat(stat *unix.Stat_t, component string, require0700 bool) error {
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("unsafe backup root component %q: not a directory", component)
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("unsafe backup root component %q: owner uid %d", component, stat.Uid)
	}
	perm := stat.Mode & 0o777
	if perm&0o022 != 0 {
		return fmt.Errorf("unsafe backup root component %q: group/world writable mode %#o", component, perm)
	}
	if require0700 && perm != 0o700 {
		return fmt.Errorf("unsafe backup root component %q: mode %#o, want 0700", component, perm)
	}
	return nil
}

func (t *Transaction) inventoryEntry(dest string) *InventoryEntry {
	if t.inventory == nil {
		return nil
	}
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

	boundSource, err := t.validateSourceDigest(tgt)
	if err != nil {
		return err
	}
	if boundSource != nil {
		defer boundSource.Close()
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
		// A durable checkpoint closes the crash window between retaining the
		// original and the first destination mutation.
		entry.State = EntryBackedUp
		if err := persistInventory(t.fs, t.inventory); err != nil {
			return err
		}
		if backupCheckpointPersisted != nil {
			backupCheckpointPersisted()
		}
	}
	parent := filepath.Dir(tgt.Destination)
	base := filepath.Base(tgt.Destination)

	var commitErr error
	linkValue := entry.LinkValue
	if tgt.Kind == plan.Symlink && tgt.SourceDigest != "" {
		// Planner-bound symlinks commit the value that validation just matched,
		// never the mutable value captured during Prepare.
		linkValue = tgt.SourceBinding.LinkValue
	}
	switch tgt.Kind {
	case plan.CopyFile:
		commitErr = t.commitFile(tgt, boundSource, parent, base)
	case plan.CopyTree:
		commitErr = t.commitTree(tgt, boundSource, parent, base)
	case plan.Symlink:
		commitErr = t.commitSymlink(tgt, linkValue, parent, base)
	default:
		commitErr = fmt.Errorf("unsupported mutation kind %q", tgt.Kind)
	}

	if commitErr != nil {
		entry.Status = report.TargetFailed
		if _, ok := commitErr.(*report.PlanDriftError); ok {
			entry.Error = commitErr
			return commitErr
		}
		entry.Error = &report.MutationError{Target: tgt, Cause: commitErr}
		return entry.Error
	}

	installed, err := t.reader.Read(tgt.Destination)
	if err != nil {
		return fmt.Errorf("read installed target identity: %w", err)
	}
	entry.InstalledDigest = installed.Digest
	entry.InstalledMode = installed.Mode
	if info, err := os.Lstat(tgt.Destination); err == nil {
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			entry.InstalledIdentity = fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
		}
	}
	entry.Status = report.TargetMutated
	entry.State = EntryMutated
	return nil
}

// backupBeforeCreate is a test seam for proving that a validated backup root is
// never reopened by path while creating a backup.
var backupBeforeCreate func()

// backupSync is a test seam for backup durability failures.
var backupSync = unix.Fsync

// backupCheckpointPersisted is a test seam for the crash window after backup
// durability and before the first destination mutation.
var backupCheckpointPersisted func()

func (t *Transaction) backupTarget(tgt plan.Target) error {
	root, err := openBackupRoot(filepath.Dir(tgt.BackupPath), false)
	if err != nil {
		return fmt.Errorf("open backup root: %w", err)
	}
	defer root.Close()
	if backupBeforeCreate != nil {
		backupBeforeCreate()
	}

	name := filepath.Base(tgt.BackupPath)
	switch tgt.PreState.Type {
	case plan.StateFile:
		if err := copyBackupFile(int(root.Fd()), name, tgt.Destination, tgt.PreState.Mode); err != nil {
			return err
		}
	case plan.StateDirectory:
		if err := copyBackupDirectory(int(root.Fd()), name, tgt.Destination, tgt.PreState.Mode); err != nil {
			return err
		}
	case plan.StateSymlink:
		link, err := os.Readlink(tgt.Destination)
		if err != nil {
			return fmt.Errorf("readlink backup: %w", err)
		}
		if err := unix.Symlinkat(link, int(root.Fd()), name); err != nil {
			return fmt.Errorf("symlink backup: %w", err)
		}
	default:
		return fmt.Errorf("unsupported pre-state %q", tgt.PreState.Type)
	}
	if err := backupSync(int(root.Fd())); err != nil {
		return fmt.Errorf("sync backup root: %w", err)
	}
	return nil
}

func copyBackupFile(rootFD int, name, source string, mode os.FileMode) error {
	in, err := unix.Open(source, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer unix.Close(in)
	return copyBackupFileAt(rootFD, name, in, mode)
}

func copyBackupFileAt(rootFD int, name string, sourceFD int, mode os.FileMode) error {
	out, err := unix.Openat(rootFD, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC, uint32(mode.Perm()))
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	output := os.NewFile(uintptr(out), name)
	defer output.Close()
	inputFD, err := unix.Dup(sourceFD)
	if err != nil {
		return fmt.Errorf("duplicate source descriptor: %w", err)
	}
	input := os.NewFile(uintptr(inputFD), "backup source")
	defer input.Close()
	if _, err := io.Copy(output, input); err != nil {
		return fmt.Errorf("copy content: %w", err)
	}
	if err := unix.Fchmod(out, unixMode(mode)); err != nil {
		return fmt.Errorf("chmod destination: %w", err)
	}
	if err := backupSync(out); err != nil {
		return fmt.Errorf("sync destination: %w", err)
	}
	return nil
}

func copyBackupDirectory(rootFD int, name, source string, mode os.FileMode) error {
	if err := unix.Mkdirat(rootFD, name, unixMode(mode)); err != nil {
		return fmt.Errorf("mkdir backup: %w", err)
	}
	src, err := openSourceDir(source)
	if err != nil {
		return fmt.Errorf("open source directory: %w", err)
	}
	defer src.Close()
	dstFD, err := unix.Openat(rootFD, name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open backup directory: %w", err)
	}
	defer unix.Close(dstFD)
	if err := copyBackupTree(int(src.Fd()), dstFD); err != nil {
		return err
	}
	if err := unix.Fchmod(dstFD, unixMode(mode)); err != nil {
		return fmt.Errorf("chmod backup directory: %w", err)
	}
	if err := backupSync(dstFD); err != nil {
		return fmt.Errorf("sync backup directory: %w", err)
	}
	return nil
}

var errRuntimeSocketRecoveryIncomplete = errors.New("runtime socket recovery incomplete")

// preserveRuntimeSockets moves unmanaged Unix sockets between directory trees.
// It validates every destination before moving anything and reverses completed
// moves if a later rename fails.
func preserveRuntimeSockets(fs Filesystem, from, to string) error {
	sockets, err := collectRuntimeSockets(fs, from, "")
	if err != nil {
		return err
	}
	for _, rel := range sockets {
		destination := filepath.Join(to, rel)
		exists, err := pathExists(fs, destination)
		if err != nil {
			return fmt.Errorf("check runtime socket destination %q: %w", rel, err)
		}
		if exists {
			return fmt.Errorf("runtime socket destination %q is occupied", rel)
		}
		if err := ensureRuntimeSocketParent(fs, to, filepath.Dir(rel)); err != nil {
			return fmt.Errorf("prepare runtime socket destination %q: %w", rel, err)
		}
	}

	moved := make([]string, 0, len(sockets))
	for _, rel := range sockets {
		if err := fs.Rename(filepath.Join(from, rel), filepath.Join(to, rel)); err != nil {
			var restoreErrs []error
			for i := len(moved) - 1; i >= 0; i-- {
				movedRel := moved[i]
				if restoreErr := fs.Rename(filepath.Join(to, movedRel), filepath.Join(from, movedRel)); restoreErr != nil {
					restoreErrs = append(restoreErrs, restoreErr)
				}
			}
			moveErr := fmt.Errorf("move runtime socket %q: %w; reverse moves: %v", rel, err, errors.Join(restoreErrs...))
			return errors.Join(moveErr, errRuntimeSocketRecoveryIncomplete)
		}
		moved = append(moved, rel)
	}
	return nil
}

func collectRuntimeSockets(fs Filesystem, root, prefix string) ([]string, error) {
	entries, err := fs.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read runtime tree %q: %w", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var sockets []string
	for _, entry := range entries {
		name := entry.Name()
		rel := name
		if prefix != "" {
			rel = filepath.Join(prefix, name)
		}
		path := filepath.Join(root, name)
		info, err := fs.Lstat(path)
		if err != nil {
			return nil, fmt.Errorf("stat runtime entry %q: %w", rel, err)
		}
		mode := info.Mode()
		switch {
		case mode&os.ModeSocket != 0:
			sockets = append(sockets, rel)
		case mode.IsDir():
			nested, err := collectRuntimeSockets(fs, path, rel)
			if err != nil {
				return nil, err
			}
			sockets = append(sockets, nested...)
		case mode.IsRegular(), mode&os.ModeSymlink != 0:
			// Managed files and links are not runtime sockets.
		default:
			return nil, fmt.Errorf("unsupported runtime entry %q: %v", rel, mode)
		}
	}
	return sockets, nil
}

func ensureRuntimeSocketParent(fs Filesystem, root, relativeParent string) error {
	if relativeParent == "." || relativeParent == "" {
		return nil
	}
	current := root
	for _, component := range strings.Split(filepath.Clean(relativeParent), string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := fs.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := fs.Mkdir(current, 0o755); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("parent %q is not a directory", current)
		}
	}
	return nil
}

func copyBackupTree(srcFD, dstFD int) error {
	readFD, err := unix.Dup(srcFD)
	if err != nil {
		return fmt.Errorf("duplicate source directory descriptor: %w", err)
	}
	src := os.NewFile(uintptr(readFD), "source directory")
	defer src.Close()
	entries, err := src.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read source directory: %w", err)
	}
	for _, entry := range entries {
		var stat unix.Stat_t
		if err := unix.Fstatat(srcFD, entry.Name(), &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("stat source entry %q: %w", entry.Name(), err)
		}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFREG:
			fileFD, err := unix.Openat(srcFD, entry.Name(), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return fmt.Errorf("open source entry %q: %w", entry.Name(), err)
			}
			err = copyBackupFileAt(dstFD, entry.Name(), fileFD, fileModeFromUnix(stat.Mode))
			unix.Close(fileFD)
			if err != nil {
				return err
			}
		case unix.S_IFDIR:
			if err := unix.Mkdirat(dstFD, entry.Name(), stat.Mode&0o777); err != nil {
				return fmt.Errorf("mkdir backup entry %q: %w", entry.Name(), err)
			}
			childSrc, err := unix.Openat(srcFD, entry.Name(), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
			if err != nil {
				return err
			}
			childDst, err := unix.Openat(dstFD, entry.Name(), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
			if err != nil {
				unix.Close(childSrc)
				return err
			}
			err = copyBackupTree(childSrc, childDst)
			if err == nil {
				err = unix.Fchmod(childDst, stat.Mode&(0o777|unix.S_ISUID|unix.S_ISGID|unix.S_ISVTX))
			}
			if err == nil {
				err = backupSync(childDst)
			}
			unix.Close(childSrc)
			unix.Close(childDst)
			if err != nil {
				return err
			}
		case unix.S_IFSOCK:
			// Runtime Unix sockets are unmanaged and intentionally omitted from
			// backups. Their live inode is preserved separately during replacement.
			continue
		case unix.S_IFLNK:
			buf := make([]byte, 4096)
			n, err := unix.Readlinkat(srcFD, entry.Name(), buf)
			if err != nil {
				return err
			}
			if err := unix.Symlinkat(string(buf[:n]), dstFD, entry.Name()); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported source entry %q", entry.Name())
		}
	}
	return nil
}

func fileModeFromUnix(mode uint32) os.FileMode {
	result := os.FileMode(mode & 0o777)
	if mode&unix.S_ISUID != 0 {
		result |= os.ModeSetuid
	}
	if mode&unix.S_ISGID != 0 {
		result |= os.ModeSetgid
	}
	if mode&unix.S_ISVTX != 0 {
		result |= os.ModeSticky
	}
	return result
}

func unixMode(mode os.FileMode) uint32 {
	result := uint32(mode.Perm())
	if mode&os.ModeSetuid != 0 {
		result |= unix.S_ISUID
	}
	if mode&os.ModeSetgid != 0 {
		result |= unix.S_ISGID
	}
	if mode&os.ModeSticky != 0 {
		result |= unix.S_ISVTX
	}
	return result
}

func (t *Transaction) ownsInstalledTarget(entry *InventoryEntry) bool {
	actual, err := t.reader.Read(entry.Target.Destination)
	if err != nil || actual.Digest != entry.InstalledDigest || actual.Mode != entry.InstalledMode {
		return false
	}
	info, err := os.Lstat(entry.Target.Destination)
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && entry.InstalledIdentity == fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
}

func (t *Transaction) retainRuntimeSocketRecovery(entry *InventoryEntry, stage, trash string) {
	if entry != nil {
		entry.StagePath, entry.TrashPath = stage, trash
	}
	t.inventory.Lifecycle = InventoryRecoveryIncomplete
}

func (t *Transaction) cleanupRuntimeSocketStage(err error, entry *InventoryEntry, stage, trash string, safe bool) bool {
	incomplete := errors.Is(err, errRuntimeSocketRecoveryIncomplete)
	if incomplete {
		t.retainRuntimeSocketRecovery(entry, stage, trash)
	} else if safe {
		_ = t.fs.RemoveAll(stage)
	}
	return incomplete
}

// RestoreTarget restores a single previously installed target to its
// pre-install state using the retained backup. Targets that did not exist
// before the install (StateAbsent) have their installed destination removed.
// The retained backup is never deleted.
func (t *Transaction) RestoreTarget(tgt plan.Target) error {
	if tgt.PreState.Type == plan.StateAbsent {
		exists, err := pathExists(t.fs, tgt.Destination)
		if err != nil {
			return fmt.Errorf("check destination: %w", err)
		}
		if exists {
			if err := t.fs.RemoveAll(tgt.Destination); err != nil {
				return fmt.Errorf("remove installed destination: %w", err)
			}
		}
		return nil
	}
	if tgt.BackupPath == "" {
		return fmt.Errorf("no backup path for %q", tgt.Destination)
	}
	if _, err := t.fs.Lstat(tgt.BackupPath); err != nil {
		return fmt.Errorf("backup %q is not readable: %w", tgt.BackupPath, err)
	}
	return t.restoreFromBackup(tgt)
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
	if err := preserveRuntimeSockets(t.fs, trashPath, stageDir); err != nil {
		restoreErr := t.fs.Rename(trashPath, tgt.Destination)
		t.cleanupRuntimeSocketStage(err, t.inventoryEntry(tgt.Destination), stageDir, "", restoreErr == nil)
		if restoreErr != nil {
			return fmt.Errorf("preserve runtime sockets: %w; restore replacement directory: %v", err, restoreErr)
		}
		return fmt.Errorf("preserve runtime sockets: %w", err)
	}
	if err := t.fs.Rename(stageDir, tgt.Destination); err != nil {
		restoreSocketsErr := preserveRuntimeSockets(t.fs, stageDir, trashPath)
		restoreErr := t.fs.Rename(trashPath, tgt.Destination)
		trash := ""
		if restoreErr != nil {
			trash = trashPath
		}
		t.cleanupRuntimeSocketStage(restoreSocketsErr, t.inventoryEntry(tgt.Destination), stageDir, trash, restoreErr == nil)
		if restoreSocketsErr != nil || restoreErr != nil {
			return fmt.Errorf("commit restore directory: %w; recover runtime sockets: %v; restore replacement directory: %v", err, restoreSocketsErr, restoreErr)
		}
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

func (t *Transaction) commitFile(tgt plan.Target, boundFile *os.File, parent, base string) error {
	source := resolvedSource(tgt)
	var in io.Reader
	var mode os.FileMode
	if boundFile != nil {
		info, err := boundFile.Stat()
		if err != nil {
			return fmt.Errorf("stat bound source: %w", err)
		}
		in, mode = boundFile, chmodMode(info.Mode())
	} else {
		info, err := t.fs.Lstat(source)
		if err != nil {
			return fmt.Errorf("stat source: %w", err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("source %q is not a regular file", source)
		}
		opened, err := t.fs.Open(source)
		if err != nil {
			return fmt.Errorf("open source: %w", err)
		}
		defer opened.Close()
		in, mode = opened, chmodMode(info.Mode())
	}

	tmp, tmpName, err := t.fs.CreateTemp(parent, "."+base+".dots-staging-*")
	if err != nil {
		return fmt.Errorf("stage file: %w", err)
	}
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("copy source: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("sync staging file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = t.fs.Remove(tmpName)
		return fmt.Errorf("close staging file: %w", err)
	}
	if err := t.fs.Chmod(tmpName, mode); err != nil {
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

func (t *Transaction) commitTree(tgt plan.Target, boundSource *os.File, parent, base string) error {
	source := resolvedSource(tgt)
	var info os.FileInfo
	var err error
	if boundSource != nil {
		info, err = boundSource.Stat()
	} else {
		info, err = t.fs.Lstat(source)
	}
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

	if boundSource != nil {
		if err := t.copyTreeBound(boundSource, "", tmpDir, tgt); err != nil {
			_ = t.fs.RemoveAll(tmpDir)
			if _, ok := err.(*report.PlanDriftError); ok {
				return err
			}
			return fmt.Errorf("copy tree: %w", err)
		}
	} else {
		if err := copyTree(t.fs, source, tmpDir); err != nil {
			_ = t.fs.RemoveAll(tmpDir)
			return fmt.Errorf("copy tree: %w", err)
		}
	}
	if err := t.fs.Chmod(tmpDir, chmodMode(info.Mode())); err != nil {
		_ = t.fs.RemoveAll(tmpDir)
		return fmt.Errorf("chmod staging directory: %w", err)
	}

	if tgt.PreState.Type != plan.StateAbsent {
		entry := t.inventoryEntry(tgt.Destination)
		trashPath := tmpDir + ".dots-trash"
		if entry != nil {
			entry.StagePath, entry.TrashPath, entry.State = tmpDir, trashPath, EntryStaged
			if err := persistInventory(t.fs, t.inventory); err != nil {
				return err
			}
		}
		if err := t.fs.Rename(tgt.Destination, trashPath); err != nil {
			return fmt.Errorf("relocate original directory: %w", err)
		}
		// The destination is now absent and its original is only recoverable at
		// TrashPath. Include it in rollback before the checkpoint can fail.
		t.mutated = append(t.mutated, tgt)
		if entry != nil {
			entry.State = EntryOriginalRelocated
			if err := persistInventory(t.fs, t.inventory); err != nil {
				return err
			}
		}
		if err := preserveRuntimeSockets(t.fs, trashPath, tmpDir); err != nil {
			restoreErr := t.fs.Rename(trashPath, tgt.Destination)
			if entry != nil {
				entry.State = EntryFailed
				entry.Error = errors.Join(err, restoreErr)
			}
			t.cleanupRuntimeSocketStage(err, entry, tmpDir, "", restoreErr == nil)
			if entry != nil {
				_ = persistInventory(t.fs, t.inventory)
			}
			if restoreErr != nil {
				t.inventory.Lifecycle = InventoryRecoveryIncomplete
				_ = persistInventory(t.fs, t.inventory)
				return fmt.Errorf("preserve runtime sockets: %w; restore original directory: %v", err, restoreErr)
			}
			return fmt.Errorf("preserve runtime sockets: %w", err)
		}
		if err := t.fs.Rename(tmpDir, tgt.Destination); err != nil {
			restoreSocketsErr := preserveRuntimeSockets(t.fs, tmpDir, trashPath)
			restoreErr := t.fs.Rename(trashPath, tgt.Destination)
			if entry != nil {
				entry.State = EntryFailed
				entry.Error = errors.Join(err, restoreSocketsErr, restoreErr)
			}
			trash := ""
			if restoreErr != nil {
				trash = trashPath
			}
			incomplete := t.cleanupRuntimeSocketStage(restoreSocketsErr, entry, tmpDir, trash, restoreErr == nil)
			if entry != nil && restoreErr == nil && !incomplete {
				entry.StagePath = ""
			}
			if restoreErr != nil {
				t.inventory.Lifecycle = InventoryRecoveryIncomplete
			}
			if entry != nil {
				_ = persistInventory(t.fs, t.inventory)
			}
			if restoreErr != nil || incomplete {
				return fmt.Errorf("commit directory: %w; recover runtime sockets: %v", err, restoreSocketsErr)
			}
			return fmt.Errorf("commit directory: %w", err)
		}
		if entry != nil {
			entry.State = EntryMutated
			if err := persistInventory(t.fs, t.inventory); err != nil {
				return err
			}
		}
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

// copyTreeBound copies the tree rooted at srcDir to dst while verifying each
// entry against the planner-built manifest. Source-side operations are
// descriptor-relative so a substituted symlink inside the source tree is never
// followed. A manifest mismatch is returned as a PlanDriftError for the target.
func (t *Transaction) copyTreeBound(srcDir *os.File, prefix, dst string, tgt plan.Target) error {
	manifest := tgt.SourceBinding.TreeManifest
	expected := make(map[string]plan.TreeManifestEntry, len(manifest))
	for _, e := range manifest {
		expected[e.RelativePath] = e
	}

	entries, err := srcDir.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	seen := make(map[string]struct{})
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}

		rel := name
		if prefix != "" {
			rel = path.Join(prefix, name)
		}
		seen[rel] = struct{}{}

		want, ok := expected[rel]
		if !ok {
			return &report.PlanDriftError{Target: tgt}
		}

		var stat unix.Stat_t
		if err := unix.Fstatat(int(srcDir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("stat %q: %w", rel, err)
		}
		mode := plan.FileModeFromUnix(stat.Mode)

		dstPath := filepath.Join(dst, name)
		switch want.Kind {
		case "file":
			if !mode.IsRegular() || !matchesIdentityFromStat(&stat, want.Identity) || chmodMode(mode) != want.Mode {
				return &report.PlanDriftError{Target: tgt}
			}

			fd, err := unix.Openat(int(srcDir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return fmt.Errorf("open %q: %w", rel, err)
			}
			f := os.NewFile(uintptr(fd), name)
			digest, err := digestOpenFile(f)
			if err != nil {
				_ = f.Close()
				return fmt.Errorf("digest %q: %w", rel, err)
			}
			if digest != want.Digest {
				_ = f.Close()
				return &report.PlanDriftError{Target: tgt}
			}
			if _, err := f.Seek(0, io.SeekStart); err != nil {
				_ = f.Close()
				return fmt.Errorf("seek %q: %w", rel, err)
			}

			out, outName, err := t.fs.CreateTemp(dst, "."+name+".dots-staging-*")
			if err != nil {
				_ = f.Close()
				return fmt.Errorf("stage %q: %w", rel, err)
			}
			if _, err := io.Copy(out, f); err != nil {
				_ = f.Close()
				_ = out.Close()
				_ = t.fs.Remove(outName)
				return fmt.Errorf("copy %q: %w", rel, err)
			}
			_ = f.Close()
			if err := out.Sync(); err != nil {
				_ = out.Close()
				_ = t.fs.Remove(outName)
				return fmt.Errorf("sync %q: %w", rel, err)
			}
			if err := out.Close(); err != nil {
				_ = t.fs.Remove(outName)
				return fmt.Errorf("close %q: %w", rel, err)
			}
			if err := t.fs.Chmod(outName, want.Mode); err != nil {
				_ = t.fs.Remove(outName)
				return fmt.Errorf("chmod %q: %w", rel, err)
			}
			if err := t.fs.Rename(outName, dstPath); err != nil {
				_ = t.fs.Remove(outName)
				return fmt.Errorf("commit %q: %w", rel, err)
			}

		case "directory":
			if !mode.IsDir() || !matchesIdentityFromStat(&stat, want.Identity) {
				return &report.PlanDriftError{Target: tgt}
			}

			if err := t.fs.Mkdir(dstPath, want.Mode); err != nil {
				return fmt.Errorf("mkdir %q: %w", rel, err)
			}
			fd, err := unix.Openat(int(srcDir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
			if err != nil {
				return fmt.Errorf("open directory %q: %w", rel, err)
			}
			sub := os.NewFile(uintptr(fd), name)
			if err := t.copyTreeBound(sub, rel, dstPath, tgt); err != nil {
				_ = sub.Close()
				return err
			}
			_ = sub.Close()
			if err := t.fs.Chmod(dstPath, want.Mode); err != nil {
				return fmt.Errorf("chmod directory %q: %w", rel, err)
			}

		case "symlink":
			if mode&os.ModeSymlink == 0 {
				return &report.PlanDriftError{Target: tgt}
			}
			buf := make([]byte, 4096)
			n, err := unix.Readlinkat(int(srcDir.Fd()), name, buf)
			if err != nil {
				return fmt.Errorf("readlink %q: %w", rel, err)
			}
			link := string(buf[:n])
			if link != want.LinkValue {
				return &report.PlanDriftError{Target: tgt}
			}
			if err := t.fs.Symlink(link, dstPath); err != nil {
				return fmt.Errorf("symlink %q: %w", rel, err)
			}

		default:
			return &report.PlanDriftError{Target: tgt}
		}
	}

	for _, want := range manifest {
		var isDirectChild bool
		if prefix == "" {
			isDirectChild = !strings.Contains(want.RelativePath, "/")
		} else {
			if strings.HasPrefix(want.RelativePath, prefix+"/") {
				rest := strings.TrimPrefix(want.RelativePath, prefix+"/")
				isDirectChild = !strings.Contains(rest, "/")
			}
		}
		if !isDirectChild {
			continue
		}
		if _, ok := seen[want.RelativePath]; !ok {
			return &report.PlanDriftError{Target: tgt}
		}
	}

	return nil
}

// verifyTreeManifest checks that the directory tree rooted at srcDir matches the
// planner-built manifest without copying anything. It is used before backup so
// that content drift inside a source directory is caught before any destination
// mutation.
func (t *Transaction) verifyTreeManifest(srcDir *os.File, prefix string, tgt plan.Target) error {
	manifest := tgt.SourceBinding.TreeManifest
	expected := make(map[string]plan.TreeManifestEntry, len(manifest))
	for _, e := range manifest {
		expected[e.RelativePath] = e
	}

	entries, err := srcDir.ReadDir(-1)
	if err != nil {
		return fmt.Errorf("read directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	seen := make(map[string]struct{})
	for _, e := range entries {
		name := e.Name()
		if name == "." || name == ".." {
			continue
		}

		rel := name
		if prefix != "" {
			rel = path.Join(prefix, name)
		}
		seen[rel] = struct{}{}

		want, ok := expected[rel]
		if !ok {
			return fmt.Errorf("unexpected entry %q", rel)
		}

		var stat unix.Stat_t
		if err := unix.Fstatat(int(srcDir.Fd()), name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("stat %q: %w", rel, err)
		}
		mode := plan.FileModeFromUnix(stat.Mode)

		switch want.Kind {
		case "file":
			if !mode.IsRegular() {
				return fmt.Errorf("entry %q type drift: got %v", rel, mode)
			}
			if !matchesIdentityFromStat(&stat, want.Identity) {
				return fmt.Errorf("entry %q identity drift: got %v want %v", rel, identityFromStat(&stat), want.Identity)
			}
			if chmodMode(mode) != want.Mode {
				return fmt.Errorf("entry %q mode drift: got %#o want %#o", rel, chmodMode(mode), want.Mode)
			}
			fd, err := unix.Openat(int(srcDir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			if err != nil {
				return fmt.Errorf("open %q: %w", rel, err)
			}
			f := os.NewFile(uintptr(fd), name)
			digest, err := digestOpenFile(f)
			_ = f.Close()
			if err != nil {
				return fmt.Errorf("digest %q: %w", rel, err)
			}
			if digest != want.Digest {
				return fmt.Errorf("entry %q digest drift", rel)
			}

		case "directory":
			if !mode.IsDir() {
				return fmt.Errorf("entry %q type drift: got %v", rel, mode)
			}
			if !matchesIdentityFromStat(&stat, want.Identity) {
				return fmt.Errorf("entry %q identity drift: got %v want %v", rel, identityFromStat(&stat), want.Identity)
			}
			fd, err := unix.Openat(int(srcDir.Fd()), name, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_DIRECTORY, 0)
			if err != nil {
				return fmt.Errorf("open directory %q: %w", rel, err)
			}
			sub := os.NewFile(uintptr(fd), name)
			if err := t.verifyTreeManifest(sub, rel, tgt); err != nil {
				_ = sub.Close()
				return err
			}
			_ = sub.Close()

		case "symlink":
			if mode&os.ModeSymlink == 0 {
				return fmt.Errorf("entry %q type drift: got %v", rel, mode)
			}
			buf := make([]byte, 4096)
			n, err := unix.Readlinkat(int(srcDir.Fd()), name, buf)
			if err != nil {
				return fmt.Errorf("readlink %q: %w", rel, err)
			}
			if string(buf[:n]) != want.LinkValue {
				return fmt.Errorf("entry %q symlink value drift", rel)
			}

		default:
			return fmt.Errorf("unsupported manifest kind %q for %q", want.Kind, rel)
		}
	}

	for _, want := range manifest {
		var isDirectChild bool
		if prefix == "" {
			isDirectChild = !strings.Contains(want.RelativePath, "/")
		} else {
			if strings.HasPrefix(want.RelativePath, prefix+"/") {
				rest := strings.TrimPrefix(want.RelativePath, prefix+"/")
				isDirectChild = !strings.Contains(rest, "/")
			}
		}
		if !isDirectChild {
			continue
		}
		if _, ok := seen[want.RelativePath]; !ok {
			return fmt.Errorf("missing entry %q", want.RelativePath)
		}
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
	if t.inventory.Path != "" {
		rpt.InventoryPath = t.inventory.Path
	}
	if t.inventory.Lifecycle == InventoryRecoveryIncomplete {
		rpt.RecoveryState = report.RecoveryManualRecoveryRequired
		rpt.RecoveryNextAction = report.ManualRecoveryNextAction
	} else if t.inventory.Lifecycle == InventoryRolledBack {
		rpt.RecoveryState = report.RecoveryComplete
	}
	for i := range t.inventory.Entries {
		e := &t.inventory.Entries[i]
		if e.State == EntryOwnershipAmbiguous || e.State == EntryFailed || e.StagePath != "" || e.TrashPath != "" {
			rpt.RecoveryArtifacts = append(rpt.RecoveryArtifacts, report.RecoveryArtifact{
				Destination: e.Target.Destination, BackupPath: e.BackupPath, StagePath: e.StagePath, TrashPath: e.TrashPath, InventoryPath: t.inventory.Path,
			})
		}
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
