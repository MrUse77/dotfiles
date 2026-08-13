# Design: MoonArch Versioned Configuration Releases

## Technical Approach

`config apply config-vX.Y.Z` and `config rollback` are the **only** configuration-mutating commands. Both run inside one exclusive kernel advisory lock on `XDG_STATE_HOME/moonarch/lock`, share the existing transaction stack, and persist durable identity under `XDG_STATE_HOME/moonarch/state.json` with a per-operation NDJSON journal beside it. `moonarch self update` (canonical) and its alias `moonarch update` are strictly CLI-only: release discovery, asset download, SHA-256 verification, atomic binary rename, and exit — they never acquire a config checkout, never call the planner, and never touch state/journal/lock/inventory. `MOONARCH_FORCE_REPO` is removed.

Configuration releases are self-contained `config-vX.Y.Z` GitHub Releases. Each contains a schema-versioned manifest, complete managed-target catalog, declared external dependencies with CLI compatibility range, asset payloads, materialized submodule contents, per-entry digests, and a separate immutable `<tag>.tar.zst.sha256` sidecar. A legacy bridge mechanism guarantees the repository-wide `/releases/latest` endpoint resolves a supported `v*` CLI release until legacy clients are retired. Apply admits an artifact only after every digest matches, every manifest entry is verified, and a fail-closed extractor rejects traversal/duplicate/escape/unsupported entries; only then does it promote into `XDG_DATA_HOME/moonarch/artifacts/<sha256>/`. Current and previous artifacts are retained there so rollback works offline.

Apply runs `transaction.Prepare` + `transaction.Commit` over the closed planner output, holding the lock through identity commit. Whole-plan preflight (including explicit evidence-bound drift authorization for first-apply on `legacy/unknown`) gates the transaction. The transaction engine keeps its existing `Inventory` lifecycle but grows an additive `ReleaseProvenance{Tag, Digest}` so a schema-1 inventory still decodes as `ReleaseProvenance = nil` ("unknown identity") and `restore` continues to work. `themes/current` is excluded from the immutable `CopyTree` replacement and is preserved as a relative bundle selection that is only rewritten when the user supplies an explicit replacement.

## Architecture Decisions

| # | Decision | Alternatives | Rationale |
|---|---|---|---|
| 1 | `ParseConfigVersion(raw) → VersionIdentity{Tag, Digest?}` accepts only exact `config-vX.Y.Z`; `CompareVersions` keeps SemVer for `v*` CLI tags only | Single parser; channel/pin lookup | Spec `config-release-resolution` rejects `latest`, channel, prerelease, and `v*`; CLI self-update keeps legacy SemVer. |
| 2 | One CLI-only pipeline (`release.Latest` → `BinaryAsset`/`ChecksumAsset` → `AtomicReplacer`) for both `self update` and `update`; `update.go` is a thin alias that calls into the same runner | Four-stage update pipeline (existing) with config toggle | Spec `moonarch-cli-self-update` forbids config mutation; alias must be byte-equivalent; removes `MOONARCH_FORCE_REPO` and the existing `runRepository`/`runConfiguration` stages from update. |
| 3 | Whole-artifact SHA-256 published as an external `<tag>.tar.zst.sha256` sidecar; manifest catalog carries per-entry digests | Embed digest in archive name; sign archive | Archive cannot hash its own bytes; an external sidecar is verified before extract and after staging. |
| 4 | Admission phases: download → sidecar SHA-256 match → fail-closed extract (no traversal/escape/dup/unsupported/manifest-mismatch) → manifest schema + CLI compat range check → declared-dependency probe (read-only) → atomic rename into `artifacts/<digest>/` | Stream-while-extract | Each phase emits a typed failure; cache is promoted only after every check passes. |
| 5 | Legacy bridge: `.github/workflows/release.yml` first publishes a synthesized `latest`-pointing `v*` CLI release that references the **highest published CLI release**, not a config release; config publication is gated on a verified bridge artifact | Manual operator step; `gh api` redirect | `scripts/install.sh` and legacy `update` clients use `/releases/latest`; until they're retired, `/releases/latest` MUST resolve a `v*` CLI tag. The bridge is a real workflow step, not an env override. |
| 6 | Immutable release identity: publication computes digest from finalized bytes; if a tag re-occurs with a different digest, the workflow refuses and surfaces the existing bytes | Mutable tag, force-replace | Spec `config-release-publication` forbids replacement under the same identity; client caches bind to `(Tag, Digest)`, not tag alone. |
| 7 | `unix.Flock(fd, LOCK_EX\|LOCK_NB)` on `XDG_STATE_HOME/moonarch/lock`; advisory only; auto-released on fd close; contention → typed error | PID file + stale-deletion; `O_EXCL` rename | Kernel-managed; no PID race; auto-release on crash/exit; `dep` already present. |
| 8 | `EvidenceToken = hex(SHA256(canonicalJSON({tag, artifactDigest, observations})))`; first-run abort prints the token and the per-path observation set; `--authorize-drift <hex>` re-runs preflight under the same lock and matches the token against a fresh full-plan scan | `--force`; `MOONARCH_FORCE_DRIFT` env | Spec binds the authorization to the exact release + observed set; re-scan under lock; no env bypass; an unbound force value cannot authorize. |
| 9 | NDJSON journal phases `op-start → prepared → committing → mutated → committed → state-finalized → op-end`; recovery walks the tail under lock | SQLite WAL; single-state file | Explicit + falsifiable; one writer under lock; truncation rules are deterministic (see §Journal phases). |
| 10 | `Inventory.ReleaseProvenance` is additive (`json:"release,omitempty"`); schema-1 decoders set it to `nil` and `restore` accepts unknown identity; decode failure on `format_version == 1` continues to be impossible because the field is omitted | Bump format version; break old inventories | Additive JSON keeps every retained run restorable; `format_version == 1` + missing `release` field is the documented "unknown identity" state. |
| 11 | XDG split: payloads `XDG_DATA_HOME/moonarch/artifacts/<digest>/`; state `XDG_STATE_HOME/moonarch/{state.json, journal.ndjson, lock}` | Single `$HOME/.moonarch` | XDG spec; state survives uninstall/reinstall; retained artifacts = offline rollback. |
| 12 | `themes/current` is **not** a managed target. It is a separate post-planning phase that reads the current relative link, validates it points at a bundle present in the desired catalog, and aborts unless the user passes `--theme-replace <id>`. Immutable `CopyTree` operates on the bundle directory only | Whole-tree copy; silent fallback | Spec `moonarch-theme-selector` requires explicit replacement; preserves the user-selected mutable state; matches existing `theme-selector` atomic-reload contract. |
| 13 | New `plan.Remove` mutation kind for "managed target was installed but is omitted from desired". Transaction commits the existing target after backup, marks the inventory entry `EntryRemoved`, and Rollback restores from backup on later failure | Mutate-as-Remove only on filesystem | Spec `installation-transaction` requires the absent target to be a no-op, unchanged/authorized drift to be backed up then deleted, later failure to restore, and removal to be inventoried. |
| 14 | Config apply uses **only** `transaction.Prepare`/`Commit` over `ManagedTargets` and a read-only declared-dependency probe (no `Runner`, no `CommandSpec` execution) | Run full external actions | Spec forbids mutating packages/services/plugins; the dependency check is informational, never privileged. |
| 15 | `config rollback` is a new transaction that uses the **previous** identity's already-admitted cache entry as the new desired target set. It reuses apply's lock/journal/preflight/backup/inventory rules with the **offline** flag set (no network, no acquisition, no dependency download) | Re-run apply in reverse | Spec requires the same lock/journal/preflight/backup/inventory guarantees plus "no network/Git". |
| 16 | `config status` reads state under the same lock after journal recovery; reports `legacy/unknown` when no verified identity has ever been written; surfaces retained artifacts and any unresolved journal tail | Read state without lock | Lock guarantees the status snapshot is not torn with a concurrent apply. |

## Data Flow

### A. `moonarch config apply config-vX.Y.Z [--authorize-drift T] [--theme-replace id]`

```text
open XDG_STATE/moonarch/lock; unix.Flock LOCK_EX|LOCK_NB
  recover journal tail (see §Journal phases)  → committed | uncommitted | indeterminate
      indeterminate → block; report; release lock; exit 2
  acquire artifact
      ParseConfigVersion  (rejects latest/channel/prerelease/v*)
      GitHubClient.GetByTag("config-vX.Y.Z")
      download <tag>.tar.zst + sidecar <tag>.tar.zst.sha256
      verify sidecar (SHA-256 of archive bytes)
      fail-closed extract to XDG_DATA/moonarch/staging/<digest>/
          reject abs paths, "..", symlink escapes, dup paths, unsupported types,
          archive entries missing from manifest, manifest entries missing from archive
      parse manifest: schema, cli_compat_range, dependency_decls, catalog[]
      verify per-entry digests in catalog against extracted bytes
      declared-dependency probe (read-only):
          for each dep: probe presence + version; NEVER install/update/remove
      atomic rename staging → XDG_DATA/moonarch/artifacts/<digest>/
  baseline selection
      current identity nil → "legacy/unknown"; if last completed inventory has
        FormatVersion==1, use its recorded managed identities as the optional
        baseline; otherwise no baseline (every desired destination = untrusted drift)
  planner.Build(run, XDG_DATA/moonarch/artifacts/<digest>/, home)
      discovers installed+desired union, classifies as creations/replacements/removals
      excludes themes/current from ManagedTargets
      themes-relative phase:
          read current themes/current → bundle name
          if absent or link escapes or points outside theme bundles → ABORT
          unless --theme-replace <id> is present and <id> is in the desired bundle set
  journal op-start   {op: apply, tag, artifact_digest}
  preflight (whole-plan, themes/current excluded)
      scan every replacement/removal; record {path, expected_identity/digest, observed}
      drift != baseline?
          no --authorize-drift → print evidence + token; ABORT
          with --authorize-drift T:
              re-scan under lock; if token != T or any observation differs → ABORT
              if fresh token matches T → continue
  transaction.Prepare
      Inventory{FormatVersion:1, ReleaseProvenance:{Tag,Digest}, ...}
      ensure backup root; back up each replacement; back up each removal
      delete-then-record each removal
  journal committed-target-1 ... committed-target-N
  transaction.Commit
      apply replacements atomically; restore runtime sockets
  journal state-finalized
      rotate identity: previous := current; current := {Tag, Digest}
      atomic write state.json (write tmp, fsync, rename in XDG_STATE_HOME/moonarch/)
  journal op-end
release fd (kernel releases flock); exit 0
```

### B. `moonarch config rollback` (offline exception)

```text
open XDG_STATE/moonarch/lock; unix.Flock LOCK_EX|LOCK_NB
  recover journal tail → indeterminate → block; release; exit 2
  read state.json
      current == nil OR previous == nil OR !previous.verified → ABORT (no offline apply)
      verify previous artifact still present in XDG_DATA/moonarch/artifacts/<digest>/
  planner.Build(run, XDG_DATA/moonarch/artifacts/<previous.Digest>/, home)   -- offline
  themes-relative phase uses --theme-replace if previous selection is invalid
  preflight → drift evidence (same path as apply)
      drift without --authorize-drift → ABORT
  transaction.Prepare+Commit with ReleaseProvenance{Tag: previous.Tag, Digest: previous.Digest}
  journal state-finalized; rotate identities (previous ↔ current)
release fd; exit 0
```

### C. `moonarch config status`

```text
open XDG_STATE/moonarch/lock; flock LOCK_EX|LOCK_NB
  recover journal tail
      uncommitted/indeterminate → status reports "unresolved journal"
      but NEVER mutates anything
  read state.json
      current == nil → identity = "legacy/unknown"
      current != nil → print {tag, digest, retention_count}
      previous != nil → print previous
  retained artifacts
      scan XDG_DATA/moonarch/artifacts/ for any digest no longer referenced;
      print only the protected set (current+previous) and "X orphans may be removed"
release fd; exit 0
```

### D. `moonarch self update` / `moonarch update` (configuration-neutral)

```text
release.Latest → BinaryAsset(amd64|arm64) → SHA256SUMS.txt
download both, stage in target dir
SHA256Verifier.Verify(staged, assetName, checksumList)
chmod 0755 staged
os.Rename(staged, ~/.local/bin/moonarch-cli)   -- atomic on same fs
exit; new binary active on next invocation
NO acquisition, NO planner, NO state.json/journal/lock touch, NO inventory write
MOONARCH_FORCE_REPO: not read; removed from update.go and install.sh
```

## File Changes

| Group | Files | Action |
|---|---|---|
| Release pkg | `cli/pkg/release/version.go` | Modify: add `ParseConfigVersion(raw) (VersionIdentity{Tag, Digest?}, error)` rejecting anything that isn't `config-vMAJOR.MINOR.PATCH`; keep legacy `CompareVersions` for `v*`. |
| Release pkg | `cli/pkg/release/client.go` | Modify: add `GetByTag(ctx, tag) (Release, error)` to `Client` interface; `Latest()` is preserved only for self-update; `GetByTag` rejects non-`config-v*` selectors and never falls back. |
| Release pkg | `cli/pkg/release/resolver.go` | Create: `Resolver` that, given an exact tag, fetches both `<tag>.tar.zst` and `<tag>.tar.zst.sha256`, verifies the sidecar, and stages to `XDG_DATA/moonarch/staging/<digest>/`. |
| Release pkg | `cli/pkg/release/admit.go` | Create: fail-closed extractor (rejects `..`, absolute, escapes, dup normalized paths, unsupported types, manifest-mismatch). Returns typed errors; never promotes cache. |
| Release pkg | `cli/pkg/release/cache.go` | Create: `Cache.Promote(staging, digest)`, `Lookup(digest)`, `Retain(current, previous)`. Promotion = atomic rename into `XDG_DATA/moonarch/artifacts/<digest>/`. Cleanup removes only digests not referenced by current/previous. |
| Release pkg | `cli/pkg/release/compat.go` | Create: schema-version + CLI compat-range check; no mutation; returns typed `CompatibilityError`. |
| Release pkg | `cli/pkg/release/depend.go` | Create: read-only declared-dependency probe. Receives a `DependencyProbe` interface (file/process probe) so tests can inject; never shells out a privileged command. |
| Release pkg | `cli/pkg/release/manifest.go` | Create: `Manifest` + `CatalogEntry` types and JSON parser. `CatalogEntry` carries `Path, Digest, Mode, Kind, Executable bool`. |
| Release pkg | `cli/pkg/release/identity.go` | Create: `Identity{Tag, Digest string}`, `EvidenceObservation`, `EvidenceToken(target, observations) string`, `VerifyToken(target, observations, presented string) error`. |
| Release pkg | `cli/pkg/release/lock_unix.go` | Create: `Lock.Acquire(path) (fd *os.File, release func(), err error)` using `unix.Flock(fd, LOCK_EX\|LOCK_NB)`; contention → `ErrLockContended`; close → kernel release. |
| Release pkg | `cli/pkg/release/journal.go` | Create: NDJSON appender with phase constants `op-start → prepared → committing → mutated → committed → state-finalized → op-end`; `Recovery()` reducer returns `Committed`, `Uncommitted`, or `Indeterminate` (see §Journal phases). |
| Release pkg | `cli/pkg/release/state.go` | Create: `State{Current, Previous *Identity; LastCompletedRunID string}` + atomic write (tmp + fsync + rename). |
| Release pkg | `cli/pkg/release/errors.go` | Modify: add typed errors `ErrLockContended`, `ErrArtifactRejected`, `ErrIndeterminateJournal`, `ErrNoPreviousIdentity`, `ErrOfflineArtifactMissing`, `ErrUnboundForce`. |
| Release pkg | `cli/pkg/release/version_test.go`, `client_test.go`, `admit_test.go`, `cache_test.go`, `journal_test.go`, `identity_test.go`, `lock_unix_test.go` | Create: table-driven unit tests (see Testing Strategy). |
| CLI commands | `cli/cmd/self_update.go` | Create: canonical CLI-only runner that calls `release.Latest`, `BinaryAsset`, `ChecksumAsset`, `AtomicReplacer`. **Rejects** `config-v*` selector. No state, journal, lock, planner, or executor. |
| CLI commands | `cli/cmd/update.go` | Modify: thin alias that delegates to the same `self_update` runner; removes the existing four-stage orchestrator (`runRepository`, `runConfiguration`); removes `MOONARCH_FORCE_REPO`. |
| CLI commands | `cli/cmd/config.go` | Create: `config` parent with subcommands `apply config-vX.Y.Z`, `rollback`, `status`. Flags: `--authorize-drift <hex>` (apply/rollback), `--theme-replace <id>` (apply/rollback), `--offline` (rollback default true), `--json`. |
| Plan | `cli/pkg/installer/plan/plan.go` | Modify: add `MutationKind Remove = "remove"`; `Target.Kind` may carry `Remove`; `PreState` semantics unchanged. |
| Plan | `cli/pkg/installer/plan/planner.go` | Modify: `buildTargets` reconciles `installed ∪ desired` into explicit `Remove` targets when installed ⊄ desired; never adds targets after the plan is closed. `Discover` (via the discoverer seam) emits the union; themes/current is emitted by a separate post-discovery phase, not as a managed target. |
| Plan | `cli/pkg/installer/plan/source_binding.go` | Modify: `buildSourceBinding` returns a zero-value `SourceBinding` and `Kind = ""` for `Remove` targets (no source digest required). |
| Plan | `cli/pkg/installer/plan/planner.go` | Modify: discoverer seam returns the installed+desired union; an explicit `themeSelectorPath = ~/.local/share/moonarch/themes/current` is **not** in the union and is validated separately by `config apply`. |
| Transaction | `cli/pkg/installer/transaction/transaction.go` | Modify: `mutateTarget` dispatches on `Kind`. New branch: `plan.Remove`. Remove semantics: if `preState.Type == StateAbsent`, skip + record `EntrySkipped(absent)`; else back up via `backupTarget`, then `fs.RemoveAll(destination)`, mark `EntryRemoved`; on later failure, `Rollback` re-runs `restoreFromBackup`. |
| Transaction | `cli/pkg/installer/transaction/inventory.go` | Modify: add `InventoryEntry.ReleaseProvenance *plan.Target` is **not** added; instead add `Inventory.ReleaseProvenance {Tag, Digest}` (additive JSON `release,omitempty`) and a new `EntryRemoved` state. `FormatVersion` stays at `1`. Schema-1 decoders leave `ReleaseProvenance = nil` (unknown identity). `Restore` and `restoreCandidates` ignore `ReleaseProvenance`. |
| Filesystem | `cli/pkg/installer/transaction/filesystem.go` | Unchanged: `Remove` and `RemoveAll` already exist; remove mutation reuses them. |
| External | `cli/pkg/installer/external/runner.go` | Unchanged: `config apply` does **not** call `external.Runner`; mutating actions are forbidden by design. The dependency probe lives in `pkg/release/depend.go` with its own read-only interface. |
| Theme | `cli/cmd/theme_phase.go` (new) | Create: post-planning phase that reads `~/.local/share/moonarch/themes/current` via `unix.Readlink`; validates the link stays inside the desired `themes/` set; aborts unless `--theme-replace <id>` names a valid bundle; on commit, atomically rewrites the link via `rename(2)` of a sibling temp. Uses `home/.local/bin/moonarch/theme-selector`'s atomic-reload contract unchanged. |
| Publication | `.github/workflows/release.yml` | Modify: add `release-cli` job (existing `v*` path) and a new `release-config` job gated on the **legacy bridge** check; bridge verifies `/releases/latest` resolves a `v*` tag before allowing a `config-v*` to be published. `release-config` job materializes pinned submodules, builds the deterministic tar.zst, computes digest, writes `<tag>.tar.zst.sha256`, and refuses to upload if the digest already exists. |
| Publication | `scripts/install.sh` | Modify: keep `latest_release()` pointed at `/releases/latest`; add a hard guard that fails if `/releases/latest` resolves a `config-v*` tag (defensive). |
| Publication | `RELEASING.md` | Modify: document the bridge requirement, the `<tag>.tar.zst.sha256` sidecar, the immutable-identity rule, and the publication gate. |
| Tests | `cli/cmd/self_update_test.go`, `cli/cmd/update_alias_test.go`, `cli/cmd/config_apply_test.go`, `cli/cmd/config_rollback_test.go`, `cli/cmd/config_status_test.go`, `cli/pkg/installer/plan/removal_test.go`, `cli/pkg/installer/transaction/remove_test.go` | Create (see Testing Strategy). |
| Shell | `tests/release-bridge_test.sh` | Create: asserts `/releases/latest` resolves `v*` while a newer `config-v*` exists; asserts that `config-v*` re-publication under the same tag is rejected. |
| CI | `.github/workflows/ci.yml` | Modify: invoke `bash tests/release-bridge_test.sh` and `cd cli && go test -race -count=1 ./...`. |

## Interfaces / Contracts

```go
// pkg/release/version.go
type VersionIdentity struct {
    Tag    string // exact "config-vMAJOR.MINOR.PATCH"
    Digest string // sha256 of artifact bytes; populated after admission
}
func ParseConfigVersion(raw string) (VersionIdentity, error) // rejects anything that isn't config-vMAJOR.MINOR.PATCH

// pkg/release/client.go
type Client interface {
    Latest(ctx context.Context) (Release, error)              // CLI self-update only
    GetByTag(ctx context.Context, tag string) (Release, error) // config apply/rollback; rejects non-config-v*
    Download(ctx context.Context, asset Asset) (io.ReadCloser, error)
}

// pkg/release/resolver.go
type Resolver interface {
    Resolve(ctx context.Context, tag string) (Artifact, error) // sidecar verify + staging
}

// pkg/release/admit.go
type Admitter interface {
    Admit(staging, digest string) error // fail-closed extraction + manifest verify
}
type ArtifactError struct{ Code string; Cause error }

// pkg/release/cache.go
type Cache interface {
    Promote(staging, digest string) error             // atomic rename into XDG_DATA/moonarch/artifacts/<digest>/
    Lookup(digest string) (string, error)             // path to admitted artifact root
    Retain(current, previous *Identity) error         // remove orphans not referenced by current/previous
}

// pkg/release/compat.go
type CompatError struct{ Reason string }

// pkg/release/depend.go
type DependencyProbe interface {
    Probe(ctx context.Context, name, constraint string) (DependencyResult, error)
}
type DependencyResult struct {
    Name       string
    Satisfied  bool
    Observed   string // empty when Satisfied=false and nothing observed
}

// pkg/release/identity.go
type Identity struct {
    Tag    string `json:"tag"`
    Digest string `json:"digest"`
}
type EvidenceObservation struct {
    Path              string `json:"path"`        // managed-target destination
    ExpectedIdentity  string `json:"expected"`    // planned pre-state identity (dev:ino + digest prefix)
    ObservedIdentity  string `json:"observed"`    // actual pre-state at preflight
    DriftClass        string `json:"class"`       // "replacement" | "removal" | "creation-pre"
}
type EvidenceTokenInput struct {
    Tag             string               `json:"tag"`
    ArtifactDigest  string               `json:"artifact_digest"`
    Observations    []EvidenceObservation `json:"observations"`
}
func ComputeEvidenceToken(in EvidenceTokenInput) string              // hex(SHA256(canonicalJSON(in)))
func VerifyEvidenceToken(in EvidenceTokenInput, presented string) error

// pkg/release/lock_unix.go
type Lock struct{ /* ... */ }
func (l *Lock) Acquire() (release func(), err error) // ErrLockContended on LOCK_NB failure

// pkg/release/journal.go
type JournalPhase string
const (
    JournalOpStart        JournalPhase = "op-start"
    JournalPrepared       JournalPhase = "prepared"
    JournalCommitting     JournalPhase = "committing"
    JournalMutated        JournalPhase = "mutated"
    JournalCommitted      JournalPhase = "committed"
    JournalStateFinalized JournalPhase = "state-finalized"
    JournalOpEnd          JournalPhase = "op-end"
)
type JournalOutcome string
const (
    JournalOutcomeCommitted    JournalOutcome = "committed"
    JournalOutcomeUncommitted  JournalOutcome = "uncommitted"
    JournalOutcomeIndeterminate JournalOutcome = "indeterminate"
)
type JournalRecord struct {
    OpID     string      `json:"op_id"`
    Phase    JournalPhase `json:"phase"`
    Tag      string      `json:"tag,omitempty"`
    Digest   string      `json:"digest,omitempty"`
    Payload  any         `json:"payload,omitempty"`
    Ts       time.Time   `json:"ts"`
}
func (j *Journal) Append(rec JournalRecord) error
func (j *Journal) Recovery() (JournalOutcome, []JournalRecord, error)

// pkg/release/state.go
type State struct {
    Current           *Identity `json:"current,omitempty"`
    Previous          *Identity `json:"previous,omitempty"`
    LastCompletedRunID string   `json:"last_completed_run_id"`
}
func (s *State) WriteAtomic(path string) error // tmp + fsync + rename

// pkg/installer/plan/plan.go
type MutationKind string
const (
    CopyFile MutationKind = "copy-file"
    CopyTree MutationKind = "copy-tree"
    Symlink  MutationKind = "symlink"
    Remove   MutationKind = "remove" // NEW: target was installed but is omitted from desired
)
// Remove targets: Source="", ResolvedSource="", SourceDigest="" (no source-binding validation)

// pkg/installer/transaction/inventory.go
type Inventory struct {
    FormatVersion     int                `json:"format_version"`     // always 1
    RunID             string             `json:"run_id"`
    Lifecycle         InventoryLifecycle `json:"lifecycle"`
    Path              string             `json:"path,omitempty"`
    Entries           []InventoryEntry   `json:"entries"`
    ReleaseProvenance *ReleaseProvenance `json:"release,omitempty"`   // NEW: additive; absent in schema-1 inventories
}
type ReleaseProvenance struct {
    Tag    string `json:"tag"`
    Digest string `json:"digest"`
}
type InventoryEntryState string
const (
    // existing values ...
    EntryRemoved InventoryEntryState = "removed" // NEW: Remove mutation completed successfully
)
// Status reports include ReleaseProvenance as "unknown" when nil.
```

## Testing Strategy

| Layer | What | How |
|---|---|---|
| Unit (Go) | `ParseConfigVersion` rejects `latest`, `v*`, channel, prerelease, bare `config`; accepts exact `config-vMAJOR.MINOR.PATCH` | Table-driven in `pkg/release/version_test.go`; covers strict-SemVer edges (1.10.0 vs 1.9.0). |
| Unit (Go) | Fail-closed admit: archive with `../etc/passwd`, absolute entry, symlink escape, duplicate normalized paths, unsupported type, manifest entry missing from archive, archive entry missing from manifest | `t.TempDir()` tar fixtures in `pkg/release/admit_test.go`; each path returns a distinct typed error and asserts the cache directory is unchanged. |
| Unit (Go) | Sidecar digest match + mismatch; per-entry digest mismatch | `bytes.Reader` fixtures; SHA-256 verifier integration test. |
| Unit (Go) | `Cache.Promote` atomic; `Cache.Retain` removes orphans but never current/previous | Fake filesystem; observe rename events. |
| Unit (Go) | `DependencyProbe` is read-only; stub returning `Satisfied=true`/`false`/`unknown` | Table-driven in `pkg/release/depend_test.go`; assert no `CommandSpec` is ever executed. |
| Unit (Go) | `ComputeEvidenceToken`/`VerifyEvidenceToken` determinism, mismatch rejection, observation-set changes | Canonical-JSON golden test. |
| Unit (Go) | `Journal.Recovery()` returns `Committed` after `op-end`, `Uncommitted` after `committed-target-N` without `state-finalized`, `Indeterminate` on truncated tail | NDJSON fixture in `pkg/release/journal_test.go`; truncations at every phase boundary. |
| Unit (Go) | `unix.Flock` contention + fd release on subprocess exit | `Lock.Acquire` from a child `cmd`; assert parent observes `ErrLockContended`. |
| Unit (Go) | `Inventory` decode of schema-1 (no `release` field) succeeds; `ReleaseProvenance == nil`; `restoreCandidates` keeps the entry available | `inventory_test.go` golden JSON fixtures. |
| Unit (Go) | `plan.Remove`: unchanged retired target is backed up + deleted + recorded; absent retired target is a no-op (`EntrySkipped(absent)`); later mutation failure restores | `pkg/installer/transaction/remove_test.go` with table-driven scenarios. |
| Unit (Go) | `themes/current` validation: valid relative bundle preserved; missing bundle aborts without `--theme-replace`; explicit `--theme-replace <id>` rewrites the relative link atomically | `cmd/theme_phase_test.go` with `t.TempDir()` tree; uses `unix.Readlink` to assert link target. |
| Unit (Go) | Alias equivalence: `self update` and `update` produce byte-identical outcomes against the same release, both reject `config-v*` | `cmd/update_alias_test.go` shared fakes. |
| Unit (Go) | Self-update is configuration-neutral: no state.json, journal, lock, inventory, or `.cache/dotfiles` change | Fakes for state/journal/inventory; assert zero calls. |
| Unit (Go) | `config status` reports `legacy/unknown` when no current identity; reports current+previous when present; reports `unresolved journal` without mutating | `cmd/config_status_test.go` with stub journal tail. |
| Unit (Go) | Rollback when previous is absent/unverified fails before mutation; rollback never touches the network | Fakes for `Resolver`/`Cache`; assert zero calls to `httpDoer`. |
| Integration (Go) | End-to-end happy path: stub GitHub client → admit → plan → preflight → tx → identity commit → next apply/rollback cycle | `cmd/config_apply_test.go` composes the same fakes; uses `t.TempDir()` for both `XDG_DATA` and `XDG_STATE` roots. |
| Shell | Legacy bridge: `/releases/latest` keeps resolving a `v*` tag while a newer `config-v*` exists | `bash tests/release-bridge_test.sh` against a recorded fixture (no live network). |
| Workflow | `release-config` job uploads `<tag>.tar.zst.sha256`; refuses re-upload with mismatched digest; only runs after `release-cli` has produced a `v*` artifact | `gh api` step + Go CLI test that asserts `gh release upload` error on mismatch. |
| Mutation (Go) | `transaction.Remove` rollback restores a deleted path from backup; entry state goes `EntryRemoved → EntryRestored` | `pkg/installer/transaction/remove_test.go` injects a failure after the remove. |

## Threat Matrix

| Boundary | Applicability | Design response | RED tests |
|---|---|---|---|
| Documentation-like paths (`requirements.txt`, `README.sh`, executable MD/MDX) | N/A — config release does not classify or execute documentation | — | — |
| Git repository selection (`git -C`, relative/absolute) | N/A — config apply does not run Git or select a repo | — | — |
| Commit state (staged, `commit -a`, empty index) | N/A — config apply does not commit | — | — |
| Push state (tracking branch, first push, refspec) | N/A — config apply does not push | — | — |
| PR commands (`--head`, env prefix, composed) | N/A — config apply does not open or modify PRs | — | — |
| Executable-file classification | Applicable — `AdmitArtifact` classifies entry modes; config release manifests may declare a small set of executable bundles (e.g., `home/.local/bin/moonarch`) | Reject any archive entry with `(mode & 0o111) != 0` unless the manifest `binaries` list names the exact path; verify the entry digest matches the manifest | `TestAdmitArtifact_RejectsUndeclaredExecutable`, `_AcceptsDeclaredBinary`, `_RejectsManifestOnlyExecutable` |
| Process integration (kernel lock + atomic rename) | Applicable — kernel advisory lock around mutation; subprocess for CLI self-update | `unix.Flock(LOCK_EX\|LOCK_NB)` on `XDG_STATE/moonarch/lock`; recovery → state-commit → close fd; contention → `ErrLockContended`; subprocess `replace` uses `os.Rename` on the same filesystem | `TestLock_Contention`, `_ReleasedOnProcessExit`; `TestJournal_RecoveryConvergesState`, `_TruncatedTailIsIndeterminate`; `TestEvidenceToken_MismatchRejects`; `TestApply_NeverCallsRunner` (asserts `external.Runner.Run` is never invoked during apply) |

## Journal Phases and Recovery Decisions

`JournalRecord` writes one NDJSON line per phase transition to `XDG_STATE_HOME/moonarch/journal.ndjson`. Recovery walks the tail **under the lock** and returns exactly one of three outcomes:

| Recovery outcome | Triggering tail | Action |
|---|---|---|
| `Committed` | Last record is `state-finalized` *or* `op-end` | Nothing to do. The on-disk identity in `state.json` matches the journal. `config status` proceeds normally. |
| `Uncommitted` | Last record is `mutated` (i.e., at least one target is `EntryMutated`/`EntryRemoved`) **without** a subsequent `committed` or `state-finalized` for the same `op_id` | Restore prior state by replaying `Rollback` against the persisted inventory (`Inventory.Rollback()` already knows how to reverse mutations). If `state-finalized` is present but `op-end` is missing, the work is committed; recovery finalizes the on-disk identity but never re-applies. |
| `Indeterminate` | The tail is **truncated** (NDJSON parse error, missing newline, partial JSON) OR a phase transition violates the legal ordering (`op-end` before `committed`, `state-finalized` before `mutated`, etc.) OR a record references an `op_id` whose inventory is missing | **Block.** `config status` and `config apply` both refuse to mutate. The recovery report names the offending byte offset and the last successfully parsed record. The user must inspect `journal.ndjson` and any orphan inventory under `XDG_STATE/moonarch/orphans/<op_id>/` manually. The CLI never infers success from a truncated tail. |

The legal phase ordering enforced by `Journal.Append` (and re-checked by `Recovery`) is:

```text
op-start → prepared → committing →
  (mutated | committed | state-finalized)* →
  committed → state-finalized → op-end
```

`mutated` records are emitted per target; multiple are allowed. `state-finalized` is the single point at which `state.json` is rotated; a partial rotation is treated as `Uncommitted` and recovery restores prior state without writing a new identity.

## Migration / Rollout

1. **Ship CLI-only self-update first.** Merge `cmd/self_update.go`; rewrite `cmd/update.go` as a thin alias; remove the existing four-stage orchestrator (`runRepository`, `runConfiguration`); remove `MOONARCH_FORCE_REPO`. Add RED tests that assert zero state/journal/lock/inventory/cache touch.
2. **Cut `config-v0.1.0` is not yet published.** The release workflow's `release-config` job is added but remains gated on the bridge. `/releases/latest` keeps resolving the latest `v*` CLI release.
3. **Bridge verification.** `.github/workflows/release.yml`'s `release-config` job refuses to publish a `config-v*` if `/releases/latest` would still resolve a `v*` after upload. `tests/release-bridge_test.sh` runs against recorded fixtures.
4. **Cut `config-v1.0.0`.** Bridge verified → upload `<tag>.tar.zst`, `<tag>.tar.zst.sha256`, and the manifest. The workflow computes the digest, refuses re-upload under the same tag with a different digest, and only after a verified `v*` already exists at `/releases/latest`.
5. **First apply.** On a legacy installation, `config status` reports `legacy/unknown`. The first `config apply config-v1.0.0` reports drift evidence + an evidence token; no mutation occurs.
6. **Authorize + apply.** Re-run with `--authorize-drift <token>`. Apply writes `format_version: 1` inventory with `release:{tag,digest}`; state.json rotates identity.
7. **Rollback dry-run.** `config rollback` runs the same lock/journal/preflight/backup/inventory rules against the previous identity, offline, without network.
8. **`restore` remains machine-state recovery.** Existing `cli/cmd/restore.go` continues to read retained run directories; schema-1 inventories remain decodable.

Rejected migration steps:

- *Auto-roll-forward to the latest config*: rejected — spec `config-release-resolution` requires exact only.
- *Replacing `v0.1.0..v0.3.0` with normalized repackaging*: out of scope per the proposal.
- *Adding a `MOONARCH_FORCE_REPO` escape hatch*: rejected — spec `moonarch-cli-self-update` forbids it.
- *Silent fallback for `themes/current`*: rejected — spec `moonarch-theme-selector` requires explicit replacement.

## Open Questions

None. Every blocker has a concrete path in the existing code (`plan.MutationKind`, `transaction.transaction.go`, `release.NewGitHubClient`) with no ambiguous fork remaining.