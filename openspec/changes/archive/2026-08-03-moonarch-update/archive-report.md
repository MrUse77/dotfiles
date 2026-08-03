# Archive Report: moonarch-update

**Status: PASS**

Archived on 2026-08-03 from the authoritative repository root `/home/agustin/Dev/dotfiles`.

## What Shipped

A release-only `moonarch update` command for the MoonArch CLI with four ordered stages (release → binary → repository → configuration):

- **Release**: native Go GitHub client against `https://api.github.com/repos/MrUse77/dotfiles/releases/latest`, optional `GITHUB_TOKEN` bearer auth, 30s bounded requests, strict SemVer validation/comparison with leading-`v` support (`cli/pkg/release/{client,version,errors}.go`).
- **Binary**: architecture asset selection (`moonarch-cli-linux-{amd64,arm64}`), SHA-256 verification against `SHA256SUMS.txt` (strict GNU parser), same-directory staged atomic replacement with verify-before-chmod-before-rename (`cli/pkg/release/{checksum,replacer}.go`). Download streams stay alive for the caller via `cancelReadCloser` (the remediated download-lifetime contract).
- **Repository**: explicit `RepositoryRequest` pinned to the validated release tag (`~/.cache/dotfiles`, `https://github.com/MrUse77/dotfiles.git`, ref = tag); `BuildRepositoryRequest()` is never called, so `"dev"`→`main` fallback and `DOTFILES_*` overrides cannot leak in (`cli/cmd/repository_acquirer.go` test hook, `cli/cmd/update_flow.go`).
- **Configuration**: configuration-only plan via `planner.BuildConfiguration` + `installer.NewActionCatalog()`, executed through `transaction.New()` prepare/commit/rollback; zero package/`paru`/`hyprpm`/external action requests.
- **UX**: per-stage reporting, TTY vs deterministic non-TTY output, no confirmation, no `--only` flags, no re-exec (new binary active on next invocation), dev-build guard before any collaborator creation (`cli/cmd/update*.go`).
- **Docs**: `README.md` update section (release-only behavior, component boundaries, recovery) and `RELEASING.md` asset/checksum compatibility contract.
- **Deps**: `github.com/Masterminds/semver/v3 v3.3.1` added; `go-isatty v0.0.22` promoted to direct.

15 new implementation/test files under `cli/pkg/release/` and `cli/cmd/`; 6 existing files modified (`go.mod`, `go.sum`, `repository_acquirer.go`, `README.md`, `RELEASING.md`, plus the working tree also shows `cli/cmd/update*.go` additions).

## Verification Outcome

**22/22 acceptance criteria PASS.** The machine-validated envelope is present at the top of `verify-report.md`:

```yaml
schema: gentle-ai.verify-result/v1
verdict: pass
blockers: 0
critical_findings: 0
requirements: 16/16
scenarios: 61/61
```

Validated with `gentle-ai sdd-verify-validate`: valid true, verdict pass, 16 requirements, 61 scenarios.

Initial verification found 17/22 PASS with a source-proven CRITICAL download-lifetime defect (`Download` deferred `cancel()` before the caller read `resp.Body`). A bounded remediation fixed it and re-verified:

1. `GitHubClient.Download` hands cancellation to the caller through a `cancelReadCloser` wrapper; deterministic regression test `TestGitHubClient_Download_KeepsContextAliveForCaller` (RED with old code, GREEN with fix).
2. Configuration-only plan proof strengthened (`updateFakePhaseCatalog` returns realistic per-family actions; test asserts exactly one configuration action and no `paru`/`hyprpm` in planned commands).
3. `gofmt -w` normalization; `gofmt -l .` now empty.
4. Fresh suite evidence (orchestrator-run): `go test -race -count=1 ./...` all green, `go vet ./...` clean, `go build ./...` ok.

All five original blockers (CRITICAL download lifetime, AC 5/7/8, AC 13, strict-TDD evidence table, current runtime evidence) are resolved. Remaining items are non-blocking follow-ups (version-passing seam, weak assertion warnings, extra blank line in `RELEASING.md`).

## Gate Resolution

- Receipt-driven development is OFF (user-owned kill switch). The native dispatcher reports `nextRecommended: archive`, no `blockedReasons`, and `reviewGate.result: invalidated` with `delivery: disabled/unmanaged` — the sanctioned archive condition under a disabled switch. Archive proceeds under ordinary repository policy, NOT under a review receipt. No review transaction was started or referenced.

## Structured Status and Action Context

- **Change:** `moonarch-update`
- **Artifact store:** `openspec` (file-backed; memory mirror via Engram)
- **Change root:** `openspec/changes/moonarch-update/`
- **Action context:** `repo-local`; workspace root and sole allowed edit root are `/home/agustin/Dev/dotfiles`.
- **Native dispatcher:** `nextRecommended: archive`; no `blockedReasons`; `reviewGate.result: invalidated`, `delivery: disabled/unmanaged`.
- **Task status:** 14/14 implementation tasks complete (T1–T14).
- **Delivery strategy:** `single-pr` with approved `size:exception` (~2,600–2,800 forecast lines; verified candidate ≈ 2,917 changed lines, exception retained).
- **Chain strategy:** `size-exception` (chained PRs not recommended).

## Artifacts Read

| Artifact | Path / Key | Engram obs. ID | Status |
|---|---|---|---|
| Proposal | `openspec/changes/moonarch-update/proposal.md` | 1138 | ✅ Read |
| Spec | `openspec/changes/moonarch-update/specs/update/spec.md` (16 requirements, 61 scenarios) | 1139 | ✅ Read |
| Design | `openspec/changes/moonarch-update/design.md` | 1140 | ✅ Read |
| Tasks | `openspec/changes/moonarch-update/tasks.md` | 1141 | ✅ Read (14/14 checked) |
| Apply Progress | `openspec/changes/moonarch-update/apply-progress.md` | 1142 | ✅ Read (corrected 14-task count, strict-TDD table) |
| Verify Report | `openspec/changes/moonarch-update/verify-report.md` | 1143 | ✅ Read (PASS, envelope validated) |
| Config | `openspec/config.yaml` | — | ✅ Read (rules.archive: warn before destructive deltas) |
| Reference archive | `openspec/changes/archive/2026-08-01-two-phase-install-flow/` | — | ✅ Read (repo archive convention) |
| Report format reference | `openspec/changes/archive/2026-07-13-safe-dotfiles-installer-filesystem-corrections/archive-report.md` | — | ✅ Read (format mirrored) |

**Memory snapshot note:** the Engram copies of `apply-progress` (obs 1142) and `verify-report` (obs 1143) predate the on-disk corrections (task-count typo 16→14 and the FAIL→PASS remediation rewrite). The on-disk files are authoritative; the new archive-report observation (this report) records the final state.

## Archive Precondition Checks

| Check | Result | Detail |
|---|---|---|
| Verify report present | PASS | `verify-report.md` found and readable |
| Verify report passing | PASS | Verdict pass; 22/22 criteria; no unresolved FAIL/BLOCKED/CRITICAL |
| Required artifacts present | PASS | proposal, spec, design, tasks, apply-progress, verify-report all present |
| Tasks complete | PASS | 14/14 checked; grep confirms zero `- [ ]` markers |
| Stale-checkbox reconciliation | N/A | No unchecked tasks — no reconciliation needed |
| Legacy root `spec.md` | N/A | No root-level `spec.md`; spec published at `specs/update/spec.md` per repo convention |
| Sync report present | NOT FOUND | No `sync-report.md` exists (repo-wide); no canonical sync is part of this repo's archive flow — see below |
| Destructive merge | N/A | No canonical sync performed; nothing merged or removed |
| Review gate | PASS (disabled) | `delivery: disabled/unmanaged` with kill switch off — sanctioned archive condition |

## Sync Decision (no canonical sync)

Per the parent-verified repo convention and the designated reference archive (`2026-08-01-two-phase-install-flow`, committed as `6defcf3 chore(openspec): archive two-phase install flow change (#92)`), archived changes keep their specs self-contained under the archived `specs/<domain>/` subdir; the reference did **not** sync its `install-flow` domain to `openspec/specs/`, and no `sync-report.md` exists anywhere in the repo. The parent prompt did not approve archive-time sync fallback, so **no canonical sync was performed**:

- `openspec/specs/update/spec.md` was **NOT** created.
- No existing canonical domain was modified or removed.
- The `update` domain spec is preserved in full at `openspec/changes/archive/2026-08-03-moonarch-update/specs/update/spec.md`.

If canonical sync of the `update` domain is ever desired, it can be done as a follow-up (new-domain addition, non-destructive).

### Requirement Names (ADDED — new `update` domain)

1. Command registration
2. Development-build guard
3. Release resolution
4. SemVer validation and comparison
5. Version-outcome routing
6. Asset selection and download
7. SHA-256 verification
8. Atomic binary replacement
9. Process handoff
10. Dotfiles reconciliation
11. Configuration-only reapplication
12. Per-stage result reporting
13. Terminal behavior
14. Failure isolation
15. Testability
16. Determinism

### MODIFIED Requirements

None.

### REMOVED Requirements

None.

### Same-Domain Change Warnings

No active change touches the `update` domain. The only other active change, `consolidate-rofi-menu`, has no `specs/` directory or requirements. No cross-change conflicts.

## Destructive Merge Guard

Not required. No canonical spec was modified or removed; no sync/merge occurred.

## Implementation Task Completion

All 14 implementation tasks (T1–T14) are marked checked (`[x]`) in `tasks.md`; zero unchecked markers (`^\s*- \[ \]`) remain. The apply-progress task-count typo ("16" vs 14) was corrected to **14/14**, matching the authoritative native status and the `[x]` count. `apply-progress.md` carries the compliant `TDD Cycle Evidence` table (Task/Test File/Layer/Safety Net/RED/GREEN/TRIANGULATE/REFACTOR) plus the remediation cycle rows.

## Uncommitted-State Note

The implementation is **UNCOMMITTED** in the working tree: `cli/pkg/release/*` (new), `cli/cmd/update*.go` (new), `cli/cmd/repository_acquirer.go` (hook), `cli/go.mod`, `cli/go.sum`, `README.md`, `RELEASING.md`. `git commit` was blocked by the environment's lifecycle-command guard earlier in the flow; delivery is out of archive scope. Stale review-store entries under `.git/gentle-ai/` and backups under `~/Dev/backup gentle-ai moonarch/` were intentionally left untouched.

## Archived Path

The change directory was moved to:

```
openspec/changes/archive/2026-08-03-moonarch-update/
```

preserving the complete audit trail: `proposal.md`, `design.md`, `tasks.md`, `apply-progress.md`, `verify-report.md`, `specs/update/spec.md`, and this `archive-report.md`.

## Risks

- **Uncommitted implementation:** the feature, tests, and docs exist only in the working tree. Archive preserves the OpenSpec record, but the user must commit before delivery; the current `git status` also shows the move of this change folder as an untracked change.
- **Canonical spec not synced:** `openspec/specs/update/spec.md` does not exist. This matches the repo convention (08-01 reference), but if the team later wants canonical specs for the `update` domain, a follow-up sync is needed.
- **Non-blocking code follow-ups:** global-`Version` testability drift in `runUpdateWithDeps`, five WARNING-level weak assertions, and the extra trailing blank line in `RELEASING.md` remain open; none blocks delivery.
- **Memory snapshot staleness:** Engram obs 1142/1143 are pre-correction snapshots; this archive report (saved to Engram under `sdd/moonarch-update/archive-report`) is the authoritative final-state record.
- **Size:** verified candidate ≈ 2,917 changed lines exceeds the forecast upper bound (~2,800) by ~117 lines; the recorded `size:exception` must be retained on the PR.

## Next Steps (delivery)

1. Commit the implementation (single commit or a small logical set) once the lifecycle-command guard permits.
2. Open a **single PR** with the approved `size:exception` (chain strategy `size-exception`; chained PRs not recommended).
3. Optionally address the non-blocking follow-ups (version-passing seam, weak assertions, trailing blank line) in the same PR or a follow-up.
4. Optionally sync the `update` domain to `openspec/specs/update/spec.md` as a new canonical addition.
