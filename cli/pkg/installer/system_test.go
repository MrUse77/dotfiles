package installer

import (
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestActionCatalogSystemOperationsAreOrderedAndNonExecuting(t *testing.T) {
	actions, err := NewActionCatalog().ExternalActions(plan.Options{EnableSSHAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	for i, action := range actions {
		if action.Order != i {
			t.Errorf("action %d has order %d", i, action.Order)
		}
		if action.Command.Name == "sh" && len(action.Command.Args) > 0 && action.Command.Args[0] == "-c" {
			t.Errorf("action %d constructs a shell command: %#v", i, action.Command)
		}
	}
}

func TestManagedTargetsIncludeFontsAndCursors(t *testing.T) {
	targets, err := NewActionCatalog().ManagedTargets("/repo", "/home/test", plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	fonts, icons := false, false
	for _, target := range targets {
		if target.Kind != plan.CopyTree {
			continue
		}
		switch target.Destination {
		case "/home/test/.local/share/fonts":
			fonts = true
		case "/home/test/.local/share/icons":
			icons = true
		}
	}
	if !fonts || !icons {
		t.Fatalf("font and cursor directories must both be managed targets: fonts=%v icons=%v", fonts, icons)
	}
}
