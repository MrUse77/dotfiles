## Exploration: Independent MoonArch CLI and configuration releases

### Current State

The direction is viable, but tags alone are not a safe configuration-release mechanism. The existing transaction engine is a strong apply/recovery foundation; release discovery, acquisition, installed-state tracking, and product policy are the missing layers.

| Verified evidence | Current behavior and consequence |
|---|---|
| Git history | `HEAD` is `d8c33ef`, exactly one commit after `v0.3.0`. Tags `v0.1.0`, `v0.2.0`, and `v0.3.0` all store managed files at repository root (`.config`, `.local`, `.zshrc`, and related paths); `HEAD` moved them under `home/`. |
| `cli/cmd/install.go` | Discovery requires `home/.local/bin/moonarch` and `home/.local/share/moonarch/themes`, scans `home/.config`, and omits absent sources. A current CLI therefore rejects historical tags before planning, and a removed top-level target is not represented for deletion. |
| `.github/workflows/release.yml`, `RELEASING.md` | One `vMAJOR.MINOR.PATCH` tag versions the whole repository, injects the same value into the CLI, and publishes only CLI binaries plus `SHA256SUMS.txt`. No configuration artifact or compatibility manifest exists. |
| `scripts/install.sh`, `cli/pkg/release/client.go` | Bootstrap and CLI update both use the repository-wide `/releases/latest` endpoint. They cannot distinguish CLI releases from configuration releases. |
| `cli/pkg/release/version.go` | Version parsing strips only one lowercase `v`; `cli-v*` and `config-v*` are invalid inputs. |
| `cli/cmd/update_flow.go` | Update is ordered as release resolution, binary replacement, forced reconciliation of `~/.cache/dotfiles` to the same tag, then configuration reapply. A later configuration failure can roll back managed targets, but it does not restore the already-replaced binary. |
| `cli/cmd/repository_acquirer.go` | Acquisition mutates one canonical checkout using `fetch`, forced detached checkout, and recursive submodule update. It is not isolated per configuration version. |
| `cli/pkg/installer/plan`, `cli/pkg/installer/transaction` | Planning binds source identity/digests and destination pre-state. Execution creates run-scoped backups under `~/.dots-backups/<runID>/`, persists a versioned lifecycle inventory, validates source drift, and automatically rolls back handled managed-target failures. This machinery should be reused. |
| `cli/cmd/restore.go` | Restore selects a retained machine run and individual targets, warns before overwriting post-install edits, and restores pre-install machine state. Inventories contain no CLI/config release identity. Restore is not coherent configuration-version rollback. |
| State search | There is no installed configuration state under XDG state, no config schema/compatibility metadata, no process lock, and no apply journal. The single mutable repository cache is the only update source. |
| MoonArch runtime | `home/.local/share/moonarch/themes/current` is a versioned relative symlink initially targeting `tokyo-night`; `theme-selector` mutates it atomically and restores it on reload failure. Because the whole themes directory is currently one `CopyTree`, configuration reapply can replace this user-selected mutable state. |
| Gitlinks | Neovim, Hyprland plugin, and Zsh plugin paths are submodules. A generated artifact must materialize or explicitly model them; a source archive alone is incomplete. |
| Tests | Go tests cover version comparison, release assets/checksums, update stage ordering, repository acquisition, planning/source binding, transaction rollback/inventory, restore, and runtime symlink preservation. `tests/moonarch-theme-selector_test.sh` covers mutable theme switching, but `test.sh` and current CI do not execute it. |

### Affected Areas

- `.github/workflows/release.yml` — split publication channels and produce a deterministic configuration artifact without breaking legacy generic-latest consumers.
- `scripts/install.sh` and `RELEASING.md` — resolve CLI releases independently and document the migration contract.
- `cli/pkg/release/client.go`, `cli/pkg/release/version.go` — channel-aware release lookup, namespaced version identity, and exact asset resolution.
- `cli/cmd/update.go`, `cli/cmd/update_flow.go` — separate self-update from configuration apply while preserving a migration path for the existing coupled command.
- `cli/cmd/repository_acquirer.go` — replace shared-checkout configuration acquisition with an isolated, verified artifact cache; retain Git acquisition only where still needed.
- `cli/cmd/install.go`, `cli/pkg/installer/catalog.go`, `cli/pkg/installer/plan/` — build plans from a normalized, complete desired target set, including safe removal of targets no longer present.
- `cli/pkg/installer/transaction/` and `cli/cmd/restore.go` — add release provenance without weakening additive inventory compatibility; keep restore distinct from version rollback.
- `home/.local/bin/moonarch/theme-selector` and `home/.local/share/moonarch/themes/current` — preserve mutable theme selection outside immutable release replacement.
- `.gitmodules`, `home/.config/nvim`, `home/.config/hypr/plugins/`, `home/.zsh_plugins/` — define whether release artifacts materialize pinned submodule contents or declare external dependencies.
- `cli/**/*_test.go`, `tests/moonarch-theme-selector_test.sh`, `test.sh`, and `.github/workflows/ci.yml` — extend tests across release channels, artifact safety, state recovery, drift, rollback, and theme preservation.

### Approaches

1. **Direct historical tag checkout** — Check out a requested `v*`/`config-v*` ref in `~/.cache/dotfiles` and run the current installer against it.
   - Pros: Minimal new acquisition code; reuses Git and the current planner.
   - Cons: Current discovery cannot consume `v0.1.0..v0.3.0`; it mutates the shared checkout; a tag/ref alone is not the verified install identity; generic release discovery remains coupled; repository layout, CLI code, submodules, and config payload remain one runtime contract; no complete desired-set or compatibility contract is added.
   - Effort: Low, but not safe enough to pursue.

2. **Isolated Git worktree or clone per version** — Resolve an exact commit and prepare a dedicated checkout for each configuration version before planning.
   - Pros: Does not move the canonical checkout; can retain an exact commit for offline reuse; Git already knows how to acquire pinned submodules.
   - Cons: Historical layouts still require adapters or repackaging; runtime now owns worktree/clone lifecycle, locking, disk cleanup, Git availability, and submodule failures; each checkout includes unrelated repository content; a manifest, digest, compatibility policy, and desired-target catalog are still required. Worktrees also couple every retained version to one mutable parent repository.
   - Effort: Medium; viable as a transition, but carries repository mechanics into the product surface.

3. **Generated immutable configuration artifact per release** — Build a normalized archive containing a manifest, complete target catalog, materialized payload, and digests; publish it from a config-specific release and cache it by content digest.
   - Pros: Decouples install format from repository layout and CLI source; supports exact verification, isolated acquisition, deterministic planning, coherent offline rollback, and future legacy repackaging; avoids mutating the canonical checkout; can be self-contained despite Gitlinks.
   - Cons: Requires a reproducible build workflow, safe archive extraction, schema/compatibility rules, content-addressed cache retention, installed-state/journal handling, and new release-channel resolution.
   - Effort: Medium-High, with the lowest long-term operational risk.

### Recommendation

Use **generated immutable configuration artifacts**. Treat `config-vX.Y.Z` as the publication/reference name and the verified artifact digest as the install identity. Feed the normalized artifact into the existing planner and transaction engine rather than replacing its backup, source-binding, inventory, or automatic rollback behavior.

**Smallest safe MVP boundary (recommended defaults, pending explicit product approval):**

1. Publish future configuration releases only, beginning from the current `home/` layout. Each artifact contains a schema-versioned compatibility manifest, a complete target catalog, materialized submodule content, and per-entry/artifact digests.
2. Add independent CLI/config release resolution. Ship a legacy-compatible bridge before any config publication can become the repository's generic latest release; old clients and bootstrap currently assume every latest release contains a `v*` CLI binary.
3. Support exact-version `config apply`, installed `config status`, and `config rollback`; defer automatic channels and pins. `config rollback` reapplies the retained previous artifact as a new transaction and creates a new backup run. Existing `restore` remains machine-state recovery.
4. Download and extract into staging, reject unsafe archive paths/types, verify before planning, atomically promote into a content-addressed cache, and retain at least the current and previous verified artifacts for offline recovery.
5. Persist installed release identity and previous release identity under XDG state, copy provenance into each additive inventory, serialize mutating operations with a process lock, and use a crash-recoverable journal so filesystem success cannot silently diverge from installed-state metadata.
6. Reconcile the union of the previous and desired target catalogs. Every replacement or removal remains a reviewed, backed-up transaction target; an omitted source must not silently leave stale managed content.
7. Exclude `themes/current` from immutable release replacement. Preserve the selected theme when its bundle still exists; the fallback behavior when it does not remains a product decision.

**Non-goals for the MVP:**

- Direct consumption of legacy `v0.1.0..v0.3.0` tags; optional normalized repackaging can follow.
- A combined automatic latest update, channels, pins, prerelease selection, visual catalog, screenshots, or a remote registry beyond exact release lookup.
- Three-way merging or a general user customization framework.
- Transactional rollback of packages, services, or other external system dependencies.
- Redesigning or deleting retained machine backups and the advanced per-target restore flow.
- Splitting repositories or renaming the repository, Go module, executable, or `~/.cache/dotfiles`; that migration is separate.

**Product/business contracts that must be resolved before proposal:**

1. **User customization:** On drift from the last installed digest, must apply abort, prompt and overwrite after backup, support an explicit force flag, or preserve designated paths? Is fail-closed acceptable for the MVP?
2. **Version selection:** Is the initial contract exact versions only, or must it include `stable`/`latest` channels and persistent pins? What must the legacy `moonarch update` command do when a config is pinned?
3. **Legacy releases:** Should `v0.1.0..v0.3.0` appear as selectable config editions through one-time normalized repackaging, or does support start at the first `config-v*` artifact?
4. **Offline retention:** Is retaining current plus previous mandatory and sufficient? Must all pinned versions be retained, and what disk quota/cleanup policy is acceptable?
5. **Dependency policy:** Must artifacts embed all Gitlink content and assets? Are package/service requirements compatibility checks only, or may config apply mutate them? What does downgrade mean when external effects are irreversible?
6. **Catalog scope:** Are GitHub Releases authoritative? Does MVP need exact lookup only, or names/changelogs/screenshots, prereleases, yanked releases, and pagination?
7. **Theme fallback:** If the selected mutable theme is absent from the target release, should apply abort, preserve an unavailable selection, or switch to a declared release default after confirmation?
8. **Managed deletion authority:** May a complete release remove a previously managed target after drift checks and backup, or must removals require separate confirmation?

**Critical migration seams:**

- Release sequencing must protect unupgraded clients that call `/releases/latest` and accept only `v*`; publishing a config release too early can break both update and bootstrap.
- Existing installations have no trustworthy config identity. Migration should record `legacy/unknown` until a verified artifact is successfully applied rather than infer a version from mutable files or the cache checkout.
- Inventory changes must remain additive so existing retained runs continue to decode and restore.
- The old root layout and the current `home/` layout need normalization at artifact-build time, not ad hoc runtime checkout logic, if legacy editions are later supported.
- Artifact generation must initialize and materialize pinned submodules reproducibly; missing Gitlink contents must fail publication.
- Mutable `themes/current` must move out of the whole-tree replacement boundary without breaking the selector's relative-link and atomic-reload guarantees.

**Critical test seams:**

- Namespaced version parsing and channel-specific release resolution, including a generic latest release that is config-only.
- Deterministic artifact generation, complete target catalogs, materialized Gitlinks, manifest compatibility ranges, and digest verification.
- Safe extraction against traversal, escaping symlinks, duplicate paths, unsupported file types, truncation, and digest mismatch.
- Atomic cache promotion, concurrent apply rejection, retention cleanup, and offline exact apply/rollback.
- Crash injection before/after transaction commit, journal transitions, and installed-state replacement; recovery must converge without guessing.
- Local drift policies for replacement and removal, including decline/force behavior and unchanged-target fast paths.
- Full version rollback as a new transaction, with new backups and unchanged installed state after failed apply.
- Backward decoding/restoration of schema-1 inventories without release metadata.
- Preservation or approved fallback of `themes/current`; add `tests/moonarch-theme-selector_test.sh` to an executed CI path.
- Historical tags must be explicitly rejected or successfully normalized; no accidental runtime layout probing.

### Risks

- A config release that wins the repository-wide latest endpoint can strand legacy bootstrap/update clients before the bridge is deployed.
- Incorrect drift or removal policy can overwrite user customization even though a backup exists.
- A crash or concurrent command between filesystem commit and state update can make the recorded version disagree with the machine unless locking and journaling are included.
- A malformed or compromised archive can escape the cache or install incomplete executable content unless extraction and digest checks fail closed.
- Missing submodule content, assets, or dependency declarations can produce a release that validates structurally but is unusable offline.
- Replacing the themes tree can reset mutable selection or leave consumers pointing at a removed bundle.
- Current/previous retention may consume substantial space because themes, fonts, icons, and materialized submodules are large.
- The implementation will likely exceed the 400-line review budget across workflow, resolver, artifact cache, state/journal, commands, and tests; task planning should use reviewable chained slices or obtain an explicit size exception under `ask-on-risk`.

### Ready for Proposal

No. Feasibility, the preferred architecture, the safe MVP boundary, and migration/test seams are clear, but a proposal would otherwise invent product behavior. Resolve at least customization behavior, exact-version versus channel/pin semantics, legacy support, offline retention, dependency policy, and catalog scope; then proceed to `sdd-propose` using this exploration as the sole technical input.
