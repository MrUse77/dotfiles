package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// State is the durable installed-identity record under
// XDG_STATE_HOME/moonarch/state.json. Current/Previous are the verified
// identities of the last two completed operations; LastCompletedRunID links
// the state back to the journal/inventory run that produced it.
type State struct {
	Current            *Identity `json:"current,omitempty"`
	Previous           *Identity `json:"previous,omitempty"`
	LastCompletedRunID string    `json:"last_completed_run_id"`
}

// stateFS is the small injectable filesystem boundary used by WriteAtomic so
// tests can force the atomic-rename step to fail.
type stateFS interface {
	MkdirAll(path string, perm os.FileMode) error
	CreateTemp(dir, pattern string) (*os.File, error)
	Rename(oldpath, newpath string) error
	Remove(name string) error
	SyncDir(dir string) error
}

type osStateFS struct{}

func (osStateFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osStateFS) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}
func (osStateFS) Rename(oldpath, newpath string) error { return os.Rename(oldpath, newpath) }
func (osStateFS) Remove(name string) error             { return os.Remove(name) }
func (osStateFS) SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// StateError wraps a failure to read or atomically write the state file.
type StateError struct {
	Op    string
	Cause error
}

func (e *StateError) Error() string { return fmt.Sprintf("state %s: %v", e.Op, e.Cause) }
func (e *StateError) Unwrap() error { return e.Cause }

// WriteAtomic persists the state via tmp file + fsync + rename, so a crash or
// failed write never exposes a partially written state.json. On any failure
// the previous state file is left untouched and the temp file is removed.
func (s *State) WriteAtomic(path string) error {
	return s.writeAtomic(path, osStateFS{})
}

func (s *State) writeAtomic(path string, fs stateFS) error {
	dir := filepath.Dir(path)
	if err := fs.MkdirAll(dir, 0o755); err != nil {
		return &StateError{Op: "mkdir", Cause: err}
	}
	tmp, err := fs.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return &StateError{Op: "create-tmp", Cause: err}
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = fs.Remove(tmpPath) }

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		_ = tmp.Close()
		cleanup()
		return &StateError{Op: "marshal", Cause: err}
	}
	data = append(data, '\n')

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return &StateError{Op: "write", Cause: err}
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return &StateError{Op: "fsync", Cause: err}
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return &StateError{Op: "close", Cause: err}
	}
	if err := fs.Rename(tmpPath, path); err != nil {
		cleanup()
		return &StateError{Op: "rename", Cause: err}
	}
	if err := fs.SyncDir(dir); err != nil {
		return &StateError{Op: "fsync-dir", Cause: err}
	}
	return nil
}

// ReadState loads the persisted state. A missing file returns the underlying
// not-exist error so callers can map it to the legacy/unknown identity state;
// malformed content returns a StateError.
func ReadState(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, &StateError{Op: "read", Cause: err}
	}
	return &s, nil
}
