# Safe Dotfiles Installer Design

## Overview

The installer becomes a two-phase operation:

1. **Read-only planning and review:** collect the existing install choices, resolve the repository and home paths, discover the complete closed set of managed targets, snapshot their pre-state, and construct a single immutable `InstallationPlan`.
2. **Confirmed execution:** the exact reviewed plan is executed once. Managed targets run in a recoverable filesystem transaction; external actions run only after that transaction commits and are reported separately because they are not rollbackable by the installer.

This design stays inside `cli/`. It replaces the mutable orchestration currently concentrated in `cmd/install.go`; it does not alter the dotfiles repository contents, add action toggles, or make package/system changes reversible.

## Architecture and Ownership

Add the following packages under `cli/pkg/installer/` and keep `cmd/install.go` as Cobra composition only:

| Component | Responsibility | Key seam |
|---|---|---|
| `plan` | Builds, validates, fingerprints, and renders the closed installation plan. | `Planner`, `TargetDiscoverer`, `StateReader`, `ActionCatalog` |
| `transaction` | Creates retained backups, stages safe mutations, commits managed targets, and rolls back. | `Filesystem`, `Clock`, `RunIDSource` |
| `external` | Executes predeclared external actions after transaction success. | `CommandRunner` |
| `ui` | Collects choices and drives review, confirmation, progress, and terminal result states. | `tea.Model`, `Executor` |
| `report` | Carries typed phase, target, rollback, and external-action outcomes to UI and Cobra. | Pure result/error types |

`cmd/install.go` resolves dependencies, invokes `ui.Run(planner, executor)`, prints the final report, and returns a Cobra error for every execution failure. It must no longer call installer functions as choices are collected or copy files directly.

Existing `pkg/installer` functions become implementations behind plan actions and command execution; their public behavior is not silently invoked during planning.

## Plan Discovery, Binding, and Drift

### Plan model

`InstallationPlan` is immutable after `Build` and contains:

- `RunID`: UTC timestamp plus cryptographically random suffix, allocated before planning and used in deterministic backup paths.
- `Options`: existing mode/AMD/plugin/SSH-agent selections.
- `ManagedTargets`: ordered, complete target records for `.config` entries, root-level `.zshrc`, `.gtkrc-2.0`, `oh-my-posh`, `.zsh_plugins`, `.themes`, font/cursor destinations, and installer-managed profile files when selected by the current action catalog.
- `ExternalActions`: ordered package, network, service, shell, cache, gsettings, and plugin operations, each with a display description, `CommandSpec`, classification, and irreversible warning.
- `Fingerprint`: SHA-256 over a canonical serialization of options, ordered target identities/mutations/pre-states, and ordered external action specs.

A `Target` has a repository source, absolute destination, mutation kind (`copy-file`, `copy-tree`, `symlink` only when supported), planned pre-state, and backup location. Target identity is the cleaned absolute destination; duplicate or ancestor/descendant overlapping targets are planning errors unless represented as one directory target. Sources must be beneath the resolved repository root. All destination paths are absolute and cleaned before use.

`PreState` is captured with `Lstat`, never by following a destination symlink. It records absent/file/directory/symlink type, mode, link value where applicable, and a deterministic tree digest for existing content. A digest includes sorted relative names, type, mode, symlink value, and file bytes. Unsupported special files are a planning error. This makes discovery complete and provides the execution binding without reading mutable state again to re-plan.

The planner performs only read operations and prerequisite checks: source readability, destination parent accessibility, target shape, and ability to create each backup root. It creates no backup and runs no command. A failed check produces a planning error and the UI never enters review.

Immediately before each managed mutation, `Transaction` re-reads the target pre-state and requires it to equal the bound `PreState`. For an absent target it requires continued absence. A mismatch is `PlanDriftError`: the target is not changed, later targets are not started, and already committed targets are rolled back. The executor never discovers targets or appends actions after confirmation.

## TUI State and Data Flow

Huh remains the small choice collector for the existing mode and optional features. Huh already uses Bubble Tea internally. After choices are available, use a dedicated Bubble Tea review model rather than a second generic Huh form so the complete plan and execution progress have explicit, testable state.

```
Cobra -> Huh choices -> Planner.Build (read-only)
      -> ReviewModel(plan) -> confirmed exact plan -> Executor.Execute(plan)
      -> Transaction managed phase -> ExternalRunner phase -> ResultModel/report
```

`ReviewModel` states are `initializing`, `review`, `executing`, `done`, `aborted`, and `error`.

- `initializing` receives either `planReadyMsg` or `planFailedMsg`.
- `review` renders all `ManagedTargets` and all `ExternalActions` before it accepts confirmation. `reviewRendered` is set only after the full list is available; only then are `Confirm all` and `Abort` actionable.
- Confirm emits the stored plan value, not a rebuilt plan. Abort, escape, Ctrl-C, and Bubble Tea termination before confirmation return `Aborted` with no executor call and exit status zero.
- `executing` receives progress messages from the executor for backup, mutation, rollback, and external action events. It has no per-action selection controls.
- `done` displays retained backup locations and successful outcome. `error` displays the typed failure and, if relevant, rollback completeness and manual recovery paths.

Execution runs as a Bubble Tea command after confirmation. The executor owns cancellation via `context.Context`; normal returned errors trigger rollback if managed mutation has begun. Uncatchable process death cannot promise automatic rollback; retained inventories provide manual recovery.

## Filesystem Transaction

### Inventory and backups

`Transaction.Prepare(plan)` allocates an `Inventory` before any managed mutation. For every target it creates a unique, deterministic backup entry:

```
<target-parent>/.dots-backups/<RunID>/<escaped-absolute-target>
```

The backup root is adjacent to the target so it is on the same filesystem and can support atomic directory swaps; it is mode `0700` where permissions allow. `escaped-absolute-target` is a reversible, path-safe encoding. Existing paths for the same run and target are collisions and fail preparation; they are never overwritten. The inventory records target, original pre-state, backup path, mutation status, restore status, and errors. It is retained on disk after success and rollback.

Before a target mutation, the transaction:

1. Performs the bound pre-state drift check.
2. Creates a backup copy of the current target without shell commands, preserving content, modes, directory structure, and symlink values; absent targets receive an explicit `Absent` inventory entry rather than an invented file.
3. Validates that the entry exists, is readable, and has the digest/type recorded in the inventory (or validates the absent marker).
4. Only then stages and commits the mutation.

A backup, validation, or staging failure occurs before that target is changed. If earlier targets were committed, it starts rollback; otherwise it exits without mutation.

### Atomic mutation and rollback

`Filesystem` uses `os`, `io`, `filepath`, and `os.Symlink`; it never assembles a shell command for copy, move, or link operations. Files are copied to a uniquely named sibling temporary path, synced/closed, assigned the planned mode, and atomically renamed into place. Directory targets are copied to a sibling staging directory and swapped only after it is complete. Existing targets are moved atomically to their retained same-filesystem backup entry before the staged replacement is renamed into the target location. For absent targets, no original is moved.

The implementation must reject target/backup layouts where an atomic rename is impossible rather than falling back to delete-then-copy. Parent directories are created only when their creation is itself represented by the planned target operation.

A target is appended to `mutated` immediately after its original has been moved or its destination has been changed, so a partial commit is eligible for restoration. On a handled failure, `Rollback` walks `mutated` in reverse order, removes the replacement safely, and restores the original from its retained backup (or removes a target that was absent before the run). It continues after individual restoration failures and returns `RollbackError` containing every failed target. Backups are never deleted. A successful rollback is still an installation failure; incomplete rollback is a distinct, higher-severity failure with inventory paths in the report.

## External Command Boundary and Sequencing

`CommandSpec` is structured (`Name`, `Args`, `Dir`, `Environment`, `Classification`, `Irreversible`) and is displayed directly in the plan. `CommandRunner.Run(ctx, spec)` constructs `exec.CommandContext(spec.Name, spec.Args...)`, sets terminal streams, and returns command/exit errors with action identity. It never uses `sh -c` or interpolated command strings. Commands requiring a working directory, such as `makepkg`, use `spec.Dir`.

Planning classifies current operations explicitly: pacman/sudo/systemctl/chsh/profile writes as `privileged`; Git clone, AUR build, package installs, and hyprpm repository/plugin operations as `supply-chain`; gsettings and `fc-cache` as `external`. The plan labels every such action as outside managed-target rollback where applicable. File copies currently done through `sh -c cp` for fonts/cursors move into managed transaction targets and use `Filesystem` instead.

The executor sequence is fixed:

1. Validate and run all managed filesystem mutations transactionally.
2. Mark the managed transaction committed and retain its inventory.
3. Run external actions in reviewed order.

A managed failure prevents all external actions. An external failure stops subsequent external actions, reports non-zero, and does **not** roll back the committed managed transaction. This is intentional: package, AUR, service, network, shell, cache, and system state are never claimed to be transactional.

## Error and Reporting Contract

Errors are typed and retain phase context: `PlanError`, `PlanDriftError`, `BackupError`, `MutationError`, `RollbackError`, and `ExternalActionError`. `ExecutionReport` includes the immutable fingerprint, per-target `pending/backed-up/mutated/restored/failed` outcomes, backup inventory paths, per-action status, primary cause, and all rollback failures.

The UI receives only report/progress values, not raw filesystem handles. Cobra maps `Aborted` to exit zero; planning, managed execution, rollback, and external errors return non-zero. A failed external action clearly says that managed files were retained and that its external effects were not rolled back. A rollback failure lists each target and retained backup path for manual recovery.

## Strict TDD Test Seams

No test reads a real home directory, invokes a package manager, or requires a terminal:

- `Planner` receives a fake repository/home resolver, `StateReader`, `ActionCatalog`, `Clock`, and `RunIDSource`. Table-driven tests prove closed discovery, root-level coverage, canonical fingerprints, duplicate rejection, source containment, and no mutations on planning errors.
- `Filesystem` is exercised in `t.TempDir()` with real temporary files. Tests prove backup-before-write, absent-target inventory, deterministic collision rejection, atomic staging behavior, drift abort, retained backups, and reverse-order rollback including continued rollback after one restore failure.
- `CommandRunner` is an interface fake for unit tests. Command integration tests use a helper process, are guarded by `testing.Short()`, and assert argument boundaries rather than shell strings.
- `ReviewModel.Update()` is tested directly with plan-ready, confirm, abort, and execution-result messages. Assertions prove that confirmation is unavailable before complete rendering, no per-item toggle event exists, decline does not call the executor, and confirmation passes the same plan fingerprint/value. Use `teatest` only for a narrow end-to-end interaction if direct state tests cannot prove the flow.
- Executor tests use fake transaction and runner to verify managed-before-external ordering, no external run after managed failure, and no managed rollback after external failure.

Follow strict TDD in implementation: add the failing scenario test first, implement the smallest behavior to make it pass, add boundary cases, then refactor while preserving the suite.

## Migration and Compatibility

1. Introduce plan, transaction, command, report, and UI seams beside the current installer functions with tests before replacing Cobra orchestration.
2. Convert existing direct file copy and `backupConflicts` behavior into discovered managed targets and retained inventories. The old `.config-backup-*` move-based convention is not reused because it lacks complete/root-file coverage and rollback metadata; existing backup directories remain untouched.
3. Convert current installer functions to produce/execute structured external actions. Replace font/cursor shell copies first; eliminate all managed-path shell copies before removing the legacy helpers.
4. Switch `installCmd` to the plan/review/executor flow only when the complete action catalog is represented. Existing user-mode intent remains supported. The currently placeholder dev mode must remain non-mutating and return an explicit unsupported planning result until a complete stow target model exists; it must not bypass review or execute hidden work.
5. Retain the Cobra command name and existing interactive choice semantics. Do not add flags, configuration migration, action selection, documentation, or CI scope in this change.

## File Changes

- Modify `cli/cmd/install.go` to compose the new flow and remove direct mutation orchestration.
- Add focused planner, transaction, external-runner, UI, and report files under `cli/pkg/installer/` plus their tests.
- Adapt `cli/pkg/installer/packages.go`, `system.go`, `gsettings.go`, `hyprland.go`, and `utils.go` behind structured actions; remove managed shell copy paths.
- Add `cli/pkg/installer/*_test.go` for the seams above.
- Update only this change's OpenSpec `design.md` and `state.yaml` during design.

## Rollout

Ship behind the existing `dots install` command with no alternate runtime path. The first release should manually exercise decline, a successful temporary-user installation fixture, a forced managed mutation failure, and a forced external failure. Existing backup folders are preserved; each new run receives a separate retained inventory. There is no automatic backup cleanup in this slice.

## Risks

- Atomic directory replacement depends on same-filesystem sibling backup/staging paths and permissions; planning must reject unsupported layouts before confirmation.
- Root-owned system targets may require privilege to prove backup and restore capability. Failure to prove it blocks the plan rather than attempting a partial installation.
- Hard termination cannot guarantee in-process rollback. The retained inventory is the recovery boundary, not crash-resume support.
- Existing external functions include some best-effort behavior (notably hyprpm); converting them to explicit action results is required so failures are visible rather than swallowed.
