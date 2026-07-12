package installer

import (
	"fmt"
	"os"
	"os/exec"
)

func UpdateAndInstallBase() error {
	fmt.Println("Actualizando sistema e instalando base-devel y git...")
	return runCommand("sudo", "pacman", "-Syu", "--noconfirm", "base-devel", "git")
}

func InstallParu() error {
	_, err := exec.LookPath("paru")
	if err == nil {
		fmt.Println("paru ya está instalado.")
		return nil
	}

	fmt.Println("Instalando paru (AUR helper)...")
	os.RemoveAll("/tmp/paru-install")
	if err := runCommand("git", "clone", "https://aur.archlinux.org/paru.git", "/tmp/paru-install"); err != nil {
		return fmt.Errorf("error clonando paru: %w", err)
	}

	cmd := exec.Command("makepkg", "-si", "--noconfirm")
	cmd.Dir = "/tmp/paru-install"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error compilando paru: %w", err)
	}
	return nil
}

func InstallPackages(hasAMD bool) error {
	fmt.Println("Instalando paquetes base y de AUR...")

	packages := []string{
		// Shell
		"zsh", "stow",

		// Core Wayland / Hyprland
		"hyprland", "hyprlock", "hyprpaper", "hypridle", "hyprsunset",
		"hyprpolkitagent", "waybar", "wofi", "dunst",
		"xdg-desktop-portal-hyprland", "xdg-desktop-portal-gtk",

		// Terminal y herramientas
		"ghostty", "zellij", "neovim", "yazi", "fzf", "eza", "bat", "zoxide",
		"ripgrep", "fd", "wl-clipboard", "direnv", "unzip", "reflector", "lazygit",

		// Administrador de archivos
		"thunar", "gvfs",

		// Gestión de energía
		"upower", "power-profiles-daemon",

		// Theming
		"qt5ct", "qt6ct", "nwg-look", "kvantum",

		// Dependencias para hyprpm
		"cpio", "cmake", "meson",

		// AUR Packages
		"oh-my-posh-bin", "fnm-bin", "nwg-dock-hyprland", "herdr-bin",
		"aur/eww", "aur/wlogout",
	}

	if hasAMD {
		fmt.Println("corectrl será instalado.")
		packages = append(packages, "corectrl")
	}

	args := []string{"-S", "--needed", "--noconfirm"}
	args = append(args, packages...)

	if err := runCommand("paru", args...); err != nil {
		return fmt.Errorf("error instalando paquetes con paru: %w", err)
	}
	return nil
}
