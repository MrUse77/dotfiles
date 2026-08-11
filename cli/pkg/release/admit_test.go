package release

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
)

// admitEntry models one tar entry inside an admission fixture archive.
type admitEntry struct {
	name    string
	mode    int64
	kind    byte // tar.TypeReg | tar.TypeDir | tar.TypeSymlink | tar.TypeFifo
	content string
	link    string
}

// sha256HexDigest returns the lowercase hex SHA-256 of s.
func sha256HexDigest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// manifestForEntries derives a valid manifest whose catalog matches entries:
// digests are computed from content (symlink: link target; dir: marker string),
// kind is inferred from the tar type, and executable mirrors mode bits.
func manifestForEntries(binaries []string, entries []admitEntry) Manifest {
	catalog := make([]CatalogEntry, 0, len(entries))
	for _, e := range entries {
		kind := "file"
		digest := sha256HexDigest(e.content)
		switch e.kind {
		case tar.TypeDir:
			kind = "dir"
			digest = sha256HexDigest("dir:" + e.name)
		case tar.TypeSymlink:
			kind = "symlink"
			digest = sha256HexDigest(e.link)
		}
		catalog = append(catalog, CatalogEntry{
			Path:       e.name,
			Digest:     digest,
			Mode:       e.mode,
			Kind:       kind,
			Executable: e.mode&0o111 != 0,
		})
	}
	return Manifest{SchemaVersion: "1", CLICompatRange: ">= v0.3.0", Binaries: binaries, Catalog: catalog}
}

// buildAdmitArchive writes a deterministic .tar.zst fixture. When manifest is
// non-nil its JSON is written as the first entry under ManifestFilename. The
// archive path and its whole-file sha256 digest are returned.
func buildAdmitArchive(t *testing.T, dir string, manifest *Manifest, entries []admitEntry) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if manifest != nil {
		mjson, err := json.Marshal(manifest)
		if err != nil {
			t.Fatalf("marshal manifest: %v", err)
		}
		hdr := &tar.Header{Name: ManifestFilename, Mode: 0o644, Size: int64(len(mjson)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write manifest header: %v", err)
		}
		if _, err := tw.Write(mjson); err != nil {
			t.Fatalf("write manifest body: %v", err)
		}
	}
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: e.mode, Typeflag: e.kind}
		switch e.kind {
		case tar.TypeReg:
			hdr.Size = int64(len(e.content))
		case tar.TypeSymlink:
			hdr.Linkname = e.link
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %s: %v", e.name, err)
		}
		if e.kind == tar.TypeReg {
			if _, err := tw.Write([]byte(e.content)); err != nil {
				t.Fatalf("write body %s: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	archivePath := filepath.Join(dir, "fixture.tar.zst")
	out, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	zw, err := zstd.NewWriter(out)
	if err != nil {
		t.Fatalf("zstd writer: %v", err)
	}
	if _, err := zw.Write(buf.Bytes()); err != nil {
		t.Fatalf("zstd write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zstd close: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}

	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	return archivePath, sha256HexDigest(string(data))
}

// seedArtifactCache plants a sentinel digest dir in artifacts/ and returns its
// digest, so rejection tests can prove the cache is never mutated.
func seedArtifactCache(t *testing.T, dataRoot string) string {
	t.Helper()
	sentinel := strings.Repeat("a", 64)
	dir := filepath.Join(dataRoot, "artifacts", sentinel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seed cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("seed cache file: %v", err)
	}
	return sentinel
}

// assertAdmitRejected asserts the fail-closed contract: a typed ArtifactError
// wrapping ErrArtifactRejected with the expected code, a preserved artifact
// cache, no partial extraction dir, and an untouched staged archive.
func assertAdmitRejected(t *testing.T, dataRoot, archivePath, digest string, err error, wantCode string) {
	t.Helper()
	if err == nil {
		t.Fatalf("Admit() error = nil, want code %q", wantCode)
	}
	var ae *ArtifactError
	if !errors.As(err, &ae) {
		t.Fatalf("error type = %T, want *ArtifactError", err)
	}
	if ae.Code != wantCode {
		t.Fatalf("code = %q, want %q (err=%v)", ae.Code, wantCode, err)
	}
	if !errors.Is(err, ErrArtifactRejected) {
		t.Fatalf("errors.Is(err, ErrArtifactRejected) = false for %T", err)
	}
	entries, err := os.ReadDir(filepath.Join(dataRoot, "artifacts"))
	if err != nil {
		t.Fatalf("read artifacts: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != strings.Repeat("a", 64) {
		t.Fatalf("artifact cache changed on rejection: %v", entries)
	}
	if data, err := os.ReadFile(filepath.Join(dataRoot, "artifacts", strings.Repeat("a", 64), "sentinel.txt")); err != nil || string(data) != "keep" {
		t.Fatalf("sentinel cache entry altered on rejection: data=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "staging", digest)); !os.IsNotExist(err) {
		t.Fatalf("staging/<digest> exists after rejection (partial extraction leaked)")
	}
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("staged archive removed on rejection: %v", err)
	}
}

func TestAdmitArtifact_ValidArtifactIsAdmitted(t *testing.T) {
	entries := []admitEntry{
		{name: "home/.local/bin/moonarch", mode: 0o755, kind: tar.TypeReg, content: "#!/bin/sh\necho hi\n"},
		{name: "home/.config/hypr", mode: 0o755, kind: tar.TypeDir},
		{name: "home/.config/hypr/hyprland.conf", mode: 0o644, kind: tar.TypeReg, content: "monitor=,preferred,auto,1\n"},
		{name: "home/.zshrc", mode: 0o644, kind: tar.TypeReg, content: "export FOO=1\n"},
		{name: "home/theme-link", mode: 0o777, kind: tar.TypeSymlink, link: "themes/tokyonight"},
	}
	manifest := manifestForEntries([]string{"home/.local/bin/moonarch"}, entries)
	dataRoot := t.TempDir()
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	admitter := NewArtifactAdmitter(dataRoot)
	if err := admitter.Admit(archivePath, digest); err != nil {
		t.Fatalf("Admit() error = %v", err)
	}

	root := filepath.Join(dataRoot, "staging", digest)
	mdata, err := os.ReadFile(filepath.Join(root, ManifestFilename))
	if err != nil {
		t.Fatalf("read extracted manifest: %v", err)
	}
	var parsed Manifest
	if err := json.Unmarshal(mdata, &parsed); err != nil {
		t.Fatalf("extracted manifest not JSON: %v", err)
	}
	if parsed.SchemaVersion != "1" {
		t.Fatalf("extracted schema = %q, want 1", parsed.SchemaVersion)
	}

	binary, err := os.ReadFile(filepath.Join(root, "home/.local/bin/moonarch"))
	if err != nil {
		t.Fatalf("read extracted binary: %v", err)
	}
	if string(binary) != "#!/bin/sh\necho hi\n" {
		t.Fatalf("binary content = %q", string(binary))
	}
	if fi, err := os.Stat(filepath.Join(root, "home/.local/bin/moonarch")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("binary perm = %v err=%v, want 0755", fi.Mode().Perm(), err)
	}

	conf, err := os.ReadFile(filepath.Join(root, "home/.config/hypr/hyprland.conf"))
	if err != nil {
		t.Fatalf("read extracted config: %v", err)
	}
	if string(conf) != "monitor=,preferred,auto,1\n" {
		t.Fatalf("config content = %q", string(conf))
	}
	if fi, err := os.Stat(filepath.Join(root, "home/.config/hypr/hyprland.conf")); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("config perm = %v err=%v, want 0644", fi.Mode().Perm(), err)
	}

	if fi, err := os.Stat(filepath.Join(root, "home/.config/hypr")); err != nil || !fi.IsDir() {
		t.Fatalf("hypr dir missing: err=%v", err)
	}

	linkTarget, err := os.Readlink(filepath.Join(root, "home/theme-link"))
	if err != nil {
		t.Fatalf("read extracted symlink: %v", err)
	}
	if linkTarget != "themes/tokyonight" {
		t.Fatalf("symlink target = %q, want themes/tokyonight", linkTarget)
	}

	// Admission never promotes the cache: artifacts/ must not exist yet.
	if _, err := os.Stat(filepath.Join(dataRoot, "artifacts")); !os.IsNotExist(err) {
		t.Fatalf("admission promoted the cache (artifacts/ exists)")
	}
}

func TestAdmitArtifact_AcceptsDeclaredBinary(t *testing.T) {
	entries := []admitEntry{{name: "home/.local/bin/moonarch", mode: 0o755, kind: tar.TypeReg, content: "binary-bytes"}}
	manifest := manifestForEntries([]string{"home/.local/bin/moonarch"}, entries)
	dataRoot := t.TempDir()
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	admitter := NewArtifactAdmitter(dataRoot)
	if err := admitter.Admit(archivePath, digest); err != nil {
		t.Fatalf("Admit() error = %v, want accepted declared binary", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "staging", digest, "home/.local/bin/moonarch")); err != nil {
		t.Fatalf("declared binary not extracted: %v", err)
	}
}

func TestAdmitArtifact_RejectsUndeclaredExecutable(t *testing.T) {
	entries := []admitEntry{{name: "home/.local/bin/evil", mode: 0o755, kind: tar.TypeReg, content: "#!/bin/sh\nrm -rf /\n"}}
	manifest := manifestForEntries(nil, entries) // executable mode but NOT declared in binaries
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectUndeclaredExecutable)
}

func TestAdmitArtifact_RejectsManifestOnlyExecutable(t *testing.T) {
	entries := []admitEntry{{name: "home/.local/bin/moonarch", mode: 0o644, kind: tar.TypeReg, content: "not-executable"}}
	manifest := manifestForEntries([]string{"home/.local/bin/moonarch"}, entries) // declared binary, but no exec bits
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectManifestOnlyExecutable)
}

func TestAdmitArtifact_RejectsTraversalPath(t *testing.T) {
	entries := []admitEntry{{name: "../evil", mode: 0o644, kind: tar.TypeReg, content: "escape"}}
	manifest := manifestForEntries(nil, entries)
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectTraversal)
}

func TestAdmitArtifact_RejectsAbsolutePath(t *testing.T) {
	entries := []admitEntry{{name: "/etc/passwd", mode: 0o644, kind: tar.TypeReg, content: "root:x:0:0"}}
	manifest := manifestForEntries(nil, entries)
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectAbsolutePath)
}

func TestAdmitArtifact_RejectsSymlinkEscape(t *testing.T) {
	entries := []admitEntry{{name: "home/link", mode: 0o777, kind: tar.TypeSymlink, link: "../../etc/passwd"}}
	manifest := manifestForEntries(nil, entries)
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectSymlinkEscape)
}

func TestAdmitArtifact_RejectsDuplicateNormalizedPath(t *testing.T) {
	// "a/./b" and "a/b" both normalize to "a/b": the second must be rejected.
	manifest := Manifest{
		SchemaVersion: "1",
		Catalog: []CatalogEntry{
			{Path: "a/b", Digest: sha256HexDigest("first"), Mode: 0o644, Kind: "file"},
		},
	}
	entries := []admitEntry{
		{name: "a/./b", mode: 0o644, kind: tar.TypeReg, content: "first"},
		{name: "a/b", mode: 0o644, kind: tar.TypeReg, content: "second"},
	}
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectDuplicatePath)
}

func TestAdmitArtifact_RejectsUnsupportedFileType(t *testing.T) {
	entries := []admitEntry{{name: "home/pipe", mode: 0o644, kind: tar.TypeFifo}}
	manifest := manifestForEntries(nil, entries)
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectUnsupportedType)
}

func TestAdmitArtifact_RejectsUndeclaredArchiveEntry(t *testing.T) {
	entries := []admitEntry{{name: "home/extra.txt", mode: 0o644, kind: tar.TypeReg, content: "not-in-catalog"}}
	manifest := manifestForEntries(nil, nil) // empty catalog: every entry is undeclared
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectUndeclaredEntry)
}

func TestAdmitArtifact_RejectsMissingManifestEntry(t *testing.T) {
	manifest := Manifest{
		SchemaVersion: "1",
		Catalog: []CatalogEntry{
			{Path: "home/missing-file", Digest: sha256HexDigest("x"), Mode: 0o644, Kind: "file"},
		},
	}
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, nil)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectMissingEntry)
}

func TestAdmitArtifact_RejectsDigestMismatch(t *testing.T) {
	entries := []admitEntry{{name: "home/.zshrc", mode: 0o644, kind: tar.TypeReg, content: "real-content"}}
	manifest := manifestForEntries(nil, entries)
	manifest.Catalog[0].Digest = sha256HexDigest("different-content") // declared digest does not match bytes
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectDigestMismatch)
}

func TestAdmitArtifact_RejectsKindMismatch(t *testing.T) {
	// Archive carries a regular file, but the manifest catalog classifies it as
	// a directory.
	manifest := Manifest{
		SchemaVersion: "1",
		Catalog: []CatalogEntry{
			{Path: "home/thing", Digest: sha256HexDigest("x"), Mode: 0o755, Kind: "dir"},
		},
	}
	entries := []admitEntry{{name: "home/thing", mode: 0o644, kind: tar.TypeReg, content: "x"}}
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectKindMismatch)
}

func TestAdmitArtifact_RejectsMissingManifest(t *testing.T) {
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	entries := []admitEntry{{name: "home/x", mode: 0o644, kind: tar.TypeReg, content: "x"}}
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), nil, entries) // no manifest.json first

	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, digest)
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectManifestInvalid)
}

func TestAdmitArtifact_RejectsInvalidDigestArgument(t *testing.T) {
	entries := []admitEntry{{name: "home/x", mode: 0o644, kind: tar.TypeReg, content: "x"}}
	manifest := manifestForEntries(nil, entries)
	dataRoot := t.TempDir()
	seedArtifactCache(t, dataRoot)
	archivePath, digest := buildAdmitArchive(t, t.TempDir(), &manifest, entries)

	// A non-hex digest must never be accepted as a staging name.
	err := NewArtifactAdmitter(dataRoot).Admit(archivePath, "../../../etc/passwd")
	assertAdmitRejected(t, dataRoot, archivePath, digest, err, ArtifactRejectInvalidDigest)
}
