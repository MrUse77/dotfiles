# Theme Engine Tasks

## Implementation

- [x] 1.1 Create `pkg/theme/waybar.go` to rewrite `~/.config/waybar/colors.css` and reload Waybar.
- [x] 1.2 Create `pkg/theme/ghostty.go` to regex replace Ghostty colors in `~/.config/ghostty/config`.
- [x] 1.3 Create `pkg/theme/hyprpaper.go` to copy wallpaper and reload Hyprpaper.
- [x] 1.4 Update `cmd/theme.go` to instantiate a dummy `theme.Theme` (e.g. Dracula) and call the 3 functions above.

## Review Workload Forecast
- 400-line budget risk: Low
- Chained PRs recommended: No
- Decision needed before apply: No
- Delivery Strategy: `single-pr`
