package installer

import (
	"path/filepath"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

// ActionCatalog describes the installer operations without discovering or executing them.
type ActionCatalog struct{}

// NewActionCatalog returns the deterministic catalog used by the planner.
func NewActionCatalog() ActionCatalog { return ActionCatalog{} }

// ExternalActions returns the selected external operations in execution order.
func (ActionCatalog) ExternalActions(opts plan.Options) ([]plan.ExternalAction, error) {
	packages := []string{
		"zsh", "stow", "hyprland", "hyprlock", "hyprpaper", "hypridle", "hyprsunset",
		"hyprpolkitagent", "waybar", "wofi", "dunst", "xdg-desktop-portal-hyprland", "xdg-desktop-portal-gtk",
		"ghostty", "zellij", "neovim", "yazi", "fzf", "eza", "bat", "zoxide", "ripgrep", "fd",
		"wl-clipboard", "direnv", "unzip", "reflector", "lazygit", "thunar", "gvfs", "upower",
		"power-profiles-daemon", "qt5ct", "qt6ct", "nwg-look", "kvantum", "cpio", "cmake", "meson",
		"oh-my-posh-bin", "fnm-bin", "nwg-dock-hyprland", "herdr-bin", "aur/eww", "aur/wlogout",
	}
	if opts.HasAMD {
		packages = append(packages, "corectrl")
	}
	actions := []plan.ExternalAction{
		action("update system and install base tools", "sudo", []string{"pacman", "-Syu", "--noconfirm", "base-devel", "git"}, "privileged", true),
		action("bootstrap paru", "git", []string{"clone", "https://aur.archlinux.org/paru.git", "/tmp/paru-install"}, "supply-chain", true),
		{Description: "build and install paru", Command: plan.CommandSpec{Name: "makepkg", Args: []string{"-si", "--noconfirm"}, Dir: "/tmp/paru-install"}, Classification: "supply-chain", Irreversible: true},
		action("install configured packages", "paru", append([]string{"-S", "--needed", "--noconfirm"}, packages...), "supply-chain", true),
	}

	actions = append(actions,
		action("change default shell to zsh", "chsh", []string{"-s", "/usr/bin/zsh"}, "system", true),
		action("update git submodules", "git", []string{"submodule", "update", "--init", "--recursive"}, "repository", true),
		action("create zsh configuration directory", "mkdir", []string{"-p", "~/.config/zsh"}, "filesystem", false),
	)

	actions = append(actions,
		action("enable power profiles", "sudo", []string{"systemctl", "enable", "--now", "power-profiles-daemon.service"}, "privileged", true),
		action("enable upower", "sudo", []string{"systemctl", "enable", "--now", "upower"}, "privileged", true),
		action("refresh font cache", "fc-cache", []string{"-f"}, "cache", false),
	)

	settings := []struct{ key, value string }{
		{"gtk-theme", "TokyoNight-zk"}, {"icon-theme", "TokyoNight-SE"},
		{"cursor-theme", "volantes_cursors"}, {"cursor-size", "24"},
		{"font-name", "CaskaydiaMono Nerd Font Mono Bold 10"}, {"color-scheme", "prefer-dark"},
	}
	for _, setting := range settings {
		actions = append(actions, action("set "+setting.key, "gsettings", []string{"set", "org.gnome.desktop.interface", setting.key, setting.value}, "external", false))
	}
	if opts.EnableSSHAgent {
		actions = append(actions, action("enable SSH agent", "systemctl", []string{"--user", "enable", "--now", "ssh-agent"}, "external", true))
	}
	if opts.InstallPlugins {
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

func action(description, name string, args []string, classification string, irreversible bool) plan.ExternalAction {
	return plan.ExternalAction{Description: description, Command: plan.CommandSpec{Name: name, Args: args}, Classification: classification, Irreversible: irreversible}
}

// ManagedTargets returns filesystem mutations for assets that were copied by the legacy installer.
func (ActionCatalog) ManagedTargets(repoRoot, homeDir string, _ plan.Options) ([]plan.Target, error) {
	return []plan.Target{
		{Source: filepath.Join(repoRoot, "assets", "fonts"), Destination: filepath.Join(homeDir, ".local", "share", "fonts"), Kind: plan.CopyTree},
		{Source: filepath.Join(repoRoot, "assets", "icons"), Destination: filepath.Join(homeDir, ".local", "share", "icons"), Kind: plan.CopyTree},
		{Source: filepath.Join(repoRoot, ".local", "moonarch", "themes"), Destination: filepath.Join(homeDir, ".local", "moonarch", "themes"), Kind: plan.CopyTree},
	}, nil
}
