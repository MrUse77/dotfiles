# Apply Progress: MoonArch Wofi Theme Selector

## Completed Tasks

- [x] 1.1 RED: Created deterministic shell seams for invalid IDs, malformed bundles, escaping symlinks, invalid `current`, and Wofi cancellation.
- [x] 1.2 RED: Added valid sorted selection, fixed command argv, missing-Waybar success, atomic-link, and reload-rollback coverage.
- [x] 1.3 GREEN: Added the immutable `tokyo-night` bundle and relative `current -> tokyo-night` link.
- [x] 1.4 GREEN: Added the validated, atomic Bash selector with rollback on reload failure.
- [x] 1.5 GREEN: Bound Hyprland and all four consumers to `themes/current`; Ghostty reads the fragment at terminal launch.
- [x] 1.6 REFACTOR: Kept the harness temp-rooted and deterministic; asserted no Wofi/Ghostty reload and no bundle-content mutation.

## TDD Sequence (Standard Mode)

The root config has `strict_tdd: false`; the Phase 1 tasks explicitly require RED → GREEN → REFACTOR and were performed in that order.

| Task | Evidence |
|---|---|
| 1.1–1.2 RED | `bash tests/moonarch-theme-selector_test.sh` exited 1 before the selector existed; the cancellation scenario exposed the missing executable after the rejection seams ran. |
| 1.3–1.5 GREEN | Added the bundle, selector, and consumer bindings; the focused harness passed all 12 scenarios. |
| 1.6 REFACTOR | Hardened the Waybar missing-process branch and manifest containment; reran the focused harness successfully. |

## Work Unit Evidence

| Evidence | Result |
|---|---|
| Focused test command and exact result | `bash tests/moonarch-theme-selector_test.sh` → exit 0; 12 PASS scenarios. |
| Runtime harness command/scenario and exact result | The same command uses temporary `MOONARCH_ROOT` trees plus fake `wofi`, `hyprctl`, `pgrep`, and `pkill`; exit 0 verifies cancellation, valid selection, missing Waybar, and both Hyprland and Waybar reload rollback. |
| Rollback boundary | Revert `.local/moonarch/`, the four consumer config imports/binding, and `tests/moonarch-theme-selector_test.sh`; no installer, Stow, CLI, or documentation behavior is removed. |

## PR Boundary

- Strategy: feature-branch-chain.
- Current slice: PR #1, `feat/moonarch-runtime-selector` → `feat/moonarch-wofi-theme-selector`.
- Included: standalone runtime selector, Tokyo Night bundle, `current` link, four consumer bindings, and deterministic shell coverage.
- Excluded: Phase 2 installer/CLI changes and Phase 3 Stow/documentation work.
