// Package plan models the immutable, read-only installation plan.
package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MutationKind describes how a managed target is installed.
type MutationKind string

const (
	CopyFile MutationKind = "copy-file"
	CopyTree MutationKind = "copy-tree"
	Symlink  MutationKind = "symlink"
	// Remove marks a managed target that was installed but is omitted from the
	// desired set. Remove targets carry empty Source, ResolvedSource, and
	// SourceDigest (and no SourceBinding): execution deletes the destination
	// after backup instead of consuming a source.
	Remove MutationKind = "remove"
)

// PreStateType records what existed at a target path before installation.
type PreStateType string

const (
	StateAbsent    PreStateType = "absent"
	StateFile      PreStateType = "file"
	StateDirectory PreStateType = "directory"
	StateSymlink   PreStateType = "symlink"
)

// PreState captures the pre-installation state of a managed target.
type PreState struct {
	Type      PreStateType
	Mode      os.FileMode
	LinkValue string
	Digest    string
}

// CommandSpec is a structured, non-shell external command.
type CommandSpec struct {
	Name string
	Args []string
	Dir  string
	Env  map[string]string
}

// ExternalAction represents a privileged or supply-chain operation.
type ExternalAction struct {
	Description    string
	Command        CommandSpec
	Classification string
	Irreversible   bool
	Order          int
}

// Target identifies one managed destination and its planned mutation.
type Target struct {
	Source         string
	ResolvedSource string
	// SourceDigest is empty only for legacy direct Target construction. Planner-built
	// targets always set it and SourceBinding; an empty value never disables binding
	// validation for other targets in the same plan.
	SourceDigest  string
	SourceBinding SourceBinding
	Destination   string
	Kind          MutationKind
	PreState      PreState
	BackupPath    string
}

// Feature groups selectable during installation.
const (
	GroupHyprland = "hyprland"
	GroupDev      = "dev"
	GroupCLI      = "cli"
	GroupAMD      = "amd"
	GroupSSHAgent = "ssh-agent"
	GroupPlugins  = "hypr-plugins"
	GroupTheming  = "theming"
)

// AllGroups returns every known feature group in display order.
func AllGroups() []string {
	return []string{GroupHyprland, GroupDev, GroupCLI, GroupTheming, GroupAMD, GroupPlugins}
}

// Options are the user-selected installation options.
type Options struct {
	Mode            string
	Groups          []string
	ExcludePackages []string
	HasAMD          bool
	InstallPlugins  bool
	EnableSSHAgent  bool
}

// PlanRole identifies the purpose of a plan within the installation flow.
type PlanRole string

const (
	// PlanRoleSingle is the legacy unified plan containing both managed targets and external actions.
	PlanRoleSingle PlanRole = "single"
	// PlanRolePackage is the repository-independent package phase plan.
	PlanRolePackage PlanRole = "package"
	// PlanRoleConfiguration is the repository-dependent configuration phase plan.
	PlanRoleConfiguration PlanRole = "configuration"
)

// InstallationRun is the immutable identity and option snapshot shared by all
// phase plans in a single installation attempt.
type InstallationRun struct {
	RunID   string
	Options Options
}

// HasGroup reports whether a feature group is selected.
func (o Options) HasGroup(name string) bool {
	for _, g := range o.Groups {
		if g == name {
			return true
		}
	}
	// Fall back to legacy boolean flags when no groups are set.
	if len(o.Groups) == 0 {
		switch name {
		case GroupAMD:
			return o.HasAMD
		case GroupPlugins:
			return o.InstallPlugins
		case GroupSSHAgent:
			return o.EnableSSHAgent
		}
	}
	return false
}

// InstallationPlan is the immutable, reviewed plan bound to execution.
type InstallationPlan struct {
	RunID       string
	Options     Options
	Fingerprint string
	Role        PlanRole

	managedTargets  []Target
	externalActions []ExternalAction
}

// NewInstallationPlan constructs a plan from internal direct targets. Targets with
// an empty SourceDigest retain legacy unbound execution semantics.
func NewInstallationPlan(runID string, targets []Target) (InstallationPlan, error) {
	return newInstallationPlanWithRole(runID, "", targets, nil)
}

// NewInstallationPlanWithActions constructs a plan with managed targets and reviewed external actions.
func NewInstallationPlanWithActions(runID string, targets []Target, actions []ExternalAction) (InstallationPlan, error) {
	return newInstallationPlanWithRole(runID, "", targets, actions)
}

func newInstallationPlan(runID string, targets []Target, actions []ExternalAction) (InstallationPlan, error) {
	return newInstallationPlanWithRole(runID, "", targets, actions)
}

func newInstallationPlanWithRole(runID string, role PlanRole, targets []Target, actions []ExternalAction) (InstallationPlan, error) {
	p := InstallationPlan{RunID: runID, Role: role, managedTargets: cloneTargets(targets), externalActions: cloneActions(actions)}
	fp, err := fingerprint(&p)
	if err != nil {
		return InstallationPlan{}, &FingerprintError{Cause: err}
	}
	p.Fingerprint = fp
	return p, nil
}

// ManagedTargets returns a deep copy of the plan's managed targets.
func (p InstallationPlan) ManagedTargets() []Target { return cloneTargets(p.managedTargets) }

// ExternalActions returns a deep copy of the plan's external actions.
func (p InstallationPlan) ExternalActions() []ExternalAction { return cloneActions(p.externalActions) }

// RefreshPreStates re-reads the actual state of every managed target and replaces
// each target's PreState with the observed value. It records the accepted observed
// state after an evidence-bound drift authorization, so the transaction's TOCTOU
// guard compares against what the operator accepted; any further change between
// refresh and mutation still fails the guard.
func (p *InstallationPlan) RefreshPreStates(reader StateReader) error {
	if reader == nil {
		reader = DefaultStateReader()
	}
	for i := range p.managedTargets {
		actual, err := reader.Read(p.managedTargets[i].Destination)
		if err != nil {
			return fmt.Errorf("refresh pre-state for %q: %w", p.managedTargets[i].Destination, err)
		}
		p.managedTargets[i].PreState = actual
	}
	return nil
}

// Clock provides time for deterministic tests.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// RunIDSource generates a unique run identifier.
type RunIDSource interface {
	Generate(now time.Time) string
}

// TargetDiscoverer enumerates managed targets from the repository and options.
type TargetDiscoverer interface {
	Discover(repoRoot, homeDir string, opts Options) ([]Target, error)
}

// ActionCatalog enumerates external actions from the selected options.
type ActionCatalog interface {
	ExternalActions(repoRoot, homeDir string, opts Options) ([]ExternalAction, error)
}

// PhaseActionCatalog enumerates external actions for each phase of a two-phase
// installation. Catalogs that support phase planning implement this interface
// in addition to ActionCatalog.
type PhaseActionCatalog interface {
	PackageActions(homeDir string, opts Options) ([]ExternalAction, error)
	ConfigurationActions(repoRoot, homeDir string, opts Options, managedTargets []Target) ([]ExternalAction, error)
}

// SourceOutsideRepoError is returned when a target source is not inside the repository.
type SourceOutsideRepoError struct {
	Source   string
	RepoRoot string
}

func (e *SourceOutsideRepoError) Error() string {
	return fmt.Sprintf("source %q is outside repository %q", e.Source, e.RepoRoot)
}

// DuplicateTargetError is returned when two targets share the same destination.
type DuplicateTargetError struct {
	Destination string
}

func (e *DuplicateTargetError) Error() string {
	return fmt.Sprintf("duplicate managed target %q", e.Destination)
}

// OverlappingTargetsError is returned when one target is an ancestor of another.
type OverlappingTargetsError struct {
	A string
	B string
}

func (e *OverlappingTargetsError) Error() string {
	return fmt.Sprintf("managed targets overlap: %q and %q", e.A, e.B)
}

// PlanError wraps planning-phase failures.
type PlanError struct {
	Phase string
	Cause error
}

func (e *PlanError) Error() string { return fmt.Sprintf("plan %s failed: %v", e.Phase, e.Cause) }
func (e *PlanError) Unwrap() error { return e.Cause }

type FingerprintError struct{ Cause error }

func (e *FingerprintError) Error() string { return fmt.Sprintf("fingerprint failed: %v", e.Cause) }
func (e *FingerprintError) Unwrap() error { return e.Cause }

// Option configures a Planner.
type Option func(*Planner)

// WithDiscoverer sets the target discoverer.
func WithDiscoverer(d TargetDiscoverer) Option {
	return func(p *Planner) { p.Discoverer = d }
}

// WithCatalog sets the external-action catalog.
func WithCatalog(c ActionCatalog) Option {
	return func(p *Planner) { p.Catalog = c }
}

// WithClock sets the clock.
func WithClock(c Clock) Option {
	return func(p *Planner) { p.Clock = c }
}

// WithRunIDSource sets the run-id generator.
func WithRunIDSource(r RunIDSource) Option {
	return func(p *Planner) { p.RunIDSource = r }
}

// WithStateReader sets the state reader used during planning.
func WithStateReader(s StateReader) Option {
	return func(p *Planner) { p.StateReader = s }
}

// BackupPath returns the deterministic backup location for a target.
func BackupPath(parent, runID, destination string) string {
	return filepath.Join(parent, ".dots-backups", runID, escapePath(destination))
}

func escapePath(p string) string {
	return base64PathEscape(p)
}

func base64PathEscape(p string) string {
	return hex.EncodeToString([]byte(p))
}

func cloneTargets(ts []Target) []Target {
	out := make([]Target, len(ts))
	copy(out, ts)
	return out
}

func cloneOptions(opts Options) Options {
	out := opts
	if opts.Groups != nil {
		out.Groups = make([]string, len(opts.Groups))
		copy(out.Groups, opts.Groups)
	}
	if opts.ExcludePackages != nil {
		out.ExcludePackages = make([]string, len(opts.ExcludePackages))
		copy(out.ExcludePackages, opts.ExcludePackages)
	}
	return out
}

// SourceDigestForPath returns the content digest for a regular file or directory.
func SourceDigestForPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return "", fmt.Errorf("source %q is not a regular file or directory", path)
	}
	return sourceDigest(path, info)
}

func sourceDigest(path string, info os.FileInfo) (string, error) {
	if info.IsDir() {
		return directoryDigest(path)
	}
	return fileDigest(path)
}
func cloneActions(as []ExternalAction) []ExternalAction {
	out := make([]ExternalAction, len(as))
	for i, a := range as {
		out[i] = a
		out[i].Command.Args = append([]string(nil), a.Command.Args...)
		if a.Command.Env != nil {
			out[i].Command.Env = make(map[string]string, len(a.Command.Env))
			for k, v := range a.Command.Env {
				out[i].Command.Env[k] = v
			}
		}
	}
	return out
}

// canonicalMarshal is the JSON marshaler used by fingerprint. It is a variable
// so tests can inject a failure without exporting a panic path.
var canonicalMarshal = json.Marshal

func canonicalJSON(v any) ([]byte, error) {
	return canonicalMarshal(v)
}

func fingerprint(plan *InstallationPlan) (string, error) {
	targets := make([]canonicalTarget, len(plan.managedTargets))
	for i, t := range plan.managedTargets {
		targets[i] = canonicalTarget{
			Source:         t.Source,
			ResolvedSource: t.ResolvedSource,
			SourceDigest:   t.SourceDigest,
			SourceBinding:  t.SourceBinding,
			Destination:    t.Destination,
			Kind:           string(t.Kind),
			PreState: canonicalPreState{
				Type:      string(t.PreState.Type),
				Mode:      uint32(t.PreState.Mode),
				LinkValue: t.PreState.LinkValue,
				Digest:    t.PreState.Digest,
			},
			BackupPath: t.BackupPath,
		}
	}

	actions := make([]canonicalAction, len(plan.externalActions))
	for i, a := range plan.externalActions {
		env := make([]envPair, 0, len(a.Command.Env))
		for k, v := range a.Command.Env {
			env = append(env, envPair{Key: k, Value: v})
		}
		sort.Slice(env, func(i, j int) bool { return env[i].Key < env[j].Key })

		actions[i] = canonicalAction{
			Description:    a.Description,
			Command:        canonicalCommand{Name: a.Command.Name, Args: a.Command.Args, Dir: a.Command.Dir, Env: env},
			Classification: a.Classification,
			Irreversible:   a.Irreversible,
			Order:          a.Order,
		}
	}

	input := canonicalPlan{
		RunID:   plan.RunID,
		Role:    string(plan.Role),
		Options: plan.Options,
		Targets: targets,
		Actions: actions,
	}

	data, err := canonicalJSON(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type envPair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type canonicalCommand struct {
	Name string    `json:"name"`
	Args []string  `json:"args"`
	Dir  string    `json:"dir"`
	Env  []envPair `json:"env"`
}

type canonicalPreState struct {
	Type      string `json:"type"`
	Mode      uint32 `json:"mode"`
	LinkValue string `json:"link_value"`
	Digest    string `json:"digest"`
}

type canonicalTarget struct {
	Source         string            `json:"source"`
	ResolvedSource string            `json:"resolved_source"`
	SourceDigest   string            `json:"source_digest"`
	SourceBinding  SourceBinding     `json:"source_binding"`
	Destination    string            `json:"destination"`
	Kind           string            `json:"kind"`
	PreState       canonicalPreState `json:"pre_state"`
	BackupPath     string            `json:"backup_path"`
}

type canonicalAction struct {
	Description    string           `json:"description"`
	Command        canonicalCommand `json:"command"`
	Classification string           `json:"classification"`
	Irreversible   bool             `json:"irreversible"`
	Order          int              `json:"order"`
}

type canonicalPlan struct {
	RunID   string            `json:"run_id"`
	Role    string            `json:"role,omitempty"`
	Options Options           `json:"options"`
	Targets []canonicalTarget `json:"targets"`
	Actions []canonicalAction `json:"actions"`
}

func isWithinRepo(repoRoot, source string) bool {
	rel, err := filepath.Rel(repoRoot, source)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// resolveSource returns the absolute, symlink-resolved path for source and the
// final target's file info. It rejects unreadable, dangling, or non-regular
// sources.
func resolveSource(source string) (string, os.FileInfo, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return "", nil, fmt.Errorf("source %q unreadable: %w", source, err)
	}
	resolved := source
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err = filepath.EvalSymlinks(source)
		if err != nil {
			return "", nil, fmt.Errorf("source %q resolves to unreadable target: %w", source, err)
		}
		info, err = os.Stat(resolved)
		if err != nil {
			return "", nil, fmt.Errorf("source %q resolved target %q unreadable: %w", source, resolved, err)
		}
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return "", nil, fmt.Errorf("source %q is not a regular file or directory", source)
	}
	return resolved, info, nil
}

func validateTargets(repoRoot string, targets []Target) error {
	for i := range targets {
		t := &targets[i]
		if !filepath.IsAbs(t.Destination) {
			return &PlanError{Phase: "validation", Cause: fmt.Errorf("target %d destination %q is not absolute", i, t.Destination)}
		}
		if t.Kind != Remove {
			if !filepath.IsAbs(t.Source) {
				return &PlanError{Phase: "validation", Cause: fmt.Errorf("target %d source %q is not absolute", i, t.Source)}
			}
			if !isWithinRepo(repoRoot, t.Source) {
				return &SourceOutsideRepoError{Source: t.Source, RepoRoot: repoRoot}
			}
			if !isWithinRepo(repoRoot, t.ResolvedSource) {
				return &SourceOutsideRepoError{Source: t.ResolvedSource, RepoRoot: repoRoot}
			}
		}
		if t.Kind != CopyFile && t.Kind != CopyTree && t.Kind != Symlink && t.Kind != Remove {
			return &PlanError{Phase: "validation", Cause: fmt.Errorf("target %d has unsupported mutation kind %q", i, t.Kind)}
		}
	}

	// Sort by destination for canonical validation.
	sorted := make([]Target, len(targets))
	copy(sorted, targets)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Destination < sorted[j].Destination
	})

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Destination == sorted[j].Destination {
				return &DuplicateTargetError{Destination: sorted[i].Destination}
			}
			if isAncestor(sorted[i].Destination, sorted[j].Destination) {
				return &OverlappingTargetsError{A: sorted[i].Destination, B: sorted[j].Destination}
			}
		}
	}
	return nil
}

func isAncestor(a, b string) bool {
	prefix := a + string(filepath.Separator)
	return strings.HasPrefix(b, prefix)
}
