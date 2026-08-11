package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

// selfCmd groups commands that manage the moonarch CLI itself.
var selfCmd = &cobra.Command{
	Use:   "self",
	Short: "Manage the moonarch CLI itself",
}

var selfUpdateCmd = newSelfUpdateCommand()

func newSelfUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update the moonarch CLI binary to the latest release",
		Long: `Update the moonarch-cli binary to the latest published GitHub release.
This command is strictly CLI-only: it never acquires a dotfiles checkout and
never touches configuration state, cache, lock, journal, or inventory.`,
		Args: updateArgs,
		RunE: runSelfUpdate,
	}
}

func runSelfUpdate(cmd *cobra.Command, _ []string) error {
	return runSelfUpdateWithFactory(cmd, Version, defaultSelfUpdateDependencies)
}

// updateArgs rejects every positional argument. Configuration release
// selectors (config-vX.Y.Z) get an explicit rejection pointing at the apply
// command; configuration releases are never resolved by an update command.
func updateArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	if strings.HasPrefix(args[0], "config-v") {
		return fmt.Errorf("configuration release selector %q rejected: run 'moonarch config apply %s' to apply a configuration release", args[0], args[0])
	}
	return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
}

// selfUpdateDependencies holds all collaborators for the CLI-only self-update
// contract. There is deliberately no repository acquirer, planner, executor,
// state, journal, lock, or inventory: self-update never enters a configuration
// path.
type selfUpdateDependencies struct {
	releaseClient release.Client
	replacer      release.BinaryReplacer
	homeResolver  func() (string, error)
	arch          func() string
}

// selfUpdateDependenciesFactory builds dependencies for one invocation.
type selfUpdateDependenciesFactory func(*cobra.Command) selfUpdateDependencies

func defaultSelfUpdateDependencies(_ *cobra.Command) selfUpdateDependencies {
	return selfUpdateDependencies{
		releaseClient: newReleaseClient(),
		replacer:      release.NewAtomicReplacer(release.OSFileOps{}, release.SHA256Verifier{}),
		homeResolver:  os.UserHomeDir,
		arch:          func() string { return runtime.GOARCH },
	}
}

// runSelfUpdateWithFactory guards development builds and then runs the
// CLI-only update with the supplied dependencies. Both the canonical command
// and the update alias share this entry point.
func runSelfUpdateWithFactory(cmd *cobra.Command, currentVersion string, newDeps selfUpdateDependenciesFactory) error {
	if currentVersion == "dev" {
		fmt.Fprintln(cmd.OutOrStdout(), "moonarch update is only available in release builds. Build with a release tag to use this command.")
		return nil
	}
	return runSelfUpdateWithDeps(cmd, currentVersion, newDeps(cmd))
}

func runSelfUpdateWithDeps(cmd *cobra.Command, currentVersion string, deps selfUpdateDependencies) error {
	runner := &selfUpdateRunner{deps: deps}
	return runner.Run(cmd.Context(), cmd.OutOrStdout(), currentVersion)
}

// selfUpdateRunner executes the CLI-only self-update contract.
type selfUpdateRunner struct {
	deps selfUpdateDependencies
}

// Run discovers the latest CLI release, compares it with the installed
// version, selects the architecture binary and its published checksum, stages
// and verifies the candidate, and atomically replaces the installed
// executable. It never acquires a configuration checkout, never plans or
// applies managed targets, and never mutates configuration state, cache,
// lock, journal, or inventory. The new binary becomes active on the next
// invocation; the current process is not re-executed.
func (r *selfUpdateRunner) Run(ctx context.Context, out io.Writer, currentVersion string) error {
	latest, err := r.deps.releaseClient.Latest(ctx)
	if err != nil {
		return fmt.Errorf("discover latest release: %w", err)
	}

	cmp, err := release.CompareVersions(currentVersion, latest.Tag)
	if err != nil {
		return err
	}
	switch cmp {
	case release.InstalledNewer:
		return fmt.Errorf("installed version %s is newer than the latest release %s", currentVersion, latest.Tag)
	case release.InstalledEqual:
		fmt.Fprintf(out, "moonarch CLI is already up to date (%s)\n", latest.Tag)
		return nil
	}

	binaryAsset, err := release.BinaryAsset(latest, r.deps.arch())
	if err != nil {
		return err
	}
	checksumAsset, err := release.ChecksumAsset(latest)
	if err != nil {
		return err
	}

	binaryReader, err := r.deps.releaseClient.Download(ctx, binaryAsset)
	if err != nil {
		return err
	}
	defer binaryReader.Close()

	checksumReader, err := r.deps.releaseClient.Download(ctx, checksumAsset)
	if err != nil {
		return err
	}
	defer checksumReader.Close()

	homeDir, err := r.deps.homeResolver()
	if err != nil {
		return err
	}
	targetPath := filepath.Join(homeDir, ".local", "bin", "moonarch-cli")

	if err := r.deps.replacer.Replace(ctx, targetPath, binaryAsset.Name, binaryReader, checksumReader); err != nil {
		return err
	}
	fmt.Fprintf(out, "moonarch CLI updated to %s; the new binary is active on the next invocation\n", latest.Tag)
	return nil
}

func init() {
	rootCmd.AddCommand(selfCmd)
	selfCmd.AddCommand(selfUpdateCmd)
}

// newReleaseClient returns the production GitHub release client.
func newReleaseClient() release.Client {
	token := os.Getenv("GITHUB_TOKEN")
	doer := &httpClient{client: defaultHTTPClient()}
	return release.NewGitHubClient(doer, token)
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// httpClient wraps *http.Client to satisfy release.HTTPDoer.
type httpClient struct {
	client *http.Client
}

func (h *httpClient) Do(req *http.Request) (*http.Response, error) {
	return h.client.Do(req)
}
