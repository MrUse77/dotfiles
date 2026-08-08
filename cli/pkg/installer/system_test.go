package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
)

func TestActionCatalogSystemOperationsAreOrderedAndNonExecuting(t *testing.T) {
	actions, err := NewActionCatalog().ExternalActions(t.TempDir(), t.TempDir(), plan.Options{EnableSSHAgent: true})
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
	actions, err := NewActionCatalog().ExternalActions(t.TempDir(), t.TempDir(), plan.Options{EnableSSHAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range actions {
		if action.Command.Name == "sudo" && len(action.Command.Args) >= 2 && action.Command.Args[0] == "tee" && strings.HasPrefix(action.Command.Args[1], "/etc/profile.d/") {
			t.Errorf("profile action must not consume stdin: %#v", action.Command)
		}
	}
}

func TestActionCatalogSSHAgentOptionNoLongerAddsAction(t *testing.T) {
	withoutSSHAgent, err := NewActionCatalog().ExternalActions(t.TempDir(), t.TempDir(), plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	withSSHAgent, err := NewActionCatalog().ExternalActions(t.TempDir(), t.TempDir(), plan.Options{EnableSSHAgent: true})
	if err != nil {
		t.Fatal(err)
	}
	// SSH agent is now handled via .zshrc, not via systemd.
	if len(withSSHAgent) != len(withoutSSHAgent) {
		t.Fatalf("SSH agent option changed action count by %d, want 0", len(withSSHAgent)-len(withoutSSHAgent))
	}
}

func TestPowerProfilesActionsRespectTLPAndMaskedUnits(t *testing.T) {
	tests := []struct {
		name         string
		state        PowerProfilesState
		descriptions []string
		commands     [][]string
	}{
		{
			name:  "active TLP skips conflicting service",
			state: PowerProfilesState{TLPActive: true},
		},
		{
			name:         "masked service is unmasked before enabling",
			state:        PowerProfilesState{Masked: true},
			descriptions: []string{"unmask power profiles", "enable power profiles"},
			commands: [][]string{
				{"sudo", "systemctl", "unmask", "power-profiles-daemon.service"},
				{"sudo", "systemctl", "enable", "--now", "power-profiles-daemon.service"},
			},
		},
		{
			name:         "unmasked service is enabled directly",
			descriptions: []string{"enable power profiles"},
			commands:     [][]string{{"sudo", "systemctl", "enable", "--now", "power-profiles-daemon.service"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := powerProfilesActions(tt.state)
			if len(actions) != len(tt.descriptions) {
				t.Fatalf("actions = %d, want %d: %#v", len(actions), len(tt.descriptions), actions)
			}
			for i, want := range tt.descriptions {
				if actions[i].Description != want {
					t.Errorf("action %d description = %q, want %q", i, actions[i].Description, want)
				}
				if !sameStrings(actions[i].Command.Name, actions[i].Command.Args, tt.commands[i]) {
					t.Errorf("action %d command = %#v, want %v", i, actions[i].Command, tt.commands[i])
				}
			}
		})
	}
}

func TestActionCatalogAppliesPowerProfilesState(t *testing.T) {
	actions, err := NewActionCatalogWithPowerProfiles(PowerProfilesState{TLPActive: true}).ExternalActions(t.TempDir(), t.TempDir(), plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range actions {
		if strings.Contains(action.Description, "power profiles") {
			t.Fatalf("TLP-active plan must omit power-profile actions: %#v", action)
		}
	}
}

func TestDetectPowerProfilesReadsTLPAndMaskState(t *testing.T) {
	state := detectPowerProfiles(func(args ...string) (string, error) {
		if len(args) == 3 && args[0] == "is-active" {
			return "", nil
		}
		return "masked-runtime\n", os.ErrInvalid
	})
	if !state.TLPActive || !state.Masked {
		t.Fatalf("state = %#v, want active TLP and masked unit", state)
	}
}

func TestEnablePowerProfilesUsesSafeCommandOrder(t *testing.T) {
	var got [][]string
	run := func(name string, args ...string) error {
		got = append(got, append([]string{name}, args...))
		return nil
	}
	if err := enablePowerProfiles(run, func() PowerProfilesState { return PowerProfilesState{Masked: true} }); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"sudo", "systemctl", "unmask", "power-profiles-daemon.service"},
		{"sudo", "systemctl", "enable", "--now", "power-profiles-daemon.service"},
	}
	if len(got) != len(want) || !sameCommandLists(got, want) {
		t.Fatalf("commands = %v, want %v", got, want)
	}
}

func sameStrings(name string, args []string, want []string) bool {
	got := append([]string{name}, args...)
	return len(got) == len(want) && sameStringSlice(got, want)
}

func sameCommandLists(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !sameStringSlice(got[i], want[i]) {
			return false
		}
	}
	return true
}

func sameStringSlice(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
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
	want := map[string]string{"/repo/home/.local/share/moonarch/themes": "/home/test/.local/share/moonarch/themes"}
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
	source := filepath.Join(repo, "home", ".local", "share", "moonarch", "themes")
	if err := os.MkdirAll(filepath.Join(source, "tokyo-night"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tokyo-night", "manifest.toml"), []byte("id = \"tokyo-night\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("tokyo-night", filepath.Join(source, "current")); err != nil {
		t.Fatal(err)
	}

	destination := filepath.Join(home, ".local", "share", "moonarch", "themes")
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

func TestExternalActions_RepositoryAndHomeScopedCommands(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	actions, err := NewActionCatalog().ExternalActions(repo, home, plan.Options{})
	if err != nil {
		t.Fatalf("ExternalActions() error = %v", err)
	}

	found := map[string]bool{}
	for _, a := range actions {
		switch a.Description {
		case "update git submodules":
			found["submodules"] = true
			if a.Command.Dir != repo {
				t.Errorf("submodules Dir = %q, want repo root %q", a.Command.Dir, repo)
			}
		case "create zsh configuration directory":
			found["zsh-mkdir"] = true
			want := filepath.Join(home, ".config", "zsh")
			if len(a.Command.Args) != 2 || a.Command.Args[1] != want {
				t.Errorf("zsh mkdir args = %v, want [\"-p\" %q]", a.Command.Args, want)
			}
		}
	}
	for name := range found {
		if !found[name] {
			t.Errorf("action %q not found", name)
		}
	}
}
