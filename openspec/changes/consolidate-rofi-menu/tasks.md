# Tasks: Consolidate Desktop Menus into Rofi

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~650-750 (additions + deletions) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Single PR with size exception OR 3-PR chain |
| Delivery strategy | single-pr-default (requires size:exception) |
| Chain strategy | size-exception |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: size-exception
400-line budget risk: High
```

**Rationale for size exception:** While the line count exceeds 400, the change is configuration-heavy (low cognitive load), mostly additive (new config files), and well-specified in design. The actual review burden is manageable as a single PR. If the reviewer finds it overwhelming, split into: (1) Rofi core + Hyprland/Waybar integration, (2) MoonArch theme migration, (3) legacy cleanup + documentation.

---

## Dependency Graph

```
T1 (Rofi config.rasi) ─┐
T2 (Rofi theme.rasi) ──┼─→ T5 (scripts/launch) ─→ T7 (hyprland.lua) ─┐
T3 (Rofi powermenu) ───┘                        T8 (waybar) ──────────┼─→ T11 (legacy removal)
                       T4 (MoonArch bundles) ──→ T6 (theme-selector) ─┤
                                                                       T9 (README) ──────────┘
```

**Critical path:** T1 → T5 → T7/T8 → T11

**Parallel opportunities:** T1, T2, T3, T4 can all start in parallel. T6 depends on T4. T9 can start after T7/T8.

---

## Task List

### Batch 1: Rofi Core Configuration (Foundation)

- [x] **T1: Create Rofi config.rasi** — ~60 lines
- **File:** `.config/rofi/config.rasi`
- **Description:** Create global Rofi behavior configuration with modes (drun, window, run, calc, powermenu), Hack Nerd Font 15, fuzzy matching, icon settings, sidebar mode, and mode labels. Set `@theme` to fallback theme path.
- **Acceptance criteria:** 1, 2, 3, 4, 5, 13, 16
- **Verification:** `rofi -config ~/.config/rofi/config.rasi -show drun` opens without errors

- [x] **T2: Create Rofi theme.rasi** — ~180 lines
- **File:** `.config/rofi/theme.rasi`
- **Description:** Create complete Tokyo Night fallback theme with all widget styling (window, mainbox, inputbar, prompt, entry, element states), Hack Nerd Font, 560px width, 12px rounding, 2px accent border, restrained opacity. Define `moonarch-*` variables with Tokyo Night defaults.
- **Acceptance criteria:** 13, 15, 16
- **Verification:** `rofi -theme ~/.config/rofi/theme.rasi -show drun` displays Tokyo Night colors

- [x] **T3: Create Rofi powermenu script** — ~140 lines
- **File:** `.config/rofi/scripts/powermenu` (executable)
- **Description:** Create Bash script implementing Rofi script-mode protocol. Emit 5 actions (lock, sleep, logout, reboot, shutdown) with Nerd Font icons via `ROFI_RETV=0`. On selection (`ROFI_RETV=1`), dispatch fixed commands with confirmation for logout/reboot/shutdown. Use `scripts/launch -dmenu` for confirmation. Cancel-first ordering. Handle errors with `notify-send` fallback, no eval/shell interpolation.
- **Acceptance criteria:** 5, 6, 7, 8, 9, 10, 11, 12
- **Verification:** `bash -n ~/.config/rofi/scripts/powermenu` passes syntax check; stub test with mock commands confirms lock/sleep immediate, logout/reboot/shutdown require confirmation, cancel is safe

- [x] **T4: Create MoonArch rofi.rasi fragments** — ~120 lines (12 files × ~10 lines each)
- **Files:** `.local/share/moonarch/themes/{catppuccin-latte,catppuccin-mocha,decay-green,edge-runner,frosted-glass,graphite-mono,gruvbox-retro,material-sakura,nordic-blue,rose-pine,synth-wave,tokyo-night}/rofi.rasi`
- **Description:** For each bundle, convert existing `wofi.css` color variables to `rofi.rasi` format using `moonarch-*` variable names. Map `wofi_background` → `moonarch-background`, `wofi_surface` → `moonarch-surface`, `wofi_foreground` → `moonarch-foreground`, `wofi_accent` → `moonarch-accent`, add `moonarch-urgent` and `moonarch-selected-text` with contrast-safe values.
- **Acceptance criteria:** 14, 17 (bundle requirement)
- **Verification:** Each `rofi.rasi` file contains valid Rasi syntax with all 6 `moonarch-*` variables

- [x] **T5: Create Rofi launch wrapper script** — ~90 lines
- **File:** `.config/rofi/scripts/launch` (executable)
- **Description:** Create Bash script that validates MoonArch current-theme fragment (`~/.local/share/moonarch/themes/current/rofi.rasi`), atomically composites it with fallback theme in `$XDG_RUNTIME_DIR`, validates with `rofi -dump-theme`, caches by mtime:size signature. On validation failure, falls back to base theme. Exec `rofi -theme <chosen> "$@"`. Use `mktemp`, 0600 permissions, quote all paths.
- **Acceptance criteria:** 14, 15, 17 (fallback behavior)
- **Verification:** `bash -n ~/.config/rofi/scripts/launch` passes; remove/corrupt `current/rofi.rasi` and verify all modes still open with Tokyo Night fallback; restore valid fragment and verify its colors override fallback

### Batch 2: Integration (Depends on Batch 1)

- [x] **T6: Update MoonArch theme-selector** — ~15 lines changed
- **File:** `.local/bin/moonarch/theme-selector`
- **Description:** Change `required_files` array from `wofi.css` to `rofi.rasi`. Replace `wofi --show dmenu` with `"$HOME/.config/rofi/scripts/launch" -dmenu -p "Theme" -no-custom`. Preserve `Super+Shift+T` binding path.
- **Acceptance criteria:** 17 (selector validation), 19 (binding preserved)
- **Verification:** `bash -n ~/.local/bin/moonarch/theme-selector` passes; selector validates bundles with `rofi.rasi`; selector invocation uses Rofi dmenu

- [x] **T7: Update Hyprland keybindings** — ~20 lines changed
- **File:** `.config/hypr/hyprland.lua`
- **Description:** Replace `local menu = "nwg-drawer -c 6 -is 64 -ovl -nofs"` with Rofi declarations: `local rofi = os.getenv("HOME") .. "/.config/rofi/scripts/launch"`, `local menu = rofi .. " -show drun"`, `local windowMenu = rofi .. " -show window"`, `local runMenu = rofi .. " -show run"`, `local powerMenu = rofi .. " -show powermenu"`. Add bindings: `Super+M` (existing, now uses new `menu`), `Super+Tab` (windowMenu), `Super+R` (runMenu), `Super+Shift+X` (powerMenu). Do not alter `Super+Shift+T` or `SUPER+SHIFT+L`.
- **Acceptance criteria:** 1, 2, 3, 5, 18, 19, 20
- **Verification:** File contains exactly 4 Rofi workflow bindings; no `nwg-drawer`, `wofi`, or `nwg-dock` references remain; `Super+Shift+T` and `SUPER+SHIFT+L` are byte-for-byte unchanged

- [x] **T8: Update Waybar configuration** — ~5 lines changed
- **File:** `.config/waybar/config.jsonc`
- **Description:** In `custom/power` module, change `"on-click": "wlogout"` to `"on-click": "$HOME/.config/rofi/scripts/launch -show powermenu"`. In `custom/launcher` module, change `"on-click": "nwg-drawer -c 6 -is 64 -ovl -nofs"` to `"on-click": "$HOME/.config/rofi/scripts/launch -show drun"`.
- **Acceptance criteria:** 23
- **Verification:** No `wlogout` or `nwg-drawer` references remain in Waybar config

### Batch 3: Cleanup and Documentation (Depends on Batch 2)

- [x] **T9: Update README.md** — ~40 lines changed
- **File:** `README.md`
- **Description:** Replace Wofi references with Rofi. Add `rofi-wayland` and Hack Nerd Font as required runtime prerequisites. Document optional calculator plugin/package (`rofi-calc-wayland` or equivalent, `libqalculate`). Document the four shortcuts: `Super+M` (apps), `Super+Tab` (windows), `Super+R` (commands/calculator), `Super+Shift+X` (powermenu). State that Wofi/nwg-drawer/nwg-dock configuration was removed. Update directory tree to show `.config/rofi/` instead of legacy directories.
- **Acceptance criteria:** 21, 22
- **Verification:** README mentions `rofi-wayland` as prerequisite; mentions optional calculator dependency; documents all four shortcuts; states legacy config removed

- [x] **T10: Remove legacy configuration directories** — ~75 lines deleted
- **Files:** `.config/wofi/` (entire directory), `.config/nwg-drawer/` (entire directory), `.config/nwg-dock-hyprland/` (entire directory)
- **Description:** Delete all three legacy configuration directories and their tracked files. This includes `.config/wofi/config`, `.config/wofi/style.css`, `.config/nwg-drawer/drawer.css`, `.config/nwg-dock-hyprland/style.css`.
- **Acceptance criteria:** 17 (legacy removal)
- **Verification:** Directories do not exist after deletion; no references to these tools in active configuration files

- [x] **T11: Remove legacy MoonArch wofi.css files** — ~12 files deleted
- **Files:** `.local/share/moonarch/themes/*/wofi.css` (all 12 bundles)
- **Description:** Delete the old `wofi.css` file from each MoonArch theme bundle. Each bundle now contains `rofi.rasi` instead (created in T4).
- **Acceptance criteria:** 17 (bundle migration)
- **Verification:** No `wofi.css` files exist in any bundle; all bundles contain `rofi.rasi`

---

## Batch Grouping for sdd-apply

### Batch 1: Rofi Core (Foundation)
**Tasks:** T1, T2, T3, T4
**Estimated lines:** ~490 additions
**Rationale:** These are all new files with no dependencies on existing code changes. They can be reviewed as a cohesive "Rofi configuration" unit. The theme fragments (T4) are repetitive but straightforward.

### Batch 2: Integration (Wiring)
**Tasks:** T5, T6, T7, T8
**Estimated lines:** ~130 additions + ~40 changes
**Rationale:** These integrate Rofi into the existing system. T5 depends on T1/T2 (theme paths), T6 depends on T4 (bundle validation), T7/T8 depend on T5 (script paths). These are surgical changes to existing files.

### Batch 3: Cleanup and Documentation
**Tasks:** T9, T10, T11
**Estimated lines:** ~40 changes + ~190 deletions
**Rationale:** These remove legacy configuration and document the migration. They should come last to avoid breaking the theme selector (T6) before it's updated to use Rofi.

---

## Execution Notes

**Strict TDD considerations:**
- Shell scripts (T3, T5, T6) should have syntax validation (`bash -n`) run immediately after creation
- Rofi config files (T1, T2) should be validated with `rofi -dump-theme` immediately after creation
- Hyprland and Waybar configs should be validated after changes (if validation tools are available)

**Testing strategy:**
- Static checks: `bash -n` on all shell scripts, `rofi -dump-theme` on theme files
- Behavioral checks: Invoke each launcher mode, verify powermenu actions with stubbed commands
- Integration checks: Verify MoonArch theme switching works end-to-end
- Regression checks: Verify `Super+Shift+T` and `SUPER+SHIFT+L` remain unchanged

**Rollback path:**
- Single Git revert restores all legacy configuration, MoonArch bundles, selector, and keybindings
- No data migration or persistent state conversion required

**Risk mitigation:**
- Theme fallback is isolated in T5 (launch wrapper); bad MoonArch fragments cannot break launcher
- Powermenu uses fixed action identifiers, not user-typed commands
- Confirmation flow prevents accidental destructive actions
- All shell scripts use `set -u`, explicit error handling, no `eval`
