package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func TestResolveRepositoryRoot(t *testing.T) {
	tests := []struct {
		name       string
		withMarker bool
		start      func(string) string
		wantRoot   bool
		wantErr    string
	}{
		{
			name:       "cwd at CLI subdirectory resolves parent root",
			withMarker: true,
			start: func(root string) string {
				return filepath.Join(root, "cli")
			},
			wantRoot: true,
		},
		{
			name:       "cwd at repository root resolves itself",
			withMarker: true,
			start: func(root string) string {
				return root
			},
			wantRoot: true,
		},
		{
			name:       "nested cwd resolves ancestor",
			withMarker: true,
			start: func(root string) string {
				return filepath.Join(root, "cli", "cmd")
			},
			wantRoot: true,
		},
		{
			name: "no repository root marker returns a clear error",
			start: func(root string) string {
				return filepath.Join(root, "nested")
			},
			wantErr: "no Git repository root found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.withMarker {
				if err := os.Mkdir(filepath.Join(root, ".git"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			start := tt.start(root)
			if err := os.MkdirAll(start, 0755); err != nil {
				t.Fatal(err)
			}

			got, err := resolveRepositoryRoot(start)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("resolveRepositoryRoot() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRepositoryRoot() error = %v", err)
			}
			if !tt.wantRoot || got != root {
				t.Fatalf("resolveRepositoryRoot() = %q, want %q", got, root)
			}
		})
	}
}

func TestInstallDiscovererIsReadOnlyAndIncludesManagedTargets(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	configDir := filepath.Join(repo, ".config")
	if err := os.MkdirAll(filepath.Join(configDir, "hypr"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "waybar"), []byte("config"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "hypr", "hyprland.conf"), []byte("config"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("hypr", filepath.Join(configDir, "current")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".zshrc"), []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(repo, ".local", "bin", "moonarch"),
		filepath.Join(repo, ".local", "share", "moonarch", "themes", "tokyo-night"),
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	targets, err := (installDiscoverer{}).Discover(repo, home, plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 6 {
		t.Fatalf("targets = %d, want three config children, root file, and MoonArch trees", len(targets))
	}
	wantConfigTargets := []struct {
		name string
		kind plan.MutationKind
	}{
		{name: "current", kind: plan.Symlink},
		{name: "hypr", kind: plan.CopyTree},
		{name: "waybar", kind: plan.CopyFile},
	}
	var configOrder []string
	for _, target := range targets {
		if filepath.Dir(target.Source) == configDir {
			configOrder = append(configOrder, filepath.Base(target.Source))
		}
	}
	if got := strings.Join(configOrder, ","); got != "current,hypr,waybar" {
		t.Fatalf("config target order = %q, want current,hypr,waybar", got)
	}
	for _, want := range wantConfigTargets {
		found := false
		for _, target := range targets {
			if target.Source == filepath.Join(configDir, want.name) &&
				target.Destination == filepath.Join(home, ".config", want.name) &&
				target.Kind == want.kind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing managed config target %q with kind %q", want.name, want.kind)
		}
	}
	for _, target := range targets {
		if target.Source == configDir || target.Destination == filepath.Join(home, ".config") {
			t.Fatalf("root .config target must not be emitted: %#v", target)
		}
	}
	after, err := os.ReadDir(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("planning mutated repository entries: before=%d after=%d", len(before), len(after))
	}
}

func TestInstallDiscovererKeepsLegacyRootSymlinkAsCopyFile(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	for _, path := range []string{
		filepath.Join(repo, ".local", "bin", "moonarch"),
		filepath.Join(repo, ".local", "share", "moonarch", "themes"),
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, "zshrc-source"), []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("zshrc-source", filepath.Join(repo, ".zshrc")); err != nil {
		t.Fatal(err)
	}

	targets, err := (installDiscoverer{}).Discover(repo, home, plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Source == filepath.Join(repo, ".zshrc") {
			if target.Kind != plan.CopyFile {
				t.Fatalf("legacy root symlink kind = %q, want %q", target.Kind, plan.CopyFile)
			}
			return
		}
	}
	t.Fatal("missing legacy root .zshrc target")
}

func TestInstallDiscovererFailsWhenMoonArchRuntimeIsMissing(t *testing.T) {
	repo := t.TempDir()
	if _, err := (installDiscoverer{}).Discover(repo, t.TempDir(), plan.Options{}); err == nil {
		t.Fatal("missing MoonArch runtime must block target discovery")
	}
}

func TestInstallDiscovererPlansMoonArchRuntimeTrees(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	for _, path := range []string{
		filepath.Join(repo, ".local", "bin", "moonarch"),
		filepath.Join(repo, ".local", "share", "moonarch", "themes", "tokyo-night"),
	} {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("tokyo-night", filepath.Join(repo, ".local", "share", "moonarch", "themes", "current")); err != nil {
		t.Fatal(err)
	}

	targets, err := (installDiscoverer{}).Discover(repo, home, plan.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		filepath.Join(repo, ".local", "bin", "moonarch"):             filepath.Join(home, ".local", "bin", "moonarch"),
		filepath.Join(repo, ".local", "share", "moonarch", "themes"): filepath.Join(home, ".local", "share", "moonarch", "themes"),
	}
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

func TestRootCommandDoesNotExposeDirectCopyTheme(t *testing.T) {
	if _, _, err := rootCmd.Find([]string{"theme"}); err == nil {
		t.Fatal("dots exposes obsolete direct-copy theme command")
	}
}

func TestPrintExecutionReportIncludesFingerprintAndRetainedBackup(t *testing.T) {
	var out bytes.Buffer
	printExecutionReport(&out, &report.ExecutionReport{
		Fingerprint: "abc",
		ManagedTargets: []report.TargetOutcome{{
			Destination: "/home/user/.zshrc", Status: report.TargetMutated, BackupPath: "/backup/zshrc",
		}},
		ExternalActions: []report.ActionOutcome{{Description: "install packages", Status: report.ActionCompleted}},
	})
	for _, want := range []string{"abc", "/home/user/.zshrc", "/backup/zshrc", "install packages"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report output missing %q: %s", want, out.String())
		}
	}
}
