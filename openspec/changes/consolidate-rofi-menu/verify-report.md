# Verify Report: Consolidate Rofi Menu

## Result Contract

- **status:** FAIL
- **executive_summary:** All 23 specified acceptance criteria pass under focused static and mocked behavioral checks. Verification is nevertheless blocked by two CRITICAL issues: the repository's required `bash test.sh` suite fails because its MoonArch contract still requires removed `wofi.css` assets, and Strict TDD Mode is active but `apply-progress.md` has no `TDD Cycle Evidence` table.
- **artifacts:**
  - `openspec/changes/consolidate-rofi-menu/verify-report.md`
  - Inputs read: `spec.md`, `tasks.md`, `design.md`, `apply-progress.md`, `openspec/config.yaml`
- **next_recommended:** remediate
- **risks:** The legacy `wofi.css` contract in `tests/moonarch-theme-palette_test.sh` must be migrated to the new `rofi.rasi` contract. No archive is permitted until the test suite and strict-TDD evidence blockers are resolved.
- **skill_resolution:** none (no skill paths were injected)

## Structured Status and Action Context

```yaml
changeName: consolidate-rofi-menu
artifactStore: openspec
changeRoot: /home/agustin/Dev/dotfiles/openspec/changes/consolidate-rofi-menu
artifacts:
  design: done
  tasks: done
  applyProgress: done
  verifyReport: done
  specs: missing  # native status engine does not recognize the present spec.md
nativeStatus:
  applyState: blocked
  verify: blocked
  nextRecommended: spec
actionContext:
  mode: repo-local
  workspaceRoot: /home/agustin/Dev/dotfiles
  allowedEditRoots:
    - /home/agustin/Dev/dotfiles
```

The declared `spec.md` exists and was read directly. Native `gentle-ai sdd-status` still reports `specs: missing`, `verify: blocked`, and no review receipt. This is an archive blocker despite the direct acceptance-criteria evidence below.

## Acceptance Criteria Coverage

| # | Criterion | Result | Evidence |
|---:|---|---|---|
| 1 | Super+M opens Rofi drun | PASS | `hyprland.lua` binds `mainMod + M` to `menu`; `menu` is `rofi ... -show drun`. |
| 2 | Super+Tab opens window switcher | PASS | Binding invokes `windowMenu`, defined as `-show window`. |
| 3 | Super+R opens command runner | PASS | Binding invokes `runMenu`, defined as `-show run`. |
| 4 | Calculator accessible from Super+R | PASS | `config.rasi` declares `calc`, labels it Calculator, and enables `sidebar-mode`; README documents the optional calc dependency and normal run fallback. |
| 5 | Super+Shift+X opens powermenu | PASS | Binding invokes `powerMenu` (`-show powermenu`); protocol test emitted five choices. |
| 6 | Powermenu shows five actions | PASS | `ROFI_RETV=0` test returned exactly `lock,sleep,logout,reboot,shutdown`. |
| 7 | Lock executes immediately | PASS | Stub test recorded `hyprlock` without confirmation. |
| 8 | Sleep executes immediately | PASS | Stub test recorded `systemctl suspend` without confirmation. |
| 9 | Logout requires confirmation | PASS | Stub test proved cancel is inert and exact `confirm` dispatches `hyprctl dispatch exit`. |
| 10 | Reboot requires confirmation | PASS | Stub test proved exact `confirm` dispatches `systemctl reboot`; cancel is inert. |
| 11 | Shutdown requires confirmation | PASS | Stub test proved exact `confirm` dispatches `systemctl poweroff`; cancel is inert. |
| 12 | Cancel is safe | PASS | Cancel, empty result, nonzero launcher exit, malformed/unknown input, and unexpected `ROFI_RETV` made no stubbed action. |
| 13 | Tokyo Night theme applied | PASS | `theme.rasi` parses with Rofi and defines the six Tokyo Night fallback variables used by shared widget states. |
| 14 | MoonArch integration works | PASS | Actual-Rofi temporary-HOME test accepted a valid fragment in a 0600 composite and showed the fragment's `#112233` background override. |
| 15 | Fallback works without MoonArch | PASS | Missing-fragment path selects the base theme by inspection; actual-Rofi test of an invalid fragment printed the fallback notice and used no cached composite. |
| 16 | Hack Nerd Font used | PASS | `config.rasi` sets `font: "Hack Nerd Font 15"`. |
| 17 | Legacy configs removed | PASS | All three legacy directories are absent; every one of the 12 bundles has `rofi.rasi` and no `wofi.css`. |
| 18 | Hyprland bindings updated | PASS | All four specified Rofi workflow bindings and declarations are present; no `nwg-drawer`, `wofi`, or `nwg-dock` reference remains. |
| 19 | Existing bindings preserved | PASS | `Super+Shift+T` still invokes `~/.local/bin/moonarch/theme-selector`; `SUPER+SHIFT+L` still invokes `hyprlock`. |
| 20 | No CLI changes | PASS | `git diff -- cli/` was empty. |
| 21 | Prerequisites documented | PASS | README lists `rofi-wayland` as a prerequisite. |
| 22 | Calculator dependency documented | PASS | README lists optional `rofi-calc-wayland` (or equivalent) and `libqalculate`. |
| 23 | Waybar power button uses Rofi | PASS | `custom/power` invokes `launch -show powermenu`; neither `wlogout` nor `nwg-drawer` is present in Waybar config. |

## Task Completion

- Implementation tasks: **11/11 complete**.
- Unchecked implementation task lines: **none**.
- Apply progress explicitly records all T1–T11 as complete and the approved single-PR size exception.

## Validation Commands

| Command | Result | Evidence |
|---|---|---|
| `bash -n .config/rofi/scripts/launch` | PASS | Valid shell syntax. |
| `bash -n .config/rofi/scripts/powermenu` | PASS | Valid shell syntax. |
| `bash -n .local/bin/moonarch/theme-selector` | PASS | Valid shell syntax. |
| `rofi -theme /home/agustin/Dev/dotfiles/.config/rofi/theme.rasi -dump-theme` | PASS | Rofi parsed the fallback Tokyo Night theme. |
| Focused static verification of files, permissions, bindings, Waybar, legacy removal, bundle variables, `git diff -- cli/`, and README | PASS | Required four Rofi paths exist; scripts are executable; 12/12 fragments have all six `moonarch-*` variables. |
| Focused powermenu protocol/dispatch test with stubbed `hyprlock`, `systemctl`, `hyprctl`, and `rofi` | PASS | Fixed identifiers and safe confirmation semantics verified. |
| Actual-Rofi temporary-HOME valid-fragment composite test | PASS | Valid fragment overrides base theme; cached composite is mode `0600`. |
| Actual-Rofi temporary-HOME invalid-fragment fallback test | PASS | Invalid Rasi emits fallback notice and does not accept a composite. |
| `bash test.sh` | **FAIL** | `tests/moonarch-theme-palette_test.sh` stops at: `FAIL: protected Tokyo file is absent or not regular: .local/share/moonarch/themes/tokyo-night/wofi.css`. |

### Full-Test Failure Detail

`test.sh` invokes `tests/moonarch-theme-palette_test.sh` before its Docker checks. That test still lists `tokyo-night/wofi.css` as a protected file (lines 14, 22, and 32) and requires `wofi.css` in every bundle (line 438). This conflicts directly with the change's specified removal of all MoonArch `wofi.css` files. The Docker/Stow portion did not run because the palette test failed first.

## Strict TDD Compliance

Strict TDD Mode is active for this verification. The root OpenSpec configuration and delegated metadata say `strict_tdd: false`, but the governing runtime instruction enables Strict TDD Mode; the stricter requirement was applied.

| Check | Result | Details |
|---|---|---|
| TDD Cycle Evidence reported | **FAIL (CRITICAL)** | `apply-progress.md` contains no `TDD Cycle Evidence` table. |
| Test files reported/cross-referenced | **FAIL (CRITICAL)** | No test files or RED/GREEN evidence were reported for the 11 completed tasks. |
| GREEN confirmed | **FAIL (CRITICAL)** | The available full runner, `bash test.sh`, currently fails. |
| Assertion quality | N/A | No tests were created or modified by this change according to apply-progress and the worktree diff. |
| Test layer distribution | N/A | No change-specific test files were reported. |
| Changed-file coverage | N/A | Root config has no coverage tool. |

**Assertion quality:** No changed/created tests to audit; missing strict-TDD evidence remains a CRITICAL compliance failure.

## Review Workload / PR Boundary

- Forecast required a size exception or chained PRs. `tasks.md` sets `Chain strategy: size-exception`; `apply-progress.md` records `Size exception: APPROVED (single PR ~650-750 lines)`.
- The implementation matches the assigned single-PR configuration, integration, cleanup, and documentation slice.
- **WARNING:** `git status --short` also contains an untracked `cli/moonarch` path not named by the task or apply-progress artifacts. `git diff -- cli/` is empty, so acceptance criterion 20 passes, but ownership of that untracked path is not established by this change.

## Blockers

1. **CRITICAL — full validation failure:** `bash test.sh` fails because the MoonArch palette test still enforces deleted `wofi.css` files rather than the new `rofi.rasi` contract.
2. **CRITICAL — strict TDD evidence absent:** `apply-progress.md` lacks the required `TDD Cycle Evidence` table, no task-to-test mapping exists, and the available full test runner is red.
3. **CRITICAL — native status not ready for archive:** authoritative OpenSpec status reports `specs: missing`, `verify: blocked`, and no review receipt.

Archive is **not ready**.
