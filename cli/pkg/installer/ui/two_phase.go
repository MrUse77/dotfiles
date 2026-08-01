package ui

import (
	"context"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

// TwoPhaseReviewDetails carries the repository information disclosed during the
// single package-plan review. The URL is intentionally not rendered because
// it may contain credentials.
type TwoPhaseReviewDetails struct {
	RepositoryDestination string
	RepositoryRef         string
}

// PackageReviewModel is a Bubble Tea model that presents the package-phase plan
// for the single authorization required by the two-phase route. It has no
// executor; the caller runs the phase after the model returns accepted.
type PackageReviewModel struct {
	Plan           plan.InstallationPlan
	Details        TwoPhaseReviewDetails
	State          State
	ReviewRendered bool
}

// NewPackageReviewModel creates a package review model for the two-phase route.
func NewPackageReviewModel(p plan.InstallationPlan, details TwoPhaseReviewDetails) *PackageReviewModel {
	return NewPackageReviewModelWithContext(context.Background(), p, details)
}

// NewPackageReviewModelWithContext creates a package review model with a context.
func NewPackageReviewModelWithContext(_ context.Context, p plan.InstallationPlan, details TwoPhaseReviewDetails) *PackageReviewModel {
	return &PackageReviewModel{Plan: p, Details: details, State: StateReview}
}

func (m PackageReviewModel) Init() tea.Cmd {
	if m.State == StateReview {
		return acknowledgeReviewCmd()
	}
	return nil
}

func (m *PackageReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case PlanReadyMsg:
		m.Plan = msg.Plan
		m.State = StateReview
		m.ReviewRendered = false
		return m, acknowledgeReviewCmd()
	case ReviewRenderedMsg:
		if m.State == StateReview {
			m.ReviewRendered = true
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
				if !m.ReviewRendered {
					return m, nil
				}
				m.State = StateExecuting
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m *PackageReviewModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Package phase review %s\n\n", m.State)
	fmt.Fprintf(&b, "Plan fingerprint: %s\n", m.Plan.Fingerprint)
	fmt.Fprintf(&b, "Repository destination: %s\n", m.Details.RepositoryDestination)
	fmt.Fprintf(&b, "Repository ref: %s\n", m.Details.RepositoryRef)
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "The exact managed targets will be discovered after the repository is acquired.")
	fmt.Fprintln(&b, "Accepting authorizes the package phase, repository acquisition, and configuration phase.")
	fmt.Fprintln(&b, "Automatic rollback applies only to managed targets, not package effects or the clone.")
	fmt.Fprintln(&b)
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
		b.WriteString("\nAccepted. Continuing with package execution.\n")
	}
	return b.String()
}

// Accepted reports whether the user accepted the package phase.
func (m PackageReviewModel) Accepted() bool { return m.State == StateExecuting }

// Aborted reports whether the user declined or cancelled the review.
func (m PackageReviewModel) Aborted() bool { return m.State == StateAborted }

// ReviewPackagePlanWithContext presents the package plan and repository details
// for the single consent required by the two-phase route. It returns accepted
// true when the user confirms, and it never invokes an executor.
func ReviewPackagePlanWithContext(
	ctx context.Context,
	p plan.InstallationPlan,
	details TwoPhaseReviewDetails,
	input io.Reader,
	output io.Writer,
	run ProgramRunner,
) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	model := NewPackageReviewModelWithContext(ctx, p, details)
	if run == nil {
		run = func(model tea.Model, input io.Reader, output io.Writer) (tea.Model, error) {
			return tea.NewProgram(model, tea.WithInput(input), tea.WithOutput(output)).Run()
		}
	}
	finished, err := run(model, input, output)
	if err != nil {
		return false, err
	}
	result, ok := finished.(*PackageReviewModel)
	if !ok || result == nil {
		return false, fmt.Errorf("%w: %T", ErrUnexpectedModel, finished)
	}
	if result.Aborted() {
		return false, nil
	}
	return result.Accepted(), nil
}

// DisplayConfigurationPlan renders the concrete configuration plan after the
// repository has been acquired. It is output-only: it takes no input, issues
// no confirmation prompt, and performs no execution.
func DisplayConfigurationPlan(output io.Writer, p plan.InstallationPlan) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Configuration phase\n\n")
	fmt.Fprintf(&b, "Plan fingerprint: %s\n", p.Fingerprint)
	fmt.Fprintf(&b, "Managed targets: %d\n", len(p.ManagedTargets()))
	for _, target := range p.ManagedTargets() {
		fmt.Fprintf(&b, "  - %s [%s]\n", target.Destination, target.Kind)
	}
	fmt.Fprintf(&b, "External actions: %d\n", len(p.ExternalActions()))
	for _, action := range p.ExternalActions() {
		warning := ""
		if action.Irreversible {
			warning = " (irreversible)"
		}
		fmt.Fprintf(&b, "  - %s [%s]%s\n", action.Description, action.Classification, warning)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "Authorization was already granted; configuration execution will start now.")
	_, err := output.Write([]byte(b.String()))
	return err
}
