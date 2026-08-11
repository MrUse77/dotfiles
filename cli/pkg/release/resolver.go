package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Artifact is a verified configuration release archive staged for admission.
// Digest is the lowercase hex SHA-256 of the archive bytes, matched against
// the published sidecar before the artifact is exposed.
type Artifact struct {
	Tag         string // exact config-vMAJOR.MINOR.PATCH selector
	Digest      string // sha256 of the archive bytes
	ArchivePath string // staged <digest>.tar.zst on disk
}

// Resolver resolves an exact configuration release tag into a sidecar-verified,
// staged artifact. It never falls back to another release.
type Resolver interface {
	Resolve(ctx context.Context, tag string) (Artifact, error)
}

// ArtifactResolver downloads the <tag>.tar.zst archive and its published
// sidecar, verifies the whole-artifact digest, and stages the archive under
// <dataRoot>/staging/<digest>.tar.zst.
type ArtifactResolver struct {
	client   Client
	fs       FileOps
	dataRoot string
}

// NewArtifactResolver creates a resolver rooted at dataRoot, the moonarch
// payload directory (callers resolve XDG_DATA_HOME/moonarch themselves).
func NewArtifactResolver(client Client, fs FileOps, dataRoot string) *ArtifactResolver {
	return &ArtifactResolver{client: client, fs: fs, dataRoot: dataRoot}
}

// sidecarDigestRe matches a bare 64-hex digest or the GNU sha256sum form
// "DIGEST  filename".
var sidecarDigestRe = regexp.MustCompile(`^([0-9a-fA-F]{64})(?:[ \t]+.+)?$`)

// SidecarMismatchError signals that the published digest does not match the
// computed SHA-256 of the downloaded archive bytes.
type SidecarMismatchError struct {
	Tag      string
	Expected string
	Computed string
}

func (e *SidecarMismatchError) Error() string {
	return fmt.Sprintf("sidecar digest mismatch for %s: expected %s, computed %s", e.Tag, e.Expected, e.Computed)
}

// SidecarFormatError signals a malformed sidecar document.
type SidecarFormatError struct {
	Value string
}

func (e *SidecarFormatError) Error() string {
	return fmt.Sprintf("sidecar %q is not a sha256 digest", e.Value)
}

// StagingError wraps a failure to stage a verified archive for admission.
type StagingError struct {
	Cause error
}

func (e *StagingError) Error() string { return fmt.Sprintf("staging failed: %v", e.Cause) }
func (e *StagingError) Unwrap() error { return e.Cause }

// Resolve fetches the exact tag, downloads the archive and sidecar, verifies
// the whole-artifact SHA-256 against the sidecar, and stages the verified
// archive under staging/<digest>.tar.zst. Any failure removes the partial
// staging file and leaves no trace behind.
func (r *ArtifactResolver) Resolve(ctx context.Context, tag string) (Artifact, error) {
	rel, err := r.client.GetByTag(ctx, tag)
	if err != nil {
		return Artifact{}, err
	}
	archiveAsset, err := findAsset(rel.Assets, tag+".tar.zst")
	if err != nil {
		return Artifact{}, err
	}
	sidecarAsset, err := findAsset(rel.Assets, tag+".tar.zst.sha256")
	if err != nil {
		return Artifact{}, err
	}

	expected, err := r.readSidecar(ctx, sidecarAsset)
	if err != nil {
		return Artifact{}, err
	}

	stagingDir := filepath.Join(r.dataRoot, "staging")
	if err := r.fs.MkdirAll(stagingDir, 0o755); err != nil {
		return Artifact{}, &StagingError{Cause: fmt.Errorf("create staging dir: %w", err)}
	}
	tmp, err := r.fs.CreateTemp(stagingDir, ".resolve-*")
	if err != nil {
		return Artifact{}, &StagingError{Cause: fmt.Errorf("create staging temp: %w", err)}
	}
	tmpPath := tmp.Name()

	computed, err := r.copyAndHash(ctx, tmp, archiveAsset)
	if err != nil {
		_ = tmp.Close()
		_ = r.fs.Remove(tmpPath)
		return Artifact{}, err
	}
	if computed != expected {
		_ = tmp.Close()
		_ = r.fs.Remove(tmpPath)
		return Artifact{}, &SidecarMismatchError{Tag: tag, Expected: expected, Computed: computed}
	}
	if err := tmp.Close(); err != nil {
		_ = r.fs.Remove(tmpPath)
		return Artifact{}, &StagingError{Cause: fmt.Errorf("close staged archive: %w", err)}
	}

	final := filepath.Join(stagingDir, computed+".tar.zst")
	if err := r.fs.Rename(tmpPath, final); err != nil {
		_ = r.fs.Remove(tmpPath)
		return Artifact{}, &StagingError{Cause: fmt.Errorf("rename staged archive: %w", err)}
	}
	return Artifact{Tag: tag, Digest: computed, ArchivePath: final}, nil
}

// copyAndHash streams the asset into the staging file while computing its
// SHA-256 in a single pass.
func (r *ArtifactResolver) copyAndHash(ctx context.Context, out *os.File, asset Asset) (string, error) {
	in, err := r.client.Download(ctx, asset)
	if err != nil {
		return "", err
	}
	defer in.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(out, h), in); err != nil {
		return "", &StagingError{Cause: fmt.Errorf("download archive: %w", err)}
	}
	if err := out.Sync(); err != nil {
		return "", &StagingError{Cause: fmt.Errorf("sync staged archive: %w", err)}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// readSidecar downloads the sidecar asset and parses the expected digest.
func (r *ArtifactResolver) readSidecar(ctx context.Context, asset Asset) (string, error) {
	in, err := r.client.Download(ctx, asset)
	if err != nil {
		return "", err
	}
	defer in.Close()

	data, err := io.ReadAll(in)
	if err != nil {
		return "", &StagingError{Cause: fmt.Errorf("read sidecar: %w", err)}
	}
	value := strings.TrimSpace(string(data))
	m := sidecarDigestRe.FindStringSubmatch(value)
	if m == nil {
		return "", &SidecarFormatError{Value: value}
	}
	return strings.ToLower(m[1]), nil
}

// findAsset returns the single asset with the exact name, or a typed
// AssetNotFoundError when it is missing or duplicated.
func findAsset(assets []Asset, name string) (Asset, error) {
	var found *Asset
	for i := range assets {
		if assets[i].Name == name {
			if found != nil {
				return Asset{}, &AssetNotFoundError{AssetName: name}
			}
			found = &assets[i]
		}
	}
	if found == nil {
		return Asset{}, &AssetNotFoundError{AssetName: name}
	}
	return *found, nil
}
