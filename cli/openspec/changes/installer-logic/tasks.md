# Installer Logic Tasks

## Implementation

- [x] 1.1 Create `pkg/installer/packages.go` to handle `pacman -Syu` and `paru` installation (including checking if paru exists and installing packages).
- [x] 1.2 Create `pkg/installer/system.go` to handle fonts (`fc-cache`), shell (`chsh`), and systemd services (`systemctl enable --now`).
- [x] 1.3 Create `pkg/installer/gsettings.go` to execute the `gsettings set` commands for the GTK theme.
- [x] 1.4 Update `cmd/install.go` to call these functions after the TUI and before/after copying files. Pass `hasAMD` to the package installer.

## Review Workload Forecast
- 400-line budget risk: Medium
- Delivery Strategy: `single-pr`
