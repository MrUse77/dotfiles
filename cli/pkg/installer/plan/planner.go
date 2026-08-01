package plan

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Planner builds an immutable InstallationPlan without mutation or command execution.
type Planner struct {
	Discoverer  TargetDiscoverer
	StateReader StateReader
	Catalog     ActionCatalog
	Clock       Clock
	RunIDSource RunIDSource
}

// New creates a Planner with sensible defaults and applies options.
func New(opts ...Option) *Planner {
	p := &Planner{
		StateReader: DefaultStateReader(),
		Clock:       realClock{},
		RunIDSource: defaultRunIDSource{},
		Discoverer:  emptyDiscoverer{},
		Catalog:     emptyCatalog{},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Clone returns a new Planner copied from p with additional options applied.
func (p *Planner) Clone(opts ...Option) *Planner {
	cpy := &Planner{
		Discoverer:  p.Discoverer,
		StateReader: p.StateReader,
		Catalog:     p.Catalog,
		Clock:       p.Clock,
		RunIDSource: p.RunIDSource,
	}
	for _, opt := range opts {
		opt(cpy)
	}
	return cpy
}

// Build constructs an InstallationPlan from the repository, home, and options.
func (p *Planner) Build(repoRoot, homeDir string, opts Options) (InstallationPlan, error) {
	repoRoot = filepath.Clean(repoRoot)
	homeDir = filepath.Clean(homeDir)

	now := p.Clock.Now()
	runID := p.RunIDSource.Generate(now)

	rawTargets, err := p.Discoverer.Discover(repoRoot, homeDir, opts)
	if err != nil {
		return InstallationPlan{}, &PlanError{Phase: "discovery", Cause: err}
	}

	targets, err := p.buildTargets(repoRoot, homeDir, runID, rawTargets)
	if err != nil {
		return InstallationPlan{}, err
	}

	rawActions, err := p.Catalog.ExternalActions(repoRoot, homeDir, opts)
	if err != nil {
		return InstallationPlan{}, &PlanError{Phase: "catalog", Cause: err}
	}
	actions := sortActions(rawActions)

	plan := InstallationPlan{
		RunID:           runID,
		Options:         opts,
		managedTargets:  cloneTargets(targets),
		externalActions: cloneActions(actions),
	}
	fp, err := fingerprint(&plan)
	if err != nil {
		return InstallationPlan{}, &FingerprintError{Cause: err}
	}
	plan.Fingerprint = fp
	return plan, nil
}

// StartRun allocates a run identity and freezes a deep copy of the accepted options.
func (p *Planner) StartRun(opts Options) InstallationRun {
	now := p.Clock.Now()
	return InstallationRun{
		RunID:   p.RunIDSource.Generate(now),
		Options: cloneOptions(opts),
	}
}

// BuildPackage constructs the repository-independent package phase plan.
func (p *Planner) BuildPackage(run InstallationRun, homeDir string) (InstallationPlan, error) {
	homeDir = filepath.Clean(homeDir)

	phaseCatalog, ok := p.Catalog.(PhaseActionCatalog)
	if !ok {
		return InstallationPlan{}, &PlanError{Phase: "package-catalog", Cause: errors.New("catalog does not support phase actions")}
	}

	rawActions, err := phaseCatalog.PackageActions(homeDir, run.Options)
	if err != nil {
		return InstallationPlan{}, &PlanError{Phase: "package-catalog", Cause: err}
	}
	actions := sortActions(rawActions)

	plan := InstallationPlan{
		RunID:           run.RunID,
		Options:         run.Options,
		Role:            PlanRolePackage,
		externalActions: cloneActions(actions),
	}
	fp, err := fingerprint(&plan)
	if err != nil {
		return InstallationPlan{}, &FingerprintError{Cause: err}
	}
	plan.Fingerprint = fp
	return plan, nil
}

// BuildConfiguration constructs the repository-dependent configuration phase plan.
func (p *Planner) BuildConfiguration(run InstallationRun, repoRoot, homeDir string) (InstallationPlan, error) {
	repoRoot = filepath.Clean(repoRoot)
	homeDir = filepath.Clean(homeDir)

	phaseCatalog, ok := p.Catalog.(PhaseActionCatalog)
	if !ok {
		return InstallationPlan{}, &PlanError{Phase: "configuration-catalog", Cause: errors.New("catalog does not support phase actions")}
	}

	rawTargets, err := p.Discoverer.Discover(repoRoot, homeDir, run.Options)
	if err != nil {
		return InstallationPlan{}, &PlanError{Phase: "discovery", Cause: err}
	}

	targets, err := p.buildTargets(repoRoot, homeDir, run.RunID, rawTargets)
	if err != nil {
		return InstallationPlan{}, err
	}

	managedTargets := cloneTargets(targets)
	rawActions, err := phaseCatalog.ConfigurationActions(repoRoot, homeDir, run.Options, managedTargets)
	if err != nil {
		return InstallationPlan{}, &PlanError{Phase: "configuration-catalog", Cause: err}
	}
	actions := sortActions(rawActions)

	plan := InstallationPlan{
		RunID:           run.RunID,
		Options:         run.Options,
		Role:            PlanRoleConfiguration,
		managedTargets:  cloneTargets(targets),
		externalActions: cloneActions(actions),
	}
	fp, err := fingerprint(&plan)
	if err != nil {
		return InstallationPlan{}, &FingerprintError{Cause: err}
	}
	plan.Fingerprint = fp
	return plan, nil
}

func (p *Planner) buildTargets(repoRoot, homeDir, runID string, rawTargets []Target) ([]Target, error) {
	// Index destinations so a target can be nested inside another represented directory.
	destSet := make(map[string]struct{}, len(rawTargets))
	for _, t := range rawTargets {
		dest := filepath.Clean(t.Destination)
		if !filepath.IsAbs(dest) {
			dest = filepath.Join(homeDir, dest)
		}
		destSet[dest] = struct{}{}
	}

	targets := make([]Target, 0, len(rawTargets))
	for _, t := range rawTargets {
		t.Source = filepath.Clean(t.Source)
		t.Destination = filepath.Clean(t.Destination)
		if !filepath.IsAbs(t.Source) {
			t.Source = filepath.Join(repoRoot, t.Source)
		}
		if !filepath.IsAbs(t.Destination) {
			t.Destination = filepath.Join(homeDir, t.Destination)
		}

		resolved, binding, err := buildSourceBinding(t.Source, t.Kind)
		if err != nil {
			return nil, &PlanError{Phase: "source-check", Cause: err}
		}
		t.ResolvedSource = resolved
		t.SourceDigest = binding.Digest
		t.SourceBinding = binding
		if err := validateDestinationParent(t.Destination, destSet, t.Kind); err != nil {
			return nil, &PlanError{Phase: "prerequisite", Cause: err}
		}

		if t.PreState.Type == "" {
			pre, err := p.StateReader.Read(t.Destination)
			if err != nil {
				return nil, &PlanError{Phase: "pre-state", Cause: err}
			}
			t.PreState = pre
		}

		if t.Destination == homeDir {
			return nil, &PlanError{Phase: "prerequisite", Cause: fmt.Errorf("destination %q cannot be the home directory: the backup root would live inside the target", t.Destination)}
		}

		t.BackupPath = BackupPath(homeDir, runID, t.Destination)
		targets = append(targets, t)
	}

	if err := validateTargets(repoRoot, targets); err != nil {
		return nil, err
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Destination < targets[j].Destination
	})
	return targets, nil
}

func sortActions(rawActions []ExternalAction) []ExternalAction {
	actions := make([]ExternalAction, len(rawActions))
	copy(actions, rawActions)
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Order != actions[j].Order {
			return actions[i].Order < actions[j].Order
		}
		return actions[i].Description < actions[j].Description
	})
	return actions
}

func validateDestinationParent(dest string, destSet map[string]struct{}, kind MutationKind) error {
	parent := filepath.Dir(dest)
	if _, ok := destSet[parent]; ok {
		return nil
	}
	info, err := os.Stat(parent)
	if err != nil {
		if kind == CopyTree && os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("destination parent %q inaccessible: %w", parent, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("destination parent %q is not a directory", parent)
	}
	if info.Mode().Perm()&0200 == 0 {
		return fmt.Errorf("destination parent %q is not writable: %w", parent, os.ErrPermission)
	}
	return nil
}

type emptyDiscoverer struct{}

func (emptyDiscoverer) Discover(repoRoot, homeDir string, opts Options) ([]Target, error) {
	return nil, nil
}

type emptyCatalog struct{}

func (emptyCatalog) ExternalActions(repoRoot, homeDir string, opts Options) ([]ExternalAction, error) {
	return nil, nil
}

type defaultRunIDSource struct{}

func (defaultRunIDSource) Generate(now time.Time) string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// crypto rand should never fail; fall back to timestamp-only id.
		return now.UTC().Format("20060102T150405")
	}
	return now.UTC().Format("20060102T150405") + "-" + hex.EncodeToString(b)
}
