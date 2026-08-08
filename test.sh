#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
image="dotfiles-tester"

echo "================================================="
echo "  Validating the MoonArch theme palette contract"
echo "================================================="
bash "$repo_root/tests/moonarch-theme-palette_test.sh"

echo "================================================="
echo "  Building isolated Arch Linux test environment"
echo "================================================="
docker build -t "$image" -f "$repo_root/Dockerfile.test" "$repo_root"

echo "================================================="
echo "  Validating the isolated Stow development helper"
echo "================================================="
docker run --rm \
    -v "$repo_root":/home/tester/dotfiles:ro \
    --name dotfiles-sandbox \
    "$image" \
    bash -lc '
        set -euo pipefail
        target="$(mktemp -d)"

        bash scripts/stow-dev.sh "$target"
        test -L "$target/.zshrc"
        test "$(readlink -f "$target/.zshrc")" = /home/tester/dotfiles/home/.zshrc
        test -L "$target/.config"

        if bash scripts/stow-dev.sh relative-target; then
            echo "expected relative target to be rejected" >&2
            exit 1
        fi
        if bash scripts/stow-dev.sh "$HOME"; then
            echo "expected the canonical home directory to be rejected" >&2
            exit 1
        fi
    '

echo "Isolated Stow validation passed."
