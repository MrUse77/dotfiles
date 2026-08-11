package release

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

// stubDependencyProbe returns canned results per dependency and records every
// probe call, proving the check layer's only execution surface.
type stubDependencyProbe struct {
	results map[string]DependencyResult
	errs    map[string]error
	calls   []string
}

func (s *stubDependencyProbe) Probe(_ context.Context, name, constraint string) (DependencyResult, error) {
	s.calls = append(s.calls, name+" "+constraint)
	if err := s.errs[name]; err != nil {
		return DependencyResult{}, err
	}
	res := s.results[name]
	res.Name = name
	return res, nil
}

func deps(names ...string) []DependencyDecl {
	decls := make([]DependencyDecl, 0, len(names))
	for _, n := range names {
		decls = append(decls, DependencyDecl{Name: n, Constraint: ">= 1.0"})
	}
	return decls
}

func satisfied(name string) DependencyResult {
	return DependencyResult{Name: name, Satisfied: true, Observed: "/usr/bin/" + name}
}

func TestCheckDependencies_PassesWhenAllSatisfied(t *testing.T) {
	probe := &stubDependencyProbe{results: map[string]DependencyResult{
		"zsh": satisfied("zsh"), "tmux": satisfied("tmux"),
	}}
	decls := deps("zsh", "tmux")
	if err := CheckDependencies(context.Background(), probe, decls); err != nil {
		t.Fatalf("CheckDependencies() error = %v, want nil", err)
	}
	if len(probe.calls) != 2 {
		t.Fatalf("probe calls = %v, want exactly 2", probe.calls)
	}
}

func TestCheckDependencies_FailsClosedOnMissingDependency(t *testing.T) {
	probe := &stubDependencyProbe{results: map[string]DependencyResult{
		"zsh": satisfied("zsh"), "hyprpm": {Satisfied: false},
	}}
	err := CheckDependencies(context.Background(), probe, deps("zsh", "hyprpm"))
	var ue *UnsatisfiedDependencyError
	if !errors.As(err, &ue) {
		t.Fatalf("error type = %T, want *UnsatisfiedDependencyError", err)
	}
	if len(ue.Requirements) != 1 || ue.Requirements[0].Name != "hyprpm" {
		t.Fatalf("unsatisfied requirements = %+v, want only hyprpm", ue.Requirements)
	}
}

func TestCheckDependencies_FailsClosedOnUnknownRequirement(t *testing.T) {
	// Unknown = Satisfied=false with nothing observed (Observed empty).
	probe := &stubDependencyProbe{results: map[string]DependencyResult{
		"mystery": {Satisfied: false},
	}}
	err := CheckDependencies(context.Background(), probe, deps("mystery"))
	var ue *UnsatisfiedDependencyError
	if !errors.As(err, &ue) {
		t.Fatalf("error type = %T, want *UnsatisfiedDependencyError", err)
	}
	if ue.Requirements[0].Name != "mystery" || ue.Requirements[0].Observed != "" {
		t.Fatalf("unknown requirement not reported as unknown: %+v", ue.Requirements[0])
	}
}

func TestCheckDependencies_FailsClosedOnProbeError(t *testing.T) {
	probe := &stubDependencyProbe{
		results: map[string]DependencyResult{"zsh": satisfied("zsh")},
		errs:    map[string]error{"nix": errors.New("probe unavailable")},
	}
	err := CheckDependencies(context.Background(), probe, deps("zsh", "nix"))
	var ue *UnsatisfiedDependencyError
	if !errors.As(err, &ue) {
		t.Fatalf("error type = %T, want *UnsatisfiedDependencyError", err)
	}
	if len(ue.Requirements) != 1 || ue.Requirements[0].Name != "nix" {
		t.Fatalf("unsatisfied requirements = %+v, want only nix", ue.Requirements)
	}
}

func TestCheckDependencies_EmptyDeclsAreNoOp(t *testing.T) {
	probe := &stubDependencyProbe{results: map[string]DependencyResult{}}
	if err := CheckDependencies(context.Background(), probe, nil); err != nil {
		t.Fatalf("CheckDependencies(nil) error = %v, want nil", err)
	}
	if len(probe.calls) != 0 {
		t.Fatalf("probe called %d times for empty decls", len(probe.calls))
	}
}

// TestCheckDependencies_ExecutesNoCommands proves the check layer never runs a
// process: the stub probe is the only execution surface, and CheckDependencies
// records only Probe invocations — no CommandSpec/exec boundary exists in it.
func TestCheckDependencies_ExecutesNoCommands(t *testing.T) {
	probe := &stubDependencyProbe{results: map[string]DependencyResult{
		"zsh": satisfied("zsh"),
	}}
	if err := CheckDependencies(context.Background(), probe, deps("zsh")); err != nil {
		t.Fatalf("CheckDependencies() error = %v", err)
	}
	if len(probe.calls) != 1 || probe.calls[0] != "zsh >= 1.0" {
		t.Fatalf("calls = %v, want single probe invocation with name+constraint", probe.calls)
	}
}

func TestOSDependencyProbe_ReadOnlyPresenceCheck(t *testing.T) {
	var lookups []string
	probe := &OSDependencyProbe{lookupPath: func(name string) (string, error) {
		lookups = append(lookups, name)
		if name == "zsh" {
			return "/usr/bin/zsh", nil
		}
		return "", os.ErrNotExist
	}}

	res, err := probe.Probe(context.Background(), "zsh", ">= 5.0")
	if err != nil {
		t.Fatalf("Probe(zsh) error = %v", err)
	}
	if !res.Satisfied || res.Observed != "/usr/bin/zsh" {
		t.Fatalf("Probe(zsh) = %+v, want satisfied with /usr/bin/zsh", res)
	}

	res, err = probe.Probe(context.Background(), "tmux", ">= 3.0")
	if err != nil {
		t.Fatalf("Probe(tmux) error = %v", err)
	}
	if res.Satisfied || res.Observed != "" {
		t.Fatalf("Probe(tmux) = %+v, want unsatisfied with empty observation", res)
	}

	// The only "execution" was the injected PATH lookup — no command ran.
	if len(lookups) != 2 {
		t.Fatalf("lookups = %v, want exactly 2 PATH lookups and no commands", lookups)
	}
}

func TestOSDependencyProbe_RespectsCancelledContext(t *testing.T) {
	probe := &OSDependencyProbe{lookupPath: func(name string) (string, error) { return "/usr/bin/zsh", nil }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := probe.Probe(ctx, "zsh", ""); err == nil {
		t.Fatalf("Probe() error = nil with cancelled context")
	}
}

// Assert DependencyDecl round-trips through the manifest JSON.
func TestManifest_DependencyDeclsRoundTrip(t *testing.T) {
	m := Manifest{
		SchemaVersion: "1",
		DependencyDecls: []DependencyDecl{
			{Name: "zsh", Constraint: ">= 5.0"},
			{Name: "git"},
		},
		Catalog: []CatalogEntry{},
	}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	parsed, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("ParseManifest() error = %v", err)
	}
	if len(parsed.DependencyDecls) != 2 ||
		parsed.DependencyDecls[0].Name != "zsh" || parsed.DependencyDecls[0].Constraint != ">= 5.0" ||
		parsed.DependencyDecls[1].Name != "git" || parsed.DependencyDecls[1].Constraint != "" {
		t.Fatalf("dependency decls = %+v, want zsh/>= 5.0 and git", parsed.DependencyDecls)
	}
}
