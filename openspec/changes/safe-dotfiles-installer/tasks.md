# Implementation Tasks: Safe Dotfiles Installer

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 1,400–2,100 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 plan/report contracts → PR 2 filesystem transaction → PR 3 external action catalog/executor → PR 4 TUI and Cobra cutover |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

**Decision required:** approve a `size:exception` before applying this as the requested single PR, or change delivery strategy to chained PRs and choose `stacked-to-main` or `feature-branch-chain`. Do not begin implementation until this decision is recorded.

## Validation Baseline

Run all commands from `cli/`:

```bash
go test ./...
go test -cover ./...
go vet ./...
go build ./...
go fmt ./...
git diff --check
```

Strict TDD is active: each work unit follows **RED → GREEN → TRIANGULATE → REFACTOR**. Use `t.TempDir()` for filesystem cases, table-driven tests for variants, direct `ReviewModel.Update()` tests for UI transitions, and fakes for command/process boundaries. Do not invoke package managers, use a real home directory, or require a terminal in tests.

## Work Unit 1 — Immutable Plan and Typed Reporting (PR 1)

**Depends on:** approval of the delivery decision.
**Start boundary:** current `cli/cmd/install.go` mutation-first flow and existing `cli/pkg/installer/` helpers remain unchanged.
**Finish boundary:** a read-only, immutable plan can describe the complete closed managed-target and external-action set with a stable fingerprint; no production command or filesystem mutation is wired to it yet.
**Rollback:** revert only the new `cli/pkg/installer/plan/` and `cli/pkg/installer/report/` files and their tests.

- [x] 1.1 **RED — plan contracts and canonical fingerprint tests.** Add table-driven failing tests in `cli/pkg/installer/plan/plan_test.go` for cleaned absolute target identity, source-within-repository validation, duplicate and ancestor/descendant target rejection, root-level target inclusion (`.zshrc`, `.gtkrc-2.0`), ordered external actions, and equal fingerprints for equivalent canonical input. Run `go test ./pkg/installer/plan -run 'Test(Build|Fingerprint)'` and confirm failure because the contracts do not exist.
- [x] 1.2 **GREEN — immutable plan model and read-only planner seams.** Add `cli/pkg/installer/plan/plan.go` and `cli/pkg/installer/plan/planner.go` with `InstallationPlan`, `Target`, `PreState`, `CommandSpec`, action classification, `Planner`, `TargetDiscoverer`, `StateReader`, `ActionCatalog`, `Clock`, and `RunIDSource`. Ensure planning only reads state/prerequisites, returns typed planning errors, has no `exec` or mutation call, allocates `RunID`, validates closed target topology/source containment, and produces SHA-256 from canonical ordered data. Run `go test ./pkg/installer/plan`.
- [x] 1.3 **TRIANGULATE — pre-state discovery boundaries.** Extend `cli/pkg/installer/plan/plan_test.go` and add `cli/pkg/installer/plan/state_test.go` to cover absent, file, directory, and symlink pre-states; `Lstat` semantics; deterministic directory digest across traversal order; unsupported special files; missing/unreadable sources; and prerequisite failure without mutation. Implement the smallest `cli/pkg/installer/plan/state.go` behavior to pass. Run `go test ./pkg/installer/plan`.
- [x] 1.4 **RED/GREEN — execution report contracts.** Add failing tests in `cli/pkg/installer/report/report_test.go` for typed phase/target outcomes, retained backup paths, primary cause plus all rollback failures, and external failure reporting. Add `cli/pkg/installer/report/report.go` with pure `ExecutionReport`, target/action status, and typed `PlanError`, `PlanDriftError`, `BackupError`, `MutationError`, `RollbackError`, and `ExternalActionError`. Run `go test ./pkg/installer/report`.
- [x] 1.5 **REFACTOR/verify work unit.** Remove test duplication without widening APIs; format and run `go test ./pkg/installer/plan ./pkg/installer/report`, `go vet ./pkg/installer/plan ./pkg/installer/report`, and `git diff --check`. Commit as one behavior unit, e.g. `feat(installer): model immutable installation plans`.

## Work Unit 2 — Recoverable Filesystem Transaction (PR 2)

**Depends on:** Work Unit 1.
**Start boundary:** planner supplies immutable targets/pre-states; no Cobra or external-action cutover.
**Finish boundary:** managed targets are safely backed up, staged, mutated, drift-checked, reported, and rolled back with retained inventories; no external commands run here.
**Rollback:** revert `cli/pkg/installer/transaction/` and its tests; it is isolated behind its transaction interface.

- [ ] 2.1 **RED — inventory and backup-before-write tests.** Add failing `t.TempDir()` tests in `cli/pkg/installer/transaction/transaction_test.go` for file, directory, symlink, absent, and root-level-file targets; deterministic `<parent>/.dots-backups/<RunID>/<escaped-target>` locations; collision rejection; backup readability/digest validation; and no destination mutation when backup preparation fails. Run `go test ./pkg/installer/transaction -run 'Test(Prepare|Backup)'` and confirm RED.
- [ ] 2.2 **GREEN — safe inventory, backup, and staging implementation.** Add `cli/pkg/installer/transaction/transaction.go`, `filesystem.go`, and `inventory.go`; define injectable `Filesystem` and implement direct `os`, `io`, `filepath`, and `os.Symlink` operations only. Create mode-appropriate retained inventory entries, safely copy content/modes/symlink values, reject impossible same-filesystem atomic layouts, and never overwrite a collision. Run `go test ./pkg/installer/transaction`.
- [ ] 2.3 **TRIANGULATE — atomic mutation and drift tests.** Extend `transaction_test.go` for special-character paths, file staging/rename, directory staging/swap, absent target creation, no delete-then-copy fallback, current-state mismatch before mutation, and absence changing to present. Implement bound pre-state checks immediately before each mutation and append a target to `mutated` as soon as its original is moved or destination changes. Run `go test ./pkg/installer/transaction -run 'Test(Mutate|Drift)'`.
- [ ] 2.4 **RED/GREEN — rollback completeness tests.** Add failing tests in `cli/pkg/installer/transaction/rollback_test.go` for failure at the first target, failure after multiple mutations, reverse restoration order, partially changed current target, continued restoration after one restore failure, backup retention after success/rollback, and aggregate `RollbackError` with manual recovery paths. Implement rollback in reverse order, restoring from retained backups (or removing originally absent targets), continuing after errors, and reporting all outcomes. Run `go test ./pkg/installer/transaction`.
- [ ] 2.5 **REFACTOR/verify work unit.** Refactor only after all transaction scenarios pass; run `go test ./pkg/installer/transaction`, `go vet ./pkg/installer/transaction`, and `git diff --check`. Commit tests with behavior, e.g. `feat(installer): transact managed file mutations safely`.

## Work Unit 3 — Structured External Actions and Executor Ordering (PR 3)

**Depends on:** Work Units 1–2.
**Start boundary:** transaction is independently testable; existing `cli/pkg/installer/packages.go`, `system.go`, `gsettings.go`, `hyprland.go`, and `utils.go` still provide the legacy behavior.
**Finish boundary:** the complete current action catalog yields reviewed structured external actions; managed shell-copy paths are represented as transaction targets; execution never uses `sh -c` for managed copies.
**Rollback:** revert catalog/adapter/executor changes together, leaving the transaction package intact.

- [ ] 3.1 **RED — external command boundary tests.** Add failing tests in `cli/pkg/installer/external/runner_test.go` for argument-preserving `CommandSpec` execution, working directory/environment handling, non-zero exits, missing binary errors, and no shell command construction. Helper-process tests, if needed, MUST skip under `testing.Short()`. Run `go test ./pkg/installer/external -run TestRunner` and confirm RED.
- [ ] 3.2 **GREEN — external runner and classification model.** Add `cli/pkg/installer/external/runner.go` with an injectable `CommandRunner` that uses `exec.CommandContext(spec.Name, spec.Args...)`, streams configured terminal I/O, and returns identity-rich errors. Use only `CommandSpec` from the plan package; do not accept interpolated shell strings. Run `go test ./pkg/installer/external`.
- [ ] 3.3 **RED — catalog adaptation tests.** Add failing table-driven tests at concrete discovery targets `cli/pkg/installer/packages_test.go`, `system_test.go`, `gsettings_test.go`, and `hyprland_test.go` proving current package/system/service/network/plugin/cache operations become ordered classified `CommandSpec`s; font/cursor copies become managed targets; and no catalog action executes while planning. Run `go test ./pkg/installer -run 'Test(ActionCatalog|ManagedTargets)'`.
- [ ] 3.4 **GREEN/TRIANGULATE — adapt current installer helpers behind the catalog.** Modify `cli/pkg/installer/packages.go`, `system.go`, `gsettings.go`, `hyprland.go`, and `utils.go` only as needed to build structured actions/targets. Explicitly classify `pacman`, `sudo`, `systemctl`, `chsh`, profile writes as privileged; Git/AUR/build/hyprpm operations as supply-chain; cache/gsettings as external. Replace any managed font/cursor shell-copy path with transaction targets and direct filesystem behavior. Add tests for every discovered current action and path with spaces/quotes. Run `go test ./pkg/installer ./pkg/installer/external`.
- [ ] 3.5 **RED/GREEN — executor sequencing tests and implementation.** Add failing tests in `cli/pkg/installer/executor_test.go` with fake transaction/runner for managed-before-external order, zero external runs on managed failure, no managed rollback after external failure, stop-after-first-external failure, progress/report events, and retained managed inventory. Add `cli/pkg/installer/executor.go` to execute the reviewed plan exactly once: transaction first, mark it committed, then external actions in reviewed order. Run `go test ./pkg/installer -run TestExecutor`.
- [ ] 3.6 **REFACTOR/verify work unit.** Confirm production managed copy/move/link paths have no `sh -c` by reviewing `cli/pkg/installer/` as a concrete discovery target; run `go test ./pkg/installer/...`, `go vet ./pkg/installer/...`, and `git diff --check`. Commit as `feat(installer): execute reviewed external actions safely`.

## Work Unit 4 — Review UI and Cobra Cutover (PR 4)

**Depends on:** Work Units 1–3.
**Start boundary:** planner, transaction, catalog, executor, and report are independently tested; legacy Cobra orchestration remains active.
**Finish boundary:** `dots install` uses a read-only plan/review/one-confirmation flow with typed reporting and no alternate mutation-first path.
**Rollback:** revert `cli/pkg/installer/ui/` and the `cli/cmd/install.go` composition change as one unit to restore the previous command flow.

- [ ] 4.1 **RED — review model state tests.** Add direct `Model.Update()` tests in `cli/pkg/installer/ui/review_test.go` for plan-ready/plan-failed messages, complete managed and external rendering before controls enable, visible classifications/irreversible warning, confirm-all only, abort via decline/escape/Ctrl-C, and no executor call before confirmation. Assert the same plan fingerprint/value reaches the executor; add no per-item toggle message/control. Run `go test ./pkg/installer/ui -run TestReviewModel` and confirm RED.
- [ ] 4.2 **GREEN — Bubble Tea review/progress/result model.** Add `cli/pkg/installer/ui/review.go` and focused `ui/run.go` seams. Implement `initializing`, `review`, `executing`, `done`, `aborted`, and `error` states; require `reviewRendered` before confirm/abort controls; emit executor work only for explicit confirmation; surface retained backups, typed failure details, rollback completeness, and non-reversible external effects. Run `go test ./pkg/installer/ui`.
- [ ] 4.3 **TRIANGULATE — cancellation and execution-result tests.** Extend `review_test.go` for terminal-loss/interrupt abandonment, managed failure with complete/incomplete rollback reports, external-action failure that retains managed files, and progress events. Ensure all pre-confirmation exits return `Aborted` without mutation and all execution errors remain non-zero at the command boundary. Run `go test ./pkg/installer/ui`.
- [ ] 4.4 **RED — command composition tests.** Add failing tests at `cli/cmd/install_test.go` using injected planner/UI/executor seams for: no planning mutation, decline exit zero, plan failure non-zero with no review, confirmed plan passed unchanged, and placeholder dev mode returning explicit unsupported planning result without hidden work. Run `go test ./cmd -run TestInstall` and confirm RED.
- [ ] 4.5 **GREEN — replace mutation-first Cobra orchestration.** Modify only `cli/cmd/install.go` to collect existing Huh choices, compose dependencies, call the UI flow, print `ExecutionReport`, map `Aborted` to zero, and return every planning/managed/rollback/external failure as a Cobra error. Remove direct calls to `backupConflicts` and direct install/mutation work from choice collection. Keep command name and existing user-mode intent; do not add flags, action toggles, docs, CI, or root-repo changes. Run `go test ./cmd`.
- [ ] 4.6 **REFACTOR/final verification.** Keep tests behavior-focused and remove only duplicate scaffolding. Run the exact full validation sequence: `go fmt ./...`, `go test ./...`, `go test -cover ./...`, `go vet ./...`, `go build ./...`, and `git diff --check`. Manually inspect the `cli/` diff to confirm only `cli/` changed and no managed copy/move/link uses shell interpolation. Commit as `feat(installer): require plan review before installation`.

## Apply Gate

- [ ] Record the delivery decision named above before starting Work Unit 1.
- [ ] If a single-PR `size:exception` is approved, preserve the four work units as separate conventional commits and stop for review if actual changed lines materially exceed the 2,100-line estimate.
- [ ] If chaining is selected, apply one PR work unit at a time in the stated dependency order and use the selected chain strategy.
