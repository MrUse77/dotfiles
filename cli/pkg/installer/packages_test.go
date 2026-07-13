package installer

import (
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestActionCatalogPackagesAreOrderedClassifiedCommands(t *testing.T) {
	actions, err := NewActionCatalog().ExternalActions(plan.Options{HasAMD: true})
	if err != nil {
		t.Fatal(err)
	}

	if actions[0].Command.Name != "sudo" || actions[0].Classification != "privileged" {
		t.Fatalf("base action = %#v", actions[0])
	}
	if actions[1].Description != "bootstrap paru" || actions[1].Command.Name != "git" {
		t.Fatalf("paru bootstrap = %#v", actions[1])
	}
	if actions[2].Description != "build and install paru" || actions[2].Command.Dir != "/tmp/paru-install" {
		t.Fatalf("paru build = %#v", actions[2])
	}
	packages := actions[3]
	if packages.Description != "install configured packages" || packages.Command.Name != "paru" || packages.Classification != "supply-chain" {
		t.Fatalf("package action = %#v", packages)
	}
	joined := strings.Join(packages.Command.Args, " ")
	for _, required := range []string{"zsh", "hyprland", "neovim", "corectrl"} {
		if !strings.Contains(joined, required) {
			t.Errorf("package action missing %q: %v", required, packages.Command.Args)
		}
	}
}
