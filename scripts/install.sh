#!/usr/bin/env bash
#
# MrUse77/dotfiles — bootstrap installer
#
# Uso:
#   curl -fsSL https://raw.githubusercontent.com/MrUse77/dotfiles/main/scripts/install.sh | bash
#
# Por defecto instala la última release estable (tag vX.Y.Z): clona el repo
# en ~/.cache/dotfiles y baja el binario publicado de la release (sin Go).
#
# Variables de entorno (opcionales):
#   DOTFILES_DIR     directorio destino (default: $HOME/.cache/dotfiles)
#   DOTFILES_REPO    URL del repo (default: https://github.com/MrUse77/dotfiles.git)
#   DOTFILES_BRANCH  rama para desarrollo (override; compila con Go, versión dev)
#
set -euo pipefail

DOTFILES_DIR="${DOTFILES_DIR:-$HOME/.cache/dotfiles}"
DOTFILES_REPO="${DOTFILES_REPO:-https://github.com/MrUse77/dotfiles.git}"

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

# install_release_binary downloads the release binary for the host
# architecture and verifies its checksum against the release's SHA256SUMS.
install_release_binary() {
  local arch
  case "$(uname -m)" in
    x86_64) arch="amd64" ;;
    aarch64 | arm64) arch="arm64" ;;
    *) die "arquitectura no soportada: $(uname -m)" ;;
  esac

  require curl "curl"

  mkdir -p "$HOME/.local/bin"
  local bin="$HOME/.local/bin/moonarch-cli"
  local base="https://github.com/MrUse77/dotfiles/releases/download/${REF}"
  local file="moonarch-cli-linux-${arch}"

  say "Descargando moonarch-cli ${REF} (linux/${arch})..."
  curl -fsSL "${base}/${file}" -o "$bin.tmp"

  if command -v sha256sum >/dev/null 2>&1; then
    local want
    want="$(curl -fsSL "${base}/SHA256SUMS.txt" 2>/dev/null | awk -v f="$file" '$2 == f {print $1}' | head -n 1 || true)"
    if [ -n "$want" ]; then
      local got
      got="$(sha256sum "$bin.tmp" | awk '{print $1}')"
      if [ "$got" != "$want" ]; then
        rm -f "$bin.tmp"
        die "el checksum del binario no coincide. Reintentá o reportá el problema."
      fi
      say "Checksum verificado."
    else
      say "AVISO: no se pudo verificar el checksum (release sin SHA256SUMS)."
    fi
  fi

  chmod +x "$bin.tmp"
  mv "$bin.tmp" "$bin"
  say "Binario instalado en $bin"
}

main() {
  local dev_build=0
  if [ -n "${DOTFILES_BRANCH:-}" ]; then
    REF="$DOTFILES_BRANCH"
    dev_build=1
  else
    REF="$(latest_release)"
    if [ -z "$REF" ]; then
      die "no se encontró ninguna release (tag vX.Y.Z) en $DOTFILES_REPO"
    fi
    say "Usando la última release: $REF"
  fi

  say "Instalador de dotfiles (MrUse77)"
  say "Directorio: $DOTFILES_DIR | Ref: $REF"

  require git "git"

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

  if [ "$dev_build" = "1" ]; then
    require go "go"
    say "Modo desarrollo: compilando moonarch-cli desde $REF..."
    mkdir -p "$HOME/.local/bin"
    (cd "$DOTFILES_DIR/cli" && go build -o "$HOME/.local/bin/moonarch-cli" .)
  else
    install_release_binary
  fi

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
