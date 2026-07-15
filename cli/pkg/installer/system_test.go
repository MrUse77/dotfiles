package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
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

func TestManagedTargetsIncludeMoonArchRuntimeTrees(t *testing.T) {
	targets, err := NewActionCatalog().ManagedTargets("/repo", "/home/test", plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"/repo/.local/moonarch/themes": "/home/test/.local/moonarch/themes"}
	for source, destination := range want {
		found := false
		for _, target := range targets {
			if target.Source == source && target.Destination == destination && target.Kind == plan.CopyTree {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing MoonArch CopyTree target %q -> %q", source, destination)
		}
	}
}

func TestCleanHomeCopyTreePreservesRelativeMoonArchCurrentLink(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, ".local", "moonarch", "themes")
	if err := os.MkdirAll(filepath.Join(source, "tokyo-night"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tokyo-night", "manifest.toml"), []byte("id = \"tokyo-night\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tokyo-night", filepath.Join(source, "current")); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(home, ".local", "moonarch", "themes")
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		t.Fatal(err)
	}
	target := plan.Target{
		Source:      source,
		Destination: destination,
		Kind:        plan.CopyTree,
		PreState:    plan.PreState{Type: plan.StateAbsent},
		BackupPath:  plan.BackupPath(filepath.Dir(destination), "moonarch-test", destination),
	}
	installationPlan, err := plan.NewInstallationPlan("moonarch-test", []plan.Target{target})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transaction.New(installationPlan).Execute(); err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(destination, "current"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "tokyo-night" {
		t.Fatalf("current link = %q, want relative tokyo-night", link)
	}
}
