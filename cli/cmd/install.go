package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/MrUse77/dots-cli/pkg/installer"
	"github.com/MrUse77/dots-cli/pkg/installer/external"
	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/MrUse77/dots-cli/pkg/installer/ui"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
)

type installDiscoverer struct{}

func (installDiscoverer) Discover(repoRoot, homeDir string, opts plan.Options) ([]plan.Target, error) {
	catalog := installer.NewActionCatalog()
	targets, err := catalog.ManagedTargets(repoRoot, homeDir, opts)
	if err != nil {
		return nil, err
	}
	filtered := targets[:0]
	for _, target := range targets {
		if _, err := os.Lstat(target.Source); err == nil {
			filtered = append(filtered, target)
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	targets = filtered
	candidates := []struct {
		source, destination string
		kind                plan.MutationKind
	}{
		{filepath.Join(repoRoot, ".config"), filepath.Join(homeDir, ".config"), plan.CopyTree},
	}
	for _, name := range []string{".zshrc", ".gtkrc-2.0", "oh-my-posh", ".zsh_plugins", ".themes"} {
		candidates = append(candidates, struct {
			source, destination string
			kind                plan.MutationKind
		}{filepath.Join(repoRoot, name), filepath.Join(homeDir, name), plan.CopyFile})
	}
	for _, candidate := range candidates {
		info, err := os.Lstat(candidate.source)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		kind := candidate.kind
		if info.IsDir() {
			kind = plan.CopyTree
		}
		targets = append(targets, plan.Target{Source: candidate.source, Destination: candidate.destination, Kind: kind})
	}
	return targets, nil
}

func newInstallPlanner() *plan.Planner {
	return plan.New(
		plan.WithDiscoverer(installDiscoverer{}),
		plan.WithCatalog(installer.NewActionCatalog()),
	)
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Copia toda la configuración del repo al sistema (~/.config)",
	RunE:  runInstall,
}

func runInstall(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Bienvenido al instalador de dotfiles")

	var hasAMD, installPlugins, enableSSHAgent bool
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().Title("¿Tenés GPU AMD? (Instalará corectrl)").Value(&hasAMD),
		huh.NewConfirm().Title("¿Instalar plugins de Hyprland via hyprpm?").Description("Requiere que Hyprland esté corriendo.").Value(&installPlugins),
		huh.NewConfirm().Title("¿Habilitar SSH Agent via systemd?").Description("Gestiona el agente con systemd --user.").Value(&enableSSHAgent),
	)).Run(); err != nil {
		return err
	}
	repoRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	selected := plan.Options{Mode: "user", HasAMD: hasAMD, InstallPlugins: installPlugins, EnableSSHAgent: enableSSHAgent}
	installationPlan, err := newInstallPlanner().Build(repoRoot, homeDir, selected)
	if err != nil {
		return err
	}

	tx := transaction.New(installationPlan)
	executor := installer.NewExecutor(tx, external.NewRunner(nil).WithStdio(cmd.InOrStdin(), out, cmd.ErrOrStderr()))
	result, aborted, err := ui.RunWithContext(cmd.Context(), installationPlan, executor, cmd.InOrStdin(), out, nil)
	if aborted {
		fmt.Fprintln(out, "Instalación cancelada.")
		return nil
	}
	if result != nil {
		printExecutionReport(out, result)
	}
	if err != nil {
		return err
	}
	return nil
}

func printExecutionReport(w io.Writer, result *report.ExecutionReport) {
	fmt.Fprintf(w, "Plan fingerprint: %s\n", result.Fingerprint)
	for _, target := range result.ManagedTargets {
		fmt.Fprintf(w, "managed %s: %s\n", target.Destination, target.Status)
		if target.BackupPath != "" {
			fmt.Fprintf(w, "  retained backup: %s\n", target.BackupPath)
		}
	}
	for _, action := range result.ExternalActions {
		fmt.Fprintf(w, "external %s: %s\n", action.Description, action.Status)
	}
	if result.RecoveryState != "" {
		fmt.Fprintf(w, "rollback: %s\n", result.RecoveryState)
	}
}

func init() {
	rootCmd.AddCommand(installCmd)
}
