#!/bin/bash

# ==========================================
# CONFIGURACIÓN Y COLORES
# ==========================================
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()     { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
error()   { echo -e "${RED}[ERROR]${NC} $1"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }
note()    { echo -e "${CYAN}[NOTE]${NC} $1"; }

# ==========================================
# ADVERTENCIA INICIAL
# ==========================================
echo ""
warn "Este script instala el entorno personal de agustin (Hyprland + TokyoNight)."
warn "Está pensado para Arch Linux con GPU AMD."
warn "Revisá el script antes de ejecutarlo y comentá lo que no necesites."
echo ""
note "Secciones marcadas con [PERSONAL] son específicas del autor y opcionales."
note "Secciones marcadas con [AMD] requieren GPU AMD."
echo ""
read -rp "¿Continuar? [s/N] " confirm
[[ "$confirm" =~ ^[sS]$ ]] || { log "Abortado."; exit 0; }

# No correr como root
if [ "$EUID" -eq 0 ]; then
  error "No ejecutes este script como root. Pedirá sudo cuando lo necesite."
  exit 1
fi

DOTFILES_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ==========================================
# 1. ACTUALIZACIÓN Y DEPENDENCIAS BASE
# ==========================================
log "Actualizando sistema e instalando base-devel y git..."
sudo pacman -Syu --noconfirm base-devel git

# ==========================================
# 2. INSTALACIÓN DE PARU (AUR HELPER)
# ==========================================
if ! command -v paru &> /dev/null; then
    log "Instalando paru (AUR helper)..."
    git clone https://aur.archlinux.org/paru.git /tmp/paru-install
    cd /tmp/paru-install
    makepkg -si --noconfirm
    cd "$DOTFILES_DIR"
else
    success "paru ya está instalado."
fi

# ==========================================
# 3. PAQUETES OFICIALES (PACMAN)
# ==========================================
log "Instalando paquetes base..."

PACKAGES=(
    # Shell
    zsh
    stow

    # Core Wayland / Hyprland
    hyprland
    hyprlock
    hyprpaper
    hypridle
    hyprsunset
    hyprpolkitagent
    waybar
    wofi
    dunst
    xdg-desktop-portal-hyprland
    xdg-desktop-portal-gtk

    # Terminal y herramientas de shell
    ghostty
    zellij
    neovim
    yazi
    fzf
    eza
    bat
    zoxide
    ripgrep
    fd
    wl-clipboard
    unzip
    reflector
    lazygit

    # Administrador de archivos
    thunar
    gvfs

    # Gestión de energía
    upower
    power-profiles-daemon

    # Theming Qt/GTK
    qt5ct
    qt6ct
    nwg-look
    kvantum

    # Dependencias para hyprpm (compilación de plugins)
    cpio
    cmake
    meson
    hyprland-headers  # Solo aplica con hyprland del repo oficial (no -git)
)

# ==========================================
# 4. PAQUETES AUR
# ==========================================

AUR_PACKAGES=(
    oh-my-posh-bin      # Prompt de shell (configurado en .zshrc)
    fnm-bin             # Node version manager (usado en .zshrc)
    nwg-drawer-bin      # App launcher
    nwg-dock-hyprland   # Dock
    eww                 # Widgets
    wlogout             # Menú de logout

    # [PERSONAL] Temas TokyoNight — hardcodeados en los configs de este repo.
    # Si querés otro tema, comentá estas líneas y configurá el tuyo manualmente.
 #   tokyonight-gtk-theme-git   # gtk-theme-name=TokyoNight-zk
#  tokyonight-icon-theme-git  # gtk-icon-theme-name=TokyoNight-SE
)

# [AMD] corectrl — control de GPU AMD. Comentar si no tenés GPU AMD.
AMD_PACKAGES=(
    corectrl
)

echo ""
read -rp "[AMD] ¿Tenés GPU AMD? Instalar corectrl [s/N] " amd_confirm
if [[ "$amd_confirm" =~ ^[sS]$ ]]; then
    AUR_PACKAGES+=("${AMD_PACKAGES[@]}")
    note "corectrl será instalado."
else
    warn "corectrl omitido. Si lo necesitás después: paru -S corectrl"
    warn "Además, comentá la línea 'exec-once = ... corectrl' en hyprland.conf"
fi

paru -S --needed --noconfirm "${PACKAGES[@]}" "${AUR_PACKAGES[@]}"

# ==========================================
# 5. ZSH COMO SHELL POR DEFECTO
# ==========================================
if [ "$SHELL" != "$(command -v zsh)" ]; then
    log "Cambiando shell por defecto a zsh..."
    chsh -s "$(command -v zsh)"
    success "Shell cambiado a zsh. Reiniciá sesión para que tome efecto."
else
    success "zsh ya es el shell por defecto."
fi

# ==========================================
# 6. FUENTES Y CURSOR THEME
# ==========================================
log "Instalando fuentes (CaskaydiaCove, CaskaydiaM, Hack Nerd Font)..."
mkdir -p "$HOME/.local/share/fonts"
cp -r "$DOTFILES_DIR/assets/fonts/"* "$HOME/.local/share/fonts/"
fc-cache -f
success "Fuentes instaladas y caché actualizado."

log "Instalando cursor theme (volantes_cursors)..."
mkdir -p "$HOME/.local/share/icons"
cp -r "$DOTFILES_DIR/assets/icons/"* "$HOME/.local/share/icons/"
success "Cursor theme instalado en ~/.local/share/icons/"

# ==========================================
# 7. DESPLEGAR DOTFILES CON STOW
# ==========================================
log "Desplegando dotfiles con GNU stow..."
cd "$DOTFILES_DIR"

# stow crea symlinks de todo el contenido del repo hacia $HOME.
# .config/* → ~/.config/*, .zshrc → ~/.zshrc, etc.
# --restow: rehace los symlinks (seguro para re-ejecuciones)
# --no-folding: crea los directorios intermedios en vez de enlazar carpetas enteras
if stow --target="$HOME" --no-folding --restow . 2>&1; then
    success "Dotfiles desplegados con stow."
else
    error "stow encontró conflictos. Revisá los archivos que ya existen en $HOME."
    error "Podés hacer backup de los conflictos y re-ejecutar."
    exit 1
fi

# Directorio para historial y caché de zsh (no tracked en el repo)
mkdir -p "$HOME/.config/zsh"

# ==========================================
# 8. [PERSONAL] PLUGINS HYPRLAND (hyprpm)
# ==========================================
# Los plugins configurados en este repo son: split-monitor-workspaces, hyprbars,
# borders-plus-plus, hyprscrolling, hyprwinwrap.
# Requieren que hyprland-headers coincida con la versión instalada.
echo ""
read -rp "[PERSONAL] ¿Instalar plugins de Hyprland via hyprpm? [s/N] " hyprpm_confirm
if [[ "$hyprpm_confirm" =~ ^[sS]$ ]]; then
    log "Inicializando hyprpm..."
    hyprpm update || warn "hyprpm update falló (Hyprland debe estar corriendo para esto)"
    hyprpm add https://github.com/hyprwm/hyprland-plugins || warn "Fallo al agregar repo de plugins"

    # split-monitor-workspaces: solo habilitar si hay más de un monitor conectado
    MONITOR_COUNT=$(hyprctl monitors -j 2>/dev/null | python3 -c "import sys,json; print(len(json.load(sys.stdin)))" 2>/dev/null || echo "0")
    if [ "$MONITOR_COUNT" -gt 1 ]; then
        hyprpm enable split-monitor-workspaces
        success "split-monitor-workspaces habilitado ($MONITOR_COUNT monitores detectados)."
    else
        hyprpm disable split-monitor-workspaces 2>/dev/null || true
        warn "split-monitor-workspaces deshabilitado (solo $MONITOR_COUNT monitor detectado)."
        note "Se habilitará automáticamente al conectar un segundo monitor (ver toggle-split-monitor.sh)."
    fi

    hyprpm enable hyprbars
    hyprpm enable borders-plus-plus
    hyprpm enable hyprscrolling
    hyprpm enable hyprwinwrap
    success "Plugins habilitados."
else
    warn "Plugins omitidos. Comentá la línea 'hyprpm reload' en hyprland.conf si no los instalás."
fi

# ==========================================
# 9. VARIABLES DE ENTORNO GLOBALES (Qt/GTK/Wayland)
# ==========================================
log "Configurando variables de entorno en /etc/profile.d/..."

sudo tee /etc/profile.d/qt-theme.sh > /dev/null <<'EOF'
export QT_QPA_PLATFORMTHEME=qt5ct
export QT_STYLE_OVERRIDE=kvantum
EOF

sudo tee /etc/profile.d/wayland-vars.sh > /dev/null <<'EOF'
export MOZ_ENABLE_WAYLAND=1
export GDK_BACKEND=wayland,x11,*
EOF

sudo chmod +x /etc/profile.d/qt-theme.sh
sudo chmod +x /etc/profile.d/wayland-vars.sh
success "Variables de entorno configuradas."

# ==========================================
# 10. [PERSONAL] APLICAR TEMAS GTK
# ==========================================
# Estos valores coinciden con los settings.ini del repo (TokyoNight).
# Si usás otro tema, cambiá los valores aquí y en .config/gtk-3.0/settings.ini
log "Aplicando temas GTK via gsettings..."
gsettings set org.gnome.desktop.interface gtk-theme    "TokyoNight-zk"
gsettings set org.gnome.desktop.interface icon-theme   "TokyoNight-SE"
gsettings set org.gnome.desktop.interface cursor-theme "volantes_cursors"
gsettings set org.gnome.desktop.interface cursor-size  24
gsettings set org.gnome.desktop.interface font-name    "CaskaydiaMono Nerd Font Mono Bold 10"
gsettings set org.gnome.desktop.interface color-scheme "prefer-dark"
success "Temas GTK aplicados."

# ==========================================
# 11. SERVICIOS
# ==========================================
log "Habilitando servicios..."

if systemctl is-active --quiet tlp; then
    warn "TLP detectado y activo. Omitiendo power-profiles-daemon para evitar conflictos."
else
    sudo systemctl enable --now power-profiles-daemon.service
    success "power-profiles-daemon habilitado."
fi

sudo systemctl enable --now upower
success "upower habilitado."

# ==========================================
# RESUMEN FINAL
# ==========================================
echo ""
echo -e "${GREEN}============================================${NC}"
success "Instalación completada."
echo -e "${GREEN}============================================${NC}"
echo ""
note "Pasos siguientes:"
note "  1. Reiniciá la sesión o el sistema."
note "  2. Abrí 'qt5ct' → seleccioná estilo 'kvantum'."
note "  3. Ejecutá 'nwg-look' para confirmar temas GTK."
if [[ ! "$amd_confirm" =~ ^[sS]$ ]]; then
    warn "  4. Comentá 'exec-once = ... corectrl' en ~/.config/hypr/hyprland.conf"
fi
echo ""
