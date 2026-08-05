## Apply Progress: moonarch-update

### Status

- Phase: sdd-apply
- All 14 implementation tasks completed (T1-T14)
- Persisted checkbox updates: T1-T14 marked `[x]` in `openspec/changes/moonarch-update/tasks.md`
- Size exception: APPROVED (single PR ~2600-2800 lines)
- Strict TDD: enforced; every implementation file was preceded by a failing test

### Files created

- `cli/pkg/release/errors.go`
- `cli/pkg/release/version.go`
- `cli/pkg/release/version_test.go`
- `cli/pkg/release/client.go`
- `cli/pkg/release/client_test.go`
- `cli/pkg/release/checksum.go`
- `cli/pkg/release/checksum_test.go`
- `cli/pkg/release/replacer.go`
- `cli/pkg/release/replacer_test.go`
- `cli/cmd/update.go`
- `cli/cmd/update_test.go`
- `cli/cmd/update_flow.go`
- `cli/cmd/update_flow_test.go`
- `cli/cmd/update_output.go`
- `cli/cmd/update_output_test.go`

### Files modified

- `cli/go.mod` — added `github.com/Masterminds/semver/v3 v3.3.1` and promoted `github.com/mattn/go-isatty v0.0.22` to direct dependency
- `cli/go.sum` — updated for semver
- `cli/cmd/repository_acquirer.go` — extracted `buildRepositoryRequestFromEnvironment` and `buildRepositoryRequestImpl` test hook without changing production behavior
- `README.md` — added `moonarch-cli update` command list and dedicated update section
- `RELEASING.md` — documented the asset/checksum contract for `moonarch-cli update`

### Files deleted

- None

### Verification run

- `go test ./...` from `cli/` — PASS
- `go test -v -race -count=1 ./...` from `cli/` — PASS
- `go vet ./...` from `cli/` — PASS
- `go build ./...` from `cli/` — PASS
- `go mod verify` — PASS
- `go list -m all | grep semver` shows `github.com/Masterminds/semver/v3 v3.3.1`
- `go list -m all | grep go-isatty` shows `github.com/mattn/go-isatty v0.0.22` without `// indirect`

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| T3 version | `pkg/release/version_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 3+ cases (leading v, no v, invalid) | ✅ Clean |
| T4 client | `pkg/release/client_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 3+ cases (anonymous, token, errors) | ✅ Clean |
| T5 checksum | `pkg/release/checksum_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 4 cases (malformed, missing, duplicate, mismatch) | ✅ Clean |
| T6 replacer | `pkg/release/replacer_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 3 cases (success, checksum fail, rename fail) | ✅ Clean |
| T8 update command | `cmd/update_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 3 cases (dev guard, args, unknown flag) | ✅ Clean |
| T9/T10 orchestrator | `cmd/update_flow_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 3+ cases (routing, stage isolation, repo ref) | ✅ Clean |
| T11 reporter | `cmd/update_output_test.go` | Unit | N/A (new) | ✅ Written | ✅ Passed | ✅ 3 cases (TTY, non-TTY, ANSI filtering) | ✅ Clean |

Remediation cycle (post-verify, orchestrator-run):

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| Download ctx fix | `pkg/release/client_test.go` | Unit | ✅ suite green | ✅ Written | ✅ Passed | ✅ 2 attempts (httptest buffered, context contract) | ✅ Clean |
| Config-only proof | `cmd/update_flow_test.go` | Unit | ✅ suite green | N/A (strengthened existing) | ✅ Passed | ✅ paru/hyprpm negative + counts | ✅ Clean |

### Test Summary

- **Total tests written**: 60+ (new update/release suites, existing suites untouched)
- **Total tests passing**: all — `go test -race -count=1 ./...` green
- **Layers used**: Unit (all)
- **Approval tests** (refactoring): None — no refactoring tasks
- **Pure functions created**: `CompareVersions`, checksum parser, asset selectors

### Notes

- The dev guard in `runUpdateWithFactory` prints the release-build-only message and returns `nil` before the dependency factory runs; a counting-factory test asserts zero collaborator calls.
- The update command constructs an explicit `RepositoryRequest` with fixed destination/URL/tag and never calls `BuildRepositoryRequest()`; a test hook override confirms zero invocations even with conflicting `DOTFILES_*` env vars.
- The configuration plan builder uses `planner.BuildConfiguration` with a fresh `installer.NewActionCatalog()`; tests assert `ConfigurationActions` is called once and `PackageActions`/`ExternalActions` are never called.
- Binary replacement is outside the configuration transaction; a configuration failure after binary success preserves `BinaryActiveOnNextInvocation`.
- Non-TTY output is deterministic, line-oriented, ANSI-free, and quoted; repeated runs with identical inputs produce byte-identical output.
- `git commit` was blocked by the environment's lifecycle-command guard despite receipt-driven development being disabled (`gentle-ai review mode status` reported `off`). All changes remain in the working tree uncommitted; tests are green.

### Next step

- `sdd-verify` to validate acceptance criteria and behavioral checks
