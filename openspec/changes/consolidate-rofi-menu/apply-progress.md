## Apply Progress: consolidate-rofi-menu

### Status
- Phase: sdd-apply
- All 11 tasks completed
- Persisted checkbox updates: T1-T11 marked `[x]` in `openspec/changes/consolidate-rofi-menu/tasks.md`
- Size exception: APPROVED (single PR ~650-750 lines)

### Files created
- `.config/rofi/config.rasi`
- `.config/rofi/theme.rasi`
- `.config/rofi/scripts/powermenu` (executable)
- `.config/rofi/scripts/launch` (executable)
- `.local/share/moonarch/themes/{catppuccin-latte,catppuccin-mocha,decay-green,edge-runner,frosted-glass,graphite-mono,gruvbox-retro,material-sakura,nordic-blue,rose-pine,synth-wave,tokyo-night}/rofi.rasi`

### Files modified
- `.local/bin/moonarch/theme-selector` — required file `rofi.rasi`, Rofi dmenu selector
- `.config/hypr/hyprland.lua` — replaced nwg-drawer with Rofi variables; added `Super+Tab`, `Super+R`, `Super+Shift+X` bindings; preserved `Super+Shift+T` and `SUPER+SHIFT+L`
- `.config/waybar/config.jsonc` — power button uses Rofi powermenu; launcher uses Rofi drun
- `README.md` — Rofi prerequisites, shortcuts, directory tree, legacy removal note

### Files deleted
- `.config/wofi/` (config, style.css)
- `.config/nwg-drawer/` (drawer.css)
- `.config/nwg-dock-hyprland/` (style.css)
- `.local/share/moonarch/themes/*/wofi.css` (12 bundles)

### Verification run
- `bash -n` passed for all shell scripts: `powermenu`, `launch`, `theme-selector`
- `rofi -dump-theme` validated `.config/rofi/theme.rasi`, `.config/rofi/config.rasi`, and all 12 `rofi.rasi` fragments
- All 12 `rofi.rasi` fragments contain all 6 `moonarch-*` variables
- Temporary HOME test: valid MoonArch fragment overrides Tokyo Night fallback; invalid fragment triggers base-theme fallback
- `hyprland.lua`: no `nwg-drawer`, `wofi`, or `nwg-dock` references remain; `Super+Shift+T` and `SUPER+SHIFT+L` unchanged
- `waybar/config.jsonc`: no `wlogout` or `nwg-drawer` references remain
- `grep -R` found no `wofi`/`nwg-drawer`/`nwg-dock` references in `.config/` or `.local/`
- `git diff -- cli/` is empty
- `README.md` mentions `rofi-wayland`, optional calculator dependency, all four shortcuts, and states legacy configs were removed

### Notes
- `rofi -dump-theme` exits 0 even for invalid themes, so the wrapper validates by checking that the dumped output contains `moonarch-background` before accepting the composite.
- `scripts/launch` uses `mktemp`, `mv` atomic rename, 0600 permissions, and `$XDG_RUNTIME_DIR` with `/tmp` fallback.
- `scripts/powermenu` uses fixed action identifiers, no `eval`, `set -u`, and `notify-send` fallback on missing commands.
- No files under `cli/` were modified.

### Next step
- `sdd-verify` to validate acceptance criteria and behavioral checks
