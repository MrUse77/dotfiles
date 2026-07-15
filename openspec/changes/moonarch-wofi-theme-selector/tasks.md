# Tasks: MoonArch Wofi Theme Selector

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 500–620 (PR #2: 180–240) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Draft tracker → PR 1 runtime → PR 2 deployment → PR 3 docs/verification |
| Delivery strategy | auto-forecast |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Feature Branch Chain

- Tracker branch: `feat/moonarch-wofi-theme-selector`, with a draft/no-merge tracker PR to `main`; merge only after child PRs integrate.
- PR #1 base = tracker branch; PR #2 base = PR #1 branch; PR #3 base = PR #2 branch. Retarget/rebase polluted child diffs.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Completed selector, bundle, and consumer imports | PR 1 → tracker branch | `bash tests/moonarch-theme-selector_test.sh` | Temp `MOONARCH_ROOT` plus fake Wofi/Hyprland/Waybar | `.local/moonarch/`, four consumer configs, shell test |
| 2 | Completed deployment and CLI retirement (180–240 lines) | PR 2 → PR 1 branch | `cd cli && go test ./cmd ./pkg/installer/...` | Clean temp HOME install; assert `readlink themes/current` | `cli/cmd/install.go`, `cli/pkg/installer/catalog.go`, retired CLI, Go tests |
| 3 | Normal-install documentation and final verification | PR 3 → PR 2 branch | `bash tests/moonarch-theme-selector_test.sh` | Clean-HOME CopyTree link scenario | `README.md` and verification-only task metadata |

## Phase 1: Runtime Contract and Selector

- [x] 1.1 RED: Create `tests/moonarch-theme-selector_test.sh` seams proving invalid IDs, missing/mismatched manifests/fragments, escaped symlinks, non-symlink/absolute `current`, and Wofi cancellation preserve `current`.
- [x] 1.2 RED: Extend that test with valid sorted Wofi selection, atomic relative-link replacement, fixed executable argv, missing-Waybar success, and reload-failure rollback.
- [x] 1.3 GREEN: Create `.local/moonarch/themes/tokyo-night/{manifest.toml,hyprland.conf,waybar.css,wofi.css,ghostty.conf}` and relative `.local/moonarch/themes/current -> tokyo-night`.
- [x] 1.4 GREEN: Create `.local/moonarch/bin/theme-selector` with contained regular-file validation, staged `mv -Tf` switch, fixed reload commands, and prior-link restoration.
- [x] 1.5 GREEN: Update `.config/{hypr/hyprland.conf,waybar/style.css,wofi/style.css,ghostty/config}` to import `current`; bind `Super+Shift+T`; keep Ghostty new-terminal-only.
- [x] 1.6 REFACTOR: Make the shell harness deterministic and assert no unsupported Wofi/Ghostty reload or mutable bundle write occurs.

## Phase 2: Transactional Deployment and Retirement

- [x] 2.1 RED: Add `cli/cmd/install_test.go` and `cli/pkg/installer/system_test.go` cases proving closed discovery includes MoonArch `bin` and `themes`, and CopyTree preserves `current -> tokyo-night` in a clean HOME.
- [x] 2.2 GREEN: Update `cli/cmd/install.go` and `cli/pkg/installer/catalog.go` to plan both MoonArch CopyTree targets before execution.
- [x] 2.3 GREEN: Delete `cli/cmd/theme.go` and `cli/pkg/theme/`; verify `dots` exposes no direct-copy `theme` command or installer UI selection.
- [x] 2.4 REFACTOR: Run focused Go tests after removing obsolete imports and keep all target discovery failures blocking execution.

## Phase 3: Documentation and Verification

- [ ] 3.1 DEFERRED: Stow assertion in `tests/moonarch-theme-selector_test.sh` for `--no-folding` and an isolated `current` link; excluded from MoonArch.
- [ ] 3.2 DEFERRED: `scripts/stow-dev.sh` and `.stow-local-ignore` changes; excluded from MoonArch.
- [ ] 3.3 Update `README.md` only with normal-install runtime selection, default-link, and rollback guidance; omit Stow instructions.
- [ ] 3.4 Verify `bash tests/moonarch-theme-selector_test.sh` and `cd cli && go test ./... && go vet ./... && go build ./...`; rerun the clean-HOME link scenario.
