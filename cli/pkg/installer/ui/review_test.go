package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

type fakeExecutor struct {
	calls      int
	got        plan.InstallationPlan
	gotContext context.Context
	err        error
	result     *report.ExecutionReport
	started    chan struct{}
	cancelled  chan struct{}
	beforeCall func()
}

func (f *fakeExecutor) Execute(ctx context.Context, p plan.InstallationPlan) (*report.ExecutionReport, error) {
	if f.beforeCall != nil {
		f.beforeCall()
	}
	f.calls++
	f.got = p
	f.gotContext = ctx
	if f.started != nil {
		close(f.started)
	}
	if f.cancelled != nil {
		<-ctx.Done()
		close(f.cancelled)
		return f.result, f.err
	}
	if f.result != nil {
		return f.result, f.err
	}
	return &report.ExecutionReport{Fingerprint: p.Fingerprint}, f.err
}

func testReviewPlan(t *testing.T) plan.InstallationPlan {
	t.Helper()
	p, err := plan.NewInstallationPlanWithActions("run-1", []plan.Target{{
		Source: "/repo/.zshrc", Destination: "/home/user/.zshrc", Kind: plan.CopyFile,
	}}, []plan.ExternalAction{{
		Description: "install packages", Classification: "privileged", Irreversible: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func updateReview(t *testing.T, m *Model, msg tea.Msg) *Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(*Model)
}

func TestReviewModel_RequiresExplicitRenderedEventBeforeConfirmation(t *testing.T) {
	p := testReviewPlan(t)
	executor := &fakeExecutor{}
	m := NewReviewModel(p, executor)
	m = updateReview(t, m, PlanReadyMsg{Plan: p})
	if m.ReviewRendered {
		t.Fatal("View and plan delivery must not acknowledge rendering")
	}
	_ = m.View()
	m = updateReview(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.State != StateReview || executor.calls != 0 {
		t.Fatalf("confirmation before render must be ignored: state=%s calls=%d", m.State, executor.calls)
	}
	m = updateReview(t, m, ReviewRenderedMsg{})
	m = updateReview(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.State != StateExecuting || executor.calls != 0 {
		t.Fatalf("confirmation should terminate the TUI without execution: state=%s calls=%d", m.State, executor.calls)
	}
}

func TestReviewModel_PreBuiltPlanStartsReviewAndAcknowledgesThroughInitCommand(t *testing.T) {
	m := NewReviewModel(testReviewPlan(t), &fakeExecutor{})
	if m.State != StateReview {
		t.Fatalf("prebuilt plan state=%s, want review", m.State)
	}
	cmd := m.Init()
	if cmd == nil || cmd() != (ReviewRenderedMsg{}) {
		t.Fatal("Init must provide the render acknowledgement event")
	}
}

func TestRunPassesCallerContextToExecutor(t *testing.T) {
	ctx := context.WithValue(context.Background(), contextKey("caller"), "present")
	executor := &fakeExecutor{}
	run := func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
		m := model.(*Model)
		m = updateReview(t, m, ReviewRenderedMsg{})
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return updated, nil
	}

	_, _, err := Run(ctx, testReviewPlan(t), executor, nil, nil, run)
	if err != nil || executor.gotContext.Value(contextKey("caller")) != "present" {
		t.Fatalf("executor context = %v, err = %v", executor.gotContext, err)
	}
}

func TestRunReportsExecutionStartedBeforeInvokingExecutor(t *testing.T) {
	var output bytes.Buffer
	executor := &fakeExecutor{beforeCall: func() {
		if !strings.Contains(output.String(), "Execution started") {
			t.Fatal("executor started before execution feedback was emitted")
		}
	}}
	run := func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
		m := model.(*Model)
		m = updateReview(t, m, ReviewRenderedMsg{})
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return updated, nil
	}

	_, aborted, err := Run(context.Background(), testReviewPlan(t), executor, nil, &output, run)
	if aborted || err != nil {
		t.Fatalf("Run() = aborted %v, err %v", aborted, err)
	}
	if !strings.Contains(output.String(), "Execution started") {
		t.Fatalf("execution feedback = %q, want execution-start message", output.String())
	}
}

func TestRunTerminatesTUIBeforeInvokingExecutor(t *testing.T) {
	wantReport := &report.ExecutionReport{Fingerprint: "completed-run"}
	executor := &fakeExecutor{result: wantReport}
	tuiTerminated := false
	run := func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
		m := model.(*Model)
		m = updateReview(t, m, ReviewRenderedMsg{})
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(*Model)
		if executor.calls != 0 {
			t.Fatal("executor ran before the TUI terminated")
		}
		tuiTerminated = true
		return m, nil
	}

	gotReport, aborted, err := Run(context.Background(), testReviewPlan(t), executor, nil, nil, run)
	if !tuiTerminated || executor.calls != 1 || gotReport != wantReport || aborted || err != nil {
		t.Fatalf("Run() = terminated %v, calls %d, report %v, aborted %v, err %v", tuiTerminated, executor.calls, gotReport, aborted, err)
	}
}

func TestRunAbortDoesNotInvokeExecutor(t *testing.T) {
	executor := &fakeExecutor{}
	run := func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
		m := model.(*Model)
		m = updateReview(t, m, ReviewRenderedMsg{})
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
		return updated, nil
	}

	gotReport, aborted, err := Run(context.Background(), testReviewPlan(t), executor, nil, nil, run)
	if executor.calls != 0 || gotReport != nil || !aborted || err != nil {
		t.Fatalf("Run() = calls %d, report %v, aborted %v, err %v", executor.calls, gotReport, aborted, err)
	}
}

func TestRunPreservesExecutorReportAndErrorAfterTUITerminates(t *testing.T) {
	wantReport := &report.ExecutionReport{Fingerprint: "partial-run"}
	wantErr := errors.New("external action failed")
	executor := &fakeExecutor{result: wantReport, err: wantErr}
	run := func(model tea.Model, _ io.Reader, _ io.Writer) (tea.Model, error) {
		m := model.(*Model)
		m = updateReview(t, m, ReviewRenderedMsg{})
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return updated, nil
	}

	gotReport, aborted, err := Run(context.Background(), testReviewPlan(t), executor, nil, nil, run)
	if executor.calls != 1 || gotReport != wantReport || aborted || !errors.Is(err, wantErr) {
		t.Fatalf("Run() = calls %d, report %v, aborted %v, err %v; want calls 1, report %v, aborted false, err %v", executor.calls, gotReport, aborted, err, wantReport, wantErr)
	}
}

func TestRunRejectsUnexpectedProgramModel(t *testing.T) {
	_, _, err := Run(context.Background(), testReviewPlan(t), &fakeExecutor{}, nil, nil,
		func(tea.Model, io.Reader, io.Writer) (tea.Model, error) { return structModel{}, nil })
	if !errors.Is(err, ErrUnexpectedModel) {
		t.Fatalf("err=%v, want ErrUnexpectedModel", err)
	}
}

type contextKey string

type structModel struct{}

func (structModel) Init() tea.Cmd                       { return nil }
func (structModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return structModel{}, nil }
func (structModel) View() string                        { return "" }

func TestReviewModel_DeclineEscapeAndCtrlCAbortWithoutExecution(t *testing.T) {
	for _, key := range []tea.KeyType{tea.KeyEscape, tea.KeyCtrlC} {
		t.Run(key.String(), func(t *testing.T) {
			executor := &fakeExecutor{}
			m := NewReviewModel(testReviewPlan(t), executor)
			m = updateReview(t, m, PlanReadyMsg{Plan: m.Plan})
			m = updateReview(t, m, ReviewRenderedMsg{})
			m = updateReview(t, m, tea.KeyMsg{Type: key})
			if !m.Aborted() || executor.calls != 0 {
				t.Fatalf("state=%s calls=%d, want aborted without execution", m.State, executor.calls)
			}
		})
	}
}

func TestReviewModel_PreservesPlanFingerprintAndRendersClassifications(t *testing.T) {
	p := testReviewPlan(t)
	m := NewReviewModel(p, &fakeExecutor{})
	m = updateReview(t, m, PlanReadyMsg{Plan: p})
	m = updateReview(t, m, ReviewRenderedMsg{})
	view := m.View()
	for _, want := range []string{".zshrc", "install packages", "privileged", "irreversible", p.Fingerprint, "Confirm all actions"} {
		if !contains(view, want) {
			t.Errorf("View() missing %q:\n%s", want, view)
		}
	}
	if contains(view, "toggle") {
		t.Error("review must not expose per-item toggles")
	}
}

func TestReviewModel_PlanFailureIsTerminalError(t *testing.T) {
	m := NewReviewModel(testReviewPlan(t), &fakeExecutor{})
	m = updateReview(t, m, PlanFailedMsg{Err: errors.New("cannot read plan")})
	if m.State != StateError || m.Error() == nil || m.ReviewRendered {
		t.Fatalf("unexpected failed-plan state: %+v", m)
	}
}

func contains(s, want string) bool {
	for i := 0; i+len(want) <= len(s); i++ {
		if s[i:i+len(want)] == want {
			return true
		}
	}
	return false
}
