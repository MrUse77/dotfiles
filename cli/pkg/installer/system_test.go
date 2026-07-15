package installer

import (
	"strings"
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

func TestActionCatalogDoesNotContainProfileTeeActions(t *testing.T) {
	actions, err := NewActionCatalog().ExternalActions(plan.Options{EnableSSHAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range actions {
		if action.Command.Name == "sudo" && len(action.Command.Args) >= 2 && action.Command.Args[0] == "tee" && strings.HasPrefix(action.Command.Args[1], "/etc/profile.d/") {
			t.Errorf("profile action must not consume stdin: %#v", action.Command)
		}
	}
}

func TestActionCatalogSSHAgentOptionAddsOnlyEnableAction(t *testing.T) {
	withoutSSHAgent, err := NewActionCatalog().ExternalActions(plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	withSSHAgent, err := NewActionCatalog().ExternalActions(plan.Options{EnableSSHAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(withSSHAgent), len(withoutSSHAgent)+1; got != want {
		t.Fatalf("SSH agent option added %d actions, want 1", got-len(withoutSSHAgent))
	}

	action := withSSHAgent[len(withoutSSHAgent)]
	if action.Description != "enable SSH agent" || action.Command.Name != "systemctl" || len(action.Command.Args) != 4 || action.Command.Args[0] != "--user" || action.Command.Args[1] != "enable" || action.Command.Args[2] != "--now" || action.Command.Args[3] != "ssh-agent" {
		t.Errorf("unexpected SSH agent action: %#v", action)
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
