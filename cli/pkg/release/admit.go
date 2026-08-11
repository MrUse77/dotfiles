package release

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// ManifestFilename is the reserved manifest document name at the archive root.
// The artifact archive MUST begin with this entry; admission treats any other
// first entry as malformed.
const ManifestFilename = "manifest.json"

// artifactDigestRe matches the lowercase hex SHA-256 digest used to address
// staged archives and cache entries.
var artifactDigestRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// sha256Hex returns the lowercase hex SHA-256 of data.
func sha256SumHex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// Artifact rejection codes. Every rejection is an *ArtifactError that unwraps
// to ErrArtifactRejected.
const (
	ArtifactRejectInvalidDigest          = "invalid-digest"
	ArtifactRejectManifestInvalid        = "manifest-invalid"
	ArtifactRejectTraversal              = "traversal"
	ArtifactRejectAbsolutePath           = "absolute-path"
	ArtifactRejectDuplicatePath          = "duplicate-path"
	ArtifactRejectUnsupportedType        = "unsupported-type"
	ArtifactRejectSymlinkEscape          = "symlink-escape"
	ArtifactRejectUndeclaredEntry        = "undeclared-entry"
	ArtifactRejectMissingEntry           = "missing-entry"
	ArtifactRejectKindMismatch           = "kind-mismatch"
	ArtifactRejectDigestMismatch         = "digest-mismatch"
	ArtifactRejectUndeclaredExecutable   = "undeclared-executable"
	ArtifactRejectManifestOnlyExecutable = "manifest-only-executable"
	ArtifactRejectExtraction             = "extraction"
)

// ArtifactError classifies a fail-closed admission rejection. Code is one of
// the ArtifactReject* constants; Entry names the offending archive path when
// the rejection is path-specific. It always unwraps to ErrArtifactRejected.
type ArtifactError struct {
	Code  string
	Entry string
	Cause error
}

func (e *ArtifactError) Error() string {
	msg := fmt.Sprintf("config artifact rejected: %s", e.Code)
	if e.Entry != "" {
		msg += " (entry " + e.Entry + ")"
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *ArtifactError) Unwrap() error { return ErrArtifactRejected }

// Admitter performs fail-closed extraction and manifest verification of a
// sidecar-verified staged archive. It never promotes cache content.
type Admitter interface {
	// Admit verifies the staged archive at archivePath and extracts its
	// verified content to <dataRoot>/staging/<digest>/. Any unsafe path,
	// unsupported type, manifest inconsistency, or digest mismatch rejects
	// the whole artifact and leaves no partial extraction behind.
	Admit(archivePath, digest string) error
}

// ArtifactAdmitter is the production fail-closed extractor for .tar.zst
// configuration artifacts. It writes through a private temp directory and
// renames it into place only after every entry verified, so a rejected or
// interrupted admission never exposes partial content.
type ArtifactAdmitter struct {
	dataRoot string
}

// NewArtifactAdmitter creates an admitter rooted at dataRoot (the moonarch
// payload directory). Staging follows the resolver convention:
// <dataRoot>/staging/<digest>.tar.zst in, <dataRoot>/staging/<digest>/ out.
func NewArtifactAdmitter(dataRoot string) *ArtifactAdmitter {
	return &ArtifactAdmitter{dataRoot: dataRoot}
}

// Admit extracts and verifies the staged archive. Supported catalog kinds are
// "file", "dir", and "symlink"; symlink targets must resolve inside the
// extraction root. Executable entries are only accepted when the manifest
// binaries list names the exact path, and every declared binary must actually
// carry executable bits.
func (a *ArtifactAdmitter) Admit(archivePath, digest string) error {
	if !artifactDigestRe.MatchString(digest) {
		return &ArtifactError{Code: ArtifactRejectInvalidDigest, Cause: fmt.Errorf("invalid digest %q", digest)}
	}

	stagingRoot := filepath.Join(a.dataRoot, "staging")
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return &ArtifactError{Code: ArtifactRejectExtraction, Cause: fmt.Errorf("create staging: %w", err)}
	}
	tempExtract, err := os.MkdirTemp(stagingRoot, ".admit-"+digest+"-*")
	if err != nil {
		return &ArtifactError{Code: ArtifactRejectExtraction, Cause: fmt.Errorf("create temp extract dir: %w", err)}
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(tempExtract)
		}
	}()

	if err := a.extractAndVerify(archivePath, digest, tempExtract); err != nil {
		return err
	}

	final := filepath.Join(stagingRoot, digest)
	if err := os.RemoveAll(final); err != nil {
		return &ArtifactError{Code: ArtifactRejectExtraction, Cause: fmt.Errorf("clear stale staging dir: %w", err)}
	}
	if err := os.Rename(tempExtract, final); err != nil {
		return &ArtifactError{Code: ArtifactRejectExtraction, Cause: fmt.Errorf("publish extracted content: %w", err)}
	}
	committed = true
	return nil
}

// extractAndVerify streams the archive once: the first entry MUST be
// manifest.json; every following entry is validated against the catalog and
// extracted under tempExtract. The manifest's required digest, kind, and
// executable classification are verified per entry.
func (a *ArtifactAdmitter) extractAndVerify(archivePath, digest string, tempExtract string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return &ArtifactError{Code: ArtifactRejectExtraction, Cause: fmt.Errorf("open staged archive: %w", err)}
	}
	defer file.Close()

	zr, err := zstd.NewReader(file)
	if err != nil {
		return &ArtifactError{Code: ArtifactRejectExtraction, Cause: fmt.Errorf("open zstd stream: %w", err)}
	}
	defer zr.Close()

	var manifest Manifest
	var catalog map[string]CatalogEntry
	var binaries map[string]bool
	seen := make(map[string]struct{})
	first := true

	tr := tar.NewReader(zr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return &ArtifactError{Code: ArtifactRejectExtraction, Cause: fmt.Errorf("read archive (truncated or corrupt): %w", err)}
		}

		if first {
			first = false
			if path.Clean(hdr.Name) != ManifestFilename || hdr.Typeflag != tar.TypeReg {
				return &ArtifactError{Code: ArtifactRejectManifestInvalid, Entry: hdr.Name,
					Cause: fmt.Errorf("first archive entry must be %s", ManifestFilename)}
			}
			var mbytes bytes.Buffer
			if _, err := io.Copy(&mbytes, tr); err != nil {
				return &ArtifactError{Code: ArtifactRejectManifestInvalid, Cause: fmt.Errorf("read manifest: %w", err)}
			}
			manifest, err = ParseManifest(mbytes.Bytes())
			if err != nil {
				return &ArtifactError{Code: ArtifactRejectManifestInvalid, Cause: err}
			}
			// The manifest document is part of the verified artifact.
			if err := os.WriteFile(filepath.Join(tempExtract, ManifestFilename), mbytes.Bytes(), 0o644); err != nil {
				return extractionFailure(ManifestFilename, err)
			}
			catalog = make(map[string]CatalogEntry, len(manifest.Catalog))
			for _, e := range manifest.Catalog {
				catalog[path.Clean(e.Path)] = e
			}
			binaries = make(map[string]bool, len(manifest.Binaries))
			for _, b := range manifest.Binaries {
				binaries[path.Clean(b)] = true
			}
			continue
		}

		if err := a.admitEntry(tr, hdr, tempExtract, catalog, binaries, seen); err != nil {
			return err
		}
	}
	if first {
		return &ArtifactError{Code: ArtifactRejectManifestInvalid, Cause: fmt.Errorf("archive is empty: missing %s", ManifestFilename)}
	}
	for entryPath := range catalog {
		if _, ok := seen[entryPath]; !ok {
			return &ArtifactError{Code: ArtifactRejectMissingEntry, Entry: entryPath,
				Cause: fmt.Errorf("manifest catalog entry absent from archive")}
		}
	}
	return nil
}

// admitEntry validates and extracts one non-manifest archive entry.
func (a *ArtifactAdmitter) admitEntry(tr *tar.Reader, hdr *tar.Header, tempExtract string, catalog map[string]CatalogEntry, binaries map[string]bool, seen map[string]struct{}) error {
	name := hdr.Name
	clean := path.Clean(name)
	if clean == "" || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return &ArtifactError{Code: ArtifactRejectTraversal, Entry: name}
	}
	if path.IsAbs(clean) {
		return &ArtifactError{Code: ArtifactRejectAbsolutePath, Entry: name}
	}
	if _, dup := seen[clean]; dup {
		return &ArtifactError{Code: ArtifactRejectDuplicatePath, Entry: name}
	}
	seen[clean] = struct{}{}

	entry, ok := catalog[clean]
	if !ok {
		return &ArtifactError{Code: ArtifactRejectUndeclaredEntry, Entry: name,
			Cause: fmt.Errorf("archive entry missing from manifest catalog")}
	}
	if err := rejectWriteThroughSymlink(tempExtract, clean, name); err != nil {
		return err
	}

	extractPath := filepath.Join(tempExtract, filepath.FromSlash(clean))
	switch hdr.Typeflag {
	case tar.TypeDir:
		if entry.Kind != "dir" {
			return kindMismatch(name, entry.Kind, "dir")
		}
		if err := os.MkdirAll(extractPath, 0o755); err != nil {
			return extractionFailure(name, err)
		}
		if err := os.Chmod(extractPath, os.FileMode(hdr.Mode)&0o7777); err != nil {
			return extractionFailure(name, err)
		}
	case tar.TypeReg:
		if entry.Kind != "file" {
			return kindMismatch(name, entry.Kind, "file")
		}
		isExec := hdr.Mode&0o111 != 0
		if isExec && !binaries[clean] {
			return &ArtifactError{Code: ArtifactRejectUndeclaredExecutable, Entry: name,
				Cause: fmt.Errorf("executable entry not declared in manifest binaries")}
		}
		if binaries[clean] && !isExec {
			return &ArtifactError{Code: ArtifactRejectManifestOnlyExecutable, Entry: name,
				Cause: fmt.Errorf("manifest declares binary but archive entry lacks executable bits")}
		}
		if err := writeVerifiedFile(tr, extractPath, os.FileMode(hdr.Mode), entry.Digest, name); err != nil {
			return err
		}
	case tar.TypeSymlink:
		if entry.Kind != "symlink" {
			return kindMismatch(name, entry.Kind, "symlink")
		}
		target := hdr.Linkname
		if err := checkSymlinkTarget(clean, target, name); err != nil {
			return err
		}
		if computed := sha256SumHex([]byte(target)); computed != entry.Digest {
			return &ArtifactError{Code: ArtifactRejectDigestMismatch, Entry: name,
				Cause: fmt.Errorf("symlink digest: expected %s, computed %s", entry.Digest, computed)}
		}
		if err := os.MkdirAll(filepath.Dir(extractPath), 0o755); err != nil {
			return extractionFailure(name, err)
		}
		if err := os.Symlink(target, extractPath); err != nil {
			return extractionFailure(name, err)
		}
	default:
		return &ArtifactError{Code: ArtifactRejectUnsupportedType, Entry: name,
			Cause: fmt.Errorf("unsupported tar object type %q", hdr.Typeflag)}
	}
	return nil
}

// writeVerifiedFile streams the entry while hashing, then atomically renames
// the temp file into place only when the digest matches.
func writeVerifiedFile(tr *tar.Reader, extractPath string, mode os.FileMode, wantDigest, name string) error {
	if err := os.MkdirAll(filepath.Dir(extractPath), 0o755); err != nil {
		return extractionFailure(name, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(extractPath), ".entry-*")
	if err != nil {
		return extractionFailure(name, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), tr); err != nil {
		_ = tmp.Close()
		return extractionFailure(name, err)
	}
	if err := tmp.Close(); err != nil {
		return extractionFailure(name, err)
	}
	computed := hex.EncodeToString(h.Sum(nil))
	if computed != wantDigest {
		return &ArtifactError{Code: ArtifactRejectDigestMismatch, Entry: name,
			Cause: fmt.Errorf("digest: expected %s, computed %s", wantDigest, computed)}
	}
	if err := os.Chmod(tmpPath, mode&0o7777); err != nil {
		return extractionFailure(name, err)
	}
	if err := os.Rename(tmpPath, extractPath); err != nil {
		return extractionFailure(name, err)
	}
	return nil
}

// rejectWriteThroughSymlink refuses to create any entry under an extracted
// symlink, so archive content can never be written through a link.
func rejectWriteThroughSymlink(root, clean, name string) error {
	rel := strings.TrimPrefix(filepath.Clean(clean), string(filepath.Separator))
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) < 2 {
		return nil
	}
	cur := root
	for _, p := range parts[:len(parts)-1] {
		cur = filepath.Join(cur, p)
		fi, err := os.Lstat(cur)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return extractionFailure(name, err)
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			return &ArtifactError{Code: ArtifactRejectSymlinkEscape, Entry: name,
				Cause: fmt.Errorf("path component %s is a symlink", cur)}
		}
	}
	return nil
}

// checkSymlinkTarget rejects absolute targets and targets whose lexical
// resolution escapes the extraction root.
func checkSymlinkTarget(clean, target, name string) error {
	if target == "" || path.IsAbs(target) {
		return &ArtifactError{Code: ArtifactRejectSymlinkEscape, Entry: name,
			Cause: fmt.Errorf("symlink target %q is absolute or empty", target)}
	}
	resolved := path.Clean(path.Join(path.Dir(clean), target))
	if resolved == ".." || strings.HasPrefix(resolved, "../") {
		return &ArtifactError{Code: ArtifactRejectSymlinkEscape, Entry: name,
			Cause: fmt.Errorf("symlink target %q escapes artifact root", target)}
	}
	return nil
}

func kindMismatch(entry, want, got string) error {
	return &ArtifactError{Code: ArtifactRejectKindMismatch, Entry: entry,
		Cause: fmt.Errorf("kind mismatch: manifest says %q, archive carries %q", want, got)}
}

func extractionFailure(entry string, err error) error {
	return &ArtifactError{Code: ArtifactRejectExtraction, Entry: entry, Cause: err}
}
