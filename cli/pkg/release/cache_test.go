package release

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// fakeCacheFS wraps the real filesystem and records renames so tests can
// inject an interrupted promotion and observe the atomic rename boundary.
type fakeCacheFS struct {
	renameErr error
	renames   []string
}

func (f *fakeCacheFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (f *fakeCacheFS) Rename(oldpath, newpath string) error {
	f.renames = append(f.renames, fmt.Sprintf("%s->%s", oldpath, newpath))
	if f.renameErr != nil {
		return f.renameErr
	}
	return os.Rename(oldpath, newpath)
}
func (f *fakeCacheFS) Remove(name string) error              { return os.Remove(name) }
func (f *fakeCacheFS) RemoveAll(path string) error           { return os.RemoveAll(path) }
func (f *fakeCacheFS) Stat(name string) (os.FileInfo, error) { return os.Stat(name) }
func (f *fakeCacheFS) ReadDir(name string) ([]os.DirEntry, error) {
	return os.ReadDir(name)
}

const cacheDigestA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const cacheDigestB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const cacheDigestOrphan = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

// cacheStagingDir plants an extracted artifact under dataRoot/staging/digest
// and returns its path.
func cacheStagingDir(t *testing.T, dataRoot, digest string) string {
	t.Helper()
	dir := filepath.Join(dataRoot, "staging", digest)
	if err := os.MkdirAll(filepath.Join(dir, "home"), 0o755); err != nil {
		t.Fatalf("mkdir staging: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "home", "conf"), []byte(digest), 0o644); err != nil {
		t.Fatalf("write staging file: %v", err)
	}
	return dir
}

// cacheArtifactDir plants an existing cache entry and returns its path.
func cacheArtifactDir(t *testing.T, dataRoot, digest string) string {
	t.Helper()
	dir := filepath.Join(dataRoot, "artifacts", digest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload"), []byte(digest), 0o644); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	return dir
}

func TestCache_Promote_MovesStagingAtomically(t *testing.T) {
	dataRoot := t.TempDir()
	staging := cacheStagingDir(t, dataRoot, cacheDigestA)

	cache := NewArtifactCache(dataRoot)
	if err := cache.Promote(staging, cacheDigestA); err != nil {
		t.Fatalf("Promote() error = %v", err)
	}
	dest := filepath.Join(dataRoot, "artifacts", cacheDigestA)
	if _, err := os.Stat(filepath.Join(dest, "home", "conf")); err != nil {
		t.Fatalf("promoted content missing: %v", err)
	}
	if _, err := os.Stat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging dir still present after promote")
	}
}

func TestCache_Promote_IsIdempotent(t *testing.T) {
	dataRoot := t.TempDir()
	staging := cacheStagingDir(t, dataRoot, cacheDigestA)

	cache := NewArtifactCache(dataRoot)
	if err := cache.Promote(staging, cacheDigestA); err != nil {
		t.Fatalf("first Promote() error = %v", err)
	}
	if err := cache.Promote(staging, cacheDigestA); err != nil {
		t.Fatalf("second Promote() error = %v, want idempotent success", err)
	}
	// The immutable entry must be the one from the first promotion.
	data, err := os.ReadFile(filepath.Join(dataRoot, "artifacts", cacheDigestA, "home", "conf"))
	if err != nil || string(data) != cacheDigestA {
		t.Fatalf("artifact content = %q err=%v, want %q", data, err, cacheDigestA)
	}
}

func TestCache_Promote_RejectsInvalidDigest(t *testing.T) {
	dataRoot := t.TempDir()
	staging := cacheStagingDir(t, dataRoot, cacheDigestA)

	cache := NewArtifactCache(dataRoot)
	err := cache.Promote(staging, "../../escape")
	if err == nil {
		t.Fatalf("Promote() error = nil for invalid digest")
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "artifacts", "escape")); !os.IsNotExist(err) {
		t.Fatalf("invalid digest escaped the artifacts root")
	}
	if _, err := os.Stat(staging); err != nil {
		t.Fatalf("staging dir was touched by rejected promote: %v", err)
	}
}

func TestCache_Promote_InterruptedLeavesEntriesIntact(t *testing.T) {
	dataRoot := t.TempDir()
	sentinel := cacheArtifactDir(t, dataRoot, cacheDigestB)
	staging := cacheStagingDir(t, dataRoot, cacheDigestA)

	files := &fakeCacheFS{renameErr: errors.New("interrupted: device busy")}
	cache := newArtifactCache(dataRoot, files)
	err := cache.Promote(staging, cacheDigestA)
	if err == nil {
		t.Fatalf("Promote() error = nil, want interrupted promotion failure")
	}
	if len(files.renames) == 0 {
		t.Fatalf("no atomic rename attempted")
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "artifacts", cacheDigestA)); !os.IsNotExist(err) {
		t.Fatalf("partial promotion exposed artifacts/<digest>")
	}
	// The previously admitted entry is untouched.
	if data, err := os.ReadFile(filepath.Join(sentinel, "payload")); err != nil || string(data) != cacheDigestB {
		t.Fatalf("protected artifact changed on interrupted promotion: %q err=%v", data, err)
	}
	// Staging content survives so the acquisition can be retried.
	if _, err := os.Stat(filepath.Join(staging, "home", "conf")); err != nil {
		t.Fatalf("staging lost on interrupted promotion: %v", err)
	}
}

func TestCache_Lookup_Found(t *testing.T) {
	dataRoot := t.TempDir()
	cacheArtifactDir(t, dataRoot, cacheDigestA)

	cache := NewArtifactCache(dataRoot)
	got, err := cache.Lookup(cacheDigestA)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if want := filepath.Join(dataRoot, "artifacts", cacheDigestA); got != want {
		t.Fatalf("Lookup() = %q, want %q", got, want)
	}
}

func TestCache_Lookup_MissingIsOfflineArtifactMiss(t *testing.T) {
	cache := NewArtifactCache(t.TempDir())
	_, err := cache.Lookup(cacheDigestA)
	if err == nil {
		t.Fatalf("Lookup() error = nil for missing artifact")
	}
	var miss *CacheMissError
	if !errors.As(err, &miss) {
		t.Fatalf("error type = %T, want *CacheMissError", err)
	}
	if !errors.Is(err, ErrOfflineArtifactMissing) {
		t.Fatalf("errors.Is(err, ErrOfflineArtifactMissing) = false")
	}
}

func TestCache_Lookup_RejectsInvalidDigest(t *testing.T) {
	cache := NewArtifactCache(t.TempDir())
	if _, err := cache.Lookup("../escape"); err == nil {
		t.Fatalf("Lookup() error = nil for invalid digest")
	}
}

func TestCache_Retain_ProtectsCurrentAndPrevious(t *testing.T) {
	dataRoot := t.TempDir()
	cacheArtifactDir(t, dataRoot, cacheDigestA)
	cacheArtifactDir(t, dataRoot, cacheDigestB)
	cacheArtifactDir(t, dataRoot, cacheDigestOrphan)

	cache := NewArtifactCache(dataRoot)
	current := &VersionIdentity{Tag: "config-v1.0.0", Digest: cacheDigestA}
	previous := &VersionIdentity{Tag: "config-v0.9.0", Digest: cacheDigestB}
	if err := cache.Retain(current, previous); err != nil {
		t.Fatalf("Retain() error = %v", err)
	}
	for _, kept := range []string{cacheDigestA, cacheDigestB} {
		if _, err := os.Stat(filepath.Join(dataRoot, "artifacts", kept)); err != nil {
			t.Fatalf("protected artifact %s removed by Retain: %v", kept, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "artifacts", cacheDigestOrphan)); !os.IsNotExist(err) {
		t.Fatalf("orphan artifact survived Retain")
	}
}

func TestCache_Retain_RemovesAllWhenNothingProtected(t *testing.T) {
	dataRoot := t.TempDir()
	cacheArtifactDir(t, dataRoot, cacheDigestA)
	cacheArtifactDir(t, dataRoot, cacheDigestB)

	cache := NewArtifactCache(dataRoot)
	if err := cache.Retain(nil, nil); err != nil {
		t.Fatalf("Retain(nil, nil) error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dataRoot, "artifacts"))
	if err != nil {
		t.Fatalf("read artifacts: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("Retain(nil, nil) left %d entries", len(entries))
	}
}

func TestCache_Retain_CurrentEqualsPreviousKeptOnce(t *testing.T) {
	dataRoot := t.TempDir()
	cacheArtifactDir(t, dataRoot, cacheDigestA)
	cacheArtifactDir(t, dataRoot, cacheDigestOrphan)

	cache := NewArtifactCache(dataRoot)
	current := &VersionIdentity{Tag: "config-v1.0.0", Digest: cacheDigestA}
	previous := &VersionIdentity{Tag: "config-v1.0.0", Digest: cacheDigestA}
	if err := cache.Retain(current, previous); err != nil {
		t.Fatalf("Retain() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "artifacts", cacheDigestA)); err != nil {
		t.Fatalf("current==previous artifact removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "artifacts", cacheDigestOrphan)); !os.IsNotExist(err) {
		t.Fatalf("orphan survived Retain with current==previous")
	}
}

func TestCache_Retain_NoArtifactsDirIsNoOp(t *testing.T) {
	cache := NewArtifactCache(t.TempDir())
	if err := cache.Retain(nil, nil); err != nil {
		t.Fatalf("Retain() on empty cache error = %v", err)
	}
}
