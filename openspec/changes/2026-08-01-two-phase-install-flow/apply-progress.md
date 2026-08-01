# Apply Progress: Two-Phase Install Flow — Work Unit 1

## Status

- Work Unit: 1 of 2
- Scope: `feat(installer): phase-scoped plans and bootstrap execution` — behavior-ready infrastructure, not yet routed by command
- All Work Unit 1 tasks implemented and verified
- Last test run: `cd cli && go test ./...` — PASS

## TDD Cycle Evidence

| Task | RED test | GREEN implementation | Test command / result |
|------|----------|---------------------|----------------------|
| 1.1 PlanRole / InstallationRun | `TestPlanRoleConstants`, `TestInstallationRun_HoldsRunIDAndOptionsSnapshot`, `TestInstallationPlan_RoleIncorporatedIntoFingerprint` | Added `PlanRole`, `InstallationRun`, `Role` field, `cloneOptions`, and `role,omitempty` fingerprint input | `cd cli && go test ./pkg/installer/plan/...` PASS |
| 1.2 PhaseActionCatalog | `TestPhaseActionCatalogContract`, `TestPlanner_PhaseBuildsRequirePhaseCatalog` | Added `PhaseActionCatalog` interface and typed `PlanError` for missing capability | `cd cli && go test ./pkg/installer/plan/...` PASS |
| 1.3 Phase builders | `TestPlanner_StartRun_AllocatesRunIDAndCopiesOptions`, `TestPlanner_BuildPackage_NoDiscoveryOrStateRead`, `TestPlanner_BuildConfiguration_FromDiscoveredSource`, `TestPlanner_PhasePlansShareRunIDAndOptionsButHaveDistinctFingerprints`, `TestPlanner_PhasePlanImmutableAgainstInputMutation` | Added `StartRun`, `BuildPackage`, `BuildConfiguration`; factored `buildTargets`/`sortActions` from legacy `Build` | `cd cli && go test ./pkg/installer/plan/...` PASS |
| 1.4 Phase catalog | `TestActionCatalog_PackageActions_*`, `TestActionCatalog_ConfigurationActions_*`, `TestActionCatalog_PhaseListsAreDisjoint`, `TestActionCatalog_ExternalActionsUnchanged` | Added `PackageActions`, `ConfigurationActions`, ownership helpers; factored shared action constructors; preserved `ExternalActions` | `cd cli && go test ./pkg/installer/...` PASS |
| 1.5 ExternalOnlyExecutor | `TestExternalOnlyExecutor_RejectsManagedTargets`, `TestExternalOnlyExecutor_RunsActionsInReviewedOrder`, `TestExternalOnlyExecutor_StopsOnFirstFailure`, `TestExternalOnlyExecutor_NeverCallsManagedExecutor` | Added `ExternalOnlyExecutor`, `NewExternalOnlyExecutor`, and shared `executeExternalActions`; preserved `Executor` ordering | `cd cli && go test ./pkg/installer/...` PASS |
| 1.6 Aggregate report types | `TestTwoPhaseOutcomeConstants`, `TestInstallPhaseConstants`, `TestPhaseStateConstants`, `TestTransactionStateConstants`, `TestTwoPhaseExecutionReport_Struct`, `TestExecutionReport_InventoryPathField` | Added two-phase report types and `InventoryPath` in `report.go`; populated `InventoryPath` from `transaction.buildReport` | `cd cli && go test ./pkg/installer/report/... ./pkg/installer/transaction/...` PASS |
| 1.7 Work unit verification | All existing + new tests pass | — | `cd cli && go test ./...` PASS |

## Files Changed

- `cli/pkg/installer/plan/plan.go` — `PlanRole`, `InstallationRun`, `Role`, `PhaseActionCatalog`, `cloneOptions`, fingerprint role
- `cli/pkg/installer/plan/planner.go` — `StartRun`, `BuildPackage`, `BuildConfiguration`, factored `buildTargets`/`sortActions`
- `cli/pkg/installer/plan/plan_test.go` — new tests for 1.1–1.3
- `cli/pkg/installer/catalog.go` — `PackageActions`, `ConfigurationActions`, ownership helpers, factored action constructors
- `cli/pkg/installer/catalog_phase_test.go` — new tests for 1.4
- `cli/pkg/installer/executor.go` — `ExternalOnlyExecutor`, shared external-action loop
- `cli/pkg/installer/executor_test.go` — new tests for 1.5
- `cli/pkg/installer/report/report.go` — two-phase types, `InventoryPath` on `ExecutionReport`
- `cli/pkg/installer/report/two_phase_test.go` — new tests for 1.6
- `cli/pkg/installer/transaction/transaction.go` — populate `InventoryPath` in `buildReport`
- `openspec/changes/2026-08-01-two-phase-install-flow/tasks.md` — completed Work Unit 1 checkboxes

## Deviations from Design

- None. Work Unit 1 is intentionally not routed by the command; legacy `Planner.Build`, `ActionCatalog.ExternalActions`, `Executor.Execute`, `ui.RunWithContext`, and `printExecutionReport` remain unchanged.

## Remaining Work

- Work Unit 2 (command orchestration): repository acquirer, UI review/display, aggregate report printer, `runInstallWithDeps`, routing in `install.go`, and end-to-end flow tests.

## PR Boundary

- **PR 1** (this unit): `feat(installer): phase-scoped plans and bootstrap execution` — planner/catalog partition, `ExternalOnlyExecutor`, aggregate report types, focused tests.
- **PR 2** (next unit): `feat(cli): orchestrate missing-clone installs in two phases` — wired routing.
- Chain strategy: `stacked-to-main` (per cached delivery strategy).

## Risks

- Legacy single-plan fingerprint: kept stable by leaving `Role` empty for legacy `Build` and using `omitempty` in canonical JSON.
- Catalog phase ownership: package/configuration lists are disjoint; submodule and power-profile actions excluded from package plan.
- Executor ordering: `Executor.Execute` still runs managed first; `ExternalOnlyExecutor` runs only external actions and rejects managed targets.
