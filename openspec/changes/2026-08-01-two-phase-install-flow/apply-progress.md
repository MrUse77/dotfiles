# Apply Progress: Two-Phase Install Flow — Work Unit 2

## Status

- Work Unit: 2 of 2
- Scope: `feat(cli): orchestrate missing-clone installs in two phases` — command routing, repository acquisition, single-consent UI, phase-aware reporting, and end-to-end flow tests
- All Work Unit 2 tasks implemented and verified
- Work Unit 1 remains intact and passing
- Last test run: `cd cli && go test ./...` — PASS

## TDD Cycle Evidence

| Task | RED test | GREEN implementation | Test command / result |
|------|----------|---------------------|----------------------|
| 2.1 Repository Acquirer | `TestRepositoryRequest_Fields`, `TestRepositoryAcquisition_Fields`, `TestRepositoryAcquirer_Interface`, `TestPreflightRepositoryDestination_*`, `TestAcquire_*`, `TestBuildRepositoryRequest_*`, `TestEnsureRepositoryClone_WrapperStillWorks` | Created `cli/cmd/repository_acquirer.go` with `RepositoryRequest`, `RepositoryAcquisition`, `RepositoryAcquirer`, read-only `PreflightRepositoryDestination`, context-aware Git runner, and `BuildRepositoryRequest` | `cd cli && go test ./cmd -run 'TestRepository|TestPreflight|TestAcquire|TestBuildRepository|TestEnsureRepository' -v` PASS |
| 2.2 Two-Phase UI | `TestTwoPhaseReviewDetails_Fields`, `TestPackageReviewModel_*`, `TestReviewPackagePlanWithContext_*`, `TestDisplayConfigurationPlan_*` | Created `cli/pkg/installer/ui/two_phase.go` with `TwoPhaseReviewDetails`, `PackageReviewModel`, `ReviewPackagePlanWithContext`, and `DisplayConfigurationPlan` | `cd cli && go test ./pkg/installer/ui -v` PASS |
| 2.3 Aggregate Report Printer | `TestPrintTwoPhaseExecutionReport_*`, `TestExistingPrintExecutionReport_Unchanged` | Created `cli/cmd/install_report.go` with `printTwoPhaseExecutionReport` | `cd cli && go test ./cmd -run 'TestPrint|TestExistingPrint' -v` PASS |
| 2.4 Phase Routing | `TestRunInstallWithDeps_RoutesExistingCloneToLegacyFlow`, `TestRunInstallWithDeps_RoutesMissingCloneToTwoPhaseFlow`, `TestRunInstallWithDeps_RouteIsReevaluatedPerInvocation`, `TestRunInstallWithDeps_PackageFailureStopsBeforeAcquisition`, `TestRunInstallWithDeps_AcquisitionFailureStopsBeforeConfiguration`, `TestRunInstallWithDeps_MenuAndReviewBeforeAnyMutation`, `TestRunInstallWithDeps_ConfigurationWithManagedTargetsRunsTransaction`, `TestRunInstallWithDeps_ConfigurationWithNoManagedTargetsUsesExternalOnlyExecutor`, `TestRunInstallWithDeps_ContextCancelledBeforeConfigurationProducesNoTransaction` | Created `cli/cmd/install_flow.go` with `runInstallWithDeps`, `runExistingCloneInstall`, `runMissingCloneInstall`, injectable dependencies, and phase boundaries | `cd cli && go test ./cmd -v` PASS |
| 2.5 Install.go Routing | `TestRunInstallWithDeps_*` (route through production deps), `TestExistingPrintExecutionReport_Unchanged`, `TestResolveRepositoryRoot_*` | Modified `cli/cmd/install.go` to route through `runInstallWithDeps`; removed `confirmAndInstallGit` and direct pre-menu clone; kept `installCmd`, `installDiscoverer`, repository helpers | `cd cli && go test ./cmd -v` PASS |
| 2.6 End-to-End Fakes | `TestRunInstallWithDeps_*` (event-log, ordering, pre-acceptance read-only, rollback, cancellation, retry) | Implemented `eventLog`, fake locator/menu/review/acquirer/executors/runners/planners in `cli/cmd/install_flow_test.go` | `cd cli && go test ./cmd -v` PASS |
| 2.7 Work Unit 2 Verification | All existing + new tests pass | — | `cd cli && go test ./...` PASS |

## Files Changed

- `cli/cmd/install.go` — route through `runInstallWithDeps`; removed `confirmAndInstallGit` and direct pre-menu clone
- `cli/cmd/install_flow.go` — new coordinator, dependency injection, legacy and two-phase helpers
- `cli/cmd/install_flow_test.go` — end-to-end flow tests with fakes and event log
- `cli/cmd/repository_acquirer.go` — new repository acquisition types, preflight, and production acquirer
- `cli/cmd/repository_acquirer_test.go` — acquisition tests and local-Git integration
- `cli/cmd/install_report.go` — new aggregate `printTwoPhaseExecutionReport`
- `cli/cmd/install_report_test.go` — aggregate report tests
- `cli/pkg/installer/ui/two_phase.go` — new `TwoPhaseReviewDetails`, `PackageReviewModel`, `ReviewPackagePlanWithContext`, `DisplayConfigurationPlan`
- `cli/pkg/installer/ui/two_phase_test.go` — two-phase UI tests
- `openspec/changes/2026-08-01-two-phase-install-flow/tasks.md` — Work Unit 2 checkboxes marked complete
- `openspec/changes/2026-08-01-two-phase-install-flow/apply-progress.md` — this merged progress

## Deviations from Design

- `TwoPhaseReviewDetails` was placed in `ui/two_phase.go` instead of `ui/review.go`; the struct is exported and reachable by the same package.
- The production package planner uses `installer.NewActionCatalogWithParu(installer.DetectParu())` (no power-profile probe) so no `systemctl` command runs before acceptance.
- Configuration planning with no managed targets uses the existing `ExternalOnlyExecutor` and reports `TransactionNotRequired`.
- `installDependencies` includes `legacyExecutor`, `packageExecutor`, `configExecutor`, `runner`, and `programRunner` for deterministic tests.

## Remaining Work

- None. Work Unit 2 is complete. Verify is next; archive follows after verification.

## PR Boundary

- **PR 1** (previous unit): `feat(installer): phase-scoped plans and bootstrap execution` — planner/catalog partition, `ExternalOnlyExecutor`, aggregate report types.
- **PR 2** (this unit): `feat(cli): orchestrate missing-clone installs in two phases` — wired routing, repository acquisition, single-consent UI, aggregate report printer, end-to-end flow tests.
- Chain strategy: `stacked-to-main` (per cached delivery strategy).

## Risks

- Legacy route regression: covered by existing `cli/cmd/install_test.go` tests and new `TestRunInstallWithDeps_RoutesExistingCloneToLegacyFlow`.
- Pre-acceptance mutation: covered by event-log tests and `TestNewInstallPackagePlanner_DoesNotCallDetectPowerProfiles`.
- Phase boundary leakage: covered by failure-injection tests (package failure, acquisition failure, cancellation).
- Aggregate report truthfulness: covered by `TestPrintTwoPhaseExecutionReport_*` and failure-injection tests.

## Previous Work Unit 1 Progress

The Work Unit 1 progress is merged into this file. Work Unit 1 remains intact; all its tests continue to pass.
