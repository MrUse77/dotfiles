package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "dots",
	Short: "Dots CLI - Gestor de dotfiles",
	Long:  `Una CLI robusta escrita en Go para administrar y aplicar temas a dotfiles de forma dinámica.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
