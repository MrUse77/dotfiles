package installer

import (
	"context"

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

	actions := p.ExternalActions()
	for i, action := range actions {
		if err := e.runner.Run(ctx, action); err != nil {
			rpt.ExternalActions = append(rpt.ExternalActions,
				report.ActionOutcome{Description: action.Description, Status: report.ActionFailed, Error: err})
			for _, skipped := range actions[i+1:] {
				rpt.ExternalActions = append(rpt.ExternalActions,
					report.ActionOutcome{Description: skipped.Description, Status: report.ActionSkipped})
			}
			rpt.PrimaryCause = err
			return rpt, err
		}
		rpt.ExternalActions = append(rpt.ExternalActions,
			report.ActionOutcome{Description: action.Description, Status: report.ActionCompleted})
	}
	return rpt, nil
}
