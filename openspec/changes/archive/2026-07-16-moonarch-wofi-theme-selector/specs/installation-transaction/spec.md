# Delta for Installation Transaction

## MODIFIED Requirements

### Requirement: Managed-Target Discovery Completes Before Execution

The installer MUST discover and enumerate all managed targets during the planning phase, before execution begins. The discovered target set SHALL be the complete input to execution; no new targets SHALL be added during execution without re-planning. The set MUST include MoonArch runtime `bin` and `themes` trees, and deployment SHALL preserve the versioned relative `themes/current` link rather than dereferencing or replacing it with an absolute link.

(Previously: Discovery required a closed managed-target set but did not define MoonArch runtime trees or relative-link preservation.)

#### Scenario: Full target set established during planning

- GIVEN the installer is building the installation plan
- WHEN target discovery runs
- THEN the resulting target set SHALL include all files and directories the installer will manage
- AND the target set SHALL be passed to the execution engine as a closed set

#### Scenario: MoonArch runtime trees are planned

- GIVEN MoonArch runtime sources contain `bin`, `themes/tokyo-night`, and relative `themes/current`
- WHEN target discovery runs
- THEN the plan SHALL include the MoonArch `bin` and `themes` trees
- AND it SHALL retain `current` as a relative link to `tokyo-night`

#### Scenario: Target discovery failure blocks execution

- GIVEN target discovery encounters an error (missing source, permission denied)
- WHEN the planning phase runs
- THEN the installer SHALL transition to an error state
- AND execution SHALL NOT begin
