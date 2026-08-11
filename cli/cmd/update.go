package cmd

import (
	"github.com/spf13/cobra"
)

var updateCmd = newUpdateCommand()

func newUpdateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update the moonarch CLI binary to the latest release",
		Long: `Update the moonarch-cli binary to the latest published GitHub release.
This command is a backward-compatible alias for 'moonarch self update' and is
strictly CLI-only: it never acquires a dotfiles checkout and never touches
configuration state, cache, lock, journal, or inventory.`,
		Args: updateArgs,
		RunE: runUpdate,
	}
}

// runUpdate delegates to the canonical CLI-only self-update runner so both
// commands share one implementation and produce an equivalent result.
func runUpdate(cmd *cobra.Command, _ []string) error {
	return runSelfUpdateWithFactory(cmd, Version, defaultSelfUpdateDependencies)
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
