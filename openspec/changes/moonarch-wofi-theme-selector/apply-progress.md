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

## TDD Cycle Evidence — PR #2 (Strict TDD)

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|---|---|---|
| 2.1 | `cli/cmd/install_test.go`, `cli/pkg/installer/system_test.go` | Unit + filesystem integration | ✅ `go test ./cmd ./pkg/installer/...` passed before edits | ✅ `go test ./cmd ./pkg/installer -run 'Test(InstallDiscovererPlansMoonArchRuntimeTrees|ManagedTargetsIncludeMoonArchRuntimeTrees|RootCommandDoesNotExposeDirectCopyTheme)$' -count=1` failed: MoonArch targets missing | ✅ Target tests passed after planning changes | ✅ Covers independent `bin` and `themes` discovery plus clean-HOME relative-link preservation | ✅ Focused packages passed after `gofmt` |
| 2.2 | `cli/cmd/install_test.go`, `cli/pkg/installer/system_test.go` | Unit | ✅ Same baseline | ✅ Existing target expectations failed before production changes | ✅ `go test ./cmd ./pkg/installer/... -count=1` passed | ✅ `bin` is discovered by `installDiscoverer`; `themes` is catalog-managed and both are checked in the composed discovery result | ➖ None needed — smallest clear target declarations |
| 2.3 | `cli/cmd/install_test.go` | Unit | ✅ Same baseline | ✅ `TestRootCommandDoesNotExposeDirectCopyTheme` failed while `theme.go` registered the command | ✅ Test passed after deleting the command/package | ➖ Structural removal: one externally visible command must be absent | ✅ Removed unused imports with the package deletion; focused packages passed |
| 2.4 | `cli/cmd/install_test.go`, `cli/pkg/installer/system_test.go` | Unit + filesystem integration | ✅ Same baseline | ✅ Covered by the prior RED checks | ✅ `go test ./cmd ./pkg/installer/... -count=1` passed | ✅ Re-ran all installer subpackages, including discovery failure and transaction coverage | ✅ `gofmt`; focused packages, full suite, vet, and build all pass |

## Work Unit Evidence — PR #2

| Evidence | Result |
|---|---|
| Focused test command and exact result | `cd cli && go test ./cmd ./pkg/installer/... -count=1` → exit 0; `cmd`, `installer`, `external`, `plan`, `report`, `transaction`, and `ui` passed. |
| Runtime harness command/scenario and exact result | `cd cli && go test ./pkg/installer -run TestCleanHomeCopyTreePreservesRelativeMoonArchCurrentLink -count=1` → exit 0; creates temporary repo/home, executes a real transaction, and asserts `readlink ~/.local/moonarch/themes/current == tokyo-night`. |
| Rollback boundary | Revert `cli/cmd/install.go`, `cli/pkg/installer/catalog.go`, `cli/cmd/theme.go`, `cli/pkg/theme/`, and the two Go test files. This removes only PR #2 deployment/retirement behavior, leaving the runtime selector and Phase 3 Stow/docs work intact. |
