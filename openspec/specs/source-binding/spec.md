# Source Binding Specification

## Purpose

Bind planner-built installation targets to the exact source identity and content reviewed during planning. Detect and reject source material drift before any target mutation. Preserve backward compatibility for legacy direct internal targets that omit source digest bindings.

## Requirements

### Requirement: Planner-Bound Source Digest

The planner MUST compute and assign a `SourceDigest` for every managed target it constructs. The digest MUST be derived from the source content observed during the planning phase using the same algorithm used at enforcement time.

#### Scenario: Planner assigns digest to file target

- GIVEN a source path pointing to a regular file inside the repository
- WHEN the planner constructs a managed `Target` for that source
- THEN the resulting `Target.SourceDigest` MUST be the SHA-256 hex digest of the file content read during planning
- AND the digest MUST be non-empty

#### Scenario: Planner assigns digest to directory target

- GIVEN a source path pointing to a directory inside the repository
- WHEN the planner constructs a managed `Target` for that source
- THEN `Target.SourceDigest` MUST be a deterministic digest computed from the directory tree content observed during planning
- AND the digest MUST be non-empty

#### Scenario: Planner assigns digest to symlink target

- GIVEN a source path that is a symbolic link inside the repository
- WHEN the planner constructs a managed `Target` for that source
- THEN `Target.SourceDigest` MUST encode both the resolved link value and the link content observed during planning
- AND the digest MUST be non-empty

### Requirement: Source Digest Enforcement Before Mutation

The transaction MUST verify that the current source content matches the bound `SourceDigest` before performing any filesystem mutation on the associated target. If the digest does not match, the transaction MUST abort mutation of that target and MUST NOT proceed to backup, staging, or rename for that target.

#### Scenario: Source content unchanged passes enforcement

- GIVEN a planner-built target with `SourceDigest` set to the SHA-256 digest of source content at planning time
- AND the source file content has not changed since planning
- WHEN the transaction begins mutating that target
- THEN the digest verification MUST succeed
- AND the mutation proceeds normally

#### Scenario: Source content drift detected before mutation

- GIVEN a planner-built target with `SourceDigest` set
- AND the source file content has been modified after planning
- WHEN the transaction attempts to mutate that target
- THEN the transaction MUST detect the digest mismatch
- AND the transaction MUST NOT create a backup of the current target destination
- AND the transaction MUST NOT stage or rename any files for that target
- AND the target entry in the inventory MUST record a plan-drift error
- AND the transaction MUST return a plan-drift error for that target

#### Scenario: Source replaced by symlink after planning

- GIVEN a planner-built target whose source was a regular file at planning time
- AND the source path has been replaced with a symlink to a different file after planning
- WHEN the transaction attempts to enforce the source binding
- THEN the transaction MUST detect that the source identity or content no longer matches the bound digest
- AND the transaction MUST refuse to consume the symlink-substituted content
- AND the target MUST be marked as failed with a plan-drift error

### Requirement: TOCTOU-Safe Source Consumption

The transaction MUST consume source content in a manner that closes the check/use gap between source identity verification and actual content read. A path-only pre-check followed by an independent path-based reopen is insufficient. The system MUST use descriptor-relative or open-and-verify patterns that bind the verified identity to the consumed content.

#### Scenario: Source verified and consumed via bound descriptor

- GIVEN a target whose source path points to a regular file
- WHEN the transaction consumes the source content for copying
- THEN the system MUST either (a) open the file by path with `O_NOFOLLOW`, verify the descriptor refers to the expected inode, and read from that descriptor, or (b) use an equivalent mechanism that binds the opened content to the verified identity
- AND a concurrent symlink substitution at the source path between verification and read MUST NOT cause the transaction to consume attacker-controlled content

#### Scenario: Source directory consumed without symlink traversal

- GIVEN a target whose source path is a directory
- WHEN the transaction copies the source directory tree
- THEN each file and subdirectory read during the copy MUST be opened or read through paths verified against the original tree walk
- AND a concurrent symlink replacement of any source subtree MUST NOT cause content from outside the source tree to be copied

### Requirement: Legacy Direct Target Compatibility

Internal `Target` values constructed directly without going through the planner MAY have an empty `SourceDigest`. The transaction MUST accept such targets for execution without rejecting them solely because `SourceDigest` is empty. Digest enforcement is skipped when no binding is present.

#### Scenario: Direct target without digest is executable

- GIVEN a `Target` constructed directly with `SourceDigest` set to the empty string
- AND the target has valid `Source`, `Destination`, `Kind`, and `PreState`
- WHEN the transaction attempts to mutate this target
- THEN the transaction MUST NOT reject the target due to missing digest
- AND the mutation proceeds under the existing compatibility semantics
- AND the target is treated as having no source-binding enforcement

#### Scenario: Planner-built target always has digest

- GIVEN a target produced by the planner
- WHEN the planner finishes constructing the target
- THEN `SourceDigest` MUST be non-empty
- AND the transaction enforces digest verification for this target

#### Scenario: Compatibility boundary does not weaken planner plans

- GIVEN a plan that contains both planner-built targets (with `SourceDigest`) and legacy direct targets (without `SourceDigest`)
- WHEN the transaction executes the plan
- THEN planner-built targets MUST have digest enforcement applied
- AND legacy direct targets MUST execute without digest enforcement
- AND the presence of legacy targets MUST NOT cause planner-built targets to skip enforcement

### Requirement: Source Identity Verification

Before consuming source content, the transaction MUST verify that the source path refers to the same filesystem object type (regular file, directory, or symlink) as was observed during planning. A type change at the source path between planning and execution MUST be treated as source drift.

#### Scenario: Source type changed from file to symlink

- GIVEN a planner-built target whose source was a regular file during planning
- AND the source path now refers to a symbolic link
- WHEN the transaction verifies source identity
- THEN the transaction MUST detect the type change
- AND the target MUST fail with a plan-drift error
- AND no filesystem mutation occurs for that target

#### Scenario: Source type changed from directory to file

- GIVEN a planner-built target whose source was a directory during planning
- AND the source path now refers to a regular file
- WHEN the transaction verifies source identity
- THEN the transaction MUST detect the type change
- AND the target MUST fail with a plan-drift error

### Requirement: Safety-Check Failure Stops Mutation

When any source identity check, digest verification, or TOCTOU guard fails or produces an ambiguous result, the transaction MUST stop mutation of the affected target. The system MUST NOT fall back to a less safe filesystem operation such as a path-only check or an unguarded copy.

#### Scenario: Ambiguous source identity halts mutation

- GIVEN a target whose source identity check returns an error that is neither a clear match nor a clear mismatch
- WHEN the transaction evaluates the source binding
- THEN the transaction MUST treat the ambiguity as a failure
- AND the target MUST be marked as failed
- AND no mutation, backup, or staging occurs for that target

#### Scenario: Descriptor open fails after path verification

- GIVEN a source path that passed initial type and identity checks
- AND the subsequent descriptor-relative open fails
- WHEN the transaction attempts to consume the source
- THEN the transaction MUST NOT fall back to re-opening the source by path without verification
- AND the target MUST be marked as failed with an appropriate error
