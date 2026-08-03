package cmd

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/MrUse77/dots-cli/pkg/installer"
	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

var updateCmd = newUpdateCommand()

func newUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update the CLI and managed dotfiles to the latest release",
		Long:  `Update the managed moonarch-cli binary and the dotfiles cache to the latest published GitHub release, then reapply file-based configuration.`,
		Args:  cobra.NoArgs,
		RunE:  runUpdate,
	}
}

func runUpdate(cmd *cobra.Command, _ []string) error {
	return runUpdateWithFactory(cmd, Version, defaultUpdateDependencies)
}

func runUpdateWithFactory(cmd *cobra.Command, currentVersion string, newDeps updateDependenciesFactory) error {
	if currentVersion == "dev" {
		fmt.Fprintln(cmd.OutOrStdout(), "moonarch update is only available in release builds. Build with a release tag to use this command.")
		return nil
	}
	return runUpdateWithDeps(cmd, newDeps(cmd))
}

func defaultUpdateDependencies(cmd *cobra.Command) updateDependencies {
	return updateDependencies{
		releaseClient:   newReleaseClient(),
		replacer:        release.NewAtomicReplacer(release.OSFileOps{}, release.SHA256Verifier{}),
		acquirer:        NewRepositoryAcquirer(),
		planBuilder:     newUpdateConfigurationPlanBuilder(installDiscoverer{}, installer.NewActionCatalog()),
		executorFactory: defaultUpdateExecutorFactory(cmd),
		homeResolver:    os.UserHomeDir,
		arch:            func() string { return runtime.GOARCH },
	}
}

func newReleaseClient() release.Client {
	token := os.Getenv("GITHUB_TOKEN")
	doer := &httpClient{client: defaultHTTPClient()}
	return release.NewGitHubClient(doer, token)
}

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

// httpClient wraps *http.Client to satisfy release.HTTPDoer.
type httpClient struct {
	client *http.Client
}

func (h *httpClient) Do(req *http.Request) (*http.Response, error) {
	return h.client.Do(req)
}
