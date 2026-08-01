package installer

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
)

func TestActionCatalogParuBootstrapDependsOnAvailability(t *testing.T) {
	tests := []struct {
		name          string
		paruAvailable bool
		wantBootstrap bool
		wantPrefix    []string
	}{
		{
			name:          "paru absent cleans and bootstraps before configured packages",
			paruAvailable: false,
			wantBootstrap: true,
			wantPrefix: []string{
				"update system and install base tools",
				"clean paru build directory",
				"bootstrap paru",
				"build and install paru",
				"install configured packages",
			},
		},
		{
			name:          "paru installed skips all bootstrap actions",
			paruAvailable: true,
			wantBootstrap: false,
			wantPrefix:    []string{"update system and install base tools", "install configured packages"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions, err := NewActionCatalogWithParu(tt.paruAvailable).ExternalActions(t.TempDir(), t.TempDir(), plan.Options{HasAMD: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(actions) < len(tt.wantPrefix) {
				t.Fatalf("actions = %d, want at least %d: %#v", len(actions), len(tt.wantPrefix), actions)
			}
			for i, action := range actions {
				if action.Order != i {
					t.Errorf("action %d order = %d, want %d", i, action.Order, i)
				}
			}
			for i, want := range tt.wantPrefix {
				if actions[i].Description != want {
					t.Errorf("action %d description = %q, want %q", i, actions[i].Description, want)
				}
			}

			var cleanup, bootstrap, build, packages *plan.ExternalAction
			for i := range actions {
				switch actions[i].Description {
				case "clean paru build directory":
					cleanup = &actions[i]
				case "bootstrap paru":
					bootstrap = &actions[i]
				case "build and install paru":
					build = &actions[i]
				case "install configured packages":
					packages = &actions[i]
				}
			}
			if packages == nil {
				t.Fatal("configured package action is missing")
			}
			if packages.Command.Name != "paru" || packages.Classification != "supply-chain" {
				t.Fatalf("package action = %#v", packages)
			}
			joined := strings.Join(packages.Command.Args, " ")
			for _, required := range []string{"zsh", "hyprland", "neovim", "corectrl"} {
				if !strings.Contains(joined, required) {
					t.Errorf("package action missing %q: %v", required, packages.Command.Args)
				}
			}

			if tt.wantBootstrap {
				if cleanup == nil || bootstrap == nil || build == nil {
					t.Fatalf("bootstrap actions = cleanup:%v bootstrap:%v build:%v", cleanup != nil, bootstrap != nil, build != nil)
				}
				if cleanup.Command.Name != "rm" || !reflect.DeepEqual(cleanup.Command.Args, []string{"-rf", "--", paruBuildDir}) {
					t.Errorf("cleanup command = %#v, want rm -rf -- %s", cleanup.Command, paruBuildDir)
				}
				if cleanup.Classification != "filesystem" || !cleanup.Irreversible {
					t.Errorf("cleanup metadata = classification %q, irreversible %v", cleanup.Classification, cleanup.Irreversible)
				}
				if !reflect.DeepEqual(bootstrap.Command.Args, []string{"clone", "https://aur.archlinux.org/paru.git", paruBuildDir}) {
					t.Errorf("bootstrap args = %v, want clone into %s", bootstrap.Command.Args, paruBuildDir)
				}
				if build.Command.Dir != paruBuildDir {
					t.Errorf("build directory = %q, want %q", build.Command.Dir, paruBuildDir)
				}
			} else if cleanup != nil || bootstrap != nil || build != nil {
				t.Fatalf("paru-available plan includes bootstrap actions: cleanup:%v bootstrap:%v build:%v", cleanup != nil, bootstrap != nil, build != nil)
			}
		})
	}
}

func TestDetectParuUsesPATH(t *testing.T) {
	tests := []struct {
		name        string
		installParu bool
		want        bool
	}{
		{name: "executable paru on PATH", installParu: true, want: true},
		{name: "paru absent from PATH", installParu: false, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			if tt.installParu {
				path := filepath.Join(binDir, "paru")
				if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", binDir)

			if got := DetectParu(); got != tt.want {
				t.Errorf("DetectParu() = %v, want %v", got, tt.want)
			}
		})
	}
}
