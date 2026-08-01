# Design: Two-Phase Install Flow for Missing Repository Clones

## Summary

The installer will select its route from a read-only repository lookup at process start:

- A resolvable usable clone retains the current single-plan path without changing its confirmation screen, report shape, or `managed transaction -> external actions` order.
- No resolvable clone uses a new, route-local two-phase coordinator. It collects menu choices first, reviews one repository-independent package plan, executes that plan, acquires the repository at the frozen version-selected ref, builds and displays a configuration plan from that exact checkout, then executes the existing managed transaction.

The implementation will not invert `installer.Executor.Execute`. A new external-only executor handles Phase A; the existing executor remains the configuration executor and remains the sole executor for the existing-clone route.

## Design decisions

### 1. Route selection is repository-state based

Refactor the repository lookup behind a read-only locator used before the menu:

```go
type RepositoryState struct {
    Root  string
    Found bool
}

type RepositoryLocator interface {
    Locate(startDir string) (RepositoryState, error)
}
```

`Locate` preserves the current lookup precedence: current-directory ancestry, `DOTFILES_DIR`, then `$HOME/.cache/dotfiles`. It uses filesystem metadata only; it does not invoke Git, clone, create directories, or update an existing checkout.

The locator distinguishes these outcomes:

1. `Found=true`: a repository is resolvable under the existing repository-root contract. `runInstall` calls a `runExistingCloneInstall` helper that retains the current code path: menu, `newInstallPlanner().Build`, `transaction.New`, `installer.NewExecutor`, `ui.RunWithContext`, and `printExecutionReport`.
2. `Found=false, err=nil`: enter `runMissingCloneInstall`.
3. `err!=nil`: stop before the menu rather than treating permission/read failures as a missing clone.

Git availability is deliberately not an input to route selection. Therefore a machine with Git but no clone follows the missing-clone route, and every invocation calls the locator again instead of caching a previous route.

For the missing route, build a frozen `RepositoryRequest` before the menu review:

```go
type RepositoryRequest struct {
    Destination string
    Ref         string
    URL         string // internal only; do not need to render it
}
```

It freezes the existing `DOTFILES_DIR`, `DOTFILES_REPO`, `DOTFILES_BRANCH`, and binary `Version` rules. A read-only `Preflight` rejects a canonical destination that already exists but lacks a usable `.git` marker. This failure occurs before acceptance and before any package action, and never deletes, adopts, or overwrites the directory.

### 2. Two immutable plans share one run identity

Extend `plan` with a run snapshot and phase-specific builders:

```go
type InstallationRun struct {
    RunID string
    // stores a deep-copied accepted Options snapshot
}

func (p *Planner) StartRun(opts Options) InstallationRun
func (p *Planner) BuildPackage(run InstallationRun, homeDir string) (InstallationPlan, error)
func (p *Planner) BuildConfiguration(
    run InstallationRun, repoRoot, homeDir string,
) (InstallationPlan, error)
```

`StartRun` is the only run-ID allocation for the missing-clone path and deep-copies `Groups` and `ExcludePackages`. Both plans receive that `RunID` and the same immutable option snapshot, but have independently calculated fingerprints. Add a `PlanRole` (`single`, `package`, `configuration`) to `InstallationPlan` and its canonical fingerprint input so a phase plan is unambiguous even when it has no targets or actions.

`BuildPackage` must not take a repository root. It calls neither `Discoverer.Discover` nor `StateReader.Read`; it only obtains phase-A actions from a new optional catalog capability:

```go
type PhaseActionCatalog interface {
    PackageActions(homeDir string, opts Options) ([]ExternalAction, error)
    ConfigurationActions(
        repoRoot, homeDir string,
        opts Options,
        managedTargets []Target,
    ) ([]ExternalAction, error)
}
```

`BuildConfiguration` reuses the existing discovery, source binding, destination pre-state capture, validation, sorting, and fingerprinting logic after acquisition. It invokes `ConfigurationActions` only after managed targets are known, which lets the catalog prevent filesystem ownership overlap. The legacy `Planner.Build` and `ActionCatalog.ExternalActions` remain the implementation of the existing single-plan behavior.

If a caller configures a planner with an old catalog that does not implement `PhaseActionCatalog`, a phase builder returns a typed `PlanError` rather than silently falling back to the legacy all-actions catalog.

### 3. Action ownership is explicit and exactly once

`installer.ActionCatalog` will add phase-specific action constructors while retaining its existing `ExternalActions(repoRoot, homeDir, opts)` implementation and order for legacy installs.

| Owner on missing-clone route | Actions |
| --- | --- |
| **Phase A package plan** | `BaseToolsAction` first; paru cleanup/bootstrap/build when needed; configured package installation; default-shell change; upower enablement; font-cache refresh; selected GTK settings; selected Hyprland plugin actions. |
| **Repository acquisition** | Clone/update at the frozen ref and recursive submodule materialization. This replaces the catalog's `update git submodules` action for this route. |
| **Phase B configuration plan** | Managed targets discovered from the acquired checkout; power-profile actions resolved after authorization; and the standalone zsh-directory action only when no managed target owns `~/.config/zsh`. |
| **Existing-clone route** | The current unified catalog and current action order, unchanged. |

The package catalog assigns local `Order` values after constructing its action slice, so `BaseToolsAction` is always first. Consequently Git is installed before `bootstrap paru`, any later Git-dependent package action, and repository acquisition.

The catalog must not construct or filter a submodule action for Phase A. It is absent from the package plan, and `RepositoryAcquirer.Acquire` owns submodule materialization exactly once through the current `git clone --recurse-submodules` / `git submodule update --init --recursive` behavior.

#### Pre-acceptance power-profile probe boundary

`newInstallPlanner()` currently calls `DetectPowerProfiles()`, which invokes `systemctl`. The missing-clone package planner must not call it: the specification forbids command execution before acceptance. `DetectParu()` is permitted because `exec.LookPath` is a read-only path lookup, not a child-command execution.

Power-profile actions are therefore configuration-owned only on the missing-clone route. After Phase A and acquisition have succeeded, the configuration planner receives a fresh `PowerProfilesState` and derives those actions before displaying the configuration plan. This preserves a reviewed immutable plan at execution time without running `systemctl` before authorization. The initial review discloses that the post-acquisition configuration plan can include configuration-owned system actions as well as concrete managed targets.

#### Filesystem-overlap audit

The current direct filesystem setup action is `create zsh configuration directory`. It cannot remain in Phase A because a fetched repository can own `~/.config/zsh` as a managed `CopyTree` target.

`ConfigurationActions` receives the discovered target list and applies this rule:

- If a target is `~/.config/zsh` or encloses/is enclosed by that path, the managed transaction owns the path and the standalone mkdir action is omitted as redundant.
- Otherwise, the mkdir action belongs only to Phase B and is listed in the displayed configuration plan.

Thus a package-phase action never establishes pre-state for a destination later claimed by the transaction. Future direct filesystem actions must declare their owned destination in the same catalog-level audit and be assigned to one owner before they can be added to a phase list.

Catalog tests will compare the package list, acquisition-owned submodule operation, and configuration list to prove that no action description can be scheduled twice or disappear unintentionally.

### 4. One authorization, then informational configuration display

Add a package-specific review API in `ui` while preserving `ui.RunWithContext` for the legacy route:

```go
type TwoPhaseReviewDetails struct {
    RepositoryDestination string
    RepositoryRef         string
}

func ReviewPackagePlanWithContext(
    ctx context.Context,
    p plan.InstallationPlan,
    details TwoPhaseReviewDetails,
    input io.Reader,
    output io.Writer,
    run ProgramRunner,
) (accepted bool, err error)

func DisplayConfigurationPlan(
    output io.Writer,
    p plan.InstallationPlan,
) error
```

`ReviewPackagePlanWithContext` reuses the current Bubble Tea review mechanics but has no executor. It displays:

- all Phase A external actions and each irreversible classification;
- the frozen repository destination and ref;
- that exact managed targets and any configuration-owned actions are deferred until the source is acquired;
- that accepting authorizes package execution, acquisition, and configuration; and
- that automatic rollback applies only to managed targets, never packages, services, settings, caches, or the clone.

Its only affirmative input is the existing review acceptance (`y`/`enter`). Decline, escape, Ctrl-C, menu cancellation, or review failure returns before an executor or acquirer is invoked.

`DisplayConfigurationPlan` is deliberately output-only: it takes no input, starts no confirmation model, and renders no yes/no prompt. It displays the concrete configuration fingerprint, targets, remaining external actions, and the statement that authorization was already granted. Immediately after rendering, the coordinator checks `ctx.Err()` before starting the configuration executor. A cancellation observed at this boundary produces a partial report with no configuration transaction or inventory. This is a display checkpoint, not a second consent.

After acceptance, phase progress and errors are printed with explicit `Package`, `Repository acquisition`, and `Configuration` labels. The UI never describes earlier package or clone effects as rollbackable.

### 5. Repository acquisition is a testable phase

Refactor `ensureRepositoryClone` behind a command-facing seam:

```go
type RepositoryAcquirer interface {
    Acquire(
        ctx context.Context,
        request RepositoryRequest,
        output io.Writer,
    ) (RepositoryAcquisition, error)
}

type RepositoryAcquisition struct {
    Root        string
    Destination string
    Ref         string
}
```

The production acquirer reuses the current clone contract and uses the frozen request, not a second environment lookup:

- absent destination: `git clone --recurse-submodules --branch <ref>`;
- usable destination observed due to a race: current fetch, detached checkout, and recursive submodule update behavior;
- non-repository destination: fail without deletion, overwrite, or adoption.

Use context-aware command execution in the refactored implementation. Keep `ensureRepositoryClone` as a narrow compatibility wrapper if existing tests or callers need it; remove the `confirmAndInstallGit` branch from `runInstall` because Phase A is now the sole owner of base-tools/Git installation.

Acquisition failures retain any clone directory already created. The acquisition result records only destination and ref for user-facing output; it does not print a potentially credential-bearing overridden repository URL.

### 6. Execution orchestration and failure boundaries

Add `installer.ExternalOnlyExecutor` alongside, not in place of, `installer.Executor`:

```go
type ExternalOnlyExecutor struct { /* CommandRunner */ }

func NewExternalOnlyExecutor(r external.CommandRunner) *ExternalOnlyExecutor
func (e *ExternalOnlyExecutor) Execute(
    ctx context.Context,
    p plan.InstallationPlan,
) (*report.ExecutionReport, error)
```

It rejects a plan with managed targets, executes reviewed actions in order, and produces completed/failed/skipped `ActionOutcome` values without constructing a transaction or inventory. Factor the action-loop implementation out of `Executor.Execute` so both executors use identical stop-on-first-failure reporting. `Executor.Execute` retains its current managed-first behavior.

The missing-clone coordinator has this data flow:

```text
read-only locator + destination preflight
  -> menu result
  -> frozen InstallationRun + repository request
  -> immutable Phase A package plan
  -> single package-plan acceptance
  -> ExternalOnlyExecutor (Phase A)
  -> RepositoryAcquirer (clone/ref/submodules)
  -> immutable Phase B configuration plan from acquired Root
  -> informational configuration-plan display
  -> existing Transaction + installer.Executor (Phase B)
  -> aggregate two-phase report
```

The coordinator stops at every phase boundary:

1. Package failure: mark remaining package actions skipped; do not acquire or construct/configure Phase B. Earlier external effects remain.
2. Acquisition failure: do not build the configuration plan or construct a transaction. Retain package effects and the partial clone for diagnosis.
3. Configuration planning/display cancellation: do not construct a transaction or mutate managed targets.
4. Managed transaction failure: preserve the current transaction backup, rollback, inventory, drift detection, and manual-recovery behavior. No package or acquisition action is rolled back.
5. A Phase-B external action failure after a successful managed transaction follows the existing executor contract: the committed managed transaction is retained, the failed external action is reported, and no false rollback is claimed. This is distinct from a managed-target mutation failure, which continues to trigger the existing rollback.

For Phase B with no managed targets, use `ExternalOnlyExecutor` rather than fabricating an empty transaction. Report the configuration transaction as `not required`; do not fabricate an inventory.

### 7. Aggregate reporting keeps recovery scoped to managed targets

Keep `report.ExecutionReport` as the per-plan execution result and add a separate aggregate model:

```go
type TwoPhaseExecutionReport struct {
    RunID              string
    Outcome            AttemptOutcome // completed, incomplete, failed, cancelled
    PrimaryFailedPhase InstallPhase
    Package            PhaseExecution
    Repository         RepositoryExecution
    Configuration      ConfigurationExecution
}

type PhaseExecution struct {
    State           PhaseState // not-started, completed, failed, skipped, cancelled
    PlanFingerprint string
    Report          *ExecutionReport
}

type ConfigurationExecution struct {
    PhaseExecution
    TransactionState TransactionState // not-started, started, completed, not-required
    InventoryPath    string
}
```

`RepositoryExecution` includes its state, destination, ref, and cause. `Package.Report` retains per-action outcomes. `Configuration.Report` retains managed-target outcomes, config-owned external outcomes, backup paths, and rollback details.

Add `InventoryPath` to the in-memory `ExecutionReport` and fill it from `transaction.Transaction.buildReport` only when a managed inventory was actually created. Do not change the versioned `transaction.Inventory` schema: it remains authoritative only for managed recovery, and it does not claim package or clone ownership. The aggregate report correlates it to the configuration fingerprint and shared run ID in memory/output.

Add `printTwoPhaseExecutionReport` in `cmd` rather than changing `printExecutionReport` used by the existing route. It must:

- label each phase and both plan fingerprints;
- emit `completed` only when all applicable phases complete;
- identify a package, acquisition, or configuration primary failure without discarding earlier outcomes;
- say `configuration transaction not started` and omit inventory references when it did not start;
- state that package/clone effects remain for all partial outcomes; and
- describe rollback as **managed-target rollback only**, including retained inventory/recovery artifacts when available.

## Proposed file changes

| File | Change |
| --- | --- |
| `cli/cmd/install.go` | Keep Cobra registration, repository helpers, and `installDiscoverer`; route `runInstall` through explicit existing/missing helpers; remove the direct pre-menu clone and `confirmAndInstallGit` path. |
| `cli/cmd/install_flow.go` (new) | Add `runInstallWithDeps`, `runExistingCloneInstall`, `runMissingCloneInstall`, phase interfaces/factories, and deterministic dependency injection for orchestration tests. |
| `cli/cmd/repository_acquirer.go` (new, or extracted from `install.go`) | Add `RepositoryRequest`, read-only destination preflight, production `RepositoryAcquirer`, context-aware Git command seam, and the compatibility wrapper around `ensureRepositoryClone` if retained. |
| `cli/cmd/install_report.go` (new) | Add `printTwoPhaseExecutionReport`; leave existing single-plan printer unchanged. |
| `cli/pkg/installer/plan/plan.go` | Add `PlanRole`, `InstallationRun`, option cloning, phase catalog contract, and phase/fingerprint metadata. |
| `cli/pkg/installer/plan/planner.go` | Add `StartRun`, `BuildPackage`, and `BuildConfiguration`; factor existing target/action construction so legacy `Build` retains its current semantics. |
| `cli/pkg/installer/catalog.go` | Add package/configuration action builders and the target-aware zsh directory ownership rule; preserve `ExternalActions` for the existing flow. |
| `cli/pkg/installer/executor.go` | Add `ExternalOnlyExecutor` and a shared external-action loop; do not change the ordering contract of `Executor.Execute`. |
| `cli/pkg/installer/report/report.go` | Add two-phase report/state types and in-memory `InventoryPath` on `ExecutionReport`. |
| `cli/pkg/installer/transaction/transaction.go` | Populate `ExecutionReport.InventoryPath` only; retain transaction and inventory behavior. |
| `cli/pkg/installer/ui/review.go`, `ui/run.go` | Add review metadata/output support and package-review API without altering the legacy `RunWithContext` contract. |
| `cli/pkg/installer/ui/two_phase.go` (new) | Add output-only configuration-plan display and phase-status formatting. |

## Test design

All new tests should be table-driven where multiple routes/failures are covered, use `t.TempDir()` for filesystem state, and use injected command/acquisition/executor seams rather than package managers or a real home directory. Bubble Tea behavior should be tested by direct `Model.Update()` state transitions.

| Requirement coverage | Test files | Key assertions |
| --- | --- | --- |
| Route selection and read-only pre-acceptance | `cli/cmd/install_flow_test.go`, `cli/cmd/install_test.go` | Existing clone uses only the legacy helper; no clone selects the two-phase helper regardless of Git availability; route is re-evaluated; event log proves menu/review occur before runner, acquirer, transaction, or command probe calls; canonical non-repository destination fails without mutation. |
| Package plan construction and immutability | `cli/pkg/installer/plan/plan_test.go` | `BuildPackage` never invokes discoverer/state reader or uses a repo root; both phase plans share one run/options snapshot but have distinct fingerprints; action ordering and input mutation cannot alter a reviewed plan. |
| Catalog ownership and overlap audit | `cli/pkg/installer/system_test.go` or new `catalog_phase_test.go` | Base tools is first; Phase A has no submodule action; acquisition owns recursive submodules; package/configuration lists are disjoint; power actions are absent before authorization; zsh mkdir is omitted when a managed target owns the path and present otherwise. |
| External-only execution | `cli/pkg/installer/executor_test.go` | Phase A reports completed/failed/skipped actions in reviewed order, rejects managed targets, and never creates/calls a managed executor. Existing tests continue to prove legacy managed-before-external ordering. |
| Repository acquisition | `cli/cmd/install_test.go`, `cli/cmd/repository_acquirer_test.go` (new) | Frozen version/dev/override ref reaches the Git seam; clone uses recursive submodules; conflict directory is retained; acquisition failure performs no cleanup; no Git lookup controls route choice. Local-Git integration coverage remains skippable under `testing.Short()`. |
| Single consent and plan visibility | `cli/pkg/installer/ui/review_test.go`, new `ui/two_phase_test.go`, `cmd/install_flow_test.go` | Initial screen includes irreversible Phase-A actions, destination/ref, deferred targets/actions, and rollback disclosure; decline invokes no phase; configuration display has no confirmation transition or input; cancellation before config execution produces no transaction. |
| Configuration transaction and no replay | `cli/cmd/install_flow_test.go`, `cli/pkg/installer/transaction/*_test.go` | Config planner receives the acquired root and accepted snapshot; Phase-A actions never appear in Phase B; managed failures retain existing rollback/inventory behavior; Phase-B external failure is reported without claiming package/clone rollback. Existing restore and rollback tests remain unchanged regression coverage. |
| Aggregate reporting | new `cli/pkg/installer/report/two_phase_test.go`, `cli/cmd/install_report_test.go` | Full success contains two fingerprints and inventory; package/acquisition failures are incomplete rather than successful; no transaction means no inventory; configuration rollback output names managed-only recovery and preserves earlier phase outcomes. |

Focused verification commands:

```bash
cd cli && go test ./cmd ./pkg/installer/... 
cd cli && go test ./...
```

Implementation should follow strict TDD: add the focused failing test for each work unit first, implement the smallest behavior to pass it, then run the broader module suite.

## Delivery and rollout

This is cross-cutting and likely to exceed a comfortable single-review size. Use two reviewable work units, each with its tests:

1. `feat(installer): add phase-scoped plans and bootstrap execution` — planner/catalog partition, external-only executor, aggregate report types, transaction report field, and focused tests. This is behavior-ready but not yet selected by the command.
2. `feat(cli): orchestrate missing-clone installs in two phases` — typed repository acquisition, command routing, single-consent UI, phase-aware output, and end-to-end fake-driven flow tests.

No root dotfiles, configuration content, CI, backup retention, restore command, or inventory JSON migration is required. Rollout is automatic by repository availability; there is no feature flag. Before release, perform a disposable supported-Arch smoke test for: no Git/no clone, Git/no clone, an existing usable clone, package failure, acquisition failure, and managed configuration failure. Confirm that a subsequent invocation after successful acquisition follows the existing-clone path.

Implementation rollback is a normal revert of the two work units. Runtime rollback remains intentionally limited: package and repository side effects are retained; only managed configuration targets can be restored through the existing transaction/restore contract.

## Risks and mitigations

- **Existing-route regression:** Legacy `Planner.Build`, `ActionCatalog.ExternalActions`, `ui.RunWithContext`, `printExecutionReport`, and `installer.Executor` retain their contracts. Route and executor-order regression tests are mandatory.
- **Deferred consent:** The initial review cannot show targets that do not exist locally. The disclosure names this limitation, the destination/ref, configuration-owned actions, and the managed-only rollback boundary; the post-acquisition display provides concrete transparency without a second consent.
- **Action duplication/omission:** Phase-specific catalog tests make acquisition a first-class owner of submodules and audit all direct filesystem actions.
- **Pre-acceptance mutation or command execution:** The missing path uses only locator/preflight metadata, menu input, `exec.LookPath`, and package-plan construction before acceptance. It must not call `DetectPowerProfiles`, external runners, Git, transactions, or directory creation.
- **Partial irreversible state:** Aggregate states and output explicitly distinguish completed, failed, and incomplete attempts. No automatic package/clone cleanup or false rollback wording is introduced.
- **Post-package baseline:** Configuration target pre-state is intentionally captured after package execution and acquisition. The target-aware zsh-directory rule prevents Phase A from contaminating a managed destination baseline.
- **Source/environment races:** The configuration plan binds the acquired source and post-package destination state; existing transaction source-drift and destination-drift safeguards remain authoritative. Process termination during package/acquisition has no automatic recovery promise.
