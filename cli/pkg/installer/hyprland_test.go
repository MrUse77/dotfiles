package installer

import (
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestActionCatalogHyprlandPluginsAreOrderedSupplyChainCommands(t *testing.T) {
	actions, err := NewActionCatalog().ExternalActions(plan.Options{InstallPlugins: true})
	if err != nil {
		t.Fatal(err)
	}
	var last int
	seen := 0
	for _, action := range actions {
		if action.Command.Name != "hyprpm" {
			continue
		}
		if action.Classification != "supply-chain" {
			t.Errorf("hyprpm action classified as %q", action.Classification)
		}
		if seen > 0 && action.Order <= last {
			t.Errorf("hyprpm actions are not ordered: %d after %d", action.Order, last)
		}
		last, seen = action.Order, seen+1
	}
	if seen != 5 {
		t.Fatalf("hyprpm actions = %d, want 5", seen)
	}
}
