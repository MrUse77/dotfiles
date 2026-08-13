#!/usr/bin/env bash
set -euo pipefail

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    return 1
}

verify_bridge() {
    local fixture="$1" tag asset
    tag="$(sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' "$fixture" | head -n 1)"
    [[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || {
        fail "latest release is not a stable CLI tag: ${tag:-missing}"
        return 1
    }
    for asset in moonarch-cli-linux-amd64 moonarch-cli-linux-arm64 SHA256SUMS.txt; do
        grep -Eq "\"name\"[[:space:]]*:[[:space:]]*\"$asset\"" "$fixture" || {
            fail "latest CLI release lacks $asset"
            return 1
        }
    done
    printf 'Bridge verified: /releases/latest resolves %s with CLI assets.\n' "$tag"
}

assert_new_identity() {
    local existing_tag="$1" existing_digest="$2" proposed_digest="$3"
    [[ "$proposed_digest" =~ ^[0-9a-f]{64}$ ]] || {
        fail "proposed artifact digest is invalid"
        return 1
    }
    if [[ -n "$existing_tag" ]]; then
        fail "$existing_tag is already published (existing digest: ${existing_digest:-unknown}; proposed digest: $proposed_digest)"
        return 1
    fi
}

if [[ "${1:-}" == verify-bridge ]]; then
    verify_bridge "$2"
    exit
fi
if [[ "${1:-}" == assert-new-identity ]]; then
    assert_new_identity "${2:-}" "${3:-}" "$4"
    exit
fi

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
digest_a="$(printf 'a%.0s' {1..64})"
digest_b="$(printf 'b%.0s' {1..64})"

cat >"$tmp/latest-cli.json" <<'JSON'
{"tag_name":"v1.2.3","published_at":"2026-01-01T00:00:00Z","assets":[{"name":"moonarch-cli-linux-amd64"},{"name":"moonarch-cli-linux-arm64"},{"name":"SHA256SUMS.txt"}]}
JSON
cat >"$tmp/newer-config.json" <<'JSON'
{"tag_name":"config-v2.0.0","published_at":"2026-02-01T00:00:00Z","assets":[]}
JSON
cat >"$tmp/cli-missing-assets.json" <<'JSON'
{"tag_name":"v1.2.4","assets":[{"name":"moonarch-cli-linux-amd64"}]}
JSON

cli_date="$(sed -n 's/.*"published_at":"\([^"]*\)".*/\1/p' "$tmp/latest-cli.json")"
config_date="$(sed -n 's/.*"published_at":"\([^"]*\)".*/\1/p' "$tmp/newer-config.json")"
[[ "$config_date" > "$cli_date" ]] || fail 'recorded config release is not newer than the CLI release'
verify_bridge "$tmp/latest-cli.json" >/dev/null
if verify_bridge "$tmp/newer-config.json" >/dev/null 2>&1; then
    fail 'a config release was accepted as the legacy latest release'
fi
if verify_bridge "$tmp/cli-missing-assets.json" >/dev/null 2>&1; then
    fail 'an incomplete CLI release was accepted as the legacy bridge'
fi
if assert_new_identity config-v2.0.0 "$digest_a" "$digest_a" >/dev/null 2>&1; then
    fail 'same-tag, same-digest republication was accepted'
fi
if assert_new_identity config-v2.0.0 "$digest_a" "$digest_b" >/dev/null 2>&1; then
    fail 'same-tag replacement was accepted'
fi
assert_new_identity '' '' "$digest_a"

workflow="$repo_root/.github/workflows/release.yml"
ci="$repo_root/.github/workflows/ci.yml"
grep -Fq 'tests/release-bridge_test.sh verify-bridge' "$workflow" || fail 'release workflow does not invoke the bridge gate'
grep -Fq 'tests/release-bridge_test.sh assert-new-identity' "$workflow" || fail 'release workflow does not invoke the identity gate'
grep -Fq -- '--latest=false' "$workflow" || fail 'config releases are not prevented from becoming latest'
grep -Fq -- '--draft --latest=false' "$workflow" || fail 'config assets are not staged before publication'
grep -Fq 'submodules: recursive' "$workflow" || fail 'release workflow does not materialize pinned submodules'
grep -Fq -- "--mtime='@0'" "$workflow" || fail 'release archive does not normalize timestamps'
grep -Fq 'manifest.json home assets' "$workflow" || fail 'manifest is not the first archive entry'
grep -Fq 'run: bash tests/release-bridge_test.sh' "$ci" || fail 'CI does not invoke the bridge test'
grep -Fq 'run: cd cli && go test -race -count=1 ./...' "$ci" || fail 'CLI changes do not run the uncached race suite'

mkdir -p "$tmp/bin" "$tmp/home"
cat >"$tmp/bin/curl" <<'SH'
#!/usr/bin/env bash
if [[ "$*" == *'/releases/latest'* ]]; then
    command cat "$LATEST_FIXTURE"
else
    : >"$UNEXPECTED_DOWNLOAD"
    exit 22
fi
SH
chmod +x "$tmp/bin/curl"
if PATH="$tmp/bin:$PATH" HOME="$tmp/home" LATEST_FIXTURE="$tmp/newer-config.json" \
    UNEXPECTED_DOWNLOAD="$tmp/downloaded" bash "$repo_root/scripts/install.sh" >"$tmp/install.out" 2>&1; then
    fail 'installer accepted a config release as the latest CLI release'
fi
grep -Fq 'release de configuración' "$tmp/install.out" || fail 'installer did not report the config-release guard'
[[ ! -e "$tmp/downloaded" ]] || fail 'installer downloaded an asset after rejecting latest'
if PATH="$tmp/bin:$PATH" HOME="$tmp/home" LATEST_FIXTURE="$tmp/latest-cli.json" \
    UNEXPECTED_DOWNLOAD="$tmp/cli-download" bash "$repo_root/scripts/install.sh" >/dev/null 2>&1; then
    fail 'fixture download unexpectedly succeeded'
fi
[[ -e "$tmp/cli-download" ]] || fail 'installer did not accept the bridged CLI release'

printf 'PASS: legacy bridge fixtures, immutable identity, and installer guard\n'
