#!/usr/bin/env bash
#
# MrUse77/dotfiles — bootstrap installer
#
# Uso:
#   curl -fsSL https://raw.githubusercontent.com/MrUse77/dotfiles/main/scripts/install.sh | bash
#
# Variables de entorno (opcionales):
#   DOTFILES_DIR     directorio destino (default: $HOME/dotfiles)
#   DOTFILES_REPO    URL del repo (default: https://github.com/MrUse77/dotfiles.git)
#   DOTFILES_BRANCH  rama a instalar (default: main)
#
set -euo pipefail

DOTFILES_DIR="${DOTFILES_DIR:-$HOME/dotfiles}"
DOTFILES_REPO="${DOTFILES_REPO:-https://github.com/MrUse77/dotfiles.git}"
DOTFILES_BRANCH="${DOTFILES_BRANCH:-main}"

say() { printf '\033[1;34m[dots]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[dots]\033[0m error: %s\n' "$*" >&2; exit 1; }

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "falta '$1'. Instalalo primero, por ejemplo: sudo pacman -S $2"
  fi
}

main() {
  say "Instalador de dotfiles (MrUse77)"
  say "Directorio: $DOTFILES_DIR | Rama: $DOTFILES_BRANCH"

  require git "git"
  require go "go"

  if [ -d "$DOTFILES_DIR/.git" ]; then
    say "Actualizando el clon existente en $DOTFILES_DIR..."
    git -C "$DOTFILES_DIR" fetch origin
    git -C "$DOTFILES_DIR" checkout "$DOTFILES_BRANCH"
    git -C "$DOTFILES_DIR" pull --ff-only origin "$DOTFILES_BRANCH"
    git -C "$DOTFILES_DIR" submodule update --init --recursive
  elif [ -e "$DOTFILES_DIR" ]; then
    die "$DOTFILES_DIR ya existe pero no es un clon de dotfiles. Movelo o borralo y volvé a intentar."
  else
    say "Clonando $DOTFILES_REPO en $DOTFILES_DIR..."
    git clone --recurse-submodules -b "$DOTFILES_BRANCH" "$DOTFILES_REPO" "$DOTFILES_DIR"
  fi

  say "Compilando e instalando el binario en ~/.local/bin/moonarch-cli..."
  mkdir -p "$HOME/.local/bin"
  (cd "$DOTFILES_DIR/cli" && go build -o "$HOME/.local/bin/moonarch-cli" .)

  say "Ejecutando 'moonarch-cli install'..."
  # Bajo 'curl | bash' el stdin es el pipe de curl, que ya se cerró: el menú
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
