# TUI Installer Tasks

## Implementation

- [x] 1.1 Update `cmd/install.go` to import `github.com/charmbracelet/huh`.
- [x] 1.2 Implement the interactive `huh.Form` in the `Run` function of `installCmd`.
- [x] 1.3 Add conditional logic: if the user aborts, `os.Exit(0)`.
- [x] 1.4 Wire the chosen install mode to either copy files (User mode) or run `stow` (Dev mode) in future implementations, but print the chosen settings for now.

## Review Workload Forecast
- 400-line budget risk: Low
- Chained PRs recommended: No
- Decision needed before apply: No
- Delivery Strategy: `single-pr`
