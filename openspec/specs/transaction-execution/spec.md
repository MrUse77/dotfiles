# Transaction Execution Specification

## Purpose

Define the filesystem transaction behavior for commit, swap ordering, directory replacement, rollback ownership, exact mode preservation, and partial-recovery reporting. The transaction must maintain recoverable state at every step and report actionable recovery information when any operation fails.

## Requirements

### Requirement: Ownership-Aware Rollback

Rollback MUST distinguish between artifacts created or displaced by the active transaction and artifacts that were substituted or modified by an external actor after the transaction mutated the target. The system MUST NOT remove, overwrite, or delete a target path unless it can establish that the current content at that path is attributable to the transaction.

#### Scenario: Rollback restores original file for transaction-owned target

- GIVEN a target that was mutated by the transaction (file copied to destination)
- AND the current file at the destination matches the content the transaction installed
- WHEN rollback is invoked for this target
- THEN the system MUST restore the original pre-state content from the backup
- AND the restored file MUST have the exact mode bits recorded in the pre-state

#### Scenario: Rollback detects externally substituted file

- GIVEN a target that was mutated by the transaction
- AND the current file at the destination does NOT match what the transaction installed (e.g., an external actor replaced it)
- WHEN rollback is invoked for this target
- THEN the system MUST detect that the current content is not transaction-owned
- AND the system MUST NOT remove or overwrite the externally substituted content
- AND the system MUST preserve the backup, the current file, and the inventory entry
- AND the rollback result MUST report this target as "not restored — content not owned by transaction"

#### Scenario: Rollback removes transaction-installed file for previously absent target

- GIVEN a target whose pre-state was `absent`
- AND the transaction created a new file at the destination
- AND the file at the destination matches the content the transaction installed
- WHEN rollback is invoked
- THEN the system MUST remove the file at the destination
- AND the removal MUST succeed cleanly

#### Scenario: Rollback preserves externally created file for previously absent target

- GIVEN a target whose pre-state was `absent`
- AND the transaction created a new file at the destination
- AND the file at the destination has been replaced by an external actor with different content
- WHEN rollback is invoked
- THEN the system MUST detect that the current content is not transaction-owned
- AND the system MUST NOT remove the externally substituted file
- AND the rollback result MUST report this target as "not removed — content not owned by transaction"

### Requirement: Recoverable Directory Swap Failures

When replacing an existing directory with a new directory (copy-tree), the system MUST maintain a recoverable ordering across the rename and swap steps. If a failure occurs after the original directory has been moved but before the replacement is committed, the system MUST preserve the original (now in a trash location), the staged replacement, and the backup. The system MUST NOT discard any of these artifacts.

#### Scenario: Directory swap succeeds completely

- GIVEN an existing target directory at the destination
- AND a staged replacement directory ready for commit
- WHEN the swap sequence executes (rename destination to trash, rename staging to destination)
- THEN both renames MUST succeed
- AND the original content MUST be available in the backup
- AND the trash directory MAY be cleaned up after both renames succeed

#### Scenario: Second rename fails after original moved to trash

- GIVEN an existing target directory at the destination
- AND the system has renamed the destination to a trash path
- AND the subsequent rename of the staged replacement to the destination fails
- WHEN the failure is detected
- THEN the system MUST rename the trash path back to the destination (restoring the original)
- AND if the restore-rename also fails, the system MUST preserve the trash path, the staged directory, and the backup
- AND the system MUST NOT remove the trash path or staged directory
- AND the failure report MUST identify the trash path and staged directory locations

#### Scenario: Directory swap failure preserves all recovery artifacts

- GIVEN a directory swap that has failed at any step after the original was moved
- WHEN the transaction reports the failure
- THEN the report MUST include:
  - The path of the trash directory containing the original (if it still exists)
  - The path of the staged replacement directory (if it still exists)
  - The path of the backup containing the pre-state content
  - The inventory entry for the target
- AND the system MUST NOT perform any best-effort cleanup that destroys these artifacts

### Requirement: Exact Mode Preservation

The system MUST preserve exact filesystem mode bits, including special bits (setuid, setgid, sticky), for installed content and restored targets. Mode application MUST use the exact bits from the source or pre-state, not broadened, normalized, or umask-derived approximations.

#### Scenario: Installed file preserves exact source mode

- GIVEN a source file with mode `0755`
- WHEN the transaction copies the file to the destination
- THEN the installed file MUST have mode `0755`
- AND the mode MUST NOT be affected by the process umask

#### Scenario: Installed file preserves setuid bit

- GIVEN a source file with mode `04755` (setuid)
- WHEN the transaction copies the file to the destination
- THEN the installed file MUST have mode `04755`
- AND the setuid bit MUST NOT be silently stripped

#### Scenario: Restored file preserves exact pre-state mode

- GIVEN a target whose pre-state mode was `0640`
- WHEN rollback restores the original file from backup
- THEN the restored file MUST have mode `0640`
- AND the mode MUST be the exact pre-state mode, not a normalized or broadened value

#### Scenario: Restored directory preserves exact pre-state mode

- GIVEN a target directory whose pre-state mode was `0750`
- WHEN rollback restores the directory
- THEN the restored directory MUST have mode `0750`

#### Scenario: Installed directory preserves exact source mode

- GIVEN a source directory with mode `0700`
- WHEN the transaction copies the directory tree to the destination
- THEN the top-level installed directory MUST have mode `0700`
- AND the mode MUST include any special bits present on the source

### Requirement: Partial Recovery Reporting

When a swap, commit, or rollback operation fails partially, the returned result MUST identify every affected target, the state of each recovery artifact, and the safest available next action. The system MUST NOT report a full rollback success when recovery is incomplete.

#### Scenario: Commit failure reports affected target and retained artifacts

- GIVEN a plan with 5 managed targets
- AND the third target fails during commit
- WHEN the transaction returns the execution result
- THEN the result MUST identify the third target as failed with the specific error
- AND the result MUST list the backup path for the third target (if a backup was created)
- AND the result MUST list the first two targets as successfully mutated
- AND the result MUST list the fourth and fifth targets as not attempted

#### Scenario: Partial rollback failure reports incomplete recovery

- GIVEN a transaction that committed 3 targets and is now rolling back
- AND the rollback of the second target fails because the content was externally substituted
- AND the rollback of the first and third targets succeeds
- WHEN the transaction returns the rollback result
- THEN the result MUST report the second target as "not restored — content not owned by transaction"
- AND the result MUST report the first and third targets as successfully restored
- AND the result MUST NOT claim that full rollback succeeded
- AND the result MUST identify the retained backup path for the second target

#### Scenario: Directory swap failure reports all retained locations

- GIVEN a directory swap failure where the original was moved to trash and the restore-rename failed
- WHEN the transaction reports the failure
- THEN the report MUST include:
  - The destination path (currently occupied by the trash or staged content)
  - The trash path (if the original is still there)
  - The staged replacement path (if it still exists)
  - The backup path containing the pre-state content
  - The inventory file path
- AND the report MUST state that manual recovery is required
- AND the report MUST NOT imply that automated recovery completed successfully

#### Scenario: Rollback failure with inventory persistence error

- GIVEN a rollback that has individual target failures
- AND the subsequent inventory persistence also fails
- WHEN the transaction returns the result
- THEN the returned error MUST be a combined error containing both the rollback failures and the persistence failure
- AND the report MUST identify the inventory persistence failure separately
- AND the report MUST list each target rollback failure individually

### Requirement: Swap Ordering and Atomicity

File and symlink mutations MUST use an atomic rename-from-staging pattern. The staging file or link MUST be created in the same directory as the destination, populated with the intended content, synced, chmod'd to the exact target mode, and then renamed to the destination path. This ensures the destination transitions from old content to new content atomically.

#### Scenario: File commit uses staging rename

- GIVEN a file target with a source file
- WHEN the transaction commits the file
- THEN the system MUST create a temporary file in the destination's parent directory
- AND the system MUST copy the source content to the temporary file
- AND the system MUST sync the temporary file
- AND the system MUST set the exact source mode on the temporary file
- AND the system MUST rename the temporary file to the destination path
- AND after rename, the destination MUST contain the source content with the exact source mode

#### Scenario: Symlink commit uses staging rename

- GIVEN a symlink target with a bound link value
- WHEN the transaction commits the symlink
- THEN the system MUST create a temporary symlink in the destination's parent directory
- AND the system MUST rename the temporary symlink to the destination path
- AND after rename, the destination MUST be a symlink pointing to the bound link value

#### Scenario: Staging file cleaned up on rename failure

- GIVEN a file commit where the rename from staging to destination fails
- WHEN the rename fails
- THEN the system MUST remove the staging file
- AND the system MUST NOT leave orphan staging files in the destination directory
- AND the original destination content MUST remain unchanged

### Requirement: Failure Preservation Over Cleanup

When any transaction step fails, the system MUST preserve every still-useful artifact — original content (in backup), staged replacements, trash directories, and inventory — rather than performing best-effort cleanup that could destroy recovery paths. Cleanup is only safe after the transaction confirms complete success.

#### Scenario: Commit failure preserves backup and staged content

- GIVEN a file commit where staging and copy succeeded but rename failed
- WHEN the rename fails
- THEN the backup of the original destination MUST remain intact
- AND any staged content MUST be cleaned up only if it has no recovery value (i.e., the backup is still available)
- AND the inventory MUST reflect the failed state

#### Scenario: No best-effort cleanup after rollback failure

- GIVEN a rollback that failed for one or more targets
- WHEN the rollback returns with failures
- THEN the system MUST NOT attempt to "clean up" the failed targets' backups, trash directories, or staged content
- AND all recovery artifacts MUST remain on disk
- AND the system MUST report the exact paths of retained artifacts
