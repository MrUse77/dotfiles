```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:3503a73c97a6b345aeb3644635dc04be4d909d0bddaa53a48b03963c15a3a067
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 20/20
scenarios: 62/62
test_command: cd cli && go test -race -count=1 ./...
test_exit_code: 0
test_output_hash: sha256:021d0af5e3fe31f723f1f91bbf34458e6d6a722f2b8b24251f608c2d6b978984
build_command: cd cli && go build ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: moonarch-versioned-config-releases
**Version**: N/A (OpenSpec change, phases 1–7)
**Mode**: Strict TDD (executable shell/workflow behavior; cli/ subproject strict_tdd=true)

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 39 |
| Tasks complete | 39 |
| Tasks incomplete | 0 |

### Build & Tests Execution
**Build**: ✅ Passed
```text
cd cli && go build ./...  -> exit 0 (empty output, hash e3b0c442…)
cd cli && go vet ./...     -> exit 0 (empty output, hash e3b0c442…)
gofmt -l .                 -> no unformatted files
```

**Tests**: ✅ 8 packages passed / 0 failed (uncached, race detector)
```text
cd cli && go test -race -count=1 ./...  -> exit 0
?   	github.com/MrUse77/dots-cli	[no test files]
ok  	github.com/MrUse77/dots-cli/cmd	1.319s
ok  	github.com/MrUse77/dots-cli/pkg/installer	1.024s
ok  	github.com/MrUse77/dots-cli/pkg/installer/external	2.034s
ok  	github.com/MrUse77/dots-cli/pkg/installer/plan	1.036s
ok  	github.com/MrUse77/dots-cli/pkg/installer/report	1.013s
ok  	github.com/MrUse77/dots-cli/pkg/installer/transaction	1.195s
ok  	github.com/MrUse77/dots-cli/pkg/installer/ui	1.020s
?   	github.com/MrUse77/dots-cli/pkg/installer/ui/menu	[no test files]
ok  	github.com/MrUse77/dots-cli/pkg/release	2.122s
```
Focused suite: `cd cli && go test -count=1 ./cmd/ -run 'TestConfigApply|TestConfigStatus|TestThemePhase|TestApply_NeverCallsRunner|TestConfigRollback|TestConfigIndeterminate'` -> exit 0 (ok .../cmd 0.015s). Runtime harness: `TestConfigApply_EndToEnd` -> PASS.

**Shell/workflow tests**: ✅
```text
bash tests/release-bridge_test.sh  -> exit 0 (PASS: legacy bridge fixtures, immutable identity, and installer guard)
bash -n scripts/install.sh         -> exit 0
bash -n tests/release-bridge_test.sh -> exit 0
actionlint v1.7.12 .github/workflows/*.yml -> exit 0 (no diagnostics)
YAML parse (PyYAML 6.0.3): release.yml, ci.yml, openspec/config.yaml, cli/openspec/config.yaml -> OK
markdownlint-cli2 (repo config .github/.markdownlint.jsonc): RELEASING.md -> 0 issues (exit 0)
```

**Coverage** (informational; `go test -cover`, threshold 0):
| Package | Coverage |
|---------|----------|
| pkg/release | 79.0% |
| cmd | 70.5% |
| pkg/installer/plan | 81.8% |
| pkg/installer/transaction | 69.7% |

### Spec Compliance Matrix
| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| config-release-publication: Immutable Self-Contained Publication | Complete release is published | `TestConfigApply_EndToEnd` (full archive: manifest, catalog, digests, sidecar, admit, apply) + release.yml `release-config` job (deterministic tar.zst, manifest, sidecar) | ✅ COMPLIANT |
| config-release-publication: Immutable Self-Contained Publication | Incomplete release is rejected | `TestAdmitArtifact_RejectsMissingManifestEntry`, `_RejectsMissingManifest`, `_RejectsDigestMismatch`, `_RejectsUndeclaredArchiveEntry`; workflow submodule/tag validation | ✅ COMPLIANT |
| config-release-publication: Immutable Self-Contained Publication | Existing identity cannot be replaced | `tests/release-bridge_test.sh` `assert_new_identity` (same-tag same/different-digest rejected); `TestCache_Promote_IsIdempotent` | ✅ COMPLIANT |
| config-release-publication: Legacy Bridge Gates Publication | Legacy client remains functional | `verify_bridge` latest-cli.json (v1.2.3 + assets) while newer config-v2.0.0 exists; installer guard accepts v* | ✅ COMPLIANT |
| config-release-publication: Legacy Bridge Gates Publication | Missing bridge blocks publication | `verify_bridge` newer-config.json rejected; workflow `needs.legacy-bridge.outputs.verified`; install.sh config-v* guard | ✅ COMPLIANT |
| config-release-resolution: Exact Resolution | Exact release exists | `TestGitHubClient_GetByTag`, `TestResolver_Resolve_Success`, `TestConfigApply_EndToEnd` | ✅ COMPLIANT |
| config-release-resolution: Exact Resolution | Non-exact selector rejected | `TestParseConfigVersion`, `TestGitHubClient_GetByTag_RejectsNonConfigSelectorBeforeRequest`, `TestConfigApplyAcceptsOnlyExactRelease` | ✅ COMPLIANT |
| config-release-resolution: Exact Resolution | Missing exact release no fallback | `TestGitHubClient_GetByTag_MissingTagHasNoFallback`, `TestResolver_Resolve_MissingArchiveAsset` | ✅ COMPLIANT |
| config-release-resolution: Admission Fails Closed | Valid artifact admitted | `TestAdmitArtifact_ValidArtifactIsAdmitted` | ✅ COMPLIANT |
| config-release-resolution: Admission Fails Closed | Malformed/untrusted rejected, cache unchanged | `TestAdmitArtifact_RejectsTraversalPath/AbsolutePath/SymlinkEscape/DuplicateNormalizedPath/UnsupportedFileType/UndeclaredArchiveEntry/MissingManifestEntry/DigestMismatch/KindMismatch` | ✅ COMPLIANT |
| config-release-resolution: Compatibility & Dependencies | Compatibility checks pass | `TestCheckCompatibility_PassesForSupportedSchemaAndRange`, `_PassesAtRangeBoundary`, `TestConfigApply_EndToEnd` | ✅ COMPLIANT |
| config-release-resolution: Compatibility & Dependencies | Compatibility check fails | `TestCheckCompatibility_RejectsUnsupportedSchema/UnsatisfiedRange/InvalidRange/InvalidCliVersion`, `TestConfigApply_CompatibilityFailureDoesNotPromoteArtifact` | ✅ COMPLIANT |
| config-release-resolution: Offline Availability | Interrupted acquisition preserves cache | `TestCache_Promote_InterruptedLeavesEntriesIntact` | ✅ COMPLIANT |
| config-release-resolution: Offline Availability | Current/previous work offline | `TestConfigRollback_UsesRetainedArtifactOfflineAndSwapsIdentities`, `TestConfigApply_EndToEnd` (zero resolver calls) | ✅ COMPLIANT |
| config-release-resolution: Offline Availability | Cleanup preserves protected | `TestCache_Retain_ProtectsCurrentAndPrevious`, `_CurrentEqualsPreviousKeptOnce` | ✅ COMPLIANT |
| config-release-operations: Installed Status | Legacy installation unknown | `TestConfigStatus_ReportsLegacyUnknownWithoutPreviousIdentity` | ✅ COMPLIANT |
| config-release-operations: Installed Status | Installed identities reported | `TestConfigStatus_ReportsVerifiedRetentionAndOrphansAsJSON` | ✅ COMPLIANT |
| config-release-operations: Exact Apply | Exact apply succeeds | `TestConfigApply_EndToEnd` | ✅ COMPLIANT |
| config-release-operations: Exact Apply | Preflight failure no changes | `TestConfigApply_PreflightFailureMakesNoChanges`, `_DriftedRemovalBlocked` | ✅ COMPLIANT |
| config-release-operations: Exact Apply | Concurrent mutation excluded | `TestConfigApply_LockContentionStopsBeforeResolution`, `TestLock_Contention`, `_ReleasedOnProcessExit` | ✅ COMPLIANT |
| config-release-operations: Exact Apply | Apply transaction fails, state restored | `TestConfigApply_TransactionFailureRollsBackBeforeIdentityCommit` | ✅ COMPLIANT |
| config-release-operations: Drift Authorization | First drift aborts | `TestConfigApply_PrintsEveryDriftObservationAndToken`, `_DriftedRemovalBlocked` | ✅ COMPLIANT |
| config-release-operations: Drift Authorization | Matching authorization | `TestConfigApply_DriftAuthorizationBindsExactCandidateAndObservations`, `TestEvidenceToken_FreshScanMatches` | ✅ COMPLIANT |
| config-release-operations: Drift Authorization | Invalid authorization rejected | `TestEvidenceToken_MismatchRejects`, `TestConfigApply_UnboundAuthorizationCannotBypassCleanPreflight` | ✅ COMPLIANT |
| config-release-operations: Journal Recovery | Crash before commit restores | `TestConfigApply_RecoveryRollsBackUncommittedInventory`, `TestJournal_RecoveryConvergesState` | ✅ COMPLIANT |
| config-release-operations: Journal Recovery | Crash after commit finalizes identity | `TestConfigApply_RecoveryFinalizesCommittedIdentityWithoutReapply` | ✅ COMPLIANT |
| config-release-operations: Journal Recovery | Indeterminate outcome blocks | `TestJournal_TruncatedTailIsIndeterminate`, `TestConfigApply_IndeterminateJournalBlocksResolution`, `TestConfigIndeterminateJournalUsesExitCodeTwo` | ✅ COMPLIANT |
| config-release-operations: Offline Rollback | Offline rollback succeeds | `TestConfigRollback_UsesRetainedArtifactOfflineAndSwapsIdentities`, `TestConfigApply_EndToEnd` | ✅ COMPLIANT |
| config-release-operations: Offline Rollback | Previous release unavailable | `TestConfigRollback_AbsentPreviousFailsBeforeMutationAndNetwork` | ✅ COMPLIANT |
| config-release-operations: Offline Rollback | Local drift blocks rollback | `TestConfigApply_DriftedRemovalBlocked` (shared `checkConfigPreflight` path also used by rollback) | ✅ COMPLIANT |
| moonarch-cli-self-update: Canonical + Alias | Canonical checks for CLI update | `TestSelfUpdate_ReplacesBinaryAndStaysConfigurationNeutral` | ✅ COMPLIANT |
| moonarch-cli-self-update: Canonical + Alias | Legacy alias equivalent | `TestUpdateAlias_ByteIdenticalOutcome`, `_AlreadyCurrentOutcomeIsIdentical`, `_DevBuildIsByteIdentical` | ✅ COMPLIANT |
| moonarch-cli-self-update: Canonical + Alias | Config selector rejected | `TestSelfUpdate_ConfigSelectorRejectedWithoutMutation`, `TestUpdateAlias_BothRejectConfigSelectorWithSameError` | ✅ COMPLIANT |
| moonarch-cli-self-update: Verified Replacement | Verified newer binary replaces | `TestSelfUpdate_ReplacesBinaryAndStaysConfigurationNeutral`, `TestAtomicReplacer_Success` | ✅ COMPLIANT |
| moonarch-cli-self-update: Verified Replacement | Already current skips download | `TestSelfUpdate_AlreadyCurrentSkipsDownload`, `TestUpdateAlias_AlreadyCurrentOutcomeIsIdentical` | ✅ COMPLIANT |
| moonarch-cli-self-update: Verified Replacement | Release discovery fails | `TestSelfUpdate_DiscoveryFailureLeavesNoTrace` | ✅ COMPLIANT |
| moonarch-cli-self-update: Verified Replacement | Candidate verification fails | `TestSelfUpdate_ChecksumFailurePreservesExistingBinary`, `TestAtomicReplacer_ChecksumMismatchRemovesTemp` | ✅ COMPLIANT |
| moonarch-cli-self-update: Verified Replacement | Binary replacement fails | `TestAtomicReplacer_RenameFailureRemovesTempAndPreservesOld` | ✅ COMPLIANT |
| moonarch-cli-self-update: Configuration-Neutral | Successful update preserves state | `assertNoConfigurationSideEffects` in `TestSelfUpdate_ReplacesBinaryAndStaysConfigurationNeutral` | ✅ COMPLIANT |
| moonarch-cli-self-update: Configuration-Neutral | Force env cannot enable config stages | `TestSelfUpdate_ForceEnvCannotEnableConfigurationStages` (sets `MOONARCH_FORCE_REPO`, asserts zero XDG writes) | ✅ COMPLIANT |
| moonarch-cli-self-update: Configuration-Neutral | Config change requires exact apply | `TestConfigApplyAcceptsOnlyExactRelease`, `TestSelfUpdate_ConfigSelectorRejectedWithoutMutation` | ✅ COMPLIANT |
| installation-transaction: Retired Targets Deleted | Unchanged retired target removed | `TestPlanRemove_ClosesUnionOfInstalledAndDesired`; `remove_test.go` (EntryRemoved) | ✅ COMPLIANT |
| installation-transaction: Retired Targets Deleted | Retired target already absent skipped | `TestPlanRemove_AbsentRetiredDestinationIsCarried`; `remove_test.go` (EntrySkipped) | ✅ COMPLIANT |
| installation-transaction: Retired Targets Deleted | Removal rolled back | `remove_test.go` (EntryRemoved → EntryRestored) | ✅ COMPLIANT |
| installation-transaction: Conservative Baselines | Completed inventory establishes baseline | `TestConfigApply_CompletedSchemaOneInventoryEstablishesBaseline` | ✅ COMPLIANT |
| installation-transaction: Conservative Baselines | Missing baseline preserves unknown paths | `TestConfigApply_LegacyWithoutBaselinePreservesUnknownPaths` | ✅ COMPLIANT |
| installation-transaction: Discovery Completes | Full target set during planning | `TestPlanRemove_ClosesUnionOfInstalledAndDesired` | ✅ COMPLIANT |
| installation-transaction: Discovery Completes | MoonArch runtime trees planned, themes excluded | `TestInstallDiscovererPlansMoonArchRuntimeTrees`, `TestConfigApply_PlannerUnionsDesiredAndRetiredAndExcludesThemeCurrent` | ✅ COMPLIANT |
| installation-transaction: Discovery Completes | Discovery failure blocks execution | `TestInstallDiscovererFailsWhenMoonArchRuntimeIsMissing`; `discoverConfigTargets`/`validateConfigPlan` error paths | ✅ COMPLIANT |
| installation-transaction: Drift Before Mutation | No drift detected | `TestConfigApply_EndToEnd` (clean full-plan preflight) | ✅ COMPLIANT |
| installation-transaction: Drift Before Mutation | Material drift reported | `TestConfigApply_PrintsEveryDriftObservationAndToken` | ✅ COMPLIANT |
| installation-transaction: Drift Before Mutation | New-target pre-state checked | `TestConfigApply_EndToEnd` (creation-pre class in `checkConfigPreflight`) | ✅ COMPLIANT |
| installation-transaction: Drift Before Mutation | Removed target local drift | `TestConfigApply_DriftedRemovalBlocked` | ✅ COMPLIANT |
| installation-transaction: Drift Before Mutation | Matching authorization permits reviewed drift | `TestConfigApply_DriftAuthorizationBindsExactCandidateAndObservations` | ✅ COMPLIANT |
| installation-transaction: Drift Before Mutation | Changed drift rejects authorization | `TestEvidenceToken_MismatchRejects` | ✅ COMPLIANT |
| backup-inventory: Additive Provenance | Apply records verified provenance | `TestInventory_ReleaseProvenance_RoundTrip`, `TestConfigApply_CommitOrdersJournalStateAndThemeConservatively` | ✅ COMPLIANT |
| backup-inventory: Additive Provenance | Legacy inventory readable, unknown | `TestInventory_Schema1Decode_NilReleaseProvenance`, `_ReleaseProvenance_OmittedWhenNil` | ✅ COMPLIANT |
| backup-inventory: Additive Provenance | Historical run restores | `TestConfigApply_CompletedSchemaOneInventoryEstablishesBaseline` (schema-1 baseline); `restore` ignores provenance | ✅ COMPLIANT |
| moonarch-theme-selector: Mutable Selection Preserved | Selected theme remains available | `TestThemePhase_PreservesValidRelativeSelection`, `TestConfigApply_EndToEnd` (assertThemeLink tokyo-night) | ✅ COMPLIANT |
| moonarch-theme-selector: Mutable Selection Preserved | Selected theme unavailable aborts | `TestThemePhase_MissingBundleRequiresReplacement` | ✅ COMPLIANT |
| moonarch-theme-selector: Mutable Selection Preserved | Explicit replacement permits apply | `TestThemePhase_ReplacementRewritesAndRollsBackRelativeLink` | ✅ COMPLIANT |
| moonarch-theme-selector: Mutable Selection Preserved | Unsafe/invalid selection blocks | `TestThemePhase_RejectsEscapingLinkWithoutMutation` | ✅ COMPLIANT |

**Compliance summary**: 62/62 scenarios compliant (all covering tests passed at runtime).

### Correctness (Static Evidence)
| Requirement | Status | Notes |
|------------|--------|-------|
| Immutable Self-Contained Configuration Publication | ✅ Implemented | release.yml `release-config` job: materialized submodules, deterministic tar.zst, manifest, sidecar, same-tag refusal |
| Legacy Client Bridge Gates Configuration Publication | ✅ Implemented | `legacy-bridge` job sets/verifies latest v*, gates `release-config`; install.sh config-v* guard |
| Exact Configuration Release Resolution | ✅ Implemented | `ParseConfigVersion`, `GetByTag` reject non-exact, no fallback |
| Artifact Admission Fails Closed | ✅ Implemented | `ArtifactAdmitter.Admit` fail-closed extraction, per-entry digest + executable classification |
| Compatibility and External Dependencies Are Verified | ✅ Implemented | `CheckCompatibility` + read-only `CheckDependencies`; no mutation |
| Current and Previous Artifacts Remain Available Offline | ✅ Implemented | `ArtifactCache.Promote/Lookup/Retain` digest-addressed, protected current/previous |
| Installed Status Uses Verified Identities | ✅ Implemented | `config status` under lock, legacy/unknown, retention/orphans, journal disclosure |
| Exact Apply Is Serialized and Fail-Closed | ✅ Implemented | flock + journal + preflight + tx + atomic state rotation |
| Evidence-Bound Drift Authorization | ✅ Implemented | `ComputeEvidenceToken`/`VerifyEvidenceToken` canonical JSON, `--authorize-drift` re-scan |
| Journal Recovery Converges State | ✅ Implemented | NDJSON phases, committed/uncommitted/indeterminate recovery under lock |
| Rollback Is a New Offline Transaction | ✅ Implemented | reuses lock/journal/preflight/tx on retained previous artifact; no network |
| Canonical Command and Alias Share One CLI-Only Contract | ✅ Implemented | `self update` canonical; `update` thin alias to same runner; `updateArgs` rejects config-v* |
| New Binaries Are Verified Before Atomic Replacement | ✅ Implemented | Latest → BinaryAsset/ChecksumAsset → SHA-256 verify → chmod → atomic rename |
| Self-Update Is Configuration-Neutral | ✅ Implemented | no checkout/planner/state/journal/lock/inventory; `MOONARCH_FORCE_REPO` not read |
| Retired Managed Targets Are Explicit Deletions | ✅ Implemented | `plan.Remove` + transaction backup/delete/EntryRemoved, absent→EntrySkipped, rollback restores |
| Conservative Legacy Baselines | ✅ Implemented | schema-1 completed inventory baseline; absent baseline = untrusted drift, no inferred deletion |
| Managed-Target Discovery Completes Before Execution | ✅ Implemented | discoverer union closed into plan; themes/current excluded |
| Plan Drift Detection Before Mutation | ✅ Implemented | whole-plan preflight observations, evidence-bound authorization, fresh re-scan |
| Release Provenance Is Additive | ✅ Implemented | `ReleaseProvenance` `release,omitempty`; schema-1 decodes nil; restore unaffected |
| Configuration Apply Preserves Mutable Theme Selection | ✅ Implemented | theme phase: relative-link validation, abort without `--theme-replace`, atomic rewrite |

### Coherence (Design)
| Decision | Followed? | Notes |
|----------|-----------|-------|
| D1 `ParseConfigVersion` exact only; SemVer for v* | ✅ Yes | `version.go` regex + strict SemVer; rejects latest/channel/prerelease/v*/bare config |
| D2 One CLI-only pipeline shared by self update + update | ✅ Yes | `self_update.go` runner; `update.go` thin alias |
| D3 Whole-artifact sidecar `<tag>.tar.zst.sha256` | ✅ Yes | `resolver.go` verifies sidecar before staging |
| D4 Admission phases fail-closed | ✅ Yes | `acquireConfigArtifact`: resolve → admit → manifest → compat → deps → promote |
| D5 Legacy bridge as real workflow step | ✅ Yes | `legacy-bridge` job + `needs.legacy-bridge.outputs.verified` gate |
| D6 Immutable identity; refuse same-tag re-publication | ✅ Yes | `assert-new-identity` gate + cache idempotent promote |
| D7 `unix.Flock` LOCK_EX|LOCK_NB on lock file | ✅ Yes | `lock_unix.go`; contention → `ErrLockContended`; release on close |
| D8 Evidence token = hex SHA256 canonical JSON | ✅ Yes | `identity.go`; nil observations normalized to empty array |
| D9 NDJSON journal phases | ✅ Yes | `journal.go`; legal-ordering enforcement + truncation → Indeterminate |
| D10 Additive `ReleaseProvenance` | ✅ Yes | `inventory.go` `release,omitempty`; FormatVersion stays 1 |
| D11 XDG split payload/state | ✅ Yes | `defaultConfigPaths` XDG_DATA_HOME/XDG_STATE_HOME |
| D12 themes/current preserved, explicit replacement | ✅ Yes | `theme_phase.go` post-planning phase; excluded from managed targets |
| D13 `plan.Remove` mutation kind | ✅ Yes | `plan.go` Remove; empty source binding |
| D14 Apply uses only Prepare/Commit, no Runner | ✅ Yes | `validateConfigPlan` forbids external actions; `TestApply_NeverCallsRunner` |
| D15 Rollback = new offline transaction on previous | ✅ Yes | `Rollback` uses retained artifact, offline flag, swaps identities |
| D16 Status reads under lock after recovery | ✅ Yes | `Status` acquires lock, runs recovery disclosure, never mutates |

### TDD Compliance
| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | TDD Cycle Evidence table present in apply-progress |
| All tasks have tests | ✅ | 12/12 evidence rows reference test files that exist on disk |
| RED confirmed (tests exist) | ✅ | config_test.go, config_apply_test.go, theme_phase_test.go, config_status_test.go, config_rollback_test.go, recovery_test.go, release-bridge_test.sh all present |
| GREEN confirmed (tests pass) | ✅ | Focused suite, full race suite, and bridge harness all PASS in this verification run |
| Triangulation adequate | ✅ | Multiple cases per task (drift, recovery, removal, theme, alias, admission) |
| Safety Net for modified files | ✅ | Full package suites PASS; prior command baselines recorded in apply-progress |

**TDD Compliance**: 6/6 checks passed

### Test Layer Distribution
| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit (Go) | ~160 test functions | pkg/release/*_test.go, pkg/installer/*_test.go | go test |
| Integration (Go runtime harness) | ~25 | cmd/config_apply_test.go, config_status_test.go, config_rollback_test.go, theme_phase_test.go | go test |
| Shell integration (recorded fixtures) | 8 assertions + fixture checks | tests/release-bridge_test.sh | bash, fake curl |
| Workflow | static lint | .github/workflows/*.yml | actionlint, PyYAML |
| E2E (live GitHub) | skipped by design | — | deterministic in-memory release.Client |

### Changed File Coverage
| File | Package | Line % | Rating |
|------|---------|--------|--------|
| cli/pkg/release/** (all change files) | pkg/release | 79.0% | ⚠️ Acceptable |
| cli/cmd/** (config/self-update/update/theme) | cmd | 70.5% | ⚠️ Low (informational) |
| cli/pkg/installer/plan/** | plan | 81.8% | ✅ Acceptable |
| cli/pkg/installer/transaction/** | transaction | 69.7% | ⚠️ Low (informational) |

**Average changed file coverage**: ~75%
{go test -cover is the available tool; per-file breakdown not emitted by this runner}

### Assertion Quality
**Assertion quality**: ✅ All assertions verify real behavior (content digests, binary bytes, exit codes, state rotation, event ordering, zero-network proofs, symlink link targets). No tautologies, ghost loops, or type-only-only assertions found.

### Quality Metrics
**Linter**: ✅ go vet ./... — no errors. actionlint — no diagnostics.
**Type Checker**: ✅ go build ./... — no errors.
**Formatter**: ✅ gofmt — clean.

### Issues Found
**CRITICAL**: None
**WARNING**:
- Live GitHub tag publication was not executed against the real repository; the bridge, immutable-identity, and installer-guard contracts were proven against recorded fixtures plus a fake `curl` and static workflow analysis (actionlint, YAML parse). All deterministic contracts are proven; live publication remains unexecuted by design and is the sole residual scope gap.
- Coverage of `cmd` (70.5%) and `transaction` (69.7%) packages is below 80% (informational per strict-TDD rules; not blocking).
**SUGGESTION**:
- The repo-wide markdownlint run reports a pre-existing MD060 error in `README.md:295` (outside this change's modified files; RELEASING.md itself passes with the repo config).
- `ci.yml` "Go test" step sets `working-directory: .` plus `run: cd cli && ...`; functionally correct but the redundant `working-directory: .` could be dropped.

### Verdict
PASS WITH WARNINGS
All 20 requirements and 62/62 scenarios verified compliant with passing runtime evidence; warnings are informational or scoped to live-GitHub publication only.
