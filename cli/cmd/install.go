package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

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
	if err := requireMoonArchRuntime(repoRoot); err != nil {
		return nil, err
	}
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
	configRoot := filepath.Join(repoRoot, ".config")
	configEntries, err := os.ReadDir(configRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		sort.Slice(configEntries, func(i, j int) bool {
			return configEntries[i].Name() < configEntries[j].Name()
		})
		for _, entry := range configEntries {
			source := filepath.Join(configRoot, entry.Name())
			target, present, err := discoverTarget(source, filepath.Join(homeDir, ".config", entry.Name()), plan.CopyFile, true)
			if err != nil {
				return nil, err
			}
			if present {
				targets = append(targets, target)
			}
		}
	}

	candidates := []struct {
		source, destination string
		kind                plan.MutationKind
	}{
		{filepath.Join(repoRoot, ".local", "bin", "moonarch"), filepath.Join(homeDir, ".local", "bin", "moonarch"), plan.CopyTree},
	}
	for _, name := range []string{".zshrc", ".gtkrc-2.0", "oh-my-posh", ".zsh_plugins", ".themes"} {
		candidates = append(candidates, struct {
			source, destination string
			kind                plan.MutationKind
		}{filepath.Join(repoRoot, name), filepath.Join(homeDir, name), plan.CopyFile})
	}
	for _, candidate := range candidates {
		target, present, err := discoverTarget(candidate.source, candidate.destination, candidate.kind, false)
		if err != nil {
			return nil, err
		}
		if present {
			targets = append(targets, target)
		}
	}
	return targets, nil
}

func discoverTarget(source, destination string, fallback plan.MutationKind, classifySymlink bool) (plan.Target, bool, error) {
	info, err := os.Lstat(source)
	if os.IsNotExist(err) {
		return plan.Target{}, false, nil
	}
	if err != nil {
		return plan.Target{}, false, err
	}

	kind := fallback
	switch {
	case classifySymlink && info.Mode()&os.ModeSymlink != 0:
		kind = plan.Symlink
	case info.IsDir():
		kind = plan.CopyTree
	case info.Mode().IsRegular():
		kind = plan.CopyFile
	}
	return plan.Target{Source: source, Destination: destination, Kind: kind}, true, nil
}

func resolveRepositoryRoot(startDir string) (string, error) {
	current, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve repository root from %q: %w", startDir, err)
	}
	info, err := os.Stat(current)
	if err != nil {
		return "", fmt.Errorf("resolve repository root from %q: %w", startDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("resolve repository root from %q: not a directory", startDir)
	}

	for {
		if _, err := os.Stat(filepath.Join(current, ".git")); err == nil {
			return current, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve repository root marker in %q: %w", current, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("resolve repository root from %q: no Git repository root found", startDir)
		}
		current = parent
	}
}

func requireMoonArchRuntime(repoRoot string) error {
	for _, source := range []string{
		filepath.Join(repoRoot, ".local", "bin", "moonarch"),
		filepath.Join(repoRoot, ".local", "share", "moonarch", "themes"),
	} {
		info, err := os.Stat(source)
		if err != nil {
			return fmt.Errorf("discover MoonArch runtime %q: %w", source, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("discover MoonArch runtime %q: not a directory", source)
		}
	}
	return nil
}

func newInstallPlanner() *plan.Planner {
	return plan.New(
		plan.WithDiscoverer(installDiscoverer{}),
		plan.WithCatalog(installer.NewActionCatalogWithPowerProfiles(installer.DetectPowerProfiles())),
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
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	repoRoot, err := resolveRepositoryRoot(workingDir)
	if err != nil {
		return err
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
