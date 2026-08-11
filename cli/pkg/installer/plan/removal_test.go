package plan

import (
	"path/filepath"
	"reflect"
	"testing"
)

// Task 5.1: MutationKind Remove.

func TestPlanRemove_KindConstant(t *testing.T) {
	if Remove != MutationKind("remove") {
		t.Errorf("Remove = %q, want %q", Remove, MutationKind("remove"))
	}
}

// Task 5.3: the closed plan reconciles the installed+desired union emitted by
// the discoverer into explicit removals, replacements, and creations.

func TestPlanRemove_ClosesUnionOfInstalledAndDesired(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()

	// Installed A is retired (omitted from desired); desired B and C replace/create.
	aDest := filepath.Join(home, ".config", "a")
	mustMkdirAll(t, filepath.Dir(aDest))
	mustWriteFile(t, aDest, []byte("installed a"))

	mustMkdirAll(t, filepath.Join(repo, "home"))
	bSrc := filepath.Join(repo, "home", "b")
	cSrc := filepath.Join(repo, "home", "c")
	mustWriteFile(t, bSrc, []byte("b"))
	mustWriteFile(t, cSrc, []byte("c"))

	disc := &fakeDiscoverer{targets: []Target{
		{Source: "", Destination: aDest, Kind: Remove},
		{Source: bSrc, Destination: filepath.Join(home, "b"), Kind: CopyFile},
		{Source: cSrc, Destination: filepath.Join(home, "c"), Kind: CopyFile},
	}}

	p, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	targets := p.ManagedTargets()
	if len(targets) != 3 {
		t.Fatalf("len(ManagedTargets) = %d, want 3 (Remove A, Replace B, Create C)", len(targets))
	}

	byDest := make(map[string]Target, len(targets))
	for _, tgt := range targets {
		byDest[tgt.Destination] = tgt
	}

	removal, ok := byDest[aDest]
	if !ok {
		t.Fatalf("closed plan missing Remove target for retired destination %q", aDest)
	}
	if removal.Kind != Remove {
		t.Errorf("retired target Kind = %q, want %q", removal.Kind, Remove)
	}
	if removal.PreState.Type != StateFile {
		t.Errorf("retired target PreState.Type = %q, want file", removal.PreState.Type)
	}
	if removal.BackupPath == "" {
		t.Error("retired target missing backup path")
	}

	replacement, ok := byDest[filepath.Join(home, "b")]
	if !ok || replacement.Kind != CopyFile || replacement.SourceDigest == "" {
		t.Errorf("desired B not planned as a bound CopyFile replacement: %#v", replacement)
	}
	creation, ok := byDest[filepath.Join(home, "c")]
	if !ok || creation.Kind != CopyFile || creation.SourceDigest == "" {
		t.Errorf("desired C not planned as a bound CopyFile creation: %#v", creation)
	}
}

// Task 5.1/5.2: Remove targets carry no source and no source binding.

func TestPlanRemove_TargetCarriesNoSourceBinding(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	aDest := filepath.Join(home, ".config", "a")
	mustMkdirAll(t, filepath.Dir(aDest))
	mustWriteFile(t, aDest, []byte("installed a"))

	disc := &fakeDiscoverer{targets: []Target{
		{Source: "", Destination: aDest, Kind: Remove},
	}}
	p, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	tgt := p.ManagedTargets()[0]
	if tgt.Source != "" || tgt.ResolvedSource != "" || tgt.SourceDigest != "" {
		t.Errorf("Remove target carries source data: Source=%q ResolvedSource=%q SourceDigest=%q",
			tgt.Source, tgt.ResolvedSource, tgt.SourceDigest)
	}
	if !reflect.DeepEqual(tgt.SourceBinding, SourceBinding{}) {
		t.Errorf("Remove target carries a source binding: %#v", tgt.SourceBinding)
	}
}

// Task 5.3: a retired target that is already absent is carried in the closed
// plan with an absent pre-state so the transaction can skip it and record
// EntrySkipped(absent) instead of erroring or recreating the path.

func TestPlanRemove_AbsentRetiredDestinationIsCarried(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	aDest := filepath.Join(home, ".config", "a")
	mustMkdirAll(t, filepath.Dir(aDest))

	disc := &fakeDiscoverer{targets: []Target{
		{Source: "", Destination: aDest, Kind: Remove},
	}}
	p, err := New(WithDiscoverer(disc)).Build(repo, home, Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	targets := p.ManagedTargets()
	if len(targets) != 1 {
		t.Fatalf("len(ManagedTargets) = %d, want 1", len(targets))
	}
	tgt := targets[0]
	if tgt.Kind != Remove {
		t.Errorf("Kind = %q, want %q", tgt.Kind, Remove)
	}
	if tgt.PreState.Type != StateAbsent {
		t.Errorf("PreState.Type = %q, want absent so the transaction can skip and record EntrySkipped", tgt.PreState.Type)
	}
}
