# Proposal: Two-Phase Install Flow for Missing Repository Clones

## Decision summary

When no usable dotfiles repository clone exists at the start of an install, the installer will collect menu choices without mutating the system, obtain one explicit acceptance, execute a repository-independent package plan, clone the repository at the binary-selected ref, and then build and execute a configuration plan using the existing backup and rollback transaction.

Machines that already have a usable clone will retain the current single-plan flow and its deliberate managed-targets-before-external-actions execution order. The new order applies only to the missing-clone path.

## Intent

Make the installer capable of bootstrapping a clean supported machine without requiring the user to install Git manually or encounter a failure before the interactive install experience begins.

The change resolves a dependency cycle in the current architecture:

1. Git is required to clone the repository.
2. The repository is required to discover managed targets and build the current immutable installation plan.
3. Git is installed by the base-tools external action contained in that plan.
4. The current executor commits managed targets before it runs external actions.

The result is that the installer cannot reach its own Git-installing action on the machine that needs it most.

## Business problem

The first-run experience does not fulfill the installer’s core promise. A user on a clean Arch Linux machine can have the installer binary but no Git installation and no local dotfiles clone. The command currently tries to resolve or clone the repository before showing the menu, so it fails before the normal guided workflow can start.

This creates avoidable user confusion and operational cost:

- the installer appears unable to install its own declared prerequisite;
- the failure occurs before users can review or accept the intended installation;
- users must discover and perform an undocumented manual prerequisite or depend on a temporary bootstrap path;
- clean-machine automation and support guidance differ from the normal install flow;
- the single-plan model cannot accurately represent the dependency boundary between package bootstrap, repository acquisition, and configuration installation.

## Target users and situations

### Primary situation

A user starts `moonarch-cli install` on a supported clean Arch Linux machine where:

- no usable dotfiles clone can be resolved from the current directory or configured canonical locations; and
- Git may not be installed.

The user expects the installer to show its menu first, install the selected system prerequisites after acceptance, fetch the matching dotfiles source, and configure the machine without manual intervention.

### Other affected situations

- A machine has Git but no repository clone. It follows the same missing-clone two-phase flow so behavior is based on repository availability, not on Git availability alone.
- A package or repository acquisition step fails after acceptance. The user needs an accurate partial-result report and must not receive a false rollback claim.
- A configuration operation fails after packages and the clone are present. Managed targets must still use the existing retained-backup and automatic-rollback behavior.

### Unaffected situation

A machine with a usable repository clone keeps the current single-plan experience and execution semantics.

## Current-state gap

The current implementation has three coupled decisions:

- `cli/cmd/install.go` resolves or clones the repository before launching the menu because target discovery reads repository contents.
- `plan.Planner.Build` creates one immutable `InstallationPlan` containing both repository-derived managed targets and ordered external actions.
- `installer.Executor` deliberately runs the recoverable managed transaction before the reviewed external actions, preventing irreversible actions when managed configuration cannot be committed.

That order is correct for the existing-clone path, but it cannot bootstrap a missing clone because the base-tools action that installs Git is unreachable until after repository-dependent planning.

The short-term clean-machine Git bootstrap behavior associated with PR #87 addresses the immediate deadlock by running the base-tools action before cloning. It is a separate change and is not the product scope of this proposal.

## Product outcome

After this change, a clean-machine user can:

1. enter the normal menu while the system is untouched;
2. review the irreversible package/system phase and understand that exact configuration targets will be derived after repository acquisition;
3. accept the installation once;
4. have the installer provide Git through the existing base-tools action and complete the remaining repository-independent external actions;
5. have the installer clone the source matching its own version;
6. see the materialized configuration plan before managed file mutation; and
7. receive the existing transactional backup and rollback protection for configuration targets.

The experience must clearly distinguish “the installation completed” from “packages completed but repository or configuration failed.”

## Proposed solution

### Route by repository availability

The installer will choose a route using read-only repository detection at the start of the invocation.

| Entry state | Product flow | Execution order |
| --- | --- | --- |
| Usable clone exists | Preserve the current single immutable plan and review flow | Managed configuration transaction, then external actions |
| No usable clone exists | Use two immutable phase plans under one accepted installation run | Repository-independent external actions, repository acquisition, managed configuration transaction |

The missing-clone route is selected even when Git is already installed. Git availability changes what the package manager must do, not whether repository-derived target planning is possible.

### Missing-clone flow

1. **Read-only menu:** Show the current selection menu. Repository detection and system capability checks may read state, but no package, command, clone, backup, directory creation, or managed-target mutation may occur.
2. **Package plan:** Build an immutable plan from the binary-owned action catalog, environment observations, and the selected options. It contains only external actions that can be planned and executed without repository contents. The existing base-tools action remains first and supplies Git.
3. **Single acceptance:** Show the package/system actions, their irreversible nature, the repository destination/ref, and a clear statement that concrete managed targets will be discovered from the fetched repository. One explicit acceptance authorizes the complete two-phase run. Declining leaves the system unchanged.
4. **Package execution:** Run the package plan in its reviewed order. A failure stops the run before repository acquisition and configuration.
5. **Repository acquisition:** Clone the configured repository, including required submodules, into the canonical location at the ref selected by the installer version and existing development overrides. A failure stops the run before configuration.
6. **Configuration plan:** Build a second immutable plan from the exact fetched repository and the original accepted menu options. This plan contains the managed targets and their post-package pre-state; external actions already attempted in the package phase must not be included or replayed. Display the concrete configuration plan before managed mutation for transparency without requiring a second affirmative authorization.
7. **Configuration execution:** Execute the existing managed transaction unchanged: bind source and destination state, create retained backups, commit managed targets, and automatically roll back handled failures.
8. **Aggregate result:** Report package, repository, and configuration outcomes as one installation attempt while preserving each plan’s identity and the configuration inventory location.

### Plan ownership and action partition

The two plans must be separate immutable values linked by one installation-run identity:

- **Package plan:** selected repository-independent external actions, with its own fingerprint and ordered outcomes.
- **Configuration plan:** repository-derived managed targets, with its own fingerprint, run identifier, and retained transaction inventory.

The accepted option snapshot is shared and must not be re-collected or silently changed between phases. Every operation must have exactly one owner so no action is duplicated or omitted.

Repository-scoped acquisition work cannot be naively moved into the pre-clone package plan. In particular, the current “update Git submodules” action requires a repository working directory. For a missing clone, clone/submodule materialization is owned by repository acquisition rather than executed as a pre-clone action. Existing-clone behavior remains unchanged.

## First-slice scope

### In scope

- Changes within the Go installer under `cli/`.
- Read-only routing between existing-clone and missing-clone installs.
- Showing the menu before any mutation on the missing-clone path.
- Building and reviewing a repository-independent package plan from accepted menu choices.
- Running the base-tools action, including Git, before any Git-dependent action or repository clone.
- Cloning the repository at the installer-selected ref after successful package execution.
- Building a separate immutable configuration plan from the fetched repository and the same accepted choices.
- Reusing the existing managed-target transaction, retained inventory, automatic rollback, and restore compatibility.
- Preserving the current single-plan path and managed-before-external order when a usable clone already exists.
- Phase-aware UI progress, errors, and final reporting for package execution, repository acquisition, and configuration execution.
- Preventing external actions from being replayed in the configuration phase.
- Tests for route selection, order, failure boundaries, plan correlation, and existing-clone compatibility.

### Scope boundary

Product code changes are confined to `cli/`. Root dotfile and application configuration content may be read as repository source at runtime but will not be edited by this change.

## Non-goals

- Implementing or redefining the short-term Git bootstrap fix from PR #87.
- Changing the current single-plan order for machines with an existing usable clone.
- Making package, account, service, cache, settings, network, or repository side effects transactional or automatically reversible.
- Rolling back installed packages when cloning or configuration fails.
- Adding per-action confirmations, a second required approval, or a new menu selection model.
- Redesigning package contents or the purpose/order of existing external actions except where ownership must be partitioned around repository acquisition.
- Supporting non-Arch package managers or broadening the installer’s supported operating systems.
- Adding crash-resume, package-phase checkpoints for automatic continuation, or cross-run transaction recovery.
- Automatically deleting a clone created by a failed run.
- Changing backup retention, `moonarch-cli restore`, or the on-disk managed inventory contract unless an additive correlation field is proven necessary in design.
- Changing root configuration files, CI, or general documentation in the first slice.

## Business rules and product constraints

1. The missing-clone menu and all pre-acceptance checks must be read-only.
2. One explicit acceptance must precede every package, repository, or managed-target mutation in the missing-clone flow.
3. Decline, menu cancellation, or failure before acceptance must leave the machine unchanged.
4. The package plan must be buildable without reading repository contents.
5. The base-tools action must run before the first Git-dependent action and before repository acquisition.
6. A failed or cancelled package phase must prevent repository acquisition and configuration execution.
7. A failed repository acquisition must prevent configuration planning and managed mutation.
8. The repository ref and override behavior must remain aligned with the existing clone contract: released binaries select their version, development builds select `main`, and existing development overrides remain honored.
9. The configuration plan must be built from the exact acquired source and the same accepted options.
10. The configuration transaction must retain its current backup-before-mutation, drift detection, automatic rollback, retained-inventory, and manual-restore semantics.
11. Configuration rollback restores managed targets to the state captured immediately before the configuration transaction. It does not claim to restore process-start state or reverse package-phase, account, service, settings, cache, or clone side effects.
12. No external action may be executed twice because it appeared in both phase plans.
13. A missing-clone implementation must not globally invert `installer.Executor`; existing-clone execution order is a compatibility invariant.
14. Concrete configuration targets must be displayed after repository acquisition and before managed mutation. The initial acceptance screen must disclose that these target details are deferred because the source does not yet exist locally.
15. Reports must never label a partial package-only or package-plus-clone outcome as a successful installation.
16. The package phase must audit direct filesystem setup actions that may overlap later managed destinations. Any such overlap must be explicitly assigned to one phase, and the report/rollback boundary must remain truthful.

## Edge cases and expected behavior

| Edge case | Expected behavior |
| --- | --- |
| No clone and no Git | Show the menu untouched; after acceptance, run base tools first, then later Git-dependent actions, then clone and configure. |
| No clone but Git already exists | Use the same two-phase route; do not skip the accepted package plan merely because Git is present. Existing idempotency behavior remains with the actions themselves. |
| Usable clone already exists | Use the current single-plan route and current managed-before-external order. |
| Canonical clone destination exists but is not a usable repository | Detect the conflict read-only where possible and fail without deleting, overwriting, or adopting the directory. No configuration mutation is allowed. |
| Package action fails after earlier actions succeeded | Stop remaining package actions, skip clone and configuration, report completed/failed/skipped actions, and state that earlier external effects remain. |
| Clone, network, authentication, ref, or submodule acquisition fails | Keep completed external effects, make no managed configuration mutation, retain the clone directory for diagnosis, and report repository acquisition as the failed phase. |
| Configuration planning fails after clone | Keep package and clone effects, create no managed mutation, and report that no configuration transaction was started. |
| Configuration mutation fails | Run the existing rollback for mutated targets, retain its inventory/backups, and leave all prior external and clone effects in place. |
| Rollback is incomplete | Preserve the current manual-recovery contract and name the retained inventory and recovery artifacts. |
| User exits after package execution but before configuration mutation | Do not mutate managed targets; report the completed external and repository effects as partial, not rolled back. |
| Process or machine terminates during package/clone | Make no automatic reversal guarantee. A later invocation evaluates current repository state normally. |
| Retry after the first run successfully created the clone | Route according to state at the new invocation; the usable clone selects the existing single-plan flow. |
| Selected action catalog contains a repository-working-directory action | Keep it out of the pre-clone plan and assign it to repository acquisition or a post-clone owner without duplication. |

## Implications and impact

### Execution orchestration

The current `installer.Executor` assumes a managed executor exists and uses its report as the base before appending external outcomes. The missing-clone flow needs the opposite high-level order, but only for that route. Design must introduce phase orchestration or distinct execution seams rather than reversing the global executor and regressing existing machines.

The package phase also needs a valid external-only execution path: it cannot fabricate a managed transaction or an empty managed inventory merely to reuse the current executor shape.

### Planning model

The current `InstallationPlan` merges targets and external actions under one fingerprint and can only be built after repository discovery. The new route requires two immutable plans and a shared correlation identity. Plan APIs must make action ownership explicit and prevent accidental re-execution when the configuration plan is created.

Because package actions run first, the configuration planner binds target pre-state after those external effects and after clone acquisition. This timing is intentional and must be reflected in rollback wording and tests.

### UI and authorization

The clean-machine acceptance occurs before concrete repository-derived targets are available. This is a deliberate tradeoff required to let the installer provide Git. The UI must compensate by:

- fully presenting the package/system actions before acceptance;
- explaining the deferred configuration scope and rollback boundary;
- identifying the repository destination and ref;
- displaying the exact configuration plan after clone and before managed mutation; and
- retaining clear cancellation and failure states without implying that earlier external effects were reversed.

Existing-clone users should not see a changed confirmation model.

### Reporting and inventory

`ExecutionReport` currently represents one plan fingerprint with managed-target and external-action outcomes, while the durable inventory belongs only to the managed transaction. The two-phase route requires an aggregate result that can identify:

- package-plan fingerprint and per-action outcomes;
- repository acquisition status, destination, and ref;
- configuration-plan fingerprint and managed-target outcomes;
- whether the managed transaction started;
- retained inventory and backup/recovery paths when applicable; and
- the primary failed phase without discarding earlier successful outcomes.

The managed inventory must remain authoritative only for configuration recovery. Package actions and the repository clone must not be written into it as if they were rollbackable managed targets. If no managed transaction starts, the report must say so rather than inventing an inventory.

### Operational and support impact

Support and diagnostics gain a clearer phase boundary but must account for legitimate partial state. Users may have packages and a clone after a later failure. Error messages need to tell them what completed, what did not, what remains on disk, and whether `moonarch-cli restore` is relevant.

### Affected code areas

- `cli/cmd/install.go`: route detection, menu timing, repository acquisition, and phase coordination.
- `cli/pkg/installer/plan/`: separate package/configuration plan construction, immutable identities, and action partitioning.
- `cli/pkg/installer/executor.go` or adjacent orchestration: external-first clean flow without changing existing executor semantics.
- `cli/pkg/installer/catalog.go`: repository-independent package catalog and ownership of repository-scoped actions.
- `cli/pkg/installer/ui/`: phase-aware review, progress, cancellation, and partial-result presentation.
- `cli/pkg/installer/report/`: correlated phase outcomes and truthful partial-completion reporting.
- `cli/pkg/installer/transaction/`: expected to retain behavior; integration must continue to expose its existing inventory and recovery outcomes.
- Focused Go tests across command orchestration, planner, executor, UI, report, and transaction integration seams.

## Explicit assumptions

These assumptions make the proposal actionable in automatic execution mode and should be hardened in specification and design:

1. **Route key:** “Existing machine” means a usable repository can be resolved under the current repository-resolution contract. The route is not selected solely by `git` availability.
2. **Single authorization:** The user provides one affirmative authorization after menu selection and package-plan review. Showing the post-clone configuration plan is required for transparency but does not require a second affirmative confirmation.
3. **Deferred target consent:** The first acceptance authorizes the class of managed configuration work described by the installer even though exact paths are derived and shown only after cloning.
4. **Package-plan source:** The binary contains enough menu and action-catalog information to build the pre-clone package plan without repository files.
5. **Action partition:** All selected repository-independent external actions run in the package phase. Repository/submodule materialization belongs to acquisition for the missing-clone route. The configuration plan does not replay package actions.
6. **Acquisition behavior:** New clones continue to use recursive submodule acquisition and the current `DOTFILES_DIR`, `DOTFILES_REPO`, and `DOTFILES_BRANCH` development overrides.
7. **Rollback baseline:** Managed rollback returns configuration targets to their state immediately before configuration execution. Earlier external and acquisition effects remain by design.
8. **Inventory ownership:** The existing versioned inventory remains scoped to the configuration transaction; aggregate reporting links to it rather than broadening its rollback claim.
9. **Retry behavior:** Route selection is evaluated at the start of each invocation. A successfully created usable clone causes a subsequent retry to use the existing single-plan route.
10. **Temporary bootstrap:** PR #87 is treated as separate current-state mitigation. This change may bypass its temporary pre-clone prompt when the two-phase route becomes authoritative, but does not count that mitigation as a deliverable or run base tools twice.
11. **No silent cleanup:** Failed acquisition leaves diagnostic state in place unless existing clone logic already proves cleanup safe; automatic clone deletion is outside this slice.

## Risks and tradeoffs

- **Deferred concrete target review:** Users authorize configuration before exact repository-derived targets can be known. Requiring a clear deferred-scope disclosure and a pre-mutation configuration display reduces, but does not eliminate, this consent tradeoff.
- **Partial irreversible state:** Package, account, service, settings, cache, and clone effects can remain when a later phase fails. Reporting must be precise so users do not expect full rollback.
- **Action duplication or omission:** Splitting one catalog into two plans can accidentally run an action twice or lose it. Explicit ownership and exactly-once tests are mandatory.
- **Existing-flow regression:** A global executor order change would weaken the reviewed safety behavior for existing clones. Route-specific orchestration and compatibility tests are required.
- **Plan correlation drift:** Different options, environment observations, or source refs across phase plans could produce an install the user did not accept. Both plans must bind to the same accepted options and run identity, and the configuration plan must bind to the acquired source.
- **Filesystem baseline change:** Repository-independent setup actions may change paths before configuration planning. The design must audit overlap and state the post-package rollback baseline honestly.
- **Repository acquisition failure after package success:** Network, ref, authentication, destination, or submodule errors leave external effects without installed configuration. The UI must make this a first-class partial outcome.
- **Interim bootstrap duplication:** Coexistence with the short-term Git bootstrap can produce two prompts or two base-tools executions unless the missing-clone route has one authoritative owner.
- **More complex diagnostics:** One aggregate attempt now has two plan identities plus an acquisition step. Without explicit phase status, support output could become harder rather than easier to interpret.

## Rollback strategy

### Implementation rollback

Revert the CLI orchestration, split-plan, UI, and reporting changes to restore the previous single-plan flow. No root configuration migration or on-disk data migration is introduced. Existing retained configuration inventories and backups remain readable and must not be deleted by an implementation rollback.

If the short-term PR #87 mitigation is present, reverting this change returns clean-machine behavior to that mitigation rather than making it part of this proposal.

### Runtime rollback

- Before acceptance: no mutation, so no rollback is needed.
- Package or clone failure: completed external and repository effects remain; the installer reports them but does not attempt automatic reversal.
- Configuration failure: the existing transaction rolls back managed targets to their pre-configuration state and retains inventory/backups for restore or manual recovery.
- Incomplete managed rollback: preserve the current explicit manual-recovery state and artifacts.

## Success criteria

- A machine with neither Git nor a dotfiles clone reaches the menu without any system mutation.
- Declining or cancelling before acceptance runs no package command, creates no clone, and mutates no managed target.
- After acceptance on a missing-clone machine, the base-tools action completes before the first Git-dependent action and before clone acquisition.
- The repository is cloned only after the package phase succeeds and is checked out at the installer-selected ref with current override behavior preserved.
- The configuration plan is built from the acquired repository and the original accepted options, then displayed before managed mutation.
- External actions attempted in the package phase are not replayed during configuration.
- A package failure prevents clone and configuration; a clone failure prevents configuration; a configuration failure invokes the existing managed rollback.
- Reports preserve completed, failed, and skipped package outcomes when a later phase fails and identify repository acquisition separately.
- Reports correlate package and configuration plan identities and expose the managed inventory only when a configuration transaction exists.
- Configuration backups, rollback, retained inventory, manual recovery, and `moonarch-cli restore` compatibility remain unchanged.
- A usable existing clone continues through the current single-plan path with managed targets committed before external actions.
- No root dotfile/configuration content is changed by the implementation itself.
