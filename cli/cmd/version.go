package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Version is the CLI version. It is injected at build time with:
//
//	go build -ldflags "-X github.com/MrUse77/dots-cli/cmd.Version=v0.1.0"
var Version = "dev"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Muestra la versión del CLI",
	RunE:  runVersion,
}

func runVersion(cmd *cobra.Command, _ []string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "moonarch %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
	return nil
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
