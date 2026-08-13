# Delta for Backup Inventory

## ADDED Requirements

### Requirement: Release Provenance Is Additive

Every backup inventory created by configuration apply or rollback MUST record the operation's exact release tag and verified artifact digest as release provenance without changing target pre-state, backup, status, or restore semantics. Inventory readers and restore operations MUST accept historical inventories that lack release provenance, treating the identity as unknown rather than invalid. Release provenance MUST NOT restrict restoration of an otherwise valid historical run.

#### Scenario: Apply inventory records verified provenance

- GIVEN an exact verified artifact is applied or rolled back
- WHEN its backup inventory becomes authoritative
- THEN the inventory MUST contain that artifact's exact tag and verified digest

#### Scenario: Legacy inventory remains readable

- GIVEN a valid historical inventory has no release provenance
- WHEN the inventory is decoded
- THEN decoding MUST succeed with release identity reported as unknown
- AND every existing target entry MUST remain available

#### Scenario: Historical run restores across release versions

- GIVEN a valid historical inventory predates release provenance and a newer release is installed
- WHEN the user restores a target from that run
- THEN restoration MUST use the recorded target backup and pre-state
- AND missing release provenance MUST NOT block the restore
