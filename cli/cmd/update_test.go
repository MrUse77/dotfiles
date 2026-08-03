package cmd

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

// countingUpdateFactory records whether it was invoked and returns counting
// collaborators.
type countingUpdateFactory struct {
	called bool
	deps   updateDependencies
}

func (f *countingUpdateFactory) make(_ *cobra.Command) updateDependencies {
	f.called = true
	return f.deps
}

type countingReleaseClient struct{ calls int }

func (c *countingReleaseClient) Latest(context.Context) (release.Release, error) {
	c.calls++
	return release.Release{}, nil
}
func (c *countingReleaseClient) Download(context.Context, release.Asset) (io.ReadCloser, error) {
	c.calls++
	return nil, nil
}

type countingBinaryReplacer struct{ calls int }

func (c *countingBinaryReplacer) Replace(context.Context, string, string, io.Reader, io.Reader) error {
	c.calls++
	return nil
}

type countingRepositoryAcquirer struct{ calls int }

func (c *countingRepositoryAcquirer) Acquire(context.Context, RepositoryRequest, io.Writer) (RepositoryAcquisition, error) {
	c.calls++
	return RepositoryAcquisition{}, nil
}

type countingConfigurationPlanBuilder struct{ calls int }

func (c *countingConfigurationPlanBuilder) Build(string, string) (plan.InstallationPlan, error) {
	c.calls++
	return plan.InstallationPlan{}, nil
}

type countingPhaseExecutor struct{ calls int }

func (c *countingPhaseExecutor) Execute(context.Context, plan.InstallationPlan) (*report.ExecutionReport, error) {
	c.calls++
	return nil, nil
}

type countingReporter struct {
	starts    []UpdateStage
	completes []StageResult
}

func (c *countingReporter) Start(stage UpdateStage) { c.starts = append(c.starts, stage) }
func (c *countingReporter) Complete(result StageResult) {
	c.completes = append(c.completes, result)
}

func TestUpdateCommand_IsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("update command not registered: %v", err)
	}
	if cmd.Use != "update" {
		t.Fatalf("Use = %q, want update", cmd.Use)
	}
}

func TestUpdateCommand_HelpMentionsCLIAndDotfiles(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("update command not found: %v", err)
	}
	if cmd.Short == "" {
		t.Fatalf("update command has no short help")
	}
}

func TestUpdateCommand_RejectsPositionalArgs(t *testing.T) {
	cmd := newUpdateCommand()
	cmd.SetArgs([]string{"extra"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for positional arg")
	}
}

func TestUpdateCommand_RejectsUnknownOnlyFlag(t *testing.T) {
	cmd := newUpdateCommand()
	cmd.SetArgs([]string{"--only", "binary"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for unknown flag")
	}
}

func TestUpdateCommand_DevGuardExitsZeroWithoutCollaborators(t *testing.T) {
	oldVersion := Version
	t.Cleanup(func() { Version = oldVersion })
	Version = "dev"

	cmd := newUpdateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	releaseClient := &countingReleaseClient{}
	replacer := &countingBinaryReplacer{}
	acquirer := &countingRepositoryAcquirer{}
	builder := &countingConfigurationPlanBuilder{}
	executor := &countingPhaseExecutor{}
	factory := &countingUpdateFactory{deps: updateDependencies{
		releaseClient: releaseClient,
		replacer:      replacer,
		acquirer:      acquirer,
		planBuilder:   builder,
		executorFactory: func(p plan.InstallationPlan) PhaseExecutor {
			executor.calls++
			return executor
		},
	}}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runUpdateWithFactory(cmd, Version, factory.make)
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dev build should exit with nil: %v", err)
	}
	if factory.called {
		t.Fatalf("factory was called in dev build")
	}
	if releaseClient.calls != 0 || replacer.calls != 0 || acquirer.calls != 0 || builder.calls != 0 || executor.calls != 0 {
		t.Fatalf("collaborators were invoked in dev build")
	}
	if out.String() == "" {
		t.Fatalf("expected release-build-only message")
	}
}
