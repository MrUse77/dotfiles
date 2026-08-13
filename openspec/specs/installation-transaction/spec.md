# Installation Transaction Specification

## Purpose

Define the transactional execution of managed-target mutations: target discovery, safe file operations, execution ordering, drift detection, and automatic rollback on handled failure. This domain ensures that managed file mutations behave as a single recoverable transaction.

## Requirements

### Requirement: Managed-Target Discovery Completes Before Execution

The installer MUST close the plan, reconciling catalogs into creations/replacements/removals; no target SHALL be added without re-planning. MoonArch binaries/immutable bundles MUST be planned; mutable `themes/current` MUST remain outside replacement as a relative selection.

(Previously: Discovery omitted retired targets and included mutable theme selection.)

#### Scenario: Full target set established during planning

- GIVEN installed A and B and desired B and C
- WHEN discovery runs
- THEN the closed plan SHALL contain removal A, replacement B, and creation C

#### Scenario: MoonArch runtime trees are planned

- GIVEN binaries, theme bundles, and relative `themes/current`
- WHEN discovery runs
- THEN binaries/bundles SHALL be planned and `themes/current` excluded

#### Scenario: Target discovery failure blocks execution

- GIVEN a catalog is incomplete, inconsistent, or inaccessible
- WHEN planning runs
- THEN error SHALL be reported and execution SHALL NOT begin

### Requirement: Safe File Operations Without Shell Interpolation

Managed file operations MUST be implemented using direct filesystem API calls or safe Go standard library functions. The installer MUST NOT construct shell commands with string interpolation for file copy, move, or link operations on managed targets.

#### Scenario: File copy uses Go stdlib, not shell

- GIVEN the installer needs to copy a managed source file to a target path
- WHEN the copy executes
- THEN the operation SHALL use `os`, `io`, or equivalent Go stdlib calls
- AND the operation SHALL NOT invoke a shell subprocess for the copy

#### Scenario: Symlink creation uses direct API

- GIVEN the installer needs to create a symlink for a managed target
- WHEN the symlink operation executes
- THEN the operation SHALL use `os.Symlink` or equivalent direct API call
- AND the operation SHALL NOT invoke `ln -s` through a shell

#### Scenario: No shell injection possible in target paths

- GIVEN a managed target path containing special characters (spaces, quotes)
- WHEN the file operation executes
- THEN the operation SHALL handle the path correctly without shell interpretation
- AND the operation SHALL NOT be vulnerable to injection through the path value

### Requirement: Automatic Rollback On Handled Failure

If any managed-target mutation fails after at least one managed target has already been mutated, the installer MUST automatically initiate rollback. Rollback SHALL restore all previously mutated managed targets using their backups.

#### Scenario: Rollback after second target fails

- GIVEN a plan with 5 managed targets
- WHEN targets 1 and 2 succeed but target 3 fails
- THEN the installer SHALL automatically restore targets 2 and 1 from backups
- AND the installer SHALL NOT mutate targets 4 and 5
- AND the installer SHALL NOT delete the retained backups

#### Scenario: Rollback after first target fails

- GIVEN a plan with 3 managed targets
- WHEN the first target mutation fails
- THEN no rollback of prior targets is needed (none were mutated)
- AND the installer SHALL NOT mutate targets 2 and 3
- AND the installer SHALL exit with a failure status

#### Scenario: Rollback preserves backups

- GIVEN rollback has completed after a handled failure
- WHEN the installer process exits
- THEN all backups created during this run SHALL still exist on disk

### Requirement: Rollback Executes In Reverse Order

When multiple managed targets have been mutated and rollback triggers, restoration SHALL proceed in reverse execution order where target ordering creates dependencies.

#### Scenario: Reverse-order rollback

- GIVEN targets A, B, C were mutated in that order
- WHEN rollback triggers after a failure following C's mutation
- THEN B SHALL be restored before A
- AND the restoration order SHALL be C (if partially mutated), B, A

#### Scenario: Independent targets rollback order

- GIVEN targets X and Y are independent (no ordering dependency)
- WHEN rollback triggers
- THEN both SHALL be restored from backups
- AND the order between them MAY be any order

### Requirement: Fail-Safe On Unrecoverable State

If planning, backup creation, or prerequisite checks cannot establish a recoverable execution path, the installer MUST fail safely without performing any mutation. The installer SHALL NOT proceed with a partially protected installation.

#### Scenario: Backup directory creation fails

- GIVEN the backup destination directory cannot be created
- WHEN the installer attempts to prepare for execution
- THEN the installer SHALL abort before any managed mutation
- AND the installer SHALL report the preparation failure

#### Scenario: Permission check fails for a managed target

- GIVEN a managed target's parent directory is not writable
- WHEN prerequisite checks run during planning
- THEN the installer SHALL abort before execution
- AND no mutation SHALL occur

### Requirement: Plan Drift Detection Before Mutation

Before mutation, the installer MUST freshly scan every operation, recording drift path+observed identity/digest. Unauthorized drift MUST report complete evidence, define authorization bound to exact target release+set, and abort. Matching authorization MAY allow only reviewed drift after a fresh scan. Any release/path/observation change MUST reject authorization, report fresh drift, and mutate nothing. Unbound force/environment MUST NOT authorize or bypass backup/inventory/other preflight.

(Previously: Drift was checked per-target without evidence-bound authorization.)

#### Scenario: No drift detected

- GIVEN replacements/removals match baseline and creations remain absent
- WHEN full-plan preflight runs
- THEN execution SHALL be eligible

#### Scenario: Material drift detected

- GIVEN two replacements differ from baseline
- WHEN full-plan preflight runs
- THEN both observations and bound authorization MUST be reported; mutation SHALL NOT occur

#### Scenario: New-target pre-state is checked

- GIVEN a creation was absent when planned
- WHEN fresh preflight finds it absent
- THEN it SHALL remain eligible

#### Scenario: Removed target has local drift

- GIVEN a removal differs from baseline
- WHEN full-plan preflight runs
- THEN its observation MUST be reported and all mutation blocked

#### Scenario: Matching authorization permits reviewed drift

- GIVEN authorization matches target release and complete reviewed evidence
- WHEN a fresh scan reproduces that set
- THEN each reviewed replacement/removal MAY proceed only after backup and inventory

#### Scenario: Changed drift rejects authorization

- GIVEN release/path/content/state differs from authorized evidence
- WHEN fresh full-plan preflight runs
- THEN authorization MUST be rejected and fresh drift reported; mutation MUST NOT occur

### Requirement: Execution Reports Per-Target Progress

The execution engine MUST report progress for each managed target as it is processed. The report SHALL indicate whether each target was mutated successfully, skipped, or failed.

#### Scenario: Per-target success reporting

- GIVEN execution is processing managed targets
- WHEN each target mutation completes
- THEN the installer SHALL record the target's outcome (success, skipped, failed)

#### Scenario: Failure target is reported

- GIVEN a managed target mutation fails
- WHEN the failure occurs
- THEN the installer SHALL record the failure with the target path and error reason
- AND the installer SHALL include this in the final exit report

### Requirement: Retired Managed Targets Are Explicit Deletions

Installed targets omitted from desired MUST be explicit removals. Existing removals—unchanged or authorized drift—MUST be backed up/inventoried before deletion. Absent targets MUST be skipped; failure MUST restore deletions.

#### Scenario: Unchanged retired target is removed

- GIVEN an unchanged installed target is omitted
- WHEN the transaction commits
- THEN the target MUST be backed up, deleted, and recorded

#### Scenario: Retired target is already absent

- GIVEN a removal target is absent
- WHEN the transaction runs
- THEN removal MUST be skipped and the target MUST NOT be recreated

#### Scenario: Removal is rolled back

- GIVEN a retired target was deleted after backup
- WHEN later mutation fails
- THEN the target MUST be restored from backup

### Requirement: Conservative Legacy Baselines

For `legacy/unknown`, a trusted completed inventory MAY baseline recorded path/identities without creating release identity. Any existing desired destination lacking that baseline MUST be untrusted drift; non-desired paths lacking baseline MUST NOT be inferred managed/deleted.

#### Scenario: Completed inventory establishes baseline

- GIVEN `legacy/unknown` has a trustworthy completed inventory
- WHEN first-apply planning runs
- THEN its recorded managed identities MAY form the baseline

#### Scenario: Missing baseline preserves unknown paths

- GIVEN `legacy/unknown` lacks a trustworthy completed inventory
- WHEN first-apply planning scans existing paths
- THEN desired destinations MUST be drift and unknown non-desired paths MUST NOT be deleted
