# Apply Progress: MoonArch Versioned Configuration Releases

## Status

- Artifact store: OpenSpec
- Mode: Strict TDD (executable shell/workflow behavior)
- Completed: 39/39 tasks
- Current work unit: PR6 / Phase 7, tasks 7.1-7.5
- Delivery strategy: stacked-to-main
- Workload decision: strict maximum 400 authored additions + deletions
- Authored Phase 7 lines: 350

## Prior Progress Merged

- Phases 1-5 (tasks 1.1-5.5) remain complete as recorded in `tasks.md` and prior Engram summaries.
- PRs 1-4 delivered CLI-only update, exact resolution/admission/cache, state/lock/journal/evidence, and planner/transaction removal with inventory provenance.
- Engram observation #1207 recorded an unavailable Phase 6 executor with no candidate changes. This direct maintainer-approved implementation supersedes that interrupted attempt without losing its history.

## Completed This Batch

- [x] 6.1 Added the `config` parent with exact `apply`, offline `rollback`, and `status` commands and flags.
- [x] 6.2 Enforced managed-only config plans; config operations have no dependency or call path to `external.Runner`.
- [x] 6.3 Added the separate mutable-theme phase with relative-link validation, explicit replacement, atomic rename, and rollback.
- [x] 6.4 Implemented serialized apply, exact acquisition/admission/compatibility/dependency checks, conservative baseline planning, evidence-bound drift preflight, journaled transaction/state commit, and crash recovery.
- [x] 6.5 Implemented locked status in human/JSON formats with `legacy/unknown`, current/previous identities, retention/orphans, and unresolved journal disclosure without mutation.
- [x] 6.6 Implemented cache-only offline rollback as a new transaction, including retained-artifact revalidation and identity swap.
- [x] 6.7 Added end-to-end apply/apply/rollback coverage over temporary XDG roots, including tampered-cache rejection and zero rollback network calls.

## Completed Phase 7

- [x] 7.1 Added bridge-gated, immutable config publication with materialized submodules, normalized `tar.zst`, manifest, and digest sidecar.
- [x] 7.2 Added recorded, offline fixtures for newer config releases, bridge assets, and same-tag rejection.
- [x] 7.3 Added the defensive bootstrap guard before any binary download.
- [x] 7.4 Wired the bridge harness and exact uncached Go race command into CI change scopes.
- [x] 7.5 Documented the bridge, sidecar, immutable identity, and publication gate.

## TDD Cycle Evidence

| Task | Test file | Layer | Safety net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 6.1 | `cli/cmd/config_test.go` | Unit/command | `go test -count=1 ./cmd/` -> PASS | `newConfigCommand`/`configOperations` undefined -> build failed | command tree/flags focused test -> PASS | exact `config-v1.2.3` dispatch plus `latest` rejection -> PASS | gofmt and focused command tests -> PASS |
| 6.2 | `cli/cmd/config_apply_test.go` | Architecture/unit | cmd safety net PASS | `validateConfigPlan` undefined -> build failed | forbidden external action rejected -> PASS | managed-only plan accepted -> PASS | structural CodeGraph trace found no config-to-Runner call path; focused tests PASS |
| 6.3 | `cli/cmd/theme_phase_test.go` | Filesystem unit | N/A (new production file) | `prepareThemePhase` undefined -> build failed | preserve/missing/replace/escape cases -> PASS | invalid regular-file selection repaired and restored after initial RED (`invalid argument`) -> PASS | full `TestThemePhase_` suite -> PASS |
| 6.4 | `cli/cmd/config_apply_test.go`, `cli/pkg/installer/transaction/recovery_test.go`, `cli/cmd/config_test.go` | Unit/integration | cmd baseline PASS; recovery files new | missing runtime/acquisition/planner/preflight/mutation/recovery/exit-code symbols each failed before implementation | focused apply/recovery tests -> PASS | lock contention/release, compatibility no-promote, legacy baseline, exact drift token, transaction rollback, committed/uncommitted recovery, mismatched-run blocking, exit 2 -> PASS | final focused and full suites -> PASS |
| 6.5 | `cli/cmd/config_status_test.go` | Unit/command | cmd package PASS | status dependency/implementation absent -> build/behavior failed | status suite -> PASS | legacy, verified JSON retention/orphans, unresolved journal with zero mutation -> PASS | no further refactor needed; final focused suite PASS |
| 6.6 | `cli/cmd/config_rollback_test.go` | Unit/integration | cmd package PASS | rollback returned `config rollback is unavailable` | rollback suite -> PASS | absent previous fails pre-mutation; retained artifact swaps identities with zero resolver calls -> PASS | retained cache verification shared with end-to-end test; focused suite PASS |
| 6.7 | `cli/cmd/config_apply_test.go` | Integration | relevant packages PASS | tampered retained A rollback returned nil instead of `ErrArtifactRejected` | `TestConfigApply_EndToEnd` -> PASS | two online exact applies, tamper rejection, restored cache, offline rollback, state/content/theme assertions -> PASS | final runtime harness and uncached full suite -> PASS |
| 7.1 | `tests/release-bridge_test.sh` | Workflow/shell | N/A: no prior bridge harness | workflow gate assertion written first -> FAIL: bridge invocation missing | bridge + publication assertions -> PASS | config-latest, missing assets, same/different digest replay -> PASS | `actionlint` and final harness -> PASS |
| 7.2 | `tests/release-bridge_test.sh` | Shell integration | N/A (new) | recorded-fixture suite written before workflow/install changes -> RED | offline fixture suite -> PASS | newer config, incomplete CLI, and two republication cases -> PASS | final harness -> PASS |
| 7.3 | `tests/release-bridge_test.sh` | Shell integration | `bash -n scripts/install.sh` -> PASS | config-latest guard assertion written before installer change | fail-fast guard before download -> PASS | config release downloads nothing; valid `v*` reaches asset request -> PASS | syntax + harness -> PASS |
| 7.4 | `tests/release-bridge_test.sh` | Workflow contract | N/A: CI-only edit | isolated rerun -> FAIL: CI bridge invocation missing | exact bridge/race command assertions -> PASS | path filters trigger both checks on workflow changes -> PASS | `actionlint` -> PASS |
| 7.5 | `RELEASING.md` | Structural docs | N/A: no executable behavior | N/A: strict TDD scoped to executable behavior | required contract documented | Triangulation skipped: documentation-only | markdownlint 0 errors |

## Test Summary

- New top-level tests: 39 across command, theme, apply/status/rollback integration, and persisted-inventory recovery.
- Layers: unit, filesystem integration, command integration, and full apply/rollback runtime harness.
- External integration skipped: live GitHub is intentionally replaced by a deterministic in-memory `release.Client`; no privileged commands execute.
- Golden files: none.
- Phase 7 adds one shell harness with behavioral fixtures and no live network.
- Phase 7 validation: bridge harness, shell syntax, YAML parse, `actionlint`, markdownlint, and full Go race suite all pass.

## Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused command | `cd cli && go test -count=1 ./cmd/ -run 'TestConfigApply\|TestConfigStatus\|TestThemePhase\|TestApply_NeverCallsRunner\|TestConfigRollback\|TestConfigIndeterminate'` -> PASS (`ok github.com/MrUse77/dots-cli/cmd 0.032s`) |
| Runtime harness | `cd cli && go test -count=1 ./cmd/ -run '^TestConfigApply_EndToEnd$' -v` -> PASS (`TestConfigApply_EndToEnd`, `ok .../cmd 0.018s`). Scenario: exact A apply, exact B apply, tampered retained A rejection, restored verified A offline rollback, identities/content/theme validated, zero rollback network calls. |
| Relevant package suites | `go test -count=1 ./cmd/`; `go test -count=1 ./pkg/installer/...`; `go test -count=1 ./pkg/release/` -> PASS |
| Full suite | `cd cli && go test -count=1 ./...` -> PASS for all packages |
| Static/build | `cd cli && go vet ./...` -> PASS; `cd cli && go build ./...` -> PASS |
| Rollback boundary | Revert only `cli/cmd/{config.go,config_runtime.go,theme_phase.go}` and their tests, `cli/pkg/installer/transaction/{recovery.go,recovery_test.go}`, the indeterminate exit-code mapping in `cli/cmd/root.go`, and Phase 6 OpenSpec progress/checkmarks. Phases 1-5 remain intact. |

## Phase 7 Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused command | `bash tests/release-bridge_test.sh` -> PASS: legacy bridge fixtures, immutable identity, and installer guard |
| Runtime harness | Same command exercises recorded latest/config release API fixtures plus fake `curl`; no live network; config-latest fails before asset download |
| Broader checks | `bash -n` -> PASS; YAML parse -> PASS; `actionlint` -> PASS; markdownlint -> 0 errors; `cd cli && go test -race -count=1 ./...` -> PASS (8 tested packages, 2 without tests) |
| Rollback boundary | Revert `.github/workflows/{release,ci}.yml`, `scripts/install.sh`, `tests/release-bridge_test.sh`, `RELEASING.md`, and Phase 7 OpenSpec checkmarks/evidence only. Phases 1-6 remain intact. |

## Deviations

- Behavioral deviations: none. The implementation preserves exact selection, fail-closed drift, no external runner, separate mutable theme handling, conservative journal recovery, and offline rollback.
- File-layout deviation: orchestration lives in `cli/cmd/config_runtime.go`, and persisted-inventory recovery lives in a focused transaction file, rather than placing all behavior in `config.go`. This keeps Cobra wiring small and crash recovery beside transaction internals.
- Delivery-plan override: the task forecast originally suggested separate config and rollback PR slices; the maintainer explicitly assigned all tasks 6.1-6.7 to PR5 with a 5,000-line size exception.
- Phase 7 deviations: none; executable behavior follows the publication design and documentation extends the existing Spanish artifact language.

## Remaining Tasks

- None. Tasks 1.1-7.5 are complete (39/39).

## PR Boundary

- Start: merged Phase 5 planner/transaction removal and inventory provenance (27/39 tasks).
- End: complete Phase 6 config apply/status/rollback/theme/drift behavior and verification (34/39 tasks).
- Out of scope: every Phase 7 publication/bridge/docs change.

## Phase 7 PR Boundary

- Strategy: stacked-to-main PR6, based on merged PR5 commit `156855f`.
- End: config publication, legacy bridge, installer guard, CI wiring, docs, and focused verification.
- Out of scope: commits, branches, pushes, issues, PR creation, and unrelated configuration changes.
