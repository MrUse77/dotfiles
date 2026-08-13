# MoonArch CLI Self-Update Specification

## Purpose

Update the MoonArch executable without coupling binary lifecycle to managed configuration.

## Requirements

### Requirement: Canonical Command and Alias Share One CLI-Only Contract

`moonarch self update` MUST be the canonical CLI self-update command. `moonarch update` MUST remain a backward-compatible alias with identical discovery, version comparison, verification, replacement, visible result, and exit outcome from equivalent state. Neither command MAY accept a configuration release selector or dispatch a configuration operation.

#### Scenario: Canonical command checks for a CLI update

- GIVEN a supported newer CLI release is available
- WHEN the user runs `moonarch self update`
- THEN the command MUST select the CLI release for the current architecture
- AND it MUST execute only the CLI update contract

#### Scenario: Legacy alias is equivalent

- GIVEN equivalent installed binaries and release responses
- WHEN `moonarch self update` and `moonarch update` run independently
- THEN both MUST select the same CLI release and produce the same outcome
- AND neither command MUST enter a configuration path

#### Scenario: Configuration selector is rejected

- GIVEN either update command is passed `config-v1.2.3`
- WHEN command arguments are validated
- THEN the command MUST reject the selector without binary or configuration mutation

### Requirement: New Binaries Are Verified Before Atomic Replacement

The updater MUST discover the latest compatible CLI release and compare it with the installed version. For a newer release, it MUST select the architecture binary and published checksum, verify it before replacement, and preserve the existing executable until replacement succeeds. The replacement MUST become active on the next invocation without re-executing the current process.

#### Scenario: Verified newer binary replaces the current binary

- GIVEN a newer compatible CLI binary matches its published checksum
- WHEN self-update completes
- THEN the verified binary MUST atomically replace the current executable
- AND it MUST be reported as active on the next invocation

#### Scenario: Installed version is already current

- GIVEN the installed and discovered CLI versions are equal
- WHEN either update command runs
- THEN it MUST report that the CLI is already current
- AND it MUST NOT download or replace binary assets

#### Scenario: Release discovery fails

- GIVEN CLI release discovery returns an error or unusable metadata
- WHEN either update command runs
- THEN it MUST fail with the existing executable unchanged
- AND no managed configuration MUST change

#### Scenario: Candidate verification fails

- GIVEN a downloaded candidate does not match its published checksum
- WHEN verification runs
- THEN replacement MUST NOT occur and the existing executable MUST remain usable
- AND no managed configuration MUST change

#### Scenario: Binary replacement fails

- GIVEN a verified candidate cannot replace the executable
- WHEN replacement is attempted
- THEN the command MUST fail with the prior executable intact
- AND no managed configuration MUST change

### Requirement: Self-Update Is Configuration-Neutral

Both update commands MUST NOT acquire or move a configuration checkout, plan or apply managed targets, or mutate configuration state, cache, lock, journal, or inventories. They MUST NOT honor an environment-variable escape hatch such as `MOONARCH_FORCE_REPO`. Only exact `moonarch config apply config-vX.Y.Z` MAY initiate configuration release mutation.

#### Scenario: Successful update preserves configuration state

- GIVEN managed files, release state, cache, and inventories have recorded values
- WHEN either update command succeeds
- THEN every recorded configuration value and checkout location MUST remain unchanged

#### Scenario: Force environment variable cannot enable configuration stages

- GIVEN `MOONARCH_FORCE_REPO` or another force variable is set
- WHEN either update command runs
- THEN no checkout acquisition, configuration planning, or configuration mutation MUST occur

#### Scenario: Configuration change requires exact apply

- GIVEN the user wants configuration release `config-v1.2.3`
- WHEN the user invokes an update command instead of `moonarch config apply config-v1.2.3`
- THEN that configuration release MUST NOT be resolved or applied
