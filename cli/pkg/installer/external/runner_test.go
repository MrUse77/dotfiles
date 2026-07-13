package external

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func TestRunnerExecutesStructuredCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("helper process")
	}
	workDir := t.TempDir()
	runner := NewRunner(nil)
	action := plan.ExternalAction{Description: "write marker", Command: plan.CommandSpec{
		Name: os.Args[0], Args: []string{"-test.run=TestRunnerHelperProcess", "--", "hello world"}, Dir: workDir,
		Env: map[string]string{"RUNNER_MARKER": "present"},
	}}
	if err := runner.Run(context.Background(), action); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got, err := os.ReadFile(filepath.Join(workDir, "runner-output"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello world:present" {
		t.Fatalf("output = %q", got)
	}
}

func TestRunnerReturnsIdentityRichWrappedError(t *testing.T) {
	cause := errors.New("boom")
	runner := NewRunner(func(context.Context, plan.CommandSpec, io.Reader, io.Writer, io.Writer) error { return cause })
	action := plan.ExternalAction{Description: "failing action"}
	err := runner.Run(context.Background(), action)
	var actionErr *report.ExternalActionError
	if !errors.As(err, &actionErr) || actionErr.Cause != cause {
		t.Fatalf("error = %v, want wrapped ExternalActionError and cause", err)
	}
	if actionErr.Action.Description != action.Description {
		t.Fatalf("action = %#v", actionErr.Action)
	}
}

func TestRunnerDoesNotConstructShellCommand(t *testing.T) {
	var got plan.CommandSpec
	runner := NewRunner(func(_ context.Context, spec plan.CommandSpec, _ io.Reader, _ io.Writer, _ io.Writer) error {
		got = spec
		return nil
	})
	action := plan.ExternalAction{Command: plan.CommandSpec{Name: "printf", Args: []string{"$(touch nope)", "a b"}}}
	if err := runner.Run(context.Background(), action); err != nil {
		t.Fatal(err)
	}
	if got.Name == "sh" || got.Name == "bash" || strings.Contains(got.Name, "-c") {
		t.Fatalf("shell command constructed: %#v", got)
	}
}

func TestRunnerReturnsTypedExitAndNotFoundErrors(t *testing.T) {
	runner := NewRunner(nil)
	err := runner.Run(context.Background(), plan.ExternalAction{Description: "exit", Command: plan.CommandSpec{Name: os.Args[0], Args: []string{"-test.run=TestRunnerExitHelper", "--"}, Env: map[string]string{"RUNNER_EXIT": "1"}}})
	var actionErr *report.ExternalActionError
	var exitErr *exec.ExitError
	if !errors.As(err, &actionErr) || !errors.As(err, &exitErr) {
		t.Fatalf("exit error = %v", err)
	}
	err = runner.Run(context.Background(), plan.ExternalAction{Description: "missing", Command: plan.CommandSpec{Name: filepath.Join(t.TempDir(), "missing")}})
	var pathErr *os.PathError
	if !errors.As(err, &actionErr) || !errors.As(err, &pathErr) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestRunnerExitHelper(t *testing.T) {
	if os.Getenv("RUNNER_EXIT") == "1" {
		os.Exit(7)
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("RUNNER_MARKER") != "present" {
		return
	}
	data, _ := io.ReadAll(os.Stdin)
	_ = data
	os.WriteFile(filepath.Join(mustGetwd(t), "runner-output"), []byte(os.Args[len(os.Args)-1]+":"+os.Getenv("RUNNER_MARKER")), 0600)
	os.Exit(0)
}
func mustGetwd(t *testing.T) string {
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

var _ = exec.ErrNotFound
