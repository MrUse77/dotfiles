package release

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// DependencyProbe is the read-only boundary for declared external dependency
// checks. Implementations MUST NOT install, update, remove, or claim rollback
// of packages, services, or other external state; Probe observes and reports.
type DependencyProbe interface {
	// Probe reports whether name satisfies constraint. Satisfied=false with an
	// empty Observed means the requirement is unknown or missing and must
	// fail closed.
	Probe(ctx context.Context, name, constraint string) (DependencyResult, error)
}

// DependencyResult reports the outcome of one dependency probe.
type DependencyResult struct {
	Name      string
	Satisfied bool
	Observed  string // empty when Satisfied=false and nothing was observed
}

// UnsatisfiedDependencyError identifies every declared dependency that failed
// its probe. The system fails closed on missing, incompatible, or unknown
// requirements, and the error names each one.
type UnsatisfiedDependencyError struct {
	Requirements []DependencyResult
}

func (e *UnsatisfiedDependencyError) Error() string {
	names := make([]string, 0, len(e.Requirements))
	for _, r := range e.Requirements {
		names = append(names, r.Name)
	}
	return fmt.Sprintf("unsatisfied declared dependencies: %s", strings.Join(names, ", "))
}

// CheckDependencies probes every declared dependency and fails closed when any
// is missing, incompatible, or unknown (probe error or Satisfied=false). It
// executes no commands itself: all probing is delegated to the injectable
// DependencyProbe, so callers can substitute stubs and this path never touches
// a process.
func CheckDependencies(ctx context.Context, probe DependencyProbe, decls []DependencyDecl) error {
	var unsatisfied []DependencyResult
	for _, d := range decls {
		res, err := probe.Probe(ctx, d.Name, d.Constraint)
		if err != nil {
			unsatisfied = append(unsatisfied, DependencyResult{Name: d.Name})
			continue
		}
		if !res.Satisfied {
			unsatisfied = append(unsatisfied, res)
		}
	}
	if len(unsatisfied) > 0 {
		return &UnsatisfiedDependencyError{Requirements: unsatisfied}
	}
	return nil
}

// OSDependencyProbe is the production read-only probe. It resolves the
// dependency's presence in PATH with exec.LookPath — a pure lookup that
// executes nothing — and never runs privileged commands or the installer
// Runner/CommandSpec machinery. Constraint evaluation is delegated to the
// probe boundary so a stricter probe can substitute version observation.
type OSDependencyProbe struct {
	lookupPath func(name string) (string, error) // nil → exec.LookPath
}

// NewOSDependencyProbe creates the default read-only dependency probe.
func NewOSDependencyProbe() *OSDependencyProbe { return &OSDependencyProbe{} }

// Probe reports presence in PATH. A dependency that cannot be resolved is
// reported unsatisfied (unknown), never as an error, so callers fail closed.
func (p *OSDependencyProbe) Probe(ctx context.Context, name, constraint string) (DependencyResult, error) {
	if err := ctx.Err(); err != nil {
		return DependencyResult{}, err
	}
	lookup := p.lookupPath
	if lookup == nil {
		lookup = exec.LookPath
	}
	path, err := lookup(name)
	if err != nil {
		return DependencyResult{Name: name, Satisfied: false}, nil
	}
	return DependencyResult{Name: name, Satisfied: true, Observed: path}, nil
}
