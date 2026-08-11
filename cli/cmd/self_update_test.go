package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

// fakeReleaseClient records calls and returns configured release metadata and
// downloadable assets. The SHA256SUMS.txt stream is generated from the binary
// bytes so the production SHA256Verifier can validate real replacements.
type fakeReleaseClient struct {
	latestCalls     int
	downloads       int
	latestTag       string
	latestErr       error
	binary          []byte
	corruptChecksum bool
	downloadErr     map[string]error
}

func (f *fakeReleaseClient) Latest(context.Context) (release.Release, error) {
	f.latestCalls++
	if f.latestErr != nil {
		return release.Release{}, f.latestErr
	}
	return release.Release{Tag: f.latestTag, Assets: []release.Asset{
		{Name: "moonarch-cli-linux-amd64", URL: "https://example.com/amd64"},
		{Name: "moonarch-cli-linux-arm64", URL: "https://example.com/arm64"},
		{Name: "SHA256SUMS.txt", URL: "https://example.com/checksums"},
	}}, nil
}

// GetByTag exists so the fake satisfies release.Client. Self-update never
// calls it; tests assert the zero-call configuration-neutrality guarantee.
func (f *fakeReleaseClient) GetByTag(context.Context, string) (release.Release, error) {
	f.downloads++
	return release.Release{}, errors.New("GetByTag must never be called by self-update")
}

func (f *fakeReleaseClient) Download(_ context.Context, asset release.Asset) (io.ReadCloser, error) {
	f.downloads++
	if err := f.downloadErr[asset.Name]; err != nil {
		return nil, err
	}
	switch asset.Name {
	case "moonarch-cli-linux-amd64", "moonarch-cli-linux-arm64":
		return io.NopCloser(bytes.NewReader(f.binary)), nil
	case "SHA256SUMS.txt":
		sum := sha256.Sum256(f.binary)
		if f.corruptChecksum {
			sum = sha256.Sum256([]byte("tampered bytes"))
		}
		line := hex.EncodeToString(sum[:])
		return io.NopCloser(strings.NewReader(
			line + "  moonarch-cli-linux-amd64\n" + line + "  moonarch-cli-linux-arm64\n",
		)), nil
	default:
		return nil, errors.New("unexpected download")
	}
}

// fakeBinaryReplacer records replacement calls.
type fakeBinaryReplacer struct {
	calls  int
	target string
	asset  string
	err    error
}

func (f *fakeBinaryReplacer) Replace(_ context.Context, targetPath, assetName string, _ io.Reader, _ io.Reader) error {
	f.calls++
	f.target = targetPath
	f.asset = assetName
	return f.err
}

func fixedHome() (string, error) {
	return "/home/user", nil
}

// installCommandWithDeps overrides RunE with the shared self-update runner so
// tests can inject fakes into either the canonical or the alias command.
func installCommandWithDeps(cmd *cobra.Command, version string, deps selfUpdateDependencies) {
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runSelfUpdateWithFactory(c, version, func(*cobra.Command) selfUpdateDependencies { return deps })
	}
}

// assertNoConfigurationSideEffects proves the CLI-only contract: no repository
// checkout, no XDG data/state dirs, and no other configuration footprint is
// created under the supplied home.
func assertNoConfigurationSideEffects(t *testing.T, home string) {
	t.Helper()
	for _, p := range []string{
		filepath.Join(home, ".cache", "dotfiles"),
		filepath.Join(home, ".local", "share", "moonarch"),
		filepath.Join(home, ".local", "state", "moonarch"),
	} {
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("configuration side effect: %s exists", p)
		}
	}
}

func TestSelfUpdateCommand_IsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"self", "update"})
	if err != nil {
		t.Fatalf("self update command not registered: %v", err)
	}
	if cmd.Use != "update" {
		t.Fatalf("Use = %q, want update", cmd.Use)
	}
}

func TestSelfUpdate_ReplacesBinaryAndStaysConfigurationNeutral(t *testing.T) {
	home := t.TempDir()
	deps := selfUpdateDependencies{
		releaseClient: &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("new binary bytes")},
		replacer:      release.NewAtomicReplacer(release.OSFileOps{}, release.SHA256Verifier{}),
		homeResolver:  func() (string, error) { return home, nil },
		arch:          func() string { return "amd64" },
	}

	cmd := newSelfUpdateCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	installCommandWithDeps(cmd, "v1.0.0", deps)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self update error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(home, ".local", "bin", "moonarch-cli"))
	if err != nil {
		t.Fatalf("replaced binary not found: %v", err)
	}
	if string(got) != "new binary bytes" {
		t.Fatalf("binary content = %q, want the verified candidate", string(got))
	}
	info, err := os.Stat(filepath.Join(home, ".local", "bin", "moonarch-cli"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("replaced binary is not executable: mode %v", info.Mode())
	}
	assertNoConfigurationSideEffects(t, home)
}

func TestSelfUpdate_ForceEnvCannotEnableConfigurationStages(t *testing.T) {
	home := t.TempDir()
	stateHome := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("MOONARCH_FORCE_REPO", "/should/never/be/used")
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("XDG_DATA_HOME", dataHome)

	deps := selfUpdateDependencies{
		releaseClient: &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("binary bytes")},
		replacer:      release.NewAtomicReplacer(release.OSFileOps{}, release.SHA256Verifier{}),
		homeResolver:  func() (string, error) { return home, nil },
		arch:          func() string { return "amd64" },
	}
	cmd := newSelfUpdateCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	installCommandWithDeps(cmd, "v1.0.0", deps)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self update error = %v", err)
	}

	// The CLI path still works: the binary is replaced.
	if _, err := os.Stat(filepath.Join(home, ".local", "bin", "moonarch-cli")); err != nil {
		t.Fatalf("CLI binary not replaced: %v", err)
	}
	// No configuration state is written under either XDG root.
	for _, dir := range []string{stateHome, dataHome} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		if len(entries) != 0 {
			t.Fatalf("configuration side effect in %s: %v", dir, entries)
		}
	}
	assertNoConfigurationSideEffects(t, home)
}

func TestSelfUpdate_AlreadyCurrentSkipsDownload(t *testing.T) {
	client := &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("binary")}
	deps := selfUpdateDependencies{
		releaseClient: client,
		replacer:      &fakeBinaryReplacer{},
		homeResolver:  fixedHome,
		arch:          func() string { return "amd64" },
	}

	var out bytes.Buffer
	cmd := newSelfUpdateCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	installCommandWithDeps(cmd, "v1.1.0", deps)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self update error = %v", err)
	}
	if client.downloads != 0 {
		t.Fatalf("downloads = %d, want 0 when already current", client.downloads)
	}
	if !strings.Contains(out.String(), "already up to date") {
		t.Fatalf("output = %q, want already-up-to-date report", out.String())
	}
}

func TestSelfUpdate_InstalledNewerFailsWithoutDownload(t *testing.T) {
	client := &fakeReleaseClient{latestTag: "v1.0.0", binary: []byte("binary")}
	deps := selfUpdateDependencies{
		releaseClient: client,
		replacer:      &fakeBinaryReplacer{},
		homeResolver:  fixedHome,
		arch:          func() string { return "amd64" },
	}
	cmd := newSelfUpdateCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	installCommandWithDeps(cmd, "v1.1.0", deps)
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error when installed is newer")
	}
	if client.downloads != 0 {
		t.Fatalf("downloads = %d, want 0", client.downloads)
	}
}

func TestSelfUpdate_DiscoveryFailureLeavesNoTrace(t *testing.T) {
	home := t.TempDir()
	client := &fakeReleaseClient{latestErr: errors.New("network down")}
	deps := selfUpdateDependencies{
		releaseClient: client,
		replacer:      &fakeBinaryReplacer{},
		homeResolver:  func() (string, error) { return home, nil },
		arch:          func() string { return "amd64" },
	}
	cmd := newSelfUpdateCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	installCommandWithDeps(cmd, "v1.0.0", deps)
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for failed discovery")
	}
	if client.downloads != 0 {
		t.Fatalf("downloads = %d, want 0", client.downloads)
	}
	assertNoConfigurationSideEffects(t, home)
}

func TestSelfUpdate_ChecksumFailurePreservesExistingBinary(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".local", "bin", "moonarch-cli")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	client := &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("new binary"), corruptChecksum: true}
	deps := selfUpdateDependencies{
		releaseClient: client,
		replacer:      release.NewAtomicReplacer(release.OSFileOps{}, release.SHA256Verifier{}),
		homeResolver:  func() (string, error) { return home, nil },
		arch:          func() string { return "amd64" },
	}
	cmd := newSelfUpdateCommand()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	installCommandWithDeps(cmd, "v1.0.0", deps)
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected checksum failure")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("existing binary lost: %v", err)
	}
	if string(got) != "old binary" {
		t.Fatalf("binary = %q, want the prior executable unchanged", string(got))
	}

	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".moonarch-cli-staging-") {
			t.Fatalf("staging temp file left behind: %s", e.Name())
		}
	}
	assertNoConfigurationSideEffects(t, home)
}

func TestSelfUpdate_ConfigSelectorRejectedWithoutMutation(t *testing.T) {
	home := t.TempDir()
	client := &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("binary")}
	deps := selfUpdateDependencies{
		releaseClient: client,
		replacer:      &fakeBinaryReplacer{},
		homeResolver:  func() (string, error) { return home, nil },
		arch:          func() string { return "amd64" },
	}

	for _, tc := range []struct {
		name string
		cmd  func() *cobra.Command
	}{
		{name: "self update", cmd: newSelfUpdateCommand},
		{name: "update", cmd: newUpdateCommand},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := tc.cmd()
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			installCommandWithDeps(cmd, "v1.0.0", deps)
			cmd.SetArgs([]string{"config-v1.2.3"})
			if err := cmd.Execute(); err == nil {
				t.Fatalf("expected rejection of config selector")
			}
			if client.downloads != 0 {
				t.Fatalf("downloads = %d, want 0 (config selector must not be resolved)", client.downloads)
			}
			if _, err := os.Stat(filepath.Join(home, ".local", "bin", "moonarch-cli")); !os.IsNotExist(err) {
				t.Fatalf("binary must not be replaced for a config selector")
			}
			assertNoConfigurationSideEffects(t, home)
		})
	}
}

func TestSelfUpdate_DevBuildExitsZeroWithoutCollaborators(t *testing.T) {
	client := &fakeReleaseClient{latestTag: "v1.1.0", binary: []byte("binary")}
	replacer := &fakeBinaryReplacer{}
	deps := selfUpdateDependencies{releaseClient: client, replacer: replacer}
	var called bool

	var out bytes.Buffer
	cmd := newSelfUpdateCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return runSelfUpdateWithFactory(c, "dev", func(*cobra.Command) selfUpdateDependencies {
			called = true
			return deps
		})
	}
	if err := cmd.Execute(); err != nil {
		t.Fatalf("dev build should exit with nil: %v", err)
	}
	if called {
		t.Fatalf("factory was called in dev build")
	}
	if client.latestCalls != 0 || replacer.calls != 0 {
		t.Fatalf("collaborators were invoked in dev build")
	}
	if out.String() == "" {
		t.Fatalf("expected release-build-only message")
	}
}
