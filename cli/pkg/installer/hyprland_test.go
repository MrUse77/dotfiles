package installer

import (
	"reflect"
	"strings"
	"testing"
)

func TestDefaultHyprlandPlugins_CanonicalOrder(t *testing.T) {
	got := DefaultHyprlandPlugins()
	want := []HyprlandPlugin{
		{Name: "hyprbars", Repo: "https://github.com/hyprwm/hyprland-plugins"},
		{Name: "split-monitor-workspaces", Repo: "https://github.com/zjeffer/split-monitor-workspaces"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultHyprlandPlugins() = %#v, want %#v", got, want)
	}
}

func TestHyprlandPluginActions_Sequence(t *testing.T) {
	tests := []struct {
		name     string
		selected []string
		wantArgs [][]string
	}{
		{
			name:     "empty selection installs every plugin in canonical order",
			selected: nil,
			wantArgs: [][]string{
				{"update"},
				{"add", "https://github.com/hyprwm/hyprland-plugins"},
				{"add", "https://github.com/zjeffer/split-monitor-workspaces"},
				{"enable", "hyprbars"},
				{"enable", "split-monitor-workspaces"},
				{"reload"},
			},
		},
		{
			name:     "hyprbars only",
			selected: []string{"hyprbars"},
			wantArgs: [][]string{
				{"update"},
				{"add", "https://github.com/hyprwm/hyprland-plugins"},
				{"enable", "hyprbars"},
				{"reload"},
			},
		},
		{
			name:     "split-monitor-workspaces only",
			selected: []string{"split-monitor-workspaces"},
			wantArgs: [][]string{
				{"update"},
				{"add", "https://github.com/zjeffer/split-monitor-workspaces"},
				{"enable", "split-monitor-workspaces"},
				{"reload"},
			},
		},
		{
			name:     "reversed selection still follows canonical order",
			selected: []string{"split-monitor-workspaces", "hyprbars"},
			wantArgs: [][]string{
				{"update"},
				{"add", "https://github.com/hyprwm/hyprland-plugins"},
				{"add", "https://github.com/zjeffer/split-monitor-workspaces"},
				{"enable", "hyprbars"},
				{"enable", "split-monitor-workspaces"},
				{"reload"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := HyprlandPluginActions(tt.selected)
			if err != nil {
				t.Fatalf("HyprlandPluginActions(%v) error = %v", tt.selected, err)
			}
			if len(actions) != len(tt.wantArgs) {
				t.Fatalf("HyprlandPluginActions() returned %d actions, want %d", len(actions), len(tt.wantArgs))
			}
			for i, action := range actions {
				if action.Order != i {
					t.Errorf("action[%d].Order = %d, want %d", i, action.Order, i)
				}
				if action.Command.Name != "hyprpm" {
					t.Errorf("action[%d].Command.Name = %q, want hyprpm", i, action.Command.Name)
				}
				if action.Classification != "supply-chain" {
					t.Errorf("action[%d].Classification = %q, want supply-chain", i, action.Classification)
				}
				if !action.Irreversible {
					t.Errorf("action[%d].Irreversible = false, want true", i)
				}
				if !reflect.DeepEqual(action.Command.Args, tt.wantArgs[i]) {
					t.Errorf("action[%d].Command.Args = %#v, want %#v", i, action.Command.Args, tt.wantArgs[i])
				}
			}
		})
	}
}

func TestHyprlandPluginActions_UnknownSelection(t *testing.T) {
	_, err := HyprlandPluginActions([]string{"hyprbars", "does-not-exist"})
	if err == nil {
		t.Fatal("HyprlandPluginActions(unknown) error = nil, want error")
	}
	for _, want := range []string{"does-not-exist", "hyprbars", "split-monitor-workspaces"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
}
