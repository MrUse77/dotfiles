#!/usr/bin/env bash
#
# MrUse77/dotfiles — bootstrap installer
#
# Uso:
#   curl -fsSL https://raw.githubusercontent.com/MrUse77/dotfiles/main/scripts/install.sh | bash
#
# Baja el binario moonarch-cli de la última release estable (con verificación
# de checksum) y lo ejecuta. El binario se encarga del resto: clona el repo
# en ~/.cache/dotfiles si falta, instala los dotfiles y deja los backups.
# No requiere Go ni Git.
#
set -euo pipefail

REPO="MrUse77/dotfiles"
API="https://api.github.com/repos/${REPO}"

say() { printf '\033[1;34m[dots]\033[0m %s\n' "$*"; }
die() { printf '\033[1;31m[dots]\033[0m error: %s\n' "$*" >&2; exit 1; }

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "falta '$1'. Instalalo primero, por ejemplo: sudo pacman -S $2"
  fi
}

# latest_release returns the tag of the newest non-draft, non-prerelease
# release of the repository.
latest_release() {
  curl -fsSL "${API}/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' \
    | head -n 1
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
  local base="https://github.com/${REPO}/releases/download/${REF}"
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
  REF="$(latest_release)"
  if [ -z "$REF" ]; then
    die "no se pudo determinar la última release de ${REPO}"
  fi
  if [[ "$REF" == config-v* ]]; then
    die "la última release es una release de configuración ($REF), no un binario compatible"
  fi
  say "Usando la última release: $REF"

  install_release_binary

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
