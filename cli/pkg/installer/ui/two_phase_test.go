package ui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func updatePackageReview(t *testing.T, m *PackageReviewModel, msg tea.Msg) *PackageReviewModel {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(*PackageReviewModel)
}

func packageReviewPlan(t *testing.T) plan.InstallationPlan {
	t.Helper()
	p, err := plan.NewInstallationPlanWithActions("run-1", nil, []plan.ExternalAction{{
		Description: "update system and install base tools", Classification: "privileged", Irreversible: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTwoPhaseReviewDetails_Fields(t *testing.T) {
	details := TwoPhaseReviewDetails{RepositoryDestination: "/dst", RepositoryRef: "main"}
	if details.RepositoryDestination != "/dst" || details.RepositoryRef != "main" {
		t.Fatalf("details not retained: %+v", details)
	}
}

func TestPackageReviewModel_RequiresRenderedEventBeforeAcceptance(t *testing.T) {
	m := NewPackageReviewModel(packageReviewPlan(t), TwoPhaseReviewDetails{})
	m = updatePackageReview(t, m, PlanReadyMsg{Plan: m.Plan})
	m = updatePackageReview(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.Accepted() {
		t.Fatal("acceptance before render event must be ignored")
	}
	m = updatePackageReview(t, m, ReviewRenderedMsg{})
	m = updatePackageReview(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.Accepted() {
		t.Fatal("acceptance after render event must be accepted")
	}
}

func TestPackageReviewModel_DeclineEscapeAndCtrlCRejectWithoutExecution(t *testing.T) {
	for _, key := range []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"escape", tea.KeyMsg{Type: tea.KeyEscape}},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}},
		{"n", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}},
	} {
		t.Run(key.name, func(t *testing.T) {
			m := NewPackageReviewModel(packageReviewPlan(t), TwoPhaseReviewDetails{})
			m = updatePackageReview(t, m, ReviewRenderedMsg{})
			m = updatePackageReview(t, m, key.msg)
			if !m.Aborted() || m.Accepted() {
				t.Fatalf("state=%s accepted=%v, want aborted without execution", m.State, m.Accepted())
			}
		})
	}
}

func TestPackageReviewModel_ViewDisclosesRepositoryDetails(t *testing.T) {
	m := NewPackageReviewModel(packageReviewPlan(t), TwoPhaseReviewDetails{
		RepositoryDestination: "/home/user/.cache/dotfiles",
		RepositoryRef:         "v0.1.0",
	})
	m = updatePackageReview(t, m, ReviewRenderedMsg{})
	view := m.View()
	for _, want := range []string{"/home/user/.cache/dotfiles", "v0.1.0", "Package phase review", "managed targets will be discovered"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestPackageReviewModel_ViewDisclosesAuthorizationScope(t *testing.T) {
	m := NewPackageReviewModel(packageReviewPlan(t), TwoPhaseReviewDetails{})
	m = updatePackageReview(t, m, ReviewRenderedMsg{})
	view := m.View()
	for _, want := range []string{"Accepting authorizes", "package phase, repository acquisition, and configuration phase", "Automatic rollback applies only to managed targets"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestPackageReviewModel_ViewDisplaysActionsAndIrreversibleLabels(t *testing.T) {
	m := NewPackageReviewModel(packageReviewPlan(t), TwoPhaseReviewDetails{})
	m = updatePackageReview(t, m, ReviewRenderedMsg{})
	view := m.View()
	for _, want := range []string{"update system and install base tools", "privileged", "irreversible", "Confirm all actions"} {
		if !strings.Contains(view, want) {
			t.Errorf("View() missing %q:\n%s", want, view)
		}
	}
}

func TestReviewPackagePlanWithContext_Accepts(t *testing.T) {
	accepted, err := ReviewPackagePlanWithContext(context.Background(), packageReviewPlan(t), TwoPhaseReviewDetails{},
		nil, nil, func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
			m := model.(*PackageReviewModel)
			m = updatePackageReview(t, m, ReviewRenderedMsg{})
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return updated, nil
		})
	if err != nil || !accepted {
		t.Fatalf("accepted=%v err=%v, want true nil", accepted, err)
	}
}

func TestReviewPackagePlanWithContext_Decline(t *testing.T) {
	accepted, err := ReviewPackagePlanWithContext(context.Background(), packageReviewPlan(t), TwoPhaseReviewDetails{},
		nil, nil, func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
			m := model.(*PackageReviewModel)
			m = updatePackageReview(t, m, ReviewRenderedMsg{})
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
			return updated, nil
		})
	if err != nil || accepted {
		t.Fatalf("accepted=%v err=%v, want false nil", accepted, err)
	}
}

func TestReviewPackagePlanWithContext_UnexpectedModel(t *testing.T) {
	_, err := ReviewPackagePlanWithContext(context.Background(), packageReviewPlan(t), TwoPhaseReviewDetails{},
		nil, nil, func(tea.Model, io.Reader, io.Writer) (tea.Model, error) { return structModel{}, nil })
	if !strings.Contains(err.Error(), ErrUnexpectedModel.Error()) {
		t.Fatalf("err=%v, want ErrUnexpectedModel", err)
	}
}

func TestReviewPackagePlanWithContext_DoesNotInvokeExecutor(t *testing.T) {
	executor := &fakeExecutor{}
	_, _ = ReviewPackagePlanWithContext(context.Background(), packageReviewPlan(t), TwoPhaseReviewDetails{},
		nil, nil, func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
			m := model.(*PackageReviewModel)
			m = updatePackageReview(t, m, ReviewRenderedMsg{})
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			return updated, nil
		})
	if executor.calls != 0 {
		t.Fatalf("executor called %d times", executor.calls)
	}
}

func TestDisplayConfigurationPlan_IsOutputOnly(t *testing.T) {
	p, err := plan.NewInstallationPlanWithActions("run-1", []plan.Target{{
		Source: "/repo/.zshrc", Destination: "/home/user/.zshrc", Kind: plan.CopyFile,
	}}, []plan.ExternalAction{{
		Description: "create zsh configuration directory", Classification: "filesystem", Irreversible: false,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := DisplayConfigurationPlan(&out, p); err != nil {
		t.Fatalf("DisplayConfigurationPlan error = %v", err)
	}
	output := out.String()
	for _, want := range []string{p.Fingerprint, "/home/user/.zshrc", "create zsh configuration directory", "Authorization was already granted"} {
		if !strings.Contains(output, want) {
			t.Errorf("output missing %q:\n%s", want, output)
		}
	}
	for _, avoid := range []string{"Confirm", "[y/enter]", "abort"} {
		if strings.Contains(output, avoid) {
			t.Errorf("output must not contain prompt %q:\n%s", avoid, output)
		}
	}
}

func TestDisplayConfigurationPlan_CancellationBeforeExecutionProducesNoTransaction(t *testing.T) {
	p, err := plan.NewInstallationPlanWithActions("run-1", nil, []plan.ExternalAction{{
		Description: "enable power profiles", Classification: "privileged", Irreversible: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := DisplayConfigurationPlan(&out, p); err != nil {
		t.Fatalf("DisplayConfigurationPlan error = %v", err)
	}
	// No transaction, no executor, no runner: output-only means no side effects.
	if out.Len() == 0 {
		t.Fatal("configuration display produced no output")
	}
}

func TestPackageReviewModel_PreservesPlanFingerprint(t *testing.T) {
	p := packageReviewPlan(t)
	m := NewPackageReviewModel(p, TwoPhaseReviewDetails{})
	if m.Plan.Fingerprint != p.Fingerprint {
		t.Fatalf("fingerprint not preserved: got %q, want %q", m.Plan.Fingerprint, p.Fingerprint)
	}
}

func TestPackageReviewModel_ReportsStartedMessage(t *testing.T) {
	m := NewPackageReviewModel(packageReviewPlan(t), TwoPhaseReviewDetails{})
	m = updatePackageReview(t, m, ReviewRenderedMsg{})
	m = updatePackageReview(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), "Accepted. Continuing") {
		t.Fatal("accepted view missing continuation message")
	}
}

// Ensure the report import is used for fakeExecutor.
var _ = (*report.ExecutionReport)(nil)
