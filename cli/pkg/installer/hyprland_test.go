package installer

import (
	"reflect"
	"testing"
)

func TestHyprlandPluginActions_OrderedSupplyChainCommands(t *testing.T) {
	actions := HyprlandPluginActions()
	want := []struct {
		args []string
	}{
		{args: []string{"update"}},
		{args: []string{"add", "https://github.com/hyprwm/hyprland-plugins"}},
		{args: []string{"add", "https://github.com/zjeffer/split-monitor-workspaces"}},
		{args: []string{"enable", "hyprbars"}},
		{args: []string{"enable", "split-monitor-workspaces"}},
		{args: []string{"reload"}},
	}
	if len(actions) != len(want) {
		t.Fatalf("HyprlandPluginActions() returned %d actions, want %d", len(actions), len(want))
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
		if !reflect.DeepEqual(action.Command.Args, want[i].args) {
			t.Errorf("action[%d].Command.Args = %#v, want %#v", i, action.Command.Args, want[i].args)
		}
	}
}
