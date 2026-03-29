# dotfiles

Configuración personal para Arch Linux con Hyprland.

> **Nota:** Este repo está diseñado para mi setup específico (GPU AMD, tema TokyoNight).
> El `install.sh` te avisa sobre las partes opcionales antes de hacer cualquier cosa.

## Stack

| Categoría | Herramienta |
|---|---|
| Compositor | Hyprland |
| Terminal | Ghostty + Zellij |
| Shell | Zsh + Oh My Posh |
| Editor | Neovim |
| Bar | Waybar |
| Launcher | nwg-drawer |
| Notificaciones | Dunst |
| File manager | Thunar + Yazi |
| Tema | TokyoNight |
| Cursor | volantes_cursors |

---

## Instalación

### Requisitos previos

- Arch Linux (o derivado)
- Conexión a internet
- Hyprland **no** corriendo al momento de instalar (algunos pasos lo requieren)

### 1. Clonar el repo

```bash
git clone --recurse-submodules https://github.com/MrUse77/dotfiles.git ~/dotfiles
```

> El flag `--recurse-submodules` es importante. Sin él los plugins de zsh y el
> config de neovim quedarán como carpetas vacías.

Si ya clonaste sin ese flag:

```bash
git submodule update --init --recursive
```

### 2. Correr el script de instalación

```bash
cd ~/dotfiles
bash install.sh
```

El script va a:

1. Actualizar el sistema e instalar `base-devel` y `git`
2. Instalar `paru` (AUR helper) si no está
3. Instalar todos los paquetes necesarios (oficiales + AUR)
4. Configurar `zsh` como shell por defecto
5. Instalar fuentes y el cursor theme
6. Desplegar los configs con `stow` (symlinks hacia `~/.config/`)
7. Preguntar si instalar los plugins de Hyprland via `hyprpm`
8. Configurar variables de entorno globales para Qt/GTK/Wayland
9. Aplicar temas GTK via `gsettings`
10. Habilitar servicios (`upower`, `power-profiles-daemon`)

### 3. Después de instalar

1. Reiniciar sesión o el sistema
2. Abrir `qt5ct` → seleccionar estilo **kvantum**
3. Ejecutar `nwg-look` para confirmar los temas GTK

---

## Estructura del repo

```
dotfiles/
├── .config/
│   ├── dunst/          # Notificaciones
│   ├── eww/            # Widgets
│   ├── ghostty/        # Terminal
│   ├── gtk-3.0/        # Tema GTK3
│   ├── gtk-4.0/        # Tema GTK4
│   ├── hypr/           # Hyprland, hyprlock, hyprpaper, hypridle, hyprsunset
│   │   └── scripts/    # Scripts de autostart (toggle-split-monitor, etc.)
│   ├── nvim/           # Config de Neovim (submodule → MrUse77/Nvim-config)
│   ├── nwg-dock-hyprland/
│   ├── nwg-drawer/
│   ├── swaync/
│   ├── waybar/
│   ├── wofi/
│   └── zellij/
├── .zsh_plugins/       # Plugins de zsh (submodules, ver abajo)
├── assets/
│   ├── fonts/          # CaskaydiaCove, CaskaydiaM, Hack Nerd Font
│   └── icons/          # volantes_cursors
├── oh-my-posh/         # Temas de prompt (.omp.json)
├── .zshrc
├── .gtkrc-2.0
└── install.sh
```

Los configs se despliegan con `stow`, que crea symlinks desde este repo hacia `$HOME`.
Esto significa que cualquier cambio que hagas en `~/.config/waybar/config.jsonc`, por
ejemplo, en realidad está editando el archivo dentro del repo directamente.

---

## Submodules

Este repo usa **git submodules** para manejar repos externos sin duplicar su código.

### ¿Qué es un submodule?

En vez de copiar el código de un proyecto externo dentro de tu repo, un submodule
guarda solo un puntero: la URL del repo y el commit exacto en el que estás parado.

```
[submodule ".zsh_plugins/zsh-autosuggestions"]
    url = https://github.com/zsh-users/zsh-autosuggestions
    # git guarda internamente a qué commit apunta
```

### Submodules en este repo

| Path | Repo | Descripción |
|---|---|---|
| `.config/nvim` | `MrUse77/Nvim-config` | Config personal de Neovim |
| `.zsh_plugins/fzf-tab` | `Aloxaf/fzf-tab` | Completado con fzf |
| `.zsh_plugins/zsh-autosuggestions` | `zsh-users/zsh-autosuggestions` | Sugerencias inline |
| `.zsh_plugins/zsh-history-substring-search` | `zsh-users/zsh-history-substring-search` | Búsqueda en historial |
| `.zsh_plugins/zsh-syntax-highlighting` | `zsh-users/zsh-syntax-highlighting` | Highlight de sintaxis |

### Comandos útiles

**Inicializar submodules después de clonar sin `--recurse-submodules`:**
```bash
git submodule update --init --recursive
```

**Actualizar todos los submodules a su último commit:**
```bash
git submodule update --remote --merge
```

**Actualizar solo uno:**
```bash
git submodule update --remote --merge .zsh_plugins/zsh-autosuggestions
```

**Ver en qué commit está cada submodule:**
```bash
git submodule status
```

**Trabajar en el config de nvim** (es un repo independiente):
```bash
cd ~/.config/nvim   # o ~/dotfiles/.config/nvim, es el mismo archivo via stow
git add .
git commit -m "..."
git push            # pushea a MrUse77/Nvim-config, no a dotfiles
```

Después de hacer cambios en un submodule, el dotfiles repo detecta que el puntero
cambió. Para registrarlo:
```bash
cd ~/dotfiles
git add .config/nvim
git commit -m "nvim: actualizar a último commit"
```

---

## Actualizar dotfiles en una nueva máquina

```bash
# Clonar
git clone --recurse-submodules https://github.com/MrUse77/dotfiles.git ~/dotfiles

# Instalar
cd ~/dotfiles && bash install.sh
```

## Sincronizar cambios desde otra máquina

```bash
cd ~/dotfiles
git pull
git submodule update --recursive   # actualiza los submodules al commit que indica el repo
```
