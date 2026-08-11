package release

import (
	"fmt"
	"os"
	"path/filepath"
)

// Cache is the immutable digest-addressed artifact store under
// XDG_DATA_HOME/moonarch/artifacts/<digest>/. Only admitted artifacts may
// enter it, and promotion never exposes partial content.
type Cache interface {
	// Promote atomically renames the verified extracted staging directory
	// into artifacts/<digest>/. A digest already present is an idempotent
	// success (immutable identity). It never removes other cache entries.
	Promote(staging, digest string) error
	// Lookup returns the path to the admitted artifact root for digest, or a
	// CacheMissError wrapping ErrOfflineArtifactMissing when absent.
	Lookup(digest string) (string, error)
	// Retain removes cache entries not referenced by current or previous,
	// and never touches the protected digests (nil identities protect
	// nothing). Stray non-digest entries are left alone.
	Retain(current, previous *VersionIdentity) error
}

// cacheFS is the small injectable filesystem boundary used by ArtifactCache.
type cacheFS interface {
	MkdirAll(path string, perm os.FileMode) error
	Rename(oldpath, newpath string) error
	Remove(name string) error
	RemoveAll(path string) error
	Stat(name string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
}

type osCacheFS struct{}

func (osCacheFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osCacheFS) Rename(oldpath, newpath string) error         { return os.Rename(oldpath, newpath) }
func (osCacheFS) Remove(name string) error                     { return os.Remove(name) }
func (osCacheFS) RemoveAll(path string) error                  { return os.RemoveAll(path) }
func (osCacheFS) Stat(name string) (os.FileInfo, error)        { return os.Stat(name) }
func (osCacheFS) ReadDir(name string) ([]os.DirEntry, error)   { return os.ReadDir(name) }

// CacheError wraps a failure to mutate or query the artifact cache.
type CacheError struct {
	Op     string
	Digest string
	Cause  error
}

func (e *CacheError) Error() string {
	return fmt.Sprintf("cache %s (digest %s): %v", e.Op, e.Digest, e.Cause)
}
func (e *CacheError) Unwrap() error { return e.Cause }

// CacheMissError signals that the requested digest is not in the cache, which
// makes it unavailable for offline rollback.
type CacheMissError struct {
	Digest string
}

func (e *CacheMissError) Error() string { return fmt.Sprintf("artifact %s not in cache", e.Digest) }
func (e *CacheMissError) Unwrap() error { return ErrOfflineArtifactMissing }

// ArtifactCache is the production cache rooted at dataRoot.
type ArtifactCache struct {
	dataRoot string
	fs       cacheFS
}

// NewArtifactCache creates a cache rooted at dataRoot (the moonarch payload
// directory resolved from XDG_DATA_HOME).
func NewArtifactCache(dataRoot string) *ArtifactCache {
	return newArtifactCache(dataRoot, osCacheFS{})
}

func newArtifactCache(dataRoot string, fs cacheFS) *ArtifactCache {
	return &ArtifactCache{dataRoot: dataRoot, fs: fs}
}

// Promote atomically renames the extracted staging directory into
// artifacts/<digest>/. If the digest already exists the staged copy is
// discarded and success is returned: a digest identifies exactly one
// immutable content set.
func (c *ArtifactCache) Promote(staging, digest string) error {
	if !artifactDigestRe.MatchString(digest) {
		return &CacheError{Op: "promote", Digest: digest, Cause: fmt.Errorf("invalid digest")}
	}
	dest := filepath.Join(c.dataRoot, "artifacts", digest)
	if _, err := c.fs.Stat(dest); err == nil {
		_ = c.fs.RemoveAll(staging)
		return nil
	} else if !os.IsNotExist(err) {
		return &CacheError{Op: "promote", Digest: digest, Cause: err}
	}
	if err := c.fs.MkdirAll(filepath.Join(c.dataRoot, "artifacts"), 0o755); err != nil {
		return &CacheError{Op: "promote", Digest: digest, Cause: err}
	}
	if err := c.fs.Rename(staging, dest); err != nil {
		return &CacheError{Op: "promote", Digest: digest, Cause: err}
	}
	return nil
}

// Lookup returns the admitted artifact root path for digest.
func (c *ArtifactCache) Lookup(digest string) (string, error) {
	if !artifactDigestRe.MatchString(digest) {
		return "", &CacheError{Op: "lookup", Digest: digest, Cause: fmt.Errorf("invalid digest")}
	}
	p := filepath.Join(c.dataRoot, "artifacts", digest)
	if _, err := c.fs.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return "", &CacheMissError{Digest: digest}
		}
		return "", &CacheError{Op: "lookup", Digest: digest, Cause: err}
	}
	return p, nil
}

// Retain removes digest directories that neither current nor previous
// references. A nil identity protects nothing; non-digest entries in the
// artifacts directory are ignored.
func (c *ArtifactCache) Retain(current, previous *VersionIdentity) error {
	artifactsRoot := filepath.Join(c.dataRoot, "artifacts")
	entries, err := c.fs.ReadDir(artifactsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return &CacheError{Op: "retain", Digest: "", Cause: err}
	}
	protected := make(map[string]bool, 2)
	for _, id := range []*VersionIdentity{current, previous} {
		if id != nil && id.Digest != "" {
			protected[id.Digest] = true
		}
	}
	for _, e := range entries {
		if !e.IsDir() || !artifactDigestRe.MatchString(e.Name()) {
			continue
		}
		if protected[e.Name()] {
			continue
		}
		if err := c.fs.RemoveAll(filepath.Join(artifactsRoot, e.Name())); err != nil {
			return &CacheError{Op: "retain", Digest: e.Name(), Cause: err}
		}
	}
	return nil
}
