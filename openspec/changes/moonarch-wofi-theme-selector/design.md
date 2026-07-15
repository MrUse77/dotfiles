# Design: MoonArch Wofi Theme Selector

## Technical Approach

Install immutable MoonArch bundles under `~/.local/moonarch/themes/<id>` and make `current` the sole mutable runtime state. Consumer configs import `current`; the selector validates a name-only bundle, atomically changes the relative link, then refreshes consumers. This implements `moonarch-theme-selector` and the installation delta without restoring the retired `dots theme` path.

**Before/after:** consumer files contain fixed Tokyo Night values; afterwards they retain non-theme settings and import fragments from `current`. Installer deployment is a closed plan containing `bin` and `themes`.

## Architecture Decisions

| Option | Tradeoff | Decision |
|---|---|---|
| Copy values into consumer configs | Mutates consumer configuration | Reject; import immutable fragments only. |
| Arbitrary path selection | Traversal/symlink escape | Accept only validated IDs and relative `current` targets. |
| In-place `ln -sf` | Observable unlink/relink window | Stage sibling symlink, then `mv -Tf` atomically. |
| Go CLI selector | Couples runtime action to installer | Bash executable; remove placeholder Cobra/theme package. |

Rationale: existing config already imports `current`, and the Go transaction already provides CopyTree staging, source binding, backups, and rollback.

## Data Flow

```text
Hypr bind -> theme-selector -> Wofi (name only) -> validate bundle
                                                  -> stage current -> rename
                                                  -> hyprctl / Waybar
configs -> themes/current/{hyprland,waybar,wofi,ghostty} fragments
installer plan -> CopyTree(bin,themes including current -> tokyo-night)
```

## Interfaces / Contracts

`theme-selector [theme-id]` accepts zero or one ID matching `^[a-z0-9][a-z0-9-]*$`; zero displays sorted valid IDs in Wofi. A bundle is exactly `themes/<id>/` with `manifest.toml` (`id = "<id>"`) plus regular, resolved-contained `hyprland.conf`, `waybar.css`, `wofi.css`, and `ghostty.conf`.

Containment is exact: resolve `themes` once with `realpath -e`; reject unless the resolved candidate equals `$themes_real/$id`; resolve every required file and reject unless it is a regular file below `$themes_real/$id/`. Reject a non-symlink `current`, an absolute/invalid target, missing root, manifest mismatch, or any validation error before mutation.

Create `.$current.$$.${RANDOM}` in the resolved themes directory with `ln -s -- "$id"`, then `mv -Tf -- "$tmp" "$current"`. Save the prior relative ID first. On failed `hyprctl reload` or `pkill -SIGUSR2 waybar`, atomically restore that link (or remove `current` when absent), retry best-effort consumer refresh, and exit non-zero. Ghostty reloads only on a new terminal. Wofi is invocation-scoped: its selected process exits and the next invocation reads `wofi.css`; no unsupported signal is sent.

Command boundary: invoke fixed executables only (`wofi --show dmenu`, `hyprctl reload`, `pkill -SIGUSR2 waybar`) with quoted arguments and no `eval`, shell composition, user-controlled executable, or user-controlled command argument. Treat Wofi cancellation as no-op; treat a missing Waybar as non-fatal only after confirming no matching process, otherwise signal failure rolls back.

## File Changes

| File | Action | Description |
|---|---|---|
| `.local/moonarch/{bin/theme-selector,themes/current,themes/tokyo-night/*}` | Create | Selector, relative default link, manifest, four fragments. |
| `.config/{hypr/hyprland.conf,waybar/style.css,wofi/style.css,ghostty/config}` | Modify | Binding/imports to runtime fragments. |
| `cli/cmd/install.go`, `cli/pkg/installer/catalog.go` | Modify | Plan CopyTree targets for MoonArch bin/themes. |
| `cli/cmd/theme.go`, `cli/pkg/theme/` | Delete | Obsolete direct-copy selector. |
| `README.md` | Modify | Runtime selector contract. |
| `cli/cmd/install_test.go`, `cli/pkg/installer/system_test.go`, `tests/moonarch-theme-selector_test.sh` | Create/Modify | Headless deployment and selector seams. |

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Shell | ID, manifest, escaped symlink, cancellation, atomic switch/rollback | Temp `MOONARCH_ROOT`, fake `wofi`, `hyprctl`, `pgrep`/`pkill`; assert link and logged argv. |
| Go | Closed discovery and CopyTree link preservation | Temp repo/home; planner and transaction tests assert bin/themes and `readlink(current)==tokyo-night`. |
| Integration | Default installation | `bash tests/moonarch-theme-selector_test.sh`; `go test ./...`; `bash test.sh` where Docker is available. |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — no classifier executes repository documents | None | None |
| Git repository selection | N/A — selector/installer do not select Git repos | None | None |
| Commit state | N/A — no commit operation | None | None |
| Push state | N/A — no push operation | None | None |
| PR commands | N/A — no PR operation | None | None |

## Migration / Rollout

No data migration. Remove `dots theme`; deploy the default relative link with the transactional theme CopyTree. Roll back by restoring the previous `current` link or transaction backup, then reverting imports/binding.

## Open Questions

- [ ] None.
