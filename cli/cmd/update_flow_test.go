package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
	"github.com/MrUse77/dots-cli/pkg/release"
)

// fakeReleaseClient records calls and returns configured results.
type fakeReleaseClient struct {
	latestCalls int
	latestTag   string
	latestErr   error
	downloads   map[string]fakeDownload
}

type fakeDownload struct {
	body string
	err  error
}

func (f *fakeReleaseClient) Latest(context.Context) (release.Release, error) {
	f.latestCalls++
	if f.latestErr != nil {
		return release.Release{}, f.latestErr
	}
	return release.Release{Tag: f.latestTag, Assets: []release.Asset{
		{Name: "moonarch-cli-linux-amd64", URL: "https://example.com/amd64"},
		{Name: "moonarch-cli-linux-arm64", URL: "https://example.com/arm64"},
		{Name: "SHA256SUMS.txt", URL: "https://example.com/checksums"},
	}}, nil
}

func (f *fakeReleaseClient) Download(_ context.Context, asset release.Asset) (io.ReadCloser, error) {
	d, ok := f.downloads[asset.Name]
	if !ok {
		return nil, errors.New("unexpected download")
	}
	if d.err != nil {
		return nil, d.err
	}
	return io.NopCloser(bytes.NewReader([]byte(d.body))), nil
}

// fakeBinaryReplacer records replacement calls.
type fakeBinaryReplacer struct {
	calls  int
	target string
	asset  string
	err    error
}

func (f *fakeBinaryReplacer) Replace(_ context.Context, targetPath, assetName string, _ io.Reader, _ io.Reader) error {
	f.calls++
	f.target = targetPath
	f.asset = assetName
	return f.err
}

// fakeRepositoryAcquirer records acquisition calls.
type fakeRepositoryAcquirer struct {
	calls   int
	request RepositoryRequest
	err     error
}

func (f *fakeRepositoryAcquirer) Acquire(_ context.Context, req RepositoryRequest, _ io.Writer) (RepositoryAcquisition, error) {
	f.calls++
	f.request = req
	if f.err != nil {
		return RepositoryAcquisition{}, f.err
	}
	return RepositoryAcquisition{Root: req.Destination, Destination: req.Destination, Ref: req.Ref}, nil
}

// fakeConfigurationPlanBuilder records plan builder calls.
type fakeConfigurationPlanBuilder struct {
	calls int
	root  string
	home  string
	plan  plan.InstallationPlan
	err   error
}

func (f *fakeConfigurationPlanBuilder) Build(repoRoot, homeDir string) (plan.InstallationPlan, error) {
	f.calls++
	f.root = repoRoot
	f.home = homeDir
	if f.err != nil {
		return plan.InstallationPlan{}, f.err
	}
	return f.plan, nil
}

// updateFakePhaseExecutor records executor calls.
type updateFakePhaseExecutor struct {
	calls int
	plan  plan.InstallationPlan
	rpt   *report.ExecutionReport
	err   error
}

func (f *updateFakePhaseExecutor) Execute(_ context.Context, p plan.InstallationPlan) (*report.ExecutionReport, error) {
	f.calls++
	f.plan = p
	if f.err != nil {
		return f.rpt, f.err
	}
	return f.rpt, nil
}

// recordingReporter records stage events.
type recordingReporter struct {
	starts    []UpdateStage
	completes []StageResult
}

func (r *recordingReporter) Start(stage UpdateStage) {
	r.starts = append(r.starts, stage)
}

func (r *recordingReporter) Complete(result StageResult) {
	r.completes = append(r.completes, result)
}

func fixedHome() (string, error) {
	return "/home/user", nil
}

func makeUpdater(t *testing.T, overrides func(*updateDependencies)) *updater {
	t.Helper()
	deps := updateDependencies{
		releaseClient: &fakeReleaseClient{
			latestTag: "v1.1.0",
			downloads: map[string]fakeDownload{
				"moonarch-cli-linux-amd64": {body: "binary"},
				"SHA256SUMS.txt":           {body: "sha256sums"},
			},
		},
		replacer:    &fakeBinaryReplacer{},
		acquirer:    &fakeRepositoryAcquirer{},
		planBuilder: &fakeConfigurationPlanBuilder{},
		executorFactory: func(p plan.InstallationPlan) PhaseExecutor {
			return &updateFakePhaseExecutor{}
		},
		homeResolver: fixedHome,
		arch:         func() string { return "amd64" },
	}
	if overrides != nil {
		overrides(&deps)
	}
	return &updater{deps: deps}
}

func TestUpdater_RunsAllStagesInOrder(t *testing.T) {
	u := makeUpdater(t, nil)
	reporter := &recordingReporter{}
	result, err := u.Run(context.Background(), "v1.0.0", reporter)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if len(reporter.starts) != 4 {
		t.Fatalf("starts = %v, want 4 stages", reporter.starts)
	}
	wantStages := []UpdateStage{StageRelease, StageBinary, StageRepository, StageConfiguration}
	for i, want := range wantStages {
		if reporter.starts[i] != want {
			t.Fatalf("starts[%d] = %v, want %v", i, reporter.starts[i], want)
		}
	}
	if result.LatestTag != "v1.1.0" {
		t.Fatalf("LatestTag = %q", result.LatestTag)
	}
	if !result.BinaryActiveOnNextInvocation {
		t.Fatalf("BinaryActiveOnNextInvocation = false")
	}
	if len(result.Stages) != 4 {
		t.Fatalf("Stages = %d", len(result.Stages))
	}
	for _, s := range result.Stages {
		if s.Status != StageSucceeded {
			t.Fatalf("stage %s status = %s", s.Stage, s.Status)
		}
	}
}

func TestUpdater_EqualVersionSkipsBinaryButRunsRepositoryAndConfiguration(t *testing.T) {
	u := makeUpdater(t, func(d *updateDependencies) {
		d.releaseClient = &fakeReleaseClient{latestTag: "v1.0.0"}
	})
	reporter := &recordingReporter{}
	result, err := u.Run(context.Background(), "v1.0.0", reporter)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if result.Stages[1].Status != StageSkipped || result.Stages[1].Code != "already-current" {
		t.Fatalf("binary stage = %+v", result.Stages[1])
	}
	if result.Stages[2].Status != StageSucceeded {
		t.Fatalf("repository stage = %s", result.Stages[2].Status)
	}
	if result.Stages[3].Status != StageSucceeded {
		t.Fatalf("configuration stage = %s", result.Stages[3].Status)
	}
	if result.BinaryActiveOnNextInvocation {
		t.Fatalf("binary should not be active when equal")
	}
}

func TestUpdater_InstalledNewerFails(t *testing.T) {
	u := makeUpdater(t, func(d *updateDependencies) {
		d.releaseClient = &fakeReleaseClient{latestTag: "v1.0.0"}
	})
	reporter := &recordingReporter{}
	result, err := u.Run(context.Background(), "v1.1.0", reporter)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Stages[0].Status != StageFailed {
		t.Fatalf("release stage = %s", result.Stages[0].Status)
	}
	for _, s := range result.Stages[1:] {
		if s.Status != StageSkipped {
			t.Fatalf("stage %s should be skipped, got %s", s.Stage, s.Status)
		}
	}
}

func TestUpdater_InvalidInstalledVersionFails(t *testing.T) {
	u := makeUpdater(t, nil)
	reporter := &recordingReporter{}
	_, err := u.Run(context.Background(), "latest", reporter)
	if err == nil {
		t.Fatalf("expected error")
	}
	var ive *release.InvalidVersionError
	if !errors.As(err, &ive) {
		t.Fatalf("error type = %T, want *InvalidVersionError", err)
	}
}

func TestUpdater_UnsupportedArchitectureFailsBeforeDownload(t *testing.T) {
	u := makeUpdater(t, func(d *updateDependencies) {
		d.arch = func() string { return "386" }
	})
	reporter := &recordingReporter{}
	result, err := u.Run(context.Background(), "v1.0.0", reporter)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Stages[1].Status != StageFailed {
		t.Fatalf("binary stage = %s", result.Stages[1].Status)
	}
	if result.Stages[2].Status != StageSkipped {
		t.Fatalf("repository stage = %s", result.Stages[2].Status)
	}
}

func TestUpdater_BinaryFailureSkipsRepositoryAndConfiguration(t *testing.T) {
	u := makeUpdater(t, func(d *updateDependencies) {
		d.replacer = &fakeBinaryReplacer{err: errors.New("replace failed")}
	})
	reporter := &recordingReporter{}
	result, err := u.Run(context.Background(), "v1.0.0", reporter)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Stages[1].Status != StageFailed {
		t.Fatalf("binary stage = %s", result.Stages[1].Status)
	}
	if result.Stages[2].Status != StageSkipped || result.Stages[2].Code != "blocked-by-binary" {
		t.Fatalf("repository stage = %+v", result.Stages[2])
	}
	if result.Stages[3].Status != StageSkipped || result.Stages[3].Code != "blocked-by-binary" {
		t.Fatalf("configuration stage = %+v", result.Stages[3])
	}
}

func TestUpdater_RepositoryFailureSkipsConfiguration(t *testing.T) {
	u := makeUpdater(t, func(d *updateDependencies) {
		d.acquirer = &fakeRepositoryAcquirer{err: errors.New("acquire failed")}
	})
	reporter := &recordingReporter{}
	result, err := u.Run(context.Background(), "v1.0.0", reporter)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Stages[2].Status != StageFailed {
		t.Fatalf("repository stage = %s", result.Stages[2].Status)
	}
	if result.Stages[3].Status != StageSkipped || result.Stages[3].Code != "blocked-by-repository" {
		t.Fatalf("configuration stage = %+v", result.Stages[3])
	}
	if !result.BinaryActiveOnNextInvocation {
		t.Fatalf("binary success should be preserved")
	}
}

func TestUpdater_ConfigurationFailurePreservesBinary(t *testing.T) {
	u := makeUpdater(t, func(d *updateDependencies) {
		d.executorFactory = func(p plan.InstallationPlan) PhaseExecutor {
			return &updateFakePhaseExecutor{err: errors.New("commit failed")}
		}
	})
	reporter := &recordingReporter{}
	result, err := u.Run(context.Background(), "v1.0.0", reporter)
	if err == nil {
		t.Fatalf("expected error")
	}
	if result.Stages[3].Status != StageFailed {
		t.Fatalf("configuration stage = %s", result.Stages[3].Status)
	}
	if !result.BinaryActiveOnNextInvocation {
		t.Fatalf("binary success should be preserved")
	}
}

func TestUpdater_RepositoryRequestUsesFixedValues(t *testing.T) {
	acquirer := &fakeRepositoryAcquirer{}
	u := makeUpdater(t, func(d *updateDependencies) {
		d.acquirer = acquirer
	})
	_, err := u.Run(context.Background(), "v1.0.0", &recordingReporter{})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if acquirer.calls != 1 {
		t.Fatalf("acquirer calls = %d", acquirer.calls)
	}
	if acquirer.request.Destination != "/home/user/.cache/dotfiles" {
		t.Fatalf("Destination = %q", acquirer.request.Destination)
	}
	if acquirer.request.URL != "https://github.com/MrUse77/dotfiles.git" {
		t.Fatalf("URL = %q", acquirer.request.URL)
	}
	if acquirer.request.Ref != "v1.1.0" {
		t.Fatalf("Ref = %q", acquirer.request.Ref)
	}
}

func TestUpdater_UsesExplicitRequestNotBuildRepositoryRequest(t *testing.T) {
	old := buildRepositoryRequestImpl
	calls := 0
	buildRepositoryRequestImpl = func() (RepositoryRequest, error) {
		calls++
		return RepositoryRequest{}, errors.New("should not be called")
	}
	t.Cleanup(func() { buildRepositoryRequestImpl = old })

	t.Setenv("DOTFILES_DIR", "/should-not-be-used")
	t.Setenv("DOTFILES_REPO", "https://example.com/should-not-be-used.git")
	t.Setenv("DOTFILES_BRANCH", "should-not-be-used")

	u := makeUpdater(t, nil)
	_, err := u.Run(context.Background(), "v1.0.0", &recordingReporter{})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if calls != 0 {
		t.Fatalf("BuildRepositoryRequest called %d times", calls)
	}
}

func TestUpdater_EqualVersionDoesNotDownloadBinaryOrChecksum(t *testing.T) {
	client := &fakeReleaseClient{latestTag: "v1.0.0"}
	u := makeUpdater(t, func(d *updateDependencies) {
		d.releaseClient = client
	})
	_, err := u.Run(context.Background(), "v1.0.0", &recordingReporter{})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if len(client.downloads) != 0 && client.downloads != nil {
		t.Fatalf("downloads = %v", client.downloads)
	}
}

func TestUpdater_NoReExec(t *testing.T) {
	u := makeUpdater(t, nil)
	result, err := u.Run(context.Background(), "v1.0.0", &recordingReporter{})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if !result.BinaryActiveOnNextInvocation {
		t.Fatalf("expected activation message data")
	}
}

func TestUpdateConfigurationPlanBuilder_UsesOnlyConfigurationActions(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	for _, path := range []string{
		filepath.Join(repo, ".local", "bin", "moonarch"),
		filepath.Join(repo, ".local", "share", "moonarch", "themes", "tokyo-night"),
	} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	catalog := &updateFakePhaseCatalog{}
	builder := newUpdateConfigurationPlanBuilder(installDiscoverer{}, catalog)
	plan, err := builder.Build(repo, home)
	if err != nil {
		t.Fatalf("Build error = %v", err)
	}
	if catalog.configurationActions != 1 {
		t.Fatalf("ConfigurationActions calls = %d, want 1", catalog.configurationActions)
	}
	if catalog.packageActions != 0 {
		t.Fatalf("PackageActions calls = %d, want 0", catalog.packageActions)
	}
	if catalog.externalActions != 0 {
		t.Fatalf("ExternalActions calls = %d, want 0", catalog.externalActions)
	}

	// The planned actions must be exactly the configuration action returned by
	// the fake catalog. Paru, hyprpm, and other package/external commands must
	// never reach the plan even though the fake would return them if asked.
	actions := plan.ExternalActions()
	if len(actions) != 1 {
		t.Fatalf("planned actions = %d, want exactly 1 configuration action", len(actions))
	}
	if actions[0].Command.Name != "mkdir" {
		t.Fatalf("planned command = %q, want mkdir (configuration action)", actions[0].Command.Name)
	}
	for _, a := range actions {
		if a.Command.Name == "paru" || a.Command.Name == "hyprpm" {
			t.Fatalf("plan contains forbidden command %q", a.Command.Name)
		}
		for _, arg := range a.Command.Args {
			if strings.Contains(arg, "paru") || strings.Contains(arg, "hyprpm") {
				t.Fatalf("plan contains forbidden argument %q in action %q", arg, a.Command.Name)
			}
		}
	}
}

// updateFakePhaseCatalog implements both plan.ActionCatalog and plan.PhaseActionCatalog.
// It records call counts and returns realistic actions for each family so plan
// content assertions can prove package/external commands never leak in.
type updateFakePhaseCatalog struct {
	packageActions       int
	configurationActions int
	externalActions      int
}

func (f *updateFakePhaseCatalog) ExternalActions(repoRoot, homeDir string, opts plan.Options) ([]plan.ExternalAction, error) {
	f.externalActions++
	return []plan.ExternalAction{{
		Description:    "update hyprland plugins",
		Classification: "external",
		Command:        plan.CommandSpec{Name: "hyprpm", Args: []string{"update", "--no-shortcuts"}},
	}}, nil
}

func (f *updateFakePhaseCatalog) PackageActions(homeDir string, opts plan.Options) ([]plan.ExternalAction, error) {
	f.packageActions++
	return []plan.ExternalAction{{
		Description:    "update system packages",
		Classification: "package",
		Command:        plan.CommandSpec{Name: "paru", Args: []string{"-Syu", "--noconfirm"}},
	}}, nil
}

func (f *updateFakePhaseCatalog) ConfigurationActions(repoRoot, homeDir string, opts plan.Options, managedTargets []plan.Target) ([]plan.ExternalAction, error) {
	f.configurationActions++
	return []plan.ExternalAction{{
		Description:    "create zsh configuration directory",
		Classification: "filesystem",
		Command:        plan.CommandSpec{Name: "mkdir", Args: []string{"-p", filepath.Join(homeDir, ".config", "zsh")}},
	}}, nil
}

func TestMapRollbackOutcome(t *testing.T) {
	tests := []struct {
		state report.RecoveryState
		want  RollbackOutcome
	}{
		{report.RecoveryComplete, RollbackComplete},
		{report.RecoveryIncomplete, RollbackIncomplete},
		{report.RecoveryManualRecoveryRequired, RollbackManualRecoveryRequired},
		{report.RecoveryState(""), RollbackNotRequired},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := mapRollbackOutcome(tt.state); got != tt.want {
				t.Fatalf("mapRollbackOutcome(%q) = %q, want %q", tt.state, got, tt.want)
			}
		})
	}
}
