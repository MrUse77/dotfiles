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

### Requirement: Configuration Apply Preserves Mutable Theme Selection

Configuration apply and rollback MUST treat `themes/current` as mutable user state and MUST NOT replace it as immutable release content. If the selected bundle is valid in the desired release, the same selection and relative-link form MUST be preserved. If it is unavailable or cannot be validated, the operation MUST fail before any managed mutation unless the user explicitly supplies a valid replacement; the system MUST NOT choose a fallback silently.

#### Scenario: Selected theme remains available

- GIVEN `themes/current` validly selects `tokyo-night` and the desired release contains that bundle
- WHEN configuration apply or rollback commits
- THEN `themes/current` MUST still select `tokyo-night` through a valid relative link

#### Scenario: Selected theme is unavailable

- GIVEN the desired release does not contain the currently selected bundle
- WHEN preflight runs without an explicit replacement
- THEN the operation MUST report the unavailable selection and abort
- AND no managed target or installed identity MUST change

#### Scenario: Explicit replacement permits apply

- GIVEN the selected bundle is unavailable and the user explicitly names another valid desired-release bundle
- WHEN preflight and the transaction succeed
- THEN `themes/current` MUST select that replacement after commit
- AND no other fallback MUST be chosen

#### Scenario: Current selection is unsafe or invalid

- GIVEN `themes/current` is broken, escaping, or does not identify a valid bundle
- WHEN configuration preflight validates mutable theme state
- THEN the operation MUST fail before managed mutation
- AND it MUST require an explicit valid replacement
