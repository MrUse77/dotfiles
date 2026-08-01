package installer

import (
	"context"
	"errors"

	"github.com/MrUse77/dots-cli/pkg/installer/external"
	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// ManagedExecutor is the recoverable managed-filesystem execution seam.
type ManagedExecutor interface {
	Execute() (*report.ExecutionReport, error)
}

// Executor commits managed targets before running any reviewed external action.
// External failures are reported without rolling back the managed transaction.
type Executor struct {
	managed ManagedExecutor
	runner  external.CommandRunner
}

// NewExecutor constructs an executor with injectable managed and external seams.
func NewExecutor(managed ManagedExecutor, runner external.CommandRunner) *Executor {
	return &Executor{managed: managed, runner: runner}
}

// Execute runs the managed transaction first, then external actions in the
// reviewed order stored in the plan. It stops at the first external failure.
func (e *Executor) Execute(ctx context.Context, p plan.InstallationPlan) (*report.ExecutionReport, error) {
	rpt, err := e.managed.Execute()
	if rpt == nil {
		rpt = &report.ExecutionReport{Fingerprint: p.Fingerprint}
	}
	if err != nil {
		if rpt.PrimaryCause == nil {
			rpt.PrimaryCause = err
		}
		return rpt, err
	}

	rpt = executeExternalActions(ctx, e.runner, p, rpt)
	if rpt.PrimaryCause != nil {
		return rpt, rpt.PrimaryCause
	}
	return rpt, nil
}

// ExternalOnlyExecutor executes reviewed external actions without a managed
// transaction. It never constructs or calls a managed executor.
type ExternalOnlyExecutor struct {
	runner external.CommandRunner
}

// NewExternalOnlyExecutor constructs an executor that runs only external actions.
func NewExternalOnlyExecutor(runner external.CommandRunner) *ExternalOnlyExecutor {
	return &ExternalOnlyExecutor{runner: runner}
}

// Execute validates the plan has no managed targets and runs its external
// actions in the reviewed order. It stops at the first external failure.
func (e *ExternalOnlyExecutor) Execute(ctx context.Context, p plan.InstallationPlan) (*report.ExecutionReport, error) {
	if len(p.ManagedTargets()) != 0 {
		return nil, errors.New("external-only executor cannot execute managed targets")
	}
	rpt := &report.ExecutionReport{Fingerprint: p.Fingerprint}
	rpt = executeExternalActions(ctx, e.runner, p, rpt)
	if rpt.PrimaryCause != nil {
		return rpt, rpt.PrimaryCause
	}
	return rpt, nil
}

// executeExternalActions runs the plan's external actions and records outcomes.
// It mutates the supplied report and returns it for convenience.
func executeExternalActions(ctx context.Context, runner external.CommandRunner, p plan.InstallationPlan, rpt *report.ExecutionReport) *report.ExecutionReport {
	actions := p.ExternalActions()
	for i, action := range actions {
		if err := runner.Run(ctx, action); err != nil {
			rpt.ExternalActions = append(rpt.ExternalActions,
				report.ActionOutcome{Description: action.Description, Status: report.ActionFailed, Error: err})
			for _, skipped := range actions[i+1:] {
				rpt.ExternalActions = append(rpt.ExternalActions,
					report.ActionOutcome{Description: skipped.Description, Status: report.ActionSkipped})
			}
			rpt.PrimaryCause = err
			return rpt
		}
		rpt.ExternalActions = append(rpt.ExternalActions,
			report.ActionOutcome{Description: action.Description, Status: report.ActionCompleted})
	}
	return rpt
}
