# Installer Logic Design

## Overview
Translate the core dependency installation logic from `install.sh` into native Go code in the CLI.

## Requirements
Replicate the following from the bash script:
1. Update system (`pacman -Syu base-devel git`)
2. Install `paru` (AUR helper) if not present by cloning and running `makepkg`
3. Install the official and AUR package lists (Hyprland, Waybar, etc.)
4. If `hasAMD` is true (from TUI), append `corectrl` to the packages list
5. Change default shell to ZSH (`chsh`)
6. Copy fonts and cursors, and run `fc-cache -f`
7. Set GTK themes via `gsettings`
8. Enable systemd services (`upower`, `power-profiles-daemon`)

## Architecture
- Extract this logic into a new `pkg/installer` package to keep `cmd/install.go` clean.
- Use `os/exec` for system commands. 
- IMPORTANT: `cmd.Stdout = os.Stdout` and `cmd.Stderr = os.Stderr` must be set for interactive pacman/paru prompts to work correctly!
