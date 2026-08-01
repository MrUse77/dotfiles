# Two-Phase Install Flow Specification

## Purpose

Define the install orchestration for machines where no usable dotfiles repository clone exists at invocation start. The installer MUST collect menu choices without mutating the system, obtain one explicit acceptance, execute a repository-independent package plan, clone the repository, and then build and execute a configuration plan using the existing managed transaction. Machines with a usable clone MUST retain the current single-plan flow and its managed-targets-before-external-actions execution order.

## Requirements

### Requirement: Route Selection by Repository Availability

The installer MUST perform a read-only repository availability check at the start of each invocation and select the install route based on its result. A usable clone selects the existing single-plan route. The absence of a usable clone selects the missing-clone two-phase route. Route selection MUST NOT depend solely on Git availability.

#### Scenario: Usable clone selects single-plan route

- GIVEN a usable dotfiles repository clone is resolvable from the current directory or configured canonical locations
- WHEN the user invokes the install command
- THEN the installer SHALL select the existing single-plan route
- AND the installer SHALL NOT enter the two-phase flow

#### Scenario: No usable clone selects two-phase route

- GIVEN no usable dotfiles repository clone can be resolved
- WHEN the user invokes the install command
- THEN the installer SHALL select the missing-clone two-phase route

#### Scenario: Git availability does not determine route

- GIVEN Git is installed on the machine
- AND no usable dotfiles repository clone can be resolved
- WHEN the user invokes the install command
- THEN the installer SHALL select the missing-clone two-phase route

#### Scenario: Route re-evaluated on each invocation

- GIVEN a previous invocation created a usable clone but the current invocation cannot resolve it (e.g., directory was removed)
- WHEN the user invokes the install command
- THEN the installer SHALL select the route based on the current state, not cached results from prior runs

### Requirement: Read-Only Pre-Acceptance on Missing-Clone Route

On the missing-clone route, the installer MUST NOT perform any system mutation before the user provides explicit acceptance. Menu display, repository detection, and system capability checks MAY read state. No package installation, command execution, clone operation, backup, directory creation, or managed-target mutation SHALL occur before acceptance.

#### Scenario: Menu shown on clean machine without Git or clone

- GIVEN a clean machine without Git and without a repository clone
- WHEN the user invokes the install command
- THEN the interactive menu SHALL be displayed with the system untouched
- AND no package installation, file mutation, clone, or directory creation SHALL have occurred before the menu is shown

#### Scenario: Decline before acceptance leaves system unchanged

- GIVEN the menu is displayed on the missing-clone route
- WHEN the user declines or cancels before providing acceptance
- THEN no package command SHALL be executed
- AND no repository clone SHALL be created
- AND no managed target SHALL be mutated
- AND the system SHALL remain in its pre-invocation state

#### Scenario: Menu cancellation leaves system unchanged

- GIVEN the menu is displayed on the missing-clone route
- WHEN the user interrupts the process or closes the terminal before acceptance
- THEN the system SHALL remain in its pre-invocation state with no mutations

### Requirement: Single Authorization

On the missing-clone route, exactly one explicit user acceptance SHALL authorize the complete two-phase run (package execution, repository acquisition, and configuration execution). The installer MUST NOT require a second affirmative confirmation after the clone is acquired. The installer MUST NOT execute any mutation before this acceptance.

#### Scenario: One acceptance authorizes both phases

- GIVEN the user has selected options from the menu
- AND the package plan has been presented for review
- WHEN the user provides one explicit acceptance
- THEN the installer SHALL be authorized to execute the package phase, repository acquisition, and configuration phase without requiring additional confirmation

#### Scenario: Acceptance screen discloses deferred configuration targets

- GIVEN the missing-clone route is active
- WHEN the acceptance screen is presented
- THEN the screen SHALL fully present the package/system actions and their irreversible nature
- AND the screen SHALL identify the repository destination and ref
- AND the screen SHALL state that concrete managed targets will be discovered after repository acquisition
- AND the screen SHALL disclose that configuration rollback restores only managed targets, not package or clone effects

#### Scenario: No acceptance prevents all mutation

- GIVEN the missing-clone route is active
- WHEN the user has not yet provided acceptance
- THEN no package command, clone operation, or managed-target mutation SHALL occur

### Requirement: Package Plan Construction

The installer MUST build an immutable package plan from the binary-owned action catalog, environment observations, and the accepted menu options. The package plan MUST be buildable without reading repository contents. It SHALL contain only external actions that can be planned and executed without repository contents. The base-tools action (providing Git) MUST be included and ordered first among external actions.

#### Scenario: Package plan built without repository

- GIVEN no usable repository clone exists
- WHEN the installer constructs the package plan
- THEN the plan SHALL be built without reading any repository contents
- AND the plan SHALL contain only repository-independent external actions

#### Scenario: Base-tools action ordered first

- GIVEN the package plan includes the base-tools action and other external actions
- WHEN the package plan is constructed
- THEN the base-tools action SHALL be the first external action in execution order

#### Scenario: Package plan is immutable

- GIVEN the package plan has been constructed and presented for review
- WHEN execution begins
- THEN the package plan SHALL be the exact plan that was reviewed
- AND the installer SHALL NOT re-plan, add actions, or modify the package plan between acceptance and package execution

#### Scenario: Repository-scoped actions excluded from package plan

- GIVEN the action catalog contains an action that requires a repository working directory (e.g., submodule update)
- WHEN the package plan is constructed on the missing-clone route
- THEN that action SHALL NOT be included in the package plan

### Requirement: Package Execution

The installer MUST execute the package plan in its reviewed order. The base-tools action MUST complete before any Git-dependent action and before repository acquisition. A package action failure MUST stop remaining package actions and prevent repository acquisition and configuration execution.

#### Scenario: Base-tools provides Git before Git-dependent actions

- GIVEN a clean machine without Git
- AND the package plan is executing
- WHEN the base-tools action completes
- THEN Git SHALL be available for subsequent Git-dependent actions in the package plan

#### Scenario: Package action fails mid-execution

- GIVEN the package plan has started executing and some actions have succeeded
- WHEN a package action fails
- THEN remaining package actions SHALL be skipped
- AND repository acquisition SHALL NOT occur
- AND configuration planning SHALL NOT occur
- AND the report SHALL identify completed, failed, and skipped actions
- AND the report SHALL state that earlier external effects remain and are not rolled back

#### Scenario: All package actions succeed

- GIVEN all package actions execute without failure
- WHEN the package phase completes
- THEN the installer SHALL proceed to repository acquisition

### Requirement: Repository Acquisition

After successful package execution, the installer MUST clone the configured repository into the canonical location at the ref selected by the installer version and existing development overrides. The clone MUST include required submodules. A repository acquisition failure MUST prevent configuration planning and managed-target mutation. The installer MUST NOT automatically delete a clone created by a failed run.

#### Scenario: Repository cloned at installer-selected ref

- GIVEN the package phase completed successfully
- WHEN repository acquisition executes
- THEN the repository SHALL be cloned into the canonical location
- AND the clone SHALL be checked out at the ref selected by the installer version
- AND existing development overrides (`DOTFILES_DIR`, `DOTFILES_REPO`, `DOTFILES_BRANCH`) SHALL be honored

#### Scenario: Submodules materialized during acquisition

- GIVEN the repository has configured submodules
- WHEN repository acquisition executes
- THEN submodules SHALL be materialized as part of the acquisition

#### Scenario: Clone failure prevents configuration

- GIVEN the package phase completed successfully
- WHEN repository acquisition fails (network, authentication, ref, destination, or submodule error)
- THEN configuration planning SHALL NOT occur
- AND no managed-target mutation SHALL occur
- AND completed package effects SHALL remain
- AND the clone directory (if partially created) SHALL be retained for diagnosis
- AND the report SHALL identify repository acquisition as the failed phase

#### Scenario: Canonical destination exists but is not a usable repository

- GIVEN the canonical clone destination directory exists
- AND the directory is not a usable Git repository
- WHEN repository acquisition is attempted
- THEN the installer SHALL fail without deleting, overwriting, or adopting the directory
- AND no configuration mutation SHALL occur

### Requirement: Configuration Plan Construction

After successful repository acquisition, the installer MUST build a second immutable configuration plan from the exact acquired repository source and the same accepted menu options. The configuration plan SHALL contain managed targets and their post-package pre-state. External actions attempted in the package phase MUST NOT be included or replayed in the configuration plan. The accepted option snapshot MUST NOT be re-collected or silently changed between phases.

#### Scenario: Configuration plan built from acquired source

- GIVEN the repository has been successfully cloned
- WHEN the configuration plan is constructed
- THEN the plan SHALL be built from the exact acquired repository contents
- AND the plan SHALL use the same accepted menu options from the initial selection

#### Scenario: Package actions not replayed in configuration plan

- GIVEN an external action was executed during the package phase
- WHEN the configuration plan is constructed
- THEN that action SHALL NOT appear in the configuration plan
- AND each external action SHALL have exactly one owner across both phases

#### Scenario: Configuration plan is immutable

- GIVEN the configuration plan has been constructed
- WHEN configuration execution begins
- THEN the configuration plan SHALL be the exact plan that was displayed after acquisition
- AND the installer SHALL NOT re-plan or modify the configuration plan between display and execution

### Requirement: Configuration Plan Display

The installer MUST display the concrete configuration plan after repository acquisition and before managed-target mutation. The initial acceptance screen MUST disclose that exact configuration targets are deferred because the source does not yet exist locally. The configuration plan display is for transparency and MUST NOT require a second affirmative confirmation.

#### Scenario: Concrete targets displayed before mutation

- GIVEN the repository has been acquired and the configuration plan is built
- WHEN the installer proceeds to configuration execution
- THEN the concrete managed targets SHALL be displayed before any managed mutation occurs

#### Scenario: Deferred target disclosure on acceptance screen

- GIVEN the missing-clone route is active
- WHEN the acceptance screen is presented before execution
- THEN the screen SHALL state that concrete managed targets will be discovered after repository acquisition and displayed before mutation

#### Scenario: No second confirmation required

- GIVEN the user has already provided acceptance
- AND the configuration plan is displayed after acquisition
- WHEN the configuration plan is shown
- THEN the installer SHALL proceed to configuration execution without requiring a second affirmative confirmation

### Requirement: Configuration Execution

The configuration phase MUST reuse the existing managed transaction: backup before mutation, retained backups, automatic rollback for handled failures, retained inventory, and manual-restore compatibility. Configuration rollback SHALL restore managed targets to their state immediately before configuration execution. Configuration rollback SHALL NOT claim to reverse package-phase, clone, account, service, settings, cache, or other side effects.

#### Scenario: Configuration failure triggers existing rollback

- GIVEN the configuration phase has started mutating managed targets
- WHEN a managed-target mutation fails
- THEN the existing automatic rollback SHALL restore previously mutated managed targets from their backups
- AND retained backups and inventory SHALL be preserved

#### Scenario: Configuration rollback does not reverse packages or clone

- GIVEN the package phase and repository acquisition completed successfully
- AND the configuration phase triggers rollback
- WHEN rollback completes
- THEN managed targets SHALL be restored to their pre-configuration state
- AND installed packages, the repository clone, and other external effects SHALL remain in place
- AND the report SHALL clearly state what was rolled back and what remains

#### Scenario: Incomplete managed rollback preserves manual recovery contract

- GIVEN the configuration rollback cannot fully restore a target (backup corrupt or unreadable)
- WHEN the rollback completes
- THEN the installer SHALL report the restoration failure
- AND the installer SHALL name the retained inventory and recovery artifacts
- AND the existing manual-recovery contract SHALL be preserved

### Requirement: Failure Boundaries

The installer MUST enforce strict phase failure boundaries. A failed package phase MUST prevent repository acquisition and configuration execution. A failed repository acquisition MUST prevent configuration planning and managed mutation. A failed configuration phase MUST invoke the existing managed rollback. Each boundary MUST be enforced regardless of which prior phase actions succeeded.

#### Scenario: Package failure stops all subsequent phases

- GIVEN the package phase has failed
- WHEN the failure is detected
- THEN repository acquisition SHALL NOT be attempted
- AND configuration planning SHALL NOT be attempted
- AND no managed-target mutation SHALL occur

#### Scenario: Clone failure prevents configuration

- GIVEN repository acquisition has failed
- WHEN the failure is detected
- THEN configuration planning SHALL NOT be attempted
- AND no managed-target mutation SHALL occur

#### Scenario: User exits between package and configuration

- GIVEN the package phase and repository acquisition completed successfully
- AND the configuration plan has been displayed
- WHEN the user exits the process before configuration execution begins mutating targets
- THEN no managed targets SHALL be mutated
- AND the report SHALL identify completed package and repository effects as partial, not rolled back

### Requirement: Aggregate Reporting

The installer MUST report package, repository acquisition, and configuration outcomes as one installation attempt while preserving each plan's identity. Reports MUST NOT label a partial package-only or package-plus-clone outcome as a successful installation. Reports MUST identify the primary failed phase without discarding earlier successful outcomes. The managed inventory SHALL be exposed only when a configuration transaction exists.

#### Scenario: Full success report

- GIVEN all three phases (package, acquisition, configuration) completed successfully
- WHEN the installation report is generated
- THEN the report SHALL include the package-plan fingerprint and per-action outcomes
- AND the report SHALL include the repository acquisition status, destination, and ref
- AND the report SHALL include the configuration-plan fingerprint and managed-target outcomes
- AND the report SHALL include the managed inventory location

#### Scenario: Partial failure report after clone failure

- GIVEN the package phase completed successfully
- AND repository acquisition failed
- WHEN the installation report is generated
- THEN the report SHALL identify completed package outcomes
- AND the report SHALL identify repository acquisition as the failed phase
- AND the report SHALL state that no configuration transaction was started
- AND the report SHALL NOT label the outcome as a successful installation

#### Scenario: Partial failure report after package failure

- GIVEN some package actions succeeded and one failed
- WHEN the installation report is generated
- THEN the report SHALL identify completed, failed, and skipped package actions
- AND the report SHALL state that repository acquisition and configuration did not occur
- AND the report SHALL state that earlier external effects remain

#### Scenario: Configuration failure report with rollback

- GIVEN the configuration phase failed and rollback executed
- WHEN the installation report is generated
- THEN the report SHALL include package and acquisition outcomes
- AND the report SHALL include configuration rollback outcomes
- AND the report SHALL expose the managed inventory and backup paths
- AND the report SHALL NOT claim that package or clone effects were reversed

#### Scenario: No configuration transaction started

- GIVEN the configuration transaction did not start (e.g., acquisition failed or configuration planning failed)
- WHEN the installation report is generated
- THEN the report SHALL state that no configuration transaction was started
- AND the report SHALL NOT fabricate or reference a managed inventory

### Requirement: Existing-Clone Compatibility

When a usable repository clone exists, the installer MUST use the current single-plan route with managed targets committed before external actions. The missing-clone two-phase flow MUST NOT alter existing-clone execution order, confirmation model, plan structure, or reporting.

#### Scenario: Existing clone uses current single-plan flow

- GIVEN a usable repository clone exists at invocation start
- WHEN the user invokes the install command
- THEN the installer SHALL build one immutable plan containing both managed targets and external actions
- AND managed targets SHALL be committed before external actions execute
- AND the existing confirmation and reporting model SHALL apply unchanged

#### Scenario: Existing-clone executor order preserved

- GIVEN the missing-clone two-phase route is implemented
- WHEN an installation runs on a machine with a usable clone
- THEN the `installer.Executor` execution order SHALL be unchanged from before this change
- AND managed configuration SHALL execute before external actions

### Requirement: Filesystem Overlap Audit

The package phase MUST audit direct filesystem setup actions that may overlap with later managed configuration destinations. Any such overlap MUST be explicitly assigned to one phase. The report and rollback boundary MUST remain truthful about what each phase owns.

#### Scenario: Overlapping destination assigned to one phase

- GIVEN a package-phase setup action may create or modify a path that a later managed configuration target also uses
- WHEN the plans are constructed
- THEN the overlap SHALL be explicitly assigned to exactly one phase
- AND the report SHALL accurately state which phase owns the path

#### Scenario: No double ownership of filesystem paths

- GIVEN a filesystem path is owned by the package phase
- WHEN the configuration plan is constructed
- THEN the configuration plan SHALL NOT claim ownership of the same path for rollback purposes

## Non-Goals

The following are explicitly out of scope for this change:

- Implementing or redefining the short-term Git bootstrap fix from PR #87.
- Changing the current single-plan execution order for machines with an existing usable clone.
- Making package, account, service, cache, settings, network, or repository side effects transactional or automatically reversible.
- Rolling back installed packages when cloning or configuration fails.
- Adding per-action confirmations, a second required approval, or a new menu selection model.
- Redesigning package contents or the purpose/order of existing external actions except where ownership must be partitioned around repository acquisition.
- Supporting non-Arch package managers or broadening the installer's supported operating systems.
- Adding crash-resume, package-phase checkpoints for automatic continuation, or cross-run transaction recovery.
- Automatically deleting a clone created by a failed run.
- Changing backup retention, `moonarch-cli restore`, or the on-disk managed inventory contract.
- Changing root configuration files, CI, or general documentation.
