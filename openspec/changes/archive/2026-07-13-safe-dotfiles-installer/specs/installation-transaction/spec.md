# Installation Transaction Specification

## Purpose

Define the transactional execution of managed-target mutations: target discovery, safe file operations, execution ordering, drift detection, and automatic rollback on handled failure. This domain ensures that managed file mutations behave as a single recoverable transaction.

## Requirements

### Requirement: Managed-Target Discovery Completes Before Execution

The installer MUST discover and enumerate all managed targets during the planning phase, before execution begins. The discovered target set SHALL be the complete input to execution; no new targets SHALL be added during execution without re-planning.

#### Scenario: Full target set established during planning

- GIVEN the installer is building the installation plan
- WHEN target discovery runs
- THEN the resulting target set SHALL include all files and directories the installer will manage
- AND the target set SHALL be passed to the execution engine as a closed set

#### Scenario: Target discovery failure blocks execution

- GIVEN target discovery encounters an error (missing source, permission denied)
- WHEN the planning phase runs
- THEN the installer SHALL transition to an error state
- AND execution SHALL NOT begin

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

Before executing each managed-target mutation, the installer SHOULD verify that the target's current state is consistent with the state observed during planning. Material drift SHALL cause a safe abort before mutation rather than silent replanning.

#### Scenario: No drift detected

- GIVEN a managed target was observed during planning with content hash H
- WHEN execution reaches this target and the current content hash is still H
- THEN the installer SHALL proceed with the mutation

#### Scenario: Material drift detected

- GIVEN a managed target was observed during planning with content hash H
- WHEN execution reaches this target and the current content hash differs from H
- THEN the installer SHALL abort execution before mutating this target
- AND the installer SHALL report the drift
- AND any previously mutated targets SHALL be rolled back

#### Scenario: Drift check skipped for new targets

- GIVEN a managed target did not exist during planning (new file creation)
- WHEN execution reaches this target
- THEN the installer MAY skip the drift check for this target's pre-state
- AND the installer SHALL still verify the target does not exist before creating it

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
