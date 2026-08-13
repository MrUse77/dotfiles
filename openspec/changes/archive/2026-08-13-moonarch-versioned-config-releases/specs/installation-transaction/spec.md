# Delta for Installation Transaction

## ADDED Requirements

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

## MODIFIED Requirements

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
