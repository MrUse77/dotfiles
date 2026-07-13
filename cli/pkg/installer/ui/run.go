package ui

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// ProgramRunner is injectable so command composition tests never require a terminal.
type ProgramRunner func(tea.Model, io.Reader, io.Writer) (tea.Model, error)

var ErrUnexpectedModel = errors.New("installer UI returned an unexpected model")

// Run presents a reviewed plan and executes it only after explicit confirmation.
// The returned aborted flag distinguishes a deliberate decline from a failure.
func Run(ctx context.Context, p plan.InstallationPlan, executor Executor, input io.Reader, output io.Writer, run ProgramRunner) (*report.ExecutionReport, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	model := NewReviewModelWithContext(ctx, p, executor)
	if run == nil {
		run = func(model tea.Model, input io.Reader, output io.Writer) (tea.Model, error) {
			return tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output)).Run()
		}
	}
	finished, err := run(model, input, output)
	if err != nil {
		return nil, false, err
	}
	result, ok := finished.(*Model)
	if !ok || result == nil {
		return nil, false, fmt.Errorf("%w: %T", ErrUnexpectedModel, finished)
	}
	if result.Aborted() {
		return nil, true, nil
	}
	if result.Error() != nil {
		return result.Result, false, result.Error()
	}
	return result.Result, false, nil
}

// RunWithContext preserves cancellation for callers while keeping Bubble Tea's
// command boundary deterministic; execution commands use the supplied context.
func RunWithContext(ctx context.Context, p plan.InstallationPlan, executor Executor, input io.Reader, output io.Writer, run ProgramRunner) (*report.ExecutionReport, bool, error) {
	return Run(ctx, p, executor, input, output, run)
}
