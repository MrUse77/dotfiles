package cmd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

// countingReleaseClient records call counts for zero-invocation assertions.
type countingReleaseClient struct {
	calls int
}

func (c *countingReleaseClient) Latest(context.Context) (release.Release, error) {
	c.calls++
	return release.Release{}, nil
}
func (c *countingReleaseClient) GetByTag(context.Context, string) (release.Release, error) {
	c.calls++
	return release.Release{}, nil
}
func (c *countingReleaseClient) Download(context.Context, release.Asset) (io.ReadCloser, error) {
	c.calls++
	return nil, nil
}

// countingBinaryReplacer records replacement calls.
type countingBinaryReplacer struct {
	calls int
}

func (c *countingBinaryReplacer) Replace(context.Context, string, string, io.Reader, io.Reader) error {
	c.calls++
	return nil
}

// countingSelfUpdateFactory records whether it was invoked.
type countingSelfUpdateFactory struct {
	called bool
	deps   selfUpdateDependencies
}

func (f *countingSelfUpdateFactory) make(_ *cobra.Command) selfUpdateDependencies {
	f.called = true
	return f.deps
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

func TestUpdateCommand_HelpIsCLIOnly(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"update"})
	if err != nil {
		t.Fatalf("update command not found: %v", err)
	}
	if !strings.Contains(cmd.Short, "CLI") {
		t.Fatalf("update short help = %q, want a CLI mention", cmd.Short)
	}
	if strings.Contains(cmd.Short, "dotfiles") {
		t.Fatalf("update short help must not promise configuration changes: %q", cmd.Short)
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
	releaseClient := &countingReleaseClient{}
	replacer := &countingBinaryReplacer{}
	factory := &countingSelfUpdateFactory{deps: selfUpdateDependencies{
		releaseClient: releaseClient,
		replacer:      replacer,
	}}
	cmd := newUpdateCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runSelfUpdateWithFactory(cmd, "dev", factory.make)
	}

	if err := cmd.Execute(); err != nil {
		t.Fatalf("dev build should exit with nil: %v", err)
	}
	if factory.called {
		t.Fatalf("factory was called in dev build")
	}
	if releaseClient.calls != 0 || replacer.calls != 0 {
		t.Fatalf("collaborators were invoked in dev build")
	}
	if out.String() == "" {
		t.Fatalf("expected release-build-only message")
	}
}
