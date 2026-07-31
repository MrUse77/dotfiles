package menu

import "github.com/MrUse77/dots-cli/pkg/installer/plan"

// Category represents a group of installable packages.
type Category struct {
	Key         string
	Title       string
	Description string
	Packages    []Package
}

// Package is a single installable item within a category.
type Package struct {
	Name        string
	Description string
	Selected    bool
}

// DefaultCategories returns the installer categories with all packages
// pre-selected by default.
func DefaultCategories() []Category {
	return []Category{
		{
			Key:         plan.GroupHyprland,
			Title:       "Hyprland Desktop Environment",
			Description: "Window manager, status bar, launcher, notifications, and desktop integration.",
			Packages: []Package{
				{Name: "hyprland", Description: "Dynamic tiling Wayland compositor", Selected: true},
				{Name: "hyprlock", Description: "Screen locker", Selected: true},
				{Name: "hyprpaper", Description: "Wallpaper daemon", Selected: true},
				{Name: "hypridle", Description: "Idle management daemon", Selected: true},
				{Name: "hyprsunset", Description: "Blue light filter", Selected: true},
				{Name: "hyprpolkitagent", Description: "Polkit authentication agent", Selected: true},
				{Name: "waybar", Description: "Status bar", Selected: true},
				{Name: "rofi", Description: "Application launcher & dmenu replacement", Selected: true},
				{Name: "dunst", Description: "Notification daemon", Selected: true},
				{Name: "xdg-desktop-portal-hyprland", Description: "Screencast & screen sharing", Selected: true},
				{Name: "xdg-desktop-portal-gtk", Description: "File picker portal", Selected: true},
				{Name: "nwg-look", Description: "GTK settings editor", Selected: true},
			},
		},
		{
			Key:         plan.GroupDev,
			Title:       "Development Tools",
			Description: "Terminal, editor, multiplexer, Git TUI, and language runtimes.",
			Packages: []Package{
				{Name: "neovim", Description: "Hyperextensible text editor", Selected: true},
				{Name: "ghostty", Description: "GPU-accelerated terminal emulator", Selected: true},
				{Name: "kitty", Description: "Fast, featureful terminal emulator", Selected: false},
				{Name: "zellij", Description: "Terminal workspace & multiplexer", Selected: true},
				{Name: "herdr-bin", Description: "Session manager & multiplexer", Selected: false},
				{Name: "lazygit", Description: "Git TUI", Selected: true},
				{Name: "fnm", Description: "Fast Node version manager", Selected: true},
				{Name: "yazi", Description: "Terminal file manager", Selected: true},
			},
		},
		{
			Key:         plan.GroupCLI,
			Title:       "CLI Utilities",
			Description: "Modern shell replacements for common commands.",
			Packages: []Package{
				{Name: "fzf", Description: "Fuzzy finder", Selected: true},
				{Name: "eza", Description: "Modern ls replacement", Selected: true},
				{Name: "bat", Description: "Cat with syntax highlighting", Selected: true},
				{Name: "zoxide", Description: "Smarter cd", Selected: true},
				{Name: "ripgrep", Description: "Fast recursive grep", Selected: true},
				{Name: "fd", Description: "Fast find replacement", Selected: true},
				{Name: "wl-clipboard", Description: "Wayland clipboard CLI", Selected: true},
				{Name: "direnv", Description: "Per-directory environment", Selected: true},
			},
		},
		{
			Key:         plan.GroupTheming,
			Title:       "Theming & Appearance",
			Description: "GTK/Qt themes, icons, prompt, and widgets.",
			Packages: []Package{
				{Name: "qt5ct", Description: "Qt5 theme configuration", Selected: true},
				{Name: "qt6ct", Description: "Qt6 theme configuration", Selected: true},
				{Name: "kvantum", Description: "SVG-based Qt theming engine", Selected: true},
				{Name: "oh-my-posh-bin", Description: "Cross-shell prompt", Selected: true},
				{Name: "aur/eww", Description: "Elkowar's wacky widgets", Selected: true},
				{Name: "aur/wlogout", Description: "Wayland logout menu", Selected: true},
			},
		},
		{
			Key:         plan.GroupAMD,
			Title:       "AMD GPU Support",
			Description: "GPU frequency and fan management for AMD cards.",
			Packages: []Package{
				{Name: "corectrl", Description: "AMD GPU control panel", Selected: true},
			},
		},
		{
			Key:         plan.GroupPlugins,
			Title:       "Hyprland Plugins (hyprpm)",
			Description: "hyprbars and split-monitor-workspaces. Needs running Hyprland.",
			Packages: []Package{
				{Name: "hyprbars", Description: "Title bars for Hyprland windows", Selected: true},
				{Name: "split-monitor-workspaces", Description: "Per-monitor workspace switching", Selected: true},
			},
		},
	}
}

// SelectedGroups returns the group keys for categories that have at least
// one package selected.
func SelectedGroups(categories []Category) []string {
	var groups []string
	for _, cat := range categories {
		for _, pkg := range cat.Packages {
			if pkg.Selected {
				groups = append(groups, cat.Key)
				break
			}
		}
	}
	return groups
}

// SelectedPackages returns the names of selected packages across all categories.
func SelectedPackages(categories []Category) map[string][]string {
	result := make(map[string][]string)
	for _, cat := range categories {
		var names []string
		for _, pkg := range cat.Packages {
			if pkg.Selected {
				names = append(names, pkg.Name)
			}
		}
		if len(names) > 0 {
			result[cat.Key] = names
		}
	}
	return result
}

// ExcludedPackages returns package names that were deselected in categories
// that are otherwise enabled (at least one package selected). Categories
// with nothing selected are excluded entirely via SelectedGroups.
func ExcludedPackages(categories []Category) []string {
	var excluded []string
	for _, cat := range categories {
		groupEnabled := false
		for _, pkg := range cat.Packages {
			if pkg.Selected {
				groupEnabled = true
				break
			}
		}
		if !groupEnabled {
			continue // entire group excluded; handled by SelectedGroups
		}
		for _, pkg := range cat.Packages {
			if !pkg.Selected {
				excluded = append(excluded, pkg.Name)
			}
		}
	}
	return excluded
}
