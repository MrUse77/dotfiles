package cmd

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/klauspost/compress/zstd"
)

type stubConfigLock struct {
	err          error
	acquisitions int
	releases     int
}

func (s *stubConfigLock) Acquire(string) (func(), error) {
	s.acquisitions++
	if s.err != nil {
		return nil, s.err
	}
	return func() { s.releases++ }, nil
}

type stubConfigResolver struct {
	calls    int
	artifact release.Artifact
	err      error
	events   *[]string
}

type integrationReleaseClient struct {
	releases  map[string]release.Release
	assets    map[string][]byte
	tagCalls  int
	downloads int
}

func (c *integrationReleaseClient) Latest(context.Context) (release.Release, error) {
	return release.Release{}, errors.New("latest release lookup is forbidden for config apply")
}

func (c *integrationReleaseClient) GetByTag(_ context.Context, tag string) (release.Release, error) {
	c.tagCalls++
	configRelease, ok := c.releases[tag]
	if !ok {
		return release.Release{}, fmt.Errorf("release %s not found", tag)
	}
	return configRelease, nil
}

func (c *integrationReleaseClient) Download(_ context.Context, asset release.Asset) (io.ReadCloser, error) {
	c.downloads++
	data, ok := c.assets[asset.URL]
	if !ok {
		return nil, fmt.Errorf("asset %s not found", asset.URL)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *stubConfigResolver) Resolve(context.Context, string) (release.Artifact, error) {
	s.calls++
	if s.events != nil {
		*s.events = append(*s.events, "resolve")
	}
	if s.err != nil {
		return release.Artifact{}, s.err
	}
	return s.artifact, nil
}

type recordingAdmitter struct {
	events *[]string
}

func (a *recordingAdmitter) Admit(string, string) error {
	*a.events = append(*a.events, "admit")
	return nil
}

type recordingCache struct {
	events *[]string
	root   string
}

func (c *recordingCache) Promote(string, string) error {
	*c.events = append(*c.events, "promote")
	return nil
}

func (c *recordingCache) Lookup(string) (string, error) {
	*c.events = append(*c.events, "lookup")
	return c.root, nil
}

func (*recordingCache) Retain(*release.VersionIdentity, *release.VersionIdentity) error {
	return nil
}

type recordingDependencyProbe struct {
	events *[]string
}

type mapStateReader map[string]plan.PreState

func (r mapStateReader) Read(path string) (plan.PreState, error) {
	if state, ok := r[path]; ok {
		return state, nil
	}
	return plan.PreState{Type: plan.StateAbsent}, nil
}

type stubConfigTransaction struct {
	events      *[]string
	inventory   *transaction.Inventory
	prepareErr  error
	commitErr   error
	rollbackErr error
}

func (s *stubConfigTransaction) Prepare() error {
	*s.events = append(*s.events, "transaction:prepare")
	return s.prepareErr
}

func (s *stubConfigTransaction) Commit() error {
	*s.events = append(*s.events, "transaction:commit")
	return s.commitErr
}

func (s *stubConfigTransaction) Rollback() error {
	*s.events = append(*s.events, "transaction:rollback")
	return s.rollbackErr
}

func (s *stubConfigTransaction) Inventory() *transaction.Inventory {
	return s.inventory
}

type stubThemeMutation struct {
	events      *[]string
	commitErr   error
	rollbackErr error
}

func (s *stubThemeMutation) Commit() error {
	*s.events = append(*s.events, "theme:commit")
	return s.commitErr
}

func (s *stubThemeMutation) Rollback() error {
	*s.events = append(*s.events, "theme:rollback")
	return s.rollbackErr
}

func (p *recordingDependencyProbe) Probe(context.Context, string, string) (release.DependencyResult, error) {
	*p.events = append(*p.events, "dependency")
	return release.DependencyResult{Name: "git", Satisfied: true, Observed: "/usr/bin/git"}, nil
}

type stubConfigJournal struct {
	outcome release.JournalOutcome
	records []release.JournalRecord
	err     error
	appends []release.JournalRecord
	events  *[]string
}

func (s *stubConfigJournal) Recovery() (release.JournalOutcome, []release.JournalRecord, error) {
	return s.outcome, append([]release.JournalRecord(nil), s.records...), s.err
}

func (s *stubConfigJournal) Append(record release.JournalRecord) error {
	s.appends = append(s.appends, record)
	if s.events != nil {
		*s.events = append(*s.events, "journal:"+string(record.Phase))
	}
	return nil
}

func TestApply_NeverCallsRunner(t *testing.T) {
	t.Parallel()

	impurePlan, err := plan.NewInstallationPlanWithActions("run-1", nil, []plan.ExternalAction{
		{
			Description: "forbidden config action",
			Command:     plan.CommandSpec{Name: "must-not-run"},
		},
	})
	if err != nil {
		t.Fatalf("build impure plan: %v", err)
	}

	if err := validateConfigPlan(impurePlan); err == nil {
		t.Fatal("config apply accepted a plan containing an external action")
	}
}

func TestConfigPlanAllowsManagedOnlyTransaction(t *testing.T) {
	t.Parallel()

	managedPlan, err := plan.NewInstallationPlan("run-1", []plan.Target{
		{Source: "/artifact/home/.zshrc", Destination: "/home/test/.zshrc", Kind: plan.CopyFile},
	})
	if err != nil {
		t.Fatalf("build managed plan: %v", err)
	}
	if err := validateConfigPlan(managedPlan); err != nil {
		t.Fatalf("validate managed-only config plan: %v", err)
	}
}

func TestConfigApply_LockContentionStopsBeforeResolution(t *testing.T) {
	t.Parallel()

	lock := &stubConfigLock{err: release.ErrLockContended}
	resolver := &stubConfigResolver{err: errors.New("unexpected resolve")}
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:    configPaths{lock: "/state/moonarch/lock"},
		lock:     lock,
		journal:  &stubConfigJournal{outcome: release.JournalOutcomeCommitted},
		resolver: resolver,
	})

	err := operations.Apply(context.Background(), io.Discard, configApplyRequest{Tag: "config-v1.2.3"})
	if !errors.Is(err, release.ErrLockContended) {
		t.Fatalf("apply error = %v, want ErrLockContended", err)
	}
	if lock.acquisitions != 1 || lock.releases != 0 {
		t.Fatalf("lock acquisitions/releases = %d/%d, want 1/0", lock.acquisitions, lock.releases)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
}

func TestConfigApply_ReleasesLockWhenResolutionFails(t *testing.T) {
	t.Parallel()

	lock := &stubConfigLock{}
	resolver := &stubConfigResolver{err: errors.New("resolution failed")}
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:    configPaths{lock: "/state/moonarch/lock"},
		lock:     lock,
		journal:  &stubConfigJournal{outcome: release.JournalOutcomeCommitted},
		resolver: resolver,
	})

	err := operations.Apply(context.Background(), io.Discard, configApplyRequest{Tag: "config-v1.2.3"})
	if err == nil {
		t.Fatal("expected resolution failure")
	}
	if lock.acquisitions != 1 || lock.releases != 1 {
		t.Fatalf("lock acquisitions/releases = %d/%d, want 1/1", lock.acquisitions, lock.releases)
	}
	if resolver.calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", resolver.calls)
	}
}

func TestConfigApply_IndeterminateJournalBlocksResolution(t *testing.T) {
	t.Parallel()

	lock := &stubConfigLock{}
	journal := &stubConfigJournal{
		outcome: release.JournalOutcomeIndeterminate,
		err:     release.ErrIndeterminateJournal,
	}
	resolver := &stubConfigResolver{err: errors.New("must not resolve")}
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:    configPaths{lock: "/state/moonarch/lock"},
		lock:     lock,
		journal:  journal,
		resolver: resolver,
	})

	err := operations.Apply(context.Background(), io.Discard, configApplyRequest{Tag: "config-v1.2.3"})
	if !errors.Is(err, release.ErrIndeterminateJournal) {
		t.Fatalf("apply error = %v, want ErrIndeterminateJournal", err)
	}
	if resolver.calls != 0 {
		t.Fatalf("resolver calls = %d, want 0", resolver.calls)
	}
	if lock.releases != 1 {
		t.Fatalf("lock releases = %d, want 1", lock.releases)
	}
}

func TestConfigApply_AcquiresVerifiedArtifactInOrder(t *testing.T) {
	t.Parallel()

	events := []string{}
	digest := strings.Repeat("a", 64)
	dataRoot := t.TempDir()
	artifactRoot := filepath.Join(dataRoot, "artifacts", digest)
	manifest := release.Manifest{
		SchemaVersion:   release.SupportedManifestSchema,
		DependencyDecls: []release.DependencyDecl{{Name: "git"}},
		Catalog:         []release.CatalogEntry{},
	}

	got, err := acquireConfigArtifact(context.Background(), "config-v1.2.3", configAcquisitionDependencies{
		dataRoot: dataRoot,
		resolver: &stubConfigResolver{
			artifact: release.Artifact{Tag: "config-v1.2.3", Digest: digest, ArchivePath: "/staging/config.tar.zst"},
			events:   &events,
		},
		admitter: &recordingAdmitter{events: &events},
		cache:    &recordingCache{events: &events, root: artifactRoot},
		readManifest: func(path string) (release.Manifest, error) {
			events = append(events, "manifest")
			want := filepath.Join(dataRoot, "staging", digest, release.ManifestFilename)
			if path != want {
				t.Fatalf("manifest path = %q, want %q", path, want)
			}
			return manifest, nil
		},
		checkCompatibility: func(got release.Manifest, version string) error {
			events = append(events, "compatibility")
			if !reflect.DeepEqual(got, manifest) || version != "v1.0.0" {
				t.Fatalf("compatibility input = %#v, %q", got, version)
			}
			return nil
		},
		dependencyProbe: &recordingDependencyProbe{events: &events},
		cliVersion:      "v1.0.0",
	})
	if err != nil {
		t.Fatalf("acquire artifact: %v", err)
	}
	if got.Identity != (release.Identity{Tag: "config-v1.2.3", Digest: digest}) || got.Root != artifactRoot {
		t.Fatalf("acquired artifact = %#v", got)
	}
	wantEvents := []string{"resolve", "admit", "manifest", "compatibility", "dependency", "promote", "lookup"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
}

func TestConfigApply_CompatibilityFailureDoesNotPromoteArtifact(t *testing.T) {
	t.Parallel()

	events := []string{}
	digest := strings.Repeat("b", 64)
	compatibilityErr := errors.New("incompatible CLI")
	_, err := acquireConfigArtifact(context.Background(), "config-v2.0.0", configAcquisitionDependencies{
		dataRoot: t.TempDir(),
		resolver: &stubConfigResolver{
			artifact: release.Artifact{Tag: "config-v2.0.0", Digest: digest, ArchivePath: "/staging/config.tar.zst"},
			events:   &events,
		},
		admitter: &recordingAdmitter{events: &events},
		cache:    &recordingCache{events: &events},
		readManifest: func(string) (release.Manifest, error) {
			events = append(events, "manifest")
			return release.Manifest{SchemaVersion: "1", Catalog: []release.CatalogEntry{}}, nil
		},
		checkCompatibility: func(release.Manifest, string) error {
			events = append(events, "compatibility")
			return compatibilityErr
		},
	})
	if !errors.Is(err, compatibilityErr) {
		t.Fatalf("acquire error = %v, want compatibility failure", err)
	}
	wantEvents := []string{"resolve", "admit", "manifest", "compatibility"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
}

func TestConfigApply_PlannerUnionsDesiredAndRetiredAndExcludesThemeCurrent(t *testing.T) {
	t.Parallel()

	artifactRoot := t.TempDir()
	home := t.TempDir()
	writeTestFile(t, filepath.Join(artifactRoot, "home", ".zshrc"), "new zsh config")
	writeTestFile(t, filepath.Join(artifactRoot, "home", ".local", "share", "moonarch", "themes", "tokyo-night", "theme.conf"), "theme")
	if err := os.Symlink("tokyo-night", filepath.Join(artifactRoot, "home", ".local", "share", "moonarch", "themes", "current")); err != nil {
		t.Fatalf("create artifact current theme link: %v", err)
	}

	themeCurrent := filepath.Join(home, ".local", "share", "moonarch", "themes", "current")
	baseline := configBaseline{
		filepath.Join(home, ".zshrc"):   {Type: plan.StateFile, Mode: 0o644, Digest: "old-zsh"},
		filepath.Join(home, ".retired"): {Type: plan.StateFile, Mode: 0o644, Digest: "old-retired"},
		themeCurrent:                    {Type: plan.StateSymlink, Mode: 0o777, LinkValue: "old-theme", Digest: "old-theme-digest"},
	}
	manifest := release.Manifest{SchemaVersion: "1", Catalog: []release.CatalogEntry{
		{Path: "home/.zshrc", Digest: "zsh", Kind: "file"},
		{Path: "home/.local/share/moonarch/themes/tokyo-night", Digest: "theme-dir", Kind: "dir"},
		{Path: "home/.local/share/moonarch/themes/tokyo-night/theme.conf", Digest: "theme", Kind: "file"},
		{Path: "home/.local/share/moonarch/themes/current", Digest: "current", Kind: "symlink"},
	}}

	configPlan, bundles, err := buildConfigPlan(artifactRoot, home, manifest, baseline)
	if err != nil {
		t.Fatalf("build config plan: %v", err)
	}
	if !reflect.DeepEqual(bundles, []string{"tokyo-night"}) {
		t.Fatalf("theme bundles = %v, want [tokyo-night]", bundles)
	}

	kinds := map[string]plan.MutationKind{}
	for _, target := range configPlan.ManagedTargets() {
		kinds[target.Destination] = target.Kind
	}
	want := map[string]plan.MutationKind{
		filepath.Join(home, ".zshrc"):   plan.CopyFile,
		filepath.Join(home, ".retired"): plan.Remove,
		filepath.Join(home, ".local", "share", "moonarch", "themes", "tokyo-night"): plan.CopyTree,
	}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("planned targets = %v, want %v", kinds, want)
	}
	if _, exists := kinds[themeCurrent]; exists {
		t.Fatal("themes/current must remain outside the config plan")
	}
	if len(configPlan.ExternalActions()) != 0 {
		t.Fatalf("external actions = %d, want 0", len(configPlan.ExternalActions()))
	}
}

func TestConfigApply_LegacyWithoutBaselinePreservesUnknownPaths(t *testing.T) {
	t.Parallel()

	artifactRoot := t.TempDir()
	home := t.TempDir()
	writeTestFile(t, filepath.Join(artifactRoot, "home", ".zshrc"), "desired")
	writeTestFile(t, filepath.Join(home, ".zshrc"), "locally managed or unknown")
	writeTestFile(t, filepath.Join(home, ".unknown-local-file"), "preserve me")
	manifest := release.Manifest{SchemaVersion: "1", Catalog: []release.CatalogEntry{
		{Path: "home/.zshrc", Digest: "zsh", Kind: "file"},
	}}

	configPlan, _, err := buildConfigPlan(artifactRoot, home, manifest, nil)
	if err != nil {
		t.Fatalf("build first-apply plan: %v", err)
	}
	targets := configPlan.ManagedTargets()
	if len(targets) != 1 {
		t.Fatalf("managed targets = %#v, want only desired .zshrc", targets)
	}
	if targets[0].Destination != filepath.Join(home, ".zshrc") || targets[0].PreState.Type != plan.StateAbsent {
		t.Fatalf("desired target = %#v, want untrusted creation pre-state", targets[0])
	}
	if targets[0].Kind == plan.Remove || targets[0].Destination == filepath.Join(home, ".unknown-local-file") {
		t.Fatal("unknown non-desired path was inferred as managed removal")
	}
}

func TestConfigApply_CompletedSchemaOneInventoryEstablishesBaseline(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	managed := filepath.Join(home, ".zshrc")
	retired := filepath.Join(home, ".retired")
	inv := &transaction.Inventory{
		FormatVersion: 1,
		Lifecycle:     transaction.InventoryCompleted,
		Entries: []transaction.InventoryEntry{
			{
				Target:          plan.Target{Destination: managed, Kind: plan.CopyFile},
				State:           transaction.EntryMutated,
				InstalledDigest: "installed-zsh",
				InstalledMode:   0o600,
			},
			{
				Target: plan.Target{Destination: retired, Kind: plan.Remove},
				State:  transaction.EntryRemoved,
			},
		},
	}

	baseline, err := baselineFromInventory(inv)
	if err != nil {
		t.Fatalf("build inventory baseline: %v", err)
	}
	want := configBaseline{managed: {Type: plan.StateFile, Mode: 0o600, Digest: "installed-zsh"}}
	if !reflect.DeepEqual(baseline, want) {
		t.Fatalf("baseline = %#v, want %#v", baseline, want)
	}
}

func TestConfigApply_IncompleteInventoryCannotEstablishBaseline(t *testing.T) {
	t.Parallel()

	inv := &transaction.Inventory{FormatVersion: 1, Lifecycle: transaction.InventoryCommitting}
	if _, err := baselineFromInventory(inv); err == nil {
		t.Fatal("expected incomplete inventory baseline to fail closed")
	}
}

func TestConfigApply_DriftAuthorizationBindsExactCandidateAndObservations(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	replacement := filepath.Join(home, ".zshrc")
	removal := filepath.Join(home, ".retired")
	configPlan, err := plan.NewInstallationPlan("run-1", []plan.Target{
		{
			Source:      "/artifact/home/.zshrc",
			Destination: replacement,
			Kind:        plan.CopyFile,
			PreState:    plan.PreState{Type: plan.StateFile, Mode: 0o644, Digest: "expected-zsh"},
		},
		{
			Destination: removal,
			Kind:        plan.Remove,
			PreState:    plan.PreState{Type: plan.StateFile, Mode: 0o644, Digest: "expected-retired"},
		},
	})
	if err != nil {
		t.Fatalf("build preflight plan: %v", err)
	}
	reader := mapStateReader{
		replacement: {Type: plan.StateFile, Mode: 0o644, Digest: "local-zsh"},
		removal:     {Type: plan.StateFile, Mode: 0o644, Digest: "local-retired"},
	}
	candidate := release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("c", 64)}

	first, err := checkConfigPreflight(configPlan, reader, candidate, "")
	if !errors.Is(err, errDriftAuthorizationRequired) {
		t.Fatalf("first preflight error = %v, want drift authorization required", err)
	}
	if len(first.Observations) != 2 || first.Token == "" {
		t.Fatalf("first preflight = %#v, want two observations and token", first)
	}
	if first.Observations[0].Path != removal || first.Observations[1].Path != replacement {
		t.Fatalf("observation paths = %#v, want sorted complete set", first.Observations)
	}

	second, err := checkConfigPreflight(configPlan, reader, candidate, first.Token)
	if err != nil {
		t.Fatalf("matching evidence token rejected: %v", err)
	}
	if !reflect.DeepEqual(second.Observations, first.Observations) {
		t.Fatalf("fresh observations = %#v, want %#v", second.Observations, first.Observations)
	}

	reader[removal] = plan.PreState{Type: plan.StateAbsent}
	changed, err := checkConfigPreflight(configPlan, reader, candidate, first.Token)
	if err == nil || errors.Is(err, errDriftAuthorizationRequired) {
		t.Fatalf("changed evidence error = %v, want token mismatch", err)
	}
	if len(changed.Observations) != 2 || changed.Observations[0].ObservedIdentity == first.Observations[0].ObservedIdentity {
		t.Fatalf("changed observations = %#v, want fresh changed removal", changed.Observations)
	}
}

func TestConfigApply_PrintsEveryDriftObservationAndToken(t *testing.T) {
	t.Parallel()

	result := configPreflightResult{
		Observations: []release.EvidenceObservation{
			{Path: "/home/test/.retired", ObservedIdentity: "file:local-retired", DriftClass: "removal"},
			{Path: "/home/test/.zshrc", ObservedIdentity: "file:local-zsh", DriftClass: "replacement"},
		},
		Token: "bound-token",
	}
	var out bytes.Buffer
	printDriftEvidence(&out, result)
	printed := out.String()
	for _, want := range []string{"/home/test/.retired", "file:local-retired", "/home/test/.zshrc", "file:local-zsh", "bound-token"} {
		if !strings.Contains(printed, want) {
			t.Fatalf("drift output %q does not contain %q", printed, want)
		}
	}
}

func TestConfigApply_UnboundAuthorizationCannotBypassCleanPreflight(t *testing.T) {
	t.Parallel()

	destination := filepath.Join(t.TempDir(), ".zshrc")
	expected := plan.PreState{Type: plan.StateFile, Mode: 0o644, Digest: "clean"}
	configPlan, err := plan.NewInstallationPlan("run-1", []plan.Target{{
		Destination: destination,
		Kind:        plan.CopyFile,
		PreState:    expected,
	}})
	if err != nil {
		t.Fatalf("build preflight plan: %v", err)
	}
	_, err = checkConfigPreflight(
		configPlan,
		mapStateReader{destination: expected},
		release.Identity{Tag: "config-v1.0.0", Digest: strings.Repeat("d", 64)},
		"broad-force-value",
	)
	if !errors.Is(err, release.ErrUnboundForce) {
		t.Fatalf("preflight error = %v, want ErrUnboundForce", err)
	}
}

func TestConfigApply_CommitOrdersJournalStateAndThemeConservatively(t *testing.T) {
	t.Parallel()

	events := []string{}
	candidate := release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("e", 64)}
	prior := &release.State{Current: &release.Identity{Tag: "config-v1.0.0", Digest: strings.Repeat("d", 64)}}
	inventory := &transaction.Inventory{
		RunID: "run-2",
		Entries: []transaction.InventoryEntry{{
			Target: plan.Target{Destination: "/home/test/.zshrc"},
			State:  transaction.EntryMutated,
		}},
	}
	journal := &stubConfigJournal{events: &events}
	tx := &stubConfigTransaction{events: &events, inventory: inventory}
	theme := &stubThemeMutation{events: &events}
	now := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)

	got, err := executeConfigMutation(candidate, prior, "run-2", configMutationDependencies{
		journal:     journal,
		transaction: tx,
		theme:       theme,
		preflight: func() error {
			events = append(events, "preflight")
			return nil
		},
		writeState: func(state *release.State) error {
			events = append(events, "state:write")
			if state.Current == nil || *state.Current != candidate {
				t.Fatalf("state current = %#v, want %#v", state.Current, candidate)
			}
			return nil
		},
		now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("execute config mutation: %v", err)
	}
	if got.Current == nil || *got.Current != candidate || got.Previous == nil || *got.Previous != *prior.Current || got.LastCompletedRunID != "run-2" {
		t.Fatalf("next state = %#v", got)
	}
	if inventory.ReleaseProvenance == nil || inventory.ReleaseProvenance.Tag != candidate.Tag || inventory.ReleaseProvenance.Digest != candidate.Digest {
		t.Fatalf("inventory provenance = %#v, want candidate", inventory.ReleaseProvenance)
	}
	wantEvents := []string{
		"journal:op-start",
		"preflight",
		"transaction:prepare",
		"journal:prepared",
		"journal:committing",
		"transaction:commit",
		"journal:mutated",
		"theme:commit",
		"journal:committed",
		"state:write",
		"journal:state-finalized",
		"journal:op-end",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
}

func TestConfigApply_PreflightFailureMakesNoChanges(t *testing.T) {
	t.Parallel()

	events := []string{}
	preflightErr := errors.New("drift detected")
	writes := 0
	_, err := executeConfigMutation(
		release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("f", 64)},
		&release.State{Current: &release.Identity{Tag: "config-v1.0.0", Digest: strings.Repeat("e", 64)}},
		"run-preflight",
		configMutationDependencies{
			journal:     &stubConfigJournal{events: &events},
			transaction: &stubConfigTransaction{events: &events, inventory: &transaction.Inventory{}},
			theme:       &stubThemeMutation{events: &events},
			preflight: func() error {
				events = append(events, "preflight")
				return preflightErr
			},
			writeState: func(*release.State) error {
				writes++
				return nil
			},
		},
	)
	if !errors.Is(err, preflightErr) {
		t.Fatalf("mutation error = %v, want preflight error", err)
	}
	if !reflect.DeepEqual(events, []string{"journal:op-start", "preflight"}) {
		t.Fatalf("events = %v, want journal start then preflight only", events)
	}
	if writes != 0 {
		t.Fatalf("state writes = %d, want 0", writes)
	}
}

func TestConfigApply_TransactionFailureRollsBackBeforeIdentityCommit(t *testing.T) {
	t.Parallel()

	events := []string{}
	commitErr := errors.New("commit failed")
	writes := 0
	priorIdentity := release.Identity{Tag: "config-v1.0.0", Digest: strings.Repeat("a", 64)}
	prior := &release.State{Current: &priorIdentity}
	_, err := executeConfigMutation(
		release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("b", 64)},
		prior,
		"run-failure",
		configMutationDependencies{
			journal: &stubConfigJournal{events: &events},
			transaction: &stubConfigTransaction{
				events:    &events,
				inventory: &transaction.Inventory{},
				commitErr: commitErr,
			},
			theme:     &stubThemeMutation{events: &events},
			preflight: func() error { events = append(events, "preflight"); return nil },
			writeState: func(*release.State) error {
				writes++
				return nil
			},
		},
	)
	if !errors.Is(err, commitErr) {
		t.Fatalf("mutation error = %v, want commit failure", err)
	}
	wantEvents := []string{
		"journal:op-start", "preflight", "transaction:prepare", "journal:prepared",
		"journal:committing", "transaction:commit", "theme:rollback", "transaction:rollback",
	}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("events = %v, want %v", events, wantEvents)
	}
	if writes != 0 || prior.Current == nil || *prior.Current != priorIdentity {
		t.Fatalf("writes/prior = %d/%#v, want unchanged identity", writes, prior)
	}
}

func TestConfigApply_DriftedRemovalBlocked(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	removal := filepath.Join(home, ".retired")
	configPlan, err := plan.NewInstallationPlan("run-drift", []plan.Target{{
		Destination: removal,
		Kind:        plan.Remove,
		PreState:    plan.PreState{Type: plan.StateFile, Mode: 0o644, Digest: "expected"},
	}})
	if err != nil {
		t.Fatalf("build drift plan: %v", err)
	}
	events := []string{}
	stateWrites := 0
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:   configPaths{home: home, lock: "/state/lock", state: "/state/state.json", themeCurrent: filepath.Join(home, "themes", "current")},
		lock:    &stubConfigLock{},
		journal: &stubConfigJournal{outcome: release.JournalOutcomeCommitted, events: &events},
		acquireArtifact: func(context.Context, string) (acquiredConfigArtifact, error) {
			return acquiredConfigArtifact{
				Identity: release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("f", 64)},
				Root:     "/artifact",
				Manifest: release.Manifest{SchemaVersion: "1", Catalog: []release.CatalogEntry{}},
			}, nil
		},
		readState: func(string) (*release.State, error) {
			return &release.State{Current: &release.Identity{Tag: "config-v1.0.0", Digest: strings.Repeat("e", 64)}}, nil
		},
		loadBaseline: func(*release.State) (configBaseline, error) { return nil, nil },
		buildPlan: func(string, string, release.Manifest, configBaseline) (plan.InstallationPlan, []string, error) {
			return configPlan, []string{"tokyo-night"}, nil
		},
		prepareTheme: func(string, []string, string) (configThemeMutation, error) {
			return &stubThemeMutation{events: &events}, nil
		},
		newTransaction: func(plan.InstallationPlan) configManagedTransaction {
			events = append(events, "transaction:new")
			return &stubConfigTransaction{events: &events, inventory: &transaction.Inventory{}}
		},
		stateReader: mapStateReader{removal: {Type: plan.StateFile, Mode: 0o644, Digest: "locally-modified"}},
		writeState: func(*release.State) error {
			stateWrites++
			return nil
		},
	})

	var out bytes.Buffer
	err = operations.Apply(context.Background(), &out, configApplyRequest{Tag: "config-v2.0.0"})
	if !errors.Is(err, errDriftAuthorizationRequired) {
		t.Fatalf("apply error = %v, want drift authorization required", err)
	}
	if !strings.Contains(out.String(), removal) || !strings.Contains(out.String(), "--authorize-drift") {
		t.Fatalf("drift output = %q, want removal and authorization token", out.String())
	}
	if stateWrites != 0 {
		t.Fatalf("state writes = %d, want 0", stateWrites)
	}
	if strings.Contains(strings.Join(events, ","), "transaction:prepare") {
		t.Fatalf("events = %v, transaction must not prepare after drift", events)
	}
}

func TestConfigApply_RecoveryRollsBackUncommittedInventory(t *testing.T) {
	t.Parallel()

	events := []string{}
	journal := &stubConfigJournal{
		outcome: release.JournalOutcomeUncommitted,
		records: []release.JournalRecord{
			{OpID: "run-crash", Phase: release.JournalOpStart, Tag: "config-v2.0.0", Digest: strings.Repeat("f", 64)},
			{OpID: "run-crash", Phase: release.JournalPrepared},
			{OpID: "run-crash", Phase: release.JournalCommitting},
			{OpID: "run-crash", Phase: release.JournalMutated, Payload: "/home/test/.zshrc"},
		},
		events: &events,
	}
	inventory := &transaction.Inventory{RunID: "run-crash", Lifecycle: transaction.InventoryCommitting}
	recovered := 0
	err := recoverConfigJournal(configRecoveryDependencies{
		journal: journal,
		loadInventory: func(opID string) (*transaction.Inventory, error) {
			if opID != "run-crash" {
				t.Fatalf("load inventory op = %q, want run-crash", opID)
			}
			return inventory, nil
		},
		recoverInventory: func(got *transaction.Inventory) error {
			events = append(events, "inventory:rollback")
			if got != inventory {
				t.Fatal("recovery received a different inventory")
			}
			recovered++
			return nil
		},
		writeState: func(*release.State) error {
			t.Fatal("uncommitted recovery must not rotate identity")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("recover journal: %v", err)
	}
	if recovered != 1 {
		t.Fatalf("inventory recoveries = %d, want 1", recovered)
	}
	wantEvents := []string{"inventory:rollback", "journal:committed", "journal:state-finalized", "journal:op-end"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("recovery events = %v, want %v", events, wantEvents)
	}
}

func TestConfigApply_RecoveryFinalizesCommittedIdentityWithoutReapply(t *testing.T) {
	t.Parallel()

	events := []string{}
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	journal := &stubConfigJournal{
		outcome: release.JournalOutcomeCommitted,
		records: []release.JournalRecord{
			{OpID: "run-committed", Phase: release.JournalOpStart, Tag: "config-v2.0.0", Digest: digestB},
			{OpID: "run-committed", Phase: release.JournalPrepared},
			{OpID: "run-committed", Phase: release.JournalCommitting},
			{OpID: "run-committed", Phase: release.JournalCommitted},
		},
		events: &events,
	}
	var written *release.State
	err := recoverConfigJournal(configRecoveryDependencies{
		journal: journal,
		readState: func(string) (*release.State, error) {
			return &release.State{Current: &release.Identity{Tag: "config-v1.0.0", Digest: digestA}}, nil
		},
		writeState: func(state *release.State) error {
			events = append(events, "state:write")
			written = state
			return nil
		},
		recoverInventory: func(*transaction.Inventory) error {
			t.Fatal("committed recovery must not reapply or roll back inventory")
			return nil
		},
	})
	if err != nil {
		t.Fatalf("recover committed journal: %v", err)
	}
	if written == nil || written.Current == nil || written.Current.Tag != "config-v2.0.0" || written.Previous == nil || written.Previous.Tag != "config-v1.0.0" || written.LastCompletedRunID != "run-committed" {
		t.Fatalf("recovered state = %#v", written)
	}
	wantEvents := []string{"state:write", "journal:state-finalized", "journal:op-end"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("recovery events = %v, want %v", events, wantEvents)
	}
}

func TestConfigApply_RecoveryRejectsCandidateStateWithMismatchedRun(t *testing.T) {
	t.Parallel()

	candidate := release.Identity{Tag: "config-v2.0.0", Digest: strings.Repeat("b", 64)}
	journal := &stubConfigJournal{
		outcome: release.JournalOutcomeCommitted,
		records: []release.JournalRecord{
			{OpID: "run-committed", Phase: release.JournalOpStart, Tag: candidate.Tag, Digest: candidate.Digest},
			{OpID: "run-committed", Phase: release.JournalPrepared},
			{OpID: "run-committed", Phase: release.JournalCommitting},
			{OpID: "run-committed", Phase: release.JournalCommitted},
		},
	}
	writes := 0
	err := recoverConfigJournal(configRecoveryDependencies{
		journal: journal,
		readState: func(string) (*release.State, error) {
			return &release.State{Current: &candidate, LastCompletedRunID: "different-run"}, nil
		},
		writeState: func(*release.State) error {
			writes++
			return nil
		},
	})
	if !errors.Is(err, release.ErrIndeterminateJournal) {
		t.Fatalf("recovery error = %v, want ErrIndeterminateJournal", err)
	}
	if writes != 0 || len(journal.appends) != 0 {
		t.Fatalf("mismatched recovery mutated state: writes=%d appends=%d", writes, len(journal.appends))
	}
}

func TestConfigApply_EndToEnd(t *testing.T) {
	t.Parallel()

	home := t.TempDir()
	dataRoot := filepath.Join(t.TempDir(), "moonarch")
	stateRoot := filepath.Join(t.TempDir(), "moonarch")
	themesRoot := filepath.Join(home, ".local", "share", "moonarch", "themes")
	if err := os.MkdirAll(themesRoot, 0o755); err != nil {
		t.Fatalf("create themes root: %v", err)
	}
	if err := os.Symlink("tokyo-night", filepath.Join(themesRoot, "current")); err != nil {
		t.Fatalf("create current theme: %v", err)
	}

	client := &integrationReleaseClient{releases: map[string]release.Release{}, assets: map[string][]byte{}}
	identities := map[string]release.Identity{}
	for _, fixture := range []struct {
		tag, zsh, theme string
	}{
		{tag: "config-v1.0.0", zsh: "release-a", theme: "theme-a"},
		{tag: "config-v2.0.0", zsh: "release-b", theme: "theme-b"},
	} {
		archive, digest := buildConfigArchive(t, fixture.zsh, fixture.theme)
		archiveName := fixture.tag + ".tar.zst"
		sidecarName := archiveName + ".sha256"
		archiveURL := "memory://" + fixture.tag + "/archive"
		sidecarURL := "memory://" + fixture.tag + "/sidecar"
		client.releases[fixture.tag] = release.Release{Tag: fixture.tag, Assets: []release.Asset{
			{Name: archiveName, URL: archiveURL},
			{Name: sidecarName, URL: sidecarURL},
		}}
		client.assets[archiveURL] = archive
		client.assets[sidecarURL] = []byte(fmt.Sprintf("%s  %s\n", digest, archiveName))
		identities[fixture.tag] = release.Identity{Tag: fixture.tag, Digest: digest}
	}

	paths := configPaths{
		home:         home,
		dataRoot:     dataRoot,
		stateRoot:    stateRoot,
		lock:         filepath.Join(stateRoot, "lock"),
		journal:      filepath.Join(stateRoot, "journal.ndjson"),
		state:        filepath.Join(stateRoot, "state.json"),
		backupRoot:   filepath.Join(home, ".dots-backups"),
		artifacts:    filepath.Join(dataRoot, "artifacts"),
		themeCurrent: filepath.Join(themesRoot, "current"),
	}
	cache := release.NewArtifactCache(dataRoot)
	operations := newConfigRuntime(configRuntimeDependencies{
		paths:           paths,
		lock:            &release.Lock{},
		journal:         release.NewJournal(paths.journal),
		resolver:        release.NewArtifactResolver(client, release.OSFileOps{}, dataRoot),
		admitter:        release.NewArtifactAdmitter(dataRoot),
		cache:           cache,
		dependencyProbe: release.NewOSDependencyProbe(),
		cliVersion:      "v1.0.0",
	})

	for _, tag := range []string{"config-v1.0.0", "config-v2.0.0"} {
		var out bytes.Buffer
		if err := operations.Apply(context.Background(), &out, configApplyRequest{Tag: tag}); err != nil {
			t.Fatalf("apply %s: %v\noutput: %s", tag, err, out.String())
		}
	}
	state, err := release.ReadState(paths.state)
	if err != nil {
		t.Fatalf("read state after applies: %v", err)
	}
	if state.Current == nil || *state.Current != identities["config-v2.0.0"] || state.Previous == nil || *state.Previous != identities["config-v1.0.0"] {
		t.Fatalf("state after applies = %#v", state)
	}
	networkCalls := client.tagCalls + client.downloads
	cachedPrevious := filepath.Join(paths.artifacts, identities["config-v1.0.0"].Digest, "home", ".zshrc")
	if err := os.WriteFile(cachedPrevious, []byte("tampered-cache"), 0o644); err != nil {
		t.Fatalf("tamper retained artifact: %v", err)
	}
	if err := operations.Rollback(context.Background(), io.Discard, configRollbackRequest{Offline: true}); !errors.Is(err, release.ErrArtifactRejected) {
		t.Fatalf("tampered offline rollback error = %v, want ErrArtifactRejected", err)
	}
	if client.tagCalls+client.downloads != networkCalls {
		t.Fatalf("rejected rollback made network calls: before=%d after=%d", networkCalls, client.tagCalls+client.downloads)
	}
	state, err = release.ReadState(paths.state)
	if err != nil {
		t.Fatalf("read state after rejected rollback: %v", err)
	}
	if state.Current == nil || *state.Current != identities["config-v2.0.0"] {
		t.Fatalf("rejected rollback changed state: %#v", state)
	}
	installed, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("read config after rejected rollback: %v", err)
	}
	if string(installed) != "release-b" {
		t.Fatalf("rejected rollback changed config to %q", installed)
	}
	if err := os.WriteFile(cachedPrevious, []byte("release-a"), 0o644); err != nil {
		t.Fatalf("restore retained artifact fixture: %v", err)
	}

	var rollbackOut bytes.Buffer
	if err := operations.Rollback(context.Background(), &rollbackOut, configRollbackRequest{Offline: true}); err != nil {
		t.Fatalf("offline rollback: %v\noutput: %s", err, rollbackOut.String())
	}
	if client.tagCalls+client.downloads != networkCalls {
		t.Fatalf("rollback made network calls: before=%d after=%d", networkCalls, client.tagCalls+client.downloads)
	}
	state, err = release.ReadState(paths.state)
	if err != nil {
		t.Fatalf("read state after rollback: %v", err)
	}
	if state.Current == nil || *state.Current != identities["config-v1.0.0"] || state.Previous == nil || *state.Previous != identities["config-v2.0.0"] {
		t.Fatalf("state after rollback = %#v", state)
	}
	zsh, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf("read rolled-back zsh config: %v", err)
	}
	if string(zsh) != "release-a" {
		t.Fatalf("rolled-back zsh config = %q, want release-a", zsh)
	}
	assertThemeLink(t, paths.themeCurrent, "tokyo-night")
}

func buildConfigArchive(t *testing.T, zshContent, themeContent string) ([]byte, string) {
	t.Helper()
	entries := []struct {
		path string
		data []byte
	}{
		{path: "home/.zshrc", data: []byte(zshContent)},
		{path: "home/.local/share/moonarch/themes/tokyo-night/theme.conf", data: []byte(themeContent)},
	}
	manifest := release.Manifest{SchemaVersion: release.SupportedManifestSchema, Catalog: make([]release.CatalogEntry, 0, len(entries))}
	for _, entry := range entries {
		digest := sha256.Sum256(entry.data)
		manifest.Catalog = append(manifest.Catalog, release.CatalogEntry{
			Path:   entry.path,
			Digest: fmt.Sprintf("%x", digest[:]),
			Mode:   0o644,
			Kind:   "file",
		})
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}

	var archive bytes.Buffer
	zstdWriter, err := zstd.NewWriter(&archive)
	if err != nil {
		t.Fatalf("create zstd writer: %v", err)
	}
	tarWriter := tar.NewWriter(zstdWriter)
	writeTarEntry := func(name string, data []byte) {
		t.Helper()
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("write tar header %s: %v", name, err)
		}
		if _, err := tarWriter.Write(data); err != nil {
			t.Fatalf("write tar entry %s: %v", name, err)
		}
	}
	writeTarEntry(release.ManifestFilename, manifestData)
	for _, entry := range entries {
		writeTarEntry(entry.path, entry.data)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := zstdWriter.Close(); err != nil {
		t.Fatalf("close zstd writer: %v", err)
	}
	digest := sha256.Sum256(archive.Bytes())
	return archive.Bytes(), fmt.Sprintf("%x", digest[:])
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
