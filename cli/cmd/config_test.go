package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/MrUse77/dots-cli/pkg/release"
)

type stubConfigOperations struct {
	apply func(context.Context, io.Writer, configApplyRequest) error
}

func (s *stubConfigOperations) Apply(ctx context.Context, out io.Writer, req configApplyRequest) error {
	if s.apply == nil {
		return nil
	}
	return s.apply(ctx, out, req)
}

func (*stubConfigOperations) Rollback(context.Context, io.Writer, configRollbackRequest) error {
	return nil
}

func (*stubConfigOperations) Status(context.Context, io.Writer, configStatusRequest) error {
	return nil
}

func TestConfigCommandDefinesOperationsAndFlags(t *testing.T) {
	t.Parallel()

	command := newConfigCommand(func(*cobra.Command) configOperations {
		return nil
	})

	apply, _, err := command.Find([]string{"apply"})
	if err != nil {
		t.Fatalf("find apply command: %v", err)
	}
	if apply.Use != "apply config-vX.Y.Z" {
		t.Fatalf("apply use = %q, want %q", apply.Use, "apply config-vX.Y.Z")
	}
	for _, name := range []string{"authorize-drift", "theme-replace"} {
		if apply.Flags().Lookup(name) == nil {
			t.Fatalf("apply flag --%s is missing", name)
		}
	}

	rollback, _, err := command.Find([]string{"rollback"})
	if err != nil {
		t.Fatalf("find rollback command: %v", err)
	}
	for _, name := range []string{"authorize-drift", "theme-replace", "offline"} {
		if rollback.Flags().Lookup(name) == nil {
			t.Fatalf("rollback flag --%s is missing", name)
		}
	}
	offline, err := rollback.Flags().GetBool("offline")
	if err != nil {
		t.Fatalf("read rollback --offline: %v", err)
	}
	if !offline {
		t.Fatal("rollback --offline must default to true")
	}

	status, _, err := command.Find([]string{"status"})
	if err != nil {
		t.Fatalf("find status command: %v", err)
	}
	if status.Flags().Lookup("json") == nil {
		t.Fatal("status flag --json is missing")
	}
}

func TestConfigApplyAcceptsOnlyExactRelease(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		selector  string
		wantError bool
	}{
		{name: "exact stable config release", selector: "config-v1.2.3"},
		{name: "latest selector", selector: "latest", wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var applied []configApplyRequest
			operations := &stubConfigOperations{apply: func(_ context.Context, _ io.Writer, req configApplyRequest) error {
				applied = append(applied, req)
				return nil
			}}
			command := newConfigCommand(func(*cobra.Command) configOperations { return operations })
			command.SetArgs([]string{"apply", tt.selector})

			err := command.Execute()
			if tt.wantError {
				if err == nil {
					t.Fatal("expected selector rejection")
				}
				if len(applied) != 0 {
					t.Fatalf("apply calls = %d, want 0", len(applied))
				}
				return
			}
			if err != nil {
				t.Fatalf("execute apply: %v", err)
			}
			if len(applied) != 1 || applied[0].Tag != tt.selector {
				t.Fatalf("apply requests = %#v, want tag %q", applied, tt.selector)
			}
		})
	}
}

func TestDefaultConfigOperationsUsesXDGStateAndDataRoots(t *testing.T) {
	home := t.TempDir()
	dataHome := filepath.Join(t.TempDir(), "data")
	stateHome := filepath.Join(t.TempDir(), "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("XDG_STATE_HOME", stateHome)

	operations := defaultConfigOperations(&cobra.Command{})
	runtime, ok := operations.(*configRuntime)
	if !ok {
		t.Fatalf("default operations type = %T, want *configRuntime", operations)
	}
	if runtime.deps.paths.dataRoot != filepath.Join(dataHome, "moonarch") {
		t.Fatalf("data root = %q", runtime.deps.paths.dataRoot)
	}
	if runtime.deps.paths.state != filepath.Join(stateHome, "moonarch", "state.json") {
		t.Fatalf("state path = %q", runtime.deps.paths.state)
	}
	if runtime.deps.paths.themeCurrent != filepath.Join(home, ".local", "share", "moonarch", "themes", "current") {
		t.Fatalf("theme current = %q", runtime.deps.paths.themeCurrent)
	}
	if runtime.deps.resolver == nil || runtime.deps.admitter == nil || runtime.deps.cache == nil || runtime.deps.journal == nil || runtime.deps.lock == nil {
		t.Fatal("default config operations omitted a required local/release dependency")
	}
}

func TestDefaultConfigOperationsFallsBackToHomeXDGDirectories(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_STATE_HOME", "")

	runtime := defaultConfigOperations(&cobra.Command{}).(*configRuntime)
	if runtime.deps.paths.dataRoot != filepath.Join(home, ".local", "share", "moonarch") {
		t.Fatalf("fallback data root = %q", runtime.deps.paths.dataRoot)
	}
	if runtime.deps.paths.stateRoot != filepath.Join(home, ".local", "state", "moonarch") {
		t.Fatalf("fallback state root = %q", runtime.deps.paths.stateRoot)
	}
}

func TestConfigIndeterminateJournalUsesExitCodeTwo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "indeterminate journal", err: fmt.Errorf("apply blocked: %w", release.ErrIndeterminateJournal), want: 2},
		{name: "ordinary command failure", err: errors.New("ordinary failure"), want: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := commandExitCode(tt.err); got != tt.want {
				t.Fatalf("commandExitCode(%v) = %d, want %d", tt.err, got, tt.want)
			}
		})
	}
}
