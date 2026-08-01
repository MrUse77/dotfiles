package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (c fakeClock) Now() time.Time { return c.now }

type fixedRunID struct {
	id string
}

func (f fixedRunID) Generate(now time.Time) string { return f.id }

type fakeDiscoverer struct {
	targets []Target
	err     error
}

func (f *fakeDiscoverer) Discover(repoRoot, homeDir string, opts Options) ([]Target, error) {
	return f.targets, f.err
}

type fakeCatalog struct {
	actions []ExternalAction
	err     error
}

func (f *fakeCatalog) ExternalActions(repoRoot, homeDir string, opts Options) ([]ExternalAction, error) {
	return f.actions, f.err
}

func mustWriteFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func TestBuildPlan(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	zshrc := filepath.Join(repo, ".zshrc")
	mustWriteFile(t, zshrc, []byte("zsh config"))

	configDir := filepath.Join(repo, ".config")
	mustMkdirAll(t, configDir)

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: zshrc, Destination: filepath.Join(home, ".zshrc"), Kind: CopyFile},
			{Source: configDir, Destination: filepath.Join(home, ".config"), Kind: CopyTree},
		},
	}
	catalog := &fakeCatalog{
		actions: []ExternalAction{
			{
				Description:    "update system",
				Command:        CommandSpec{Name: "sudo", Args: []string{"pacman", "-Syu"}},
				Classification: "privileged",
				Irreversible:   true,
				Order:          1,
			},
		},
	}
	planner := New(
		WithDiscoverer(disc),
		WithCatalog(catalog),
		WithClock(fakeClock{now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}),
		WithRunIDSource(fixedRunID{id: "20260712T120000Z-abc"}),
	)

	plan, err := planner.Build(repo, home, Options{Mode: "user"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	if plan.RunID != "20260712T120000Z-abc" {
		t.Errorf("RunID = %q, want %q", plan.RunID, "20260712T120000Z-abc")
	}
	if plan.Options.Mode != "user" {
		t.Errorf("Options.Mode = %q, want user", plan.Options.Mode)
	}
	if len(plan.managedTargets) != 2 {
		t.Errorf("len(ManagedTargets) = %d, want 2", len(plan.managedTargets))
	}
	if len(plan.externalActions) != 1 {
		t.Errorf("len(ExternalActions) = %d, want 1", len(plan.externalActions))
	}
	if plan.Fingerprint == "" {
		t.Error("Fingerprint is empty")
	}
}

func TestBuildPlan_CleanedAbsoluteTargetIdentity(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))

	rawDest := filepath.Join(home, "subdir", "..", "target")
	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: src, Destination: rawDest, Kind: CopyFile},
		},
	}

	plan, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	want := filepath.Join(home, "target")
	if plan.managedTargets[0].Destination != want {
		t.Errorf("Destination = %q, want %q", plan.managedTargets[0].Destination, want)
	}
	if plan.managedTargets[0].Source != src {
		t.Errorf("Source = %q, want %q", plan.managedTargets[0].Source, src)
	}
}

func TestBuildPlan_SourceWithinRepoValidation(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	outside := t.TempDir()

	src := filepath.Join(outside, "file")
	mustWriteFile(t, src, []byte("x"))

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: src, Destination: filepath.Join(home, "file"), Kind: CopyFile},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error for source outside repo")
	}
	var outsideErr *SourceOutsideRepoError
	if !errors.As(err, &outsideErr) {
		t.Fatalf("expected *SourceOutsideRepoError, got %T: %v", err, err)
	}
	if outsideErr.Source != src {
		t.Errorf("Source = %q, want %q", outsideErr.Source, src)
	}
}

func TestBuildPlan_DuplicateTargetRejection(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	srcA := filepath.Join(repo, "a")
	srcB := filepath.Join(repo, "b")
	mustWriteFile(t, srcA, []byte("a"))
	mustWriteFile(t, srcB, []byte("b"))

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: srcA, Destination: filepath.Join(home, "x"), Kind: CopyFile},
			{Source: srcB, Destination: filepath.Join(home, "x"), Kind: CopyFile},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error for duplicate target")
	}
	var dupErr *DuplicateTargetError
	if !errors.As(err, &dupErr) {
		t.Fatalf("expected *DuplicateTargetError, got %T: %v", err, err)
	}
	if dupErr.Destination != filepath.Join(home, "x") {
		t.Errorf("Destination = %q, want %q", dupErr.Destination, filepath.Join(home, "x"))
	}
}

func TestBuildPlan_AncestorDescendantOverlapRejection(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	configDir := filepath.Join(repo, ".config")
	hyprDir := filepath.Join(configDir, "hypr")
	mustMkdirAll(t, hyprDir)

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: configDir, Destination: filepath.Join(home, ".config"), Kind: CopyTree},
			{Source: hyprDir, Destination: filepath.Join(home, ".config", "hypr"), Kind: CopyTree},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error for overlapping targets")
	}
	var overlapErr *OverlappingTargetsError
	if !errors.As(err, &overlapErr) {
		t.Fatalf("expected *OverlappingTargetsError, got %T: %v", err, err)
	}
}

func TestBuildPlan_RootLevelTargetInclusion(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	for _, name := range []string{".zshrc", ".gtkrc-2.0"} {
		mustWriteFile(t, filepath.Join(repo, name), []byte(name))
	}

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: filepath.Join(repo, ".zshrc"), Destination: filepath.Join(home, ".zshrc"), Kind: CopyFile},
			{Source: filepath.Join(repo, ".gtkrc-2.0"), Destination: filepath.Join(home, ".gtkrc-2.0"), Kind: CopyFile},
		},
	}

	plan, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	dests := make(map[string]bool)
	for _, tgt := range plan.managedTargets {
		dests[tgt.Destination] = true
	}
	for _, want := range []string{".zshrc", ".gtkrc-2.0"} {
		if !dests[filepath.Join(home, want)] {
			t.Errorf("missing root-level target %q", want)
		}
	}
}

func TestBuildPlan_OrderedExternalActions(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	catalog := &fakeCatalog{
		actions: []ExternalAction{
			{Description: "second", Order: 2, Classification: "external"},
			{Description: "first", Order: 1, Classification: "external"},
			{Description: "tie-a", Order: 3, Classification: "external"},
			{Description: "tie-b", Order: 3, Classification: "external"},
		},
	}

	plan, err := New(WithCatalog(catalog)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	got := make([]string, len(plan.externalActions))
	for i, a := range plan.externalActions {
		got[i] = a.Description
	}
	want := []string{"first", "second", "tie-a", "tie-b"}
	if len(got) != len(want) {
		t.Fatalf("len(actions) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("action[%d].Description = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFingerprint_EquivalentCanonicalInput(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	srcA := filepath.Join(repo, "a")
	srcB := filepath.Join(repo, "b")
	mustWriteFile(t, srcA, []byte("a"))
	mustWriteFile(t, srcB, []byte("b"))

	disc1 := &fakeDiscoverer{
		targets: []Target{
			{Source: srcA, Destination: filepath.Join(home, "a"), Kind: CopyFile},
			{Source: srcB, Destination: filepath.Join(home, "b"), Kind: CopyFile},
		},
	}
	disc2 := &fakeDiscoverer{
		targets: []Target{
			{Source: srcB, Destination: filepath.Join(home, "b"), Kind: CopyFile},
			{Source: srcA, Destination: filepath.Join(home, "a"), Kind: CopyFile},
		},
	}
	catalog := &fakeCatalog{
		actions: []ExternalAction{
			{Description: "z", Order: 2, Classification: "external"},
			{Description: "a", Order: 1, Classification: "external"},
		},
	}

	p := New(
		WithCatalog(catalog),
		WithClock(fakeClock{now: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)}),
		WithRunIDSource(fixedRunID{id: "run"}),
	)

	plan1, err := p.Clone(WithDiscoverer(disc1)).Build(repo, home, Options{Mode: "user"})
	if err != nil {
		t.Fatalf("Build() #1 error = %v", err)
	}
	plan2, err := p.Clone(WithDiscoverer(disc2)).Build(repo, home, Options{Mode: "user"})
	if err != nil {
		t.Fatalf("Build() #2 error = %v", err)
	}

	if plan1.Fingerprint != plan2.Fingerprint {
		t.Errorf("fingerprints differ for equivalent canonical input: %q vs %q", plan1.Fingerprint, plan2.Fingerprint)
	}
}

func TestBuildPlan_DiscovererError(t *testing.T) {
	disc := &fakeDiscoverer{err: errors.New("discovery failed")}
	_, err := New(WithDiscoverer(disc)).Build(t.TempDir(), t.TempDir(), Options{})
	if err == nil {
		t.Fatal("expected error from discoverer")
	}
}

func TestBuildPlan_CatalogError(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	catalog := &fakeCatalog{err: errors.New("catalog failed")}
	_, err := New(WithCatalog(catalog)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error from catalog")
	}
}

func TestBuildPlan_MissingSourceBlocksPlan(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: filepath.Join(repo, "missing"), Destination: filepath.Join(home, "x"), Kind: CopyFile},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestBuildPlan_PrerequisiteFailureWithoutMutation(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))

	// Destination parent does not exist and is not represented as a target.
	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: src, Destination: filepath.Join(home, "missing", "subdir", "file"), Kind: CopyFile},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected prerequisite error")
	}

	// No backup directory should have been created.
	backupRoot := filepath.Join(home, ".dots-backups")
	if _, statErr := os.Stat(backupRoot); statErr == nil {
		t.Errorf("backup root %s was created during planning", backupRoot)
	}
}

func TestBuildPlan_DeterministicBackupPath(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))

	runID := "20260712T120000Z-abc"
	dest := filepath.Join(home, ".config", "target")
	mustMkdirAll(t, filepath.Join(home, ".config"))
	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: src, Destination: dest, Kind: CopyFile},
		},
	}

	plan, err := New(
		WithDiscoverer(disc),
		WithRunIDSource(fixedRunID{id: runID}),
	).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	want := BackupPath(home, runID, dest)
	if plan.managedTargets[0].BackupPath != want {
		t.Errorf("BackupPath = %q, want %q", plan.managedTargets[0].BackupPath, want)
	}
}

func TestBuildPlan_WholeHomeDestinationRejected(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	src := filepath.Join(repo, "config")
	mustMkdirAll(t, src)
	mustWriteFile(t, filepath.Join(src, "managed.conf"), []byte("x"))

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: src, Destination: home, Kind: CopyTree},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected whole-home destination to be rejected")
	}
	if !strings.Contains(err.Error(), "home directory") {
		t.Errorf("error = %q, want mention of the home directory", err)
	}
}

func TestBuildPlan_RepoSymlinkResolvingOutsideIsRejected(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	outside := t.TempDir()

	outsideFile := filepath.Join(outside, "secret")
	mustWriteFile(t, outsideFile, []byte("secret"))

	link := filepath.Join(repo, "link")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: link, Destination: filepath.Join(home, "link"), Kind: CopyFile},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error for symlink resolving outside repo")
	}
	var outsideErr *SourceOutsideRepoError
	if !errors.As(err, &outsideErr) {
		t.Fatalf("expected *SourceOutsideRepoError, got %T: %v", err, err)
	}
}

func TestBuildPlan_RepoSymlinkResolvingInsideIsAccepted(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	realFile := filepath.Join(repo, "real")
	mustWriteFile(t, realFile, []byte("x"))

	link := filepath.Join(repo, "link")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	disc := &fakeDiscoverer{
		targets: []Target{
			{Source: link, Destination: filepath.Join(home, "link"), Kind: CopyFile},
		},
	}

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
}

func TestInstallationPlan_IsImmutableAfterBuild(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))

	action := ExternalAction{
		Description: "update",
		Command:     CommandSpec{Name: "cmd", Args: []string{"a", "b"}, Env: map[string]string{"K": "V"}},
	}
	disc := &fakeDiscoverer{targets: []Target{{Source: src, Destination: filepath.Join(home, "file"), Kind: CopyFile}}}
	catalog := &fakeCatalog{actions: []ExternalAction{action}}

	plan, err := New(WithDiscoverer(disc), WithCatalog(catalog)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	t.Run("target slice", func(t *testing.T) {
		view := plan.ManagedTargets()
		view[0].Source = "/mutated"
		view = append(view, Target{})
		if got := plan.ManagedTargets(); len(got) != 1 || got[0].Source != src {
			t.Errorf("target mutation leaked: %#v", got)
		}
	})

	t.Run("action command", func(t *testing.T) {
		view := plan.ExternalActions()
		view[0].Command.Args[0] = "mutated"
		view[0].Command.Env["K"] = "mutated"
		got := plan.ExternalActions()[0].Command
		if got.Args[0] != "a" || got.Env["K"] != "V" {
			t.Errorf("action command mutation leaked: args=%v env=%v", got.Args, got.Env)
		}
	})

	t.Run("catalog mutation after build", func(t *testing.T) {
		action.Command.Args[0] = "mutated"
		action.Command.Env["K"] = "mutated"
		got := plan.ExternalActions()[0].Command
		if got.Args[0] != "a" || got.Env["K"] != "V" {
			t.Errorf("catalog mutation leaked into plan: args=%v env=%v", got.Args, got.Env)
		}
	})
}

func TestBuildPlan_FingerprintError(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))

	disc := &fakeDiscoverer{targets: []Target{{Source: src, Destination: filepath.Join(home, "file"), Kind: CopyFile}}}
	old := canonicalMarshal
	canonicalMarshal = func(any) ([]byte, error) { return nil, errors.New("marshal failed") }
	defer func() { canonicalMarshal = old }()

	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error for fingerprint failure")
	}
	var fpErr *FingerprintError
	if !errors.As(err, &fpErr) {
		t.Fatalf("expected *FingerprintError, got %T: %v", err, err)
	}
}

func TestBuildPlan_BindsSourceContentAndResolvedIdentity(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	realFile := filepath.Join(repo, "real")
	mustWriteFile(t, realFile, []byte("v1"))
	link := filepath.Join(repo, "link")
	if err := os.Symlink(realFile, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	disc := &fakeDiscoverer{targets: []Target{{Source: link, Destination: filepath.Join(home, "link"), Kind: CopyFile}}}

	plan, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	tgt := plan.managedTargets[0]
	if tgt.ResolvedSource != realFile {
		t.Errorf("ResolvedSource = %q, want %q", tgt.ResolvedSource, realFile)
	}
	if tgt.SourceDigest == "" {
		t.Error("SourceDigest is empty")
	}
	if tgt.SourceBinding.PathIdentity.Inode == 0 {
		t.Error("SourceBinding.PathIdentity is empty for declared symlink")
	}
	if tgt.SourceBinding.LinkValue != realFile {
		t.Errorf("SourceBinding.LinkValue = %q, want %q", tgt.SourceBinding.LinkValue, realFile)
	}
	if tgt.SourceBinding.LinkDigest == "" {
		t.Error("SourceBinding.LinkDigest is empty for declared symlink")
	}

	// A content change after planning must change the fingerprint, proving the
	// reviewed plan is bound to source content and enables execution-drift checks.
	mustWriteFile(t, realFile, []byte("v2"))
	plan2, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() after content change error = %v", err)
	}
	if plan2.Fingerprint == plan.Fingerprint {
		t.Errorf("Fingerprint did not change after source content mutation: %q", plan.Fingerprint)
	}
	if plan2.managedTargets[0].SourceDigest == tgt.SourceDigest {
		t.Errorf("SourceDigest did not change after source content mutation")
	}
}

func TestBuildPlan_UnsupportedOrEmptyKindRejected(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))

	for _, kind := range []MutationKind{"", "move"} {
		disc := &fakeDiscoverer{targets: []Target{{Source: src, Destination: filepath.Join(home, "file"), Kind: kind}}}
		_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
		if err == nil {
			t.Fatalf("expected error for kind %q", kind)
		}
		var planErr *PlanError
		if !errors.As(err, &planErr) || planErr.Phase != "validation" {
			t.Fatalf("expected validation PlanError, got %T: %v", err, err)
		}
	}
}

func TestBuildPlan_ReadOnlyParentBlocksPlan(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses filesystem permission checks")
	}

	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))
	parent := filepath.Join(home, "readonly")
	mustMkdirAll(t, parent)
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(parent, 0755)

	disc := &fakeDiscoverer{targets: []Target{{Source: src, Destination: filepath.Join(parent, "file"), Kind: CopyFile}}}
	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error for read-only destination parent")
	}
	var planErr *PlanError
	if !errors.As(err, &planErr) || planErr.Phase != "prerequisite" {
		t.Fatalf("expected prerequisite PlanError, got %T: %v", err, err)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Errorf("expected os.ErrPermission in error chain, got %v", err)
	}
}

func TestBuildPlan_DestinationParentIsFile(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))
	parent := filepath.Join(home, "notadir")
	mustWriteFile(t, parent, []byte("x"))

	disc := &fakeDiscoverer{targets: []Target{{Source: src, Destination: filepath.Join(parent, "file"), Kind: CopyFile}}}
	_, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err == nil {
		t.Fatal("expected error when destination parent is not a directory")
	}
	var planErr *PlanError
	if !errors.As(err, &planErr) || planErr.Phase != "prerequisite" {
		t.Fatalf("expected prerequisite PlanError, got %T: %v", err, err)
	}
}

func TestBuildPlan_DoesNotCreateOrRemoveBackupRoot(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))
	disc := &fakeDiscoverer{targets: []Target{{Source: src, Destination: filepath.Join(home, "file"), Kind: CopyFile}}}

	// Planning succeeds and leaves no .dots-backups directory behind.
	if _, err := New(WithDiscoverer(disc)).Build(repo, home, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".dots-backups")); err == nil {
		t.Error("planning created .dots-backups directory")
	}

	// Planning preserves an existing empty .dots-backups directory.
	backups := filepath.Join(home, ".dots-backups")
	if err := os.Mkdir(backups, 0755); err != nil {
		t.Fatalf("mkdir .dots-backups: %v", err)
	}
	if _, err := New(WithDiscoverer(disc)).Build(repo, home, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if _, err := os.Stat(backups); err != nil {
		t.Error("planning removed an existing empty .dots-backups directory")
	}

	// Planning preserves existing backup content.
	if err := os.RemoveAll(backups); err != nil {
		t.Fatalf("remove .dots-backups: %v", err)
	}
	mustMkdirAll(t, backups)
	marker := filepath.Join(backups, "old")
	mustWriteFile(t, marker, []byte("old"))

	if _, err := New(WithDiscoverer(disc)).Build(repo, home, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("planning removed existing backup content")
	}
}

func TestSourceBinding_PlannerBindsFileIdentityAndFingerprint(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	source := filepath.Join(repo, "source")
	mustWriteFile(t, source, []byte("reviewed"))

	build := func() InstallationPlan {
		t.Helper()
		p, err := New(WithDiscoverer(&fakeDiscoverer{targets: []Target{{
			Source: source, Destination: filepath.Join(home, "destination"), Kind: CopyFile,
		}}})).Build(repo, home, Options{})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		return p
	}

	first := build()
	target := first.ManagedTargets()[0]
	if target.SourceBinding.Kind != "file" || target.SourceBinding.Digest == "" || target.SourceBinding.Identity.Inode == 0 {
		t.Fatalf("SourceBinding = %#v, want bound file identity and digest", target.SourceBinding)
	}
	if target.SourceDigest != target.SourceBinding.Digest {
		t.Errorf("SourceDigest = %q, want binding digest %q", target.SourceDigest, target.SourceBinding.Digest)
	}

	if err := os.Chmod(source, 0o755); err != nil {
		t.Fatalf("chmod source: %v", err)
	}
	second := build()
	if second.Fingerprint == first.Fingerprint {
		t.Error("fingerprint did not include source binding mode")
	}
}

func TestSourceBinding_RecordsRootSourceMode(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	for _, tt := range []struct {
		name  string
		kind  MutationKind
		mode  os.FileMode
		setup func(string)
	}{
		{
			name:  "file",
			kind:  CopyFile,
			mode:  os.FileMode(0o755) | os.ModeSetuid,
			setup: func(source string) { mustWriteFile(t, source, []byte("content")) },
		},
		{
			name:  "directory",
			kind:  CopyTree,
			mode:  os.FileMode(0o755) | os.ModeSticky,
			setup: func(source string) { mustMkdirAll(t, source) },
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			source := filepath.Join(repo, tt.name)
			tt.setup(source)
			if err := os.Chmod(source, tt.mode); err != nil {
				t.Fatalf("chmod source: %v", err)
			}

			p, err := New(WithDiscoverer(&fakeDiscoverer{targets: []Target{{
				Source: source, Destination: filepath.Join(home, tt.name), Kind: tt.kind,
			}}})).Build(repo, home, Options{})
			if err != nil {
				t.Fatalf("Build() error = %v", err)
			}

			if got := p.ManagedTargets()[0].SourceBinding.Mode; got != tt.mode {
				t.Errorf("SourceBinding.Mode = %#o, want %#o", got, tt.mode)
			}
		})
	}
}

func TestSourceBinding_DirectoryTreeManifest(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	srcDir := filepath.Join(repo, "config")
	mustMkdirAll(t, filepath.Join(srcDir, "sub"))
	mustWriteFile(t, filepath.Join(srcDir, "file.txt"), []byte("hello"))
	mustWriteFile(t, filepath.Join(srcDir, "sub", "nested"), []byte("world"))
	if err := os.Symlink("file.txt", filepath.Join(srcDir, "link")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	p, err := New(WithDiscoverer(&fakeDiscoverer{targets: []Target{{
		Source: srcDir, Destination: filepath.Join(home, ".config"), Kind: CopyTree,
	}}})).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	target := p.ManagedTargets()[0]
	if target.SourceBinding.Kind != "directory" {
		t.Fatalf("SourceBinding.Kind = %q, want directory", target.SourceBinding.Kind)
	}
	if len(target.SourceBinding.TreeManifest) == 0 {
		t.Fatal("TreeManifest is empty")
	}

	byPath := make(map[string]TreeManifestEntry)
	for _, e := range target.SourceBinding.TreeManifest {
		byPath[e.RelativePath] = e
	}

	if e, ok := byPath["file.txt"]; !ok || e.Kind != "file" || e.Digest == "" || e.Identity.Inode == 0 {
		t.Errorf("file.txt entry = %#v, want bound file with digest and identity", e)
	}
	if e, ok := byPath["sub"]; !ok || e.Kind != "directory" || e.Identity.Inode == 0 {
		t.Errorf("sub entry = %#v, want bound directory with identity", e)
	}
	if e, ok := byPath["sub/nested"]; !ok || e.Kind != "file" {
		t.Errorf("sub/nested entry = %#v, want bound file", e)
	}
	if e, ok := byPath["link"]; !ok || e.Kind != "symlink" || e.LinkValue != "file.txt" || e.Digest == "" {
		t.Errorf("link entry = %#v, want bound symlink", e)
	}
}

func TestSourceBinding_RetriesSymlinkSubstitutionWithoutMixedBinding(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	first := filepath.Join(repo, "first")
	second := filepath.Join(repo, "second")
	mustWriteFile(t, first, []byte("first"))
	mustWriteFile(t, second, []byte("second"))
	source := filepath.Join(repo, "source")
	if err := os.Symlink(first, source); err != nil {
		t.Fatalf("symlink source: %v", err)
	}

	calls := 0
	sourceBindingCaptureHook = func() {
		calls++
		if calls != 1 {
			return
		}
		if err := os.Remove(source); err != nil {
			t.Fatalf("remove source: %v", err)
		}
		if err := os.Symlink(second, source); err != nil {
			t.Fatalf("replace source: %v", err)
		}
	}
	defer func() { sourceBindingCaptureHook = nil }()

	p, err := New(WithDiscoverer(&fakeDiscoverer{targets: []Target{{
		Source: source, Destination: filepath.Join(home, "destination"), Kind: CopyFile,
	}}})).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	binding := p.ManagedTargets()[0].SourceBinding
	if calls < 2 {
		t.Fatalf("capture calls = %d, want retry", calls)
	}
	if binding.LinkValue != second {
		t.Errorf("LinkValue = %q, want coherent replacement link %q", binding.LinkValue, second)
	}
	wantDigest, err := SourceDigestForPath(second)
	if err != nil {
		t.Fatalf("SourceDigestForPath(second): %v", err)
	}
	if binding.Digest != wantDigest {
		t.Errorf("Digest = %q, want digest of coherent replacement %q", binding.Digest, wantDigest)
	}
	if binding.PathIdentity == binding.Identity {
		t.Error("declared symlink identity unexpectedly matches resolved file identity")
	}
}

func TestSourceBinding_RejectsPersistentSymlinkSubstitution(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	first := filepath.Join(repo, "first")
	second := filepath.Join(repo, "second")
	mustWriteFile(t, first, []byte("first"))
	mustWriteFile(t, second, []byte("second"))
	source := filepath.Join(repo, "source")
	if err := os.Symlink(first, source); err != nil {
		t.Fatalf("symlink source: %v", err)
	}

	current := first
	calls := 0
	sourceBindingCaptureHook = func() {
		calls++
		if current == first {
			current = second
		} else {
			current = first
		}
		if err := os.Remove(source); err != nil {
			t.Fatalf("remove source: %v", err)
		}
		if err := os.Symlink(current, source); err != nil {
			t.Fatalf("replace source: %v", err)
		}
	}
	defer func() { sourceBindingCaptureHook = nil }()

	_, err := New(WithDiscoverer(&fakeDiscoverer{targets: []Target{{
		Source: source, Destination: filepath.Join(home, "destination"), Kind: CopyFile,
	}}})).Build(repo, home, Options{})
	var drift *SourceBindingDriftError
	if !errors.As(err, &drift) {
		t.Fatalf("Build() error = %v, want SourceBindingDriftError", err)
	}
	if calls != sourceBindingCaptureAttempts {
		t.Errorf("capture calls = %d, want bounded attempts %d", calls, sourceBindingCaptureAttempts)
	}
}

func TestBuildPlan_DoesNotMutateDestinationParentDuringPrerequisiteCheck(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	src := filepath.Join(repo, "file")
	mustWriteFile(t, src, []byte("x"))
	parent := filepath.Join(home, "parent")
	mustMkdirAll(t, parent)
	disc := &fakeDiscoverer{targets: []Target{{Source: src, Destination: filepath.Join(parent, "file"), Kind: CopyFile}}}

	before, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("Stat(parent): %v", err)
	}

	if _, err := New(WithDiscoverer(disc)).Build(repo, home, Options{}); err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	after, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("Stat(parent) after: %v", err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Errorf("destination parent mtime changed during planning: %v -> %v", before.ModTime(), after.ModTime())
	}
}
