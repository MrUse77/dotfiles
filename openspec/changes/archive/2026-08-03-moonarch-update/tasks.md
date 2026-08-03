# Tasks: MoonArch Safe Release Update

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~2600-2800 (additions + deletions) |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Suggested split | Single PR with size exception |
| Delivery strategy | single-pr (requires size:exception) |
| Chain strategy | size-exception |

```text
Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High
```

**Rationale for size exception:** The change implements a complete self-update mechanism with four ordered stages (release, binary, repository, configuration), native HTTP client, SemVer validation, SHA-256 verification, atomic binary replacement, and comprehensive test coverage. The design mandates strict TDD with fake-based tests for every seam, which drives the line count. While significantly over 400 lines, the change is:
- Well-specified with clear acceptance criteria (22 criteria in spec)
- Architecturally coherent (single feature boundary)
- Test-heavy by design (safety-critical update operation)
- Not splittable without breaking the TDD contract or feature completeness

Splitting into chained PRs would fragment the TDD RED→GREEN cycles and create intermediate states where tests reference unimplemented seams. A single PR with size exception preserves the design's integrity and allows atomic review of the complete update mechanism.

---

## Dependency Graph

```text
T1 (dependencies) ─→ T2 (errors) ─→ T3 (version) ─┐
                                                     ├─→ T6 (replacer) ─→ T7 (test hook) ─┐
                           T4 (client) ─────────────┘                                      │
                           T5 (checksum) ─→ T6 (replacer)                                  │
                                                                                            ├─→ T10 (update.go) ─→ T11 (update_flow.go orchestrator)
                                                                                           │
                                                                                           ├─→ T12 (update_flow.go config builder)
                                                                                           │
                                                                                           └─→ T13 (update_output.go) ─→ T14 (verification) ─→ T15 (README) ─→ T16 (RELEASING)
```

**Critical path:** T1 → T2 → T3/T4/T5 → T6 → T7 → T10 → T11/T12/T13 → T14 → T15/T16

**Parallel opportunities:**
- T3, T4 can run in parallel after T2
- T5 can start after T2, but T6 waits for both T4 and T5
- T11, T12, T13 can run in parallel after T10
- T15, T16 can run in parallel after T14

---

## Task List

### Batch 1: Foundation (pkg/release) - Strict TDD

- [x] **T1: Add SemVer dependency and promote go-isatty** — ~10 lines
- **Files:** `cli/go.mod`, `cli/go.sum`
- **Dependencies:** none
- **Description:** Add `github.com/Masterminds/semver/v3 v3.3.1` as a direct dependency. Promote `github.com/mattn/go-isatty v0.0.22` from indirect to direct use because `update_output.go` will import it. Run `go mod tidy` to update `go.sum`.
- **Acceptance check:** `go mod verify` succeeds; `go list -m all | grep semver` shows v3.3.1; `go list -m all | grep go-isatty` shows v0.0.22 without `// indirect` comment
- **Changed lines:** ~10 (go.mod additions + go.sum updates)

- [x] **T2: Create pkg/release/errors.go with typed errors** — ~80 lines
- **Files:** `cli/pkg/release/errors.go`
- **Dependencies:** T1
- **Description:** Define typed error types for the release subsystem: `TransportError`, `RateLimitError`, `HTTPStatusError`, `MalformedReleaseError`, `InvalidVersionError` (with Subject and Value fields), `UnsupportedArchitectureError`, `AssetNotFoundError`, `ChecksumFormatError`, `ChecksumEntryMissingError`, `ChecksumEntryAmbiguousError`, `ChecksumMismatchError`, `BinaryReplacementError`. Each implements `Error() string` and supports `errors.Is`/`errors.As` unwrapping where appropriate.
- **Acceptance check:** Package compiles with `go build ./pkg/release`; error types are exported and implement the error interface; `errors.As` works for typed error assertions
- **Changed lines:** ~80 (new file)

- [x] **T3: pkg/release/version — RED → GREEN → REFACTOR** — ~170 lines (120 test + 50 impl)
- **Files:** `cli/pkg/release/version_test.go`, `cli/pkg/release/version.go`
- **Dependencies:** T1, T2
- **Description:** 
  - **RED:** Write table-driven tests for `CompareVersions` covering: leading `v` prefix on both/either/neither, semantic ordering (`v1.10.0 > v1.9.0`), equality, invalid installed version, invalid release tag, empty values, partial versions. Assert `InvalidVersionError.Subject` is "installed version" or "release tag" as appropriate.
  - **GREEN:** Implement `VersionComparison` type (InstalledOlder, InstalledEqual, InstalledNewer), `InvalidVersionError` struct, and `CompareVersions(installedRaw, latestRaw string) (VersionComparison, error)`. Strip exactly one lowercase `v` prefix, call `semver.StrictNewVersion`, compare with `LessThan`/`Equal`/`GreaterThan`.
  - **REFACTOR:** Ensure error messages are clear and test names describe scenarios.
- **Acceptance check:** `go test -v ./pkg/release -run TestCompareVersions` passes; table covers all spec scenarios (AC 4, 5); invalid versions return `InvalidVersionError` with correct Subject
- **Changed lines:** ~170 (120 test + 50 impl)

- [x] **T4: pkg/release/client — RED → GREEN → REFACTOR** — ~400 lines (250 test + 150 impl)
- **Files:** `cli/pkg/release/client_test.go`, `cli/pkg/release/client.go`
- **Dependencies:** T1, T2
- **Description:**
  - **RED:** Write tests with a fake `HTTPDoer` that captures method, URL, headers, deadline. Cover: anonymous request (no Authorization header), authenticated request (Bearer token), exact endpoint URL, valid JSON with tag_name and assets, missing tag_name (malformed), malformed JSON, transport error, 403 rate limit, 429 rate limit, other non-200 status, bounded context deadline (30s max), no fallback data. Test `BinaryAsset` for amd64/arm64/unsupported arch. Test `ChecksumAsset` for present/missing/duplicate.
  - **GREEN:** Implement `HTTPDoer` interface, `Asset` struct (Name, URL), `Release` struct (Tag, Assets), `Client` interface (Latest, Download), `GitHubClient` struct with HTTPDoer/token/timeout. `NewGitHubClient(doer, token)` constructor. `Latest(ctx)` sends GET to `https://api.github.com/repos/MrUse77/dotfiles/releases/latest` with `Accept: application/vnd.github+json`, adds Bearer header only when token non-empty, decodes JSON, validates tag_name present. `Download(ctx, asset)` returns `io.ReadCloser` from asset URL. Both derive per-request context capped at 30s. `BinaryAsset(release, goarch)` maps amd64→`moonarch-cli-linux-amd64`, arm64→`moonarch-cli-linux-arm64`, else `UnsupportedArchitectureError`. `ChecksumAsset(release)` finds exact `SHA256SUMS.txt`, errors on missing/duplicate.
  - **REFACTOR:** Ensure error types from T2 are returned; verify no shell-out or offline fallback.
- **Acceptance check:** `go test -v ./pkg/release -run TestGitHubClient` passes; fake HTTPDoer asserts zero network calls in tests (AC 3, 19, 20); unsupported arch returns error before download (AC 6)
- **Changed lines:** ~400 (250 test + 150 impl)

- [x] **T5: pkg/release/checksum — RED → GREEN → REFACTOR** — ~260 lines (180 test + 80 impl)
- **Files:** `cli/pkg/release/checksum_test.go`, `cli/pkg/release/checksum.go`
- **Dependencies:** T1, T2
- **Description:**
  - **RED:** Write tests with `bytes.Reader` fixtures. Cover: valid one-entry checksum match, missing asset entry, duplicate asset entry, malformed line (wrong hash length, non-hex, wrong separator), checksum mismatch, empty checksum list. Assert typed errors: `ChecksumFormatError`, `ChecksumEntryMissingError`, `ChecksumEntryAmbiguousError`, `ChecksumMismatchError`.
  - **GREEN:** Implement `ChecksumVerifier` interface with `Verify(assetName string, binary io.Reader, checksumList io.Reader) error`. Implement `SHA256Verifier` struct. Parser accepts GNU sha256sum format: 64-char hex, two ASCII spaces, non-empty filename. Validates every non-empty line, requires exactly one entry for asset, computes SHA-256 with `crypto/sha256`. Returns typed errors for malformed/missing/duplicate/mismatch.
  - **REFACTOR:** Ensure parser is strict (no warning-only path); verify-before-chmod contract is clear in comments.
- **Acceptance check:** `go test -v ./pkg/release -run TestSHA256Verifier` passes (AC 7); all error cases return distinct typed errors; no test uses real files
- **Changed lines:** ~260 (180 test + 80 impl)

- [x] **T6: pkg/release/replacer — RED → GREEN → REFACTOR** — ~350 lines (250 test + 100 impl)
- **Files:** `cli/pkg/release/replacer_test.go`, `cli/pkg/release/replacer.go`
- **Dependencies:** T4, T5
- **Description:**
  - **RED:** Write tests with fake `FileOps` (inject rename/chmod failures) and fake `ChecksumVerifier` (observing, failing). Use `t.TempDir()` for filesystem tests. Cover: success leaves executable `0o755`, checksum mismatch removes temp and leaves old target unchanged, chmod failure removes temp, rename failure removes temp and leaves old target, target directory creation when absent, verify-before-chmod ordering (checksum runs before 0o755), failing reader interrupts staging.
  - **GREEN:** Implement `FileOps` interface (MkdirAll, CreateTemp, Open, Chmod, Rename, Remove), `BinaryReplacer` interface with `Replace(ctx, targetPath, assetName, binary, checksumList)`, `AtomicReplacer` struct with FileOps and ChecksumVerifier. `NewAtomicReplacer(files, verifier)` constructor. Implementation: resolve target from injected home (not os.Executable), CreateTemp in target dir, copy+sync+close binary, reopen and verify checksum, chmod 0o755 only after verify, rename temp→target, cleanup temp on any failure.
  - **REFACTOR:** Ensure binary is never placed in installer plan; verify temp cleanup paths.
- **Acceptance check:** `go test -v ./pkg/release -run TestAtomicReplacer` passes (AC 7, 8); fake FileOps asserts verify-before-chmod ordering; checksum mismatch leaves old target bytes unchanged
- **Changed lines:** ~350 (250 test + 100 impl)

### Batch 2: Command Layer (cmd/) - Strict TDD

- [x] **T7: Extract BuildRepositoryRequest test hook** — ~10 lines
- **Files:** `cli/cmd/repository_acquirer.go`
- **Dependencies:** T1
- **Description:** Extract the body of `BuildRepositoryRequest()` to a package-private function `buildRepositoryRequestFromEnvironment()`. Introduce a package-level variable `var buildRepositoryRequestImpl = buildRepositoryRequestFromEnvironment`. Make `BuildRepositoryRequest()` a wrapper that calls `buildRepositoryRequestImpl()`. This allows update tests to override the hook with a counter/panic fake and assert zero invocations. Production behavior is unchanged.
- **Acceptance check:** Existing install tests still pass; `BuildRepositoryRequest()` returns the same result; no behavior change (AC 12)
- **Changed lines:** ~10 (refactoring)

- [x] **T8: cmd/update.go Cobra registration + dev guard — RED → GREEN** — ~200 lines (120 test + 80 impl)
- **Files:** `cli/cmd/update_test.go`, `cli/cmd/update.go`
- **Dependencies:** T7
- **Description:**
  - **RED:** Write tests: root command lists `update` subcommand; help text mentions CLI and dotfiles; positional argument rejected before factory invocation; unknown `--only` flag rejected before factory invocation; dev build (`Version == "dev"`) exits 0 with release-build-only message. Dev test uses counting factory and asserts zero calls to release client, replacer, acquirer, planner, executor fakes. Restore global `Version` with `t.Cleanup`.
  - **GREEN:** Implement `updateCmd` with `Use: "update"`, help text, `Args: cobra.NoArgs`, `RunE: runUpdate`. `runUpdate` calls `runUpdateWithFactory(cmd, Version, defaultUpdateDependencies)`. `runUpdateWithFactory` checks `currentVersion == "dev"` first, prints message, returns nil before creating any collaborator. `defaultUpdateDependencies` factory constructs real dependencies (implemented in T11/T12/T13). Register with `rootCmd.AddCommand(updateCmd)` in `init()`. Define no flags.
  - **Acceptance check:** `go test -v ./cmd -run TestUpdateCommand` passes (AC 1, 2, 17, 18); dev guard test asserts zero collaborator calls; positional args and unknown flags fail before factory
- **Changed lines:** ~200 (120 test + 80 impl)

- [x] **T9: cmd/update_flow.go types + orchestrator — RED → GREEN** — ~650 lines (400 test + 250 impl)
- **Files:** `cli/cmd/update_flow_test.go`, `cli/cmd/update_flow.go`
- **Dependencies:** T8
- **Description:**
  - **RED:** Write tests with dedicated fakes (release client, replacer, acquirer, planner, executor, reporter) that record event logs. Cover: installed < latest runs all stages in order; installed == latest skips binary (code="already-current") but still runs repository/configuration; installed > latest fails with no mutation; invalid installed version fails; invalid release tag fails; unsupported architecture fails before downloads; explicit repository request has fixed destination/URL/tag (not from BuildRepositoryRequest); binary failure skips repository/configuration; repository failure skips configuration; configuration failure preserves binary success; partial success reporting; no re-exec (current process completes); legacy builder not called (override `buildRepositoryRequestImpl` with panic fake, set conflicting DOTFILES_* env, assert zero calls and captured fixed request).
  - **GREEN:** Define `UpdateStage` (release, binary, repository, configuration), `StageStatus` (success, skipped, failed), `RollbackOutcome` (not-required, complete, incomplete, manual-recovery-required), `StageResult`, `UpdateResult`, `StageReporter` interface, `UpdateOrchestrator` interface, `ConfigurationPlanBuilder` interface, `ConfigurationExecutorFactory`, `StageError` (with Stage, Code, Cause, Unwrap). Implement private `updater` struct with dependencies (release.Client, BinaryReplacer, RepositoryAcquirer, ConfigurationPlanBuilder, ConfigurationExecutorFactory, home resolver, arch func). `Run(ctx, currentVersion, reporter)` implements stage flow: release (resolve, validate, compare), binary (older: select/download/verify/rename; equal: skip with already-current; newer: fail), repository (acquire with explicit request), configuration (build plan, execute via transaction). Returns ordered stage results. Binary success sets `BinaryActiveOnNextInvocation=true` before repository/configuration.
  - **REFACTOR:** Ensure error taxonomy maps to stage codes per design; verify no stage attempts coordinated rollback of prior stages.
- **Acceptance check:** `go test -v ./cmd -run TestUpdateOrchestrator` passes (AC 4, 5, 6, 8, 9, 10, 11, 12, 15); fake event logs prove stage ordering and isolation; legacy builder test asserts zero calls with conflicting env
- **Changed lines:** ~650 (400 test + 250 impl)

- [x] **T10: cmd/update_flow.go configuration plan builder — RED → GREEN** — ~150 lines (100 test + 50 impl)
- **Files:** `cli/cmd/update_flow_test.go` (additional tests), `cli/cmd/update_flow.go` (additional impl)
- **Dependencies:** T9
- **Description:**
  - **RED:** Write tests with fake catalog implementing both `PackageActions` and `ConfigurationActions`. Assert `ConfigurationActions == 1`, `PackageActions == 0`, no `ExternalActions`/plugin path requested. Captured plan contains no paru, package-manager, hyprpm, or non-configuration action. Test transaction-backed executor factory: prepare/commit success; prepare failure (no rollback needed); commit failure with rollback (complete, incomplete, manual-recovery-required outcomes).
  - **GREEN:** Implement `updateConfigurationCatalog` interface (embeds `plan.ActionCatalog` and `plan.PhaseActionCatalog`). Implement `newUpdateConfigurationPlanBuilder(discoverer, catalog)` returning `ConfigurationPlanBuilder`. `Build(repoRoot, homeDir)` creates `plan.InstallationRun` with `plan.Options{Mode: "user"}`, calls only `planner.BuildConfiguration(run, repoRoot, homeDir)`. Default executor factory: `func(p plan.InstallationPlan) PhaseExecutor { tx := transaction.New(p); return installer.NewExecutor(tx, external.NewRunner(nil).WithStdio(cmd.InOrStdin(), io.Discard, io.Discard)) }`. Map execution report rollback state to `RollbackOutcome`.
  - **REFACTOR:** Verify no call to `PackageActions`, `ExternalActions`, `ui.Run`, Bubble Tea, Huh, or plugins command.
- **Acceptance check:** `go test -v ./cmd -run TestUpdateConfiguration` passes (AC 13, 14); fake catalog asserts ConfigurationActions called once, PackageActions zero times; transaction rollback outcomes mapped correctly
- **Changed lines:** ~150 (100 test + 50 impl)

- [x] **T11: cmd/update_output.go reporter — RED → GREEN** — ~220 lines (120 test + 100 impl)
- **Files:** `cli/cmd/update_output_test.go`, `cli/cmd/update_output.go`
- **Dependencies:** T9
- **Description:**
  - **RED:** Write tests with `func(io.Writer) bool { return false }` (non-TTY) and `true` (TTY) injected. Non-TTY: lines are deterministic, ANSI-free, prompt-free, parseable key=value format with fixed key order, quoted dynamic values with control chars removed. TTY: labels expose all stages, stage transitions visible. Test failure/skip ordering stable. Compare repeated non-TTY output byte-for-byte.
  - **GREEN:** Implement `ttyDetector` func type, `isTTY(w io.Writer) bool` using `isatty.IsTerminal`/`isatty.IsCygwinTerminal`. Implement `StageReporter` interface with `Start(stage)` and `Complete(result)`. `newUpdateReporter(out, detectTTY)` returns reporter that branches on TTY. Non-TTY: emit `stage=<name> status=running`, then `stage=<name> status=<status> code=<code> detail=<detail> ...` with fixed key order, no ANSI, no prompts. TTY: emit `[<stage>] <human-label>` with readable progress. Both: no blocking prompt, no terminal-control dependency.
  - **REFACTOR:** Ensure non-TTY output is byte-stable for same inputs; verify TTY uses no Bubble Tea or Huh.
- **Acceptance check:** `go test -v ./cmd -run TestUpdateReporter` passes (AC 15, 16, 17); non-TTY test asserts no `\x1b` in output; byte-identical on repeated runs
- **Changed lines:** ~220 (120 test + 100 impl)

### Batch 3: Verification & Documentation

- [x] **T12: Full test suite and build verification** — 0 lines (verification task)
- **Files:** none (verification only)
- **Dependencies:** T9, T10, T11
- **Description:** Run the complete test suite and build from `cli/`. Execute: `go test ./...`, `go test -v -race -count=1 ./...`, `go vet ./...`, `go build ./...`. All must pass with zero failures. This is a gate task, not an implementation task.
- **Acceptance check:** All commands exit 0; no test failures; no vet warnings; build succeeds (AC 21)
- **Changed lines:** 0 (verification only)

- [x] **T13: Update README.md with moonarch update documentation** — ~40 lines
- **Files:** `README.md` (at repo root)
- **Dependencies:** T12
- **Description:** Add `moonarch-cli update` to the command list. Add a focused section covering: release-build-only availability and dev no-op; required online GitHub access and optional `GITHUB_TOKEN`; what updates (managed binary, fixed cache clone at release tag, file-based configuration); what never runs (packages, paru, Hyprland plugins/hyprpm, non-configuration installer actions); equal-version reconciliation, no re-exec, next-invocation activation; independent recovery boundaries (configuration uses transaction backup/inventory; binary/repository require restoring previous verified release/tag independently).
- **Acceptance check:** README mentions `moonarch update`; documents release-only restriction; lists component boundaries; explains recovery paths (AC 22)
- **Changed lines:** ~40 (documentation)

- [x] **T14: Update RELEASING.md with asset/checksum contract** — ~30 lines
- **Files:** `RELEASING.md` (at repo root)
- **Dependencies:** T12
- **Description:** Document the compatibility contract that releases must retain for `moonarch update`: stable SemVer tags, both Linux architecture asset names (`moonarch-cli-linux-amd64`, `moonarch-cli-linux-arm64`), canonical `SHA256SUMS.txt` with one entry per asset in GNU sha256sum format (64-char hex, two spaces, filename). This makes future release-pipeline edits consciously preserve update compatibility.
- **Acceptance check:** RELEASING.md documents asset naming contract; documents SHA256SUMS.txt format; warns that breaking this contract breaks `moonarch update` (AC 22)
- **Changed lines:** ~30 (documentation)

---

## Batch Grouping for sdd-apply

### Batch 1: Foundation (pkg/release)

**Tasks:** T1, T2, T3, T4, T5, T6  
**Estimated lines:** ~1270 additions  
**Rationale:** These establish the release subsystem with strict TDD. Each task is a complete RED→GREEN→REFACTOR cycle with tests and implementation. They can be reviewed as a cohesive "release protocol" unit. T3/T4/T5 can run in parallel after T2. T6 depends on T4 and T5.

### Batch 2: Command Layer (cmd/)

**Tasks:** T7, T8, T9, T10, T11  
**Estimated lines:** ~1010 additions  
**Rationale:** These wire the Cobra command and orchestration with strict TDD. T7 is a small refactoring for testability. T8 establishes the command registration and dev guard. T9/T10/T11 implement the orchestrator, configuration builder, and reporter. T9/T10/T11 can run in parallel after T8.

### Batch 3: Verification & Documentation

**Tasks:** T12, T13, T14  
**Estimated lines:** ~70 additions (docs only)  
**Rationale:** T12 is the final verification gate. T13/T14 document the feature. They should come last to ensure all tests pass before documentation.

---

## Execution Notes

**Strict TDD considerations:**
- Every implementation task follows RED → GREEN → REFACTOR
- Tests land with or just before their implementation
- Fake-based tests prove seam isolation and call counts
- No real network, filesystem, or GitHub calls in tests
- `t.TempDir()` for all filesystem tests
- Table-driven tests for multiple scenarios

**Testing strategy:**
- Unit tests: every pkg/release type has focused tests
- Integration tests: cmd/ tests use fakes for all collaborators
- Fake-based assertions: zero-call assertions for dev guard, package exclusion, legacy builder bypass
- Deterministic output: non-TTY tests assert byte-identical output
- Race detection: `go test -race` in verification task

**Rollback path:**
- Single Git revert removes the update command, pkg/release, tests, and documentation
- No data migration or persistent state conversion required
- RepositoryAcquirer test hook (T7) reverts cleanly; production behavior unchanged

**Risk mitigation:**
- Dev guard precedes all collaborator creation; counting-factory test proves it
- Explicit repository request bypasses BuildRepositoryRequest; test hook makes absence observable
- Configuration-only plan builder; fake catalog asserts zero package/hyprpm calls
- Binary outside transaction; configuration rollback cannot restore prior executable
- Non-TTY output is deterministic; no ANSI, no prompts, no terminal-control dependency

**Work-unit commits:**
- Each task is a reviewable work unit with clear start/finish/verification
- Tests and implementation land in the same commit (TDD contract)
- Documentation lands with the feature (T13/T14 after T12)
- Each commit tells a story: foundation → command → verification → docs
