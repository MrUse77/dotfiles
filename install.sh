#!/bin/bash

# ==========================================
# CONFIGURACIÓN Y COLORES
# ==========================================
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[OK]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# Verificación de no correr como root (paru no debe correrse como root)
if [ "$EUID" -eq 0 ]; then
  error "Por favor, no ejecutes este script como root. El script pedirá sudo cuando lo necesite."
  exit 1
fi

# ==========================================
# 1. ACTUALIZACIÓN Y DEPENDENCIAS BASE
# ==========================================
log "Actualizando sistema e instalando base-devel y git..."
sudo pacman -Syu --noconfirm base-devel git

# ==========================================
# 2. INSTALACIÓN DE PARU (AUR HELPER)
# ==========================================
if ! command -v paru &> /dev/null; then
    log "Paru no encontrado. Instalando..."
    cd /tmp
    git clone https://aur.archlinux.org/paru.git
    cd paru
    makepkg -si --noconfirm
    cd -
else
    success "Paru ya está instalado."
fi

# ==========================================
# 3. PAQUETES OFICIALES (PACMAN)
# ==========================================
log "Instalando paquetes..."

# Core + Utils  + Theming Tools + Neovim Deps
PACKAGES=(
    # Core Wayland/Hyprland
    hyprland
    hyprlock
    waybar         # Casi mandatorio si usas hyprland, aunque no lo pediste explícitamente
    wofi           # Launcher alternativo por si nwg-drawer falla
    hyprpolkitagent
    
    # Terminal & Shell Utils
    zellij
    neovim
    yazi
    ripgrep        # Nvim dep (telescope)
    fd             # Nvim dep
    wl-clipboard   # Clipboard para nvim en wayland
    unzip
    npm            # Nvim dep (LSP servers)
    nodejs
    
    # Power Management
    upower
    power-profiles-daemon
    
    # Theming & QT/GTK
    qt5ct
    qt6ct
    nwg-look       # GUI para configurar GTK en Wayland
    kvantum        # Temas SVG para QT
 
    # Hyprpm dependencies (compilación de plugins)
    cpio
    cmake
    meson
    hyprland-headers # CRÍTICO para hyprpm en versión estable
)

# ==========================================
# 4. PAQUETES AUR
# ==========================================

AUR_PACKAGES=(
    ghostty         # Ojo: Compilación pesada
    nwg-drawer-bin  # Binario para ahorrar tiempo de compilación Go
    swaync          # Notificaciones
    wlogout         # Menú de salida
    bibata-cursor-theme # Cursor moderno recomendado
)

paru -S --needed --noconfirm "${PACKAGES[@]}" "${AUR_PACKAGES[@]}"

# Fuentes
cp ./assets/fonts/* /home/$USER/.local/share/fonts
#curosor
cp ./assets/icons/* /home/$USER/.local/share/icons


# ==========================================
# 5. CONFIGURACIÓN DE PLUGINS HYPRLAND
# ==========================================
log "Inicializando hyprpm (Hyprland Plugin Manager)..."
# Esto a menudo falla si los headers no coinciden, intentamos actualizar
hyprpm update

hyprpm add https://github.com/hyprwm/hyprland-plugins
hyprpm enable split-monitor-workspaces 
hyprpm enable hyprbars 
hyprpm enable borders-plus-plus 
hyprpm enable hyprscrolling 
hyprpm enable hyprwinwrap


# ==========================================
# 6. CONFIGURACIÓN DE VARIABLES DE ENTORNO (THEMING)
# ==========================================
log "Configurando variables de entorno para temas QT/GTK..."

# Creamos un archivo en profile.d para que sea global
echo "export QT_QPA_PLATFORMTHEME=qt5ct" | sudo tee /etc/profile.d/qt-theme.sh
echo "export QT_STYLE_OVERRIDE=kvantum" | sudo tee -a /etc/profile.d/qt-theme.sh
# Forzar backend wayland para GTK y QT
echo "export MOZ_ENABLE_WAYLAND=1" | sudo tee /etc/profile.d/wayland-vars.sh
echo "export GDK_BACKEND=wayland,x11" | sudo tee -a /etc/profile.d/wayland-vars.sh

# Aplicar permisos
sudo chmod +x /etc/profile.d/qt-theme.sh
sudo chmod +x /etc/profile.d/wayland-vars.sh

# ==========================================
# 7. APLICACIÓN DE TEMAS (INTENTO AUTOMÁTICO)
# ==========================================
log "Aplicando configuración GTK básica..."

# Crear configuración GTK-3.0 si no existe
mkdir -p ~/.config/gtk-3.0
mkdir -p ~/.config/gtk-4.0

CAT_CONFIG="[Settings]
gtk-theme-name=Arc-Dark
gtk-icon-theme-name=Papirus-Dark
gtk-font-name=Hack Nerd Font 11
gtk-cursor-theme-name=Bibata-Modern-Ice
gtk-application-prefer-dark-theme=1
"

# Solo escribimos si no existe para no destruir tu config actual
if [ ! -f ~/.config/gtk-3.0/settings.ini ]; then
    echo "$CAT_CONFIG" > ~/.config/gtk-3.0/settings.ini
    success "Configuración GTK-3 generada."
else
    log "Configuración GTK-3 ya existente. Omitiendo sobreescritura."
fi

# Configuración básica de gsettings (GNOME/GTK apps)
gsettings set org.gnome.desktop.interface gtk-theme "Arc-Dark"
gsettings set org.gnome.desktop.interface icon-theme "Papirus-Dark"
gsettings set org.gnome.desktop.interface cursor-theme "Bibata-Modern-Ice"
gsettings set org.gnome.desktop.interface font-name "Hack Nerd Font 11"

# ==========================================
# 8. SERVICIOS
# ==========================================
log "Habilitando servicios de energía..."

# Verificar conflicto con TLP
if systemctl is-active --quiet tlp; then
    error "TLP detectado. No se habilitará power-profiles-daemon para evitar conflictos."
else
    sudo systemctl enable --now power-profiles-daemon.service
    success "power-profiles-daemon habilitado."
fi

sudo systemctl enable --now upower

log "---------------------------------------------------------"
success "Instalación completada." 
log "Pasos siguientes obligatorios:"
log "1. Reinicia tu sesión o el equipo."
log "2. Abre 'qt5ct' y selecciona el estilo 'kvantum' o 'gtk2'."
log "3. Ejecuta 'nwg-look' para confirmar los temas GTK."
log "4. Configura tu hyprland.conf para iniciar 'swaync' y 'polkit-gnome'."
log "---------------------------------------------------------"
