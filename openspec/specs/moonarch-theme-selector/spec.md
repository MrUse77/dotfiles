# MoonArch Theme Selector Specification

## Purpose

Provide safe runtime selection of one coherent MoonArch theme for Hyprland, Waybar, Wofi, and Ghostty.

## Requirements

### Requirement: Tokyo Night Is the Initial Valid Theme

The system MUST provide a versioned `tokyo-night` bundle under the MoonArch themes root. A fresh installation SHALL provide `themes/current` as a valid relative link to that bundle. A bundle MUST contain a valid identity/manifest and required fragments for all four supported consumers.

#### Scenario: Fresh installation selects Tokyo Night

- GIVEN MoonArch is installed into a previously unconfigured home
- WHEN installation completes successfully
- THEN `themes/current` SHALL resolve to the `tokyo-night` bundle
- AND all four consumer fragments SHALL be available

#### Scenario: Incomplete bundle is not valid

- GIVEN a candidate bundle lacks its manifest or a required consumer fragment
- WHEN the selector validates the bundle
- THEN it SHALL reject the bundle without changing `current`

### Requirement: Name-Only Validated Atomic Switching

The Wofi selector MUST accept only a bundle name, reject invalid identities and resolved paths outside the themes root, and MUST NOT mutate bundle contents. It SHALL atomically replace only `current` after validation, reload supported running consumers, and restore the prior link if a reload fails.

#### Scenario: Valid selection switches all consumers

- GIVEN a valid named bundle and an existing active theme
- WHEN the user selects that name in Wofi
- THEN `current` SHALL atomically point to the selected bundle
- AND Hyprland, Waybar, and Wofi SHALL reload; Ghostty SHALL apply on new terminals

#### Scenario: Unsafe or failed selection preserves active theme

- GIVEN an invalid name, escaping path, or reload failure
- WHEN selection is attempted
- THEN `current` SHALL remain or be restored to its prior link
- AND no partial or invalid bundle SHALL become active

### Requirement: Bounded Runtime Scope

The system SHALL support only Hyprland, Waybar, Wofi, and Ghostty fragments in this slice. It MUST NOT provide wallpaper, Wofi previews, GTK, Qt, Yazi, multi-theme bundles, a general theme engine, extensions to `dots theme`, or runtime selection in installer UI.

#### Scenario: Unsupported request is excluded

- GIVEN a user requests an excluded integration or installer-UI selection
- WHEN this capability is used or documented
- THEN no such behavior SHALL be offered
