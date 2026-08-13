# Config Release Resolution Specification

## Purpose

Resolve, admit, and retain exact configuration artifacts safely for online application and offline recovery.

## Requirements

### Requirement: Exact Configuration Release Resolution

The system MUST resolve only an explicitly requested stable `config-vMAJOR.MINOR.PATCH` GitHub release and MUST bind the result to its exact tag, manifest identity, artifact bytes, and published digest. It MUST NOT substitute a latest release, channel, pin, prerelease, CLI release, historical `v0.1.0..v0.3.0` tag, or nearby version.

#### Scenario: Exact release exists

- GIVEN `config-v1.2.3` exists with the required artifact and integrity metadata
- WHEN that exact identity is requested
- THEN only the assets bound to `config-v1.2.3` MUST be resolved

#### Scenario: Non-exact selector is rejected

- GIVEN a request names `latest`, a channel, a prerelease, or a legacy `v*` tag
- WHEN resolution begins
- THEN the request MUST fail before artifact admission

#### Scenario: Missing exact release has no fallback

- GIVEN `config-v1.2.3` does not exist but another configuration release does
- WHEN `config-v1.2.3` is requested
- THEN resolution MUST fail without selecting the other release

### Requirement: Artifact Admission Fails Closed

An artifact MUST remain unavailable for planning until its published integrity metadata, whole-artifact digest, manifest, complete catalog, and every declared entry digest are verified. Admission MUST reject absolute or traversal paths, duplicate normalized paths, links escaping the artifact root, unsupported object types, and undeclared, missing, truncated, or digest-mismatched content. Rejection MUST NOT promote cache content or mutate managed targets.

#### Scenario: Valid artifact is admitted

- GIVEN a complete artifact whose paths, types, manifest, catalog, and digests are valid
- WHEN admission completes
- THEN the verified artifact MUST become available under its digest

#### Scenario: Malformed or untrusted artifact is rejected

- GIVEN an artifact has unsafe paths or links, unsupported content, missing integrity metadata, or any digest mismatch
- WHEN admission runs
- THEN the artifact MUST be rejected before planning
- AND all previously admitted cache entries MUST remain unchanged

### Requirement: Compatibility and External Dependencies Are Verified

Before an artifact is eligible for planning, the system MUST verify its manifest schema and CLI compatibility range and MUST check every declared external dependency. Missing, incompatible, or unknown requirements MUST fail closed. Dependency checks MUST NOT install, update, remove, or claim rollback of packages, services, or other external state.

#### Scenario: Compatibility checks pass

- GIVEN the CLI supports the manifest and every declared external dependency satisfies its constraint
- WHEN compatibility verification runs
- THEN the artifact MUST be eligible for planning
- AND external state MUST remain unchanged

#### Scenario: Compatibility check fails

- GIVEN the schema or CLI range is unsupported, or a declared external dependency is missing or incompatible
- WHEN compatibility verification runs
- THEN planning MUST NOT begin
- AND the failure MUST identify each unsatisfied requirement

### Requirement: Current and Previous Artifacts Remain Available Offline

Only admitted artifacts MAY enter the immutable digest-addressed cache, and promotion MUST NOT expose partial content. Failed or interrupted acquisition MUST leave admitted entries intact. The artifacts referenced by installed current and previous identities MUST be protected from cleanup, and either retained identity MUST be usable without network access. Other unreferenced artifacts MAY be removed.

#### Scenario: Interrupted acquisition preserves cache

- GIVEN verified current and previous artifacts are cached
- WHEN acquisition of another artifact is interrupted before promotion
- THEN neither protected artifact nor its cache identity MUST change

#### Scenario: Current and previous work offline

- GIVEN current and previous identities reference verified retained artifacts
- WHEN the network and GitHub are unavailable
- THEN either artifact MUST remain available for exact application or rollback

#### Scenario: Cleanup preserves protected artifacts

- GIVEN current, previous, and unreferenced verified artifacts are cached
- WHEN retention cleanup runs
- THEN current and previous artifacts MUST remain intact
- AND unreferenced artifacts MAY be removed
