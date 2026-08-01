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

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MrUse77/dots-cli/pkg/installer/external"
	"github.com/MrUse77/dots-cli/pkg/installer/plan"
	"github.com/MrUse77/dots-cli/pkg/installer/report"
	"github.com/MrUse77/dots-cli/pkg/installer/transaction"
	"github.com/MrUse77/dots-cli/pkg/installer/ui"
	"github.com/MrUse77/dots-cli/pkg/installer/ui/menu"
	"github.com/spf13/cobra"
)

// eventLog records named orchestration events with their position so tests can
// assert ordering and absence of pre-acceptance mutations.
type eventLog struct {
	events []string
}

func (e *eventLog) record(name string) {
	e.events = append(e.events, name)
}

func (e *eventLog) index(name string) int {
	for i, event := range e.events {
		if event == name {
			return i
		}
	}
	return -1
}

func (e *eventLog) before(a, b string) bool {
	ai, bi := e.index(a), e.index(b)
	return ai >= 0 && bi >= 0 && ai < bi
}

func (e *eventLog) contains(name string) bool {
	return e.index(name) >= 0
}

type fakeExternalRunner struct {
	log *eventLog
}

func (f *fakeExternalRunner) Run(ctx context.Context, action plan.ExternalAction) error {
	f.log.record("external:" + action.Description)
	return nil
}

func confirmProgramRunner(model tea.Model, input io.Reader, output io.Writer) (tea.Model, error) {
	switch m := model.(type) {
	case *ui.Model:
		m2, _ := m.Update(ui.ReviewRenderedMsg{})
		m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return m3, nil
	case *ui.PackageReviewModel:
		m2, _ := m.Update(ui.ReviewRenderedMsg{})
		m3, _ := m2.Update(tea.KeyMsg{Type: tea.KeyEnter})
		return m3, nil
	default:
		return model, nil
	}
}

func newFakeLocator(found bool, root string, err error) RepositoryLocator {
	return &fakeLocator{found: found, root: root, err: err}
}

type fakeLocator struct {
	found bool
	root  string
	err   error
}

func (f *fakeLocator) Locate(startDir string) (RepositoryState, error) {
	return RepositoryState{Found: f.found, Root: f.root}, f.err
}

func newFakeMenuRunner(log *eventLog, result *MenuResult, err error) MenuRunner {
	return func(input io.Reader, output io.Writer, errOutput io.Writer) (*MenuResult, error) {
		log.record("menu")
		return result, err
	}
}

func newFakePackageReviewRunner(log *eventLog, accept bool, err error) PackageReviewRunner {
	return func(ctx context.Context, p plan.InstallationPlan, details ui.TwoPhaseReviewDetails, input io.Reader, output io.Writer, run ui.ProgramRunner) (bool, error) {
		log.record("review")
		return accept, err
	}
}

func newFakeConfigurationDisplay(log *eventLog, err error) ConfigurationDisplay {
	return func(output io.Writer, p plan.InstallationPlan) error {
		log.record("display-config")
		return err
	}
}

func newFakePhaseExecutor(log *eventLog, name string, rpt *report.ExecutionReport, err error) PhaseExecutor {
	return &fakePhaseExecutor{log: log, name: name, report: rpt, err: err}
}

type fakePhaseExecutor struct {
	log    *eventLog
	name   string
	report *report.ExecutionReport
	err    error
}

func (f *fakePhaseExecutor) Execute(ctx context.Context, p plan.InstallationPlan) (*report.ExecutionReport, error) {
	f.log.record(f.name)
	return f.report, f.err
}

func newFakeAcquirer(log *eventLog, acq RepositoryAcquisition, err error) RepositoryAcquirer {
	return &fakeAcquirer{log: log, acq: acq, err: err}
}

type fakeAcquirer struct {
	log *eventLog
	acq RepositoryAcquisition
	err error
}

func (f *fakeAcquirer) Acquire(ctx context.Context, request RepositoryRequest, output io.Writer) (RepositoryAcquisition, error) {
	f.log.record("acquire")
	return f.acq, f.err
}

func newFakeReportPrinter(log *eventLog, captured **report.TwoPhaseExecutionReport) TwoPhaseReportPrinter {
	return func(w io.Writer, r *report.TwoPhaseExecutionReport) {
		log.record("print-report")
		if captured != nil {
			*captured = r
		}
	}
}

type fakePhaseCatalog struct {
	pkgActions []plan.ExternalAction
	cfgActions []plan.ExternalAction
	err        error
}

func (f *fakePhaseCatalog) ExternalActions(repoRoot, homeDir string, opts plan.Options) ([]plan.ExternalAction, error) {
	return nil, f.err
}

func (f *fakePhaseCatalog) PackageActions(homeDir string, opts plan.Options) ([]plan.ExternalAction, error) {
	return append([]plan.ExternalAction(nil), f.pkgActions...), f.err
}

func (f *fakePhaseCatalog) ConfigurationActions(repoRoot, homeDir string, opts plan.Options, managedTargets []plan.Target) ([]plan.ExternalAction, error) {
	return append([]plan.ExternalAction(nil), f.cfgActions...), f.err
}

func fakePlanner(pkgActions, cfgActions []plan.ExternalAction, cfgTargets []plan.Target) *plan.Planner {
	catalog := &fakePhaseCatalog{pkgActions: pkgActions, cfgActions: cfgActions}
	disc := &fakeTargetDiscoverer{targets: cfgTargets}
	return plan.New(
		plan.WithCatalog(catalog),
		plan.WithDiscoverer(disc),
	)
}

type fakeTargetDiscoverer struct {
	targets []plan.Target
}

func (f *fakeTargetDiscoverer) Discover(repoRoot, homeDir string, opts plan.Options) ([]plan.Target, error) {
	return append([]plan.Target(nil), f.targets...), nil
}

type fixedRunIDSource struct{}

func (fixedRunIDSource) Generate(_ interface{}) string { return "run-1" }

func TestRunInstallWithDeps_RoutesExistingCloneToLegacyFlow(t *testing.T) {
	log := &eventLog{}
	var captured *report.TwoPhaseExecutionReport
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".local", "bin", "moonarch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".local", "share", "moonarch", "themes"), 0o755); err != nil {
		t.Fatal(err)
	}

	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(true, repo, nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, &captured),
		packagePlanner:    func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		configPlanner:     func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("runInstallWithDeps error = %v", err)
	}
	if !log.contains("menu") {
		t.Fatal("legacy flow did not run menu")
	}
	if log.contains("review") {
		t.Fatal("legacy flow should not run two-phase package review")
	}
	if !log.contains("legacy-exec") {
		t.Fatal("legacy flow did not execute single-plan")
	}
	if captured != nil {
		t.Fatal("legacy flow should not print aggregate report")
	}
}

func TestRunInstallWithDeps_RoutesMissingCloneToTwoPhaseFlow(t *testing.T) {
	log := &eventLog{}
	var captured *report.TwoPhaseExecutionReport
	acqRoot := t.TempDir()

	home := t.TempDir()
	pkgPlan, err := plan.NewInstallationPlanWithActions("run-1", nil, []plan.ExternalAction{{Description: "base tools", Command: plan.CommandSpec{Name: "true"}}})
	if err != nil {
		t.Fatal(err)
	}
	cfgPlan, err := plan.NewInstallationPlanWithActions("run-1",
		[]plan.Target{{Source: filepath.Join(acqRoot, ".zshrc"), Destination: filepath.Join(home, ".zshrc"), Kind: plan.CopyFile}},
		[]plan.ExternalAction{{Description: "zsh dir", Command: plan.CommandSpec{Name: "true"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acqRoot, ".zshrc"), []byte("zsh"), 0o644); err != nil {
		t.Fatal(err)
	}

	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{Root: acqRoot, Destination: filepath.Join(home, ".cache", "dotfiles"), Ref: "main"}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, &captured),
		packagePlanner:    func() *plan.Planner { return newPlannerWithPlan(pkgPlan) },
		configPlanner:     func() *plan.Planner { return newPlannerWithPlan(cfgPlan) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}

	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("runInstallWithDeps error = %v", err)
	}
	if !log.contains("menu") || !log.contains("review") || !log.contains("package-exec") || !log.contains("acquire") || !log.contains("display-config") || !log.contains("config-exec") || !log.contains("print-report") {
		t.Fatalf("missing expected events: %v", log.events)
	}
	if captured == nil || captured.Outcome != report.OutcomeCompleted {
		t.Fatalf("unexpected aggregate report: %+v", captured)
	}
}

func TestRunInstallWithDeps_RouteIsReevaluatedPerInvocation(t *testing.T) {
	log := &eventLog{}
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".local", "bin", "moonarch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, ".local", "share", "moonarch", "themes"), 0o755); err != nil {
		t.Fatal(err)
	}

	// First invocation finds repo.
	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(true, repo, nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, nil),
		packagePlanner:    func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		configPlanner:     func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("first invocation error = %v", err)
	}
	if log.contains("review") {
		t.Fatal("first invocation should not use two-phase review")
	}

	// Second invocation loses repo.
	log.events = nil
	deps.locator = newFakeLocator(false, "", nil)
	deps.acquirer = newFakeAcquirer(log, RepositoryAcquisition{Root: t.TempDir()}, nil)
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("second invocation error = %v", err)
	}
	if !log.contains("review") {
		t.Fatal("second invocation should use two-phase review")
	}
}

func TestRunInstallWithDeps_PackageFailureStopsBeforeAcquisition(t *testing.T) {
	log := &eventLog{}
	var captured *report.TwoPhaseExecutionReport
	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{ExternalActions: []report.ActionOutcome{{Description: "base tools", Status: report.ActionFailed}}}, errors.New("package failed")),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, &captured),
		packagePlanner:    func() *plan.Planner { return fakePlanner([]plan.ExternalAction{{Description: "base tools"}}, nil, nil) },
		configPlanner:     func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err == nil {
		t.Fatal("expected package failure")
	}
	if log.contains("acquire") || log.contains("display-config") || log.contains("config-exec") {
		t.Fatalf("phases after package failure ran: %v", log.events)
	}
	if captured == nil || captured.PrimaryFailedPhase != report.PhasePackage {
		t.Fatalf("unexpected primary failed phase: %+v", captured)
	}
}

func TestRunInstallWithDeps_AcquisitionFailureStopsBeforeConfiguration(t *testing.T) {
	log := &eventLog{}
	var captured *report.TwoPhaseExecutionReport
	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{}, errors.New("network unreachable")),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, &captured),
		packagePlanner:    func() *plan.Planner { return fakePlanner([]plan.ExternalAction{{Description: "base tools"}}, nil, nil) },
		configPlanner:     func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err == nil {
		t.Fatal("expected acquisition failure")
	}
	if log.contains("display-config") || log.contains("config-exec") {
		t.Fatalf("configuration phases ran after acquisition failure: %v", log.events)
	}
	if captured == nil || captured.PrimaryFailedPhase != report.PhaseRepository {
		t.Fatalf("unexpected primary failed phase: %+v", captured)
	}
	if captured.Configuration.TransactionState != report.TransactionNotStarted {
		t.Fatalf("transaction should not start: %+v", captured.Configuration)
	}
}

func TestRunInstallWithDeps_DeclineLeavesSystemUnchanged(t *testing.T) {
	log := &eventLog{}
	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, false, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, nil),
		packagePlanner:    func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		configPlanner:     func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("runInstallWithDeps error = %v", err)
	}
	for _, forbidden := range []string{"package-exec", "acquire", "display-config", "config-exec"} {
		if log.contains(forbidden) {
			t.Fatalf("decline caused %s: %v", forbidden, log.events)
		}
	}
}

func TestRunInstallWithDeps_MenuAndReviewBeforeAnyMutation(t *testing.T) {
	log := &eventLog{}
	acqRoot := t.TempDir()
	home := t.TempDir()
	pkgPlan, err := plan.NewInstallationPlanWithActions("run-1", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfgPlan, err := plan.NewInstallationPlanWithActions("run-1",
		[]plan.Target{{Source: filepath.Join(acqRoot, ".zshrc"), Destination: filepath.Join(home, ".zshrc"), Kind: plan.CopyFile}},
		nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acqRoot, ".zshrc"), []byte("zsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{Root: acqRoot, Destination: filepath.Join(home, ".cache", "dotfiles")}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, nil),
		packagePlanner:    func() *plan.Planner { return newPlannerWithPlan(pkgPlan) },
		configPlanner:     func() *plan.Planner { return newPlannerWithPlan(cfgPlan) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("runInstallWithDeps error = %v", err)
	}
	if !log.before("menu", "review") {
		t.Fatalf("menu not before review: %v", log.events)
	}
	if !log.before("review", "package-exec") {
		t.Fatalf("review not before package execution: %v", log.events)
	}
	if !log.before("package-exec", "acquire") {
		t.Fatalf("package execution not before acquisition: %v", log.events)
	}
	if !log.before("acquire", "display-config") {
		t.Fatalf("acquisition not before configuration display: %v", log.events)
	}
	if !log.before("display-config", "config-exec") {
		t.Fatalf("configuration display not before config execution: %v", log.events)
	}
}

func TestRunInstallWithDeps_ConfigurationWithManagedTargetsRunsTransaction(t *testing.T) {
	log := &eventLog{}
	var captured *report.TwoPhaseExecutionReport
	acqRoot := t.TempDir()
	home := t.TempDir()

	pkgPlan, err := plan.NewInstallationPlanWithActions("run-1", nil, []plan.ExternalAction{{Description: "base tools", Command: plan.CommandSpec{Name: "true"}}})
	if err != nil {
		t.Fatal(err)
	}
	cfgPlan, err := plan.NewInstallationPlanWithActions("run-1", []plan.Target{{
		Source: filepath.Join(acqRoot, ".zshrc"), Destination: filepath.Join(home, ".zshrc"), Kind: plan.CopyFile,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(acqRoot, ".zshrc"), []byte("zsh"), 0o644); err != nil {
		t.Fatal(err)
	}

	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{Root: acqRoot, Destination: "/dst", Ref: "main"}, nil),
		configExecutor:    nil, // force production transaction path
		printReport:       newFakeReportPrinter(log, &captured),
		packagePlanner:    func() *plan.Planner { return newPlannerWithPlan(pkgPlan) },
		configPlanner:     func() *plan.Planner { return newPlannerWithPlan(cfgPlan) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("runInstallWithDeps error = %v", err)
	}
	if captured == nil || captured.Configuration.TransactionState != report.TransactionCompleted {
		t.Fatalf("expected completed transaction: %+v", captured)
	}
	if captured.Configuration.InventoryPath == "" {
		t.Fatal("expected inventory path")
	}
}

func TestRunInstallWithDeps_ConfigurationWithNoManagedTargetsUsesExternalOnlyExecutor(t *testing.T) {
	log := &eventLog{}
	var captured *report.TwoPhaseExecutionReport
	acqRoot := t.TempDir()

	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{Root: acqRoot}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, &captured),
		packagePlanner:    func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		configPlanner:     func() *plan.Planner { return fakePlanner(nil, []plan.ExternalAction{{Description: "zsh dir"}}, nil) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	if err := runInstallWithDeps(cmd, deps); err != nil {
		t.Fatalf("runInstallWithDeps error = %v", err)
	}
	if captured == nil || captured.Configuration.TransactionState != report.TransactionNotRequired {
		t.Fatalf("expected not-required transaction: %+v", captured)
	}
}

func TestRunInstallWithDeps_PreflightNonRepositoryFailsWithoutMutation(t *testing.T) {
	log := &eventLog{}
	home := t.TempDir()
	conflict := filepath.Join(home, ".cache", "dotfiles")
	if err := os.MkdirAll(conflict, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(conflict, "not-git"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")

	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: newFakeConfigurationDisplay(log, nil),
		packageExecutor:   newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:          newFakeAcquirer(log, RepositoryAcquisition{}, nil),
		configExecutor:    newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:    newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:       newFakeReportPrinter(log, nil),
		packagePlanner:    func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		configPlanner:     func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		runner:            &fakeExternalRunner{log: log},
		programRunner:     confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := runInstallWithDeps(cmd, deps)
	if err == nil || !strings.Contains(err.Error(), "ya existe pero no es un clon") {
		t.Fatalf("expected preflight error, got %v", err)
	}
	if log.contains("package-exec") || log.contains("acquire") {
		t.Fatalf("mutation occurred during preflight failure: %v", log.events)
	}
}

func TestRunInstallWithDeps_ContextCancelledBeforeConfigurationProducesNoTransaction(t *testing.T) {
	log := &eventLog{}
	var captured *report.TwoPhaseExecutionReport
	acqRoot := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())

	deps := installDependencies{
		out:               &bytes.Buffer{},
		errOut:            &bytes.Buffer{},
		input:             &bytes.Buffer{},
		locator:           newFakeLocator(false, "", nil),
		runMenu:           newFakeMenuRunner(log, &MenuResult{Groups: []string{plan.GroupCLI}}, nil),
		reviewPackagePlan: newFakePackageReviewRunner(log, true, nil),
		displayConfigPlan: func(output io.Writer, p plan.InstallationPlan) error {
			log.record("display-config")
			cancel()
			return nil
		},
		packageExecutor: newFakePhaseExecutor(log, "package-exec", &report.ExecutionReport{}, nil),
		acquirer:        newFakeAcquirer(log, RepositoryAcquisition{Root: acqRoot}, nil),
		configExecutor:  newFakePhaseExecutor(log, "config-exec", &report.ExecutionReport{}, nil),
		legacyExecutor:  newFakePhaseExecutor(log, "legacy-exec", &report.ExecutionReport{}, nil),
		printReport:     newFakeReportPrinter(log, &captured),
		packagePlanner:  func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		configPlanner:   func() *plan.Planner { return fakePlanner(nil, nil, nil) },
		runner:          &fakeExternalRunner{log: log},
		programRunner:   confirmProgramRunner,
	}
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	if err := runInstallWithDeps(cmd, deps); err == nil {
		t.Fatal("expected cancellation error")
	}
	if log.contains("config-exec") {
		t.Fatalf("config execution ran after cancellation: %v", log.events)
	}
	if captured == nil || captured.Configuration.TransactionState != report.TransactionNotStarted {
		t.Fatalf("expected no transaction: %+v", captured)
	}
}

func TestRunInstallWithDeps_DefaultDependenciesAreProduction(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	deps := defaultInstallDependencies(cmd)
	if deps.locator == nil {
		t.Fatal("missing locator")
	}
	if deps.runMenu == nil {
		t.Fatal("missing menu runner")
	}
	if deps.reviewPackagePlan == nil {
		t.Fatal("missing review runner")
	}
	if deps.packageExecutor == nil {
		t.Fatal("missing package executor")
	}
	if deps.acquirer == nil {
		t.Fatal("missing acquirer")
	}
	if deps.packagePlanner == nil {
		t.Fatal("missing package planner")
	}
	if deps.configPlanner == nil {
		t.Fatal("missing config planner")
	}
}

func TestNewInstallPackagePlanner_DoesNotCallDetectPowerProfiles(t *testing.T) {
	binDir := t.TempDir()
	marker := filepath.Join(binDir, "systemctl-called")
	if err := os.WriteFile(filepath.Join(binDir, "systemctl"), []byte("#!/bin/sh\ntouch "+marker+"\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "paru"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	planner := newInstallPackagePlanner()
	actions, err := planner.Catalog.(plan.PhaseActionCatalog).PackageActions(t.TempDir(), plan.Options{})
	if err != nil {
		t.Fatalf("PackageActions error = %v", err)
	}
	for _, a := range actions {
		if strings.Contains(a.Description, "power profiles") {
			t.Errorf("package actions contain power-profile action: %v", a.Description)
		}
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("package planner called systemctl before acceptance")
	}
}

// newPlannerWithPlan returns a planner that produces a fixed plan. It is useful
// for end-to-end tests that need deterministic phase plans backed by real
// source files.
func newPlannerWithPlan(p plan.InstallationPlan) *plan.Planner {
	return plan.New(
		plan.WithCatalog(&fixedPlanCatalog{plan: p}),
		plan.WithDiscoverer(&fixedPlanDiscoverer{plan: p}),
		plan.WithStateReader(&fixedStateReader{}),
	)
}

type fixedPlanCatalog struct {
	plan plan.InstallationPlan
}

func (f *fixedPlanCatalog) ExternalActions(repoRoot, homeDir string, opts plan.Options) ([]plan.ExternalAction, error) {
	return f.plan.ExternalActions(), nil
}

func (f *fixedPlanCatalog) PackageActions(homeDir string, opts plan.Options) ([]plan.ExternalAction, error) {
	return f.plan.ExternalActions(), nil
}

func (f *fixedPlanCatalog) ConfigurationActions(repoRoot, homeDir string, opts plan.Options, managedTargets []plan.Target) ([]plan.ExternalAction, error) {
	return f.plan.ExternalActions(), nil
}

type fixedPlanDiscoverer struct {
	plan plan.InstallationPlan
}

func (f *fixedPlanDiscoverer) Discover(repoRoot, homeDir string, opts plan.Options) ([]plan.Target, error) {
	return f.plan.ManagedTargets(), nil
}

type fixedStateReader struct{}

func (fixedStateReader) Read(path string) (plan.PreState, error) {
	return plan.PreState{Type: plan.StateAbsent}, nil
}

// Ensure the transaction import is used by tests that exercise the real
// transaction path.
var _ = transaction.New(plan.InstallationPlan{})
var _ = external.NewRunner(nil)
var _ = menu.Category{}
var _ = tea.KeyMsg{}

func TestRepositoryLocator_PropagatesLookupErrors(t *testing.T) {
	// A start directory that is a regular file is not a repository, but the
	// lookup failure is a real error, not proof of absence: it must propagate
	// so the missing-clone (mutating) flow never runs on lookup errors.
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	loc := repositoryLocator{}
	if _, err := loc.Locate(notADir); err == nil {
		t.Fatal("Locate() error = nil, want propagation of the lookup error")
	}
}

func TestRepositoryLocator_AbsenceIsNotAnError(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("DOTFILES_DIR", "")

	loc := repositoryLocator{}
	state, err := loc.Locate(t.TempDir())
	if err != nil {
		t.Fatalf("Locate() error = %v, want nil for absence", err)
	}
	if state.Found {
		t.Error("Found = true, want false when no repository exists anywhere")
	}
}
