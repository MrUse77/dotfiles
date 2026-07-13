package transaction

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// File is the minimal abstraction over an open file.
type File interface {
	io.ReadWriteCloser
	Sync() error
}

// Filesystem abstracts filesystem operations so tests can inject failures and
// spy on calls without touching a real home directory.
type Filesystem interface {
	Lstat(path string) (os.FileInfo, error)
	Mkdir(path string, perm os.FileMode) error
	MkdirAll(path string, perm os.FileMode) error
	MkdirTemp(dir, pattern string) (string, error)
	CreateTemp(dir, pattern string) (File, string, error)
	Remove(path string) error
	RemoveAll(path string) error
	Rename(oldpath, newpath string) error
	Open(path string) (File, error)
	Create(path string) (File, error)
	Readlink(name string) (string, error)
	Symlink(oldname, newname string) error
	Chmod(name string, mode os.FileMode) error
	ReadDir(path string) ([]os.DirEntry, error)
}

// OSFilesystem returns a Filesystem backed by the real os package.
func OSFilesystem() Filesystem { return osFS{} }

type osFS struct{}

func (osFS) Lstat(path string) (os.FileInfo, error)        { return os.Lstat(path) }
func (osFS) Mkdir(path string, perm os.FileMode) error     { return os.Mkdir(path, perm) }
func (osFS) MkdirAll(path string, perm os.FileMode) error  { return os.MkdirAll(path, perm) }
func (osFS) Remove(path string) error                      { return os.Remove(path) }
func (osFS) RemoveAll(path string) error                   { return os.RemoveAll(path) }
func (osFS) Rename(oldpath, newpath string) error          { return os.Rename(oldpath, newpath) }
func (osFS) Open(path string) (File, error)                { return os.Open(path) }
func (osFS) Create(path string) (File, error)              { return os.Create(path) }
func (osFS) Readlink(name string) (string, error)          { return os.Readlink(name) }
func (osFS) Symlink(oldname, newname string) error         { return os.Symlink(oldname, newname) }
func (osFS) Chmod(name string, mode os.FileMode) error     { return os.Chmod(name, mode) }
func (osFS) ReadDir(path string) ([]os.DirEntry, error)    { return os.ReadDir(path) }
func (osFS) MkdirTemp(dir, pattern string) (string, error) { return os.MkdirTemp(dir, pattern) }
func (osFS) CreateTemp(dir, pattern string) (File, string, error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, "", err
	}
	return f, f.Name(), nil
}

// copyFile copies src to dst using the provided filesystem, preserving mode.
// It refuses to overwrite an existing destination.
func copyFile(fs Filesystem, src, dst string, mode os.FileMode) error {
	in, err := fs.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	exists, err := pathExists(fs, dst)
	if err != nil {
		return fmt.Errorf("check destination: %w", err)
	}
	if exists {
		return fmt.Errorf("create destination: file exists")
	}

	out, err := fs.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy content: %w", err)
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return fmt.Errorf("sync destination: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close destination: %w", err)
	}
	if err := fs.Chmod(dst, mode); err != nil {
		return fmt.Errorf("chmod destination: %w", err)
	}
	return nil
}

// copyTree recursively copies src to dst, preserving modes and symlink values.
func copyTree(fs Filesystem, src, dst string) error {
	entries, err := fs.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read dir %q: %w", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("stat %q: %w", srcPath, err)
		}
		mode := info.Mode()

		switch {
		case mode.IsRegular():
			if err := copyFile(fs, srcPath, dstPath, mode.Perm()); err != nil {
				return err
			}
		case mode.IsDir():
			if err := fs.Mkdir(dstPath, mode.Perm()); err != nil {
				return fmt.Errorf("mkdir %q: %w", dstPath, err)
			}
			if err := copyTree(fs, srcPath, dstPath); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			link, err := fs.Readlink(srcPath)
			if err != nil {
				return fmt.Errorf("readlink %q: %w", srcPath, err)
			}
			if err := fs.Symlink(link, dstPath); err != nil {
				return fmt.Errorf("symlink %q: %w", dstPath, err)
			}
		default:
			return fmt.Errorf("unsupported entry %q: %v", srcPath, mode)
		}
	}
	return nil
}

// pathExists reports whether path exists on fs.
func pathExists(fs Filesystem, path string) (bool, error) {
	_, err := fs.Lstat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}
