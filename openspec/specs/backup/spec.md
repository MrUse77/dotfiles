# Backup Specification

## Purpose

Define the backup lifecycle for managed installation targets: creation before mutation, retention after success or rollback, and restoration during automatic rollback. Every managed target — including root-level files — MUST have a backup before it is mutated.

## Requirements

### Requirement: Backup Before Every Managed Mutation

The installer MUST create a backup of each managed target's current content before mutating that target. No managed target SHALL be written without a preceding successful backup.

#### Scenario: Backup created before file mutation

- GIVEN a managed target at `~/.config/hypr/hyprland.conf` with existing content
- WHEN the installer prepares to mutate this target
- THEN a backup of the file's current content SHALL exist on disk before the mutation begins

#### Scenario: Backup created before root-level file mutation

- GIVEN a managed root-level file at `~/.zshrc` with existing content
- WHEN the installer prepares to mutate this target
- THEN a backup of the file's current content SHALL exist on disk before the mutation begins

#### Scenario: Backup created before directory target mutation

- GIVEN a managed directory target (e.g. stowed config directory) with existing content
- WHEN the installer prepares to mutate this target
- THEN a backup capturing the directory's current state SHALL exist on disk before the mutation begins

#### Scenario: Mutation blocked when backup fails

- GIVEN a managed target exists but the backup destination is unwritable
- WHEN the installer attempts to prepare for mutation
- THEN the installer SHALL abort before mutating the target
- AND the installer SHALL report the backup failure

### Requirement: Backup Coverage Includes Root-Level Files

Backup coverage MUST extend to root-level files managed by the installer, not only to files within managed directories. A root-level file (e.g. `~/.zshrc`, `~/.gitconfig`) SHALL receive the same backup treatment as any directory-based target.

#### Scenario: Root-level file backup parity

- GIVEN the installer manages both a root-level file (`~/.zshrc`) and a directory target (`~/.config/ghostty/`)
- WHEN backup preparation runs
- THEN both the root-level file and the directory target SHALL have backups created
- AND neither SHALL be mutated before its backup succeeds

### Requirement: Backup Retention After Success

Retained backups MUST NOT be automatically deleted after a successful installation. The backup set SHALL remain on disk for manual recovery by the user.

#### Scenario: Backups persist after successful install

- GIVEN the installer completed all managed mutations successfully
- WHEN the installer process exits
- THEN all backups created during this installation SHALL still exist on disk
- AND the installer SHALL NOT have deleted any backup

#### Scenario: Backups persist after rollback

- GIVEN the installer triggered automatic rollback after a handled failure
- WHEN the rollback completes and the installer process exits
- THEN all backups created during this installation SHALL still exist on disk
- AND the installer SHALL NOT have deleted any backup

### Requirement: Deterministic Backup Naming

Each backup MUST use a deterministic naming scheme that allows unambiguous identification of the source target and the installation run. Backup names SHALL be reproducible given the target path and run context.

#### Scenario: Backup path is deterministic

- GIVEN a managed target at `/home/user/.zshrc` in installation run `20260712T103000`
- WHEN the backup is created
- THEN the backup SHALL be stored at a path derivable from the target path and run identifier
- AND the same target and run identifier SHALL always produce the same backup path

#### Scenario: Backup name collision handling

- GIVEN an existing backup for the same target and run identifier already exists
- WHEN the installer attempts to create a new backup for that target
- THEN the installer SHALL treat this as an error condition
- AND the installer SHALL NOT silently overwrite the existing backup

### Requirement: Backup Restoration During Rollback

When automatic rollback is triggered, the installer MUST restore each managed target to its pre-mutation state using the corresponding backup. Restoration SHALL use the backup content, not a re-generation of the original.

#### Scenario: Rollback restores from backup

- GIVEN a managed target was mutated during installation
- AND a backup of its pre-mutation content exists
- WHEN automatic rollback executes for this target
- THEN the target SHALL be restored to exactly the content captured in the backup

#### Scenario: Rollback of multiple targets

- GIVEN three managed targets were mutated in order A, B, C
- WHEN automatic rollback triggers after target C fails
- THEN targets A and B SHALL be restored from their backups
- AND restoration SHALL proceed in reverse execution order (B, then A) where ordering matters

#### Scenario: Rollback when backup is corrupt or missing

- GIVEN a managed target was mutated
- AND its backup is corrupt or missing at rollback time
- THEN the installer SHALL report the restoration failure
- AND the installer SHALL continue attempting to restore remaining targets
- AND the installer SHALL exit with a non-zero status indicating incomplete rollback

### Requirement: Recoverability Check Before Mutation

Before mutating any managed target, the installer MUST verify that the backup for that target is valid and restorable. If the recoverability check fails, mutation SHALL be blocked.

#### Scenario: Recoverability verified before mutation

- GIVEN a backup has been created for a managed target
- WHEN the installer performs the recoverability pre-check
- THEN the installer SHALL verify the backup exists and is readable
- AND only then SHALL the installer proceed with the mutation

#### Scenario: Unreadable backup blocks mutation

- GIVEN a backup was created but is not readable (permission, corruption)
- WHEN the installer performs the recoverability pre-check
- THEN the installer SHALL abort before mutating this target
- AND the installer SHALL report the recoverability failure

### Requirement: Manual Restoration From Retained Backups

The installer MUST provide a manual restore command (`dots restore`) that restores managed targets from retained backups. Restoration SHALL use the retained backup content, never a re-generation of the original. The retained backups MUST NOT be deleted by the restore command.

#### Scenario: Restore a file target from its retained backup

- GIVEN a managed file target was installed in a completed run with a retained backup
- WHEN the user runs `dots restore` and selects that target
- THEN the destination SHALL be replaced with the exact content captured in the backup
- AND the backup SHALL remain on disk

#### Scenario: Restore removes targets that did not exist before the install

- GIVEN a managed target had no pre-existing content (absent pre-state) and was created by the install
- WHEN the user restores that target
- THEN the installed destination SHALL be removed
- AND the restore SHALL not fail when the destination is already absent

#### Scenario: Modified destination is confirmed before overwrite

- GIVEN a destination was modified after the installation (content differs from the installed digest)
- WHEN the user selects it for restore
- THEN the restore SHALL ask for confirmation before overwriting the destination
- AND the restore SHALL be cancelled when the user declines
