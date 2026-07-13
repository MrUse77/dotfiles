# External Actions Specification

## Purpose

Define the handling of privileged and supply-chain actions — operations that affect external systems (package managers, system services, network downloads) and cannot be transactionally reversed by managed-target rollback. These actions MUST be visible in the plan, deferred until after confirmation, and sequenced to minimize unrecoverable partial-installation risk.

## Requirements

### Requirement: External Actions Are Visible In Plan

Every privileged or supply-chain action MUST be listed in the installation plan displayed during TUI review. Each external action SHALL be clearly labeled with its classification (e.g. "privileged", "supply-chain", "external") so the user can distinguish it from managed file mutations.

#### Scenario: Plan displays external actions with labels

- GIVEN the installation plan includes a `pacman -Syu` system update and a `paru` AUR helper installation
- WHEN the TUI renders the plan-review view
- THEN each external action SHALL be displayed with its command or description
- AND each external action SHALL carry a visible classification label indicating it is external/privileged/supply-chain

#### Scenario: Plan displays mix of managed and external actions

- GIVEN the plan contains 10 managed file targets and 3 external actions
- WHEN the TUI renders the plan-review view
- THEN all 10 managed targets and all 3 external actions SHALL appear
- AND the external actions SHALL be visually distinguishable from managed targets

### Requirement: External Actions Deferred Until After Confirmation

External actions MUST NOT execute before the user's final confirmation. No privileged command, package installation, or network download SHALL run during the planning or review phase.

#### Scenario: No external action during planning

- GIVEN the installer is building the plan
- WHEN planning completes and the TUI enters the plan-review state
- THEN no external action SHALL have been executed
- AND no package manager command SHALL have run

#### Scenario: External actions execute only after confirmation

- GIVEN the TUI is in the plan-review state
- WHEN the user has NOT confirmed
- THEN no external action SHALL execute

#### Scenario: External actions execute after confirmation

- GIVEN the user has confirmed the plan
- WHEN execution begins
- THEN external actions SHALL execute as part of the confirmed plan

### Requirement: External Actions Sequenced After Managed Mutations

External actions that affect system state in potentially irreversible ways SHOULD be sequenced after all managed-target file mutations. This ordering ensures that file-level rollback can complete cleanly before external side effects occur, minimizing the chance of an unrecoverable partial installation.

#### Scenario: File mutations precede external actions

- GIVEN the plan contains managed file mutations and external actions
- WHEN execution proceeds
- THEN all managed-target file mutations SHALL execute before any external action begins
- AND if a file mutation fails, external actions SHALL NOT have run

#### Scenario: External action failure does not trigger file rollback

- GIVEN all managed-target file mutations have completed successfully
- WHEN an external action fails during execution
- THEN the installer SHALL NOT roll back the already-completed file mutations
- AND the installer SHALL report the external action failure
- AND the installer SHALL exit with a non-zero status

### Requirement: Irreversible External Effects Are Explicitly Labeled

When an external action's effects cannot be reversed (e.g. package installation, system service enablement), the plan display MUST clearly indicate that this action is not covered by the managed-target rollback guarantee.

#### Scenario: Irreversible action labeled in plan

- GIVEN the plan includes enabling a systemd service via `systemctl enable --now`
- WHEN the TUI renders this action in the plan-review view
- THEN the display SHALL indicate this action is external and not reversible by the installer's rollback mechanism

#### Scenario: Reversible external action still labeled

- GIVEN the plan includes a `fc-cache` font cache rebuild (side-effect-only, low risk)
- WHEN the TUI renders this action
- THEN the display SHALL still indicate it is an external action
- AND the classification MAY indicate lower risk where applicable

### Requirement: External Action Failure Is Reported

If an external action fails during execution, the installer MUST report the failure with the action description and error details. The failure SHALL not be silently swallowed.

#### Scenario: External command returns non-zero exit

- GIVEN execution is running an external action (e.g. `pacman -Syu`)
- WHEN the command exits with a non-zero status
- THEN the installer SHALL record the failure with the command and exit status
- AND the installer SHALL display the failure to the user

#### Scenario: External command not found

- GIVEN execution is running an external action that requires a binary not on PATH
- WHEN the installer attempts to execute it
- THEN the installer SHALL report a clear error indicating the missing dependency
- AND the installer SHALL NOT panic or produce an unhandled error

### Requirement: External Actions Do Not Run Before Final Confirmation

This is a reinforcement of the confirmation boundary: external actions are subject to the same confirmation gate as managed targets. No external action SHALL begin execution in any path that does not pass through the user's affirmative confirmation of the complete plan.

#### Scenario: Confirmation required for external actions

- GIVEN the plan contains external actions
- WHEN the user declines or abandons confirmation
- THEN no external action SHALL have been executed
- AND the system state SHALL be unchanged by external actions
