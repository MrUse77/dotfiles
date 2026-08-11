//go:build linux

package release

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Lock is an exclusive kernel advisory lock on XDG_STATE_HOME/moonarch/lock.
// The kernel owns release semantics: closing the fd (or process exit) frees
// the flock with no userspace bookkeeping, so a crashed holder can never
// wedge future operations.
type Lock struct{}

// LockError wraps a failure to acquire the config-release lock. Contention is
// reported through ErrLockContended so callers can branch with errors.Is.
type LockError struct {
	Op    string
	Path  string
	Cause error
}

func (e *LockError) Error() string {
	return fmt.Sprintf("release lock %s %s: %v", e.Op, e.Path, e.Cause)
}
func (e *LockError) Unwrap() error { return e.Cause }

// Acquire takes an exclusive non-blocking flock on the file at path, creating
// its parent directory and the file if needed. On success it returns a release
// function that closes the fd (the kernel then drops the lock). If another
// process holds the lock the returned error satisfies errors.Is(err,
// ErrLockContended) and no fd is left open.
func (l *Lock) Acquire(path string) (release func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, &LockError{Op: "mkdir", Path: path, Cause: err}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, &LockError{Op: "open", Path: path, Cause: err}
	}
	for {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err != unix.EINTR {
			break
		}
	}
	if err != nil {
		_ = f.Close()
		if err == unix.EWOULDBLOCK || err == unix.EAGAIN {
			return nil, &LockError{Op: "acquire", Path: path, Cause: ErrLockContended}
		}
		return nil, &LockError{Op: "acquire", Path: path, Cause: err}
	}
	return func() { _ = f.Close() }, nil
}
