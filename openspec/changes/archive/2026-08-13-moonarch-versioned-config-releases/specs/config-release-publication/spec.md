# Config Release Publication Specification

## Purpose

Define immutable, self-contained configuration releases without stranding legacy MoonArch clients.

## Requirements

### Requirement: Immutable Self-Contained Configuration Publication

Each stable `config-vX.Y.Z` release MUST identify one immutable, self-contained artifact. The artifact MUST include a schema-versioned manifest, complete managed-target catalog, CLI compatibility range, declared external dependencies, all configuration and asset payloads, materialized pinned submodule content, per-entry digests, and a whole-artifact digest. Consumption MUST require no repository checkout, Git or submodule operation, or payload download. Published bytes MUST NOT change for an existing release identity.

#### Scenario: Complete release is published

- GIVEN every catalog entry and pinned submodule is materialized and all digests agree
- WHEN `config-v1.2.3` is published
- THEN one self-contained artifact and its integrity metadata MUST be available under that exact identity
- AND the artifact MUST contain everything required for configuration planning

#### Scenario: Incomplete release is rejected

- GIVEN a required asset is absent, a submodule is unmaterialized, or a declared digest is inconsistent
- WHEN publication validation runs
- THEN the configuration release MUST NOT be published
- AND the failure MUST identify the incomplete or inconsistent content

#### Scenario: Existing identity cannot be replaced

- GIVEN `config-v1.2.3` already identifies published bytes with digest A
- WHEN publication proposes different bytes with digest B under the same identity
- THEN publication MUST reject the replacement
- AND the original release MUST remain authoritative

### Requirement: Legacy Client Bridge Gates Configuration Publication

Before a configuration release can affect repository-wide latest-release discovery, the system MUST verify a bridge that keeps legacy bootstrap and update clients resolving a supported `v*` CLI release and its expected assets. Legacy clients MUST NOT receive a configuration artifact as a CLI binary. Configuration publication MUST fail closed when that compatibility cannot be demonstrated.

#### Scenario: Legacy client remains functional

- GIVEN the bridge is active and a `config-v*` release is newer than the latest CLI release
- WHEN a legacy client performs its existing latest-release lookup
- THEN it MUST resolve a supported `v*` CLI release
- AND its expected binary and checksum assets MUST remain obtainable

#### Scenario: Missing bridge blocks publication

- GIVEN legacy latest-release lookup would resolve an incompatible configuration-only release
- WHEN configuration publication is attempted without a verified bridge
- THEN publication MUST be blocked
- AND existing legacy discovery behavior MUST remain unchanged
