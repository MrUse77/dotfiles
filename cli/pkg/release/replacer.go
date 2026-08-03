package release

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FileOps abstracts the filesystem operations required for atomic replacement.
type FileOps interface {
	MkdirAll(path string, perm os.FileMode) error
	CreateTemp(dir, pattern string) (*os.File, error)
	Open(name string) (*os.File, error)
	Chmod(name string, mode os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
}

// OSFileOps is the production filesystem implementation.
type OSFileOps struct{}

func (OSFileOps) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (OSFileOps) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}
func (OSFileOps) Open(name string) (*os.File, error)        { return os.Open(name) }
func (OSFileOps) Chmod(name string, mode os.FileMode) error { return os.Chmod(name, mode) }
func (OSFileOps) Rename(oldpath, newpath string) error      { return os.Rename(oldpath, newpath) }
func (OSFileOps) Remove(name string) error                  { return os.Remove(name) }

// BinaryReplacer stages a verified binary and atomically replaces the target.
type BinaryReplacer interface {
	Replace(
		ctx context.Context,
		targetPath string,
		assetName string,
		binary io.Reader,
		checksumList io.Reader,
	) error
}

// AtomicReplacer stages the binary in the same directory as the target and
// performs a same-filesystem atomic rename after SHA-256 verification.
type AtomicReplacer struct {
	files    FileOps
	verifier ChecksumVerifier
}

// NewAtomicReplacer constructs a replacer with the supplied filesystem and
// verifier boundaries.
func NewAtomicReplacer(files FileOps, verifier ChecksumVerifier) *AtomicReplacer {
	return &AtomicReplacer{files: files, verifier: verifier}
}

// Replace copies binary into a temporary file in the target directory, verifies
// it against checksumList, applies executable permissions, and renames it over
// targetPath. Any failure removes the temp file and leaves the prior target
// untouched.
func (r *AtomicReplacer) Replace(ctx context.Context, targetPath, assetName string, binary io.Reader, checksumList io.Reader) error {
	targetDir := filepath.Dir(targetPath)
	if err := r.files.MkdirAll(targetDir, 0o755); err != nil {
		return &BinaryReplacementError{Cause: fmt.Errorf("create target directory: %w", err)}
	}

	tempFile, err := r.files.CreateTemp(targetDir, ".moonarch-cli-staging-*")
	if err != nil {
		return &BinaryReplacementError{Cause: fmt.Errorf("create temp file: %w", err)}
	}
	tempPath := tempFile.Name()

	cleanup := func() {
		_ = r.files.Remove(tempPath)
	}

	if err := r.copyBinary(ctx, tempFile, binary); err != nil {
		cleanup()
		return &BinaryReplacementError{Cause: fmt.Errorf("stage binary: %w", err)}
	}

	if err := r.verifyStagedBinary(ctx, tempPath, assetName, checksumList); err != nil {
		cleanup()
		return err
	}

	if err := r.files.Chmod(tempPath, 0o755); err != nil {
		cleanup()
		return &BinaryReplacementError{Cause: fmt.Errorf("chmod binary: %w", err)}
	}

	if err := r.files.Rename(tempPath, targetPath); err != nil {
		cleanup()
		return &BinaryReplacementError{Cause: fmt.Errorf("rename binary: %w", err)}
	}
	return nil
}

func (r *AtomicReplacer) copyBinary(ctx context.Context, out *os.File, in io.Reader) error {
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy binary: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync binary: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close binary: %w", err)
	}
	return ctx.Err()
}

func (r *AtomicReplacer) verifyStagedBinary(ctx context.Context, tempPath, assetName string, checksumList io.Reader) error {
	staged, err := r.files.Open(tempPath)
	if err != nil {
		return &BinaryReplacementError{Cause: fmt.Errorf("open staged binary: %w", err)}
	}
	defer staged.Close()
	if err := r.verifier.Verify(assetName, staged, checksumList); err != nil {
		return err
	}
	return ctx.Err()
}
