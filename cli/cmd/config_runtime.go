package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/MrUse77/dots-cli/pkg/release"
)

type configPaths struct {
	home         string
	dataRoot     string
	stateRoot    string
	lock         string
	journal      string
	state        string
	backupRoot   string
	artifacts    string
	themeCurrent string
}

type configLock interface {
	Acquire(path string) (release func(), err error)
}

type configJournal interface {
	Recovery() (release.JournalOutcome, []release.JournalRecord, error)
	Append(release.JournalRecord) error
}

type configRuntimeDependencies struct {
	initErr            error
	paths              configPaths
	lock               configLock
	journal            configJournal
	resolver           release.Resolver
	admitter           release.Admitter
	cache              release.Cache
	readManifest       func(string) (release.Manifest, error)
	checkCompatibility func(release.Manifest, string) error
	dependencyProbe    release.DependencyProbe
	cliVersion         string
	acquireArtifact    func(context.Context, string) (acquiredConfigArtifact, error)
	readState          func(string) (*release.State, error)
	loadBaseline       func(*release.State) (configBaseline, error)
	buildPlan          func(string, string, release.Manifest, configBaseline, plan.StateReader) (plan.InstallationPlan, []string, error)
	prepareTheme       func(string, []string, string) (configThemeMutation, error)
	newTransaction     func(plan.InstallationPlan) configManagedTransaction
	stateReader        plan.StateReader
	writeState         func(*release.State) error
	recoverJournal     func() error
	now                func() time.Time
}

type configRuntime struct {
	deps configRuntimeDependencies
}

type configAcquisitionDependencies struct {
	dataRoot           string
	resolver           release.Resolver
	admitter           release.Admitter
	cache              release.Cache
	readManifest       func(string) (release.Manifest, error)
	checkCompatibility func(release.Manifest, string) error
	dependencyProbe    release.DependencyProbe
	cliVersion         string
}

type acquiredConfigArtifact struct {
	Identity release.Identity
	Root     string
	Manifest release.Manifest
}

type configBaseline map[string]plan.PreState

var errDriftAuthorizationRequired = errors.New("configuration drift requires evidence-bound authorization")

type configPreflightResult struct {
	Observations []release.EvidenceObservation
	Token        string
}

type configStatusReport struct {
	Current        string `json:"current"`
	CurrentDigest  string `json:"current_digest,omitempty"`
	Previous       string `json:"previous,omitempty"`
	PreviousDigest string `json:"previous_digest,omitempty"`
	RetentionCount int    `json:"retention_count"`
	OrphanCount    int    `json:"orphan_count"`
	Journal        string `json:"journal"`
}

type configDiscoverer struct {
	targets []plan.Target
}

type configManagedTransaction interface {
	Prepare() error
	Commit() error
	Rollback() error
	Inventory() *transaction.Inventory
}

type configThemeMutation interface {
	Commit() error
	Rollback() error
}

type configMutationDependencies struct {
	journal     configJournal
	transaction configManagedTransaction
	theme       configThemeMutation
	preflight   func() error
	writeState  func(*release.State) error
	now         func() time.Time
}

type configRecoveryDependencies struct {
	journal          configJournal
	statePath        string
	loadInventory    func(string) (*transaction.Inventory, error)
	recoverInventory func(*transaction.Inventory) error
	readState        func(string) (*release.State, error)
	writeState       func(*release.State) error
	now              func() time.Time
}

func (d configDiscoverer) Discover(string, string, plan.Options) ([]plan.Target, error) {
	return append([]plan.Target(nil), d.targets...), nil
}

func newConfigRuntime(deps configRuntimeDependencies) *configRuntime {
	return &configRuntime{deps: deps}
}

func defaultConfigPaths() (configPaths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return configPaths{}, fmt.Errorf("resolve home directory: %w", err)
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}
	dataRoot := filepath.Join(dataHome, "moonarch")
	stateRoot := filepath.Join(stateHome, "moonarch")
	return configPaths{
		home:         home,
		dataRoot:     dataRoot,
		stateRoot:    stateRoot,
		lock:         filepath.Join(stateRoot, "lock"),
		journal:      filepath.Join(stateRoot, "journal.ndjson"),
		state:        filepath.Join(stateRoot, "state.json"),
		backupRoot:   filepath.Join(home, ".dots-backups"),
		artifacts:    filepath.Join(dataRoot, "artifacts"),
		themeCurrent: filepath.Join(home, ".local", "share", "moonarch", "themes", "current"),
	}, nil
}

func acquireConfigArtifact(ctx context.Context, tag string, deps configAcquisitionDependencies) (acquiredConfigArtifact, error) {
	if deps.resolver == nil {
		return acquiredConfigArtifact{}, errors.New("config artifact resolver is unavailable")
	}
	artifact, err := deps.resolver.Resolve(ctx, tag)
	if err != nil {
		return acquiredConfigArtifact{}, fmt.Errorf("resolve configuration release: %w", err)
	}
	if artifact.Tag != tag {
		return acquiredConfigArtifact{}, fmt.Errorf("resolved tag %q does not match requested exact tag %q", artifact.Tag, tag)
	}
	if deps.admitter == nil || deps.cache == nil {
		return acquiredConfigArtifact{}, errors.New("config artifact admission is unavailable")
	}
	if err := deps.admitter.Admit(artifact.ArchivePath, artifact.Digest); err != nil {
		return acquiredConfigArtifact{}, fmt.Errorf("admit configuration release: %w", err)
	}

	stagingRoot := filepath.Join(deps.dataRoot, "staging", artifact.Digest)
	readManifest := deps.readManifest
	if readManifest == nil {
		readManifest = readConfigManifest
	}
	manifest, err := readManifest(filepath.Join(stagingRoot, release.ManifestFilename))
	if err != nil {
		return acquiredConfigArtifact{}, fmt.Errorf("read admitted manifest: %w", err)
	}
	checkCompatibility := deps.checkCompatibility
	if checkCompatibility == nil {
		checkCompatibility = release.CheckCompatibility
	}
	if err := checkCompatibility(manifest, deps.cliVersion); err != nil {
		return acquiredConfigArtifact{}, err
	}
	if len(manifest.DependencyDecls) != 0 && deps.dependencyProbe == nil {
		return acquiredConfigArtifact{}, errors.New("declared dependency probe is unavailable")
	}
	if err := release.CheckDependencies(ctx, deps.dependencyProbe, manifest.DependencyDecls); err != nil {
		return acquiredConfigArtifact{}, err
	}
	if err := deps.cache.Promote(stagingRoot, artifact.Digest); err != nil {
		return acquiredConfigArtifact{}, fmt.Errorf("promote configuration artifact: %w", err)
	}
	root, err := deps.cache.Lookup(artifact.Digest)
	if err != nil {
		return acquiredConfigArtifact{}, fmt.Errorf("lookup promoted configuration artifact: %w", err)
	}
	return acquiredConfigArtifact{
		Identity: release.Identity{Tag: artifact.Tag, Digest: artifact.Digest},
		Root:     root,
		Manifest: manifest,
	}, nil
}

func readConfigManifest(path string) (release.Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return release.Manifest{}, err
	}
	return release.ParseManifest(data)
}

func buildConfigPlan(artifactRoot, home string, manifest release.Manifest, baseline configBaseline, reader plan.StateReader) (plan.InstallationPlan, []string, error) {
	if reader == nil {
		reader = plan.DefaultStateReader()
	}
	targets, bundles, err := discoverConfigTargets(artifactRoot, home, manifest)
	if err != nil {
		return plan.InstallationPlan{}, nil, err
	}
	themeCurrent := filepath.Join(home, ".local", "share", "moonarch", "themes", "current")
	desired := make(map[string]struct{}, len(targets))
	desiredKinds := make(map[string]plan.MutationKind, len(targets))
	for i := range targets {
		destination := filepath.Clean(targets[i].Destination)
		targets[i].Destination = destination
		desired[destination] = struct{}{}
		desiredKinds[destination] = targets[i].Kind
		targets[i].PreState = resolveConfigPreState(destination, baseline, reader)
	}
	for destination, expected := range baseline {
		destination = filepath.Clean(destination)
		if destination == themeCurrent {
			continue
		}
		if !pathWithin(home, destination) {
			return plan.InstallationPlan{}, nil, fmt.Errorf("baseline destination %q escapes home %q", destination, home)
		}
		if _, ok := desired[destination]; ok {
			continue
		}
		if baselineCoveredByDesired(destination, desiredKinds) {
			continue
		}
		targets = append(targets, plan.Target{
			Destination: destination,
			Kind:        plan.Remove,
			PreState:    expected,
		})
	}

	planner := plan.New(plan.WithDiscoverer(configDiscoverer{targets: targets}))
	configPlan, err := planner.Build(artifactRoot, home, plan.Options{})
	if err != nil {
		return plan.InstallationPlan{}, nil, err
	}
	if err := validateConfigPlan(configPlan); err != nil {
		return plan.InstallationPlan{}, nil, err
	}
	return configPlan, bundles, nil
}

// resolveConfigPreState assigns the pre-installation state of a desired target.
// An exact baseline key keeps its recorded identity; otherwise, when a baseline
// StateDirectory entry is a proper ancestor of the destination (legacy schema-1
// whole-tree management), the current filesystem state is read so the migration
// stays drift-free. Any read failure or absent destination falls back to absent.
func resolveConfigPreState(destination string, baseline configBaseline, reader plan.StateReader) plan.PreState {
	if expected, ok := baseline[destination]; ok {
		return expected
	}
	for ancestor, expected := range baseline {
		if expected.Type != plan.StateDirectory {
			continue
		}
		if !pathWithin(ancestor, destination) {
			continue
		}
		actual, err := reader.Read(destination)
		if err != nil || actual.Type == plan.StateAbsent {
			return plan.PreState{Type: plan.StateAbsent}
		}
		return actual
	}
	return plan.PreState{Type: plan.StateAbsent}
}

// baselineCoveredByDesired reports whether a baseline destination is already
// represented by the desired set: either it is a proper ancestor of a desired
// destination (removing it would destroy desired content), or it is a proper
// descendant of a desired CopyTree destination (the tree copy covers it).
func baselineCoveredByDesired(destination string, desiredKinds map[string]plan.MutationKind) bool {
	for desiredDestination, kind := range desiredKinds {
		if pathWithin(destination, desiredDestination) {
			return true
		}
		if kind == plan.CopyTree && pathWithin(desiredDestination, destination) {
			return true
		}
	}
	return false
}

func discoverConfigTargets(artifactRoot, home string, manifest release.Manifest) ([]plan.Target, []string, error) {
	var targets []plan.Target
	appendTarget := func(source, destination string) error {
		info, err := os.Lstat(source)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(artifactRoot, source)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !manifestDeclaresRoot(manifest, rel) {
			return fmt.Errorf("managed source %q is absent from the verified manifest catalog", rel)
		}
		kind, err := configMutationKind(info)
		if err != nil {
			return fmt.Errorf("managed source %q: %w", rel, err)
		}
		targets = append(targets, plan.Target{Source: source, Destination: destination, Kind: kind})
		return nil
	}

	homeRoot := filepath.Join(artifactRoot, "home")
	homeEntries, err := sortedDirectory(homeRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("discover artifact home: %w", err)
	}
	for _, entry := range homeEntries {
		switch entry.Name() {
		case ".config":
			configRoot := filepath.Join(homeRoot, ".config")
			entries, err := sortedDirectory(configRoot)
			if err != nil {
				return nil, nil, err
			}
			for _, configEntry := range entries {
				if err := appendTarget(filepath.Join(configRoot, configEntry.Name()), filepath.Join(home, ".config", configEntry.Name())); err != nil {
					return nil, nil, err
				}
			}
		case ".local":
			if err := appendTarget(
				filepath.Join(homeRoot, ".local", "bin", "moonarch"),
				filepath.Join(home, ".local", "bin", "moonarch"),
			); err != nil {
				return nil, nil, err
			}
		default:
			if err := appendTarget(filepath.Join(homeRoot, entry.Name()), filepath.Join(home, entry.Name())); err != nil {
				return nil, nil, err
			}
		}
	}

	for _, name := range []string{"fonts", "icons"} {
		if err := appendTarget(filepath.Join(artifactRoot, "assets", name), filepath.Join(home, ".local", "share", name)); err != nil {
			return nil, nil, err
		}
	}

	themesSource := filepath.Join(homeRoot, ".local", "share", "moonarch", "themes")
	themeEntries, err := sortedDirectoryIfPresent(themesSource)
	if err != nil {
		return nil, nil, err
	}
	var bundles []string
	for _, entry := range themeEntries {
		if entry.Name() == "current" {
			continue
		}
		if !validThemeBundle(entry.Name()) || !entry.IsDir() {
			return nil, nil, fmt.Errorf("invalid desired theme bundle %q", entry.Name())
		}
		bundles = append(bundles, entry.Name())
		if err := appendTarget(
			filepath.Join(themesSource, entry.Name()),
			filepath.Join(home, ".local", "share", "moonarch", "themes", entry.Name()),
		); err != nil {
			return nil, nil, err
		}
	}
	return targets, bundles, nil
}

func configMutationKind(info os.FileInfo) (plan.MutationKind, error) {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return plan.Symlink, nil
	case info.IsDir():
		return plan.CopyTree, nil
	case info.Mode().IsRegular():
		return plan.CopyFile, nil
	default:
		return "", fmt.Errorf("unsupported filesystem type %v", info.Mode())
	}
}

func manifestDeclaresRoot(manifest release.Manifest, root string) bool {
	for _, entry := range manifest.Catalog {
		clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(entry.Path)), "./")
		if clean == root || strings.HasPrefix(clean, root+"/") {
			return true
		}
	}
	return false
}

func sortedDirectory(path string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func sortedDirectoryIfPresent(path string) ([]os.DirEntry, error) {
	entries, err := sortedDirectory(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return entries, err
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func baselineFromInventory(inventory *transaction.Inventory) (configBaseline, error) {
	if inventory == nil {
		return nil, errors.New("baseline inventory is missing")
	}
	if inventory.FormatVersion != transaction.InventoryFormatVersion {
		return nil, fmt.Errorf("baseline inventory format %d is unsupported", inventory.FormatVersion)
	}
	if inventory.Lifecycle != transaction.InventoryCompleted {
		return nil, fmt.Errorf("baseline inventory %q is not completed", inventory.RunID)
	}

	baseline := make(configBaseline)
	for _, entry := range inventory.Entries {
		if entry.Target.Kind == plan.Remove {
			continue
		}
		if entry.State != transaction.EntryMutated || entry.InstalledDigest == "" {
			return nil, fmt.Errorf("baseline inventory target %q lacks a completed installed identity", entry.Target.Destination)
		}
		var stateType plan.PreStateType
		switch entry.Target.Kind {
		case plan.CopyFile:
			stateType = plan.StateFile
		case plan.CopyTree:
			stateType = plan.StateDirectory
		case plan.Symlink:
			stateType = plan.StateSymlink
		default:
			return nil, fmt.Errorf("baseline inventory target %q has unsupported kind %q", entry.Target.Destination, entry.Target.Kind)
		}
		destination := filepath.Clean(entry.Target.Destination)
		if _, duplicate := baseline[destination]; duplicate {
			return nil, fmt.Errorf("baseline inventory repeats destination %q", destination)
		}
		baseline[destination] = plan.PreState{
			Type:      stateType,
			Mode:      entry.InstalledMode,
			LinkValue: entry.LinkValue,
			Digest:    entry.InstalledDigest,
		}
	}
	return baseline, nil
}

func loadConfigBaseline(state *release.State, paths configPaths) (configBaseline, error) {
	if state == nil {
		state = &release.State{}
	}
	if state.LastCompletedRunID != "" {
		inventory, err := loadInventory(paths.backupRoot, state.LastCompletedRunID)
		if err != nil {
			return nil, err
		}
		if state.Current != nil {
			if inventory.ReleaseProvenance == nil ||
				inventory.ReleaseProvenance.Tag != state.Current.Tag ||
				inventory.ReleaseProvenance.Digest != state.Current.Digest {
				return nil, fmt.Errorf("inventory %q provenance does not match current verified identity", state.LastCompletedRunID)
			}
		}
		return baselineFromInventory(inventory)
	}
	if state.Current != nil {
		return nil, errors.New("verified current identity has no completed inventory baseline")
	}
	if paths.backupRoot == "" {
		return nil, nil
	}
	runs, err := listBackupRuns(paths.backupRoot)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		inventory, err := loadInventory(paths.backupRoot, run.ID)
		if err != nil {
			continue
		}
		baseline, err := baselineFromInventory(inventory)
		if err == nil {
			return baseline, nil
		}
	}
	return nil, nil
}

func versionIdentity(identity *release.Identity) *release.VersionIdentity {
	if identity == nil {
		return nil
	}
	return &release.VersionIdentity{Tag: identity.Tag, Digest: identity.Digest}
}

func validConfigIdentity(identity *release.Identity) bool {
	if identity == nil || !validArtifactDigest(identity.Digest) {
		return false
	}
	parsed, err := release.ParseConfigVersion(identity.Tag)
	return err == nil && parsed.Tag == identity.Tag
}

func scanArtifactRetention(artifactsRoot string, state *release.State) (retained, orphans int, err error) {
	entries, err := os.ReadDir(artifactsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, fmt.Errorf("scan retained artifacts: %w", err)
	}
	protected := make(map[string]struct{}, 2)
	if state != nil {
		for _, identity := range []*release.Identity{state.Current, state.Previous} {
			if identity != nil && identity.Digest != "" {
				protected[identity.Digest] = struct{}{}
			}
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validArtifactDigest(entry.Name()) {
			continue
		}
		if _, ok := protected[entry.Name()]; ok {
			retained++
		} else {
			orphans++
		}
	}
	return retained, orphans, nil
}

func validArtifactDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func verifyRetainedArtifact(root string, manifest release.Manifest) error {
	catalog := make(map[string]release.CatalogEntry, len(manifest.Catalog))
	for _, entry := range manifest.Catalog {
		clean, err := safeCatalogPath(entry.Path)
		if err != nil {
			return retainedArtifactError(entry.Path, err)
		}
		if _, duplicate := catalog[clean]; duplicate {
			return retainedArtifactError(clean, errors.New("duplicate catalog path"))
		}
		catalog[clean] = entry
		if err := verifyRetainedEntry(root, clean, entry); err != nil {
			return retainedArtifactError(clean, err)
		}
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == release.ManifestFilename {
			return nil
		}
		if _, declared := catalog[rel]; declared {
			return nil
		}
		if entry.IsDir() && catalogHasDescendant(catalog, rel) {
			return nil
		}
		return fmt.Errorf("undeclared cache entry %q", rel)
	})
	if err != nil {
		return retainedArtifactError("", err)
	}
	return nil
}

func verifyRetainedEntry(root, rel string, entry release.CatalogEntry) error {
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := rejectSymlinkParents(root, full); err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return err
	}
	switch entry.Kind {
	case "file":
		if !info.Mode().IsRegular() {
			return fmt.Errorf("expected regular file, got %v", info.Mode())
		}
		file, err := os.Open(full)
		if err != nil {
			return err
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if fmt.Sprintf("%x", hash.Sum(nil)) != entry.Digest {
			return errors.New("content digest mismatch")
		}
	case "dir":
		if !info.IsDir() {
			return fmt.Errorf("expected directory, got %v", info.Mode())
		}
	case "symlink":
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("expected symlink, got %v", info.Mode())
		}
		target, err := os.Readlink(full)
		if err != nil {
			return err
		}
		resolved := filepath.Clean(filepath.Join(filepath.Dir(full), target))
		if resolved != filepath.Clean(root) && !pathWithin(root, resolved) {
			return errors.New("symlink escapes retained artifact")
		}
		digest := sha256.Sum256([]byte(target))
		if fmt.Sprintf("%x", digest[:]) != entry.Digest {
			return errors.New("symlink digest mismatch")
		}
	default:
		return fmt.Errorf("unsupported catalog kind %q", entry.Kind)
	}
	if entry.Mode != 0 && int64(info.Mode().Perm()) != entry.Mode {
		return fmt.Errorf("mode mismatch: got %#o want %#o", info.Mode().Perm(), entry.Mode)
	}
	if (info.Mode().Perm()&0o111 != 0) != entry.Executable {
		return errors.New("executable classification mismatch")
	}
	return nil
}

func safeCatalogPath(raw string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(raw))
	if raw == "" || filepath.IsAbs(raw) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", errors.New("unsafe catalog path")
	}
	return clean, nil
}

func rejectSymlinkParents(root, full string) error {
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("cache entry escapes retained artifact")
	}
	current := filepath.Clean(root)
	parts := strings.Split(rel, string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("parent %q is a symlink", current)
		}
	}
	return nil
}

func catalogHasDescendant(catalog map[string]release.CatalogEntry, path string) bool {
	prefix := path + "/"
	for declared := range catalog {
		if strings.HasPrefix(declared, prefix) {
			return true
		}
	}
	return false
}

func retainedArtifactError(path string, cause error) error {
	if path == "" {
		return fmt.Errorf("%w: retained artifact verification failed: %v", release.ErrArtifactRejected, cause)
	}
	return fmt.Errorf("%w: retained artifact entry %q: %v", release.ErrArtifactRejected, path, cause)
}

func checkConfigPreflight(configPlan plan.InstallationPlan, reader plan.StateReader, candidate release.Identity, presented string) (configPreflightResult, error) {
	if reader == nil {
		return configPreflightResult{}, errors.New("config preflight state reader is unavailable")
	}
	observations := make([]release.EvidenceObservation, 0)
	for _, target := range configPlan.ManagedTargets() {
		actual, err := reader.Read(target.Destination)
		if err != nil {
			return configPreflightResult{}, fmt.Errorf("scan managed target %q: %w", target.Destination, err)
		}
		if target.PreState == actual {
			continue
		}
		class := "replacement"
		switch {
		case target.Kind == plan.Remove:
			class = "removal"
		case target.PreState.Type == plan.StateAbsent:
			class = "creation-pre"
		}
		observations = append(observations, release.EvidenceObservation{
			Path:             target.Destination,
			ExpectedIdentity: formatPreStateIdentity(target.PreState),
			ObservedIdentity: formatPreStateIdentity(actual),
			DriftClass:       class,
		})
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].Path < observations[j].Path })
	result := configPreflightResult{Observations: observations}
	if len(observations) == 0 {
		if presented != "" {
			return result, release.ErrUnboundForce
		}
		return result, nil
	}
	input := release.EvidenceTokenInput{
		Tag:            candidate.Tag,
		ArtifactDigest: candidate.Digest,
		Observations:   observations,
	}
	result.Token = release.ComputeEvidenceToken(input)
	if presented == "" {
		return result, errDriftAuthorizationRequired
	}
	if err := release.VerifyEvidenceToken(input, presented); err != nil {
		return result, err
	}
	return result, nil
}

func formatPreStateIdentity(state plan.PreState) string {
	return fmt.Sprintf("%s:mode=%#o:digest=%s:link=%s", state.Type, state.Mode, state.Digest, state.LinkValue)
}

func printDriftEvidence(out io.Writer, result configPreflightResult) {
	fmt.Fprintln(out, "configuration drift detected:")
	for _, observation := range result.Observations {
		fmt.Fprintf(out, "- %s [%s]\n  expected: %s\n  observed: %s\n",
			observation.Path, observation.DriftClass, observation.ExpectedIdentity, observation.ObservedIdentity)
	}
	if result.Token != "" {
		fmt.Fprintf(out, "authorize this exact candidate and observation set with --authorize-drift %s\n", result.Token)
	}
}

func executeConfigMutation(candidate release.Identity, prior *release.State, runID string, deps configMutationDependencies) (*release.State, error) {
	if deps.journal == nil || deps.transaction == nil || deps.writeState == nil {
		return nil, errors.New("config mutation dependencies are unavailable")
	}
	now := deps.now
	if now == nil {
		now = time.Now
	}
	appendPhase := func(phase release.JournalPhase, payload any) error {
		return deps.journal.Append(release.JournalRecord{
			OpID:    runID,
			Phase:   phase,
			Tag:     candidate.Tag,
			Digest:  candidate.Digest,
			Payload: payload,
			Ts:      now().UTC(),
		})
	}

	if err := appendPhase(release.JournalOpStart, nil); err != nil {
		return nil, err
	}
	if deps.preflight != nil {
		if err := deps.preflight(); err != nil {
			return nil, err
		}
	}
	if err := deps.transaction.Prepare(); err != nil {
		return nil, err
	}
	inventory := deps.transaction.Inventory()
	if inventory == nil {
		return nil, errors.New("prepared config transaction has no inventory")
	}
	inventory.ReleaseProvenance = &transaction.ReleaseProvenance{Tag: candidate.Tag, Digest: candidate.Digest}
	if err := appendPhase(release.JournalPrepared, inventory.Path); err != nil {
		return nil, err
	}
	if err := appendPhase(release.JournalCommitting, nil); err != nil {
		return nil, err
	}
	if err := deps.transaction.Commit(); err != nil {
		return nil, joinMutationRollback(err, deps.transaction, deps.theme)
	}
	for _, entry := range inventory.Entries {
		if entry.State != transaction.EntryMutated && entry.State != transaction.EntryRemoved {
			continue
		}
		if err := appendPhase(release.JournalMutated, entry.Target.Destination); err != nil {
			return nil, joinMutationRollback(err, deps.transaction, deps.theme)
		}
	}
	if deps.theme != nil {
		if err := deps.theme.Commit(); err != nil {
			return nil, joinMutationRollback(err, deps.transaction, deps.theme)
		}
	}
	if err := appendPhase(release.JournalCommitted, inventory.Path); err != nil {
		return nil, joinMutationRollback(err, deps.transaction, deps.theme)
	}

	next := rotatedConfigState(prior, candidate, runID)
	if err := deps.writeState(next); err != nil {
		return nil, err
	}
	if err := appendPhase(release.JournalStateFinalized, nil); err != nil {
		return nil, err
	}
	if err := appendPhase(release.JournalOpEnd, nil); err != nil {
		return nil, err
	}
	return next, nil
}

func rotatedConfigState(prior *release.State, candidate release.Identity, runID string) *release.State {
	next := &release.State{Current: &release.Identity{Tag: candidate.Tag, Digest: candidate.Digest}, LastCompletedRunID: runID}
	if prior != nil && prior.Current != nil {
		next.Previous = &release.Identity{Tag: prior.Current.Tag, Digest: prior.Current.Digest}
	}
	return next
}

func joinMutationRollback(cause error, tx configManagedTransaction, theme configThemeMutation) error {
	errs := []error{cause}
	if theme != nil {
		if err := theme.Rollback(); err != nil {
			errs = append(errs, err)
		}
	}
	if tx != nil {
		if err := tx.Rollback(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func recoverConfigJournal(deps configRecoveryDependencies) error {
	if deps.journal == nil {
		return errors.New("config journal is unavailable")
	}
	outcome, records, err := deps.journal.Recovery()
	if err != nil {
		return fmt.Errorf("recover config journal: %w", err)
	}
	if outcome == release.JournalOutcomeIndeterminate {
		return fmt.Errorf("recover config journal: %w", release.ErrIndeterminateJournal)
	}
	if len(records) == 0 || records[len(records)-1].Phase == release.JournalOpEnd {
		return nil
	}

	start := -1
	for i := len(records) - 1; i >= 0; i-- {
		if records[i].Phase == release.JournalOpStart {
			start = i
			break
		}
	}
	if start < 0 {
		return indeterminateRecoveryError("journal tail has no op-start")
	}
	tail := records[start:]
	op := tail[0]
	if op.OpID == "" || op.Tag == "" || op.Digest == "" {
		return indeterminateRecoveryError("journal op-start lacks exact operation identity")
	}
	for _, record := range tail {
		if record.OpID != op.OpID {
			return indeterminateRecoveryError("journal tail mixes operation IDs")
		}
	}

	now := deps.now
	if now == nil {
		now = time.Now
	}
	appendPhase := func(phase release.JournalPhase, payload any) error {
		return deps.journal.Append(release.JournalRecord{
			OpID:    op.OpID,
			Phase:   phase,
			Tag:     op.Tag,
			Digest:  op.Digest,
			Payload: payload,
			Ts:      now().UTC(),
		})
	}
	last := tail[len(tail)-1].Phase
	if outcome == release.JournalOutcomeUncommitted {
		if last != release.JournalOpStart {
			if deps.loadInventory == nil || deps.recoverInventory == nil {
				return indeterminateRecoveryError("uncommitted operation has no inventory recovery boundary")
			}
			inventory, err := deps.loadInventory(op.OpID)
			if err != nil {
				return indeterminateRecoveryError(fmt.Sprintf("inventory %q is unavailable: %v", op.OpID, err))
			}
			if inventory == nil || inventory.RunID != op.OpID {
				return indeterminateRecoveryError("uncommitted operation inventory identity does not match journal")
			}
			if err := deps.recoverInventory(inventory); err != nil {
				return fmt.Errorf("rollback uncommitted operation %q: %w", op.OpID, err)
			}
		}
		switch last {
		case release.JournalOpStart:
			if err := appendPhase(release.JournalPrepared, map[string]string{"recovery": "aborted-before-prepare"}); err != nil {
				return err
			}
			if err := appendPhase(release.JournalCommitting, nil); err != nil {
				return err
			}
		case release.JournalPrepared:
			if err := appendPhase(release.JournalCommitting, nil); err != nil {
				return err
			}
		case release.JournalCommitting, release.JournalMutated:
		default:
			return indeterminateRecoveryError(fmt.Sprintf("unsupported uncommitted tail phase %q", last))
		}
		if err := appendPhase(release.JournalCommitted, map[string]string{"recovery": "rolled-back"}); err != nil {
			return err
		}
		if err := appendPhase(release.JournalStateFinalized, map[string]string{"recovery": "prior-state-preserved"}); err != nil {
			return err
		}
		return appendPhase(release.JournalOpEnd, nil)
	}

	candidate := release.Identity{Tag: op.Tag, Digest: op.Digest}
	readState := deps.readState
	if readState == nil {
		readState = release.ReadState
	}
	state, err := readState(deps.statePath)
	if errors.Is(err, os.ErrNotExist) {
		state, err = &release.State{}, nil
	}
	if err != nil {
		return fmt.Errorf("read state during journal recovery: %w", err)
	}
	if state == nil {
		state = &release.State{}
	}
	switch last {
	case release.JournalCommitted:
		if state.Current != nil && *state.Current == candidate && state.LastCompletedRunID != op.OpID {
			return indeterminateRecoveryError("committed candidate identity has a mismatched completed run")
		}
		if state.Current == nil || *state.Current != candidate {
			if deps.writeState == nil {
				return indeterminateRecoveryError("committed operation cannot finalize identity")
			}
			state = rotatedConfigState(state, candidate, op.OpID)
			if err := deps.writeState(state); err != nil {
				return fmt.Errorf("finalize committed operation identity: %w", err)
			}
		}
		if err := appendPhase(release.JournalStateFinalized, map[string]string{"recovery": "committed-identity-finalized"}); err != nil {
			return err
		}
		return appendPhase(release.JournalOpEnd, nil)
	case release.JournalStateFinalized:
		if state.Current == nil || *state.Current != candidate || state.LastCompletedRunID != op.OpID {
			return indeterminateRecoveryError("state-finalized journal does not match durable state")
		}
		return appendPhase(release.JournalOpEnd, nil)
	default:
		return indeterminateRecoveryError(fmt.Sprintf("unsupported committed tail phase %q", last))
	}
}

func indeterminateRecoveryError(reason string) error {
	return fmt.Errorf("%w: %s", release.ErrIndeterminateJournal, reason)
}

func (r *configRuntime) Apply(ctx context.Context, out io.Writer, req configApplyRequest) error {
	if r.deps.initErr != nil {
		return r.deps.initErr
	}
	if r.deps.lock == nil {
		return errors.New("config lock is unavailable")
	}
	releaseLock, err := r.deps.lock.Acquire(r.deps.paths.lock)
	if err != nil {
		return err
	}
	defer releaseLock()

	if r.deps.journal == nil {
		return errors.New("config journal is unavailable")
	}
	recoveryWriteState := r.deps.writeState
	if recoveryWriteState == nil {
		recoveryWriteState = func(state *release.State) error { return state.WriteAtomic(r.deps.paths.state) }
	}
	recoverJournal := r.deps.recoverJournal
	if recoverJournal == nil {
		recoverJournal = func() error {
			return recoverConfigJournal(configRecoveryDependencies{
				journal:   r.deps.journal,
				statePath: r.deps.paths.state,
				loadInventory: func(opID string) (*transaction.Inventory, error) {
					return loadInventory(r.deps.paths.backupRoot, opID)
				},
				recoverInventory: func(inventory *transaction.Inventory) error {
					return transaction.RecoverInventory(inventory)
				},
				readState:  r.deps.readState,
				writeState: recoveryWriteState,
				now:        r.deps.now,
			})
		}
	}
	if err := recoverJournal(); err != nil {
		return err
	}

	acquire := r.deps.acquireArtifact
	if acquire == nil {
		acquire = func(ctx context.Context, tag string) (acquiredConfigArtifact, error) {
			return acquireConfigArtifact(ctx, tag, configAcquisitionDependencies{
				dataRoot:           r.deps.paths.dataRoot,
				resolver:           r.deps.resolver,
				admitter:           r.deps.admitter,
				cache:              r.deps.cache,
				readManifest:       r.deps.readManifest,
				checkCompatibility: r.deps.checkCompatibility,
				dependencyProbe:    r.deps.dependencyProbe,
				cliVersion:         r.deps.cliVersion,
			})
		}
	}
	artifact, err := acquire(ctx, req.Tag)
	if err != nil {
		return err
	}

	readState := r.deps.readState
	if readState == nil {
		readState = release.ReadState
	}
	state, err := readState(r.deps.paths.state)
	if errors.Is(err, os.ErrNotExist) {
		state = &release.State{}
		err = nil
	}
	if err != nil {
		return fmt.Errorf("read configuration state: %w", err)
	}
	if state == nil {
		state = &release.State{}
	}

	loadBaseline := r.deps.loadBaseline
	if loadBaseline == nil {
		loadBaseline = func(state *release.State) (configBaseline, error) {
			return loadConfigBaseline(state, r.deps.paths)
		}
	}
	baseline, err := loadBaseline(state)
	if err != nil {
		return err
	}
	reader := r.deps.stateReader
	if reader == nil {
		reader = plan.DefaultStateReader()
	}
	buildPlan := r.deps.buildPlan
	if buildPlan == nil {
		buildPlan = buildConfigPlan
	}
	configPlan, bundles, err := buildPlan(artifact.Root, r.deps.paths.home, artifact.Manifest, baseline, reader)
	if err != nil {
		return err
	}
	prepareTheme := r.deps.prepareTheme
	if prepareTheme == nil {
		prepareTheme = func(current string, bundles []string, replacement string) (configThemeMutation, error) {
			return prepareThemePhase(current, bundles, replacement)
		}
	}
	theme, err := prepareTheme(r.deps.paths.themeCurrent, bundles, req.ThemeReplace)
	if err != nil {
		return err
	}
	newTransaction := r.deps.newTransaction
	if newTransaction == nil {
		newTransaction = func(configPlan plan.InstallationPlan) configManagedTransaction {
			return transaction.New(configPlan)
		}
	}
	tx := newTransaction(configPlan)
	writeState := r.deps.writeState
	if writeState == nil {
		writeState = func(state *release.State) error { return state.WriteAtomic(r.deps.paths.state) }
	}

	preflight := func() error {
		result, err := checkConfigPreflight(configPlan, reader, artifact.Identity, req.AuthorizeDrift)
		if err != nil && len(result.Observations) != 0 {
			printDriftEvidence(out, result)
		}
		return err
	}
	next, err := executeConfigMutation(artifact.Identity, state, configPlan.RunID, configMutationDependencies{
		journal:     r.deps.journal,
		transaction: tx,
		theme:       theme,
		preflight:   preflight,
		writeState:  writeState,
		now:         r.deps.now,
	})
	if err != nil {
		return err
	}
	if r.deps.cache != nil {
		if err := r.deps.cache.Retain(versionIdentity(next.Current), versionIdentity(next.Previous)); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "applied %s (%s)\n", artifact.Identity.Tag, artifact.Identity.Digest)
	return nil
}

func (r *configRuntime) Rollback(ctx context.Context, out io.Writer, req configRollbackRequest) error {
	if r.deps.initErr != nil {
		return r.deps.initErr
	}
	if !req.Offline {
		return errors.New("config rollback is offline-only; --offline=false is not supported")
	}
	if r.deps.lock == nil {
		return errors.New("config lock is unavailable")
	}
	releaseLock, err := r.deps.lock.Acquire(r.deps.paths.lock)
	if err != nil {
		return err
	}
	defer releaseLock()
	if r.deps.journal == nil {
		return errors.New("config journal is unavailable")
	}

	writeState := r.deps.writeState
	if writeState == nil {
		writeState = func(state *release.State) error { return state.WriteAtomic(r.deps.paths.state) }
	}
	recoverJournal := r.deps.recoverJournal
	if recoverJournal == nil {
		recoverJournal = func() error {
			return recoverConfigJournal(configRecoveryDependencies{
				journal:   r.deps.journal,
				statePath: r.deps.paths.state,
				loadInventory: func(opID string) (*transaction.Inventory, error) {
					return loadInventory(r.deps.paths.backupRoot, opID)
				},
				recoverInventory: func(inventory *transaction.Inventory) error {
					return transaction.RecoverInventory(inventory)
				},
				readState:  r.deps.readState,
				writeState: writeState,
				now:        r.deps.now,
			})
		}
	}
	if err := recoverJournal(); err != nil {
		return err
	}

	readState := r.deps.readState
	if readState == nil {
		readState = release.ReadState
	}
	state, err := readState(r.deps.paths.state)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return release.ErrNoPreviousIdentity
		}
		return fmt.Errorf("read configuration state: %w", err)
	}
	if state == nil || !validConfigIdentity(state.Current) || !validConfigIdentity(state.Previous) {
		return release.ErrNoPreviousIdentity
	}
	if r.deps.cache == nil {
		return errors.New("config artifact cache is unavailable")
	}
	artifactRoot, err := r.deps.cache.Lookup(state.Previous.Digest)
	if err != nil {
		return err
	}
	readManifest := r.deps.readManifest
	if readManifest == nil {
		readManifest = readConfigManifest
	}
	manifest, err := readManifest(filepath.Join(artifactRoot, release.ManifestFilename))
	if err != nil {
		return fmt.Errorf("read retained manifest: %w", err)
	}
	if err := verifyRetainedArtifact(artifactRoot, manifest); err != nil {
		return err
	}
	checkCompatibility := r.deps.checkCompatibility
	if checkCompatibility == nil {
		checkCompatibility = release.CheckCompatibility
	}
	if err := checkCompatibility(manifest, r.deps.cliVersion); err != nil {
		return err
	}
	if len(manifest.DependencyDecls) != 0 && r.deps.dependencyProbe == nil {
		return errors.New("declared dependency probe is unavailable")
	}
	if err := release.CheckDependencies(ctx, r.deps.dependencyProbe, manifest.DependencyDecls); err != nil {
		return err
	}
	loadBaseline := r.deps.loadBaseline
	if loadBaseline == nil {
		loadBaseline = func(state *release.State) (configBaseline, error) {
			return loadConfigBaseline(state, r.deps.paths)
		}
	}
	baseline, err := loadBaseline(state)
	if err != nil {
		return err
	}
	reader := r.deps.stateReader
	if reader == nil {
		reader = plan.DefaultStateReader()
	}
	buildPlan := r.deps.buildPlan
	if buildPlan == nil {
		buildPlan = buildConfigPlan
	}
	configPlan, bundles, err := buildPlan(artifactRoot, r.deps.paths.home, manifest, baseline, reader)
	if err != nil {
		return err
	}
	prepareTheme := r.deps.prepareTheme
	if prepareTheme == nil {
		prepareTheme = func(current string, bundles []string, replacement string) (configThemeMutation, error) {
			return prepareThemePhase(current, bundles, replacement)
		}
	}
	theme, err := prepareTheme(r.deps.paths.themeCurrent, bundles, req.ThemeReplace)
	if err != nil {
		return err
	}
	newTransaction := r.deps.newTransaction
	if newTransaction == nil {
		newTransaction = func(configPlan plan.InstallationPlan) configManagedTransaction {
			return transaction.New(configPlan)
		}
	}
	tx := newTransaction(configPlan)
	candidate := *state.Previous
	preflight := func() error {
		result, err := checkConfigPreflight(configPlan, reader, candidate, req.AuthorizeDrift)
		if err != nil && len(result.Observations) != 0 {
			printDriftEvidence(out, result)
		}
		return err
	}
	next, err := executeConfigMutation(candidate, state, configPlan.RunID, configMutationDependencies{
		journal:     r.deps.journal,
		transaction: tx,
		theme:       theme,
		preflight:   preflight,
		writeState:  writeState,
		now:         r.deps.now,
	})
	if err != nil {
		return err
	}
	if err := r.deps.cache.Retain(versionIdentity(next.Current), versionIdentity(next.Previous)); err != nil {
		return err
	}
	fmt.Fprintf(out, "rolled back to %s (%s) offline\n", candidate.Tag, candidate.Digest)
	return nil
}

func (r *configRuntime) Status(_ context.Context, out io.Writer, req configStatusRequest) error {
	if r.deps.initErr != nil {
		return r.deps.initErr
	}
	if r.deps.lock == nil {
		return errors.New("config lock is unavailable")
	}
	releaseLock, err := r.deps.lock.Acquire(r.deps.paths.lock)
	if err != nil {
		return err
	}
	defer releaseLock()
	if r.deps.journal == nil {
		return errors.New("config journal is unavailable")
	}

	journalState := "clean"
	outcome, records, journalErr := r.deps.journal.Recovery()
	if journalErr != nil || (len(records) != 0 && records[len(records)-1].Phase != release.JournalOpEnd) {
		switch {
		case errors.Is(journalErr, release.ErrIndeterminateJournal) || outcome == release.JournalOutcomeIndeterminate:
			journalState = "unresolved (indeterminate)"
		case outcome != "":
			journalState = fmt.Sprintf("unresolved (%s)", outcome)
		default:
			journalState = "unresolved (error)"
		}
	}

	readState := r.deps.readState
	if readState == nil {
		readState = release.ReadState
	}
	state, err := readState(r.deps.paths.state)
	if errors.Is(err, os.ErrNotExist) {
		state, err = &release.State{}, nil
	}
	if err != nil {
		return fmt.Errorf("read configuration state: %w", err)
	}
	if state == nil {
		state = &release.State{}
	}
	retained, orphans, err := scanArtifactRetention(r.deps.paths.artifacts, state)
	if err != nil {
		return err
	}
	report := configStatusReport{
		Current:        "legacy/unknown",
		RetentionCount: retained,
		OrphanCount:    orphans,
		Journal:        journalState,
	}
	if state.Current != nil {
		report.Current = state.Current.Tag
		report.CurrentDigest = state.Current.Digest
	}
	if state.Previous != nil {
		report.Previous = state.Previous.Tag
		report.PreviousDigest = state.Previous.Digest
	}
	if req.JSON {
		encoder := json.NewEncoder(out)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(report)
	}
	fmt.Fprintf(out, "current: %s", report.Current)
	if report.CurrentDigest != "" {
		fmt.Fprintf(out, " (%s)", report.CurrentDigest)
	}
	fmt.Fprintln(out)
	if report.Previous != "" {
		fmt.Fprintf(out, "previous: %s (%s)\n", report.Previous, report.PreviousDigest)
	}
	fmt.Fprintf(out, "retained artifacts: %d\n", report.RetentionCount)
	fmt.Fprintf(out, "orphan artifacts: %d\n", report.OrphanCount)
	fmt.Fprintf(out, "journal: %s\n", report.Journal)
	return nil
}
