package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/spf13/cobra"
)

func TestRootCommandExposesPlugins(t *testing.T) {
	command, _, err := rootCmd.Find([]string{"plugins"})
	if err != nil {
		t.Fatalf("Find(plugins) error = %v", err)
	}
	if command != pluginsCmd {
		t.Fatalf("plugins command = %p, want registered command %p", command, pluginsCmd)
	}
}

func TestRunPluginsWithDeps_RequiresHyprlandInstanceSignature(t *testing.T) {
	for _, value := range []string{"", "missing"} {
		t.Run(value, func(t *testing.T) {
			built := false
			deps := pluginsDependencies{
				lookupEnv: func(string) (string, bool) {
					if value == "missing" {
						return "", false
					}
					return value, true
				},
				buildPlan: func() (plan.InstallationPlan, error) {
					built = true
					return plan.InstallationPlan{}, nil
				},
			}
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetOut(&bytes.Buffer{})
			err := runPluginsWithDeps(cmd, deps)
			if err == nil {
				t.Fatal("runPluginsWithDeps() error = nil, want preflight error")
			}
			for _, want := range []string{"HYPRLAND_INSTANCE_SIGNATURE", "iniciá Hyprland", "moonarch-cli plugins"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q missing %q", err, want)
				}
			}
			if built {
				t.Fatal("plugin plan built before Hyprland preflight")
			}
		})
	}
}

func TestRunPluginsWithDeps_PropagatesPlanFailureAfterPreflight(t *testing.T) {
	wantErr := errors.New("plan failed")
	deps := pluginsDependencies{
		lookupEnv: func(string) (string, bool) { return "instance", true },
		buildPlan: func() (plan.InstallationPlan, error) { return plan.InstallationPlan{}, wantErr },
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runPluginsWithDeps(cmd, deps); !errors.Is(err, wantErr) {
		t.Fatalf("runPluginsWithDeps() error = %v, want %v", err, wantErr)
	}
}
