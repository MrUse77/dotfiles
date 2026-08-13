# Proposal: MoonArch Versioned Configuration Releases

## Intent

Separate CLI-only self-update from explicit, exact configuration apply/status/rollback while protecting drift and mutable themes.

## Scope

### In Scope
- Publish self-contained `config-vX.Y.Z` artifacts with manifest/digests, assets, materialized submodules, and a legacy bridge.
- `moonarch self update` is CLI-only; `moonarch update` aliases it. Neither moves checkouts nor applies configuration.
- Only `moonarch config apply config-vX.Y.Z` changes configuration; verified current/previous artifacts use state, locking, and journaling.
- Reuse transactions for drift/removal preflight, backed-up deletion, theme preservation, and dependency checks only.

### Out of Scope
- Repackaging `v0.1.0..v0.3.0`; automatic latest config, channels/pins, prereleases, visual catalog, or `MOONARCH_FORCE_REPO`.
- Implicit overwrite, merging, dependency mutation/rollback, `restore` redesign, or full repository/module/cache/executable rename.

## Capabilities

### New Capabilities
- `config-release-publication`: Self-contained artifacts; legacy-safe publication.
- `config-release-resolution`: Exact GitHub verification, compatibility, and two-artifact cache.
- `config-release-operations`: Explicit status/apply/rollback, state, lock/journal, and offline rollback.
- `moonarch-cli-self-update`: Canonical CLI-only command and alias.

### Modified Capabilities
- `installation-transaction`: Whole-plan drift/removal preflight; backed-up deletion.
- `backup-inventory`: Add release provenance; preserve legacy restore.
- `moonarch-theme-selector`: Preserve selection; missing themes require replacement.

## Approach

Both update commands share a binary-only path without repository/configuration stages. After the legacy bridge, CI publishes config releases. Explicit apply safely verifies one exact GitHub tag into the digest cache. Under one lock: recover, preflight, transact, then record identities atomically. Rollback reapplies the previous artifact as a new transaction.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `.github/workflows/release.yml`, `RELEASING.md`, `scripts/install.sh` | Modified | Publication/bridge |
| `cli/cmd/`, `cli/pkg/release/` | Modified/New | CLI-only update; explicit config operations |
| `cli/pkg/installer/{plan,transaction}/`, `home/.local/{bin,share}/moonarch` | Modified | Preflight/removal/theme boundary |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Update coupling/legacy breakage | High | One CLI-only path; bridge first |
| Drift/removal data loss | Medium | Report all paths; abort; require later authorization |
| Artifact/state corruption | Medium | Verify; lock/journal; atomic writes; retain two |

## Rollback Plan

Stop publication and revert workflow/bridge. Failed applies auto-rollback; version rollback reapplies the verified previous artifact with new backups. `restore` remains machine recovery.

## Dependencies

- GitHub Releases/checksums, transaction stack, and `cli/openspec/changes/archive/theme-engine/` context.

## Success Criteria

- [ ] `self update` and its alias replace only the CLI; only exact `config apply config-vX.Y.Z` changes configuration, with no checkout/force/latest/channel/pin path.
- [ ] Exact apply records verified identity; offline rollback reapplies the retained previous artifact as a new transaction.
- [ ] Replacement/removal drift reports every path with zero mutations; unchanged retired targets are backed up and removed.
- [ ] Self-contained apply needs no Git/submodule fetch or dependency mutation; missing themes block pending explicit replacement; legacy clients survive first `config-v*`.
