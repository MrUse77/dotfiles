# Theme Engine Design

## Overview
Implement the first phase of the dotfiles CLI theme manager for Ghostty, Waybar, and Hyprpaper.

## Architecture
- **Waybar**: Go script overwrites `~/.config/waybar/colors.css` using the values from `theme.Waybar`, then executes `killall -SIGUSR2 waybar`.
- **Ghostty**: Go script reads `~/.config/ghostty/config`, replaces `palette = X=#color` and `background = #color` lines using the values from `theme.Ghostty`, and writes it back.
- **Hyprpaper**: Go script copies `theme.Wallpaper` to `~/.config/hypr/assets/wallpaper.png` and executes `hyprctl hyprpaper wallpaper ",~/.config/hypr/assets/wallpaper.png"`.

## Constraints
- Use Go standard library for file manipulation.
- Execute external commands using `os/exec`.
