# Consolidate Rofi Menu — Specification

## Purpose

Replace the fragmented launcher setup (nwg-drawer, wofi, nwg-dock-hyprland) with a unified Rofi-based menu system providing application launching, window switching, command execution, calculator access, and session power actions, while preserving Tokyo Night visual identity and MoonArch theme integration.

---

## Functional Requirements

### Requirement: Application Launcher (drun mode)

The system MUST provide an application launcher via Rofi in `drun` mode, accessible through the `Super+M` keybinding.

#### Scenario: User opens application launcher

- GIVEN the user is on the Hyprland desktop session
- WHEN the user presses `Super+M`
- THEN Rofi opens in `drun` mode displaying installed applications
- AND the user can type to filter applications
- AND selecting an application launches it

#### Scenario: Application launcher with MoonArch theme

- GIVEN the MoonArch current theme fragment exists and is valid
- WHEN the user opens the application launcher
- THEN Rofi displays using Tokyo Night colors from the MoonArch theme

#### Scenario: Application launcher fallback without MoonArch theme

- GIVEN the MoonArch current theme fragment is missing or unreadable
- WHEN the user opens the application launcher
- THEN Rofi opens successfully using hardcoded Tokyo Night default colors
- AND the launcher remains fully functional

---

### Requirement: Window Switcher

The system MUST provide a window switcher via Rofi in window mode, accessible through the `Super+Tab` keybinding.

#### Scenario: User opens window switcher

- GIVEN the user has multiple windows open
- WHEN the user presses `Super+Tab`
- THEN Rofi opens in window-switcher mode listing open windows
- AND the user can type to filter windows by title or class
- AND selecting a window switches focus to it

#### Scenario: Window switcher with MoonArch theme

- GIVEN the MoonArch current theme fragment exists and is valid
- WHEN the user opens the window switcher
- THEN Rofi displays using Tokyo Night colors from the MoonArch theme

---

### Requirement: Command Runner and Calculator

The system MUST provide command execution via Rofi in run mode, accessible through the `Super+R` keybinding. Calculator functionality MUST be accessible from this flow without a dedicated keybinding.

#### Scenario: User opens command runner

- GIVEN the user is on the Hyprland desktop session
- WHEN the user presses `Super+R`
- THEN Rofi opens in run/command mode
- AND the user can type shell commands for execution

#### Scenario: User accesses calculator from command runner

- GIVEN Rofi is open in command runner mode
- WHEN the user switches to calculator mode (via mode switch or built-in calculator support)
- THEN the calculator interface is available
- AND mathematical expressions can be evaluated
- AND results are displayed

#### Scenario: Command runner without calculator plugin

- GIVEN the installed Rofi package does not include calculator support
- WHEN the user presses `Super+R`
- THEN Rofi opens in run mode successfully
- AND command execution works normally
- AND the absence of calculator does not prevent other functionality

---

### Requirement: Power Menu

The system MUST provide a power/session menu via Rofi, accessible through the `Super+Shift+X` keybinding, offering lock, sleep, logout, reboot, and shutdown actions.

#### Scenario: User opens power menu

- GIVEN the user is on the Hyprland desktop session
- WHEN the user presses `Super+Shift+X`
- THEN Rofi opens displaying five options: lock, sleep, logout, reboot, shutdown

#### Scenario: User selects lock

- GIVEN the power menu is open
- WHEN the user selects "lock"
- THEN the screen locks immediately using the configured lock command
- AND no confirmation prompt appears

#### Scenario: User selects sleep

- GIVEN the power menu is open
- WHEN the user selects "sleep"
- THEN the system enters sleep/suspend mode immediately
- AND no confirmation prompt appears

#### Scenario: User selects logout with confirmation

- GIVEN the power menu is open
- WHEN the user selects "logout"
- THEN a confirmation prompt appears
- AND if the user confirms, the Hyprland session exits
- AND if the user cancels, no action is taken

#### Scenario: User selects reboot with confirmation

- GIVEN the power menu is open
- WHEN the user selects "reboot"
- THEN a confirmation prompt appears
- AND if the user confirms, the system reboots
- AND if the user cancels, no action is taken

#### Scenario: User selects shutdown with confirmation

- GIVEN the power menu is open
- WHEN the user selects "shutdown"
- THEN a confirmation prompt appears
- AND if the user confirms, the system shuts down
- AND if the user cancels, no action is taken

#### Scenario: User cancels power menu

- GIVEN the power menu is open
- WHEN the user presses Escape or closes the menu without selecting
- THEN no action is taken
- AND the desktop session continues normally

#### Scenario: Power menu with malformed selection

- GIVEN the power menu script receives an invalid or unrecognized action identifier
- WHEN the invalid action is passed
- THEN no action is taken
- AND no error disrupts the user session

---

### Requirement: Waybar Power Button Integration

The system MUST replace the `wlogout` invocation in Waybar's power button with the Rofi powermenu.

#### Scenario: Waybar power button opens Rofi powermenu

- GIVEN the user clicks the power button in Waybar
- WHEN the click event fires
- THEN Rofi opens the powermenu (the same script used by `Super+Shift+X`)
- AND wlogout is no longer invoked from Waybar

---

## Theme Requirements

### Requirement: Tokyo Night Color Integration

Rofi configuration MUST use Tokyo Night colors for background, surface, foreground, and accent colors, consistent with the desktop's visual identity.

#### Scenario: Rofi uses Tokyo Night colors

- GIVEN Rofi is opened in any mode
- WHEN the theme is applied
- THEN background colors match Tokyo Night palette
- AND foreground text uses Tokyo Night text colors
- AND accent/selection colors use Tokyo Night accent colors

---

### Requirement: MoonArch Theme Fragment Integration

Rofi configuration MUST consume the MoonArch current-theme fragment when available, using the theme's color definitions.

#### Scenario: MoonArch theme fragment exists

- GIVEN `~/.local/share/moonarch/themes/current/` contains a valid Rofi theme fragment
- WHEN Rofi opens in any mode
- THEN the theme uses colors from the MoonArch current theme
- AND the visual appearance matches the active MoonArch theme

#### Scenario: MoonArch theme fragment is missing

- GIVEN `~/.local/share/moonarch/themes/current/` does not exist or is unreadable
- WHEN Rofi opens in any mode
- THEN Rofi uses hardcoded Tokyo Night default colors
- AND Rofi opens successfully without errors
- AND all functionality remains available

---

### Requirement: Typography and Visual Style

Rofi MUST use `Hack Nerd Font` for text rendering and maintain a professional, compact visual style with clear states for selection, urgency, and confirmation.

#### Scenario: Font rendering

- GIVEN Rofi is configured
- WHEN Rofi opens in any mode
- THEN text is rendered using `Hack Nerd Font`
- AND Nerd Font icons display correctly in action prompts

#### Scenario: Visual states

- GIVEN Rofi is displaying a list of options
- WHEN the user navigates or types
- THEN the selected/highlighted item is visually distinct
- AND confirmation prompts have clear visual differentiation
- AND border radius, spacing, and transparency are restrained and consistent

---

## Configuration Requirements

### Requirement: Rofi Configuration File Structure

The system MUST provide Rofi configuration under `.config/rofi/` with separated behavior, theme, and script files.

#### Scenario: Configuration structure exists

- GIVEN the dotfiles are stowed or deployed
- WHEN checking the configuration directory
- THEN `.config/rofi/config.rasi` exists containing global behavior settings
- AND `.config/rofi/theme.rasi` exists containing layout and component styling
- AND `.config/rofi/scripts/powermenu` exists and is executable
- AND the configuration enables drun, window, run, and calculator modes (when available)

#### Scenario: Configuration uses Rasi syntax

- GIVEN the Rofi configuration files exist
- WHEN inspecting the files
- THEN all theme and configuration files use Rasi syntax (not GTK CSS)
- AND behavior, layout, and action dispatch are separated across files

---

### Requirement: Hyprland Keybinding Integration

The system MUST update `.config/hypr/hyprland.lua` to replace the nwg-drawer command with Rofi commands and register the four required keybindings.

#### Scenario: Menu variable replacement

- GIVEN `.config/hypr/hyprland.lua` is loaded
- WHEN inspecting the `menu` variable or equivalent launcher definition
- THEN the `menu` variable references Rofi in drun mode (not nwg-drawer)
- AND no references to `nwg-drawer` remain in the file

#### Scenario: Super+M binding

- GIVEN `.config/hypr/hyprland.lua` is loaded
- WHEN inspecting keybindings
- THEN `Super+M` is bound to execute Rofi in drun mode
- AND this replaces the previous nwg-drawer binding

#### Scenario: Super+Tab binding

- GIVEN `.config/hypr/hyprland.lua` is loaded
- WHEN inspecting keybindings
- THEN `Super+Tab` is bound to execute Rofi in window-switcher mode

#### Scenario: Super+R binding

- GIVEN `.config/hypr/hyprland.lua` is loaded
- WHEN inspecting keybindings
- THEN `Super+R` is bound to execute Rofi in run/command mode
- AND calculator access is available from this mode

#### Scenario: Super+Shift+X binding

- GIVEN `.config/hypr/hyprland.lua` is loaded
- WHEN inspecting keybindings
- THEN `Super+Shift+X` is bound to execute the Rofi powermenu script

#### Scenario: Existing bindings preserved

- GIVEN `.config/hypr/hyprland.lua` is loaded
- WHEN inspecting keybindings
- THEN `Super+Shift+T` remains bound to MoonArch theme selector
- THEN `Super+Shift+L` remains bound to hyprlock
- AND all other unrelated bindings remain unchanged

---

### Requirement: Powermenu Action Mapping

The powermenu script MUST map each selection to a fixed, reviewed command using explicit action identifiers, not display labels.

#### Scenario: Lock action

- GIVEN the powermenu script receives the "lock" action identifier
- WHEN the action is executed
- THEN the configured lock command (e.g., `hyprlock`) is invoked

#### Scenario: Sleep action

- GIVEN the powermenu script receives the "sleep" action identifier
- WHEN the action is executed
- THEN the system suspend command (e.g., `systemctl suspend`) is invoked

#### Scenario: Logout action

- GIVEN the powermenu script receives the "logout" action identifier
- WHEN the action is executed
- THEN a confirmation prompt is shown
- AND upon confirmation, the Hyprland session exits (e.g., `hyprctl dispatch exit`)

#### Scenario: Reboot action

- GIVEN the powermenu script receives the "reboot" action identifier
- WHEN the action is executed
- THEN a confirmation prompt is shown
- AND upon confirmation, the system reboots (e.g., `systemctl reboot`)

#### Scenario: Shutdown action

- GIVEN the powermenu script receives the "shutdown" action identifier
- WHEN the action is executed
- THEN a confirmation prompt is shown
- AND upon confirmation, the system shuts down (e.g., `systemctl poweroff`)

---

## Removal Requirements

### Requirement: Legacy Configuration Removal

The system MUST remove all tracked configuration for nwg-drawer, wofi, and nwg-dock-hyprland from the repository.

#### Scenario: nwg-drawer configuration removed

- GIVEN the change is applied
- WHEN checking the repository
- THEN `.config/nwg-drawer/` directory does not exist
- AND no files reference nwg-drawer configuration

#### Scenario: wofi configuration removed

- GIVEN the change is applied
- WHEN checking the repository
- THEN `.config/wofi/` directory does not exist
- AND no files reference wofi configuration

#### Scenario: nwg-dock-hyprland configuration removed

- GIVEN the change is applied
- WHEN checking the repository
- THEN `.config/nwg-dock-hyprland/` directory does not exist
- AND no files reference nwg-dock-hyprland configuration

#### Scenario: No legacy launcher references in Hyprland config

- GIVEN the change is applied
- WHEN inspecting `.config/hypr/hyprland.lua`
- THEN no references to `nwg-drawer`, `wofi`, or `nwg-dock-hyprland` commands remain

---

## Acceptance Criteria

The following acceptance criteria MUST be verifiable by `sdd-verify`:

1. **Super+M opens Rofi drun mode**: Pressing `Super+M` launches Rofi in application launcher mode.
2. **Super+Tab opens window switcher**: Pressing `Super+Tab` launches Rofi in window-switcher mode.
3. **Super+R opens command runner**: Pressing `Super+R` launches Rofi in run/command mode.
4. **Calculator accessible from Super+R**: Calculator functionality is reachable from the Super+R flow without a dedicated keybinding.
5. **Super+Shift+X opens powermenu**: Pressing `Super+Shift+X` launches the Rofi powermenu with five options.
6. **Powermenu shows five actions**: The powermenu displays lock, sleep, logout, reboot, and shutdown.
7. **Lock executes immediately**: Selecting "lock" from powermenu locks the screen without confirmation.
8. **Sleep executes immediately**: Selecting "sleep" from powermenu suspends the system without confirmation.
9. **Logout requires confirmation**: Selecting "logout" shows a confirmation prompt before exiting the session.
10. **Reboot requires confirmation**: Selecting "reboot" shows a confirmation prompt before rebooting.
11. **Shutdown requires confirmation**: Selecting "shutdown" shows a confirmation prompt before powering off.
12. **Cancel is safe**: Canceling any powermenu or confirmation prompt performs no action.
13. **Tokyo Night theme applied**: Rofi uses Tokyo Night colors in all modes.
14. **MoonArch integration works**: When MoonArch current theme exists, Rofi uses its colors.
15. **Fallback works without MoonArch**: When MoonArch current theme is missing, Rofi uses Tokyo Night defaults and remains functional.
16. **Hack Nerd Font used**: Rofi renders text using Hack Nerd Font.
17. **Legacy configs removed**: `.config/nwg-drawer/`, `.config/wofi/`, and `.config/nwg-dock-hyprland/` do not exist.
18. **Hyprland bindings updated**: `.config/hypr/hyprland.lua` binds Super+M, Super+Tab, Super+R, Super+Shift+X to Rofi commands.
19. **Existing bindings preserved**: Super+Shift+T and Super+Shift+L remain unchanged.
20. **No CLI changes**: No files under `cli/` are modified.
21. **Prerequisites documented**: Documentation states `rofi-wayland` as a prerequisite.
22. **Calculator dependency documented**: If calculator requires an additional package, it is documented.
23. **Waybar power button uses Rofi**: Waybar power button invokes Rofi powermenu instead of wlogout.

---

## Dependencies

### Runtime Dependencies

- **rofi-wayland**: Required. The AUR package `rofi-lbonn-wayland-git` or equivalent Wayland-compatible Rofi build.
- **calc plugin** (optional): May be required for calculator mode depending on the Rofi build. If the installed package does not include calculator support, the dependency must be documented.

### Build/Configuration Dependencies

- **MoonArch**: The theme integration consumes MoonArch's current-theme fragment. MoonArch must be installed and configured for theme integration to work, but Rofi must remain functional without it.
- **Hack Nerd Font**: Required for proper text and icon rendering.
- **Hyprland**: Required for keybinding integration.

---

## Non-functional Requirements

### Requirement: Performance

Rofi menus MUST open within a perceptibly instant timeframe (<200ms on typical hardware) and remain responsive during filtering and navigation.

#### Scenario: Menu responsiveness

- GIVEN the user invokes any Rofi menu
- WHEN the menu opens
- THEN the menu appears without noticeable delay
- AND typing and navigation respond immediately

---

### Requirement: Error Handling

Rofi and the powermenu script MUST handle errors gracefully without disrupting the user session.

#### Scenario: Missing executable

- GIVEN a powermenu action command is not available on the system
- WHEN the user selects that action and confirms
- THEN an error is logged or displayed
- AND the user session continues without crashing

#### Scenario: Malformed theme

- GIVEN the Rofi theme file contains syntax errors
- WHEN Rofi attempts to open
- THEN Rofi falls back to a default or minimal theme
- AND Rofi opens successfully

#### Scenario: Invalid powermenu input

- GIVEN the powermenu script receives empty, null, or malformed input
- WHEN the input is processed
- THEN no action is executed
- AND the script exits cleanly without error

---

### Requirement: Usability

The powermenu confirmation prompts MUST be clear and prevent accidental destructive actions.

#### Scenario: Confirmation clarity

- GIVEN a destructive action (logout, reboot, shutdown) is selected
- WHEN the confirmation prompt appears
- THEN the prompt clearly states the action being confirmed
- AND the user must explicitly confirm (not just press Enter)
- AND canceling is easy (Escape or explicit cancel option)

---

## Out of Scope

The following are explicitly out of scope for this change:

- Modifying the Go installer or any files under `cli/`.
- Automatically installing or uninstalling system packages.
- Managing package-provider selection between `rofi-wayland` and alternatives.
- Replacing unrelated launchers, selectors, bars, notification tools, lock screens, or terminal workflows.
- Redesigning MoonArch beyond the minimum Rofi theme fragment needed.
- Changing existing keybindings unrelated to launcher, window, command, or power menus.
- Adding a dock replacement.
