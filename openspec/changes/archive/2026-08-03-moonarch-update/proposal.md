# Add a safe MoonArch update command

## Intent

Add a release-only `moonarch update` command that brings both the MoonArch CLI binary and its managed dotfiles to the latest published SemVer release. The command should make the supported update path discoverable and repeatable while preserving checksum verification, pinning configuration to the same release tag, and limiting reapplication to file-based configuration protected by the existing transaction and backup/rollback machinery.

Development builds remain deliberately outside this workflow. When `cmd.Version == "dev"`, the command must stop before any network, repository, binary, or configuration operation and explain that updates are available only from release builds.

## Problem statement

MoonArch currently has no in-CLI update workflow. Release discovery, architecture selection, binary download, checksum verification, and installation are implemented only in `scripts/install.sh`. A user who already has MoonArch installed must therefore know to rerun or reproduce installer behavior rather than invoke an explicit update command.

That gap creates several risks:

- The installed CLI and the managed clone under `~/.cache/dotfiles` can drift to different versions.
- Reusing the general install path could unexpectedly run package installation, Hyprland plugin refreshes, or external actions when the user only intended to update MoonArch and its files.
- The repository acquisition defaults are not sufficient for this workflow: update must resolve the latest release and pin the clone to that exact tag, never to `main`.
- Ad hoc binary replacement can leave a corrupt or partially downloaded executable if release integrity and atomic replacement are not handled together.
- Development builds use `Version == "dev"` and currently fall back to `main` when building a repository request, which is explicitly unsafe for this update contract.

## Proposed solution

Register a new Cobra `update` command and implement a staged updater with an early release-build guard. The command will:

1. Reject `Version == "dev"` as a successful no-op with a clear release-build-only message, before creating clients or performing side effects.
2. Resolve GitHub's latest published release, validate its tag as SemVer, and compare it with the injected current `Version`.
3. Select the existing Linux release asset for `amd64` or `arm64`, download it to a temporary file, verify its entry from `SHA256SUMS.txt`, and atomically replace the managed executable only after verification succeeds.
4. Acquire or refresh `~/.cache/dotfiles` with an explicit repository request whose ref is the resolved release tag. The update path must not inherit the development fallback to `main`.
5. Reapply only `ConfigurationActions()` through the existing installer executor and `transaction.New()` prepare/commit/rollback lifecycle.
6. Report the outcome of the binary, repository, and configuration stages separately so a partial failure is visible and recoverable.

### Release logic recommendation

Port the required release logic from `scripts/install.sh` into Go rather than shelling out to the script.

| Option | Assessment |
| --- | --- |
| Native Go release client | **Recommended.** It avoids adding runtime dependencies on shell tools, supports typed HTTP and filesystem errors, allows checksum and atomic-replacement behavior to be unit tested through dependency injection, and integrates cleanly with Cobra and the CLI's existing fake-based tests. |
| Shell out to `scripts/install.sh` | Not recommended. The script is an installation entry point rather than a stable update API, mixes release acquisition with installation concerns, is harder to constrain to the locked update scope, and would make structured progress and error handling depend on parsing subprocess behavior. |

The Go implementation should preserve the release protocol already established by the script: GitHub's latest-release endpoint, the `moonarch-cli-linux-${arch}` asset naming convention, and verification against `SHA256SUMS.txt`. Keeping the shell installer and Go updater consistent becomes an explicit maintenance responsibility; redesigning the installer is not part of this change.

Binary replacement is a separate rollback domain and must **not** be added to the configuration transaction. The downloaded binary is staged and verified independently, while only file-based configuration participates in the existing backup/rollback transaction.

## Scope

### In scope

- Add and register the `moonarch update` Cobra command.
- Disable the command for `Version == "dev"` before all update side effects.
- Query the latest published GitHub release and validate its tag as SemVer.
- Compare the installed release version with the latest release version and avoid redundant binary replacement when they are equal.
- Port release asset selection, download, and SHA-256 verification from `scripts/install.sh` to testable Go code.
- Support the currently published Linux `amd64` and `arm64` binary assets.
- Stage the binary in the destination filesystem and atomically replace the managed executable only after successful checksum verification.
- Update an existing managed clone or create it when absent, always checking out the resolved release tag in detached mode with submodules initialized recursively.
- Reapply only the catalog's file-based `ConfigurationActions()` through the existing executor and transaction prepare/commit/rollback machinery.
- Provide clear stage progress, already-current output, partial-failure reporting, and TTY-safe behavior.
- Add unit and command tests using dependency injection and fakes, including strict no-side-effect coverage for development builds.
- Document the command's release-only behavior, update boundaries, and recovery path.

### Out of scope

- Installing or upgrading system packages, including any `paru` operation.
- Refreshing Hyprland plugins through `hyprpm`.
- Running catalog external actions or any other non-file installation action set.
- Updating development builds or using `main` as an update source.
- Adding prerelease, nightly, branch, arbitrary-ref, or downgrade channels.
- Background update checks, scheduled updates, or automatic invocation outside the explicit command.
- Adding release artifacts for operating systems or architectures not already supported by the installer contract.
- Refactoring `scripts/install.sh` into a shared executable or changing the release workflow solely to support this command.
- Enrolling binary replacement or repository acquisition in the configuration transaction.
- Guaranteeing a single coordinated rollback across binary, repository, and configuration state.

## Architecture overview

### Command orchestration

The Cobra command should remain a thin orchestrator over injected collaborators for release lookup, downloading, filesystem replacement, repository acquisition, and configuration execution. This follows the CLI's existing fake-and-dependency-injection testing style and keeps external effects controllable.

The first branch is always the development-build guard. No GitHub request, filesystem probe, repository fetch, transaction, or progress session may begin before that guard returns. Release builds then normalize and compare the injected version and the latest release tag as SemVer values rather than comparing raw strings.

The updater should expose a per-stage result for:

- release discovery and version comparison;
- binary verification and replacement;
- managed repository acquisition at the resolved tag; and
- configuration transaction prepare and commit.

The final mutation order and failure boundary must be made explicit in design so users never receive an undifferentiated success message after only one component changed.

### Release metadata and binary self-update

A native Go release client should query `https://api.github.com/repos/MrUse77/dotfiles/releases/latest`, validate the response and `tag_name`, and derive the same asset URLs used by the installer. The checksum parser must select the exact architecture-specific filename from `SHA256SUMS.txt`; a missing, duplicate, malformed, or mismatched entry is a hard failure.

The binary should be downloaded to a temporary file in the target directory so the final rename stays on one filesystem. Verification must complete before executable permissions are applied and before the destination is replaced. Interrupted downloads, unsupported architectures, checksum failures, and permission failures must leave the existing binary untouched.

On Linux, replacing the executable path does not replace the image of the process that is already running. The specification and design must therefore confirm whether the current process finishes the dotfiles/configuration stages and reports that the new binary takes effect on the next invocation, or whether the updater performs one controlled re-exec into the verified binary. Binary replacement remains outside the configuration transaction in either case.

### Managed repository update

The updater should reuse `RepositoryAcquirer`, but construct an explicit request with:

- destination `~/.cache/dotfiles`;
- URL `https://github.com/MrUse77/dotfiles.git`; and
- ref equal to the latest validated release tag.

For an existing managed clone, acquisition fetches that ref, force-checks out detached `FETCH_HEAD`, and updates submodules recursively. If the cache does not exist, acquisition clones it fresh at the same tag. The update command must not call a request builder in a way that can translate `"dev"` to `main`.

### Configuration reapplication

After the release-tag checkout is available, the command should build the installer executor with only `ConfigurationActions()`. Those actions retain the current two-phase transaction behavior:

1. prepare file operations and backups;
2. commit when all preparation succeeds; and
3. roll back prepared configuration changes when preparation or commit fails according to the existing transaction contract.

Package, Hyprland plugin, and external action sets must never be appended to the update execution plan. Repository state and the CLI binary are not configuration transaction participants; a later configuration failure can therefore produce a visible partial update that requires the independent rollback procedure below.

### User experience and terminal behavior

Output should identify the current version, latest release tag, and each stage without exposing checksum or transport internals unless an error occurs. When the installed version equals the latest release, the recommended baseline is to print that the binary is already up to date, skip its download and replacement, and still ensure the managed dotfiles are pinned to that tag and reapply file configuration.

Network unavailability and GitHub API rate limiting should produce distinct, actionable failures. Release resolution must happen before update mutations, and the command must not silently use `main`, a stale tag, or an unverified cached binary as an offline fallback.

TTY behavior should reuse or match the installer's terminal conventions. Interactive terminals may receive structured progress, while redirected/non-TTY output must remain line-oriented, deterministic, and free of blocking prompts or terminal control sequences. The exact confirmation and flag surface remains a specification decision recorded below.

## Affected areas

| Area | Expected change |
| --- | --- |
| `cli/cmd/root.go` | Register the new `update` command. |
| `cli/cmd/version.go` | Continue using the release-injected `Version` value and enforce the `"dev"` guard. |
| New update command and release collaborators under `cli/cmd/` | Orchestrate release lookup, SemVer comparison, asset verification, binary replacement, repository acquisition, and configuration-only execution. Exact file boundaries belong in design. |
| `cli/cmd/repository_acquirer.go` | Reuse acquisition behavior with an explicit latest-release ref; adjust seams only if required for testable orchestration. |
| `cli/pkg/installer/` and `cli/pkg/installer/transaction/` | Reuse the existing executor, `ConfigurationActions()`, and backup/rollback transaction without broadening their action sets. |
| CLI tests | Add fake-driven command, release, filesystem, repository, and transaction coverage, including development no-op and failure cases. |
| User-facing documentation | Describe `moonarch update`, release-only availability, component boundaries, output, and recovery. |
| `scripts/install.sh` | Remain the reference for the established release protocol; no change is required by this proposal. |

## Benefits

- Users gain one explicit, supported command for keeping the CLI and managed dotfiles aligned to the same release.
- Checksum verification and atomic replacement prevent partially downloaded binaries from becoming active.
- Pinning to a release tag makes the resulting configuration reproducible and avoids accidental movement to `main`.
- Reusing the existing configuration transaction preserves file backup and rollback behavior without triggering package or external installation work.
- Native Go collaborators make network, version, integrity, and failure paths testable without invoking real release infrastructure.
- A hard development-build guard prevents local builds from mutating release-managed state.

## Trade-offs

- Porting the release protocol to Go duplicates some knowledge from `scripts/install.sh`, so asset naming and checksum behavior can drift unless covered by shared contract tests or release documentation.
- Binary, repository, and configuration updates have separate mutation and rollback boundaries; the command can report and recover from partial success but cannot make all three resources globally atomic.
- Continuing in the original process after replacement is simpler, but means the running code remains the old version until it exits. Re-execution would align behavior immediately but adds restart-loop, state-handoff, and testing complexity.
- Strict SemVer validation rejects unconventional release tags that the shell script might otherwise pass through as opaque strings.
- A no-confirmation command is efficient for automation, while an interactive confirmation can protect users from unintended configuration reapplication; supporting both consistently increases UX complexity.
- Unauthenticated GitHub API access is simple but subject to lower rate limits; optional authentication introduces credential-handling requirements.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| GitHub is offline, times out, returns malformed metadata, or rejects requests due to rate limits. | Use bounded HTTP requests, distinguish transport, API, and rate-limit errors, and stop before update mutations. Never fall back to `main` or an unverified cached release. |
| The latest tag or installed release string is not valid SemVer. | Normalize the supported leading `v` form, validate both values, and fail clearly before mutation rather than using lexical comparison. |
| A release asset is missing, truncated, or has the wrong checksum. | Require the exact architecture asset and checksum entry; stage to a temporary file and leave the current executable untouched on any mismatch. |
| Atomic rename fails because of permissions, symlinks, or a cross-filesystem temporary path. | Resolve and validate the managed target, create the temporary file in its destination directory, preserve executable permissions, and report a recoverable error without deleting the existing binary. |
| The running process continues to execute old code after its path is replaced. | Make process handoff an explicit design decision; either perform one guarded re-exec or clearly state that the verified binary becomes active on the next invocation. |
| Binary replacement succeeds but repository or configuration update fails. | Report per-stage state, retain configuration rollback evidence, and document independent binary and tag rollback steps rather than claiming global atomicity. |
| Updating the managed clone discards local edits under `~/.cache/dotfiles`. | Treat the path as an application-managed cache, state that expectation in documentation, and keep acquisition pinned to a reviewed release tag. |
| Non-configuration installer actions run accidentally. | Build the execution plan exclusively from `ConfigurationActions()` and verify with fake executors that package, plugin, and external action sets are never requested. |
| A configuration action fails after preparation. | Delegate file restoration to the existing transaction rollback machinery and surface the rollback outcome. |
| The Go updater and shell installer diverge in release naming or checksum parsing. | Encode the existing release URL and asset contract in focused tests and update both paths deliberately when release packaging changes. |
| TTY progress or confirmation blocks automation. | Detect terminal capability consistently with the installer and keep non-TTY output deterministic and non-blocking. |
| A development build reaches update collaborators. | Test that `Version == "dev"` returns before all injected network, filesystem, repository, and installer dependencies are called. |

## Rollback

Rollback follows the independent state boundaries rather than pretending the update is globally transactional:

1. If release lookup, download, checksum verification, or staging fails before atomic replacement, discard the temporary file; the installed binary remains unchanged.
2. If file configuration preparation or commit fails, use the existing transaction rollback and backup records to restore the prior file state.
3. If the binary was replaced before a later stage failed, reinstall the previous release's verified architecture asset at the managed executable path.
4. Restore the managed repository to the previous release tag with `RepositoryAcquirer` semantics, then reapply that release's file configuration or restore the transaction backup as appropriate.
5. Revert the implementation change through Git to remove the command if the feature itself must be withdrawn.

No package, plugin, or external-action state is changed by this command, so those systems require no update rollback.

## Success criteria

- `moonarch update` is registered and discoverable as a Cobra command.
- With `Version == "dev"`, the command exits as a no-op with a clear release-build-only message and makes no network, filesystem, repository, binary, or installer calls.
- A release build resolves and validates the latest published SemVer tag from the configured GitHub repository.
- Installed and latest versions are compared semantically, including the supported leading `v` tag form.
- When the versions match, the command reports that the binary is already up to date, skips binary replacement, and continues to reconcile dotfiles and file configuration at that tag.
- When a newer release exists, the command selects the correct Linux `amd64` or `arm64` asset, verifies it against `SHA256SUMS.txt`, and atomically replaces the managed executable.
- A missing asset, malformed checksum list, checksum mismatch, interrupted download, or replacement error leaves the previously installed binary usable.
- The managed repository is updated in place when present or cloned when absent, and its detached checkout resolves to the latest release tag rather than `main`.
- Only `ConfigurationActions()` are executed, using the existing transaction prepare/commit/rollback machinery.
- Package installation, `paru`, `hyprpm`, plugin refresh, and external actions are never invoked.
- Configuration failure triggers the existing rollback behavior and reports whether rollback succeeded.
- Binary, repository, and configuration stage outcomes are reported separately, including partial success.
- Offline, timeout, malformed-release, unsupported-architecture, and GitHub rate-limit failures are actionable and never trigger a fallback to `main`.
- TTY output is consistent with installer conventions, while non-TTY execution is deterministic and does not block for terminal input.
- Fake-driven tests cover release discovery, SemVer outcomes, architecture mapping, checksum verification, atomic replacement boundaries, existing/fresh repository acquisition, configuration-only execution, rollback, and development-build no-op behavior.
- The CLI test suite passes with `go test ./...`, and CI remains compatible with `go test -v -race -count=1 ./...`.

## Proposal decisions confirmed with the user

- The command updates both the MoonArch CLI binary and the managed dotfiles; it is not a binary-only or dotfiles-only updater by default.
- Dotfiles are pinned to the latest published SemVer release tag, never to `main`.
- Update reapplies only file-based configuration through the existing transaction and backup/rollback machinery.
- Package installation, `paru`, `hyprpm` plugin refresh, and external actions do not run during update.
- Development builds are disabled: `Version == "dev"` produces a clear no-op before any update side effect.

## Proposal question round

The locked scope above is not reopened. The following decisions should be confirmed before specification and design so implementation does not silently choose user-visible behavior or recovery policy.

| Question | Recommended baseline and implication |
| --- | --- |
| Should release acquisition be fully native Go, or may the command shell out to installer logic? | Confirm the proposal's native Go recommendation. It improves testability and scope isolation, at the cost of maintaining the release asset contract in two implementations. |
| After atomic binary replacement, should the old in-memory process finish the dotfiles/configuration stages, or should it perform one guarded re-exec into the new binary? | Prefer finishing in the current process for the first slice and state that the new version is active on the next invocation, unless compatibility analysis shows that latest-tag configuration must always be applied by latest-version code. A re-exec is safer for version alignment but requires explicit loop prevention and state handoff. |
| What are the exact version outcomes? | Recommended: lower installed version updates; equal version prints `already up to date` and still reconciles dotfiles/configuration; installed version newer than GitHub latest fails safely without downgrade or mutation. Confirm whether the newer-than-latest case should instead be allowed to reconcile dotfiles. |
| How should GitHub authentication, rate limits, and offline use behave? | Recommended: allow an optional standard token for higher limits, keep unauthenticated access supported, and fail before mutation when latest-release metadata cannot be resolved. Do not use stale metadata or an offline fallback because that weakens the meaning of “latest.” |
| What is the first-slice command UX across TTY and non-TTY execution? | Recommended: no `--only` selectors, no confirmation after the user explicitly invokes `update`, per-stage progress on a TTY, and deterministic line output without prompts when redirected. Confirm whether a TTY confirmation or selective component flags are required despite the locked update-both default. |

Assumptions used until these questions are confirmed:

- The GitHub latest-release endpoint identifies the stable release to install and retains the current asset/checksum naming contract.
- `~/.cache/dotfiles` is application-managed state whose local edits may be replaced by the force-detached release checkout.
- Linux `amd64` and `arm64` remain the only update targets in the first slice.
- The installed release uses the managed executable location established by `scripts/install.sh`; target-path and symlink handling will be made explicit in design.
