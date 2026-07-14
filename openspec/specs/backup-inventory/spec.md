# Backup and Inventory Specification

## Purpose

Ensure backup roots are created without trusting attacker-controlled symlinks or path substitution, and that recovery inventory becomes authoritative only through atomic persistence with restrictive permissions. Protect the durability and integrity of the recovery metadata that users depend on for manual recovery after interruption.

## Requirements

### Requirement: Symlink-Safe Backup Root Creation

The system MUST create backup roots without following or accepting attacker-controlled symlinks at any component of the backup root path. If any existing path component in the backup root chain is a symlink, the system MUST refuse to use that path and MUST report the substitution.

#### Scenario: Backup root path is clean

- GIVEN a backup root path where every component is either absent or a real directory owned by the current user
- WHEN the system creates the backup root
- THEN the system MUST create the directory with mode `0700` (owner read/write/execute only)
- AND the created directory MUST be a real directory, not a symlink target

#### Scenario: Backup root path contains attacker-controlled symlink

- GIVEN a backup root path where an intermediate component has been replaced with a symlink by a hostile actor
- WHEN the system attempts to create the backup root
- THEN the system MUST detect the symlink substitution
- AND the system MUST NOT create or write any files through the substituted path
- AND the system MUST return an error identifying the symlink component
- AND no data MUST be written to the attacker-controlled symlink target

#### Scenario: Backup root already exists as a real directory

- GIVEN a backup root path that already exists as a real directory with mode `0700`
- WHEN the system ensures the backup root exists
- THEN the system MUST accept the existing directory without error
- AND the system MUST verify that the existing directory is not a symlink

#### Scenario: Backup root parent has unsafe ownership

- GIVEN a backup root parent directory that is owned by a different user or is world-writable
- WHEN the system validates the backup root chain
- THEN the system MUST detect the unsafe ownership or permissions
- AND the system MUST refuse to create or use backup roots under that parent
- AND the system MUST report the ownership or permission issue

### Requirement: Atomic Inventory Persistence

The system MUST write the recovery inventory to disk using an atomic replacement pattern. The inventory file MUST NOT become visible or authoritative while it contains partial or incomplete data. The system MUST write to a temporary file in the same directory, sync, and then rename to the final path.

#### Scenario: Inventory written atomically on successful preparation

- GIVEN a transaction that has completed preparation of all managed targets
- WHEN the system persists the inventory
- THEN the system MUST write the complete inventory JSON to a temporary file in the backup root directory
- AND the system MUST sync the temporary file to durable storage
- AND the system MUST set restrictive permissions (mode `0600`) on the temporary file before rename
- AND the system MUST rename the temporary file to `inventory.json` in the backup root
- AND after rename, `inventory.json` MUST contain the complete, valid inventory data

#### Scenario: Inventory persistence survives process interruption during write

- GIVEN a transaction that is persisting the inventory via atomic write
- AND the process is interrupted after the temporary file is written but before the rename
- WHEN the user inspects the backup root after the interruption
- THEN `inventory.json` from a prior successful run (if any) MUST remain intact and uncorrupted
- AND the temporary file MAY exist as an incomplete artifact
- AND the temporary file MUST NOT be mistaken for authoritative inventory by subsequent recovery logic

#### Scenario: Inventory update during commit is atomic

- GIVEN a transaction that has committed one or more targets and needs to update the inventory
- WHEN the system persists the updated inventory
- THEN the system MUST use the same atomic write-rename pattern
- AND the updated `inventory.json` MUST reflect the current state of all entries
- AND a crash during this write MUST NOT leave `inventory.json` in a partially written state

### Requirement: Restrictive Inventory Permissions

The inventory file MUST be written with mode `0600` (owner read/write only) before it is treated as authoritative. The system MUST NOT create the inventory with broader permissions at any point, including during the temporary-file phase.

#### Scenario: Inventory created with restrictive permissions

- GIVEN a new inventory being persisted for the first time
- WHEN the system writes the inventory file
- THEN the final `inventory.json` MUST have mode `0600`
- AND the temporary file used during atomic write MUST also have mode `0600` before rename

#### Scenario: Inventory permissions not broadened by umask

- GIVEN a process umask that would normally produce broader permissions (e.g., `0022`)
- WHEN the system persists the inventory
- THEN the system MUST explicitly set mode `0600` regardless of the process umask
- AND the resulting file MUST NOT be group-readable or world-readable

### Requirement: Inventory Content Integrity

The persisted inventory MUST contain a complete record of every managed target, its pre-state, backup path, and execution status. The inventory format MUST be stable enough for human-assisted recovery. Error conditions MUST be recorded as string descriptions without preventing the rest of the inventory from being written.

#### Scenario: Inventory records all managed targets after preparation

- GIVEN a plan with N managed targets
- WHEN preparation completes and the inventory is persisted
- THEN the inventory MUST contain exactly N entries
- AND each entry MUST include the target definition, original pre-state, backup path, and status

#### Scenario: Inventory records failure state without losing other entries

- GIVEN a plan with multiple managed targets
- AND one target fails during commit
- WHEN the inventory is persisted after the failure
- THEN the failed target's entry MUST record the failure status and error description
- AND all other entries MUST retain their current status and data
- AND the inventory file MUST be valid JSON

### Requirement: Backup Path Isolation

Backup paths MUST be located within a run-scoped directory that is isolated from the managed target destinations. The backup path derivation MUST be deterministic based on the run ID and destination. Backup paths MUST NOT be redirectable through symlinks at the backup root or intermediate components.

#### Scenario: Backup path is inside run-scoped directory

- GIVEN a managed target with destination `$HOME/.config/app/config.yml`
- AND a run ID `20260713-abc123`
- WHEN the system computes the backup path
- THEN the backup path MUST be under a `.dots-backups/<runID>/` directory
- AND the backup path MUST NOT overlap with any managed target destination

#### Scenario: Backup collision detected before mutation

- GIVEN two targets in the same plan that would produce the same backup path
- WHEN the system prepares the plan
- THEN the system MUST detect the backup collision
- AND the system MUST refuse to proceed with either target
- AND the system MUST report the collision with both target destinations

### Requirement: Backup Root Validation Before Use

Before using any backup root for writing, the system MUST validate that the backup root is a real directory with appropriate ownership and permissions. If the backup root exists but was created by a different process or with unexpected properties, the system MUST refuse to use it.

#### Scenario: Backup root validated before backup creation

- GIVEN a backup root path that exists as a real directory with mode `0700` owned by the current user
- WHEN the system validates the backup root before creating a backup
- THEN the validation MUST succeed
- AND the system MAY proceed to create the backup file

#### Scenario: Backup root exists with wrong permissions

- GIVEN a backup root path that exists as a real directory but with mode `0755`
- WHEN the system validates the backup root
- THEN the system MUST detect the overly permissive mode
- AND the system MUST either correct the permissions to `0700` or refuse to use the directory
- AND no backup data MUST be written until the root is validated

#### Scenario: Backup root creation after hostile symlink insertion

- GIVEN a backup root path where the final component was a symlink when the system last checked
- AND the system is about to create the backup root directory
- WHEN the system attempts to create the directory
- THEN the system MUST use `O_NOFOLLOW` or equivalent to prevent following the symlink
- AND the system MUST detect and report the substitution
- AND no data MUST be written through the symlink
