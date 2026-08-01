# Tasks: Two-Phase Install Flow for Missing Repository Clones

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~600-800 lines across both work units |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 (two independent work units) |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

**Decision needed before apply:** Yes  
**Chained PRs recommended:** Yes  
**Chain strategy:** stacked-to-main  
**400-line budget risk:** High  

**Rationale:** The design explicitly splits into two reviewable work units: (1) phase-scoped plans and bootstrap execution (behavior-ready, not yet routed), and (2) CLI orchestration with two-phase routing. Each work unit delivers independent, testable value and can be reviewed/approved separately. Total estimated lines (~600-800) significantly exceeds the 400-line budget.

---

## Work Unit 1: feat(installer): phase-scoped plans and bootstrap execution

**Goal:** Introduce planner partition, external-only executor, and aggregate report types. Behavior-ready but not yet routed by the command.

**Test runner:** `cd cli && go test ./...`

### Task 1.1: Add PlanRole and InstallationRun to plan package

**Files:**
- `cli/pkg/installer/plan/plan.go` (modify)
- `cli/pkg/installer/plan/plan_test.go` (modify)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/pkg/installer/plan/plan_test.go`:
  - Test `PlanRole` type with constants `PlanRoleSingle`, `PlanRolePackage`, `PlanRoleConfiguration`
  - Test `InstallationRun` struct with `RunID` and frozen `Options` snapshot
  - Test that `InstallationPlan` includes `PlanRole` and incorporates it into fingerprint calculation

GREEN:
- [x] Add `PlanRole` type and constants to `cli/pkg/installer/plan/plan.go`
- [x] Add `InstallationRun` struct to `cli/pkg/installer/plan/plan.go`
- [x] Add `Role` field to `InstallationPlan` struct
- [x] Update fingerprint calculation to include `Role`
- [x] Run `cd cli && go test ./pkg/installer/plan/...`

REFACTOR:
- [x] Ensure `PlanRole` is properly documented
- [x] Verify fingerprint backward compatibility for existing single-plan tests

**Verification:** `cd cli && go test ./pkg/installer/plan/... -v`

---

### Task 1.2: Add PhaseActionCatalog contract to plan package

**Files:**
- `cli/pkg/installer/plan/plan.go` (modify)
- `cli/pkg/installer/plan/plan_test.go` (modify)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/pkg/installer/plan/plan_test.go`:
  - Test `PhaseActionCatalog` interface with `PackageActions(homeDir, opts)` and `ConfigurationActions(repoRoot, homeDir, opts, managedTargets)` methods
  - Test that `Planner` accepts `PhaseActionCatalog` as optional capability
  - Test error when planner configured with old catalog that doesn't implement `PhaseActionCatalog`

GREEN:
- [x] Add `PhaseActionCatalog` interface to `cli/pkg/installer/plan/plan.go`
- [x] Add capability check in planner initialization
- [x] Add typed `PlanError` for missing phase catalog
- [x] Run `cd cli && go test ./pkg/installer/plan/...`

REFACTOR:
- [x] Document the contract clearly
- [x] Ensure backward compatibility with existing `ActionCatalog`

**Verification:** `cd cli && go test ./pkg/installer/plan/... -v`

---

### Task 1.3: Add StartRun, BuildPackage, and BuildConfiguration to Planner

**Files:**
- `cli/pkg/installer/plan/planner.go` (modify)
- `cli/pkg/installer/plan/plan_test.go` (modify)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/pkg/installer/plan/plan_test.go`:
  - Test `StartRun(opts)` returns `InstallationRun` with unique `RunID` and deep-copied options
  - Test `BuildPackage(run, homeDir)` creates package plan without calling discoverer or state reader
  - Test `BuildConfiguration(run, repoRoot, homeDir)` creates configuration plan from acquired source
  - Test both phase plans share same `RunID` and option snapshot but have distinct fingerprints
  - Test input mutation cannot alter a reviewed plan (immutability check)

GREEN:
- [x] Implement `StartRun` in `cli/pkg/installer/plan/planner.go` with run-ID allocation and options cloning
- [x] Implement `BuildPackage` that calls `PhaseActionCatalog.PackageActions` only
- [x] Implement `BuildConfiguration` that calls `PhaseActionCatalog.ConfigurationActions` with managed targets
- [x] Ensure both builders use the same `RunID` and frozen options
- [x] Run `cd cli && go test ./pkg/installer/plan/...`

REFACTOR:
- [x] Factor existing target/action construction logic so legacy `Build` retains semantics
- [x] Ensure `BuildPackage` never invokes `Discoverer.Discover` or `StateReader.Read`

**Verification:** `cd cli && go test ./pkg/installer/plan/... -v`

---

### Task 1.4: Add phase-specific action builders to ActionCatalog

**Files:**
- `cli/pkg/installer/catalog.go` (modify)
- `cli/pkg/installer/system_test.go` or new `cli/pkg/installer/catalog_phase_test.go` (modify or create)

**Strict TDD:**

RED:
- [x] Write failing tests:
  - Test `PackageActions(homeDir, opts)` returns base tools first, excludes submodule action, includes configured package actions
  - Test `ConfigurationActions(repoRoot, homeDir, opts, managedTargets)` returns power-profile actions and zsh-directory action only when appropriate
  - Test zsh mkdir is omitted when managed target owns `~/.config/zsh`
  - Test package/configuration lists are disjoint
  - Test base tools is always first in package actions
  - Test power actions are absent from package plan (deferred until authorization)

GREEN:
- [x] Implement `PackageActions` method on `ActionCatalog` in `cli/pkg/installer/catalog.go`
- [x] Implement `ConfigurationActions` method with target-aware zsh directory ownership rule
- [x] Ensure `BaseToolsAction` is ordered first in package actions
- [x] Exclude submodule action from package actions
- [x] Run `cd cli && go test ./pkg/installer/...`

REFACTOR:
- [x] Ensure legacy `ExternalActions` method remains unchanged for existing flow
- [x] Document the ownership rules clearly

**Verification:** `cd cli && go test ./pkg/installer/... -v`

---

### Task 1.5: Add ExternalOnlyExecutor

**Files:**
- `cli/pkg/installer/executor.go` (modify)
- `cli/pkg/installer/executor_test.go` (modify)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/pkg/installer/executor_test.go`:
  - Test `NewExternalOnlyExecutor(r)` constructor
  - Test `Execute(ctx, plan)` rejects plan with managed targets
  - Test execution in reviewed order with completed/failed/skipped `ActionOutcome` values
  - Test stop-on-first-failure behavior
  - Test no transaction or inventory is created
  - Test existing `Executor.Execute` tests continue to pass (legacy behavior unchanged)

GREEN:
- [x] Add `ExternalOnlyExecutor` struct to `cli/pkg/installer/executor.go`
- [x] Implement `Execute` method that validates plan has no managed targets
- [x] Factor shared external-action loop from `Executor.Execute` so both executors use identical logic
- [x] Ensure `ExternalOnlyExecutor` never creates/calls managed executor
- [x] Run `cd cli && go test ./pkg/installer/...`

REFACTOR:
- [x] Ensure `Executor.Execute` ordering contract is unchanged
- [x] Document the difference between the two executors

**Verification:** `cd cli && go test ./pkg/installer/... -v`

---

### Task 1.6: Add two-phase report types and InventoryPath field

**Files:**
- `cli/pkg/installer/report/report.go` (modify)
- `cli/pkg/installer/transaction/transaction.go` (modify)
- `cli/pkg/installer/report/two_phase_test.go` (create)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/pkg/installer/report/two_phase_test.go`:
  - Test `AttemptOutcome` constants: `completed`, `incomplete`, `failed`, `cancelled`
  - Test `InstallPhase` constants: `package`, `repository`, `configuration`
  - Test `PhaseState` constants: `not-started`, `completed`, `failed`, `skipped`, `cancelled`
  - Test `TransactionState` constants: `not-started`, `started`, `completed`, `not-required`
  - Test `TwoPhaseExecutionReport` struct with `RunID`, `Outcome`, `PrimaryFailedPhase`, `Package`, `Repository`, `Configuration` fields
  - Test `PhaseExecution` struct with `State`, `PlanFingerprint`, `Report` fields
  - Test `ConfigurationExecution` extends `PhaseExecution` with `TransactionState` and `InventoryPath`
  - Test `RepositoryExecution` with state, destination, ref, and cause
  - Test `InventoryPath` field on `ExecutionReport` (in-memory only, no schema change)

GREEN:
- [x] Add aggregate report types to `cli/pkg/installer/report/report.go`
- [x] Add `InventoryPath` field to `ExecutionReport` struct
- [x] Populate `InventoryPath` in `cli/pkg/installer/transaction/transaction.go` from `buildReport` when inventory exists
- [x] Run `cd cli && go test ./pkg/installer/report/... ./pkg/installer/transaction/...`

REFACTOR:
- [x] Document that `InventoryPath` is additive and doesn't change versioned inventory schema
- [x] Ensure existing report tests continue to pass

**Verification:** `cd cli && go test ./pkg/installer/report/... ./pkg/installer/transaction/... -v`

---

### Task 1.7: Work unit 1 verification

**Test command:** `cd cli && go test ./pkg/installer/... -v`

**Acceptance criteria:**
- [x] All existing tests pass
- [x] All new tests pass
- [x] Code compiles without errors
- [x] No regression in legacy single-plan behavior
- [x] Behavior is ready but not yet routed by command

**Rollback boundary:** Revert changes to `plan.go`, `planner.go`, `catalog.go`, `executor.go`, `report.go`, `transaction.go`, and their test files. Existing command routing is unaffected.

---

## Work Unit 2: feat(cli): orchestrate missing-clone installs in two phases

**Goal:** Wire up command routing, repository acquisition, single-consent UI, phase-aware output, and end-to-end flow tests.

**Test runner:** `cd cli && go test ./...`

### Task 2.1: Add RepositoryRequest and RepositoryAcquirer

**Files:**
- `cli/cmd/repository_acquirer.go` (create)
- `cli/cmd/repository_acquirer_test.go` (create)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/cmd/repository_acquirer_test.go`:
  - Test `RepositoryRequest` struct with `Destination`, `Ref`, `URL` fields
  - Test `RepositoryAcquisition` struct with `Root`, `Destination`, `Ref` fields
  - Test `RepositoryAcquirer` interface with `Acquire(ctx, request, output)` method
  - Test read-only destination preflight rejects canonical destination that exists but lacks `.git`
  - Test preflight failure occurs before acceptance and never deletes/adopts/overwrites
  - Test frozen version/dev/override ref reaches Git seam
  - Test clone uses recursive submodules
  - Test conflict directory is retained on failure
  - Test acquisition failure performs no cleanup
  - Test local-Git integration coverage is skippable under `testing.Short()`

GREEN:
- [x] Create `cli/cmd/repository_acquirer.go`
- [x] Add `RepositoryRequest` and `RepositoryAcquisition` types
- [x] Add `RepositoryAcquirer` interface
- [x] Implement read-only destination preflight
- [x] Implement production `RepositoryAcquirer` with context-aware Git command seam
- [x] Use frozen request instead of second environment lookup
- [x] Preserve existing clone contract (absent destination: clone; usable destination: fetch/checkout; non-repository: fail)
- [x] Run `cd cli && go test ./cmd/...`

REFACTOR:
- [x] Extract from `install.go` if appropriate
- [x] Keep `ensureRepositoryClone` as narrow compatibility wrapper if needed
- [x] Remove `confirmAndInstallGit` branch from `runInstall`

**Verification:** `cd cli && go test ./cmd/... -v`

---

### Task 2.2: Add two-phase review API to UI package

**Files:**
- `cli/pkg/installer/ui/review.go` (modify)
- `cli/pkg/installer/ui/two_phase.go` (create)
- `cli/pkg/installer/ui/review_test.go` (modify)
- `cli/pkg/installer/ui/two_phase_test.go` (create)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/pkg/installer/ui/review_test.go` and `cli/pkg/installer/ui/two_phase_test.go`:
  - Test `TwoPhaseReviewDetails` struct with `RepositoryDestination` and `RepositoryRef`
  - Test `ReviewPackagePlanWithContext` displays all Phase A actions and irreversible classifications
  - Test review shows frozen repository destination and ref
  - Test review states that exact managed targets are deferred until source is acquired
  - Test review states accepting authorizes package, acquisition, and configuration
  - Test review states rollback applies only to managed targets
  - Test decline/escape/Ctrl-C returns `accepted=false` without invoking executor or acquirer
  - Test `DisplayConfigurationPlan` is output-only (no input, no confirmation)
  - Test configuration display shows concrete fingerprint, targets, remaining actions
  - Test configuration display states authorization already granted
  - Test cancellation before config execution produces no transaction

GREEN:
- [x] Add `TwoPhaseReviewDetails` struct to `cli/pkg/installer/ui/review.go`
- [x] Implement `ReviewPackagePlanWithContext` reusing Bubble Tea review mechanics
- [x] Create `cli/pkg/installer/ui/two_phase.go`
- [x] Implement `DisplayConfigurationPlan` as output-only function
- [x] Add phase-status formatting helpers
- [x] Test Bubble Tea behavior via direct `Model.Update()` state transitions
- [x] Run `cd cli && go test ./pkg/installer/ui/...`

REFACTOR:
- [x] Ensure legacy `RunWithContext` contract is unchanged
- [x] Document the difference between review and display

**Verification:** `cd cli && go test ./pkg/installer/ui/... -v`

---

### Task 2.3: Add printTwoPhaseExecutionReport

**Files:**
- `cli/cmd/install_report.go` (create)
- `cli/cmd/install_report_test.go` (create)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/cmd/install_report_test.go`:
  - Test `printTwoPhaseExecutionReport` labels each phase and both plan fingerprints
  - Test emits `completed` only when all applicable phases complete
  - Test identifies package/acquisition/configuration primary failure without discarding earlier outcomes
  - Test says `configuration transaction not started` and omits inventory references when not started
  - Test states package/clone effects remain for all partial outcomes
  - Test describes rollback as managed-target rollback only
  - Test includes retained inventory/recovery artifacts when available
  - Test existing `printExecutionReport` tests continue to pass

GREEN:
- [x] Create `cli/cmd/install_report.go`
- [x] Implement `printTwoPhaseExecutionReport` function
- [x] Leave existing `printExecutionReport` unchanged
- [x] Run `cd cli && go test ./cmd/...`

REFACTOR:
- [x] Ensure output is clear and actionable
- [x] Verify no false rollback wording

**Verification:** `cd cli && go test ./cmd/... -v`

---

### Task 2.4: Add runInstallWithDeps and phase routing

**Files:**
- `cli/cmd/install_flow.go` (create)
- `cli/cmd/install_flow_test.go` (create)

**Strict TDD:**

RED:
- [x] Write failing tests in `cli/cmd/install_flow_test.go`:
  - Test `runInstallWithDeps` accepts injected dependencies for deterministic testing
  - Test `runExistingCloneInstall` uses legacy helper only
  - Test `runMissingCloneInstall` uses two-phase helper
  - Test existing clone uses only the legacy helper regardless of Git availability
  - Test no clone selects two-phase helper regardless of Git availability
  - Test route is re-evaluated on each invocation
  - Test event log proves menu/review occur before runner, acquirer, transaction, or command probe calls
  - Test canonical non-repository destination fails without mutation
  - Test coordinator data flow: locator + preflight → menu → frozen run + request → package plan → acceptance → executor → acquirer → config plan → display → transaction → aggregate report

GREEN:
- [x] Create `cli/cmd/install_flow.go`
- [x] Add `runInstallWithDeps` with phase interfaces/factories and dependency injection
- [x] Implement `runExistingCloneInstall` helper preserving current code path
- [x] Implement `runMissingCloneInstall` coordinator with explicit phase boundaries
- [x] Ensure coordinator stops at every phase boundary (package failure, acquisition failure, config planning/display cancellation, managed transaction failure, Phase-B external failure)
- [x] Use `ExternalOnlyExecutor` for Phase B with no managed targets
- [x] Report configuration transaction as `not required` when no targets
- [x] Run `cd cli && go test ./cmd/...`

REFACTOR:
- [x] Ensure phase boundaries are explicit and testable
- [x] Document the coordinator's stop conditions

**Verification:** `cd cli && go test ./cmd/... -v`

---

### Task 2.5: Route runInstall through explicit existing/missing helpers

**Files:**
- `cli/cmd/install.go` (modify)
- `cli/cmd/install_test.go` (modify)

**Strict TDD:**

RED:
- [x] Write/update failing tests in `cli/cmd/install_test.go`:
  - Test `runInstall` routes through `runInstallWithDeps` with production dependencies
  - Test keep Cobra registration, repository helpers, and `installDiscoverer`
  - Test remove direct pre-menu clone
  - Test remove `confirmAndInstallGit` path (Phase A is sole owner)
  - Test existing tests continue to pass

GREEN:
- [x] Modify `cli/cmd/install.go` to route `runInstall` through `runInstallWithDeps`
- [x] Keep Cobra registration, repository helpers, and `installDiscoverer`
- [x] Remove direct pre-menu clone path
- [x] Remove `confirmAndInstallGit` call from `runInstall`
- [x] Keep `ensureRepositoryClone` as compatibility wrapper if needed
- [x] Run `cd cli && go test ./cmd/...`

REFACTOR:
- [x] Ensure backward compatibility
- [x] Clean up any dead code

**Verification:** `cd cli && go test ./cmd/... -v`

---

### Task 2.6: End-to-end flow tests with fakes

**Files:**
- `cli/cmd/install_flow_test.go` (extend)
- `cli/cmd/install_test.go` (extend)

**Strict TDD:**

RED:
- [x] Write failing end-to-end tests using injected fakes:
  - Test clean machine (no Git, no clone): menu shown, package phase runs base tools first, repository acquired, config plan displayed, managed transaction executes
  - Test Git present, no clone: same two-phase route
  - Test existing clone: legacy single-plan flow unchanged
  - Test package failure: stops before acquisition and configuration, reports completed/failed/skipped
  - Test acquisition failure: stops before configuration, retains package effects
  - Test configuration failure: managed rollback executes, package/clone effects remain
  - Test user declines acceptance: no mutation occurs
  - Test user cancels before config execution: no transaction created
  - Test retry after successful acquisition: routes to existing-clone path
  - Test event log proves no pre-acceptance mutation (no package commands, no clone, no directory creation, no managed targets)
  - Test event log proves no `DetectPowerProfiles` call before acceptance
  - Test event log proves no command probe calls before acceptance

GREEN:
- [x] Implement fake `CommandRunner`, `RepositoryAcquirer`, `Executor`, and `Transaction`
- [x] Build event log to track invocation order
- [x] Run all scenarios
- [x] Run `cd cli && go test ./cmd/... -v`

REFACTOR:
- [x] Ensure fakes are reusable
- [x] Document the test scenarios

**Verification:** `cd cli && go test ./cmd/... -v`

---

### Task 2.7: Work unit 2 verification

**Test commands:**
- `cd cli && go test ./cmd/... -v`
- `cd cli && go test ./...`

**Acceptance criteria:**
- [x] All existing tests pass
- [x] All new tests pass
- [x] Code compiles without errors
- [x] Route selection works correctly (existing clone → single-plan, no clone → two-phase)
- [x] Pre-acceptance is read-only (no mutation, no command execution)
- [x] Single consent authorizes both phases
- [x] Phase boundaries are enforced (package failure stops acquisition/config, acquisition failure stops config)
- [x] Aggregate reporting is truthful (no false rollback claims, no false success labels)
- [x] Existing-clone behavior is unchanged
- [x] End-to-end flow tests pass with fakes

**Rollback boundary:** Revert changes to `install.go`, `install_flow.go`, `repository_acquirer.go`, `install_report.go`, and their test files. Work unit 1 remains intact.

---

## Delivery Notes

**PR 1:** Work Unit 1 (installer package changes)  
**PR 2:** Work Unit 2 (command orchestration changes)

Each PR is independently reviewable and testable. PR 1 delivers behavior-ready infrastructure. PR 2 wires it into the command routing.

**Smoke test scenarios (before release):**
1. No Git, no clone → full two-phase flow
2. Git present, no clone → two-phase flow
3. Existing usable clone → single-plan flow
4. Package failure → stops before acquisition
5. Acquisition failure → stops before configuration
6. Configuration failure → managed rollback, package/clone remain
7. Retry after successful acquisition → existing-clone path
