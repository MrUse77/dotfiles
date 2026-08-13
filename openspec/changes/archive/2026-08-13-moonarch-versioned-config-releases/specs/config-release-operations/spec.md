# Config Release Operations Specification

## Purpose

Define recoverable operations.

## Requirements

### Requirement: Installed Status Uses Verified Identities

Status MUST report durable current/previous tags/digests and retention. Before first verified apply, current MUST be `legacy/unknown` and previous absent. Files MUST NOT imply identity; unresolved journals MUST be disclosed.

#### Scenario: Legacy installation has unknown identity

- GIVEN no verified apply
- WHEN status runs
- THEN current MUST be `legacy/unknown` and previous MUST be absent

#### Scenario: Installed identities are reported

- GIVEN current B and retained previous A
- WHEN status runs
- THEN both identities and retention MUST be reported

### Requirement: Exact Apply Is Serialized and Fail-Closed

Apply MUST use an admitted exact artifact under exclusive lock through state commit. Artifact/source/compatibility/dependency/theme and full-preflight checks MUST pass. Success MUST inventory backups and atomically rotate identities; failure MUST restore mutations and preserve identities. Drift MAY pass only via evidence-bound authorization.

#### Scenario: Exact apply succeeds

- GIVEN current A and admitted B pass checks
- WHEN B commits
- THEN B MUST become current, A previous, and inventory retained

#### Scenario: Preflight failure makes no changes

- GIVEN a mandatory check fails
- WHEN B is applied
- THEN no target or installed identity MUST change

#### Scenario: Concurrent mutation is excluded

- GIVEN another mutation holds the lock
- WHEN mutation is requested
- THEN contention MUST be reported; mutation MUST NOT occur

#### Scenario: Apply transaction fails

- GIVEN apply mutates but cannot commit
- WHEN failure is handled
- THEN state MUST be restored; identities MUST remain unchanged

### Requirement: Evidence-Bound Drift Authorization

First apply/rollback drift MUST abort, report each path+observed identity/digest, and define authorization bound to exact release tag/digest+set. Later invocation MAY proceed only after explicit match and fresh full-plan scan. Any release/path/observation change MUST reject authorization, report fresh drift, and mutate nothing. Unbound force/environment values MUST NOT authorize. Authorization MUST NOT bypass artifact/source/compatibility/dependency/theme/lock/journal/backup/inventory checks.

#### Scenario: First drift aborts

- GIVEN apply/rollback finds unauthorized drift
- WHEN preflight completes
- THEN all evidence and bound authorization MUST be reported; mutation MUST NOT occur

#### Scenario: Matching authorization

- GIVEN authorization matches target release and reviewed evidence
- WHEN a fresh scan reproduces the exact set
- THEN reviewed targets MAY proceed through every remaining requirement

#### Scenario: Invalid authorization

- GIVEN release/path/evidence differs, or only broad force/environment input exists
- WHEN fresh preflight runs
- THEN authorization MUST be rejected and fresh drift reported; mutation MUST NOT occur

### Requirement: Journal Recovery Converges State

Mutation MUST retain a journal until commit and state agree. Recovery MUST precede status/mutation under lock. Committed work MUST finalize identity; uncommitted work MUST restore prior state. Indeterminate outcomes MUST block mutation without guessing.

#### Scenario: Crash occurs before transaction commit

- GIVEN an uncommitted journal
- WHEN recovery runs
- THEN prior state MUST be restored

#### Scenario: Crash occurs after commit but before state update

- GIVEN commit lacks identity update
- WHEN recovery runs
- THEN identity MUST finalize without reapplication

#### Scenario: Journal outcome is indeterminate

- GIVEN commit outcome is indeterminate
- WHEN recovery runs
- THEN mutation MUST be blocked and reported

### Requirement: Rollback Is a New Offline Transaction

Rollback MUST reapply retained previous as a new transaction under apply's lock/journal/check/preflight/backup rules, without network/Git. Success MUST swap identities; failure MUST preserve them and restore mutations.

#### Scenario: Offline rollback succeeds

- GIVEN current B, previous A, and no network
- WHEN rollback commits
- THEN A MUST become current, B previous, and inventory recorded

#### Scenario: Previous release is unavailable

- GIVEN previous is absent/unverified
- WHEN rollback is requested
- THEN rollback MUST fail before mutation; identities MUST remain unchanged

#### Scenario: Local drift blocks rollback

- GIVEN replacement/removal drift since B
- WHEN rollback preflight runs
- THEN all drift and bound authorization MUST be reported; mutation MUST NOT occur
