package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeReleaseClient serves release metadata and asset bodies for resolver
// tests without any network access.
type fakeReleaseClient struct {
	release     Release
	getErr      error
	downloadErr error
	assets      map[string]string
	gotTag      string
	downloads   []string
}

func (f *fakeReleaseClient) Latest(context.Context) (Release, error) { return Release{}, nil }
func (f *fakeReleaseClient) GetByTag(_ context.Context, tag string) (Release, error) {
	f.gotTag = tag
	return f.release, f.getErr
}
func (f *fakeReleaseClient) Download(_ context.Context, asset Asset) (io.ReadCloser, error) {
	f.downloads = append(f.downloads, asset.Name)
	if f.downloadErr != nil {
		return nil, f.downloadErr
	}
	body, ok := f.assets[asset.Name]
	if !ok {
		return nil, &AssetNotFoundError{AssetName: asset.Name}
	}
	return io.NopCloser(strings.NewReader(body)), nil
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

const resolverTag = "config-v1.2.3"

func resolverRelease(payload, sidecar string) Release {
	return Release{Tag: resolverTag, Assets: []Asset{
		{Name: resolverTag + ".tar.zst", URL: "u://archive"},
		{Name: resolverTag + ".tar.zst.sha256", URL: "u://sidecar"},
	}}
}

func resolverAssets(payload, sidecar string) map[string]string {
	return map[string]string{
		resolverTag + ".tar.zst":        payload,
		resolverTag + ".tar.zst.sha256": sidecar,
	}
}

func TestResolver_Resolve_Success(t *testing.T) {
	payload := "archive-bytes"
	sidecar := sha256Hex(payload) + "  " + resolverTag + ".tar.zst\n"
	client := &fakeReleaseClient{
		release: resolverRelease(payload, sidecar),
		assets:  resolverAssets(payload, sidecar),
	}
	dataRoot := t.TempDir()
	resolver := NewArtifactResolver(client, &fakeFileOps{}, dataRoot)

	art, err := resolver.Resolve(context.Background(), resolverTag)
	if err != nil {
		t.Fatalf("Resolve error = %v", err)
	}
	if art.Tag != resolverTag {
		t.Fatalf("Tag = %q, want %q", art.Tag, resolverTag)
	}
	if art.Digest != sha256Hex(payload) {
		t.Fatalf("Digest = %q, want %q", art.Digest, sha256Hex(payload))
	}
	if client.gotTag != resolverTag {
		t.Fatalf("GetByTag called with %q, want exact %q", client.gotTag, resolverTag)
	}
	if want := filepath.Join(dataRoot, "staging", art.Digest+".tar.zst"); art.ArchivePath != want {
		t.Fatalf("ArchivePath = %q, want %q", art.ArchivePath, want)
	}
	staged, err := os.ReadFile(art.ArchivePath)
	if err != nil {
		t.Fatalf("read staged archive: %v", err)
	}
	if string(staged) != payload {
		t.Fatalf("staged content = %q, want %q", string(staged), payload)
	}
	if len(client.downloads) != 2 {
		t.Fatalf("downloads = %v, want archive and sidecar", client.downloads)
	}
}

func TestResolver_Resolve_SidecarMismatch(t *testing.T) {
	payload := "archive-bytes"
	wrong := sha256Hex("different-bytes")
	client := &fakeReleaseClient{
		release: resolverRelease(payload, wrong),
		assets:  resolverAssets(payload, wrong),
	}
	dataRoot := t.TempDir()
	resolver := NewArtifactResolver(client, &fakeFileOps{}, dataRoot)

	_, err := resolver.Resolve(context.Background(), resolverTag)
	if err == nil {
		t.Fatalf("expected sidecar mismatch error")
	}
	var me *SidecarMismatchError
	if !errors.As(err, &me) {
		t.Fatalf("error type = %T, want *SidecarMismatchError", err)
	}
	if me.Expected != wrong || me.Computed != sha256Hex(payload) {
		t.Fatalf("mismatch = expected %s computed %s, want expected %s computed %s", me.Expected, me.Computed, wrong, sha256Hex(payload))
	}
	entries, err := os.ReadDir(filepath.Join(dataRoot, "staging"))
	if err != nil {
		t.Fatalf("read staging dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("staging not cleaned after mismatch: %v", entries)
	}
}

func TestResolver_Resolve_MalformedSidecar(t *testing.T) {
	payload := "archive-bytes"
	client := &fakeReleaseClient{
		release: resolverRelease(payload, "not-a-sha256"),
		assets:  resolverAssets(payload, "not-a-sha256"),
	}
	resolver := NewArtifactResolver(client, &fakeFileOps{}, t.TempDir())

	_, err := resolver.Resolve(context.Background(), resolverTag)
	if err == nil {
		t.Fatalf("expected sidecar format error")
	}
	var se *SidecarFormatError
	if !errors.As(err, &se) {
		t.Fatalf("error type = %T, want *SidecarFormatError", err)
	}
}

func TestResolver_Resolve_MissingArchiveAsset(t *testing.T) {
	sidecar := sha256Hex("x")
	release := Release{Tag: resolverTag, Assets: []Asset{{Name: resolverTag + ".tar.zst.sha256", URL: "u"}}}
	client := &fakeReleaseClient{
		release: release,
		assets:  map[string]string{resolverTag + ".tar.zst.sha256": sidecar},
	}
	resolver := NewArtifactResolver(client, &fakeFileOps{}, t.TempDir())

	_, err := resolver.Resolve(context.Background(), resolverTag)
	if err == nil {
		t.Fatalf("expected missing archive asset error")
	}
	var ae *AssetNotFoundError
	if !errors.As(err, &ae) || ae.AssetName != resolverTag+".tar.zst" {
		t.Fatalf("error = %T %v, want *AssetNotFoundError for %s", err, err, resolverTag+".tar.zst")
	}
}

func TestResolver_Resolve_MissingSidecarAsset(t *testing.T) {
	payload := "archive-bytes"
	release := Release{Tag: resolverTag, Assets: []Asset{{Name: resolverTag + ".tar.zst", URL: "u"}}}
	client := &fakeReleaseClient{
		release: release,
		assets:  map[string]string{resolverTag + ".tar.zst": payload},
	}
	resolver := NewArtifactResolver(client, &fakeFileOps{}, t.TempDir())

	_, err := resolver.Resolve(context.Background(), resolverTag)
	if err == nil {
		t.Fatalf("expected missing sidecar asset error")
	}
	var ae *AssetNotFoundError
	if !errors.As(err, &ae) || ae.AssetName != resolverTag+".tar.zst.sha256" {
		t.Fatalf("error = %T %v, want *AssetNotFoundError for %s", err, err, resolverTag+".tar.zst.sha256")
	}
}

func TestResolver_Resolve_StagingFailure(t *testing.T) {
	payload := "archive-bytes"
	sidecar := sha256Hex(payload)
	client := &fakeReleaseClient{
		release: resolverRelease(payload, sidecar),
		assets:  resolverAssets(payload, sidecar),
	}
	files := &fakeFileOps{mkdirAllErr: errors.New("disk full")}
	resolver := NewArtifactResolver(client, files, t.TempDir())

	_, err := resolver.Resolve(context.Background(), resolverTag)
	if err == nil {
		t.Fatalf("expected staging error")
	}
	var se *StagingError
	if !errors.As(err, &se) {
		t.Fatalf("error type = %T, want *StagingError", err)
	}
}

func TestResolver_Resolve_DownloadFailure(t *testing.T) {
	payload := "archive-bytes"
	sidecar := sha256Hex(payload)
	client := &fakeReleaseClient{
		release:     resolverRelease(payload, sidecar),
		assets:      resolverAssets(payload, sidecar),
		downloadErr: errors.New("boom"),
	}
	resolver := NewArtifactResolver(client, &fakeFileOps{}, t.TempDir())

	_, err := resolver.Resolve(context.Background(), resolverTag)
	if err == nil {
		t.Fatalf("expected download failure")
	}
	if !errors.Is(err, client.downloadErr) {
		t.Fatalf("error = %v, want download error %v", err, client.downloadErr)
	}
}
