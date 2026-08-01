package installer

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestActionCatalog_PackageActions_BaseToolsFirstAndNoSubmodule(t *testing.T) {
	home := t.TempDir()
	catalog := NewActionCatalogWithParu(true)
	actions, err := catalog.PackageActions(home, plan.Options{})
	if err != nil {
		t.Fatalf("PackageActions() error = %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("PackageActions() returned no actions")
	}
	if actions[0].Description != "update system and install base tools" {
		t.Errorf("first action = %q, want base tools", actions[0].Description)
	}
	for _, a := range actions {
		if a.Description == "update git submodules" {
			t.Errorf("package plan must not contain submodule action: %#v", a)
		}
		if containsDescription(a.Description, "power profiles") {
			t.Errorf("package plan must not contain power-profile actions: %#v", a)
		}
	}
}

func TestActionCatalog_PackageActions_ParuBootstrapWhenMissing(t *testing.T) {
	home := t.TempDir()
	catalog := NewActionCatalogWithParu(false)
	actions, err := catalog.PackageActions(home, plan.Options{})
	if err != nil {
		t.Fatalf("PackageActions() error = %v", err)
	}
	want := []string{"clean paru build directory", "bootstrap paru", "build and install paru"}
	for _, desc := range want {
		if !hasAction(actions, desc) {
			t.Errorf("missing paru bootstrap action %q", desc)
		}
	}
}

func TestActionCatalog_PackageActions_IncludesSelectedGroups(t *testing.T) {
	home := t.TempDir()
	catalog := NewActionCatalogWithParu(true)
	opts := plan.Options{Groups: []string{plan.GroupPlugins, plan.GroupTheming}}
	actions, err := catalog.PackageActions(home, opts)
	if err != nil {
		t.Fatalf("PackageActions() error = %v", err)
	}
	if !hasAction(actions, "update Hyprland plugins") {
		t.Error("missing Hyprland plugin update action")
	}
	if !hasAction(actions, "set gtk-theme") {
		t.Error("missing GTK theme action")
	}
	if hasAction(actions, "enable power profiles") {
		t.Error("power-profile action must not appear in package plan")
	}
}

func TestActionCatalog_ConfigurationActions_PowerProfilesAndZshDirectory(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	catalog := NewActionCatalogWithPowerProfiles(PowerProfilesState{Masked: true})
	actions, err := catalog.ConfigurationActions(repo, home, plan.Options{}, nil)
	if err != nil {
		t.Fatalf("ConfigurationActions() error = %v", err)
	}
	if !hasAction(actions, "unmask power profiles") {
		t.Error("missing unmask power profiles action")
	}
	if !hasAction(actions, "enable power profiles") {
		t.Error("missing enable power profiles action")
	}
	if !hasAction(actions, "create zsh configuration directory") {
		t.Error("missing zsh directory action when no managed target owns the path")
	}
}

func TestActionCatalog_ConfigurationActions_OmitsZshDirectoryWhenManaged(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	catalog := NewActionCatalogWithPowerProfiles(PowerProfilesState{})
	managed := []plan.Target{
		{Source: filepath.Join(repo, ".config", "zsh"), Destination: filepath.Join(home, ".config", "zsh"), Kind: plan.CopyTree},
	}
	actions, err := catalog.ConfigurationActions(repo, home, plan.Options{}, managed)
	if err != nil {
		t.Fatalf("ConfigurationActions() error = %v", err)
	}
	if hasAction(actions, "create zsh configuration directory") {
		t.Error("zsh directory action must be omitted when a managed target owns the path")
	}
}

func TestActionCatalog_ConfigurationActions_OmitsZshDirectoryWhenParentManaged(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	catalog := NewActionCatalogWithPowerProfiles(PowerProfilesState{})
	managed := []plan.Target{
		{Source: filepath.Join(repo, ".config"), Destination: filepath.Join(home, ".config"), Kind: plan.CopyTree},
	}
	actions, err := catalog.ConfigurationActions(repo, home, plan.Options{}, managed)
	if err != nil {
		t.Fatalf("ConfigurationActions() error = %v", err)
	}
	if hasAction(actions, "create zsh configuration directory") {
		t.Error("zsh directory action must be omitted when parent managed target encloses the path")
	}
}

func TestActionCatalog_PhaseListsAreDisjoint(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	catalog := NewActionCatalogWithPowerProfilesAndParu(PowerProfilesState{Masked: true}, false)
	opts := plan.Options{Groups: []string{plan.GroupPlugins, plan.GroupTheming}}
	pkg, err := catalog.PackageActions(home, opts)
	if err != nil {
		t.Fatalf("PackageActions() error = %v", err)
	}
	cfg, err := catalog.ConfigurationActions(repo, home, opts, nil)
	if err != nil {
		t.Fatalf("ConfigurationActions() error = %v", err)
	}
	pkgDescs := descriptions(pkg)
	cfgDescs := descriptions(cfg)
	for _, d := range cfgDescs {
		if slices.Contains(pkgDescs, d) {
			t.Errorf("action %q appears in both package and configuration lists", d)
		}
	}
	for _, d := range pkgDescs {
		if slices.Contains(cfgDescs, d) {
			t.Errorf("action %q appears in both package and configuration lists", d)
		}
	}
}

func TestActionCatalog_ExternalActionsUnchanged(t *testing.T) {
	repo, home := t.TempDir(), t.TempDir()
	catalog := NewActionCatalogWithPowerProfilesAndParu(PowerProfilesState{Masked: true}, true)
	legacy, err := catalog.ExternalActions(repo, home, plan.Options{})
	if err != nil {
		t.Fatalf("ExternalActions() error = %v", err)
	}
	if len(legacy) == 0 {
		t.Fatal("ExternalActions() returned no actions")
	}
	if legacy[0].Description != "update system and install base tools" {
		t.Errorf("legacy first action = %q, want base tools", legacy[0].Description)
	}
	if !hasAction(legacy, "update git submodules") {
		t.Error("legacy ExternalActions missing submodule action")
	}
	if !hasAction(legacy, "enable power profiles") {
		t.Error("legacy ExternalActions missing power-profile action")
	}
}

func hasAction(actions []plan.ExternalAction, description string) bool {
	for _, a := range actions {
		if a.Description == description {
			return true
		}
	}
	return false
}

func descriptions(actions []plan.ExternalAction) []string {
	out := make([]string, len(actions))
	for i, a := range actions {
		out[i] = a.Description
	}
	return out
}

func containsDescription(desc, substring string) bool {
	return len(desc) >= len(substring) && desc[0:len(substring)] == substring || len(desc) > len(substring) && containsSubstr(desc, substring)
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
