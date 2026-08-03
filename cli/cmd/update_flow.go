package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/MrUse77/dots-cli/pkg/installer"
	"github.com/MrUse77/dots-cli/pkg/installer/external"
	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/MrUse77/dots-cli/pkg/release"
	"github.com/spf13/cobra"
)

// UpdateStage identifies a stage of the update command.
type UpdateStage string

const (
	StageRelease       UpdateStage = "release"
	StageBinary        UpdateStage = "binary"
	StageRepository    UpdateStage = "repository"
	StageConfiguration UpdateStage = "configuration"
)

// StageStatus is the terminal status of a stage.
type StageStatus string

const (
	StageSucceeded StageStatus = "success"
	StageSkipped   StageStatus = "skipped"
	StageFailed    StageStatus = "failed"
)

// RollbackOutcome reports the state of a configuration rollback.
type RollbackOutcome string

const (
	RollbackNotRequired            RollbackOutcome = "not-required"
	RollbackComplete               RollbackOutcome = "complete"
	RollbackIncomplete             RollbackOutcome = "incomplete"
	RollbackManualRecoveryRequired RollbackOutcome = "manual-recovery-required"
)

// StageResult records the outcome of a single stage.
type StageResult struct {
	Stage    UpdateStage
	Status   StageStatus
	Code     string
	Detail   string
	Rollback RollbackOutcome
	Err      error
}

// UpdateResult is the ordered outcome of all four stages.
type UpdateResult struct {
	CurrentVersion               string
	LatestTag                    string
	BinaryActiveOnNextInvocation bool
	Stages                       []StageResult
}

// StageReporter receives stage lifecycle events.
type StageReporter interface {
	Start(stage UpdateStage)
	Complete(result StageResult)
}

// UpdateOrchestrator runs the four update stages.
type UpdateOrchestrator interface {
	Run(ctx context.Context, currentVersion string, reporter StageReporter) (UpdateResult, error)
}

// ConfigurationPlanBuilder builds a configuration-only plan from the acquired repository.
type ConfigurationPlanBuilder interface {
	Build(repoRoot, homeDir string) (plan.InstallationPlan, error)
}

// ConfigurationExecutorFactory constructs an executor for a configuration plan.
type ConfigurationExecutorFactory func(plan.InstallationPlan) PhaseExecutor

// updateDependenciesFactory builds the dependencies for a single invocation.
type updateDependenciesFactory func(*cobra.Command) updateDependencies

// updateDependencies holds all collaborators for the update command.
type updateDependencies struct {
	releaseClient   release.Client
	replacer        release.BinaryReplacer
	acquirer        RepositoryAcquirer
	planBuilder     ConfigurationPlanBuilder
	executorFactory ConfigurationExecutorFactory
	homeResolver    func() (string, error)
	arch            func() string
}

// StageError is a failing stage with a classified code.
type StageError struct {
	Stage UpdateStage
	Code  string
	Cause error
}

func (e *StageError) Error() string {
	return fmt.Sprintf("%s stage failed (%s): %v", e.Stage, e.Code, e.Cause)
}
func (e *StageError) Unwrap() error { return e.Cause }

// updater implements UpdateOrchestrator.
type updater struct {
	deps updateDependencies
}

// runUpdateWithDeps executes the update flow using the supplied dependencies.
func runUpdateWithDeps(cmd *cobra.Command, deps updateDependencies) error {
	reporter := newUpdateReporter(cmd.OutOrStdout(), isTTY)
	u := &updater{deps: deps}
	result, err := u.Run(cmd.Context(), Version, reporter)
	_ = result
	return err
}

// Run implements the four ordered update stages.
func (u *updater) Run(ctx context.Context, currentVersion string, reporter StageReporter) (UpdateResult, error) {
	result := UpdateResult{
		CurrentVersion: currentVersion,
		Stages: []StageResult{
			{Stage: StageRelease},
			{Stage: StageBinary},
			{Stage: StageRepository},
			{Stage: StageConfiguration},
		},
	}

	reporter.Start(StageRelease)
	latest, err := u.runRelease(ctx, currentVersion)
	if err != nil {
		result.LatestTag = latest.Tag
		result.Stages[0] = stageFailed(StageRelease, err)
		result.Stages[1] = stageSkipped(StageBinary, "blocked-by-release")
		result.Stages[2] = stageSkipped(StageRepository, "blocked-by-release")
		result.Stages[3] = stageSkipped(StageConfiguration, "blocked-by-release")
		for _, s := range result.Stages {
			reporter.Complete(s)
		}
		return result, stageErrorFrom(StageRelease, err)
	}
	result.LatestTag = latest.Tag
	result.Stages[0] = stageSucceeded(StageRelease, fmt.Sprintf("resolved %s", latest.Tag))
	reporter.Complete(result.Stages[0])

	reporter.Start(StageBinary)
	binaryResult, binaryErr := u.runBinary(ctx, currentVersion, latest)
	result.Stages[1] = binaryResult
	reporter.Complete(binaryResult)
	if binaryErr != nil {
		result.Stages[2] = stageSkipped(StageRepository, "blocked-by-binary")
		result.Stages[3] = stageSkipped(StageConfiguration, "blocked-by-binary")
		reporter.Complete(result.Stages[2])
		reporter.Complete(result.Stages[3])
		return result, stageErrorFrom(StageBinary, binaryErr)
	}
	if binaryResult.Status == StageSucceeded {
		result.BinaryActiveOnNextInvocation = true
	}

	reporter.Start(StageRepository)
	repoResult, repoErr := u.runRepository(ctx, latest)
	result.Stages[2] = repoResult
	reporter.Complete(repoResult)
	if repoErr != nil {
		result.Stages[3] = stageSkipped(StageConfiguration, "blocked-by-repository")
		reporter.Complete(result.Stages[3])
		return result, stageErrorFrom(StageRepository, repoErr)
	}

	reporter.Start(StageConfiguration)
	configResult, configErr := u.runConfiguration(ctx)
	result.Stages[3] = configResult
	reporter.Complete(configResult)
	if configErr != nil {
		return result, stageErrorFrom(StageConfiguration, configErr)
	}

	return result, nil
}

func (u *updater) runRelease(ctx context.Context, currentVersion string) (release.Release, error) {
	latest, err := u.deps.releaseClient.Latest(ctx)
	if err != nil {
		return latest, err
	}
	cmp, err := release.CompareVersions(currentVersion, latest.Tag)
	if err != nil {
		return latest, err
	}
	switch cmp {
	case release.InstalledNewer:
		return latest, errors.New("installed version is newer than the latest release")
	case release.InstalledEqual, release.InstalledOlder:
		return latest, nil
	default:
		return latest, fmt.Errorf("unknown comparison result")
	}
}

func (u *updater) runBinary(ctx context.Context, currentVersion string, latest release.Release) (StageResult, error) {
	cmp, err := release.CompareVersions(currentVersion, latest.Tag)
	if err != nil {
		return stageFailed(StageBinary, &StageError{Stage: StageBinary, Code: "version-comparison", Cause: err}), stageError(StageBinary, "version-comparison", err)
	}
	if cmp == release.InstalledEqual {
		return StageResult{
			Stage:  StageBinary,
			Status: StageSkipped,
			Code:   "already-current",
			Detail: "binary already up to date",
		}, nil
	}

	goarch := u.deps.arch()
	binaryAsset, err := release.BinaryAsset(latest, goarch)
	if err != nil {
		return stageFailed(StageBinary, err), stageErrorFrom(StageBinary, err)
	}
	checksumAsset, err := release.ChecksumAsset(latest)
	if err != nil {
		return stageFailed(StageBinary, err), stageErrorFrom(StageBinary, err)
	}

	binaryReader, err := u.deps.releaseClient.Download(ctx, binaryAsset)
	if err != nil {
		return stageFailed(StageBinary, err), stageErrorFrom(StageBinary, err)
	}
	defer binaryReader.Close()

	checksumReader, err := u.deps.releaseClient.Download(ctx, checksumAsset)
	if err != nil {
		return stageFailed(StageBinary, err), stageErrorFrom(StageBinary, err)
	}
	defer checksumReader.Close()

	homeDir, err := u.deps.homeResolver()
	if err != nil {
		return stageFailed(StageBinary, err), stageErrorFrom(StageBinary, err)
	}
	targetPath := filepath.Join(homeDir, ".local", "bin", "moonarch-cli")

	if err := u.deps.replacer.Replace(ctx, targetPath, binaryAsset.Name, binaryReader, checksumReader); err != nil {
		return stageFailed(StageBinary, err), stageErrorFrom(StageBinary, err)
	}
	return StageResult{
		Stage:  StageBinary,
		Status: StageSucceeded,
		Code:   "replaced",
		Detail: "replaced",
	}, nil
}

func (u *updater) runRepository(ctx context.Context, latest release.Release) (StageResult, error) {
	homeDir, err := u.deps.homeResolver()
	if err != nil {
		return stageFailed(StageRepository, err), stageErrorFrom(StageRepository, err)
	}
	req := RepositoryRequest{
		Destination: filepath.Join(homeDir, ".cache", "dotfiles"),
		URL:         "https://github.com/MrUse77/dotfiles.git",
		Ref:         latest.Tag,
	}
	if _, err := u.deps.acquirer.Acquire(ctx, req, io.Discard); err != nil {
		return stageFailed(StageRepository, err), stageErrorFrom(StageRepository, err)
	}
	return StageResult{
		Stage:  StageRepository,
		Status: StageSucceeded,
		Code:   "reconciled",
		Detail: fmt.Sprintf("reconciled at %s", latest.Tag),
	}, nil
}

func (u *updater) runConfiguration(ctx context.Context) (StageResult, error) {
	homeDir, err := u.deps.homeResolver()
	if err != nil {
		return stageFailed(StageConfiguration, err), stageErrorFrom(StageConfiguration, err)
	}
	repoRoot := filepath.Join(homeDir, ".cache", "dotfiles")
	cfgPlan, err := u.deps.planBuilder.Build(repoRoot, homeDir)
	if err != nil {
		return stageFailed(StageConfiguration, err), stageErrorFrom(StageConfiguration, err)
	}
	executor := u.deps.executorFactory(cfgPlan)
	rpt, err := executor.Execute(ctx, cfgPlan)
	rollback := RollbackNotRequired
	if err != nil {
		if rpt != nil {
			rollback = mapRollbackOutcome(rpt.RecoveryState)
		}
		return stageFailedWithRollback(StageConfiguration, err, rollback), stageErrorFrom(StageConfiguration, err)
	}
	if rpt != nil {
		rollback = mapRollbackOutcome(rpt.RecoveryState)
	}
	return StageResult{
		Stage:    StageConfiguration,
		Status:   StageSucceeded,
		Code:     "reapplied",
		Detail:   "configuration reapplied",
		Rollback: rollback,
	}, nil
}

func mapRollbackOutcome(state report.RecoveryState) RollbackOutcome {
	switch state {
	case report.RecoveryComplete:
		return RollbackComplete
	case report.RecoveryIncomplete:
		return RollbackIncomplete
	case report.RecoveryManualRecoveryRequired:
		return RollbackManualRecoveryRequired
	default:
		return RollbackNotRequired
	}
}

func stageErrorFrom(stage UpdateStage, err error) error {
	var se *StageError
	if errors.As(err, &se) {
		return err
	}
	return &StageError{Stage: stage, Cause: err}
}

func stageError(stage UpdateStage, code string, err error) *StageError {
	return &StageError{Stage: stage, Code: code, Cause: err}
}

func stageFailed(stage UpdateStage, err error) StageResult {
	var se *StageError
	if errors.As(err, &se) {
		return StageResult{Stage: stage, Status: StageFailed, Code: se.Code, Detail: err.Error(), Err: err}
	}
	return StageResult{Stage: stage, Status: StageFailed, Code: "unknown", Detail: err.Error(), Err: err}
}

func stageFailedWithRollback(stage UpdateStage, err error, rollback RollbackOutcome) StageResult {
	r := stageFailed(stage, err)
	r.Rollback = rollback
	return r
}

func stageSkipped(stage UpdateStage, code string) StageResult {
	return StageResult{Stage: stage, Status: StageSkipped, Code: code}
}

func stageSucceeded(stage UpdateStage, detail string) StageResult {
	return StageResult{Stage: stage, Status: StageSucceeded, Detail: detail}
}

// updateConfigurationCatalog is the narrow catalog seam used by the update command.
type updateConfigurationCatalog interface {
	plan.ActionCatalog
	plan.PhaseActionCatalog
}

// updateConfigurationPlanBuilder builds a configuration-only plan.
type updateConfigurationPlanBuilder struct {
	discoverer plan.TargetDiscoverer
	catalog    updateConfigurationCatalog
}

// newUpdateConfigurationPlanBuilder creates a configuration plan builder.
func newUpdateConfigurationPlanBuilder(discoverer plan.TargetDiscoverer, catalog updateConfigurationCatalog) ConfigurationPlanBuilder {
	return &updateConfigurationPlanBuilder{discoverer: discoverer, catalog: catalog}
}

// Build builds a configuration plan from the repository root and home directory.
func (b *updateConfigurationPlanBuilder) Build(repoRoot, homeDir string) (plan.InstallationPlan, error) {
	opts := plan.Options{Mode: "user"}
	planner := plan.New(
		plan.WithDiscoverer(b.discoverer),
		plan.WithCatalog(b.catalog),
	)
	run := planner.StartRun(opts)
	return planner.BuildConfiguration(run, repoRoot, homeDir)
}

// defaultUpdateExecutorFactory is the production executor factory.
func defaultUpdateExecutorFactory(cmd *cobra.Command) ConfigurationExecutorFactory {
	return func(p plan.InstallationPlan) PhaseExecutor {
		tx := transaction.New(p)
		return installer.NewExecutor(tx, external.NewRunner(nil).WithStdio(cmd.InOrStdin(), io.Discard, io.Discard))
	}
}

// export nothing extra.
