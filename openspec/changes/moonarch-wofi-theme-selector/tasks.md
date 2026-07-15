# Tasks: MoonArch Wofi Theme Selector

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 550–700 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | Draft tracker → PR 1 runtime selector → PR 2 installer deployment |
| Delivery strategy | auto-forecast |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Feature Branch Chain

- Tracker branch: `feat/moonarch-wofi-theme-selector`, with a draft/no-merge tracker PR to `main`; merge only after both child PRs integrate.
- PR #1 base = tracker branch; PR #2 base = PR #1 branch. Retarget/rebase any child whose diff includes an earlier unit.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Validated selector, bundle, and consumer imports | PR 1 → tracker branch | `bash tests/moonarch-theme-selector_test.sh` | Temp `MOONARCH_ROOT` plus fake Wofi/Hyprland/Waybar | `.local/moonarch/`, four consumer configs, shell test |
| 2 | Transactional installation and retired CLI removal | PR 2 → PR 1 branch | `cd cli && go test ./cmd ./pkg/installer/...` | Clean temp HOME install; assert `readlink themes/current` | `cli/cmd/theme.go`, `cli/pkg/theme/`, installer targets/tests, docs |

## Phase 1: Runtime Contract and Selector

- [x] 1.1 RED: Create `tests/moonarch-theme-selector_test.sh` seams proving invalid IDs, missing/mismatched manifests/fragments, escaped symlinks, non-symlink/absolute `current`, and Wofi cancellation preserve `current`.
- [x] 1.2 RED: Extend that test with valid sorted Wofi selection, atomic relative-link replacement, fixed executable argv, missing-Waybar success, and reload-failure rollback.
- [x] 1.3 GREEN: Create `.local/moonarch/themes/tokyo-night/{manifest.toml,hyprland.conf,waybar.css,wofi.css,ghostty.conf}` and relative `.local/moonarch/themes/current -> tokyo-night`.
- [x] 1.4 GREEN: Create `.local/moonarch/bin/theme-selector` with contained regular-file validation, staged `mv -Tf` switch, fixed reload commands, and prior-link restoration.
- [x] 1.5 GREEN: Update `.config/{hypr/hyprland.conf,waybar/style.css,wofi/style.css,ghostty/config}` to import `current`; bind `Super+Shift+T`; keep Ghostty new-terminal-only.
- [x] 1.6 REFACTOR: Make the shell harness deterministic and assert no unsupported Wofi/Ghostty reload or mutable bundle write occurs.

## Phase 2: Transactional Deployment and Retirement

- [ ] 2.1 RED: Add `cli/cmd/install_test.go` and `cli/pkg/installer/system_test.go` cases proving closed discovery includes MoonArch `bin` and `themes`, and CopyTree preserves `current -> tokyo-night` in a clean HOME.
- [ ] 2.2 GREEN: Update `cli/cmd/install.go` and `cli/pkg/installer/catalog.go` to plan both MoonArch CopyTree targets before execution.
- [ ] 2.3 GREEN: Delete `cli/cmd/theme.go` and `cli/pkg/theme/`; verify `dots` exposes no direct-copy `theme` command or installer UI selection.
- [ ] 2.4 REFACTOR: Run focused Go tests after removing obsolete imports and keep all target discovery failures blocking execution.

## Phase 3: Stow, Documentation, and Verification

- [ ] 3.1 RED: Add Stow/default-install assertions to `tests/moonarch-theme-selector_test.sh` for `--no-folding` and an isolated runtime `current` link.
- [ ] 3.2 GREEN: Update `scripts/stow-dev.sh`, `.stow-local-ignore`, and `README.md` with the bounded four-consumer contract and rollback behavior; exclude wallpaper and installer UI selection.
- [ ] 3.3 Verify `bash tests/moonarch-theme-selector_test.sh`, `cd cli && go test ./... && go vet ./... && go build ./...`, and `bash test.sh` where Docker is available.
