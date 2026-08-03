# Update Command Specification

## Purpose

Add a release-only `moonarch update` command that resolves the latest published SemVer release, replaces the managed binary only after SHA-256 verification, reconciles the dotfiles cache to the same release tag, and reapplies only file-based configuration through the existing transaction machinery. Development builds refuse to run. No component of the command touches packages, Hyprland plugins, or external installer actions.

---

## Functional Requirements

### Requirement: Command registration

The system MUST register a Cobra `update` subcommand on the root command. The command MUST accept no positional arguments and no flags in the first slice.

#### Scenario: Command is discoverable

- GIVEN the CLI is built as a release binary
- WHEN the user runs `moonarch --help`
- THEN the `update` subcommand is listed in the available commands
- AND the command usage text states it updates the CLI and managed dotfiles to the latest release

#### Scenario: Positional arguments rejected

- GIVEN the CLI is built as a release binary
- WHEN the user runs `moonarch update extra-arg`
- THEN the command exits with a non-zero status
- AND no side effects (no network, no filesystem mutation) occur

---

### Requirement: Development-build guard

The command MUST reject `cmd.Version == "dev"` before any network call, filesystem probe, repository operation, or installer interaction. The rejection MUST be observable via fake-based tests that assert zero calls to every injected collaborator.

#### Scenario: Dev build exits cleanly with clear message

- GIVEN `cmd.Version` equals `"dev"`
- WHEN the user runs `moonarch update`
- THEN the command prints a message stating that updates are available only from release builds
- AND the command exits with status 0
- AND no HTTP client, release client, filesystem replacement, repository acquirer, or installer executor call is made
- AND a fake-based test with every collaborator stubbed asserts zero invocations

#### Scenario: Dev build guard runs before network availability check

- GIVEN `cmd.Version` equals `"dev"`
- AND the machine has no network connectivity
- WHEN the user runs `moonarch update`
- THEN the command exits successfully with the release-build message
- AND no network error is reported

---

### Requirement: Release resolution

The system MUST resolve the latest published release through a native Go client (no shell-out) that queries `https://api.github.com/repos/MrUse77/dotfiles/releases/latest`, parses the JSON response, and extracts `tag_name`. The client MUST support an optional bearer token from the `GITHUB_TOKEN` environment variable.

#### Scenario: Anonymous resolution succeeds

- GIVEN `GITHUB_TOKEN` is not set
- AND GitHub returns a 200 response with a valid `tag_name` `"v1.2.3"`
- WHEN the release client resolves the latest release
- THEN the resolved tag is `"v1.2.3"`
- AND the outgoing request contains no `Authorization` header

#### Scenario: Authenticated resolution succeeds

- GIVEN `GITHUB_TOKEN` is set to `"ghp_example"`
- WHEN the release client resolves the latest release
- THEN the outgoing request contains `Authorization: Bearer ghp_example`
- AND the resolved tag matches `tag_name`

#### Scenario: Missing tag_name field

- GIVEN GitHub returns a 200 response whose body has no `tag_name` field
- WHEN the release client resolves the latest release
- THEN the client returns an error describing a malformed release response
- AND no update mutation is attempted

#### Scenario: HTTP transport error

- GIVEN the HTTP client returns a transport error (timeout, DNS failure, TLS failure)
- WHEN the release client resolves the latest release
- THEN the client returns an error identifying a transport failure
- AND no update mutation is attempted

#### Scenario: GitHub returns non-200 status

- GIVEN GitHub returns 403 (rate limit) or 500 (server error)
- WHEN the release client resolves the latest release
- THEN the client returns an error describing the HTTP status
- AND no update mutation is attempted

#### Scenario: HTTP request is bounded

- GIVEN the release client is configured
- WHEN it performs a request
- THEN the request has a bounded timeout (at most 30 seconds per request)

#### Scenario: No offline fallback

- GIVEN the release client cannot reach GitHub
- WHEN resolution is attempted
- THEN no cached, stale, or fallback release metadata is used
- AND no mutation occurs

---

### Requirement: SemVer validation and comparison

The system MUST validate both the installed `cmd.Version` and the resolved release `tag_name` as SemVer values. A leading `v` prefix MUST be accepted and stripped before parsing. Comparison MUST be semantic, not lexical.

#### Scenario: Valid versions with leading v

- GIVEN `cmd.Version` is `"v1.0.0"` and the release tag is `"v1.1.0"`
- WHEN versions are compared
- THEN the installed version is reported as older than the latest
- AND the update proceeds

#### Scenario: Valid versions without leading v

- GIVEN `cmd.Version` is `"2.0.0"` and the release tag is `"v2.0.0"`
- WHEN versions are compared
- THEN the versions are reported as equal
- AND binary replacement is skipped

#### Scenario: Installed newer than latest

- GIVEN `cmd.Version` is `"v2.0.0"` and the release tag is `"v1.9.0"`
- WHEN versions are compared
- THEN the command fails with a clear error stating the installed version is newer than the latest release
- AND no mutation of any kind occurs (no binary replacement, no repository update, no configuration reapplication)

#### Scenario: Installed version is not valid SemVer

- GIVEN `cmd.Version` is `"dev"` (already guarded), or a non-SemVer release string such as `"latest"` or `"abc"`
- WHEN the installed version is validated
- THEN the command fails with an error identifying the invalid installed version
- AND no mutation occurs

#### Scenario: Release tag is not valid SemVer

- GIVEN the resolved `tag_name` is `"release-2024"`
- WHEN the tag is validated
- THEN the command fails with an error identifying the invalid release tag
- AND no mutation occurs

---

### Requirement: Version-outcome routing

The command MUST route execution based on the semantic comparison of installed and latest versions.

#### Scenario: Installed older than latest

- GIVEN installed `v1.0.0` and latest `v1.1.0`
- WHEN routing
- THEN the binary replacement, dotfiles reconciliation, and configuration reapplication stages run in order

#### Scenario: Installed equals latest

- GIVEN installed `v1.1.0` and latest `v1.1.0`
- WHEN routing
- THEN the binary stage reports "already up to date" and skips download and replacement
- AND the dotfiles stage still acquires the repository at tag `v1.1.0`
- AND the configuration stage still reapplies `ConfigurationActions()`
- AND no binary asset is requested from GitHub

#### Scenario: Installed newer than latest

- GIVEN installed `v2.0.0` and latest `v1.9.0`
- WHEN routing
- THEN the command fails with a clear error
- AND no stage performs mutation
- AND no HTTP call beyond release resolution is made

---

### Requirement: Asset selection and download

The system MUST select the architecture-specific release asset using `runtime.GOARCH` mapping (`"amd64"` → `amd64`, `"arm64"` → `arm64`) and the asset name `moonarch-cli-linux-${arch}`. Any other `GOARCH` value MUST fail before any download.

#### Scenario: amd64 host selects amd64 asset

- GIVEN `runtime.GOARCH` is `"amd64"`
- WHEN the asset is selected
- THEN the requested asset name is `"moonarch-cli-linux-amd64"`

#### Scenario: arm64 host selects arm64 asset

- GIVEN `runtime.GOARCH` is `"arm64"`
- WHEN the asset is selected
- THEN the requested asset name is `"moonarch-cli-linux-arm64"`

#### Scenario: Unsupported architecture

- GIVEN `runtime.GOARCH` is `"386"` or any value other than `"amd64"` or `"arm64"`
- WHEN asset selection is attempted
- THEN the command fails with an error stating the architecture is not supported
- AND no download, replacement, repository, or configuration mutation occurs

#### Scenario: Download to temp file in target directory

- GIVEN the asset is selected for a host
- WHEN the binary is downloaded
- THEN the destination is a temporary file in `$HOME/.local/bin/` (the same directory as the managed executable)
- AND the temp file path is distinct from the final `$HOME/.local/bin/moonarch-cli` path

#### Scenario: Asset not present in release

- GIVEN the release JSON does not contain an asset matching the selected name
- WHEN the download is attempted
- THEN the command fails with an error identifying the missing asset
- AND the existing managed binary remains untouched

---

### Requirement: SHA-256 verification

The system MUST download `SHA256SUMS.txt` from the same release, parse it, select the entry matching the architecture asset filename, and verify the downloaded binary's SHA-256 against that entry. Any parse or match failure MUST be a hard error.

#### Scenario: Checksum matches

- GIVEN `SHA256SUMS.txt` contains exactly one entry `"abcd1234...  moonarch-cli-linux-amd64"`
- AND the downloaded binary's SHA-256 is `abcd1234...`
- WHEN verification runs
- THEN verification succeeds
- AND the binary replacement may proceed

#### Scenario: Checksum mismatch

- GIVEN `SHA256SUMS.txt` contains an entry for the asset with a different hash
- WHEN verification runs
- THEN the command fails with an error identifying the checksum mismatch
- AND the existing managed binary remains untouched
- AND the temp file is removed

#### Scenario: Missing SHA256SUMS.txt

- GIVEN the release does not provide `SHA256SUMS.txt` (HTTP 404)
- WHEN verification is attempted
- THEN the command fails with an error identifying the missing checksum list
- AND the existing managed binary remains untouched

#### Scenario: SHA256SUMS.txt has no entry for the asset

- GIVEN `SHA256SUMS.txt` is valid but contains no line whose filename matches the selected asset
- WHEN verification runs
- THEN the command fails with an error identifying the missing asset entry
- AND the existing managed binary remains untouched

#### Scenario: SHA256SUMS.txt has duplicate entries for the asset

- GIVEN `SHA256SUMS.txt` contains two or more lines whose filename matches the selected asset
- WHEN verification runs
- THEN the command fails with an error identifying the ambiguous entry
- AND the existing managed binary remains untouched

#### Scenario: SHA256SUMS.txt line is malformed

- GIVEN `SHA256SUMS.txt` contains a line for the asset with a non-hex hash or with unexpected separator format
- WHEN verification runs
- THEN the command fails with an error identifying the malformed entry
- AND the existing managed binary remains untouched

#### Scenario: Checksum is verified before executable permissions are applied

- GIVEN the binary is downloaded to the temp file
- WHEN verification is performed
- THEN the SHA-256 is verified before any executable permission (`0o755`) is set on the temp file
- AND the atomic rename is performed only after successful verification

---

### Requirement: Atomic binary replacement

The system MUST atomically replace `$HOME/.local/bin/moonarch-cli` with the verified binary using a same-filesystem rename. The replacement MUST preserve executable permissions (`0o755`). Any failure MUST leave the previous binary intact.

#### Scenario: Successful atomic replace

- GIVEN the verified binary is staged at a temp file in `$HOME/.local/bin/`
- WHEN replacement runs
- THEN `$HOME/.local/bin/moonarch-cli` is replaced by an atomic `os.Rename` from the temp file
- AND the resulting binary is executable
- AND the temp file no longer exists

#### Scenario: Replace preserves permissions

- GIVEN the replacement succeeds
- WHEN the new binary is inspected
- THEN its mode includes the executable bit (`0o755` owner-execute at minimum)

#### Scenario: Rename failure leaves previous binary intact

- GIVEN the rename fails (permissions, cross-device, target busy)
- WHEN replacement is attempted
- THEN the previous `$HOME/.local/bin/moonarch-cli` remains unchanged
- AND the command fails with an error identifying the replacement failure
- AND the temp file is cleaned up

#### Scenario: Missing target directory is created

- GIVEN `$HOME/.local/bin/` does not exist
- WHEN replacement is attempted
- THEN the directory is created with mode `0o755`
- AND the binary is installed into it

#### Scenario: Binary replacement is outside the configuration transaction

- GIVEN the binary has been replaced
- WHEN the later configuration stage fails
- THEN the configuration transaction rollback does NOT restore the previous binary
- AND recovery guidance references reinstalling the previous release asset independently

---

### Requirement: Process handoff

After binary replacement, the current process MUST finish dotfiles reconciliation and configuration reapplication using the current in-memory code. The command MUST report that the replaced binary takes effect on the next invocation. The command MUST NOT re-exec or restart itself.

#### Scenario: Current process completes remaining stages

- GIVEN the binary has been replaced successfully
- WHEN dotfiles and configuration stages run
- THEN they run in the same process without re-exec
- AND the command reports that the new binary is active on the next invocation

#### Scenario: No re-exec

- GIVEN the update command has replaced the binary
- WHEN the command completes
- THEN no `syscall.Exec`, `os.Exec`, or child-process invocation of the new binary occurs

---

### Requirement: Dotfiles reconciliation

The system MUST acquire the dotfiles repository at the resolved release tag into `$HOME/.cache/dotfiles`, using an explicit `RepositoryRequest` (destination `$HOME/.cache/dotfiles`, URL `https://github.com/MrUse77/dotfiles.git`, ref = validated release tag). It MUST NOT call `BuildRepositoryRequest()` and therefore MUST NOT inherit the `"dev"` → `"main"` fallback, `DOTFILES_DIR`, `DOTFILES_REPO`, or `DOTFILES_BRANCH` overrides.

#### Scenario: Fresh clone when destination is absent

- GIVEN `$HOME/.cache/dotfiles` does not exist
- WHEN dotfiles acquisition runs with resolved tag `"v1.2.3"`
- THEN the acquirer clones `https://github.com/MrUse77/dotfiles.git` into `$HOME/.cache/dotfiles`
- AND the resulting HEAD is a detached checkout at tag `v1.2.3`
- AND submodules are initialized recursively

#### Scenario: Existing clone is updated in place

- GIVEN `$HOME/.cache/dotfiles` is an existing Git clone
- WHEN dotfiles acquisition runs with resolved tag `"v1.2.3"`
- THEN the acquirer fetches from the configured remote
- AND performs a force detached checkout of `FETCH_HEAD` (or the matching tag ref) at `v1.2.3`
- AND updates submodules recursively

#### Scenario: Update command request is independent of BuildRepositoryRequest

- GIVEN the update command runs
- WHEN the repository request is constructed
- THEN the destination is exactly `$HOME/.cache/dotfiles` (not `DOTFILES_DIR`)
- AND the URL is exactly `https://github.com/MrUse77/dotfiles.git` (not `DOTFILES_REPO`)
- AND the ref is the validated release tag (never `"main"`)
- AND a fake-based test asserts `BuildRepositoryRequest()` is never called

#### Scenario: Repository acquisition failure is reported

- GIVEN Git fetch or checkout fails
- WHEN dotfiles acquisition runs
- THEN the command reports the repository stage as failed
- AND the error identifies the acquisition failure
- AND the command exits with non-zero status

---

### Requirement: Configuration-only reapplication

The system MUST build the executor plan exclusively from `ConfigurationActions()` of the installer `ActionCatalog` and run it through the existing `transaction.New()` prepare/commit/rollback lifecycle. No other action set MUST be requested.

#### Scenario: Configuration plan uses only ConfigurationActions

- GIVEN the dotfiles are reconciled at the release tag
- WHEN the configuration executor plan is built
- THEN `ConfigurationActions(repoRoot, homeDir, opts, managedTargets)` is called
- AND `PackageActions` is never requested
- AND no external installer action set beyond `ConfigurationActions()` is requested
- AND a fake-based test asserts packages, `hyprpm`, and other non-configuration actions are never invoked

#### Scenario: Transaction prepare and commit succeed

- GIVEN configuration preparation succeeds for all actions
- WHEN commit runs
- THEN all file operations are applied
- AND the configuration stage reports success

#### Scenario: Transaction prepare fails and rolls back

- GIVEN one or more configuration actions fail during preparation
- WHEN preparation fails
- THEN the transaction rolls back already-prepared changes
- AND the command reports the configuration stage as failed and that rollback was applied

#### Scenario: Transaction commit fails and rolls back

- GIVEN preparation succeeded
- AND commit fails
- WHEN commit fails
- THEN the transaction rolls back
- AND the command reports the configuration stage as failed and whether rollback succeeded

#### Scenario: Packages are never invoked

- GIVEN the update command runs end-to-end
- WHEN execution completes
- THEN no `paru`, AUR, or package-manager invocation has occurred
- AND a fake-based test confirms zero package actions were requested

#### Scenario: Hyprpm is never invoked

- GIVEN the update command runs end-to-end
- WHEN execution completes
- THEN no `hyprpm` invocation has occurred
- AND a fake-based test confirms zero plugin refresh actions were requested

---

### Requirement: Per-stage result reporting

The command MUST report each stage (release, binary, repository, configuration) with a distinct outcome: success, skipped, or failed. Partial success MUST be visible.

#### Scenario: Full success output

- GIVEN all stages succeed and the binary was replaced
- WHEN the command completes
- THEN the output reports release resolved (with the new tag), binary replaced, dotfiles reconciled at the tag, and configuration reapplied
- AND the output notes the new binary is active on next invocation
- AND the command exits with status 0

#### Scenario: Already up to date output

- GIVEN installed equals latest
- WHEN the command completes
- THEN the output reports the binary is already up to date at that version
- AND dotfiles reconciled at the tag
- AND configuration reapplied
- AND the command exits with status 0

#### Scenario: Binary replaced but configuration failed

- GIVEN binary replacement succeeded
- AND configuration failed with rollback
- WHEN the command completes
- THEN the output reports binary replaced, dotfiles stage result, and configuration failed with rollback outcome
- AND the command exits with non-zero status

#### Scenario: Release resolution fails

- GIVEN release resolution fails
- WHEN the command runs
- THEN the output reports the release stage as failed with the error
- AND no further stages run
- AND the command exits with non-zero status

---

### Requirement: Terminal behavior

The command MUST detect TTY capability. On a TTY, per-stage progress output MAY use structured formatting (e.g., status lines with progress indicators). On a non-TTY, output MUST be deterministic, line-oriented, and free of terminal control sequences or blocking prompts.

#### Scenario: Non-TTY output is line-oriented

- GIVEN stdout is redirected to a file
- WHEN the command runs
- THEN every progress line is a complete, deterministic, parseable line
- AND no ANSI escape sequences appear in the output
- AND no blocking prompt is emitted

#### Scenario: TTY output identifies stages

- GIVEN stdout is a terminal
- WHEN the command runs
- THEN the output identifies each stage (release, binary, repository, configuration)
- AND stage transitions are visible

#### Scenario: No confirmation prompt

- GIVEN the command is invoked
- WHEN it runs on either TTY or non-TTY
- THEN no confirmation prompt is emitted
- AND the command proceeds without interactive approval

#### Scenario: No --only flags

- GIVEN the command is invoked
- WHEN the user tries `moonarch update --only binary` or similar
- THEN the command rejects the unknown flag
- AND no partial update runs

---

## Acceptance Criteria

The following MUST be verifiable by `sdd-verify`:

1. **`moonarch update` registered**: the root command lists `update` as a subcommand with usage text.
2. **Dev guard blocks all collaborators**: a fake-based test with `cmd.Version == "dev"` asserts zero calls to the release client, binary replacer, repository acquirer, and installer executor.
3. **Release resolution native**: no `os/exec` call to curl, wget, or scripts; all HTTP is performed through a Go `net/http`-backed client that is injectable.
4. **SemVer validation**: installed and release versions are compared semantically; leading `v` is accepted; invalid values fail with clear errors before mutation.
5. **Version-outcome routing**: installed < latest → update; equal → "already up to date" for binary but still reconciles dotfiles and config; installed > latest → hard error with no mutation.
6. **Asset selection**: `moonarch-cli-linux-amd64` or `moonarch-cli-linux-arm64` only; unsupported GOARCH fails before download.
7. **SHA-256 verified**: checksum matches the single `SHA256SUMS.txt` entry for the asset; missing, duplicate, malformed, or mismatched entries fail hard.
8. **Atomic replacement**: binary is staged in the target directory, renamed atomically, executable permissions preserved; existing binary untouched on any failure.
9. **Binary outside transaction**: configuration rollback does NOT restore the previous binary; recovery guidance is documented.
10. **No re-exec**: after replacement, the current process completes remaining stages; no self-restart.
11. **Dotfiles pinned to release tag**: `$HOME/.cache/dotfiles` is reconciled at the validated release tag, not `main`.
12. **`BuildRepositoryRequest()` is never called by update**: fake-based test asserts zero invocations; explicit `RepositoryRequest` with fixed destination, URL, and tag is used.
13. **Configuration-only plan**: fake-based test asserts only `ConfigurationActions()` is called; zero calls to `PackageActions()`, `hyprpm`, or external action sets.
14. **Transaction lifecycle**: prepare/commit/rollback run through existing `transaction.New()`; rollback outcome is reported on failure.
15. **Per-stage reporting**: release, binary, repository, and configuration outcomes are reported independently, including partial failure.
16. **Non-TTY determinism**: redirected output contains no ANSI escapes, no blocking prompts, and is line-oriented.
17. **No confirmation prompt**: neither TTY nor non-TTY execution asks for confirmation.
18. **No `--only` flags**: component-selecting flags are rejected.
19. **GitHub auth optional**: `GITHUB_TOKEN` is used when set; absence is supported; rate-limit and transport errors fail before mutation.
20. **No offline fallback**: when GitHub is unreachable, no cached or stale release is used.
21. **Tests pass**: `go test ./...` and `go test -v -race -count=1 ./...` succeed.
22. **Docs updated**: `moonarch update` is documented with release-only availability, component boundaries, output, and recovery path.

---

## Dependencies

### Runtime Dependencies

- **GitHub HTTPS API**: required for release resolution and asset download. A reachable network is required; no offline fallback is supported.
- **`GITHUB_TOKEN`** (optional): when set, sent as `Authorization: Bearer <token>` for higher rate limits.

### Build Dependencies

- **`net/http`**: for the release client.
- **SemVer library**: the design phase selects the library; the spec requires only semantic comparison with leading-`v` support.
- **`crypto/sha256`**: for checksum verification.

### Internal Dependencies

- **`cmd.RepositoryAcquirer`**: reused with an explicit `RepositoryRequest`.
- **`installer.ActionCatalog.ConfigurationActions`**: sole source of configuration actions.
- **`installer/transaction.New()`**: existing prepare/commit/rollback lifecycle.
- **`cmd.Version`**: injected at build time via `-ldflags`; `"dev"` is the guard value.

---

## Non-functional Requirements

### Requirement: Failure isolation

Every stage failure MUST stop further mutation of that stage and subsequent stages. Earlier successful stages (e.g., binary replacement) are not rolled back by later failures; their outcome is reported and recovery is documented.

#### Scenario: Release failure blocks everything

- GIVEN release resolution fails
- WHEN the command runs
- THEN no filesystem, repository, or configuration mutation occurs

#### Scenario: Configuration failure does not roll back binary

- GIVEN binary replacement succeeded
- AND configuration fails during commit
- WHEN the command completes
- THEN the replaced binary remains
- AND the command reports the configuration failure and its rollback outcome
- AND the output directs the user to the recovery procedure for the binary

---

### Requirement: Testability

All collaborators (release client, binary replacer, repository acquirer, configuration executor, version source) MUST be injectable so fake-based tests can assert call counts, argument values, and error propagation without real network or filesystem side effects.

#### Scenario: Fake-based dev guard test

- GIVEN a test harness with fakes for every collaborator
- WHEN `Version == "dev"` is exercised
- THEN the test passes only if every fake reports zero invocations

#### Scenario: Fake-based package exclusion test

- GIVEN a test harness with a fake `PhaseActionCatalog`
- WHEN the update flow runs to completion
- THEN the fake asserts `PackageActions` was never requested
- AND any Hyprland/external action method was never requested

---

### Requirement: Determinism

Given the same installed version, resolved release, host architecture, and collaborator responses, the command MUST produce the same stage outcomes and the same non-TTY output.

#### Scenario: Identical runs produce identical non-TTY output

- GIVEN two non-TTY runs with identical inputs and fake collaborator responses
- WHEN both runs complete
- THEN their non-TTY outputs are byte-identical

---

## Out of Scope

The following are explicitly out of scope for this change:

- Installing or upgrading system packages (`paru`, AUR, or any package manager operation).
- Refreshing Hyprland plugins via `hyprpm`.
- Running catalog external actions or any non-file configuration action set.
- Updating development builds; using `main` as an update source.
- Adding prerelease, nightly, branch, arbitrary-ref, or downgrade channels.
- Background update checks, scheduled updates, or automatic invocation.
- Adding release artifacts for operating systems or architectures not already published.
- Refactoring `scripts/install.sh` to share logic with the Go updater.
- Enrolling binary replacement or repository acquisition in the configuration transaction.
- Guaranteeing a single coordinated rollback across binary, repository, and configuration state.
- Any `--only` component selectors, `--dry-run`, or confirmation prompts.
- Re-exec into the new binary; the replaced binary becomes active on the next invocation.
- Reading or honoring `DOTFILES_DIR`, `DOTFILES_REPO`, `DOTFILES_BRANCH` for the update dotfiles destination.
