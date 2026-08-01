#!/usr/bin/env bash
#
# MrUse77/dotfiles — bootstrap installer
#
# Uso:
#   curl -fsSL https://raw.githubusercontent.com/MrUse77/dotfiles/main/scripts/install.sh | bash
#
# Variables de entorno (opcionales):
#   DOTFILES_DIR     directorio destino (default: $HOME/.cache/dotfiles)
#   DOTFILES_REPO    URL del repo (default: https://github.com/MrUse77/dotfiles.git)
#   DOTFILES_BRANCH  rama a instalar (default: main)
#
set -euo pipefail

DOTFILES_DIR="${DOTFILES_DIR:-$HOME/.cache/dotfiles}"
DOTFILES_REPO="${DOTFILES_REPO:-https://github.com/MrUse77/dotfiles.git}"
DOTFILES_BRANCH="${DOTFILES_BRANCH:-main}"

say() { printf '\033[1;34m[dots]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[dots]\033[0m error: %s\n' "$*" >&2; exit 1; }

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "falta '$1'. Instalalo primero, por ejemplo: sudo pacman -S $2"
  fi
}

# latest_release prints the highest SemVer tag (vMAJOR.MINOR.PATCH) of the
# repository. The installer always targets the last stable release, never
# the development branch.
latest_release() {
  git ls-remote --tags --refs "$DOTFILES_REPO" 2>/dev/null \
    | sed 's|.*refs/tags/||' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -n 1
}

main() {
  if [ -n "${DOTFILES_BRANCH:-}" ]; then
    REF="$DOTFILES_BRANCH"
  else
    REF="$(latest_release)"
    if [ -z "$REF" ]; then
      die "no se encontró ninguna release (tag vX.Y.Z) en $DOTFILES_REPO"
    fi
    say "Usando la última release: $REF"
  fi

  say "Instalador de dotfiles (MrUse77)"
  say "Directorio: $DOTFILES_DIR | Release: $REF"

  require git "git"
  require go "go"

  if [ -d "$DOTFILES_DIR/.git" ]; then
    say "Actualizando el clon existente en $DOTFILES_DIR..."
    git -C "$DOTFILES_DIR" fetch origin "$REF"
    git -C "$DOTFILES_DIR" checkout --force --detach FETCH_HEAD
    git -C "$DOTFILES_DIR" submodule update --init --recursive
  elif [ -e "$DOTFILES_DIR" ]; then
    die "$DOTFILES_DIR ya existe pero no es un clon de dotfiles. Movelo o borralo y volvé a intentar."
  else
    say "Clonando $DOTFILES_REPO en $DOTFILES_DIR..."
    git clone --recurse-submodules --branch "$REF" "$DOTFILES_REPO" "$DOTFILES_DIR"
  fi

  say "Compilando e instalando el binario en ~/.local/bin/moonarch-cli..."
  mkdir -p "$HOME/.local/bin"
  (cd "$DOTFILES_DIR/cli" && go build -o "$HOME/.local/bin/moonarch-cli" .)

  say "Ejecutando 'moonarch-cli install'..."  # Bajo 'curl | bash' el stdin es el pipe de curl, que ya se cerró: el menú
  # interactivo necesita el TTY real para capturar teclas.
  if [ ! -t 0 ]; then
    if [ -r /dev/tty ]; then
      exec < /dev/tty
    else
      die "se necesita una terminal interactiva para ejecutar 'moonarch-cli install'"
    fi
  fi
  exec "$HOME/.local/bin/moonarch-cli" install
}

main "$@"
