package release

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeStateFS wraps the real filesystem and injects rename failures so tests
// can prove WriteAtomic leaves the prior state intact and never leaks temp
// files when the atomic rename fails.
type fakeStateFS struct {
	renameErr error
}

func (f *fakeStateFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (f *fakeStateFS) CreateTemp(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}
func (f *fakeStateFS) Rename(oldpath, newpath string) error {
	if f.renameErr != nil {
		return f.renameErr
	}
	return os.Rename(oldpath, newpath)
}
func (f *fakeStateFS) Remove(name string) error { return os.Remove(name) }
func (f *fakeStateFS) SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// stateFixture returns a State with both identities populated so round-trip
// tests exercise every field, not just zero values.
func stateFixture() *State {
	return &State{
		Current:            &Identity{Tag: "config-v1.2.3", Digest: strings.Repeat("a", 64)},
		Previous:           &Identity{Tag: "config-v1.1.0", Digest: strings.Repeat("b", 64)},
		LastCompletedRunID: "run-abc123",
	}
}

func TestState_WriteAtomic_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "moonarch", "state.json")

	original := stateFixture()
	if err := original.WriteAtomic(path); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.Current == nil || got.Current.Tag != original.Current.Tag || got.Current.Digest != original.Current.Digest {
		t.Fatalf("current identity not round-tripped: got %+v want %+v", got.Current, original.Current)
	}
	if got.Previous == nil || got.Previous.Tag != original.Previous.Tag || got.Previous.Digest != original.Previous.Digest {
		t.Fatalf("previous identity not round-tripped: got %+v want %+v", got.Previous, original.Previous)
	}
	if got.LastCompletedRunID != original.LastCompletedRunID {
		t.Fatalf("LastCompletedRunID not round-tripped: got %q want %q", got.LastCompletedRunID, original.LastCompletedRunID)
	}
}

func TestState_WriteAtomic_NilIdentitiesRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	original := &State{LastCompletedRunID: "run-empty"}
	if err := original.WriteAtomic(path); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}

	got, err := ReadState(path)
	if err != nil {
		t.Fatalf("ReadState: %v", err)
	}
	if got.Current != nil || got.Previous != nil {
		t.Fatalf("expected nil identities, got current=%+v previous=%+v", got.Current, got.Previous)
	}
	if got.LastCompletedRunID != "run-empty" {
		t.Fatalf("LastCompletedRunID = %q, want %q", got.LastCompletedRunID, "run-empty")
	}
}

func TestState_WriteAtomic_JSONContract(t *testing.T) {
	// The persisted shape is part of the design contract: lowercase tag/digest
	// keys inside identities and a snake_case run id at the top level.
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := stateFixture().WriteAtomic(path); err != nil {
		t.Fatalf("WriteAtomic: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	for _, want := range []string{`"last_completed_run_id"`, `"current"`, `"previous"`, `"tag"`, `"digest"`, `config-v1.2.3`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("state.json missing %q; got:\n%s", want, data)
		}
	}
}

func TestState_WriteAtomic_FailurePreservesPriorState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	prior := []byte(`{"current":{"tag":"config-v0.9.0","digest":"` + strings.Repeat("c", 64) + `"},"last_completed_run_id":"run-old"}` + "\n")
	if err := os.WriteFile(path, prior, 0o644); err != nil {
		t.Fatalf("write prior state: %v", err)
	}

	s := &State{Current: &Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("d", 64)}, LastCompletedRunID: "run-new"}
	err := s.writeAtomic(path, &fakeStateFS{renameErr: errors.New("injected rename failure")})
	if err == nil {
		t.Fatal("expected WriteAtomic to fail on injected rename error")
	}
	var se *StateError
	if !errors.As(err, &se) {
		t.Fatalf("expected *StateError, got %T: %v", err, err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read prior state: %v", err)
	}
	if string(got) != string(prior) {
		t.Fatalf("prior state was clobbered:\n got %q\nwant %q", got, prior)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Fatalf("failed write leaked temp file %q", e.Name())
		}
	}
}

func TestState_ReadState_MissingFile(t *testing.T) {
	_, err := ReadState(filepath.Join(t.TempDir(), "nope", "state.json"))
	if err == nil {
		t.Fatal("expected error reading missing state file")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist to be reachable, got %v", err)
	}
}

func TestState_ReadState_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write malformed state: %v", err)
	}
	_, err := ReadState(path)
	if err == nil {
		t.Fatal("expected error decoding malformed state")
	}
	var se *StateError
	if !errors.As(err, &se) {
		t.Fatalf("expected *StateError, got %T: %v", err, err)
	}
}
