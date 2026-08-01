# Verification Report: Two-Phase Install Flow

## Result

**FAIL — not ready for archive.**

The repository-lookup remediation is present and correctly changes the source-level route boundary, but this re-run cannot establish current GREEN status: the native SDD attempt authority returned `state: complete` before test execution, so no focused or full Go test command was launched. Strict TDD remains **CRITICAL** because `apply-progress.md` no longer contains the required `TDD Cycle Evidence` table. The native OpenSpec dispatcher also still fails to recognize the readable `spec.md`; this is a dispatcher limitation, not an implementation defect, but it keeps native archive routing blocked.

CodeGraph MCP was unavailable (`MCP not initialized`), so the read-only source inspection used direct Git/file reads.

## Prior Blockers Rechecked

| Prior blocker | Status | Evidence |
|---|---|---|
| Repository lookup errors entered the missing-clone route | **Source resolved; runtime GREEN unconfirmed** | Commit `6a23b70` adds `ErrRepositoryNotFound`. `resolveRepositoryRoot` propagates non-absence errors (`cli/cmd/install.go`), `repositoryLocator.Locate` maps only that sentinel to `Found:false` (`cli/cmd/install_flow.go`), and `runInstallWithDeps` returns on lookup error before menu routing. Regression tests exist: `TestRepositoryLocator_PropagatesLookupErrors` and `TestRepositoryLocator_AbsenceIsNotAnError` (`cli/cmd/install_flow_test.go`). |
| Strict TDD evidence incomplete | **CRITICAL — unresolved** | The remediated file now narrates RED/GREEN/triangulation for both work units, but it contains no `TDD Cycle Evidence` heading or table, no per-row Safety Net field, and therefore does not meet the active strict-TDD verification contract. |
| Native status cannot recognize the spec artifact | **Unresolved dispatcher limitation** | `spec.md` is readable at the required path, but native status still returns `artifacts.specs: missing`, `dependencies.verify/archive: blocked`, and `nextRecommended: spec`. This is not counted as an implementation defect, but it blocks native archive routing. |

## Structured Status and Action Context

- Change selection: exact requested change `2026-08-01-two-phase-install-flow`; change directory is readable.
- Artifact store: `openspec` (authoritative native status).
- Read artifacts: `spec.md`, `design.md`, `tasks.md`, `apply-progress.md`, prior `verify-report.md`, root and CLI OpenSpec config, changed implementation, and changed tests.
- Native status command:
  ```bash
  gentle-ai sdd-status 2026-08-01-two-phase-install-flow --cwd /home/agustin/Dev/dotfiles --json --instructions
  ```
  It reports proposal/design/tasks/apply-progress/verify-report as done; `specs` as missing despite the readable `spec.md`; 117/117 tasks complete; and archive blocked.
- `actionContext`: `repo-local`, workspace root `/home/agustin/Dev/dotfiles`, allowed edit root `/home/agustin/Dev/dotfiles`. All inspected implementation and test files are inside that root.

## Spec Coverage

The spec contains 13 requirements and 42 scenarios. Source inspection finds the intended behavior implemented; live runtime confirmation is unavailable for every row because the native attempt guard prohibited a new test run.

| Requirement | Status | Evidence |
|---|---|---|
| Route selection by repository availability | **Source PASS / runtime unconfirmed** | Only `ErrRepositoryNotFound` maps to the missing-clone route; other lookup errors propagate before the menu. Existing clone, absence, Git-independent routing, and per-invocation lookup have focused test coverage. |
| Read-only pre-acceptance | **Source PASS / runtime unconfirmed** | Missing-clone flow obtains menu input, builds a read-only package plan, and reviews it before executor/acquirer/transaction calls. |
| Single authorization | **Source PASS / runtime unconfirmed** | `ReviewPackagePlanWithContext` has no executor; `DisplayConfigurationPlan` is output-only and no second acceptance is requested. |
| Package plan construction | **Source PASS / runtime unconfirmed** | `BuildPackage` uses `PhaseActionCatalog` only; base tools are first; submodule and power-profile actions are excluded. |
| Package execution | **Source PASS / runtime unconfirmed** | `ExternalOnlyExecutor` preserves reviewed order and stops after the first external-action failure. |
| Repository acquisition | **Source PASS / runtime unconfirmed** | Frozen request, read-only destination preflight, recursive clone/update, retained failed destination, and override handling are implemented. |
| Configuration plan construction | **Source PASS / runtime unconfirmed** | Configuration planning uses acquired root plus shared run/options and phase-specific actions. |
| Configuration plan display | **Source PASS / runtime unconfirmed** | Concrete plan display has no input or confirmation surface. |
| Configuration execution | **Source PASS / runtime unconfirmed** | Managed targets use the existing transaction; no-target configuration uses the external-only executor. |
| Failure boundaries | **Source PASS / runtime unconfirmed** | Package/acquisition/configuration cancellation and failure paths stop later phases and emit partial outcomes. |
| Aggregate reporting | **Source PASS / runtime unconfirmed** | `TwoPhaseExecutionReport` and `printTwoPhaseExecutionReport` retain phase outcomes and scope rollback to managed targets. |
| Existing-clone compatibility | **Source PASS / runtime unconfirmed** | Existing clone delegates to the legacy planner/UI/executor path, preserving managed-before-external ordering. |
| Filesystem overlap audit | **Source PASS / runtime unconfirmed** | `ConfigurationActions` omits standalone zsh-directory creation when a managed target owns the path. |

### Requirement 1 Route Selection

- **Lookup error:** `findRepositoryRoot` returns a non-sentinel error for a non-directory/unreadable lookup; `resolveRepositoryRoot` and `repositoryLocator.Locate` propagate it; `runInstallWithDeps` returns before either route/menu.
- **True absence:** missing start/canonical locations produce `ErrRepositoryNotFound`, which alone becomes `RepositoryState{Found:false}` and selects the two-phase route.
- **Existing clone:** a resolved root produces `Found:true` and selects `runExistingCloneInstall`.
- The requested focused tests and `cd cli && go test ./...` were **not executed** because of the native attempt result below. Therefore current GREEN is not independently confirmed.

## Task Completion

- Implementation task markers: **117/117 checked**.
- Unchecked implementation lines matching `^\s*- \[ \]`: **none**.
- No unchecked task can be used to block archive; the blockers below are independent of checkbox completion.

## Validation Commands

| Command | Result |
|---|---|
| `gentle-ai sdd-attempt acquire --cwd /home/agustin/Dev/dotfiles --change 2026-08-01-two-phase-install-flow --request-id verify-post-remediation-20260801-01 --work-unit post-remediation-verification --evidence-goal verify-route-selection-and-full-go-suite-6a23b70 --max-attempts 1 --max-changed-lines 0` | **`state: complete`** — no opaque token was issued; the runtime-bearing verification launch must stop. |
| `cd cli && go test ./cmd -run 'TestRepositoryLocator_(PropagatesLookupErrors|AbsenceIsNotAnError)$' -v` | **NOT RUN** — prohibited after native attempt state `complete`. |
| `cd cli && go test ./cmd -run 'TestRunInstallWithDeps_Routes|TestRunInstallWithDeps_RouteIsReevaluatedPerInvocation' -v` | **NOT RUN** — prohibited after native attempt state `complete`. |
| `cd cli && go test ./...` | **NOT RUN** — prohibited after native attempt state `complete`. |
| `git diff --check c851a48..HEAD` | **WARNING** — trailing whitespace remains in `design.md:307` and `tasks.md:14-17,482`. |

`apply-progress.md` records a post-remediation full-suite PASS, but that historical assertion is not a substitute for an independently executed current GREEN check.

## Strict TDD Compliance

Strict TDD is active in `cli/openspec/config.yaml`; the global strict-TDD verify guidance was applied (no project-local override exists).

| Check | Result | Details |
|---|---|---|
| TDD Cycle Evidence table present | ❌ | No `TDD Cycle Evidence` table exists in `apply-progress.md`. |
| All task rows have formal evidence | ❌ | The file has narrative sections for the 14 task areas, not the required table rows/columns. |
| RED evidence cross-references real tests | ⚠️ | The eight changed test files exist (113 `Test*` functions), but the formal row mapping is absent. The claimed `TestRunInstallWithDeps_ConfigPhase*` pattern has no matching test function. |
| GREEN still true | ❌ | Current focused/full tests could not be run after native attempt authority returned `complete`. |
| Triangulation adequate | ⚠️ | Narrative triangulation exists, but it is not row-level evidence with verifiable counts. |
| Safety Net for modified files | ❌ | Work-unit safety-net prose exists, but there is no required per-row Safety Net evidence. |

**TDD compliance: 0/6 formal checks verified. This is a CRITICAL archive blocker.**

### Test Layer Distribution (static census)

| Layer | Tests | Files | Tools |
|---|---:|---:|---|
| Unit | 111 | 8 | `go test` |
| Integration | 2 | 1 | local Git, skippable under `testing.Short()` |
| E2E | 0 | 0 | — |
| **Total** | **113** | **8** | |

The CLI capability config says integration tests are unavailable, but `repository_acquirer_test.go` includes two local-Git integration tests. This is a non-blocking metadata warning.

### Assertion Quality

No tautologies or ghost loops were found in the eight changed test files. The following tests are weak behavioral evidence:

| File | Line | Assertion/test | Issue | Severity |
|---|---:|---|---|---|
| `cli/pkg/installer/executor_test.go` | 178 | `TestExternalOnlyExecutor_NeverCallsManagedExecutor` | The `fakeManaged` instance is never injected into `ExternalOnlyExecutor`; its zero call count is disconnected from production behavior. | WARNING |
| `cli/pkg/installer/ui/two_phase_test.go` | 144 | `TestReviewPackagePlanWithContext_DoesNotInvokeExecutor` | The fake executor is never supplied to the function under test, so its zero call count proves nothing about production behavior. | WARNING |
| `cli/pkg/installer/ui/two_phase_test.go` | 184 | `TestDisplayConfigurationPlan_CancellationBeforeExecutionProducesNoTransaction` | It exercises only output generation; no cancellation signal, transaction, executor, or coordinator is involved. | WARNING |
| `cli/cmd/repository_acquirer_test.go` | 49 | `TestRepositoryAcquirer_Interface` | Compile-time interface assertion only; no behavioral assertion. | WARNING |

**Assertion quality: 0 CRITICAL, 4 WARNING.**

## Review Workload and PR Boundary

- The tasks forecast requires chained PRs with `auto-chain` and `stacked-to-main`.
- The current branch `feat/installer-two-phase-2` is based on `feat/installer-two-phase-1` at `17a6557`; the PR-2 delta contains the command/UI slice plus task/progress artifacts and the focused remediation. This respects the assigned work-unit boundary.
- The PR-2 delta is still **2,724 additions / 278 deletions across 11 files**, far above the 400-line forecast budget. No `size:exception` is recorded. **WARNING.**
- The ancestor PR-1 contains `.github/workflows/ci.yml` Markdown-lint exclusions for active OpenSpec artifacts. That change is outside the spec's CI non-goal and remains **scope-creep WARNING**.

## Exact Blockers

1. **CRITICAL:** Restore a compliant `TDD Cycle Evidence` table in `apply-progress.md`, with one verifiable row per task area and RED, GREEN, Triangulate, Safety Net, and test-file evidence.
2. **BLOCKED:** Obtain a native-authorized verification attempt. The current `sdd-attempt acquire` result is `state: complete`, so this executor could not run the requested focused/full Go tests or confirm current GREEN.
3. **LIFECYCLE BLOCKED (not an implementation defect):** Reconcile the native dispatcher/spec artifact mismatch. Native status still reports `specs: missing` and archive blocked despite the readable required `spec.md`.

## Non-Blocking Follow-Ups

- Replace the disconnected/mock-only assertions noted above with coordinator or observable behavior tests.
- Reconcile the PR-size exception or split/review boundary and remove or justify the CI scope creep.
- Resolve the Markdown trailing whitespace reported by `git diff --check`.
