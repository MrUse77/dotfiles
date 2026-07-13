# TUI Plan Specification

## Purpose

Define the terminal UI flow that builds, displays, and confirms the complete installation plan before any system mutation occurs. This domain covers the Bubbletea/Huh-driven state machine from initial plan construction through final confirmation or abandonment.

## Requirements

### Requirement: Plan Construction Completes Before Display

The installer MUST build the complete installation plan — including all managed targets and all external actions — before transitioning the TUI to the plan-display state. No partial plan SHALL be presented to the user.

#### Scenario: Successful plan construction

- GIVEN the user has invoked the install command
- WHEN the installer completes plan construction without error
- THEN the TUI SHALL transition to the plan-review state with the full plan available for rendering

#### Scenario: Plan construction failure

- GIVEN the user has invoked the install command
- WHEN plan construction encounters an error (e.g. target discovery failure, permission check failure)
- THEN the TUI SHALL transition to an error state displaying the failure reason
- AND the TUI SHALL NOT transition to the plan-review state
- AND no filesystem mutation SHALL have occurred

### Requirement: Full Plan Is Rendered Before Confirmation Prompt

The TUI MUST render the entire plan — every managed target mutation and every external action — in the review view before presenting the confirmation prompt. The confirmation control SHALL NOT be reachable until the full plan is displayed.

#### Scenario: User reviews complete plan then confirms

- GIVEN the TUI is in the plan-review state with a complete plan
- WHEN the plan view renders all managed targets and all external actions
- THEN the confirmation prompt SHALL become available

#### Scenario: Plan includes both managed targets and external actions

- GIVEN the plan contains 5 managed file targets and 2 external actions
- WHEN the TUI renders the plan-review view
- THEN the view SHALL display all 5 managed targets with their mutation type
- AND the view SHALL display all 2 external actions with their classification label

### Requirement: Single Confirmation Authorizes Entire Plan

One affirmative confirmation SHALL authorize execution of the entire displayed plan. The installer SHALL NOT execute any part of the plan before this confirmation. The confirmation control MUST require an explicit positive action (e.g. selecting "Confirm" and pressing enter).

#### Scenario: User confirms execution

- GIVEN the TUI is displaying the plan-review view with confirmation available
- WHEN the user selects the confirm option and activates it
- THEN the TUI SHALL transition to the execution state
- AND the installer SHALL begin executing the bound plan

#### Scenario: User declines execution

- GIVEN the TUI is displaying the plan-review view with confirmation available
- WHEN the user selects the decline/abort option
- THEN the TUI SHALL transition to the aborted state
- AND no filesystem mutation SHALL have occurred
- AND the installer process SHALL exit with a zero status code

#### Scenario: User interrupts before confirmation

- GIVEN the TUI is in the plan-review state
- WHEN the user sends an interrupt signal (Ctrl-C, terminal close)
- THEN no filesystem mutation SHALL have occurred
- AND the installer process SHALL exit

### Requirement: No Per-Action Toggles During Review

The plan-review TUI MUST NOT provide controls to enable, disable, or selectively skip individual actions. The plan is presented as a single atomic unit; the user's only choices are to confirm the entire plan or decline it entirely.

#### Scenario: Review view has no toggle controls

- GIVEN the TUI is in the plan-review state
- WHEN the user inspects the available controls
- THEN the TUI SHALL offer only confirm-all and decline-all options
- AND the TUI SHALL NOT render checkboxes, toggles, or per-item selection controls

### Requirement: Plan Is Bound To Confirmation

The plan displayed during review SHALL be the exact plan bound to execution upon confirmation. The installer MUST NOT re-plan, re-discover targets, or modify the plan between the confirmation event and the start of execution.

#### Scenario: Confirmed plan matches reviewed plan

- GIVEN the TUI displayed a plan with N managed targets and M external actions
- WHEN the user confirms and execution begins
- THEN the execution engine SHALL receive the exact same plan that was displayed
- AND no additional targets or actions SHALL be added without user review

### Requirement: Confirmation Abandonment Leaves System Unchanged

If the user abandons the confirmation flow at any point — by declining, interrupting, or closing the terminal — the system MUST remain in its pre-installation state with no mutations applied.

#### Scenario: User presses escape during confirmation

- GIVEN the TUI is in the plan-review state
- WHEN the user presses escape or another abort key
- THEN no filesystem mutation SHALL have occurred
- AND the installer SHALL exit cleanly

#### Scenario: Terminal disconnects during review

- GIVEN the TUI is in the plan-review state
- WHEN the terminal session is lost (SSH disconnect, terminal emulator crash)
- THEN no filesystem mutation SHALL have occurred

## TUI State Machine

The TUI operates through these states:

```
initializing → plan-review → (confirming) → executing → done
                    ↓              ↓
                error ←──── aborted
```

| State | Allowed transitions | User-visible behavior |
|---|---|---|
| `initializing` | → `plan-review`, → `error` | Spinner or progress indicator |
| `plan-review` | → `executing` (confirm), → `aborted` (decline), → `error` | Full plan rendered, confirm/decline controls |
| `executing` | → `done`, → `error` (handled failure triggers rollback) | Progress display |
| `done` | (terminal) | Success message |
| `aborted` | (terminal) | Clean-exit message |
| `error` | (terminal) | Error details, may include rollback status |
