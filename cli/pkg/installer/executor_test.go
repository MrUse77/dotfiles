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
