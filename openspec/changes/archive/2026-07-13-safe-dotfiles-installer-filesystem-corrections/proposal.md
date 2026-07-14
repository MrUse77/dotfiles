# Proposal: Correct Critical Filesystem Transactions in the Dotfiles Installer

## Intent

Harden the Go installer's filesystem transaction boundary against hostile local manipulation and partial swap or rollback failures without rewriting the existing installer change history.

This is a new, independent OpenSpec change. It preserves the existing `safe-dotfiles-installer` slice history while correcting critical gaps in source binding, backup isolation, durable recovery metadata, rollback ownership, directory replacement, and metadata preservation.

## Product outcome

A planned installation either applies the exact reviewed filesystem content with recoverable state recorded durably, or fails without trusting attacker-controlled path substitutions. If a swap or rollback cannot complete, the installer preserves the remaining recoverable artifacts and reports their locations and state instead of cleaning them up or implying that recovery succeeded.

Existing direct internal `Target` construction remains compatible when `SourceDigest` is absent. Plans produced by the planner, however, enforce source binding and reject source identity or content drift before mutation.

## Scope

### In scope

- Treat local source paths, source symlinks, destination symlinks, backup roots, and recovery inventory paths as hostile inputs that may be manipulated concurrently.
- Bind planner-built targets to the source identity and digest reviewed during planning, and reject material source drift before target mutation.
- Preserve compatibility for legacy direct internal `Target` values that omit `SourceDigest`; digest enforcement is mandatory only when a binding is present or when the planner builds the target.
- Persist protected recovery inventory atomically so interruption cannot expose a partially written inventory as valid recovery state.
- Validate and create backup roots without following attacker-controlled symlinks or accepting unsafe ownership or path substitution.
- Make rollback ownership-aware so it restores or removes only artifacts created or displaced by the active transaction.
- Preserve recoverable state and report actionable recovery details when a target swap, directory swap, or rollback step fails.
- Correct directory replacement behavior so failures during multi-step swaps do not discard the original directory, staged replacement, backup, or inventory needed for recovery.
- Preserve exact filesystem mode bits required by the source or pre-installation target state rather than broadening or normalizing permissions accidentally.
- Add focused automated coverage for hostile path manipulation, TOCTOU checks, persistence interruption, swap failures, rollback ownership, compatibility behavior, and exact mode preservation.

### Out of scope

- Reopening, rewriting, or squashing the existing `safe-dotfiles-installer` slice-2 history.
- Redesigning the TUI, plan presentation, confirmation flow, or action selection model.
- Adding new installer features, managed targets, package actions, privileged actions, or supply-chain actions.
- General-purpose sandboxing or protection against an attacker with control of the running installer process, its executable, or equivalent privileges.
- Crash-resumable execution that automatically continues a transaction after process or machine restart; this slice preserves and reports recoverable state for manual or later recovery.
- Backup retention policy, garbage collection, or automatic deletion of historical backup sets.
- A public API guarantee for arbitrary external construction of internal transaction models.
- CI, README, or unrelated documentation changes.
- Changes outside the Go installer under `cli/`, except OpenSpec artifacts for this change.

## Business and safety rules

1. The threat model MUST assume a hostile local actor can replace or redirect accessible sources, symlinks, backup roots, and inventory paths between checks and use.
2. Planner-built plans MUST bind each managed source to the identity and digest reviewed during planning and MUST fail safely on binding drift before mutating that target.
3. Source validation MUST account for TOCTOU risk; a path-only pre-check followed by an unbound reopen is insufficient.
4. Legacy direct internal targets without `SourceDigest` MUST remain executable under their existing compatibility semantics; they MUST NOT be rejected solely because the digest is absent.
5. Planner-built targets MUST include the binding data required for enforcement. Compatibility for legacy direct targets MUST NOT weaken planner-built plan binding.
6. Backup and recovery roots MUST be created and accessed without trusting symlink traversal or attacker-substituted path components.
7. Recovery inventory MUST be written atomically and protected with restrictive permissions before it is treated as authoritative.
8. Rollback MUST distinguish transaction-owned artifacts from pre-existing or concurrently substituted artifacts and MUST NOT delete or overwrite paths it cannot prove the transaction owns.
9. A swap or rollback failure MUST preserve every still-useful original, staged replacement, backup, and inventory artifact.
10. Failure reporting MUST identify incomplete recovery, affected targets, retained recovery locations, and the safest available next action; it MUST NOT report a full rollback when recovery is partial.
11. Directory replacement MUST maintain a recoverable ordering across rename or swap steps, including failure after the original has moved but before the replacement becomes active.
12. File and directory mode preservation MUST retain exact applicable permission and special mode bits rather than applying permissive defaults or process-umask-derived approximations.
13. Safety-check failure or ambiguity MUST stop mutation of the affected target instead of falling back to a less safe filesystem operation.

## Affected areas

- `cli/` planning models and planner construction of managed targets.
- Source inspection, identity binding, digest verification, and file opening/copying paths.
- Filesystem abstraction and symlink-safe path traversal or creation primitives.
- Backup-root validation, backup creation, and retained recovery layout.
- Recovery inventory serialization, permissions, atomic replacement, and error reporting.
- Transaction execution, file and directory swap ordering, ownership tracking, and rollback coordination.
- Permission and mode capture, application, and restoration.
- Unit and integration tests around installer filesystem transactions.

The specification and design should identify exact symbols and package boundaries from the current `cli/` implementation. This proposal does not presume a particular low-level API, but the resulting design must close the check/use gap rather than merely add more path validation.

## Risks and tradeoffs

- **Platform semantics:** Symlink-safe traversal, descriptor-relative operations, rename guarantees, and special mode bits vary by operating system and filesystem. The design must state supported platforms and fail closed where guarantees cannot be established.
- **Compatibility escape hatch:** Allowing digest-less direct internal targets preserves existing callers and tests but creates weaker semantics than planner-built plans. The boundary must remain explicit and must not become the planner default.
- **Recovery over cleanup:** Preserving multiple artifacts on failure can consume disk space and leave an untidy state, but deleting ambiguous state would increase data-loss risk. Recoverability takes priority.
- **Conservative ownership checks:** Rollback may stop rather than overwrite a concurrently changed target. This can require manual recovery, but it avoids destroying changes the transaction does not own.
- **Atomicity limits:** Atomic inventory replacement protects committed metadata, but filesystem or machine failures can still leave staged files. Reports and retained paths must make that state diagnosable.
- **Implementation breadth:** Correcting source binding, safe path handling, inventory durability, swap recovery, rollback ownership, and modes crosses several transaction layers. Combining them carelessly would create a large, difficult-to-review security change.
- **Error complexity:** More precise partial-recovery reporting introduces richer failure states. Tests and user-facing summaries must prevent those states from collapsing into a misleading generic failure.

## Rollback strategy

The repository change can be rolled back by reverting this independent change without modifying the historical `safe-dotfiles-installer` slices. No persistent repository migration is introduced.

At runtime, rollback is deliberately conservative. It restores a prior target only when transaction ownership and expected identity can be established. If restoration or cleanup fails, the installer leaves the original, staged replacement, retained backup, and protected inventory in place where available, marks recovery as incomplete, and reports exact recovery locations. A failed rollback must never trigger best-effort cleanup that destroys the remaining recovery path.

Any later format change to recovery inventory must be versioned or remain readable by the implementation that creates it; this proposal does not authorize an incompatible inventory migration without an explicit design decision.

## Success criteria

- Planner-built targets carry source-binding data and reject source identity or digest drift before target mutation.
- Source consumption does not rely on a path-only check followed by an independently resolved read vulnerable to symlink substitution.
- Legacy direct internal targets without `SourceDigest` continue to work, while planner-built plans cannot silently omit binding enforcement.
- Backup roots and recovery inventory cannot be redirected through attacker-controlled symlinks or accepted with unsafe ownership assumptions.
- Recovery inventory becomes authoritative only through an atomic persistence path and uses restrictive exact permissions.
- Rollback removes or replaces only transaction-owned artifacts and preserves concurrently substituted or ambiguous paths for investigation.
- Injected failures at every directory swap boundary retain enough state to recover the original target or complete recovery manually.
- Swap and rollback failures return a partial-recovery result that names affected targets and retained recovery locations.
- Exact expected mode bits are preserved for installed content and restored targets, including covered special bits where supported.
- Focused tests reproduce the hostile manipulation and failure cases and demonstrate fail-safe behavior without regressing digest-less direct-target compatibility.
- The implementation remains confined to the confirmed critical filesystem-transaction corrections and does not alter the TUI or prior change history.

## Confirmed proposal decisions

The proposal question round is complete. The user confirmed:

- The local filesystem threat model is hostile, including concurrent manipulation of sources, symlinks, backup roots, and inventory paths.
- Recoverable state must be preserved and explicitly reported when swap or rollback cannot complete.
- Legacy direct internal targets without `SourceDigest` remain compatible, while planner-built plans enforce source binding.
- This work is a new independent OpenSpec change so the existing slice-2 history remains unchanged.

## PR and workload forecast

- **Estimated changed lines:** 500–900 across filesystem primitives, transaction orchestration, recovery reporting, and failure-injection tests.
- **400-line budget risk:** High.
- **Chained PRs recommended:** Yes.
- **Decision needed before apply:** Yes — select and record the chain strategy and the first implementation boundary.
- **Review target:** Keep each PR at or below 400 changed lines and reviewable in approximately 60 minutes, with tests in the same slice as the behavior they verify.

A likely review decomposition is:

1. Source binding, TOCTOU-safe consumption, exact mode handling, and digest-less direct-target compatibility.
2. Symlink-safe backup roots and atomic protected recovery inventory.
3. Ownership-aware rollback, directory swap failure recovery, and partial-recovery reporting.

The final task plan may adjust these boundaries to match actual package dependencies, but it must not combine the full forecast into one oversized PR without an explicit `size:exception`.
