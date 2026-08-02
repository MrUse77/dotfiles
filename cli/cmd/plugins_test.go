package cmd

import (
	"bytes"
	"context"
	"errors"
	"reflect"
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

func TestPluginsCmd_HasOnlyFlag(t *testing.T) {
	flag := pluginsCmd.Flags().Lookup("only")
	if flag == nil {
		t.Fatal("pluginsCmd has no --only flag")
	}
	if flag.Shorthand != "" {
		t.Errorf("--only shorthand = %q, want none", flag.Shorthand)
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
				buildPlan: func(_ []string) (plan.InstallationPlan, error) {
					built = true
					return plan.InstallationPlan{}, nil
				},
			}
			cmd := &cobra.Command{}
			cmd.SetContext(context.Background())
			cmd.SetOut(&bytes.Buffer{})
			err := runPluginsWithDeps(cmd, deps, nil)
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
		buildPlan: func(_ []string) (plan.InstallationPlan, error) { return plan.InstallationPlan{}, wantErr },
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runPluginsWithDeps(cmd, deps, nil); !errors.Is(err, wantErr) {
		t.Fatalf("runPluginsWithDeps() error = %v, want %v", err, wantErr)
	}
}

func TestRunPluginsWithDeps_PropagatesSelectionToBuildPlan(t *testing.T) {
	var got []string
	wantErr := errors.New("plan failed")
	deps := pluginsDependencies{
		lookupEnv: func(string) (string, bool) { return "instance", true },
		buildPlan: func(selected []string) (plan.InstallationPlan, error) {
			got = append([]string(nil), selected...)
			return plan.InstallationPlan{}, wantErr
		},
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runPluginsWithDeps(cmd, deps, []string{"hyprbars"}); !errors.Is(err, wantErr) {
		t.Fatalf("runPluginsWithDeps() error = %v, want %v", err, wantErr)
	}
	if !reflect.DeepEqual(got, []string{"hyprbars"}) {
		t.Fatalf("buildPlan received selection %#v, want [hyprbars]", got)
	}
}

func TestNewHyprlandPluginsPlan_EmptySelectionDefaultsToAllPlugins(t *testing.T) {
	p, err := newHyprlandPluginsPlan(nil)
	if err != nil {
		t.Fatalf("newHyprlandPluginsPlan(nil) error = %v", err)
	}
	actions := p.ExternalActions()
	if len(actions) != 6 {
		t.Fatalf("newHyprlandPluginsPlan(nil) built %d actions, want 6", len(actions))
	}
	var enables []string
	for _, a := range actions {
		if len(a.Command.Args) == 2 && a.Command.Args[0] == "enable" {
			enables = append(enables, a.Command.Args[1])
		}
	}
	want := []string{"hyprbars", "split-monitor-workspaces"}
	if !reflect.DeepEqual(enables, want) {
		t.Fatalf("enabled plugins = %#v, want %#v", enables, want)
	}
}
