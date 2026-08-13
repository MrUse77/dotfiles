# Tasks: MoonArch Versioned Configuration Releases

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~3,200 (range 2,800–3,800) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 8 (work units below) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | CLI-only self-update + alias; remove config-coupled stages | PR 1 | `cd cli && go test -count=1 ./cmd/ ./pkg/release/ -run 'TestSelfUpdate\|TestUpdateAlias\|TestParseConfigVersion'` | N/A — no live release server; equivalence proven by shared-fake alias test | Revert `cli/cmd/{self_update,update,update_flow}.go`, `cli/pkg/release/version.go`, `scripts/install.sh` force-env removal |
| 2 | Exact resolution: `GetByTag`, resolver + sidecar verify, manifest, compat, read-only deps | PR 2 | `cd cli && go test -count=1 ./pkg/release/ -run 'TestGetByTag\|TestResolver\|TestManifest\|TestCompat\|TestDependencyProbe'` | N/A — HTTP mocked via `Client` fakes; no live GitHub in tests | Revert `cli/pkg/release/{client,resolver,manifest,compat,depend}.go` + tests |
| 3 | Fail-closed admission + digest cache retention (threat: executable classification) | PR 3 | `cd cli && go test -count=1 ./pkg/release/ -run 'TestAdmitArtifact\|TestCache'` | N/A — filesystem-only via `t.TempDir()` tar fixtures | Revert `cli/pkg/release/{admit,cache}.go` + tests |
| 4 | Kernel lock, NDJSON journal recovery, atomic state, evidence token (threat: lock/process-exit, truncation, evidence mismatch) | PR 4 | `cd cli && go test -count=1 ./pkg/release/ -run 'TestLock\|TestJournal\|TestState\|TestEvidence'` | `go test ./pkg/release/ -run TestLock -v` — real child processes prove flock release on exit | Revert `cli/pkg/release/{lock_unix,journal,state,identity,errors}.go` + tests |
| 5 | Planner/transaction `Remove`; inventory release provenance | PR 5 | `cd cli && go test -count=1 ./pkg/installer/... -run 'TestPlanRemove\|TestRemove\|TestInventory'` | N/A — docker `bash test.sh` covers installer TUI; Remove semantics unit-proven | Revert `cli/pkg/installer/plan/{plan,planner,source_binding}.go`, `transaction/{transaction,inventory}.go` + tests |
| 6 | `config apply` + `status` + theme boundary + drift auth (threat: runner-not-called) | PR 6 | `cd cli && go test -count=1 ./cmd/ -run 'TestConfigApply\|TestConfigStatus\|TestThemePhase\|TestApply_NeverCallsRunner'` | `go test ./cmd/ -run TestConfigApply_EndToEnd` — full apply pipeline over `t.TempDir()` XDG roots | Revert `cli/cmd/{config.go,theme_phase.go}` + tests |
| 7 | Offline `config rollback` transaction | PR 7 | `cd cli && go test -count=1 ./cmd/ -run 'TestConfigRollback'` | N/A — no-network proven by zero-`httpDoer` fakes; real GitHub never contacted | Revert rollback wiring in `cli/cmd/config.go` + `config_rollback_test.go` |
| 8 | Publication workflow, legacy bridge, install guard, CI, docs | PR 8 | `bash tests/release-bridge_test.sh` | `bash tests/release-bridge_test.sh` against recorded fixtures (no live network) | Revert `.github/workflows/{release,ci}.yml`, `scripts/install.sh` guard, `tests/release-bridge_test.sh`, `RELEASING.md` |

## Phase 1: Release Package Foundations (`cli/pkg/release`)

- [x] 1.1 `cli/pkg/release/version.go`: add `ParseConfigVersion(raw) (VersionIdentity, error)` — exact `config-vMAJOR.MINOR.PATCH` only; reject latest/channel/prerelease/`v*`/bare `config`; keep `CompareVersions` for `v*`. Verify: table-driven `version_test.go` incl. strict-SemVer 1.10.0 vs 1.9.0. Dep: none.
- [x] 1.2 `cli/pkg/release/manifest.go`: `Manifest` + `CatalogEntry{Path,Digest,Mode,Kind,Executable}` + `binaries` list JSON parser. Verify: `manifest_test.go` (schema, required fields). Dep: none.
- [x] 1.3 `cli/pkg/release/errors.go`: add `ErrLockContended`, `ErrArtifactRejected`, `ErrIndeterminateJournal`, `ErrNoPreviousIdentity`, `ErrOfflineArtifactMissing`, `ErrUnboundForce`. Verify: `go build ./pkg/release/`. Dep: none.
- [x] 1.4 `cli/pkg/release/client.go`: extend `Client` with `GetByTag(ctx, tag)` — rejects non-`config-v*`, never falls back; keep `Latest()` for self-update. Verify: `client_test.go` (non-exact rejected; missing tag no fallback). Dep: 1.1.

## Phase 2: CLI-Only Self-Update and Alias

- [x] 2.1 RED `cli/cmd/self_update_test.go`: fakes assert `self update`/`update` make zero state/journal/lock/inventory/cache calls and both reject `config-v*`. Verify: `go test ./cmd/ -run TestSelfUpdate -count=1` (fails before impl). Dep: 1.1.
- [x] 2.2 `cli/cmd/self_update.go`: canonical CLI-only runner — `release.Latest` → `BinaryAsset`(amd64|arm64) → `ChecksumAsset` → SHA-256 verify → chmod 0755 → `os.Rename` to `~/.local/bin/moonarch-cli`; reject `config-v*`. Verify: `go test ./cmd/ -run TestSelfUpdate`. Dep: 2.1, 1.4.
- [x] 2.3 `cli/cmd/update.go` + `update_flow.go`: rewrite `update.go` as thin alias into the self-update runner; delete `runRepository`/`runConfiguration` stages; remove `MOONARCH_FORCE_REPO` reads. Verify: `go build ./cmd/...`. Dep: 2.2.
- [x] 2.4 `scripts/install.sh`: remove `MOONARCH_FORCE_REPO`; keep `latest_release()` on `/releases/latest`. Verify: `bash -n scripts/install.sh`. Dep: none.
- [x] 2.5 `cli/cmd/update_alias_test.go`: byte-identical outcome for `self update` vs `update` against shared fakes; both reject `config-v*`. Verify: `go test ./cmd/ -run TestUpdateAlias`. Dep: 2.3.

## Phase 3: Resolution, Admission, Retention (`cli/pkg/release`)

- [x] 3.1 `cli/pkg/release/resolver.go`: `Resolver.Resolve(ctx, tag)` — `GetByTag` → fetch `<tag>.tar.zst` + `<tag>.tar.zst.sha256` sidecar → verify whole-artifact SHA-256 → stage `XDG_DATA/moonarch/staging/<digest>/`. Verify: sidecar match/mismatch cases. Dep: 1.4, 1.2.
- [x] 3.2 RED `cli/pkg/release/admit_test.go`: threat — `TestAdmitArtifact_RejectsUndeclaredExecutable`, `_AcceptsDeclaredBinary`, `_RejectsManifestOnlyExecutable` (`t.TempDir()` tar fixtures; cache unchanged). Verify: `go test ./pkg/release/ -run TestAdmitArtifact` (fails before impl). Dep: 1.2.
- [x] 3.3 `cli/pkg/release/admit.go`: fail-closed extractor — reject `..`/absolute/escape/dup-normalized/unsupported types/manifest-mismatch; per-entry digest verify; typed errors; never promotes cache. Verify: `go test ./pkg/release/ -run TestAdmitArtifact`. Dep: 3.2.
- [x] 3.4 `cli/pkg/release/cache.go`: `Cache.Promote` (atomic rename → `artifacts/<digest>/`), `Lookup`, `Retain` (orphan-only cleanup). Verify: interrupted acquisition preserves entries; `Retain` keeps current+previous. Dep: 3.3.
- [x] 3.5 `cli/pkg/release/compat.go`: manifest schema + `cli_compat_range` check → typed `CompatibilityError`; no mutation. Verify: `compat_test.go` (unsupported schema/range fail). Dep: 1.2.
- [x] 3.6 `cli/pkg/release/depend.go`: read-only `DependencyProbe.Probe` → `DependencyResult`; never shells out a privileged command. Verify: `depend_test.go` — stub Satisfied true/false/unknown; assert no `CommandSpec` executed. Dep: 1.2.

## Phase 4: State, Lock, Journal, Evidence (`cli/pkg/release`)

- [x] 4.1 `cli/pkg/release/state.go`: `State{Current,Previous *Identity, LastCompletedRunID}` + `WriteAtomic` (tmp + fsync + rename). Verify: round-trip write/read. Dep: 1.1.
- [x] 4.2 RED `cli/pkg/release/lock_unix_test.go`: threat — `TestLock_Contention` + `TestLock_ReleasedOnProcessExit` (child cmd holds/leaves flock). Verify: `go test ./pkg/release/ -run TestLock` (fails before impl). Dep: 1.3.
- [x] 4.3 `cli/pkg/release/lock_unix.go`: `Lock.Acquire` via `unix.Flock(fd, LOCK_EX|LOCK_NB)`; contention → `ErrLockContended`; release closes fd. Verify: `go test ./pkg/release/ -run TestLock -count=1`. Dep: 4.2.
- [x] 4.4 RED `cli/pkg/release/journal_test.go`: threat — `TestJournal_RecoveryConvergesState`, `TestJournal_TruncatedTailIsIndeterminate`; truncation at every phase boundary + illegal ordering. Verify: `go test ./pkg/release/ -run TestJournal` (fails before impl). Dep: 1.3.
- [x] 4.5 `cli/pkg/release/journal.go`: NDJSON appender, phases `op-start→prepared→committing→mutated→committed→state-finalized→op-end`; `Recovery()` → Committed/Uncommitted/Indeterminate. Verify: `go test ./pkg/release/ -run TestJournal -count=1`. Dep: 4.4.
- [x] 4.6 RED `cli/pkg/release/identity_test.go`: threat — `TestEvidenceToken_MismatchRejects`; canonical-JSON determinism golden; observation-set change rejects. Verify: `go test ./pkg/release/ -run TestEvidence` (fails before impl). Dep: none.
- [x] 4.7 `cli/pkg/release/identity.go`: `Identity`, `EvidenceObservation`, `ComputeEvidenceToken` (hex SHA-256 of canonical JSON), `VerifyEvidenceToken`. Verify: `go test ./pkg/release/ -run TestEvidence`. Dep: 4.6.

## Phase 5: Planner/Transaction `Remove`; Inventory Provenance

- [x] 5.1 `cli/pkg/installer/plan/plan.go`: add `MutationKind Remove = "remove"`; Remove targets carry empty Source/ResolvedSource/SourceDigest. Verify: `go test ./pkg/installer/plan/ -run TestPlanRemove`. Dep: none.
- [x] 5.2 `cli/pkg/installer/plan/source_binding.go`: `buildSourceBinding` returns zero-value binding + `Kind=""` for Remove. Verify: no source digest required for Remove. Dep: 5.1.
- [x] 5.3 `cli/pkg/installer/plan/planner.go`: discoverer emits installed∪desired; `buildTargets` closes plan with explicit Remove for installed∉desired; `themes/current` excluded (separate post-discovery phase). Verify: `removal_test.go` — installed A,B + desired B,C → Remove A, Replace B, Create C. Dep: 5.2.
- [x] 5.4 `cli/pkg/installer/transaction/transaction.go`: `mutateTarget` `plan.Remove` branch — absent → skip `EntrySkipped(absent)`; else `backupTarget` → `fs.RemoveAll` → `EntryRemoved`; later failure → `Rollback` `restoreFromBackup`. Verify: `remove_test.go` — unchanged removed, absent no-op, failure restores, `EntryRemoved→EntryRestored`. Dep: 5.3.
- [x] 5.5 `cli/pkg/installer/transaction/inventory.go`: add `ReleaseProvenance{Tag,Digest}` (`json:"release,omitempty"`) + `EntryRemoved`; `FormatVersion` stays 1. Verify: `inventory_test.go` — schema-1 golden decode → nil provenance; `restoreCandidates`/`Restore` unaffected (backup-inventory spec). Dep: 5.4.

## Phase 6: Config Commands — Apply, Status, Rollback, Theme, Drift

- [x] 6.1 `cli/cmd/config.go`: `config` parent + `apply config-vX.Y.Z`, `rollback`, `status`; flags `--authorize-drift <hex>`, `--theme-replace <id>`, `--offline`, `--json`. Verify: `go build ./cmd/...`. Dep: 1.3, 4.1.
- [x] 6.2 RED `cli/cmd/config_apply_test.go`: threat — `TestApply_NeverCallsRunner` (external.Runner.Run never invoked during apply). Verify: `go test ./cmd/ -run TestApply_NeverCallsRunner` (fails before impl). Dep: 4.3, 4.5, 4.7.
- [x] 6.3 `cli/cmd/theme_phase.go`: read `~/.local/share/moonarch/themes/current` via `unix.Readlink`; validate relative link inside desired bundle set; abort unless valid `--theme-replace <id>`; atomic rewrite via sibling temp rename(2). Verify: `theme_phase_test.go` — valid preserved; missing aborts; replace rewrites; escaping link fails. Dep: none.
- [x] 6.4 `cli/cmd/config.go` apply flow: lock → recovery (indeterminate → exit 2) → resolve/admit/compat/depend → Promote → baseline (`legacy/unknown`; optional FormatVersion==1 inventory baseline) → planner → theme phase → journal `op-start` → whole-plan preflight (evidence+token; `--authorize-drift` re-scan) → `transaction.Prepare`/`Commit` → journal `state-finalized` → rotate identity + atomic `state.json` → `op-end`. Verify: `go test ./cmd/ -run TestConfigApply` — preflight-failure-no-change, lock-contention, drifted-removal-blocked. Dep: 3.1–3.6, 4.3, 4.5, 4.7, 5.5, 6.2, 6.3.
- [x] 6.5 `cli/cmd/config.go` status flow: under lock after recovery — `legacy/unknown` when nil; current+previous+retention; unresolved journal reported without mutation; retained-artifact scan (orphan count). Verify: `config_status_test.go` with stub journal tail. Dep: 4.1, 4.5, 6.4.
- [x] 6.6 `cli/cmd/config.go` rollback flow: lock → recovery → state check (current+previous verified, previous artifact present, else `ErrNoPreviousIdentity`/`ErrOfflineArtifactMissing`) → plan previous digest offline → theme phase → preflight → tx → swap identities. Verify: `config_rollback_test.go` — zero `httpDoer` calls; absent previous aborts pre-mutation. Dep: 6.4, 4.7.
- [x] 6.7 Integration `cli/cmd/config_apply_test.go`: end-to-end happy path — stub GitHub client → admit → plan → preflight → tx → identity commit → apply/rollback cycle with `t.TempDir()` XDG_DATA+XDG_STATE. Verify: `go test ./cmd/ -run TestConfigApply_EndToEnd`. Dep: 6.4, 6.6.

## Phase 7: Publication, Legacy Bridge, Docs

- [x] 7.1 `.github/workflows/release.yml`: add `release-config` job gated on legacy bridge check (`/releases/latest` must resolve `v*` after upload); materialize pinned submodules; deterministic `tar.zst`; compute digest; write `<tag>.tar.zst.sha256`; refuse upload when digest already exists. Verify: workflow lint + bridge test. Dep: none.
- [x] 7.2 RED `tests/release-bridge_test.sh`: `/releases/latest` keeps resolving `v*` while newer `config-v*` exists; same-tag re-publication rejected. Verify: `bash tests/release-bridge_test.sh` (recorded fixtures, no live network). Dep: 7.1.
- [x] 7.3 `scripts/install.sh`: hard guard — fail if `/releases/latest` resolves `config-v*`. Verify: `bash -n scripts/install.sh` + bridge test. Dep: 2.4, 7.2.
- [x] 7.4 `.github/workflows/ci.yml`: invoke `bash tests/release-bridge_test.sh` and `cd cli && go test -race -count=1 ./...`. Verify: CI run. Dep: 7.2, all code phases.
- [x] 7.5 `RELEASING.md`: document bridge requirement, `<tag>.tar.zst.sha256` sidecar, immutable-identity rule, publication gate. Verify: read-through. Dep: 7.1.
