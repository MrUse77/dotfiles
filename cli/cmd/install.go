package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/MrUse77/dots-cli/pkg/installer"
	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
	"github.com/MrUse77/dots-cli/pkg/installer/ui/menu"
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

// ErrRepositoryNotFound marks that no repository clone exists in the current
// directory or any canonical location. It is distinct from lookup failures
// (permissions, IO): an error is not proof of absence and must never trigger
// the mutating missing-clone flow.
var ErrRepositoryNotFound = errors.New("repository not found")

func resolveRepositoryRoot(startDir string) (string, error) {
	root, err := findRepositoryRoot(startDir)
	if err == nil {
		return root, nil
	}
	if !errors.Is(err, ErrRepositoryNotFound) {
		return "", err
	}
	// Fall back to the canonical locations where the bootstrap installer
	// (scripts/install.sh) clones the repository.
	for _, candidate := range repositoryCandidates() {
		root, cerr := findRepositoryRoot(candidate)
		if cerr == nil {
			return root, nil
		}
		if !errors.Is(cerr, ErrRepositoryNotFound) {
			return "", cerr
		}
	}
	return "", err
}

// repositoryCandidates returns the canonical locations for the dotfiles
// clone, in priority order: DOTFILES_DIR, then $HOME/.cache/dotfiles.
// The cache location keeps the clone disposable and predictable: the
// repository is always the source of truth.
func repositoryCandidates() []string {
	var candidates []string
	if env := os.Getenv("DOTFILES_DIR"); env != "" {
		candidates = append(candidates, env)
	}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".cache", "dotfiles"))
	}
	return candidates
}

func findRepositoryRoot(startDir string) (string, error) {
	current, err := filepath.Abs(startDir)
	if err != nil {
		return "", fmt.Errorf("resolve repository root from %q: %w", startDir, err)
	}
	info, err := os.Stat(current)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: no Git repository root found", ErrRepositoryNotFound)
		}
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
			return "", fmt.Errorf("%w: no Git repository root found", ErrRepositoryNotFound)
		}
		current = parent
	}
}

// location (DOTFILES_DIR or $HOME/.cache/dotfiles) and returns its path.
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
		plan.WithCatalog(installer.NewActionCatalogWithPowerProfilesAndParu(installer.DetectPowerProfiles(), installer.DetectParu())),
	)
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Interactive dotfiles installer for Arch Linux + Hyprland",
	Long: `An interactive TUI installer that lets you pick exactly which
components to install — from whole groups down to individual packages.

Core system packages (zsh, stow, base-devel, git) and dotfiles are
always installed. Use the menu to toggle groups or dive into each
category for fine-grained control.`,
	RunE: runInstall,
}

func printSummary(w io.Writer, categories []menu.Category, p plan.InstallationPlan) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "╭────────────────────────────────────────────────╮")
	fmt.Fprintln(w, "│              Installation Summary               │")
	fmt.Fprintln(w, "╰────────────────────────────────────────────────╯")
	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "  Targets to link/copy:  %d\n", len(p.ManagedTargets()))
	fmt.Fprintf(w, "  System actions:        %d\n", len(p.ExternalActions()))
	fmt.Fprintln(w, "")

	for _, cat := range categories {
		var selected, deselected []string
		for _, pkg := range cat.Packages {
			if pkg.Selected {
				selected = append(selected, pkg.Name)
			} else {
				deselected = append(deselected, pkg.Name)
			}
		}
		if len(selected) > 0 {
			fmt.Fprintf(w, "  ▸ %s\n", cat.Title)
			for _, n := range selected {
				fmt.Fprintf(w, "    ✓ %s\n", n)
			}
			for _, n := range deselected {
				fmt.Fprintf(w, "    ✗ %s\n", n)
			}
		} else {
			fmt.Fprintf(w, "  ▸ %s  (skipped)\n", cat.Title)
		}
	}
	fmt.Fprintln(w, "")
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
