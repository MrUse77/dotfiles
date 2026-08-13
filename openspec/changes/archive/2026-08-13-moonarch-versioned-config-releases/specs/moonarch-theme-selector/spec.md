# Delta for MoonArch Theme Selector

## ADDED Requirements

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
