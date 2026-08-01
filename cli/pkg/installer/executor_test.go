package installer

import (
	"context"
	"errors"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

type fakeManaged struct {
	report *report.ExecutionReport
	err    error
	calls  int
}

func (f *fakeManaged) Execute() (*report.ExecutionReport, error) { f.calls++; return f.report, f.err }

type fakeRunner struct{ calls []string }

func (f *fakeRunner) Run(_ context.Context, a plan.ExternalAction) error {
	f.calls = append(f.calls, a.Description)
	return nil
}

func TestExecutorManagedFailureSkipsExternalActions(t *testing.T) {
	managedErr := errors.New("managed failed")
	managed := &fakeManaged{report: &report.ExecutionReport{}, err: managedErr}
	runner := &fakeRunner{}
	executor := NewExecutor(managed, runner)

	p, err := plan.NewInstallationPlan("run", nil)
	if err != nil {
		t.Fatal(err)
	}
	rpt, got := executor.Execute(context.Background(), p)
	if !errors.Is(got, managedErr) {
		t.Fatalf("error = %v, want %v", got, managedErr)
	}
	if managed.calls != 1 {
		t.Fatalf("managed calls = %d", managed.calls)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("external calls = %v", runner.calls)
	}
	if rpt != managed.report {
		t.Fatal("managed report was not preserved")
	}
}

type failingRunner struct {
	calls []string
	fail  string
}

func (f *failingRunner) Run(_ context.Context, a plan.ExternalAction) error {
	f.calls = append(f.calls, a.Description)
	if a.Description == f.fail {
		return errors.New("external failed")
	}
	return nil
}

func TestExecutorStopsAfterExternalFailure(t *testing.T) {
	managed := &fakeManaged{report: &report.ExecutionReport{}}
	runner := &failingRunner{fail: "second"}
	executor := NewExecutor(managed, runner)
	actions := []plan.ExternalAction{
		{Description: "first", Command: plan.CommandSpec{Name: "first"}, Order: 1},
		{Description: "second", Command: plan.CommandSpec{Name: "second"}, Order: 2},
		{Description: "third", Command: plan.CommandSpec{Name: "third"}, Order: 3},
	}
	p, err := plan.NewInstallationPlanWithActions("run", nil, actions)
	if err != nil {
		t.Fatal(err)
	}
	rpt, got := executor.Execute(context.Background(), p)
	if got == nil || got.Error() != "external failed" {
		t.Fatalf("error = %v", got)
	}
	if len(runner.calls) != 2 || runner.calls[1] != "second" {
		t.Fatalf("calls = %v", runner.calls)
	}
	if len(rpt.ExternalActions) != 3 || rpt.ExternalActions[0].Status != report.ActionCompleted || rpt.ExternalActions[1].Status != report.ActionFailed || rpt.ExternalActions[2].Status != report.ActionSkipped {
		t.Fatalf("outcomes = %#v", rpt.ExternalActions)
	}
	if rpt.PrimaryCause == nil {
		t.Fatal("missing primary cause")
	}
}

// Task 1.5: ExternalOnlyExecutor.

func TestExternalOnlyExecutor_RejectsManagedTargets(t *testing.T) {
	managed := &fakeManaged{report: &report.ExecutionReport{}}
	runner := &fakeRunner{}
	executor := NewExternalOnlyExecutor(runner)

	p, err := plan.NewInstallationPlanWithActions("run", []plan.Target{
		{Source: "src", Destination: "/dst", Kind: plan.CopyFile},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, got := executor.Execute(context.Background(), p)
	if got == nil || got.Error() != "external-only executor cannot execute managed targets" {
		t.Fatalf("error = %v", got)
	}
	if len(runner.calls) != 0 {
		t.Errorf("runner calls = %v", runner.calls)
	}
	if managed.calls != 0 {
		t.Errorf("managed executor calls = %d", managed.calls)
	}
}

func TestExternalOnlyExecutor_RunsActionsInReviewedOrder(t *testing.T) {
	runner := &fakeRunner{}
	executor := NewExternalOnlyExecutor(runner)
	actions := []plan.ExternalAction{
		{Description: "first", Command: plan.CommandSpec{Name: "first"}, Order: 1},
		{Description: "second", Command: plan.CommandSpec{Name: "second"}, Order: 2},
		{Description: "third", Command: plan.CommandSpec{Name: "third"}, Order: 3},
	}
	p, err := plan.NewInstallationPlanWithActions("run", nil, actions)
	if err != nil {
		t.Fatal(err)
	}
	rpt, got := executor.Execute(context.Background(), p)
	if got != nil {
		t.Fatalf("error = %v", got)
	}
	want := []string{"first", "second", "third"}
	if len(runner.calls) != len(want) || !sameStringSlice(runner.calls, want) {
		t.Errorf("calls = %v, want %v", runner.calls, want)
	}
	for i, a := range rpt.ExternalActions {
		if a.Status != report.ActionCompleted {
			t.Errorf("action[%d].Status = %q, want completed", i, a.Status)
		}
	}
	if rpt.Fingerprint != p.Fingerprint {
		t.Errorf("Fingerprint = %q, want %q", rpt.Fingerprint, p.Fingerprint)
	}
}

func TestExternalOnlyExecutor_StopsOnFirstFailure(t *testing.T) {
	runner := &failingRunner{fail: "second"}
	executor := NewExternalOnlyExecutor(runner)
	actions := []plan.ExternalAction{
		{Description: "first", Command: plan.CommandSpec{Name: "first"}, Order: 1},
		{Description: "second", Command: plan.CommandSpec{Name: "second"}, Order: 2},
		{Description: "third", Command: plan.CommandSpec{Name: "third"}, Order: 3},
	}
	p, err := plan.NewInstallationPlanWithActions("run", nil, actions)
	if err != nil {
		t.Fatal(err)
	}
	rpt, got := executor.Execute(context.Background(), p)
	if got == nil || got.Error() != "external failed" {
		t.Fatalf("error = %v", got)
	}
	if len(runner.calls) != 2 || runner.calls[1] != "second" {
		t.Fatalf("calls = %v", runner.calls)
	}
	if len(rpt.ExternalActions) != 3 {
		t.Fatalf("len(actions) = %d", len(rpt.ExternalActions))
	}
	if rpt.ExternalActions[0].Status != report.ActionCompleted || rpt.ExternalActions[1].Status != report.ActionFailed || rpt.ExternalActions[2].Status != report.ActionSkipped {
		t.Fatalf("outcomes = %#v", rpt.ExternalActions)
	}
	if rpt.PrimaryCause == nil {
		t.Fatal("missing primary cause")
	}
}

func TestExternalOnlyExecutor_NeverCallsManagedExecutor(t *testing.T) {
	managed := &fakeManaged{report: &report.ExecutionReport{}}
	runner := &fakeRunner{}
	executor := NewExternalOnlyExecutor(runner)
	p, err := plan.NewInstallationPlanWithActions("run", nil, []plan.ExternalAction{
		{Description: "only", Command: plan.CommandSpec{Name: "only"}, Order: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Execute(context.Background(), p); err != nil {
		t.Fatalf("error = %v", err)
	}
	if managed.calls != 0 {
		t.Errorf("managed executor was called %d times", managed.calls)
	}
}
