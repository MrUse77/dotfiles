# Apply Progress — 2026-08-01-two-phase-install-flow

Strict TDD evidence. Test runner: `cd cli && go test ./...`.

## Work Unit 1 — feat(installer): phase-scoped plans and bootstrap execution

### 1.1 PlanRole + InstallationRun (plan package)

- ✅ Written (RED): `TestPlanRoleConstants`, `TestInstallationRun_HoldsRunIDAndOptionsSnapshot`, `TestInstallationPlan_RoleIncorporatedIntoFingerprint` (`cli/pkg/installer/plan/plan_test.go`)
- ✅ Passed (GREEN): `go test ./pkg/installer/plan/...`
- ✅ Triangulation: role-empty legacy fingerprint stability + role-incorporated fingerprint; run snapshot immutability against options mutation.

### 1.2 PhaseActionCatalog contract

- ✅ Written (RED): `TestPhaseActionCatalogContract`, `TestPlanner_PhaseBuildsRequirePhaseCatalog` (`cli/pkg/installer/plan/plan_test.go`)
- ✅ Passed (GREEN): `go test ./pkg/installer/plan/...`
- ✅ Triangulation: missing phase catalog rejected; provided catalog accepted.

### 1.3 Planner StartRun / BuildPackage / BuildConfiguration

- ✅ Written (RED): `TestPlanner_StartRun_*`, `TestPlanner_BuildPackage_NoDiscoveryOrStateRead`, `TestPlanner_BuildConfiguration_FromDiscoveredSource`, `TestPlanner_PhasePlansShareRunIDAndOptionsButHaveDistinctFingerprints`, `TestPlanner_PhasePlanImmutableAgainstInputMutation` (`cli/pkg/installer/plan/plan_test.go`)
- ✅ Passed (GREEN): `go test ./pkg/installer/plan/...`
- ✅ Triangulation: package plan never touches discoverer/state reader; two phase plans share run/options but differ in fingerprint; input mutation cannot alter a reviewed plan.

### 1.4 Catalog PackageActions / ConfigurationActions

- ✅ Written (RED): `TestActionCatalog_PackageActions_*`, `TestActionCatalog_ConfigurationActions_*`, `TestActionCatalog_PhaseListsAreDisjoint`, `TestActionCatalog_ExternalActionsUnchanged` (`cli/pkg/installer/catalog_phase_test.go`)
- ✅ Passed (GREEN): `go test ./pkg/installer/...`
- ✅ Triangulation: disjoint ownership (submodules → acquisition; zsh mkdir → configuration when a managed target owns `~/.config/zsh`); legacy `ExternalActions` unchanged.

### 1.5 ExternalOnlyExecutor

- ✅ Written (RED): `TestExternalOnlyExecutor_*` (`cli/pkg/installer/executor_test.go`)
- ✅ Passed (GREEN): `go test ./pkg/installer/...`
- ✅ Triangulation: reports completed/failed/skipped in reviewed order; rejects managed targets; never creates a managed executor; legacy `Executor.Execute` ordering tests still pass.

### 1.6 Two-phase report types + ExecutionReport.InventoryPath

- ✅ Written (RED): `TestTwoPhaseOutcomeConstants`, `TestInstallPhaseConstants`, `TestPhaseStateConstants`, `TestTransactionStateConstants`, `TestTwoPhaseExecutionReport_Struct`, `TestExecutionReport_InventoryPathField` (`cli/pkg/installer/report/two_phase_test.go`, `cli/pkg/installer/report/report_test.go`)
- ✅ Passed (GREEN): `go test ./pkg/installer/report/... ./pkg/installer/transaction/...`
- ✅ Triangulation: two-phase states distinct; inventory path populated by the transaction report builder.

### 1.7 Safety Net (Work Unit 1)

- ✅ Passed: full regression `cd cli && go test ./...` — all packages green.

## Work Unit 2 — feat(cli): orchestrate missing-clone installs in two phases

### 2.1 Repository acquisition (repository_acquirer.go)

- ✅ Written (RED): `TestRepositoryAcquirer_*` (`cli/cmd/repository_acquirer_test.go`): frozen version/dev/override ref reaches the Git seam; recursive submodules; conflict directory retained; acquisition failure performs no cleanup; no Git lookup controls route choice.
- ✅ Passed (GREEN): `go test ./cmd -run TestRepositoryAcquirer`
- ✅ Triangulation: version/dev/override refs; clone vs update of existing clone; local-Git integration under `testing.Short()`.

### 2.2 Route selection + error boundary (repositoryLocator)

- ✅ Written (RED): `TestRepositoryLocator_PropagatesLookupErrors`, `TestRepositoryLocator_AbsenceIsNotAnError` (`cli/cmd/install_flow_test.go`) — a lookup error must propagate, never convert into the mutating missing-clone route.
- ✅ Passed (GREEN): `go test ./cmd -run TestRepositoryLocator` + `TestResolveRepositoryRoot` (sentinel `ErrRepositoryNotFound`; missing candidates are absence, not errors).
- ✅ Triangulation: absent repo → `Found:false, nil`; unreadable/non-directory start → error; existing clone → `Found:true`.

### 2.3 runInstallWithDeps routing + single consent

- ✅ Written (RED): `TestRunInstallWithDeps_Routes*` (`cli/cmd/install_flow_test.go`): existing clone uses only the legacy helper; no clone selects the two-phase helper regardless of Git availability; route re-evaluated per invocation; event log proves menu/review occur before runner/acquirer/transaction calls; decline invokes no phase.
- ✅ Passed (GREEN): `go test ./cmd -run TestRunInstallWithDeps`
- ✅ Triangulation: consent decline vs accept; existing vs missing routes; configuration display has no second confirmation.

### 2.4 Two-phase UI display (ui/two_phase.go)

- ✅ Written (RED): `TestTwoPhase*` (`cli/pkg/installer/ui/two_phase_test.go`): initial screen includes irreversible Phase-A actions, destination/ref, deferred targets/actions, rollback disclosure; configuration display is output-only (no confirmation transition/input).
- ✅ Passed (GREEN): `go test ./pkg/installer/ui/...`
- ✅ Triangulation: phase-status formatting; direct `Model.Update()` state transitions.

### 2.5 Aggregate reporting (install_report.go)

- ✅ Written (RED): `TestPrintTwoPhaseExecutionReport*` (`cli/cmd/install_report_test.go`): full success contains two fingerprints and inventory; package/acquisition failures are incomplete rather than successful; no transaction means no inventory; configuration rollback names managed-only recovery.
- ✅ Passed (GREEN): `go test ./cmd -run TestPrintTwoPhaseExecutionReport`
- ✅ Triangulation: success, package failure, acquisition failure, config rollback.

### 2.6 Configuration phase reuses the existing transaction

- ✅ Written (RED): `TestRunInstallWithDeps_ConfigPhase*` (`cli/cmd/install_flow_test.go`): config planner receives the acquired root and accepted snapshot; Phase-A actions never appear in Phase B; managed failures retain existing rollback/inventory; Phase-B external failure reported without claiming package/clone rollback.
- ✅ Passed (GREEN): `go test ./cmd -run TestRunInstallWithDeps` + existing `./pkg/installer/transaction/...` regression suite.
- ✅ Triangulation: rollback and restore tests unchanged and passing.

### 2.7 Safety Net (Work Unit 2)

- ✅ Passed: full regression `cd cli && go test ./...` — all packages green; `go vet ./...` clean; `gofmt -l` empty.

## Verify remediation (post-verify blockers)

- CRITICAL route-selection defect fixed with RED tests first: `TestRepositoryLocator_PropagatesLookupErrors` failed before the `ErrRepositoryNotFound` sentinel and error propagation in `resolveRepositoryRoot`/`Locate`; all green after.
- Full suite re-run after remediation: `cd cli && go test ./...` PASS.
