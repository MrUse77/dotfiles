# Proposal: Safe Dotfiles Installer

## Intent

Replace the Go installer's current interaction with a guided terminal UI that presents the complete installation plan before any system mutation and executes that plan only after one explicit final confirmation.

The change addresses the risk that installation can currently mutate a user's environment without a complete preview, comprehensive backups, or transactional recovery. It must specifically account for unsafe shell-based copying, incomplete backup coverage for root-level files, non-transactional writes, and privileged or supply-chain actions that are enabled by default.

## Product outcome

A user starting the installer can review every planned managed-target mutation and any privileged or supply-chain action in one coherent plan. Nothing is changed before confirmation. Confirming once runs the entire plan as one managed operation. Every managed target has a retained backup, and a failure after mutation starts triggers automatic rollback of managed targets while preserving backups for manual recovery.

## Scope

### In scope

- Changes within the Go installer under `cli/`.
- A guided TUI that builds and displays the full installation plan before mutation.
- A read-only review step covering all managed targets and clearly identifying privileged or supply-chain actions.
- One final confirmation for execution of the entire plan.
- Backup creation and retention for every managed target, including root-level files managed by the installer.
- Transactional orchestration of managed-target mutations.
- Automatic rollback of managed targets when execution fails after mutation begins.
- Preservation of retained backups after success or rollback so users can recover manually.
- Removal or containment of unsafe shell-copy behavior in the managed installation path.
- Safe handling of current default privileged and supply-chain actions within the plan-and-confirm flow.

### Out of scope

- CI changes.
- README or other documentation changes.
- Configuration validation.
- Per-action selection or toggles in the TUI.
- Changes outside `cli/`, including root dotfile configuration.
- Redesigning the content or purpose of installation actions beyond what is required to preview, back up, execute, and recover them safely.

## Business and safety rules

1. The installer MUST complete planning before performing any mutation.
2. The TUI MUST show the full plan before asking for confirmation.
3. The initial TUI MUST NOT allow individual actions to be enabled or disabled.
4. A single explicit confirmation MUST authorize the complete displayed plan; declining or abandoning confirmation MUST leave the system unchanged.
5. Every managed target MUST have a retained backup before that target is mutated.
6. Backup coverage MUST include root-level files managed by the installer, not only directory-based or stowed targets.
7. If execution fails after any managed-target mutation, the installer MUST automatically restore managed targets to their pre-installation state.
8. Automatic rollback MUST NOT delete the retained backups.
9. Privileged and supply-chain actions MUST be visible in the plan and MUST NOT run before final confirmation.
10. Managed file operations MUST avoid unsafe shell interpolation and copy behavior.
11. The operation MUST fail safely if planning, backup creation, or prerequisites cannot establish a recoverable execution path.

## Affected areas

- `cli/` TUI flow and installer command orchestration.
- Installation action modeling and plan rendering.
- Managed-target discovery and root-file coverage.
- Backup creation, retention, and restoration.
- File mutation mechanisms currently relying on shell copy operations.
- Error propagation and rollback coordination.
- Invocation and presentation of privileged or supply-chain actions.
- Existing `cli/openspec/changes/` work related to installer TUI and installer logic, which should be reconciled during specification and design to avoid conflicting behavior or duplicate scope.

This proposal affects only the CLI subproject. It does not change root configuration files, although the installer may continue to manage those files at runtime.

## Risks and tradeoffs

- **Rollback completeness:** An incomplete inventory of managed targets could leave partial mutations after failure. The design must establish the target set before execution.
- **Backup reliability:** Retaining every backup may consume disk space and requires deterministic naming and collision handling.
- **Irreversible external effects:** Package installation, network downloads, or other privileged/supply-chain actions may not be fully reversible like file mutations. The design must distinguish managed-target rollback guarantees from external side effects and order actions to minimize exposure.
- **Plan drift:** Environment changes between preview and execution could make the displayed plan stale. Execution must detect material drift or bind execution to the reviewed plan.
- **Permission failures:** Root-owned targets may prevent backup or restoration. The installer must prove it can establish recoverability before mutating each managed target.
- **TUI interruption:** Cancellation, terminal loss, or process interruption during execution could bypass normal in-process rollback; durable backup retention remains essential for manual recovery.
- **Larger initial flow:** Removing per-action choice simplifies the safety contract but means users cannot skip unwanted actions in this delivery.

## Rollback strategy

The implementation itself can be rolled back by reverting the `cli/` changes and restoring the previous installer flow; no repository configuration migration is introduced by this proposal.

At runtime, the installer will create retained backups before mutation. If a managed operation fails after mutation begins, it will restore all managed targets already touched, in reverse execution order where ordering matters, while leaving the backup set intact. Privileged or supply-chain side effects that cannot be transactionally reversed must be explicitly represented as such and sequenced to minimize the chance of an unrecoverable partial installation.

## Success criteria

- Users see the complete installation plan before any mutation.
- The plan clearly includes managed targets and identifies privileged or supply-chain actions.
- No installation action executes without one final affirmative confirmation.
- Declining or exiting before confirmation produces no mutation.
- Every managed target, including root-level files, receives a retained backup before mutation.
- A failure after mutation begins automatically restores all managed targets touched by the operation.
- Backups remain available after successful execution and after rollback.
- Managed copying no longer depends on unsafe shell command construction or interpolation.
- A backup or recoverability failure prevents mutation rather than allowing a partially protected install.
- The delivered change remains confined to `cli/` and does not add action toggles, config validation, CI, or documentation work.

## Proposal question round

The approved decisions define the first product slice. The following questions should be resolved during specification or design; the assumptions below keep the proposal actionable without expanding scope:

1. **How long should retained backups remain available?** Assumption: the installer does not automatically delete prior backup sets in this delivery; retention cleanup is a later concern.
2. **What should happen when an external privileged or supply-chain action cannot be rolled back?** Assumption: the UI labels it clearly, execution orders recoverable file mutations and external effects to minimize risk, and the managed-target rollback guarantee remains strict without claiming reversal of external systems.
3. **How should plan drift between review and execution be handled?** Assumption: the reviewed plan is bound to the execution, and material target-state changes cause a safe abort before mutation rather than silent replanning.
4. **What is the expected recovery behavior after a hard process or machine interruption?** Assumption: automatic rollback is guaranteed for handled runtime failures; retained backups support manual recovery after uncatchable termination, while crash-resumable transactions are not part of the first slice unless design shows they are required for the stated guarantee.

These assumptions need product review before the specification hardens acceptance scenarios.
