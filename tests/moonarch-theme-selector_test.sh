#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
selector="${THEME_SELECTOR:-$repo_root/.local/bin/moonarch/theme-selector}"

pass_count=0

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

assert_eq() {
    [[ "$1" == "$2" ]] || fail "expected '$2', got '$1'"
}

assert_success() {
    "$@" || fail "command failed: $*"
}

assert_failure() {
    if "$@"; then
        fail "command unexpectedly succeeded: $*"
    fi
}

new_case() {
    case_dir="$(mktemp -d)"
    themes="$case_dir/themes"
    fake_bin="$case_dir/bin"
    command_log="$case_dir/commands.log"
    wofi_input="$case_dir/wofi-input"
    mkdir -p "$themes" "$fake_bin"
    : > "$command_log"

    cat > "$fake_bin/wofi" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'wofi %s\n' "$*" >> "$COMMAND_LOG"
cat > "$WOFI_INPUT"
[[ "${WOFI_CANCEL:-0}" == 1 ]] && exit 1
printf '%s\n' "${WOFI_OUTPUT:-}"
EOF
    cat > "$fake_bin/hyprctl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'hyprctl %s\n' "$*" >> "$COMMAND_LOG"
[[ "${HYPRCTL_FAIL:-0}" != 1 ]]
EOF
    cat > "$fake_bin/pgrep" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'pgrep %s\n' "$*" >> "$COMMAND_LOG"
[[ "${WAYBAR_RUNNING:-0}" == 1 ]]
EOF
    cat > "$fake_bin/pkill" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf 'pkill %s\n' "$*" >> "$COMMAND_LOG"
[[ "${PKILL_FAIL:-0}" != 1 ]]
EOF
    chmod +x "$fake_bin"/*
}

make_bundle() {
    local id="$1"
    local bundle="$themes/$id"
    mkdir -p "$bundle"
    printf 'id = "%s"\n' "$id" > "$bundle/manifest.toml"
    printf '# hyprland %s\n' "$id" > "$bundle/hyprland.conf"
    printf '/* waybar %s */\n' "$id" > "$bundle/waybar.css"
    printf '/* wofi %s */\n' "$id" > "$bundle/wofi.css"
    printf '# ghostty %s\n' "$id" > "$bundle/ghostty.conf"
}

run_selector() {
    env \
        MOONARCH_THEMES_ROOT="$themes" \
        PATH="$fake_bin:$PATH" \
        COMMAND_LOG="$command_log" \
        WOFI_INPUT="$wofi_input" \
        "$selector" "$@"
}

assert_current() {
    assert_eq "$(readlink "$themes/current")" "$1"
}

preserves_current_on_failure() {
    local description="$1"
    shift
    new_case
    make_bundle tokyo-night
    ln -s tokyo-night "$themes/current"
    assert_failure "$@"
    assert_current tokyo-night
    printf 'PASS: %s\n' "$description"
    pass_count=$((pass_count + 1))
}

preserves_current_on_failure "invalid IDs are rejected" run_selector '../escape'

new_case
make_bundle tokyo-night
ln -s tokyo-night "$themes/current"
rm "$themes/tokyo-night/manifest.toml"
assert_failure run_selector tokyo-night
assert_current tokyo-night
printf 'PASS: missing manifest is rejected\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
ln -s tokyo-night "$themes/current"
printf 'id = "wrong"\n' > "$themes/tokyo-night/manifest.toml"
assert_failure run_selector tokyo-night
assert_current tokyo-night
printf 'PASS: mismatched manifest is rejected\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
ln -s tokyo-night "$themes/current"
rm "$themes/tokyo-night/wofi.css"
assert_failure run_selector tokyo-night
assert_current tokyo-night
printf 'PASS: missing fragment is rejected\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
ln -s tokyo-night "$themes/current"
outside="$case_dir/outside.conf"
printf 'outside\n' > "$outside"
rm "$themes/tokyo-night/hyprland.conf"
ln -s "$outside" "$themes/tokyo-night/hyprland.conf"
assert_failure run_selector tokyo-night
assert_current tokyo-night
printf 'PASS: escaped fragment symlink is rejected\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
printf 'not a link\n' > "$themes/current"
assert_failure run_selector tokyo-night
[[ -f "$themes/current" ]] || fail 'non-symlink current was mutated'
printf 'PASS: non-symlink current is rejected\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
ln -s "$themes/tokyo-night" "$themes/current"
assert_failure run_selector tokyo-night
assert_eq "$(readlink "$themes/current")" "$themes/tokyo-night"
printf 'PASS: absolute current is rejected\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
ln -s tokyo-night "$themes/current"
assert_success env WOFI_CANCEL=1 MOONARCH_THEMES_ROOT="$themes" PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" WOFI_INPUT="$wofi_input" "$selector"
assert_current tokyo-night
if grep -Eq '^(hyprctl|pgrep|pkill) ' "$command_log"; then
    fail 'cancellation ran a reload command'
fi
printf 'PASS: Wofi cancellation is a no-op\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
make_bundle alpha
ln -s tokyo-night "$themes/current"
bundle_digest_before="$(sha256sum "$themes"/tokyo-night/* "$themes"/alpha/*)"
assert_success env WOFI_OUTPUT=alpha MOONARCH_THEMES_ROOT="$themes" PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" WOFI_INPUT="$wofi_input" WAYBAR_RUNNING=0 "$selector"
assert_current alpha
assert_eq "$(cat "$wofi_input")" $'alpha\ntokyo-night'
bundle_digest_after="$(sha256sum "$themes"/tokyo-night/* "$themes"/alpha/*)"
assert_eq "$bundle_digest_after" "$bundle_digest_before"
grep -qx 'hyprctl reload' "$command_log" || fail 'Hyprland reload argv differs'
grep -qx 'pgrep -x waybar' "$command_log" || fail 'Waybar process check argv differs'
grep -qx 'wofi --show dmenu' "$command_log" || fail 'Wofi argv differs'
if grep -Eq 'ghostty|wofi.*reload' "$command_log"; then
    fail 'selector invoked an unsupported Wofi or Ghostty reload'
fi
if compgen -G "${themes}/.current*" >/dev/null; then
    fail 'atomic switch left a temporary link'
fi
printf 'PASS: sorted selection atomically switches with fixed commands\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
make_bundle alpha
ln -s tokyo-night "$themes/current"
assert_failure env HYPRCTL_FAIL=1 WOFI_OUTPUT=alpha MOONARCH_THEMES_ROOT="$themes" PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" WOFI_INPUT="$wofi_input" WAYBAR_RUNNING=0 "$selector"
assert_current tokyo-night
grep -c '^hyprctl reload$' "$command_log" | grep -qx '2' || fail 'rollback did not retry Hyprland reload'
printf 'PASS: reload failure restores prior link\n'
pass_count=$((pass_count + 1))

new_case
make_bundle tokyo-night
make_bundle alpha
ln -s tokyo-night "$themes/current"
assert_failure env PKILL_FAIL=1 WOFI_OUTPUT=alpha MOONARCH_THEMES_ROOT="$themes" PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" WOFI_INPUT="$wofi_input" WAYBAR_RUNNING=1 "$selector"
assert_current tokyo-night
grep -c '^pkill -SIGUSR2 waybar$' "$command_log" | grep -qx '2' || fail 'rollback did not retry Waybar reload'
printf 'PASS: Waybar reload failure restores prior link\n'
pass_count=$((pass_count + 1))

grep -Fqx 'source = ~/.local/share/moonarch/themes/current/hyprland.conf' "$repo_root/.config/hypr/hyprland.conf" || fail 'Hyprland does not import the current theme'
grep -Fqx "      bind = \$mainMod SHIFT, T, exec, ~/.local/bin/moonarch/theme-selector" "$repo_root/.config/hypr/hyprland.conf" || fail 'Hyprland selector binding is missing'
grep -Fqx '@import url("../../.local/share/moonarch/themes/current/waybar.css");' "$repo_root/.config/waybar/style.css" || fail 'Waybar does not import the current theme'
grep -Fqx '@import url("../../.local/share/moonarch/themes/current/wofi.css");' "$repo_root/.config/wofi/style.css" || fail 'Wofi does not import the current theme'
grep -Fqx 'config-file = "~/.local/share/moonarch/themes/current/ghostty.conf"' "$repo_root/.config/ghostty/config" || fail 'Ghostty does not import the current theme'
if grep -Eq '^[[:space:]]*config-file[[:space:]]*=' "$repo_root/.config/ghostty/config-clean"; then
    fail 'Ghostty clean config must rely on the default profile and define no config-file settings'
fi
printf 'PASS: all consumers bind only to the current theme\n'
pass_count=$((pass_count + 1))

printf 'PASS: %d MoonArch selector scenarios\n' "$pass_count"
