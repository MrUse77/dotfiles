# Design: Consolidate Desktop Menus into Rofi

## Decision summary

Use the Arch community package **`rofi-wayland`** as the supported Rofi implementation. It replaces the active `nwg-drawer` workflow and the inactive Wofi/dock configuration. `rofi-lbonn-wayland-git` remains an unsupported alternative, not the documented default.

Rofi is launched through a small wrapper so MoonArch theme loading is optional and validated. This is necessary because a direct Rasi `@import` of a missing or invalid current-theme fragment can prevent Rofi from parsing; fallback must never disable a launcher or power action.

The implementation also makes the minimal MoonArch consumer change required to preserve `Super+Shift+T`: the selector moves from `wofi --show dmenu` to Rofi dmenu and validates a `rofi.rasi` fragment instead of `wofi.css`. The Hyprland binding itself remains unchanged.

## Architecture and data flow

```text
Hyprland / Waybar
  -> ~/.config/rofi/scripts/launch -show <mode>
  -> validate and compose fallback theme + optional MoonArch current/rofi.rasi
  -> rofi-wayland
     -> drun | window | run | calc | powermenu
     -> powermenu custom mode invokes ~/.config/rofi/scripts/powermenu
        -> optional confirmation dmenu
        -> fixed action identifier -> fixed command
```

All normal launch paths use `scripts/launch`, including Waybar and the MoonArch selector. The wrapper chooses only a theme; it forwards every Rofi argument unchanged and does not change mode behavior.

## File structure and responsibilities

```text
.config/rofi/
├── config.rasi          # Rofi behavior, modes, font, matching, base theme selection
├── theme.rasi           # Tokyo Night fallback variables and all widget/layout styling
└── scripts/
    ├── launch           # Theme-safe Rofi launcher/wrapper; executable
    └── powermenu        # Rofi script-mode provider and fixed action dispatcher; executable
```

### `.config/rofi/config.rasi`

`config.rasi` contains configuration only, plus `@theme "~/.config/rofi/theme.rasi"` as the permanent fallback/base theme.

Required configuration values:

- `modi`: `drun,window,run,calc,powermenu:~/.config/rofi/scripts/powermenu`.
  - `calc` is an optional plugin mode. If its plugin is absent, Rofi must still expose `drun`, `window`, `run`, and `powermenu`; this is verified on the installed `rofi-wayland` build.
- `show-icons: true`, `icon-theme: "Papirus-Dark"`, and `drun-display-format: "{name}"`.
- `font: "Hack Nerd Font 15"`.
- `matching: "fuzzy"`, `case-sensitive: false`, and a deterministic sorting method (`fzf`).
- Mode labels: Apps (`drun`), Windows (`window`), Run (`run`), Calculator (`calc`), and Power (`powermenu`), so the mode switcher makes calculator discoverable from `Super+R` without a separate binding.
- `sidebar-mode: true` (or the equivalent visible mode switcher configuration), so the user can switch from run to calc with Rofi's mode-switch key.

`window` is the selected window mode, not `windowcd`: it is the standard window switcher and directly fulfils the title/class filtering contract. `windowcd` changes working directories and is not a window-focus workflow.

Calculator uses the `calc` modi/plugin integration, not a `qalc` command wrapper. Document `rofi-calc-wayland` (or the package supplying Rofi's `calc` mode for the chosen build) and `libqalculate` as optional calculator prerequisites. The launcher remains usable when they are absent.

### `.config/rofi/theme.rasi`

This file is a complete, independently valid theme. It defines fallback variables first and styles all widgets using only those variables.

Fallback Tokyo Night palette:

```rasi
* {
    moonarch-background: #1a1b26e6;
    moonarch-surface: #24283b;
    moonarch-foreground: #c0caf5;
    moonarch-accent: #7aa2f7;
    moonarch-urgent: #f7768e;
    moonarch-selected-text: #1a1b26;
}
```

All MoonArch Rofi fragments use exactly these `moonarch-*` names and override values only; they do not contain layout or widget selectors. This separates per-theme color output from the shared Rofi UI design.

The layout is centered and compact: approximately 560px wide, with a 12px rounded window, 2px accent border, restrained background opacity, 10px input/list spacing, and application icons at 24px. `Hack Nerd Font 15` is set by `config.rasi` and action glyphs are Nerd Font codepoints.

Required widget/state styling:

- `window`, `mainbox`, `inputbar`, `prompt`, `entry`, `element`, `element-icon`, and `element-text` define the common layout.
- `element selected` uses `@moonarch-accent` background and `@moonarch-selected-text` foreground.
- `element urgent` uses `@moonarch-urgent` with readable foreground for destructive-action emphasis.
- `message` and `textbox` use the surface/background colors; confirmation prompts use an urgent border/prompt so they are visually distinct from ordinary selection.
- Normal, alternate, selected, and urgent element states have explicit foreground/background values rather than inheriting tool defaults.

### `.config/rofi/scripts/launch`

The launcher prevents an unavailable or malformed MoonArch fragment from breaking Rofi:

1. Set `base_theme="$HOME/.config/rofi/theme.rasi"` and `fragment="$HOME/.local/share/moonarch/themes/current/rofi.rasi"`.
2. Start from the validated base theme.
3. If the fragment is readable, atomically write a runtime composite Rasi file containing:

   ```rasi
   @import "<absolute base_theme path>";
   @import "<absolute current rofi.rasi path>";
   ```

   The fragment import is last, so its color variables override the fallback values.
4. Validate that composite once per fragment content signature (`mtime:size`) with `rofi -theme <composite> -dump-theme`. Cache the valid composite in `$XDG_RUNTIME_DIR` (fall back to `/tmp` with a UID-specific filename). Revalidate only after the signature changes.
5. On a validation failure, write the reason to stderr and launch with `base_theme` only. Do not show a blocking dialog and do not return failure merely because theming failed.
6. `exec rofi -theme <chosen-theme> "$@"`.

The temporary/composite file is created with `mktemp`, written before rename, and permission `0600`; shell paths and arguments are quoted. This makes a MoonArch switch atomic from Rofi's perspective and keeps normal opens below the responsiveness target after the one-time validation.

## MoonArch integration contract

Each bundle under `.local/share/moonarch/themes/<theme-id>/` supplies a minimal `rofi.rasi` fragment:

```rasi
* {
    moonarch-background: #rrggbbaa;
    moonarch-surface: #rrggbb;
    moonarch-foreground: #rrggbbaa;
    moonarch-accent: #rrggbbaa;
    moonarch-urgent: #rrggbbaa;
    moonarch-selected-text: #rrggbbaa;
}
```

For Tokyo Night, use the fallback values shown above, with `moonarch-urgent: #f7768e` and `moonarch-selected-text: #1a1b26`. Other bundles convert their existing `wofi_background`, `wofi_surface`, `wofi_foreground`, and `wofi_accent` values to the corresponding Rasi variables, and add contrast-safe urgent/selected-text values.

The `current` symlink is still MoonArch-owned. The Rofi wrapper imports `current/rofi.rasi` only after it is readable and parse-valid; therefore missing, unreadable, or malformed current output results in the hardcoded Tokyo Night fallback.

Update `.local/bin/moonarch/theme-selector` minimally:

- Change the required bundle file from `wofi.css` to `rofi.rasi`.
- Replace `valid_ids | wofi --show dmenu` with `valid_ids | "$HOME/.config/rofi/scripts/launch" -dmenu -p "Theme" -no-custom`.
- Leave the `Super+Shift+T` Hyprland binding and its selector path unchanged.

This avoids leaving Wofi as a hidden runtime dependency after deleting its configuration.

## Powermenu design

### Rofi script-mode protocol

`scripts/powermenu` is a Bash script. With an initial Rofi script-mode request (`ROFI_RETV=0`), it emits the five machine identifiers below. Each row uses Rofi row metadata so the visible string is a friendly icon/label while the selected value remains the identifier. For example:

```bash
printf 'lock\0display\x1f󰌾  Lock\n'
```

The remaining rows use `sleep`, `logout`, `reboot`, and `shutdown` in the same form. On a selection (`ROFI_RETV=1`), the script accepts only the exact first positional argument identifier. It never derives a command from a label or arbitrary typed text.

### Action table

| Identifier | Visible label | Confirmation | Command |
| --- | --- | --- | --- |
| `lock` | `󰌾 Lock` | No | `hyprlock` |
| `sleep` | `󰤄 Sleep` | No | `systemctl suspend` |
| `logout` | `󰍃 Log out` | Yes | `hyprctl dispatch exit` |
| `reboot` | `󰜉 Reboot` | Yes | `systemctl reboot` |
| `shutdown` | `󰐥 Shutdown` | Yes | `systemctl poweroff` |

`lock` and `sleep` dispatch immediately after exact matching. `logout`, `reboot`, and `shutdown` call `confirm_action <identifier>` before dispatch.

### Confirmation flow

`confirm_action` pipes two identifier rows into the same launcher in dmenu mode:

1. `cancel` is the first/default row and is displayed as `Cancel`.
2. `confirm` is displayed as `Confirm <action label>` and marked urgent.
3. It invokes `scripts/launch -dmenu -p "Confirm <action>?" -selected-row 0 -no-custom`.
4. Only an exact returned value of `confirm` dispatches the fixed command. Escape, a nonzero Rofi exit code, empty output, `cancel`, or any other string returns success with no side effect.

Putting Cancel first prevents Enter from confirming by default; the user must explicitly move to or click Confirm.

### Error and input handling

- Use `set -u` and explicit status checks, not `set -e`, so an unavailable notification command cannot terminate the control flow unexpectedly.
- A `run_action` helper checks `command -v` for the executable (`hyprlock`, `systemctl`, or `hyprctl`) before invocation. Missing commands and nonzero command exits call `report_error`.
- `report_error` writes a clear message to stderr and sends a critical `notify-send` notification only if `notify-send` exists. It never exits Hyprland or retries another action.
- Empty initial arguments, canceled menus, malformed identifiers, direct invocations with unknown arguments, and unexpected `ROFI_RETV` values exit 0 with no action.
- The script performs no `eval`, shell interpolation of selection text, or command construction from user input.

## Hyprland changes

In `.config/hypr/hyprland.lua`, replace the current launcher declaration:

```lua
local menu = "nwg-drawer -c 6 -is 64 -ovl -nofs"
```

with these declarations:

```lua
local rofi = os.getenv("HOME") .. "/.config/rofi/scripts/launch"
local menu = rofi .. " -show drun"
local windowMenu = rofi .. " -show window"
local runMenu = rofi .. " -show run"
local powerMenu = rofi .. " -show powermenu"
```

Keep the existing `Super+M` line, now using the new `menu`, and add adjacent launcher bindings:

```lua
hl.bind(mainMod .. " + M", hl.dsp.exec_cmd(menu))
hl.bind(mainMod .. " + Tab", hl.dsp.exec_cmd(windowMenu))
hl.bind(mainMod .. " + R", hl.dsp.exec_cmd(runMenu))
hl.bind(mainMod .. " + SHIFT + X", hl.dsp.exec_cmd(powerMenu))
```

Do not alter `Super+Shift+T` (MoonArch selector), `SUPER + SHIFT + L` (`hyprlock`), or any other binding. No `nwg-drawer`, Wofi, or nwg-dock command may remain in this file.

## Waybar change

In `.config/waybar/config.jsonc`, change only the `custom/power` click command:

```jsonc
"on-click": "$HOME/.config/rofi/scripts/launch -show powermenu"
```

This replaces the current `"on-click": "wlogout"` and invokes the same custom Rofi mode and theme-safe launcher as `Super+Shift+X`. The unrelated `custom/launcher` is also changed from its current `nwg-drawer ...` command to `"$HOME/.config/rofi/scripts/launch -show drun"`; otherwise the Waybar launcher would retain a removed dependency.

## Legacy removal and MoonArch asset migration

Delete these legacy configuration directories and their currently tracked files:

- `.config/wofi/` — `config`, `style.css`
- `.config/nwg-drawer/` — `drawer.css`
- `.config/nwg-dock-hyprland/` — `style.css`

Replace (delete the old CSS file and add the Rasi file) in every current MoonArch bundle:

- `.local/share/moonarch/themes/catppuccin-latte/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/catppuccin-mocha/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/decay-green/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/edge-runner/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/frosted-glass/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/graphite-mono/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/gruvbox-retro/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/material-sakura/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/nordic-blue/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/rose-pine/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/synth-wave/wofi.css` -> `rofi.rasi`
- `.local/share/moonarch/themes/tokyo-night/wofi.css` -> `rofi.rasi`

Update `README.md` to replace its Wofi selector wording with Rofi, list `rofi-wayland` and Hack Nerd Font as required runtime prerequisites, list the optional calculator plugin/package and `libqalculate`, document the four shortcuts, and state that Wofi/nwg-drawer/nwg-dock configuration was removed. Do not modify anything under `cli/`.

## Verification plan

### Static checks

1. Confirm the five new `.config/rofi` files exist and both scripts are executable.
2. Run `rofi -theme ~/.config/rofi/theme.rasi -dump-theme` and validate every generated MoonArch `rofi.rasi` through the launcher composite path.
3. Run `bash -n` on `scripts/launch`, `scripts/powermenu`, and `.local/bin/moonarch/theme-selector`.
4. Check `hyprland.lua` has exactly the four Rofi workflow bindings; confirm `Super+Shift+T` and `SUPER + SHIFT + L` are byte-for-byte unchanged.
5. Parse/check the Waybar JSONC configuration with its available validation command, and confirm no `wlogout`, `wofi`, `nwg-drawer`, or `nwg-dock-hyprland` references remain outside intentionally historical documentation.
6. Confirm all listed legacy directories and MoonArch `wofi.css` fragments are absent, all bundles now contain `rofi.rasi`, and selector bundle validation succeeds.
7. Confirm `git diff -- cli/` is empty.

### Behavioral checks

1. Invoke each launcher command and verify drun filtering, window title/class filtering, and run execution.
2. From `Super+R`, use Rofi's mode switcher to reach `calc` when the optional plugin is installed; repeat without it and verify run still opens.
3. Verify the custom powermenu displays exactly five labels and outputs only identifiers.
4. Stub `hyprlock`, `systemctl`, and `hyprctl` through `PATH` in script tests: lock/sleep run immediately; logout/reboot/shutdown run only after `confirm`; cancel, Escape/nonzero Rofi exit, empty input, malformed input, and missing executables produce no destructive command.
5. Temporarily remove or corrupt `current/rofi.rasi`; verify all modes open with Tokyo Night fallback. Restore a valid fragment and verify its colors override fallback values.
6. Select a non-Tokyo MoonArch theme via the preserved `Super+Shift+T` selector and confirm it uses Rofi, swaps `current` atomically, and reloads existing Hyprland/Waybar consumers.

## Rollout and rollback

1. Install/document `rofi-wayland` and, if calculator is expected, the matching calc plugin before reloading Hyprland.
2. Deploy the Rofi files, MoonArch fragments, and selector update before removing Wofi so `Super+Shift+T` remains functional throughout the transition.
3. Reload Hyprland, restart/reload Waybar, and run the behavioral checks before deleting legacy directories in the same reviewed change.
4. Roll back by reverting the change as one unit. This restores legacy configuration, `wofi.css` bundle requirements, selector invocation, Waybar commands, and the original `nwg-drawer` `menu` declaration. Reload Hyprland or restart the session after rollback.

## Risks and constraints

- Rofi calc packaging differs by Arch package source; the documented plugin name must be verified against the chosen `rofi-wayland` package during apply. This does not block run mode.
- The existing MoonArch selector is a hidden Wofi dependency. Migrating its minimal input/output contract is required to meet both legacy removal and binding-preservation requirements.
- Theme parsing is isolated in `scripts/launch`; a bad fragment can reduce visual fidelity only, never launch functionality.
- This change intentionally does not alter `cli/` package installation lists. Package cleanup remains the planned follow-up.
