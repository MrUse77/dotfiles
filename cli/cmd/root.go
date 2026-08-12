package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "moonarch-cli",
	Short: "Moonarch CLI - Gestor de dotfiles",
	Long:  `Una CLI robusta escrita en Go para administrar y aplicar temas a dotfiles de forma dinámica.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(commandExitCode(err))
	}
}

func commandExitCode(err error) int {
	if errors.Is(err, release.ErrIndeterminateJournal) {
		return 2
	}
	return 1
}
