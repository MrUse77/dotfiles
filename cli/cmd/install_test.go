package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
)

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

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
			// Isolate HOME so the canonical fallback (~/.cache/dotfiles) can
			// never leak a real clone into the test.
			t.Setenv("HOME", t.TempDir())
			t.Setenv("DOTFILES_DIR", "")

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

func TestNewInstallPlannerUsesDetectedParu(t *testing.T) {
	tests := []struct {
		name          string
		paruAvailable bool
		wantCleanup   bool
	}{
		{name: "paru available", paruAvailable: true, wantCleanup: false},
		{name: "paru unavailable", paruAvailable: false, wantCleanup: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binDir := t.TempDir()
			if tt.paruAvailable {
				if err := os.WriteFile(filepath.Join(binDir, "paru"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", binDir)

			actions, err := newInstallPlanner().Catalog.ExternalActions(t.TempDir(), t.TempDir(), plan.Options{})
			if err != nil {
				t.Fatal(err)
			}
			gotCleanup := false
			for _, action := range actions {
				if action.Description == "clean paru build directory" {
					gotCleanup = true
					break
				}
			}
			if gotCleanup != tt.wantCleanup {
				t.Errorf("cleanup action present = %v, want %v", gotCleanup, tt.wantCleanup)
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

func TestResolveRepositoryRoot_FallsBackToCanonicalHome(t *testing.T) {
	home := t.TempDir()
	clone := filepath.Join(home, ".cache", "dotfiles")
	if err := os.MkdirAll(filepath.Join(clone, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir clone: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")

	root, err := resolveRepositoryRoot(t.TempDir())
	if err != nil {
		t.Fatalf("resolveRepositoryRoot() error = %v", err)
	}
	if root != clone {
		t.Errorf("root = %q, want %q", root, clone)
	}
}

func TestResolveRepositoryRoot_DotfilesDirEnvWins(t *testing.T) {
	home := t.TempDir()
	envClone := filepath.Join(home, "custom-location")
	homeClone := filepath.Join(home, ".cache", "dotfiles")
	for _, dir := range []string{envClone, homeClone} {
		if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", envClone)

	root, err := resolveRepositoryRoot(t.TempDir())
	if err != nil {
		t.Fatalf("resolveRepositoryRoot() error = %v", err)
	}
	if root != envClone {
		t.Errorf("root = %q, want DOTFILES_DIR %q", root, envClone)
	}
}

func TestEnsureRepositoryClone(t *testing.T) {
	source := t.TempDir()
	mustRunGit(t, source, "init", "-q", "-b", "main")
	mustWriteFile(t, filepath.Join(source, ".zshrc"), []byte("zsh"))
	mustRunGit(t, source, "add", ".")
	mustRunGit(t, source, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")
	t.Setenv("DOTFILES_REPO", source)
	t.Setenv("DOTFILES_BRANCH", "main")

	var out strings.Builder
	root, err := ensureRepositoryClone(&out)
	if err != nil {
		t.Fatalf("ensureRepositoryClone() error = %v", err)
	}
	want := filepath.Join(home, ".cache", "dotfiles")
	if root != want {
		t.Errorf("root = %q, want %q", root, want)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		t.Errorf("clone missing .git: %v", err)
	}
	if got := readFileString(t, filepath.Join(root, ".zshrc")); got != "zsh" {
		t.Errorf("cloned .zshrc = %q, want zsh", got)
	}
}

func TestEnsureRepositoryClone_UpdatesExisting(t *testing.T) {
	source := t.TempDir()
	mustRunGit(t, source, "init", "-q", "-b", "main")
	mustWriteFile(t, filepath.Join(source, ".zshrc"), []byte("v1"))
	mustRunGit(t, source, "add", ".")
	mustRunGit(t, source, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "v1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")
	t.Setenv("DOTFILES_REPO", source)
	t.Setenv("DOTFILES_BRANCH", "main")

	var out strings.Builder
	root, err := ensureRepositoryClone(&out)
	if err != nil {
		t.Fatalf("first ensureRepositoryClone() error = %v", err)
	}

	// Advance the source and re-run: the existing clone must be updated, not
	// re-cloned or rejected.
	mustWriteFile(t, filepath.Join(source, ".zshrc"), []byte("v2"))
	mustRunGit(t, source, "add", ".")
	mustRunGit(t, source, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "v2")

	root, err = ensureRepositoryClone(&out)
	if err != nil {
		t.Fatalf("second ensureRepositoryClone() error = %v", err)
	}
	if got := readFileString(t, filepath.Join(root, ".zshrc")); got != "v2" {
		t.Errorf("updated .zshrc = %q, want v2", got)
	}
}

func mustRunGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestEnsureRepositoryClone_UsesOwnVersionTag(t *testing.T) {
	source := t.TempDir()
	mustRunGit(t, source, "init", "-q", "-b", "main")
	mustWriteFile(t, filepath.Join(source, ".zshrc"), []byte("tag-content"))
	mustRunGit(t, source, "add", ".")
	mustRunGit(t, source, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "tag")
	mustRunGit(t, source, "tag", "v0.1.0")
	mustWriteFile(t, filepath.Join(source, ".zshrc"), []byte("main-content"))
	mustRunGit(t, source, "add", ".")
	mustRunGit(t, source, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "main")

	old := Version
	t.Cleanup(func() { Version = old })
	Version = "v0.1.0"

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")
	t.Setenv("DOTFILES_REPO", source)
	t.Setenv("DOTFILES_BRANCH", "")

	var out strings.Builder
	root, err := ensureRepositoryClone(&out)
	if err != nil {
		t.Fatalf("ensureRepositoryClone() error = %v", err)
	}
	if got := readFileString(t, filepath.Join(root, ".zshrc")); got != "tag-content" {
		t.Errorf("cloned .zshrc = %q, want tag content (not main)", got)
	}
}
