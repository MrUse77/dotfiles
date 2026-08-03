# Design: Safe MoonArch Release Update

## Decision summary

`moonarch update` will be a thin Cobra command over a native Go release client and a command-layer update orchestrator. It will never invoke `scripts/install.sh`, `curl`, `wget`, a shell, or a replacement process. The current process finishes repository and configuration work after replacing the executable; the replacement is reported as active on the next invocation.

The command has four ordered stages: **release**, **binary**, **repository**, and **configuration**. Every completed invocation produces an ordered result for all four stages. A failure marks later stages as skipped, preserves earlier successes, returns an error to Cobra, and therefore uses the existing process-level exit behavior (`0` for `RunE == nil`, `1` for a returned error).

The development-build guard is outside the dependency factory. When `cmd.Version == "dev"`, the command prints its release-build-only message and returns `nil` before creating a reporter or any release, filesystem, repository, planner, executor, or HTTP collaborator.

## Scope guardrails

| Concern | Fixed design decision |
| --- | --- |
| Release discovery | Native Go `net/http` client against `https://api.github.com/repos/MrUse77/dotfiles/releases/latest`; no offline or cached fallback. |
| Authentication | Read `GITHUB_TOKEN` only after the dev guard. If non-empty, send `Authorization: Bearer <token>`; never render the token. |
| Version outcomes | Installed `<` latest replaces the binary then reconciles repository/configuration; `==` skips only binary download/replacement and still reconciles; `>` fails before any mutation. |
| Repository ref | Build an explicit fixed `RepositoryRequest` from the validated release tag. Do not call `BuildRepositoryRequest()` and do not honor `DOTFILES_DIR`, `DOTFILES_REPO`, or `DOTFILES_BRANCH`. |
| Configuration scope | Build only `plan.Planner.BuildConfiguration`, backed by `installer.NewActionCatalog().ConfigurationActions`; never call package, full external, plugin, or interactive UI paths. |
| Rollback boundary | Binary replacement and repository acquisition remain outside the configuration transaction. Only configuration managed targets use `transaction.New()` and its prepare/commit/rollback lifecycle. |
| Process handoff | No `syscall.Exec`, child-process restart, or re-exec. A successful binary stage says the new executable is active on the next invocation. |
| Command surface | `update` has `cobra.NoArgs` and defines no flags, including no `--only` or confirmation path. |

## Evidence from the current implementation

The design preserves the repository's current seams instead of adding parallel installer behavior:

- `cmd.Version` is already injected by release builds with `-ldflags`; `.github/workflows/release.yml` injects the Git tag into `github.com/MrUse77/dots-cli/cmd.Version`.
- `scripts/install.sh` installs exactly to `$HOME/.local/bin/moonarch-cli`, publishes `moonarch-cli-linux-amd64` / `moonarch-cli-linux-arm64`, and reads `SHA256SUMS.txt`. The release workflow creates the same two asset names and checksum file. The Go updater adopts this contract without calling the script.
- `RepositoryAcquirer.Acquire` already accepts a complete `RepositoryRequest` containing `Destination`, `Ref`, and `URL`. Its existing-clone route fetches the requested ref, force-detaches `FETCH_HEAD`, and recursively updates submodules; its fresh route clones `--recurse-submodules --branch <ref>`.
- `BuildRepositoryRequest()` deliberately maps a dev build to `main` and honors `DOTFILES_*` overrides. That behavior is correct for install but unsafe for update, so update must bypass it entirely.
- `plan.Planner.BuildConfiguration` already calls only `PhaseActionCatalog.ConfigurationActions` after discovering managed targets. `transaction.New(plan)` plus `installer.NewExecutor` already performs prepare, commit, and rollback for those targets.
- `newInstallPlanner()` is deliberately **not** reusable for update: it calls `installer.DetectPowerProfiles()` and `installer.DetectParu()`, and its populated catalog can add power-profile system actions. `DetectPowerProfiles()` shells out to `systemctl`; update must not probe or plan that behavior.
- There is no existing TTY helper in `cli/`. `github.com/mattn/go-isatty v0.0.22` is already present indirectly through the Bubble Tea dependency graph.

## Package and file layout

The update orchestration stays in `cmd` because it composes the command-owned `RepositoryAcquirer` seam with installer planning. A separate `pkg/update` would either duplicate those command seams or create an unnecessary dependency direction. Reusable release protocol and filesystem integrity logic live under `pkg/release`.

### New files

```text
cli/
├── cmd/
│   ├── update.go               # Cobra registration, dev guard, default factories
│   ├── update_flow.go          # Stage types, dependencies, updater orchestration, config-plan builder
│   ├── update_output.go        # TTY detection and stage reporters
│   ├── update_test.go          # Cobra registration, argument rejection, dev guard
│   ├── update_flow_test.go     # Routing, seams, stage ordering, failure isolation
│   └── update_output_test.go   # TTY/non-TTY rendering and deterministic output
└── pkg/release/
    ├── client.go               # GitHub latest-release client and asset download
    ├── errors.go               # Typed release, checksum, and replacement errors
    ├── version.go              # Strict SemVer normalization and comparison
    ├── checksum.go             # SHA256SUMS parser and SHA-256 verifier
    ├── replacer.go             # Same-directory staged atomic binary replacement
    ├── client_test.go
    ├── version_test.go
    ├── checksum_test.go
    └── replacer_test.go
```

### Existing files changed

| File | Change |
| --- | --- |
| `cli/go.mod`, `cli/go.sum` | Add the direct SemVer dependency and promote `go-isatty` to direct use because the new code imports it. |
| `cli/cmd/repository_acquirer.go` | No `RepositoryAcquirer` or `RepositoryRequest` interface change. Extract the existing environment-backed request body behind the package-private test hook described below so the negative `BuildRepositoryRequest()` call is observable; production behavior remains unchanged. |
| `README.md` | Document the command, release-only restriction, component boundaries, output, partial-state recovery, and next-invocation activation. |
| `RELEASING.md` | Document the asset/checksum contract that releases must retain for `moonarch update`. |

`cmd/root.go` remains unchanged: as with `version.go` and `install.go`, `update.go` registers itself with `rootCmd.AddCommand(updateCmd)` in `init()`. `scripts/install.sh` and `.github/workflows/release.yml` need no behavior change because their target path, asset names, release tag injection, and checksum artifact already match this design.

## SemVer decision

### Chosen dependency

Add the direct dependency:

```text
github.com/Masterminds/semver/v3 v3.3.1
```

The CLI already carries direct dependencies on Cobra, Bubble Tea, Huh, and `x/sys`; a small, maintained SemVer library is a proportionate dependency for release safety. It avoids a home-grown parser accidentally accepting partial versions, mishandling prereleases/build metadata, or comparing strings lexically. The project has no existing SemVer utility. `go-isatty` is already in the module graph, so using it does not introduce a second terminal-detection implementation.

### Comparison contract

`cli/pkg/release/version.go` owns semantic normalization and comparison:

```go
type VersionComparison uint8

const (
    InstalledOlder VersionComparison = iota
    InstalledEqual
    InstalledNewer
)

type InvalidVersionError struct {
    Subject string // "installed version" or "release tag"
    Value   string
    Cause   error
}

func CompareVersions(installedRaw, latestRaw string) (VersionComparison, error)
```

`CompareVersions` removes exactly one leading lowercase `v` from each input, then calls `semver.StrictNewVersion` on the remainder. It retains the original release tag for GitHub asset selection, display, and repository checkout; normalization is used only for validation and comparison. Empty values, a bare `v`, extra prefixes, partial versions, or malformed SemVer return `InvalidVersionError` before any mutation. `LessThan`, `Equal`, and `GreaterThan` provide the routing result, so `v1.10.0` correctly sorts after `v1.9.0`.

## Release client and integrity contracts

### Release client

`cli/pkg/release/client.go` defines the injectable HTTP boundary:

```go
type HTTPDoer interface {
    Do(*http.Request) (*http.Response, error)
}

type Asset struct {
    Name string
    URL  string // GitHub browser_download_url
}

type Release struct {
    Tag    string // JSON tag_name, retained verbatim after validation
    Assets []Asset
}

type Client interface {
    Latest(ctx context.Context) (Release, error)
    Download(ctx context.Context, asset Asset) (io.ReadCloser, error)
}

type GitHubClient struct { /* HTTPDoer, token, request timeout */ }

func NewGitHubClient(doer HTTPDoer, token string) *GitHubClient
func (c *GitHubClient) Latest(ctx context.Context) (Release, error)
func (c *GitHubClient) Download(ctx context.Context, asset Asset) (io.ReadCloser, error)
```

The default factory supplies an `http.Client` with a 30-second timeout. `GitHubClient` additionally derives a per-request context capped at 30 seconds, so an injected parent deadline can only shorten the request. `Latest` sends `Accept: application/vnd.github+json`, adds the bearer header only when a token is present, decodes `tag_name` and assets, and rejects missing/empty tag metadata as malformed. `Download` uses the `browser_download_url` selected from that same response. It has no cache, retry-to-stale-data behavior, or alternate release source.

The updater selects the binary by `runtime.GOARCH` through an injected `func() string` seam:

```go
func BinaryAsset(release Release, goarch string) (Asset, error)
func ChecksumAsset(release Release) (Asset, error)
```

`amd64` maps to `moonarch-cli-linux-amd64`; `arm64` maps to `moonarch-cli-linux-arm64`; all other values return `UnsupportedArchitectureError` before asset download, target-directory creation, repository acquisition, or configuration work. `ChecksumAsset` requires the exact `SHA256SUMS.txt` asset. Missing or duplicate matching assets are hard failures.

### Checksum verifier

`cli/pkg/release/checksum.go` exposes a small pure seam:

```go
type ChecksumVerifier interface {
    Verify(assetName string, binary io.Reader, checksumList io.Reader) error
}

type SHA256Verifier struct{}

func (SHA256Verifier) Verify(assetName string, binary io.Reader, checksumList io.Reader) error
```

The parser accepts the canonical GNU `sha256sum` release format generated by the existing workflow: one 64-character hexadecimal SHA-256 digest, exactly two ASCII spaces, then a non-empty filename. It validates every non-empty checksum-list line, requires exactly one entry for the requested asset name, and computes the staged file's SHA-256 with `crypto/sha256`. Missing, duplicate, malformed, and mismatched entries are separate typed errors. The verifier does not allow a warning-only checksum path.

### Atomic binary replacer

`cli/pkg/release/replacer.go` owns only local binary staging and replacement; downloading stays in `Client` so HTTP and filesystem failures remain independently testable.

```go
type FileOps interface {
    MkdirAll(path string, perm os.FileMode) error
    CreateTemp(dir, pattern string) (*os.File, error)
    Open(name string) (*os.File, error)
    Chmod(name string, mode os.FileMode) error
    Rename(oldpath, newpath string) error
    Remove(name string) error
}

type BinaryReplacer interface {
    Replace(
        ctx context.Context,
        targetPath string,
        assetName string,
        binary io.Reader,
        checksumList io.Reader,
    ) error
}

type AtomicReplacer struct { /* FileOps and ChecksumVerifier */ }

func NewAtomicReplacer(files FileOps, verifier ChecksumVerifier) *AtomicReplacer
func (r *AtomicReplacer) Replace(
    ctx context.Context,
    targetPath string,
    assetName string,
    binary io.Reader,
    checksumList io.Reader,
) error
```

The production `FileOps` delegates to `os.MkdirAll`, `os.CreateTemp`, `os.Open`, `os.Chmod`, `os.Rename`, and `os.Remove`. The exact replacement sequence is:

1. Resolve the managed target from the injected home directory as `filepath.Join(home, ".local", "bin", "moonarch-cli")`. Do not use `os.Executable`, `$PATH`, or `EvalSymlinks`; the updater replaces the managed directory entry rather than an arbitrary invocation path. This matches `scripts/install.sh` exactly.
2. Select both release assets and open both HTTP download streams before touching the target directory. This keeps unsupported architecture, missing asset, and release-download failures mutation-free.
3. Create `$HOME/.local/bin` with mode `0o755` if absent, then call `CreateTemp` in that same directory. The unique temporary sibling guarantees that the final `os.Rename` is on one filesystem.
4. Copy the binary stream to the temporary file, `Sync`, and close it. A copy, sync, close, or context error removes the temporary file.
5. Reopen the staged file and run `ChecksumVerifier.Verify` against `SHA256SUMS.txt`. On any checksum error, close and remove the temporary file. The existing target has not been changed.
6. Only after successful verification, apply `0o755` to the staged file.
7. Call `os.Rename(tempPath, targetPath)` through `FileOps.Rename`. On rename or permission failure, remove the temporary file and return `BinaryReplacementError`; the prior target remains untouched. On success, do not remove the now-renamed path.

No binary is placed in the installer plan or transaction. A later configuration rollback restores only managed dotfile targets, never the prior executable.

## Command and orchestration design

### Cobra wiring and version flow

`cmd/update.go` follows the command-file registration convention:

```go
var updateCmd = newUpdateCommand()

func newUpdateCommand() *cobra.Command
func runUpdate(cmd *cobra.Command, args []string) error

type updateDependenciesFactory func(*cobra.Command) updateDependencies

func runUpdateWithFactory(
    cmd *cobra.Command,
    currentVersion string,
    newDeps updateDependenciesFactory,
) error
```

`newUpdateCommand` sets `Use: "update"`, a help string that states it updates both the CLI and managed dotfiles to the latest release, `Args: cobra.NoArgs`, and `RunE: runUpdate`. It defines no flags. Cobra rejects positional arguments and unknown `--only` flags before `RunE`, so dependency construction and side effects cannot begin.

`runUpdate` passes the existing build-time `Version` variable to `runUpdateWithFactory`. The factory helper performs this first branch:

```text
if currentVersion == "dev": write release-build-only message; return nil
```

Only a release build invokes `defaultUpdateDependencies`, creates the reporter, reads `GITHUB_TOKEN`, resolves the home directory, or contacts GitHub. `runUpdateWithDeps` repeats the guard defensively so direct command-flow tests cannot bypass it.

### Update types and dependency injection

`cmd/update_flow.go` contains the command-layer contracts:

```go
type UpdateStage string

const (
    StageRelease       UpdateStage = "release"
    StageBinary        UpdateStage = "binary"
    StageRepository    UpdateStage = "repository"
    StageConfiguration UpdateStage = "configuration"
)

type StageStatus string

const (
    StageSucceeded StageStatus = "success"
    StageSkipped   StageStatus = "skipped"
    StageFailed    StageStatus = "failed"
)

type RollbackOutcome string

const (
    RollbackNotRequired           RollbackOutcome = "not-required"
    RollbackComplete              RollbackOutcome = "complete"
    RollbackIncomplete            RollbackOutcome = "incomplete"
    RollbackManualRecoveryRequired RollbackOutcome = "manual-recovery-required"
)

type StageResult struct {
    Stage    UpdateStage
    Status   StageStatus
    Code     string
    Detail   string
    Rollback RollbackOutcome
    Err      error
}

type UpdateResult struct {
    CurrentVersion                  string
    LatestTag                       string
    BinaryActiveOnNextInvocation    bool
    Stages                          []StageResult // always release, binary, repository, configuration
}

type StageReporter interface {
    Start(stage UpdateStage)
    Complete(result StageResult)
}

type UpdateOrchestrator interface {
    Run(ctx context.Context, currentVersion string, reporter StageReporter) (UpdateResult, error)
}

type ConfigurationPlanBuilder interface {
    Build(repoRoot, homeDir string) (plan.InstallationPlan, error)
}

type ConfigurationExecutorFactory func(plan.InstallationPlan) PhaseExecutor
```

`PhaseExecutor` is reused unchanged from `install_flow.go`:

```go
type PhaseExecutor interface {
    Execute(context.Context, plan.InstallationPlan) (*report.ExecutionReport, error)
}
```

The private `updateDependencies` holds `release.Client`, `release.BinaryReplacer`, `RepositoryAcquirer`, `ConfigurationPlanBuilder`, `ConfigurationExecutorFactory`, `func() (string, error)` for home resolution, and `func() string` for architecture. All side effects therefore have a fakeable boundary. The concrete `updater` implements `UpdateOrchestrator` and returns a `StageError` that preserves the typed underlying cause:

```go
type StageError struct {
    Stage UpdateStage
    Code  string
    Cause error
}

func (e *StageError) Error() string
func (e *StageError) Unwrap() error
```

### Stage data flow

```text
Cobra validation
  -> Version == "dev" guard (successful no-op)
  -> release.Client.Latest
  -> release.CompareVersions(Version, latest.Tag)
  -> [older only] select/download/verify/rename binary
  -> RepositoryAcquirer.Acquire(explicit release-tag request)
  -> configuration-only plan builder
  -> transaction.New(plan) + installer.NewExecutor.Execute
  -> ordered stage results and Cobra return
```

The stage behavior is fixed as follows:

1. **Release**: Resolve latest metadata, validate both versions, and compare semantically. No mutable collaborator runs in this stage. Invalid installed or release versions, malformed release data, rate limits, transport failures, and installed-newer outcomes fail this stage and mark the other three as skipped.
2. **Binary**: For `InstalledOlder`, select assets, download them through the native client, verify the staged binary, chmod it, and atomically rename it. For `InstalledEqual`, emit a skipped result with code `already-current`; do not request either binary asset or checksum asset and do not invoke the replacer. A binary failure skips repository and configuration.
3. **Repository**: Resolve home once through the injected function and construct this request directly:

   ```go
   RepositoryRequest{
       Destination: filepath.Join(homeDir, ".cache", "dotfiles"),
       URL:         "https://github.com/MrUse77/dotfiles.git",
       Ref:         latest.Tag,
   }
   ```

   Call the existing seam exactly as it exists today:

   ```go
   Acquire(ctx context.Context, request RepositoryRequest, output io.Writer) (RepositoryAcquisition, error)
   ```

   Pass `io.Discard` for the acquirer's internal progress and let the update reporter own user-visible stage output. The request does not read configuration environment overrides and does not pass through `BuildRepositoryRequest()`.
4. **Configuration**: Build a configuration plan from the acquired repository root, run it through a new transaction-backed executor, and render the transaction recovery state if it fails. This runs in the same old in-memory process even when the binary was replaced.

A binary success sets `BinaryActiveOnNextInvocation` immediately, so the fact is preserved and reported even if the repository or configuration stage later fails. No stage attempts a coordinated rollback of a prior stage.

## RepositoryAcquirer decision

No `RepositoryAcquirer` interface change is required. The current signature already accepts the explicit ref that update needs:

```go
type RepositoryAcquirer interface {
    Acquire(ctx context.Context, request RepositoryRequest, output io.Writer) (RepositoryAcquisition, error)
}
```

`RepositoryRequest` already exposes `Destination`, `Ref`, and `URL`; no new method, request field, or parameter is necessary. This is preferable to adding a special update method because the existing implementation already gives update both required behaviors: fresh clone at a tag and existing-clone fetch/detached checkout at the requested ref.

To make the negative requirement testable without changing this interface, extract the current body of `BuildRepositoryRequest` to `buildRepositoryRequestFromEnvironment` and retain the public function as a package-private-hook wrapper used by install only:

```go
var buildRepositoryRequestImpl = buildRepositoryRequestFromEnvironment

func BuildRepositoryRequest() (RepositoryRequest, error) {
    return buildRepositoryRequestImpl()
}
```

The update test temporarily replaces `buildRepositoryRequestImpl` with a counter/panic fake and restores it with `t.Cleanup`; `runUpdateWithDeps` must leave its count at zero. The tests are not parallel while this package-level test hook is overridden. The update's fake `RepositoryAcquirer` also captures and asserts all three fixed request fields while `DOTFILES_*` contains conflicting values. This proves both the absence of the legacy builder call and the release-tag request behavior.

## Configuration-only plan and transaction

The production configuration builder is intentionally distinct from `newInstallPlanner`:

```go
type updateConfigurationCatalog interface {
    plan.ActionCatalog
    plan.PhaseActionCatalog
}

func newUpdateConfigurationPlanBuilder(
    discoverer plan.TargetDiscoverer,
    catalog updateConfigurationCatalog,
) ConfigurationPlanBuilder
```

Production wiring passes `installDiscoverer{}` and `installer.NewActionCatalog()`. `Build` creates one run with `plan.Options{Mode: "user"}` and calls only:

```go
planner.BuildConfiguration(run, repoRoot, homeDir)
```

`BuildConfiguration` discovers managed targets, binds their pre-state, calls `ConfigurationActions(repoRoot, homeDir, opts, managedTargets)`, and returns a configuration-role plan. It does not call `PackageActions` or `ExternalActions`. `installer.NewActionCatalog()` has no detected power-profile state, so its configuration action set is limited to its file-configuration behavior and cannot inject package, `paru`, `hyprpm`, plugin, or privileged power-profile work.

The default executor factory is exactly:

```go
func(p plan.InstallationPlan) PhaseExecutor {
    tx := transaction.New(p)
    return installer.NewExecutor(
        tx,
        external.NewRunner(nil).WithStdio(cmd.InOrStdin(), io.Discard, io.Discard),
    )
}
```

It is used even when a configuration plan has no managed targets, preserving the update contract that configuration execution goes through the existing transaction lifecycle. `transaction.Execute` owns prepare, commit, and rollback; the update layer reads its `report.ExecutionReport` only for evidence and maps rollback state to `RollbackNotRequired`, `RollbackComplete`, `RollbackIncomplete`, or `RollbackManualRecoveryRequired`. A prepare failure has no mutated target to restore, and an external configuration action can fail after a successful managed transaction; both report `rollback="not-required"`. A managed commit failure follows the existing transaction rollback path and reports `complete`, `incomplete`, or `manual-recovery-required` from the report. It never calls `ui.Run`, Bubble Tea, Huh, `PackageActions`, `ExternalActions`, or the plugins command.

## TTY and stage reporting

### TTY decision

Use `github.com/mattn/go-isatty`, already present in `go.mod` indirectly, rather than a new syscall implementation. `cli/cmd/update_output.go` defines:

```go
type ttyDetector func(io.Writer) bool

func isTTY(w io.Writer) bool {
    file, ok := w.(*os.File)
    return ok && (isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd()))
}

func newUpdateReporter(out io.Writer, detectTTY ttyDetector) StageReporter
```

`runUpdateWithDeps` calls `newUpdateReporter(cmd.OutOrStdout(), isTTY)` only after the dev guard. The output branch belongs solely in the reporter factory; release, repository, checksum, transaction, and updater code emit typed stage events and never inspect terminal state. A wrapped/non-file writer safely takes the deterministic non-TTY branch.

### Result and output contract

Results are always ordered `release -> binary -> repository -> configuration`. `StageSucceeded`, `StageSkipped`, and `StageFailed` are the only terminal statuses. A stage blocked by a prior failure is reported as `skipped` with `code="blocked-by-<stage>"`; equal-version binary work is `skipped` with `code="already-current"`.

Non-TTY output is the canonical machine-facing form. Every event is one complete ASCII-safe line with fixed key order; dynamic values are quoted after removing control characters. It contains no ANSI sequence, prompt, progress spinner, or URL/token. Representative output is:

```text
stage=release status=running
stage=release status=success current="v1.0.0" latest="v1.1.0"
stage=binary status=success detail="replaced" activation="next-invocation"
stage=repository status=success ref="v1.1.0"
stage=configuration status=success
```

An equal-version run emits `stage=binary status=skipped code="already-current" detail="binary already up to date"` before repository and configuration success lines. A configuration failure after replacement emits prior success lines, then for example:

```text
stage=configuration status=failed code="configuration-transaction" rollback="complete" error="..."
```

TTY output uses the same stage order and facts with human-readable labels such as `[release] resolving`, `[binary] replaced; active on next invocation`, and `[configuration] failed; rollback complete`. It uses no blocking prompt and no terminal-control dependency; the TTY branch exists for readable progress, while non-TTY remains byte-stable for the same collaborator responses.

## Error taxonomy and exit behavior

`pkg/release/errors.go` supplies typed causes, and `StageError` adds the failing stage/code without erasing `errors.Is` / `errors.As` behavior.

| Typed cause / condition | Stage result code | Stage status and subsequent work | Exit |
| --- | --- | --- | --- |
| `*release.TransportError` from latest metadata | `transport` | Release failed; binary/repository/configuration skipped; no mutation | 1 |
| `*release.RateLimitError` (403 exhausted or 429) | `github-rate-limit` | Release failed before mutation; a download-time rate limit instead fails binary | 1 |
| `*release.HTTPStatusError` other non-2xx | `github-http-status` | Release or binary fails at the operation that received it | 1 |
| `*release.MalformedReleaseError` (invalid JSON, missing tag) | `malformed-release` | Release failed; later stages skipped | 1 |
| `*release.InvalidVersionError` | `invalid-installed-version` or `invalid-release-tag` | Release failed; later stages skipped | 1 |
| Installed semantically newer | `installed-newer-than-latest` | Release failed; no binary/repository/config mutation | 1 |
| `*release.UnsupportedArchitectureError` | `unsupported-architecture` | Binary failed before downloads/target creation; repository/configuration skipped | 1 |
| `*release.AssetNotFoundError` | `asset-missing` or `checksum-list-missing` | Binary failed; current executable unchanged; later stages skipped | 1 |
| `*release.ChecksumFormatError`, `*release.ChecksumEntryMissingError`, `*release.ChecksumEntryAmbiguousError` | `checksum-malformed`, `checksum-entry-missing`, or `checksum-entry-ambiguous` | Binary failed; temp removed and prior executable unchanged | 1 |
| `*release.ChecksumMismatchError` | `checksum-mismatch` | Binary failed; temp removed and prior executable unchanged | 1 |
| `*release.BinaryReplacementError` (directory, write, sync, chmod, permission, or rename) | `binary-replace` | Binary failed; temp cleanup attempted and prior executable unchanged; later stages skipped | 1 |
| `*RepositoryAcquisitionError` wrapping `Acquire` | `repository-acquisition` | Repository failed; configuration skipped; earlier binary result retained | 1 |
| `*ConfigurationPlanError` | `configuration-plan` | Configuration failed before transaction execution | 1 |
| `*ConfigurationTransactionError` carrying `*report.ExecutionReport` | `configuration-transaction` | Configuration failed; report rollback as not-required, complete, incomplete, or manual-recovery-required | 1 |
| Development build | none; direct release-build-only message | Successful no-op; no collaborators constructed or called | 0 |
| All four terminal stages succeed, including equal-version binary skip | none | Success | 0 |

No command path calls `os.Exit` directly. The existing `Execute()` function prints a returned `RunE` error to stderr and exits `1`; successful no-op and successful reconciliation return `nil`. The stage reporter renders the classified stage outcome before the error reaches `Execute()`.

## Test plan (strict TDD)

Implementation starts with focused RED tests, then the smallest implementation needed to pass them. Use table-driven cases, `t.TempDir()` for every filesystem test, injected HTTP/filesystem/repository/executor seams, and do not call real GitHub or mutate the real home directory. The mandatory suite is run from `cli/`:

```bash
go test ./...
go test -v -race -count=1 ./...
```

| Test file | Tests written first | Fakes and key assertions |
| --- | --- | --- |
| `pkg/release/version_test.go` | Leading `v`, no prefix, semantic ordering (`1.10.0 > 1.9.0`), equality, invalid installed, invalid release tag. | Table-driven `CompareVersions` tests assert `InvalidVersionError.Subject` and no raw lexical comparison. |
| `pkg/release/client_test.go` | Anonymous/authenticated headers, exact latest endpoint, bounded request context, valid JSON, missing tag, malformed JSON, transport error, rate limit, other non-200, and no fallback. | A fake `HTTPDoer` captures method, URL, headers, and deadline; it returns deterministic response bodies/errors. No test invokes a network or shell. |
| `pkg/release/checksum_test.go` | Valid one-entry verification; missing, duplicate, malformed, and mismatched target entry; missing checksum asset selection. | `bytes.Reader` fixtures and typed-error assertions. The malformed cases cover wrong hash length/non-hex and wrong separator. |
| `pkg/release/replacer_test.go` | Success leaves executable `0o755`; checksum mismatch/read interruption/chmod/rename failure leave old target bytes unchanged and remove temp files; target directory creation; verify-before-chmod. | `t.TempDir`, a fake `FileOps` that injects rename/chmod failures, an observing fake verifier, and a failing reader. No real home path is used. |
| `cmd/update_test.go` | Root command finds `update`; help/usage communicates CLI-plus-dotfiles; positional argument and unknown `--only` fail before factory invocation; dev build exits `0`. | `runUpdateWithFactory` receives a counting factory. The dev test also supplies counting release/replacer/acquirer/planner/executor fakes and asserts every call count remains zero. It restores the global `Version` with `t.Cleanup`. |
| `cmd/update_flow_test.go` | Older/equal/newer routing; invalid version; architecture mapping; explicit repository request; binary/repository/configuration failure isolation; partial success; no re-exec behavior. | Dedicated fakes record an event log, release asset/download calls, replacement calls, `RepositoryRequest`, plan calls, executor calls, and reporter events. Equal version asserts zero binary/checksum download and zero replacer calls while repository/configuration still run. Newer/invalid/unsupported cases assert zero mutation collaborators. |
| `cmd/update_flow_test.go` configuration tests | Build a configuration-role plan through `newUpdateConfigurationPlanBuilder`, execute it through a transaction-backed factory, and surface rollback state. | A fake catalog implements both `PackageActions` and `ConfigurationActions`; assert `ConfigurationActions == 1`, `PackageActions == 0`, and no `ExternalActions`/plugin path is requested. The captured plan contains no `paru`, package-manager, `hyprpm`, or non-configuration action. Faked reports cover not-required, complete, incomplete, and manual-recovery rollback outcomes. |
| `cmd/update_flow_test.go` legacy-builder test | Update ignores legacy environment request construction. | Override `buildRepositoryRequestImpl` with a counter/panic fake, set conflicting `DOTFILES_DIR`, `DOTFILES_REPO`, and `DOTFILES_BRANCH`, execute update, and assert zero builder calls plus a captured fixed destination/URL/release-tag request. Restore the hook with `t.Cleanup`; do not mark this test parallel. |
| `cmd/update_output_test.go` | Non-TTY lines are deterministic and ANSI/prompt free; TTY labels expose all stages; failure/skip ordering is stable. | Pass `func(io.Writer) bool { return false }` and `true` into `newUpdateReporter`; compare repeated non-TTY output byte-for-byte and assert no `\x1b`. |
| Existing installer/repository tests | Preserve fresh/existing tag acquisition and transaction behavior. | Extend only where necessary to keep the explicit frozen-ref assertions. Existing `fakeGitRunner`, `fakeAcquirer`, `fakePhaseExecutor`, `fakePhaseCatalog`, and transaction fakes demonstrate the repository's established style; update tests use focused equivalents rather than real GitHub or package tools. |

The end-to-end fake-flow test is the critical safety test: it proves `Version == "dev"` invokes zero collaborators, and the successful release path invokes only release/binary/repository/configuration seams in order. Its configuration fake proves that neither package actions, `paru`, `hyprpm`, nor an external installer action set is requested.

## Documentation, rollout, and rollback

### User-facing documentation

`README.md` gains `moonarch-cli update` in the command list and a focused update section covering:

- release-build-only availability and the `dev` no-op;
- required online GitHub access and optional `GITHUB_TOKEN`;
- exactly what updates: managed binary, fixed cache clone at the release tag, and file-based configuration;
- what never runs: packages, `paru`, Hyprland plugins/`hyprpm`, and non-configuration installer actions;
- equal-version reconciliation, no re-exec, and next-invocation activation;
- independent recovery boundaries: configuration uses retained transaction backup/inventory; binary/repository recovery requires restoring a previous verified release/tag independently.

`RELEASING.md` records the compatibility contract already satisfied by the workflow: stable SemVer tags, both Linux architecture asset names, and a canonical `SHA256SUMS.txt` with one entry per asset. This makes future release-pipeline edits consciously preserve update compatibility.

### Rollout

1. Add the pure SemVer, release-client, checksum, and atomic-replacer RED tests before their implementations.
2. Add command-flow fakes and the dev/no-side-effect test before wiring the Cobra command.
3. Add the configuration-plan isolation tests before reusing installer planner/executor seams.
4. Run focused package tests while implementing, then run `go test ./...` and `go test -v -race -count=1 ./...` from `cli/`.
5. Validate a release artifact manually in a disposable home directory: older-version update, equal-version reconciliation, and a forced configuration failure that shows binary success plus configuration rollback reporting.

### Recovery and code rollback

If failure occurs before rename, remove only the staged temp file; the prior executable remains. If configuration fails after a binary replacement, do not claim a global rollback: retain the transaction inventory/backups, leave the new executable in place, and document reinstalling a verified previous release asset plus restoring the previous repository tag when that is desired. Reverting this change removes the command and its documentation but does not undo any user machine state that was already updated.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Go and shell release paths drift. | Lock the current asset/checksum contract in native Go unit tests and document it in `RELEASING.md`; do not refactor the shell installer in this change. |
| A dev build accidentally creates dependencies or probes the machine. | The version guard precedes factory, reporter, home, token, architecture, HTTP, repository, planner, executor, and transaction creation; a counting-factory test proves it. |
| An update uses `main` or environment override state. | Reuse the existing request-bearing acquirer interface with a fixed tag request and make `BuildRepositoryRequest()` absence observable in tests. |
| Installer planner accidentally introduces package/power-profile behavior. | Use a new `installer.NewActionCatalog()` configuration builder, not `newInstallPlanner()`, and assert phase catalog call counts/actions. |
| Binary and configuration states diverge after a later failure. | Report each stage independently, retain configuration rollback evidence, and document separate recovery; never put the executable in the configuration transaction. |
| Redirected output becomes non-deterministic or interactive. | Centralize output behind a post-guard reporter; non-TTY output is fixed line-oriented data without ANSI or prompts. |
