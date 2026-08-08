#!/usr/bin/env bash
set -euo pipefail

usage() {
    echo "Usage: $0 ABSOLUTE_TARGET_DIRECTORY" >&2
}

if [[ $# -ne 1 ]]; then
    usage
    exit 2
fi

if [[ "$1" != /* ]]; then
    echo "Target directory must be an absolute path." >&2
    usage
    exit 2
fi

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
target="$(realpath -m -- "$1")"
host_home="$(realpath -m -- "$HOME")"

if [[ "$target" == "$host_home" ]]; then
    echo "Refusing to Stow into the canonical host home directory." >&2
    exit 2
fi

mkdir -p -- "$target"
exec stow --dir="$repo_root" --target="$target" home
