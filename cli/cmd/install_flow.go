package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MrUse77/dots-cli/pkg/installer"
	"github.com/MrUse77/dots-cli/pkg/installer/external"
	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/MrUse77/dots-cli/pkg/installer/ui"
	"github.com/MrUse77/dots-cli/pkg/installer/ui/menu"
	"github.com/spf13/cobra"
)

// RepositoryState is the result of a read-only repository lookup.
type RepositoryState struct {
	Root  string
	Found bool
}

// RepositoryLocator resolves the repository root without mutation or command
// execution.
type RepositoryLocator interface {
	Locate(startDir string) (RepositoryState, error)
}

// repositoryLocator implements the current lookup precedence: current-directory
// ancestry, DOTFILES_DIR, then $HOME/.cache/dotfiles.
type repositoryLocator struct{}

func (repositoryLocator) Locate(startDir string) (RepositoryState, error) {
	root, err := resolveRepositoryRoot(startDir)
	if err == nil {
		return RepositoryState{Root: root, Found: true}, nil
	}
	if errors.Is(err, ErrRepositoryNotFound) {
		return RepositoryState{Found: false}, nil
	}
	// A lookup error is neither proof of absence nor a safe basis for the
	// mutating missing-clone flow: propagate it.
	return RepositoryState{}, err
}

// PhaseExecutor runs a phase plan and returns a per-phase execution report.
type PhaseExecutor interface {
	Execute(context.Context, plan.InstallationPlan) (*report.ExecutionReport, error)
}

// MenuResult is the menu's output after the user confirms or cancels.
type MenuResult struct {
	Categories []menu.Category
	Groups     []string
	Excluded   []string
}

// MenuRunner displays the interactive menu and returns the confirmed selections.
type MenuRunner func(input io.Reader, output io.Writer, errOutput io.Writer) (*MenuResult, error)

// PackageReviewRunner presents the package plan for the single two-phase
// authorization and returns whether the user accepted.
type PackageReviewRunner func(
	ctx context.Context,
	p plan.InstallationPlan,
	details ui.TwoPhaseReviewDetails,
	input io.Reader,
	output io.Writer,
	run ui.ProgramRunner,
) (bool, error)

// ConfigurationDisplay renders the post-acquisition configuration plan without
// requiring input.
type ConfigurationDisplay func(output io.Writer, p plan.InstallationPlan) error

// TwoPhaseReportPrinter renders the aggregate two-phase report.
type TwoPhaseReportPrinter func(io.Writer, *report.TwoPhaseExecutionReport)

// installDependencies is the injectable boundary for the install command.
// Production wiring is supplied by the default factories; tests replace the
// implementations to prove ordering and failure boundaries.
type installDependencies struct {
	out               io.Writer
	errOut            io.Writer
	input             io.Reader
	locator           RepositoryLocator
	runMenu           MenuRunner
	reviewPackagePlan PackageReviewRunner
	displayConfigPlan ConfigurationDisplay
	packageExecutor   PhaseExecutor
	acquirer          RepositoryAcquirer
	configExecutor    PhaseExecutor
	legacyExecutor    PhaseExecutor
	packagePlanner    func() *plan.Planner
	configPlanner     func() *plan.Planner
	programRunner     ui.ProgramRunner
	runner            external.CommandRunner
	printReport       TwoPhaseReportPrinter
}

// defaultInstallDependencies wires the production command behavior.
func defaultInstallDependencies(cmd *cobra.Command) installDependencies {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	input := cmd.InOrStdin()
	return installDependencies{
		out:     out,
		errOut:  errOut,
		input:   input,
		locator: repositoryLocator{},
		runMenu: func(input io.Reader, output io.Writer, errOutput io.Writer) (*MenuResult, error) {
			categories := menu.DefaultCategories()
			m := menu.New(categories)
			p := tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(errOutput))
			final, err := p.Run()
			if err != nil {
				return nil, fmt.Errorf("menu: %w", err)
			}
			menuModel, ok := final.(menu.Model)
			if !ok {
				return nil, fmt.Errorf("menu: unexpected model type %T", final)
			}
			result := menuModel.Result()
			if result == nil {
				return &MenuResult{Categories: categories}, nil
			}
			return &MenuResult{
				Categories: result.Categories,
				Groups:     result.Groups,
				Excluded:   menu.ExcludedPackages(result.Categories),
			}, nil
		},
		reviewPackagePlan: ui.ReviewPackagePlanWithContext,
		displayConfigPlan: ui.DisplayConfigurationPlan,
		packageExecutor:   installer.NewExternalOnlyExecutor(external.NewRunner(nil).WithStdio(input, out, errOut)),
		acquirer:          NewRepositoryAcquirer(),
		configExecutor:    nil,
		packagePlanner:    newInstallPackagePlanner,
		configPlanner:     newInstallPlanner,
		programRunner:     nil,
		runner:            external.NewRunner(nil).WithStdio(input, out, errOut),
		printReport:       printTwoPhaseExecutionReport,
	}
}

// runInstallWithDeps selects the install route and coordinates both the
// existing-clone and missing-clone paths.
func runInstallWithDeps(cmd *cobra.Command, deps installDependencies) error {
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	state, err := deps.locator.Locate(workingDir)
	if err != nil {
		return fmt.Errorf("locate repository: %w", err)
	}
	if state.Found {
		return runExistingCloneInstall(cmd, deps, state.Root)
	}
	return runMissingCloneInstall(cmd, deps)
}

// runExistingCloneInstall preserves the current single-plan flow.
func runExistingCloneInstall(cmd *cobra.Command, deps installDependencies, repoRoot string) error {
	selected, err := deps.runMenu(deps.input, deps.out, deps.errOut)
	if err != nil {
		return err
	}
	if len(selected.Groups) == 0 {
		fmt.Fprintln(deps.out, "Nothing selected. Exiting.")
		return nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	options := plan.Options{
		Mode:            "user",
		Groups:          selected.Groups,
		ExcludePackages: selected.Excluded,
	}
	installationPlan, err := newInstallPlanner().Build(repoRoot, homeDir, options)
	if err != nil {
		return err
	}

	printSummary(deps.out, selected.Categories, installationPlan)

	if deps.legacyExecutor == nil {
		tx := transaction.New(installationPlan)
		deps.legacyExecutor = installer.NewExecutor(tx, deps.runner)
	}
	rpt, aborted, err := ui.RunWithContext(cmd.Context(), installationPlan, deps.legacyExecutor, deps.input, deps.out, deps.programRunner)
	if aborted {
		fmt.Fprintln(deps.out, "Installation cancelled.")
		return nil
	}
	if rpt != nil {
		printExecutionReport(deps.out, rpt)
	}
	return err
}

// runMissingCloneInstall coordinates the two-phase route. Menu choices are made
// read-only; package execution and repository acquisition happen only after the
// single authorization; configuration is built from the acquired repository and
// displayed before the managed transaction runs.
func runMissingCloneInstall(cmd *cobra.Command, deps installDependencies) error {
	selected, err := deps.runMenu(deps.input, deps.out, deps.errOut)
	if err != nil {
		return err
	}
	if len(selected.Groups) == 0 {
		fmt.Fprintln(deps.out, "Nothing selected. Exiting.")
		return nil
	}

	request, err := BuildRepositoryRequest()
	if err != nil {
		return err
	}
	if err := PreflightRepositoryDestination(request.Destination); err != nil {
		return err
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}

	options := plan.Options{
		Mode:            "user",
		Groups:          selected.Groups,
		ExcludePackages: selected.Excluded,
	}
	planner := deps.packagePlanner()
	run := planner.StartRun(options)

	pkgPlan, err := planner.BuildPackage(run, homeDir)
	if err != nil {
		return err
	}

	accepted, err := deps.reviewPackagePlan(
		cmd.Context(),
		pkgPlan,
		ui.TwoPhaseReviewDetails{
			RepositoryDestination: request.Destination,
			RepositoryRef:         request.Ref,
		},
		deps.input,
		deps.out,
		deps.programRunner,
	)
	if err != nil {
		return err
	}
	if !accepted {
		fmt.Fprintln(deps.out, "Installation cancelled.")
		return nil
	}

	aggregate := &report.TwoPhaseExecutionReport{
		RunID: run.RunID,
	}

	pkgReport, err := deps.packageExecutor.Execute(cmd.Context(), pkgPlan)
	if pkgReport != nil {
		aggregate.Package = report.PhaseExecution{
			State:           report.PhaseCompleted,
			PlanFingerprint: pkgPlan.Fingerprint,
			Report:          pkgReport,
		}
		if err != nil {
			aggregate.Package.State = report.PhaseFailed
		}
	} else if err != nil {
		aggregate.Package = report.PhaseExecution{
			State:           report.PhaseFailed,
			PlanFingerprint: pkgPlan.Fingerprint,
		}
	}
	if err != nil {
		aggregate.Outcome = report.OutcomeIncomplete
		aggregate.PrimaryFailedPhase = report.PhasePackage
		aggregate.Repository.State = report.PhaseNotStarted
		aggregate.Configuration.TransactionState = report.TransactionNotStarted
		deps.printReport(deps.out, aggregate)
		return err
	}

	acq, err := deps.acquirer.Acquire(cmd.Context(), request, deps.out)
	if err != nil {
		aggregate.Repository = report.RepositoryExecution{
			State:       report.PhaseFailed,
			Destination: request.Destination,
			Ref:         request.Ref,
			Cause:       err,
		}
		aggregate.Outcome = report.OutcomeIncomplete
		aggregate.PrimaryFailedPhase = report.PhaseRepository
		aggregate.Configuration.TransactionState = report.TransactionNotStarted
		deps.printReport(deps.out, aggregate)
		return err
	}
	aggregate.Repository = report.RepositoryExecution{
		State:       report.PhaseCompleted,
		Destination: acq.Destination,
		Ref:         acq.Ref,
	}

	cfgPlanner := deps.configPlanner()
	cfgPlan, err := cfgPlanner.BuildConfiguration(run, acq.Root, homeDir)
	if err != nil {
		aggregate.Configuration = report.ConfigurationExecution{
			PhaseExecution:   report.PhaseExecution{State: report.PhaseFailed, PlanFingerprint: ""},
			TransactionState: report.TransactionNotStarted,
		}
		aggregate.Outcome = report.OutcomeIncomplete
		aggregate.PrimaryFailedPhase = report.PhaseConfiguration
		deps.printReport(deps.out, aggregate)
		return err
	}

	if err := deps.displayConfigPlan(deps.out, cfgPlan); err != nil {
		return err
	}
	if err := cmd.Context().Err(); err != nil {
		aggregate.Configuration = report.ConfigurationExecution{
			PhaseExecution:   report.PhaseExecution{State: report.PhaseCancelled, PlanFingerprint: cfgPlan.Fingerprint},
			TransactionState: report.TransactionNotStarted,
		}
		aggregate.Outcome = report.OutcomeCancelled
		deps.printReport(deps.out, aggregate)
		return err
	}

	var cfgReport *report.ExecutionReport
	if len(cfgPlan.ManagedTargets()) == 0 {
		executor := installer.NewExternalOnlyExecutor(deps.runner)
		cfgReport, err = executor.Execute(cmd.Context(), cfgPlan)
		aggregate.Configuration = report.ConfigurationExecution{
			PhaseExecution:   report.PhaseExecution{State: report.PhaseCompleted, PlanFingerprint: cfgPlan.Fingerprint, Report: cfgReport},
			TransactionState: report.TransactionNotRequired,
		}
		if err != nil {
			aggregate.Configuration.State = report.PhaseFailed
			aggregate.Outcome = report.OutcomeIncomplete
			aggregate.PrimaryFailedPhase = report.PhaseConfiguration
		}
	} else {
		if deps.configExecutor == nil {
			tx := transaction.New(cfgPlan)
			deps.configExecutor = installer.NewExecutor(tx, external.NewRunner(nil).WithStdio(deps.input, deps.out, deps.errOut))
		}
		cfgReport, err = deps.configExecutor.Execute(cmd.Context(), cfgPlan)
		aggregate.Configuration = report.ConfigurationExecution{
			PhaseExecution:   report.PhaseExecution{State: report.PhaseCompleted, PlanFingerprint: cfgPlan.Fingerprint, Report: cfgReport},
			TransactionState: report.TransactionCompleted,
		}
		if cfgReport != nil && cfgReport.InventoryPath != "" {
			aggregate.Configuration.InventoryPath = cfgReport.InventoryPath
		}
		if err != nil {
			aggregate.Configuration.State = report.PhaseFailed
			aggregate.Outcome = report.OutcomeIncomplete
			aggregate.PrimaryFailedPhase = report.PhaseConfiguration
		}
	}

	if aggregate.Outcome == "" {
		aggregate.Outcome = report.OutcomeCompleted
	}
	deps.printReport(deps.out, aggregate)
	return err
}

// runInstall is the Cobra entry point. It delegates to runInstallWithDeps with
// production dependencies.
func runInstall(cmd *cobra.Command, _ []string) error {
	return runInstallWithDeps(cmd, defaultInstallDependencies(cmd))
}

// newInstallPackagePlanner returns a planner for the repository-independent
// package phase. It deliberately does not invoke DetectPowerProfiles so no
// system command runs before the user accepts the plan.
func newInstallPackagePlanner() *plan.Planner {
	return plan.New(
		plan.WithCatalog(installer.NewActionCatalogWithParu(installer.DetectParu())),
	)
}
