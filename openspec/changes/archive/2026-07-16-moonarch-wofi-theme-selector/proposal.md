# Proposal: MoonArch Wofi Theme Selector

## Intent

Provide a standalone MoonArch runtime selector so users can safely switch a coherent Hyprland, Waybar, Wofi, and Ghostty theme without mutating managed configuration. Retire the obsolete direct-copy `dots theme` command; `dots install` remains installer-only.

## Proposal Question Round

The approved exploration and product decisions resolve the initial product questions. No unresolved business rule blocks this bounded first slice.

## Scope

### In Scope
- Versioned `tokyo-night` bundle and a default relative `themes/current` link.
- Wofi name-only selector that validates a bundle, atomically switches `current`, reloads supported consumers, and restores the prior link on failed reload.
- Hyprland, Waybar, Wofi, and Ghostty theme-fragment consumption; Ghostty applies on newly launched terminals.
- Transactional installer deployment of MoonArch `bin` and `themes` for normal user installation.
- Headless safety/default-install tests and runtime-contract documentation.

### Out of Scope
- Wallpaper, Wofi visual previews, GTK, Qt, Yazi, multi-theme bundles, and a general theme engine.
- Extending `dots theme` or making runtime selection part of the installer UI.
- GNU Stow development integration; defer it to a dedicated follow-up change.

## Capabilities

### New Capabilities
- `moonarch-theme-selector`: Validated MoonArch bundles and atomic runtime theme selection for four consumers.

### Modified Capabilities
- `installation-transaction`: Managed-target discovery includes MoonArch runtime `bin` and `themes` trees, preserving the default relative link during transactional deployment.

## Approach

Keep immutable bundles in `~/.local/moonarch/themes/<id>` and switch only `current`. The selector rejects invalid IDs, missing/incorrect manifests or required fragments, and paths escaping the themes root before any mutation. It stages and atomically replaces the link, retaining/restoring the prior link when a reload fails.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `.local/moonarch/` | New | Selector, Tokyo Night bundle, default link |
| `.config/{hypr,waybar,wofi,ghostty}/` | Modified | Consume selected fragments; Hyprland binding |
| `cli/cmd/{theme.go,install.go}` | Modified/Removed | Retire placeholder; deploy runtime trees |
| `cli/pkg/installer/catalog.go` | Modified | Register themes target |
| `tests/`, `README.md` | Modified | Runtime-contract tests and documentation |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Unsafe bundle path | Med | Validate identity, files, and resolved containment before switch |
| Broken live reload | Med | Restore prior link; test filesystem contract and verify graphically |

## Rollback Plan

Restore the prior `current` link (or remove the new runtime tree), revert consumer imports/binding and installer targets, then reinstall the previous configuration transactionally.

## Dependencies

- Hyprland, Waybar, Wofi, Ghostty, and the existing installer transaction.

## Success Criteria

- [ ] Fresh installation activates Tokyo Night through a valid relative `current` link.
- [ ] Invalid or unsafe bundles leave the active theme unchanged.
- [ ] Wofi switches only valid bundle names across all four supported consumers.
