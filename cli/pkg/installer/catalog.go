package installer

import (
	"path/filepath"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

// ActionCatalog describes the installer operations without executing them.
type ActionCatalog struct {
	powerProfiles *PowerProfilesState
	paruAvailable bool
}

// NewActionCatalog returns the deterministic catalog used by the planner.
// Environment-dependent power-profile actions are omitted until state is supplied.
func NewActionCatalog() ActionCatalog { return ActionCatalog{} }

// NewActionCatalogWithParu includes paru availability in the planned actions.
func NewActionCatalogWithParu(paruAvailable bool) ActionCatalog {
	return ActionCatalog{paruAvailable: paruAvailable}
}

// NewActionCatalogWithPowerProfiles adds power-profile actions selected from
// the supplied read-only systemd state.
func NewActionCatalogWithPowerProfiles(state PowerProfilesState) ActionCatalog {
	return ActionCatalog{powerProfiles: &state}
}

// NewActionCatalogWithPowerProfilesAndParu includes detected system state in the catalog.
func NewActionCatalogWithPowerProfilesAndParu(state PowerProfilesState, paruAvailable bool) ActionCatalog {
	return ActionCatalog{powerProfiles: &state, paruAvailable: paruAvailable}
}

// ExternalActions returns the selected external operations in execution order.
// repoRoot and homeDir anchor repository-scoped and home-scoped commands.
func (catalog ActionCatalog) ExternalActions(repoRoot, homeDir string, opts plan.Options) ([]plan.ExternalAction, error) {
	packages := collectPackages(opts)

	actions := []plan.ExternalAction{
		action("update system and install base tools", "sudo", []string{"pacman", "-Syu", "--noconfirm", "base-devel", "git"}, "privileged", true),
	}
	if !catalog.paruAvailable {
		actions = append(actions,
			action("clean paru build directory", "rm", []string{"-rf", "--", paruBuildDir}, "filesystem", true),
			action("bootstrap paru", "git", []string{"clone", "https://aur.archlinux.org/paru.git", paruBuildDir}, "supply-chain", true),
			plan.ExternalAction{Description: "build and install paru", Command: plan.CommandSpec{Name: "makepkg", Args: []string{"-si", "--noconfirm"}, Dir: paruBuildDir}, Classification: "supply-chain", Irreversible: true},
		)
	}
	actions = append(actions, action("install configured packages", "paru", append([]string{"-S", "--needed", "--noconfirm"}, packages...), "supply-chain", true))

	actions = append(actions,
		action("change default shell to zsh", "chsh", []string{"-s", "/usr/bin/zsh"}, "system", true),
	)

	submodules := action("update git submodules", "git", []string{"submodule", "update", "--init", "--recursive"}, "repository", true)
	submodules.Command.Dir = repoRoot
	actions = append(actions, submodules,
		action("create zsh configuration directory", "mkdir", []string{"-p", filepath.Join(homeDir, ".config", "zsh")}, "filesystem", false),
	)

	if catalog.powerProfiles != nil {
		actions = append(actions, powerProfilesActions(*catalog.powerProfiles)...)
	}
	actions = append(actions,
		action("enable upower", "sudo", []string{"systemctl", "enable", "--now", "upower"}, "privileged", true),
		action("refresh font cache", "fc-cache", []string{"-f"}, "cache", false),
	)

	if opts.HasGroup(plan.GroupTheming) || len(opts.Groups) == 0 {
		settings := []struct{ key, value string }{
			{"gtk-theme", "TokyoNight-zk"}, {"icon-theme", "TokyoNight-SE"},
			{"cursor-theme", "volantes_cursors"}, {"cursor-size", "24"},
			{"font-name", "CaskaydiaMono Nerd Font Mono Bold 10"}, {"color-scheme", "prefer-dark"},
		}
		for _, setting := range settings {
			actions = append(actions, action("set "+setting.key, "gsettings", []string{"set", "org.gnome.desktop.interface", setting.key, setting.value}, "external", false))
		}
	}

	if opts.HasGroup(plan.GroupPlugins) {
		actions = append(actions,
			action("update Hyprland plugins", "hyprpm", []string{"update"}, "supply-chain", true),
			action("add Hyprland plugins", "hyprpm", []string{"add", "https://github.com/hyprwm/hyprland-plugins"}, "supply-chain", true),
			action("enable hyprbars", "hyprpm", []string{"enable", "hyprbars"}, "supply-chain", true),
			action("add split monitor workspaces", "hyprpm", []string{"add", "https://github.com/zjeffer/split-monitor-workspaces"}, "supply-chain", true),
			action("enable split monitor workspaces", "hyprpm", []string{"enable", "split-monitor-workspaces"}, "supply-chain", true),
		)
	}
	for i := range actions {
		actions[i].Order = i
	}
	return actions, nil
}

// collectPackages builds the package list from the selected feature groups.
// Base packages are always included; optional groups add their own packages.
func collectPackages(opts plan.Options) []string {
	base := []string{
		"zsh", "stow", "unzip", "reflector", "cpio", "cmake", "meson",
		"thunar", "gvfs", "upower", "power-profiles-daemon",
	}

	groups := map[string][]string{
		plan.GroupHyprland: {
			"hyprland", "hyprlock", "hyprpaper", "hypridle", "hyprsunset",
			"hyprpolkitagent", "aur/waybar-git", "rofi", "dunst",
			"xdg-desktop-portal-hyprland", "xdg-desktop-portal-gtk",
			"nwg-look",
		},
		plan.GroupDev: {
			"ghostty", "kitty", "zellij", "herdr-bin", "neovim", "yazi", "lazygit", "fnm",
		},
		plan.GroupCLI: {
			"fzf", "eza", "bat", "zoxide", "ripgrep", "fd",
			"wl-clipboard", "direnv",
		},
		plan.GroupTheming: {
			"qt5ct", "qt6ct", "kvantum", "oh-my-posh-bin",
			"aur/eww", "aur/wlogout",
		},
		plan.GroupAMD: {"corectrl"},
	}

	excludeSet := make(map[string]bool, len(opts.ExcludePackages))
	for _, p := range opts.ExcludePackages {
		excludeSet[p] = true
	}

	packages := make([]string, len(base))
	copy(packages, base)

	for _, group := range plan.AllGroups() {
		if opts.HasGroup(group) {
			if pkgs, ok := groups[group]; ok {
				for _, p := range pkgs {
					if !excludeSet[p] {
						packages = append(packages, p)
					}
				}
			}
		}
	}

	// When no groups are selected at all (legacy mode), install everything
	// but still respect exclusions.
	if len(opts.Groups) == 0 {
		for _, pkgs := range groups {
			for _, p := range pkgs {
				if !excludeSet[p] {
					packages = append(packages, p)
				}
			}
		}
	}

	return packages
}

func action(description, name string, args []string, classification string, irreversible bool) plan.ExternalAction {
	return plan.ExternalAction{Description: description, Command: plan.CommandSpec{Name: name, Args: args}, Classification: classification, Irreversible: irreversible}
}

// ManagedTargets returns filesystem mutations for assets that were copied by the legacy installer.
func (ActionCatalog) ManagedTargets(repoRoot, homeDir string, _ plan.Options) ([]plan.Target, error) {
	return []plan.Target{
		{Source: filepath.Join(repoRoot, "assets", "fonts"), Destination: filepath.Join(homeDir, ".local", "share", "fonts"), Kind: plan.CopyTree},
		{Source: filepath.Join(repoRoot, "assets", "icons"), Destination: filepath.Join(homeDir, ".local", "share", "icons"), Kind: plan.CopyTree},
		{Source: filepath.Join(repoRoot, ".local", "share", "moonarch", "themes"), Destination: filepath.Join(homeDir, ".local", "share", "moonarch", "themes"), Kind: plan.CopyTree},
	}, nil
}
