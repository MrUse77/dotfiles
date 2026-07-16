package ui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

// State describes the review lifecycle.
type State string

const (
	StateInitializing State = "initializing"
	StateReview       State = "review"
	StateExecuting    State = "executing"
	StateDone         State = "done"
	StateAborted      State = "aborted"
	StateError        State = "error"
)

// Executor is the command boundary used after explicit confirmation.
type Executor interface {
	Execute(context.Context, plan.InstallationPlan) (*report.ExecutionReport, error)
}

type PlanReadyMsg struct{ Plan plan.InstallationPlan }
type PlanFailedMsg struct{ Err error }
type ReviewRenderedMsg struct{}
type ExecutionFinishedMsg struct {
	Report *report.ExecutionReport
	Err    error
}
type ProgressMsg struct{ Text string }

// Model is the review/progress/result Bubble Tea model. Confirmation is always
// all-or-nothing; there are intentionally no per-item controls.
type Model struct {
	Plan           plan.InstallationPlan
	Executor       Executor
	State          State
	ReviewRendered bool
	Result         *report.ExecutionReport
	Err            error
	Progress       []string
}

func NewReviewModel(p plan.InstallationPlan, executor Executor) *Model {
	return NewReviewModelWithContext(context.Background(), p, executor)
}

func NewReviewModelWithContext(_ context.Context, p plan.InstallationPlan, executor Executor) *Model {
	return &Model{Plan: p, Executor: executor, State: StateReview}
}

func (m Model) Init() tea.Cmd {
	if m.State == StateReview {
		return acknowledgeReviewCmd()
	}
	return nil
}

func acknowledgeReviewCmd() tea.Cmd {
	return func() tea.Msg { return ReviewRenderedMsg{} }
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PlanReadyMsg:
		m.Plan = msg.Plan
		m.State = StateReview
		m.ReviewRendered = false
		return m, acknowledgeReviewCmd()
	case PlanFailedMsg:
		m.State, m.Err = StateError, msg.Err
		return m, nil
	case ReviewRenderedMsg:
		if m.State == StateReview {
			m.ReviewRendered = true
		}
		return m, nil
	case ProgressMsg:
		m.Progress = append(m.Progress, msg.Text)
		return m, nil
	case ExecutionFinishedMsg:
		m.Result, m.Err = msg.Report, msg.Err
		if msg.Err != nil {
			m.State = StateError
		} else {
			m.State = StateDone
		}
		return m, nil
	case tea.KeyMsg:
		switch m.State {
		case StateReview:
			switch msg.String() {
			case "ctrl+c", "esc", "n", "N":
				m.State = StateAborted
				return m, tea.Quit
			case "enter", "y", "Y":
				if !m.ReviewRendered || m.Executor == nil {
					return m, nil
				}
				m.State = StateExecuting
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *Model) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Installation plan %s\n\n", m.State)
	fmt.Fprintf(&b, "Plan fingerprint: %s\n", m.Plan.Fingerprint)
	fmt.Fprintf(&b, "Managed targets: %d\n", len(m.Plan.ManagedTargets()))
	for _, target := range m.Plan.ManagedTargets() {
		fmt.Fprintf(&b, "  - %s [%s]\n", target.Destination, target.Kind)
	}
	fmt.Fprintf(&b, "External actions: %d\n", len(m.Plan.ExternalActions()))
	for _, action := range m.Plan.ExternalActions() {
		warning := ""
		if action.Irreversible {
			warning = " (irreversible)"
		}
		fmt.Fprintf(&b, "  - %s [%s]%s\n", action.Description, action.Classification, warning)
	}
	if m.State == StateReview && m.ReviewRendered {
		b.WriteString("\nConfirm all actions? [y/enter] yes, [n/esc] abort\n")
	}
	if m.State == StateExecuting {
		b.WriteString("\nExecution started.\n")
	}
	for _, progress := range m.Progress {
		fmt.Fprintf(&b, "\n%s", progress)
	}
	if m.Err != nil {
		fmt.Fprintf(&b, "\nError: %v\n", m.Err)
	}
	return b.String()
}

func (m Model) Aborted() bool { return m.State == StateAborted }
func (m Model) Error() error  { return m.Err }
