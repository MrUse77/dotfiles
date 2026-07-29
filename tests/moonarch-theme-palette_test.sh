#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
themes_root="$repo_root/.local/share/moonarch/themes"
style_file="$repo_root/.config/waybar/style.css"
clean_ghostty="$repo_root/.config/ghostty/config-clean"

protected_paths=(
    .local/share/moonarch/themes/tokyo-night
    Temas/Tokyo_Night/paleta.txt
)
protected_files=(
    .local/share/moonarch/themes/tokyo-night/ghostty.conf
    .local/share/moonarch/themes/tokyo-night/hyprland.conf
    .local/share/moonarch/themes/tokyo-night/manifest.toml
    .local/share/moonarch/themes/tokyo-night/waybar.css
    .local/share/moonarch/themes/tokyo-night/wofi.css
    Temas/Tokyo_Night/paleta.txt
)

# Immutable blob IDs from the original Tokyo Night bundle commit.
declare -A protected_file_hashes=(
    [.local/share/moonarch/themes/tokyo-night/ghostty.conf]=7e37b35d301ae8061282a2798a83698c479b5312
    [.local/share/moonarch/themes/tokyo-night/hyprland.conf]=247001b754aad47ce14e9610e181946b96fea84a
    [.local/share/moonarch/themes/tokyo-night/manifest.toml]=45309b48d1ce282093fe64adb8ebb582d2983794
    [.local/share/moonarch/themes/tokyo-night/waybar.css]=2b39a86a7c986c48581e6d8aac83d6dc8ff3e830
    [.local/share/moonarch/themes/tokyo-night/wofi.css]=78293929c5003a86ac8aff602c0570d08985cf6a
    [Temas/Tokyo_Night/paleta.txt]=b51e9ab96664bd55b26cca8d5b9f59ff9a5c0080
)

fail() {
    printf 'FAIL: %s\n' "$*" >&2
    exit 1
}

assert_eq() {
    [[ "$1" == "$2" ]] || fail "expected '$2', got '$1'"
}

assert_protected_paths_unchanged() {
    local expected_paths actual_paths path actual_hash git_status

    for path in "${protected_files[@]}"; do
        [[ -f "$repo_root/$path" && ! -L "$repo_root/$path" ]] || {
            fail "protected Tokyo file is absent or not regular: $path"
        }
        actual_hash="$(git -C "$repo_root" hash-object --no-filters -- "$repo_root/$path")"
        [[ "$actual_hash" == "${protected_file_hashes[$path]}" ]] || {
            fail "protected Tokyo file content changed: $path"
        }
    done

    expected_paths="$(printf '%s\n' "${protected_files[@]}" | LC_ALL=C sort)"
    actual_paths="$(git -C "$repo_root" ls-files -- "${protected_paths[@]}" | LC_ALL=C sort)"
    [[ "$actual_paths" == "$expected_paths" ]] || {
        fail 'protected Tokyo file set changed'
    }

    git_status="$(git -C "$repo_root" status --porcelain=v1 --untracked-files=all -- \
        "${protected_paths[@]}")"
    [[ -z "$git_status" ]] || fail "protected Tokyo Night paths have worktree entries: $git_status"
}

normalize_value() {
    tr '[:upper:]' '[:lower:]' | tr -d '[:space:];'
}

source_value() {
    local source_file="$1"
    local section="$2"
    local key="$3"

    awk -v wanted_section="$section" -v wanted_key="$key" '
        BEGIN { in_section = 0 }
        /^[[:space:]]*\[/ {
            in_section = index(tolower($0), tolower(wanted_section)) > 0
            next
        }
        !in_section { next }
        {
            line = $0
            sub(/^[[:space:]]+/, "", line)
            sub(/[[:space:]]+$/, "", line)
            field_count = split(line, fields, /[[:space:]]+/)

            if (fields[1] == "@define-color" && fields[2] == wanted_key) {
                value = fields[3]
                first_value_field = 4
            } else {
                token = fields[1]
                sub(/:$/, "", token)
                if (token != wanted_key) {
                    next
                }
                first_value_field = 2
                if (fields[first_value_field] == "=") {
                    first_value_field++
                }
                value = fields[first_value_field]
                first_value_field++
            }

            for (i = first_value_field; i <= field_count; i++) {
                value = value " " fields[i]
            }
            sub(/;$/, "", value)
            print value
            exit
        }
    ' "$source_file"
}

fragment_define_value() {
    local fragment="$1"
    local key="$2"

    awk -v wanted_key="$key" '
        function strip_comments(line, start, end, prefix, rest) {
            while (1) {
                if (in_comment) {
                    end = index(line, "*/")
                    if (!end) {
                        return ""
                    }
                    line = substr(line, end + 2)
                    in_comment = 0
                }
                start = index(line, "/*")
                if (!start) {
                    return line
                }
                prefix = substr(line, 1, start - 1)
                rest = substr(line, start + 2)
                end = index(rest, "*/")
                if (!end) {
                    in_comment = 1
                    return prefix
                }
                line = prefix substr(rest, end + 2)
            }
        }
        function trim(value) {
            sub(/^[[:space:]]+/, "", value)
            sub(/[[:space:]]+$/, "", value)
            return value
        }
        {
            line = trim(strip_comments($0))
            if (line !~ /^@define-color[[:space:]]+/) {
                next
            }
            rest = line
            sub(/^@define-color[[:space:]]+/, "", rest)
            field_count = split(rest, fields, /[[:space:]]+/)
            if (field_count < 2 || fields[1] != wanted_key ||
                rest !~ /;[[:space:]]*$/) {
                next
            }
            value = rest
            sub(/^[^[:space:]]+[[:space:]]+/, "", value)
            sub(/;$/, "", value)
            value = trim(value)
            if (value != "") {
                last_value = value
            }
        }
        END {
            if (last_value != "") {
                print last_value
            }
        }
    ' "$fragment"
}

ghostty_setting_values() {
    local fragment="$1"
    local key="$2"

    awk -v wanted_key="$key" '
        function trim(value) {
            sub(/^[[:space:]]+/, "", value)
            sub(/[[:space:]]+$/, "", value)
            return value
        }
        {
            line = trim($0)
            if (line == "" || line ~ /^#/) {
                next
            }
            separator = index(line, "=")
            if (!separator) {
                next
            }
            setting = trim(substr(line, 1, separator - 1))
            if (setting != wanted_key) {
                next
            }
            value = trim(substr(line, separator + 1))
            print value
        }
    ' "$fragment"
}

fragment_setting_value() {
    local fragment="$1"
    local key="$2"
    local value last_value=''

    while IFS= read -r value; do
        last_value="$value"
    done < <(ghostty_setting_values "$fragment" "$key")
    printf '%s\n' "$last_value"
}

fragment_palette_value() {
    local fragment="$1"
    local index="$2"

    awk -v wanted_index="$index" '
        function trim(value) {
            sub(/^[[:space:]]+/, "", value)
            sub(/[[:space:]]+$/, "", value)
            return value
        }
        {
            line = trim($0)
            if (line == "" || line ~ /^#/) {
                next
            }
            separator = index(line, "=")
            if (!separator || trim(substr(line, 1, separator - 1)) != "palette") {
                next
            }
            value = trim(substr(line, separator + 1))
            field_count = split(value, fields, /[[:space:]]+/)
            if (field_count >= 3 && fields[1] == wanted_index && fields[2] == "=") {
                last_value = fields[3]
                next
            }
            pair_separator = index(fields[1], "=")
            if (!pair_separator || substr(fields[1], 1, pair_separator - 1) != wanted_index) {
                next
            }
            last_value = substr(fields[1], pair_separator + 1)
        }
        END {
            if (last_value != "") {
                print last_value
            }
        }
    ' "$fragment"
}

assert_define() {
    local fragment="$1"
    local key="$2"
    local value

    value="$(fragment_define_value "$fragment" "$key")"
    [[ -n "$value" ]] || fail "$fragment lacks an actual @define-color $key declaration"
}

assert_mapping() {
    local description="$1"
    local expected="$2"
    local actual="$3"
    local expected_normalized actual_normalized

    [[ -n "$expected" ]] || fail "source mapping is missing: $description"
    [[ -n "$actual" ]] || fail "bundle mapping is missing: $description"
    expected_normalized="$(normalize_value <<<"$expected")"
    actual_normalized="$(normalize_value <<<"$actual")"
    [[ "$actual_normalized" == "$expected_normalized" ]] || {
        fail "$description differs: expected '$expected', got '$actual'"
    }
}

assert_rule_uses() {
    local selector="$1"
    local alias="$2"

    awk -v wanted_selector="$selector" -v wanted_alias="$alias" '
        function strip_comments(line, start, end, prefix, rest) {
            while (1) {
                if (in_comment) {
                    end = index(line, "*/")
                    if (!end) {
                        return ""
                    }
                    line = substr(line, end + 2)
                    in_comment = 0
                }
                start = index(line, "/*")
                if (!start) {
                    return line
                }
                prefix = substr(line, 1, start - 1)
                rest = substr(line, start + 2)
                end = index(rest, "*/")
                if (!end) {
                    in_comment = 1
                    return prefix
                }
                line = prefix substr(rest, end + 2)
            }
        }
        function contains_token(line, token, position, before, after, remainder) {
            remainder = line
            while ((position = index(remainder, token)) != 0) {
                before = ""
                if (position > 1) {
                    before = substr(remainder, position - 1, 1)
                }
                after = substr(remainder, position + length(token), 1)
                if ((before == "" || before !~ /[[:alnum:]_-]/) &&
                    (after == "" || after !~ /[[:alnum:]_-]/)) {
                    return 1
                }
                remainder = substr(remainder, position + length(token))
            }
            return 0
        }
        BEGIN { result = 1 }
        {
            line = strip_comments($0)
        }
        !in_rule && contains_token(line, wanted_selector) {
            in_rule = 1
            found_alias = 0
        }
        in_rule && contains_token(line, wanted_alias) {
            found_alias = 1
        }
        in_rule && line ~ /^[[:space:]]*}/ {
            if (found_alias) {
                result = 0
                exit
            }
            in_rule = 0
        }
        END { exit result }
    ' "$style_file" || fail "rule '$selector' does not use $alias"
}

assert_ghostty_clean_config() {
    local config_file_count=0
    local value key override_count override_values
    local color_keys=(
        theme
        palette
        background
        foreground
        selection-background
        selection-foreground
        cursor-color
        cursor-text
    )

    while IFS= read -r value; do
        config_file_count=$((config_file_count + 1))
    done < <(ghostty_setting_values "$clean_ghostty" config-file)
    [[ "$config_file_count" -eq 0 ]] || {
        fail "Ghostty clean config must rely on the default profile and define no config-file settings (found $config_file_count)"
    }

    for key in "${color_keys[@]}"; do
        override_count=0
        override_values=''
        while IFS= read -r value; do
            override_count=$((override_count + 1))
            if [[ -n "$override_values" ]]; then
                override_values+=', '
            fi
            override_values+="${value:-<empty>}"
        done < <(ghostty_setting_values "$clean_ghostty" "$key")
        [[ "$override_count" -eq 0 ]] || {
            fail "Ghostty clean config overrides $key: $override_values"
        }
    done
    printf 'PASS: Ghostty clean config relies on default profile for current theme\n'
}

expected_theme_ids=(
    catppuccin-latte
    catppuccin-mocha
    decay-green
    edge-runner
    frosted-glass
    graphite-mono
    gruvbox-retro
    material-sakura
    nordic-blue
    rose-pine
    synth-wave
    tokyo-night
)

declare -A source_dirs=(
    [catppuccin-latte]=Catppuccin_Latte
    [catppuccin-mocha]=Catppuccin_Mocha
    [decay-green]=Decay_Green
    [edge-runner]=Edge_Runner
    [frosted-glass]=Frosted_Glass
    [graphite-mono]=Graphite_Mono
    [gruvbox-retro]=Gruvbox_Retro
    [material-sakura]=Material_Sakura
    [nordic-blue]=Nordic_Blue
    [rose-pine]=Rose_Pine
    [synth-wave]=Synth_Wave
)

verify_bundle_contract() {
    local actual_count=0
    local bundle id source_file file index cursor_text
    local required_files=(manifest.toml hyprland.conf waybar.css wofi.css ghostty.conf)
    local aliases=(text_main bg_dark accent_blue urgent_red)

    [[ -L "$themes_root/current" ]] || fail 'current theme is not a symlink'
    assert_eq "$(readlink "$themes_root/current")" tokyo-night
    assert_protected_paths_unchanged

    for bundle in "$themes_root"/*; do
        [[ -d "$bundle" ]] || continue
        id="${bundle##*/}"
        [[ "$id" == current ]] && continue
        actual_count=$((actual_count + 1))
    done
    assert_eq "$actual_count" "${#expected_theme_ids[@]}"

    for id in "${expected_theme_ids[@]}"; do
        bundle="$themes_root/$id"
        [[ -d "$bundle" && ! -L "$bundle" ]] || fail "invalid bundle directory: $id"
        grep -Eq "^id[[:space:]]*=[[:space:]]*\"$id\"[[:space:]]*$" \
            "$bundle/manifest.toml" || fail "manifest ID mismatch: $id"
        for file in "${required_files[@]}"; do
            [[ -f "$bundle/$file" && ! -L "$bundle/$file" ]] || {
                fail "missing regular fragment '$file' in $id"
            }
        done
        for alias in "${aliases[@]}"; do
            assert_define "$bundle/waybar.css" "$alias"
        done
    done

    for id in "${!source_dirs[@]}"; do
        bundle="$themes_root/$id"
        source_file="$repo_root/Temas/${source_dirs[$id]}/paleta.txt"

        assert_mapping "$id Hyprland active border" \
            "$(source_value "$source_file" Hyprland col.active_border)" \
            "$(fragment_setting_value "$bundle/hyprland.conf" col.active_border)"
        assert_mapping "$id Hyprland inactive border" \
            "$(source_value "$source_file" Hyprland col.inactive_border)" \
            "$(fragment_setting_value "$bundle/hyprland.conf" col.inactive_border)"

        assert_mapping "$id Waybar text alias" \
            "$(source_value "$source_file" Waybar main-fg)" \
            "$(fragment_define_value "$bundle/waybar.css" text_main)"
        assert_mapping "$id Waybar background alias" \
            "$(source_value "$source_file" Waybar main-bg)" \
            "$(fragment_define_value "$bundle/waybar.css" bg_dark)"
        assert_mapping "$id Waybar accent alias" \
            "$(source_value "$source_file" Waybar wb-act-bg)" \
            "$(fragment_define_value "$bundle/waybar.css" accent_blue)"
        # The source palettes do not define a Waybar urgent color; use Kitty red.
        assert_mapping "$id Waybar urgent alias" \
            "$(source_value "$source_file" 'Kitty Terminal' color1)" \
            "$(fragment_define_value "$bundle/waybar.css" urgent_red)"

        assert_mapping "$id Wofi background" \
            "$(source_value "$source_file" Rofi main-bg)" \
            "$(fragment_define_value "$bundle/wofi.css" wofi_background)"
        assert_mapping "$id Wofi foreground" \
            "$(source_value "$source_file" Rofi main-fg)" \
            "$(fragment_define_value "$bundle/wofi.css" wofi_foreground)"
        assert_mapping "$id Wofi surface" \
            "$(source_value "$source_file" Waybar main-bg)" \
            "$(fragment_define_value "$bundle/wofi.css" wofi_surface)"
        assert_mapping "$id Wofi accent" \
            "$(source_value "$source_file" Rofi select-bg)" \
            "$(fragment_define_value "$bundle/wofi.css" wofi_accent)"

        for index in {0..15}; do
            assert_mapping "$id Ghostty palette $index" \
                "$(source_value "$source_file" 'Kitty Terminal' "color$index")" \
                "$(fragment_palette_value "$bundle/ghostty.conf" "$index")"
        done
        assert_mapping "$id Ghostty background" \
            "$(source_value "$source_file" 'Kitty Terminal' background)" \
            "$(fragment_setting_value "$bundle/ghostty.conf" background)"
        assert_mapping "$id Ghostty foreground" \
            "$(source_value "$source_file" 'Kitty Terminal' foreground)" \
            "$(fragment_setting_value "$bundle/ghostty.conf" foreground)"
        assert_mapping "$id Ghostty selection background" \
            "$(source_value "$source_file" 'Kitty Terminal' selection_background)" \
            "$(fragment_setting_value "$bundle/ghostty.conf" selection-background)"
        assert_mapping "$id Ghostty selection foreground" \
            "$(source_value "$source_file" 'Kitty Terminal' selection_foreground)" \
            "$(fragment_setting_value "$bundle/ghostty.conf" selection-foreground)"
        assert_mapping "$id Ghostty cursor" \
            "$(source_value "$source_file" 'Kitty Terminal' cursor)" \
            "$(fragment_setting_value "$bundle/ghostty.conf" cursor-color)"
        cursor_text="$(source_value "$source_file" 'Kitty Terminal' cursor_text_color)"
        [[ -n "$cursor_text" ]] || {
            cursor_text="$(source_value "$source_file" 'Kitty Terminal' foreground)"
        }
        assert_mapping "$id Ghostty cursor text" \
            "$cursor_text" \
            "$(fragment_setting_value "$bundle/ghostty.conf" cursor-text)"
    done

    assert_protected_paths_unchanged
    printf 'PASS: protected Tokyo Night bundle and %d semantic mappings\n' \
        "${#source_dirs[@]}"
}

verify_shared_waybar() {
    if grep -Eq '#[[:xdigit:]]{3,8}([[:space:];,)]|$)|rgba[[:space:]]*\(' "$style_file"; then
        fail 'shared Waybar stylesheet contains a fixed color literal'
    fi
    assert_rule_uses '.modules-left' '@accent_blue'
    assert_rule_uses '#workspaces button' '@text_main'
    assert_rule_uses '#workspaces button.active' '@accent_blue'
    assert_rule_uses '#workspaces button:hover' '@accent_blue'
    assert_rule_uses '#groups-hardware' '@bg_dark'
    assert_rule_uses '#taskbar button:hover' '@bg_dark'
    assert_rule_uses '#tray:hover' '@accent_blue'
    assert_rule_uses '#custom-launcher:hover' '@accent_blue'
    assert_rule_uses '#battery.warning' '@urgent_red'
    assert_rule_uses '#battery.charging' '@accent_blue'
    assert_rule_uses '#battery,' '@text_main'
    assert_rule_uses '#clock:hover' '@bg_dark'
    assert_rule_uses '#custom-pacman:hover' '@accent_blue'
    printf 'PASS: shared Waybar rules use theme aliases\n'
}

verify_bundle_contract
verify_shared_waybar
assert_ghostty_clean_config
