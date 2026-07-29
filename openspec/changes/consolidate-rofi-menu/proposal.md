# Consolidate desktop menus into Rofi

## Intent

Replace the current fragmented launcher setup with one Rofi-based menu system for application launching, window switching, command execution, calculator access, and session power actions. The result should reduce configuration drift while preserving the repository's Tokyo Night visual identity and MoonArch-driven theming.

## Problem statement

The desktop currently carries configuration for three launcher or dock tools with different roles and styling systems:

- `nwg-drawer` is the active application launcher, invoked by `Super+M` from `.config/hypr/hyprland.lua`.
- `wofi` has a configured `drun` launcher and MoonArch theme import, but no active Hyprland keybinding.
- `nwg-dock-hyprland` has styling configuration but is not used.

Only one of these tools is active, while all three remain represented in the dotfiles. This creates unnecessary package and configuration surface, makes the intended launcher difficult to identify, and leaves menu capabilities split or absent. The active launcher also does not provide the requested window, command, calculator, and power workflows through one consistent interface.

## Proposed solution

Adopt `rofi-wayland` as the single menu tool and remove the tracked configuration for `nwg-drawer`, `wofi`, and `nwg-dock-hyprland`.

Rofi will provide distinct user workflows rather than one overloaded menu:

| Workflow | Keybinding | Behavior |
| --- | --- | --- |
| Applications | `Super+M` | Open Rofi directly in `drun` mode. |
| Windows | `Super+Tab` | Open Rofi in window-switcher mode. |
| Commands and calculator | `Super+R` | Open the command-oriented Rofi flow. Calculator functionality is available from this flow or its mode switching, without a dedicated calculator keybinding. |
| Power and session actions | `Super+Shift+X` | Open a dedicated Rofi powermenu containing lock, sleep, logout, reboot, and shutdown. |

`Super+Shift+T` is already bound to the MoonArch theme selector and is preserved unchanged. `Super+Shift+X` was chosen for powermenu to avoid collision.

Logout, reboot, and shutdown must require an explicit confirmation step. Lock and sleep execute immediately after selection.

## Scope

### In scope

- Add a Rofi configuration under `.config/rofi/` suitable for Wayland.
- Configure `drun`, window, run, calculator, and custom powermenu capabilities.
- Replace the current `nwg-drawer` menu command and add the agreed Rofi keybindings in `.config/hypr/hyprland.lua`.
- Add a small powermenu command or script that exposes the required session actions and confirmation behavior.
- Replace `wlogout` in Waybar's power button with the Rofi powermenu (`rofi -show powermenu`).
- Integrate Rofi colors and typography with the current Tokyo Night and MoonArch theme model.
- Use `Hack Nerd Font` and preserve a professional, compact visual style consistent with the rest of the desktop.
- Remove `.config/wofi/`, `.config/nwg-drawer/`, and `.config/nwg-dock-hyprland/` from the repository.
- Document `rofi-wayland` as a prerequisite and document any additional calculator-mode dependency if the selected Rofi package does not include it.

### Out of scope

- Modifying the Go installer or any files under `cli/`.
- Automatically installing or uninstalling system packages.
- **Package cleanup (second iteration):** Removing `nwg-drawer`, `wofi`, `nwg-dock-hyprland`, and `wlogout` from `cli/pkg/installer/packages.go` and `catalog.go` is deferred to a follow-up change. This iteration focuses purely on configuration migration and documentation.
- Managing package-provider selection between `rofi-wayland` and `rofi-lbonn-wayland`; documentation will identify the supported prerequisite, with `rofi-wayland` as the default.
- Replacing unrelated launchers, selectors, bars, notification tools, lock screens, or terminal workflows.
- Redesigning MoonArch beyond the minimum Rofi theme output or import needed for this integration.
- Changing existing keybindings unrelated to launcher, window, command, or power menus.
- Adding a dock replacement.

## Architecture overview

### Configuration structure

The intended structure is:

```text
.config/rofi/
├── config.rasi              # Global behavior, enabled modes, font, icons, matching
├── theme.rasi               # Layout and component styling
└── scripts/
    └── powermenu            # Action selection, confirmation, and dispatch
```

The exact split may be adjusted during design if MoonArch's generated-file conventions require a dedicated import file, but behavior, layout, and action dispatch should remain separated. The powermenu must use explicit action identifiers rather than executing display labels directly.

### Hyprland integration

`.config/hypr/hyprland.lua` will replace:

```lua
local menu = "nwg-drawer -c 6 -is 64 -ovl -nofs"
```

with Rofi-oriented commands and bindings for the four agreed workflows. `Super+M` remains the application-launcher shortcut, minimizing disruption to the existing habit. Existing `Super+Shift+L` lock behavior remains unchanged; lock is additionally available from the powermenu.

### Theme integration

Rofi's theme will use Rasi syntax rather than the GTK CSS used by the retired tools. It should consume a MoonArch-compatible current-theme fragment when available and provide Tokyo Night defaults when that fragment is missing or unreadable. The visual contract is:

- Tokyo Night background, surface, foreground, and accent colors.
- `Hack Nerd Font` for text and Nerd Font action icons.
- Clear selected, urgent, and confirmation states.
- Consistent border radius, spacing, and restrained transparency.
- A usable fallback that does not prevent Rofi from opening when MoonArch state is incomplete.

The implementation must avoid coupling menu functionality to successful theme generation: theme failures may reduce visual fidelity but must not disable launching or power actions.

### Powermenu behavior

The powermenu presents lock, sleep, logout, reboot, and shutdown. It must map each selection to a fixed, reviewed command.

- Lock and sleep dispatch immediately.
- Logout, reboot, and shutdown open a second confirmation prompt.
- Canceling or dismissing either prompt performs no action.
- Unknown, empty, or malformed selections perform no action.

## Affected areas

| Area | Expected change |
| --- | --- |
| `.config/hypr/hyprland.lua` | Replace the `nwg-drawer` command and register separate Rofi bindings. |
| `.config/rofi/` | Add behavior, layout, theme, and powermenu configuration. |
| `.config/waybar/config.jsonc` | Replace `wlogout` on-click with Rofi powermenu. |
| MoonArch theme assets | Add or expose the minimum Rofi-compatible color fragment needed by the active theme mechanism. |
| Dotfiles documentation | State the Rofi prerequisite, optional calculator dependency, shortcuts, and migration notes. |
| Legacy menu directories | Delete Wofi, nwg-drawer, and nwg-dock-hyprland configuration. |
| Package installer (cli/) | **Not modified.** Package cleanup (removing nwg-drawer, wofi, nwg-dock-hyprland, wlogout) deferred to second iteration. |

## Benefits

- One menu dependency and one configuration model replace three launcher-related tools.
- Applications, windows, commands, calculator access, and session actions share consistent interaction and styling.
- Separate shortcuts keep frequent workflows direct and predictable.
- Removing inactive configuration makes the repository's intended desktop setup easier to understand and maintain.
- Confirmation reduces accidental destructive session actions.
- MoonArch integration keeps theme changes centralized while fallback colors preserve availability.

## Trade-offs

- Rofi configuration and custom scripts are more capable but more explicit than the current single `nwg-drawer` command.
- Calculator support may require an additional mode/plugin package depending on the installed Rofi build.
- Rofi does not replace the visual behavior of a persistent dock; this is acceptable because the existing dock configuration is unused and no dock replacement is requested.
- Removing legacy configuration makes rollback depend on Git history rather than keeping dormant files in the active tree.
- Supporting theme fallback adds a small amount of duplication, but prevents the launcher from becoming unavailable when MoonArch output is missing.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Rofi is not installed after stowing the new configuration. | Document `rofi-wayland` as a prerequisite before users activate the new bindings. |
| Calculator mode is unavailable in the installed package. | Document the required compatible plugin/package and keep normal run mode functional when calculator support is absent. |
| A malformed MoonArch import prevents Rofi from rendering. | Keep valid Tokyo Night defaults and make current-theme integration optional or generated atomically. |
| Powermenu selection causes an unintended destructive action. | Use fixed action mapping, require confirmation for logout/reboot/shutdown, and make unknown or canceled input a no-op. |
| Keybinding collision, especially `Super+Tab` or `Super+R`. | Validate Hyprland's effective bindings before migration and preserve unrelated bindings. |
| A command differs across the target Arch/Hyprland environment. | Use commands already available in the environment where possible and document required runtime dependencies. |
| Removal of legacy configuration makes immediate manual rollback less obvious. | Keep the migration in one reviewable commit/PR and provide a Git-based rollback path. |

## Rollback

Rollback is configuration-only:

1. Revert the change through Git to restore the three legacy configuration directories.
2. Restore the `nwg-drawer` `menu` command and original `Super+M` binding in `.config/hypr/hyprland.lua`.
3. Reload Hyprland or restart the session.
4. Remove the Rofi configuration symlink created by Stow if it remains after reverting.

No user data migration or persistent state conversion is required.

## Success criteria

- `Super+M` opens Rofi in application (`drun`) mode.
- `Super+Tab` opens the Rofi window switcher.
- `Super+R` opens command execution and allows calculator access without a dedicated calculator shortcut.
- `Super+Shift+X` opens a powermenu with lock, sleep, logout, reboot, and shutdown.
- Waybar power button opens the same Rofi powermenu (replaces `wlogout`).
- Lock and sleep execute immediately; logout, reboot, and shutdown require confirmation.
- Canceling a powermenu or confirmation prompt has no side effects.
- Rofi uses `Hack Nerd Font` and matches the Tokyo Night/MoonArch visual system.
- Rofi remains usable with fallback Tokyo Night colors if the current MoonArch theme fragment is unavailable.
- The Wofi, nwg-drawer, and nwg-dock-hyprland configuration directories are absent after migration.
- The repository documents `rofi-wayland` and any calculator dependency as prerequisites.
- No files under `cli/` are changed.
- Existing unrelated Hyprland bindings and desktop services continue to work.

## Proposal decisions confirmed with the user

- Keep workflows separate instead of using one combined launcher.
- Use `Super+M` for applications, `Super+Tab` for windows, `Super+R` for commands/calculator access, and `Super+Shift+X` for the powermenu.
- Preserve `Super+Shift+T` for the existing MoonArch theme selector (no change).
- Do not add a dedicated calculator shortcut.
- Require confirmation for logout, reboot, and shutdown; execute lock and sleep immediately.
- Limit delivery to configuration and documentation; do not modify the Go installer.
